package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/k8s"
	"github.com/fslongjin/liteboxd/backend/internal/store"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	defaultDeletionRetryInterval  = 10 * time.Second
	maxDeletionRetryInterval      = 60 * time.Second
	podDeleteTimeout              = 45 * time.Second
	deploymentDeleteTimeout       = 60 * time.Second
	pvcDeleteTimeout              = 2 * time.Minute
	pvcForceCleanupThreshold      = 5 * time.Minute
	storageAttachmentCleanupAfter = 15 * time.Minute
)

type SandboxDeletionService struct {
	k8sClient    *k8s.Client
	sandboxStore *store.SandboxStore
}

func NewSandboxDeletionService(k8sClient *k8s.Client, sandboxStore *store.SandboxStore) *SandboxDeletionService {
	return &SandboxDeletionService{
		k8sClient:    k8sClient,
		sandboxStore: sandboxStore,
	}
}

func (s *SandboxDeletionService) Start(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			if err := s.RunPending(context.Background()); err != nil {
				slog.Default().With("component", "sandbox_deletion").Error("scheduled deletion run failed", "error", err)
			}
		}
	}()
}

func (s *SandboxDeletionService) RunPending(ctx context.Context) error {
	records, err := s.sandboxStore.ListPendingDeletion(ctx, time.Now().UTC())
	if err != nil {
		return err
	}
	for i := range records {
		if err := s.processSandbox(ctx, &records[i]); err != nil {
			logWithSandboxID(ctx, records[i].ID).Warn("deletion reconcile failed", "error", err)
		}
	}
	return nil
}

func (s *SandboxDeletionService) processSandbox(ctx context.Context, rec *store.SandboxRecord) error {
	now := time.Now().UTC()
	retryAt := now.Add(computeDeletionRetry(rec.DeletionAttempts))
	nextAttempts := rec.DeletionAttempts + 1
	if err := s.sandboxStore.RecordDeletionAttempt(ctx, rec.ID, nextAttempts, rec.DeletionForceLevel, &now, &retryAt, "", now); err != nil {
		return err
	}
	rec.DeletionAttempts = nextAttempts
	rec.DeletionLastAttemptAt = &now
	rec.DeletionNextRetryAt = &retryAt

	snapshot, err := s.snapshot(ctx, rec)
	if err != nil {
		_ = s.sandboxStore.RecordDeletionAttempt(ctx, rec.ID, rec.DeletionAttempts, rec.DeletionForceLevel, &now, &retryAt, err.Error(), now)
		return err
	}

	phase := rec.DeletionPhase
	if phase == "" {
		phase = store.DeletionPhaseRequested
	}
	logger := logWithSandboxID(ctx, rec.ID).With(
		"deletion_phase", phase,
		"deletion_force_level", rec.DeletionForceLevel,
		"deletion_attempts", rec.DeletionAttempts,
		"has_deployment", snapshot.Deployment != nil,
		"pod_count", len(snapshot.Pods),
		"has_pvc", snapshot.PVC != nil,
		"has_pv", snapshot.PV != nil,
		"volume_attachment_count", len(snapshot.VolumeAttachments),
	)
	switch phase {
	case store.DeletionPhaseRequested:
		return s.transitionPhase(ctx, rec, store.DeletionPhaseQuiescingRuntime)
	case store.DeletionPhaseQuiescingRuntime:
		return s.handleQuiescingRuntime(ctx, rec, snapshot, now)
	case store.DeletionPhaseDeletingStorage:
		return s.handleDeletingStorage(ctx, rec, snapshot, now)
	case store.DeletionPhaseForceCleanup:
		return s.handleForceCleanup(ctx, rec, snapshot, now)
	case store.DeletionPhaseVerifying:
		return s.handleVerifying(ctx, rec, snapshot, now)
	case store.DeletionPhaseCompleted:
		return nil
	default:
		logger.Warn("unknown deletion phase, resetting to requested")
		return s.transitionPhase(ctx, rec, store.DeletionPhaseRequested)
	}
}

func (s *SandboxDeletionService) handleQuiescingRuntime(ctx context.Context, rec *store.SandboxRecord, snapshot *k8s.SandboxDeletionSnapshot, now time.Time) error {
	if rec.PersistenceEnabled {
		if snapshot.Deployment != nil {
			if err := s.k8sClient.DeletePersistentSandbox(ctx, rec.ID, "", "Retain"); err != nil {
				return err
			}
			return s.markReadyForImmediateRetry(ctx, rec)
		}
	} else if len(snapshot.Pods) > 0 {
		if err := s.k8sClient.DeletePod(ctx, rec.ID); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		return s.markReadyForImmediateRetry(ctx, rec)
	}

	if len(snapshot.Pods) > 0 {
		if exceededDeletionTimeout(rec.DeletionStartedAt, podDeleteTimeout, now) {
			return s.transitionPhase(ctx, rec, store.DeletionPhaseForceCleanup)
		}
		return nil
	}

	if rec.PersistenceEnabled && rec.VolumeReclaimPolicy != "Retain" && rec.VolumeClaimName != "" {
		if snapshot.PVC != nil {
			return s.transitionPhase(ctx, rec, store.DeletionPhaseDeletingStorage)
		}
	}
	return s.transitionPhase(ctx, rec, store.DeletionPhaseVerifying)
}

func (s *SandboxDeletionService) handleDeletingStorage(ctx context.Context, rec *store.SandboxRecord, snapshot *k8s.SandboxDeletionSnapshot, now time.Time) error {
	if snapshot.PVC == nil {
		return s.transitionPhase(ctx, rec, store.DeletionPhaseVerifying)
	}
	if len(snapshot.Pods) > 0 {
		return s.transitionPhase(ctx, rec, store.DeletionPhaseForceCleanup)
	}
	if snapshot.PVC.DeletionTimestamp == nil {
		if err := s.k8sClient.DeletePersistentSandbox(ctx, rec.ID, rec.VolumeClaimName, rec.VolumeReclaimPolicy); err != nil {
			return err
		}
		return s.markReadyForImmediateRetry(ctx, rec)
	}
	if exceededDeletionTimeout(rec.DeletionStartedAt, pvcDeleteTimeout, now) {
		return s.transitionPhase(ctx, rec, store.DeletionPhaseForceCleanup)
	}
	return nil
}

func (s *SandboxDeletionService) handleForceCleanup(ctx context.Context, rec *store.SandboxRecord, snapshot *k8s.SandboxDeletionSnapshot, now time.Time) error {
	forceLevel := rec.DeletionForceLevel
	switch forceLevel {
	case 0:
		if len(snapshot.Pods) > 0 {
			for i := range snapshot.Pods {
				if err := s.k8sClient.ForceDeletePodByName(ctx, snapshot.Pods[i].Name); err != nil {
					return err
				}
			}
		}
        if snapshot.Deployment != nil && snapshot.Deployment.DeletionTimestamp != nil {
            if err := s.k8sClient.PatchDeploymentFinalizers(ctx, snapshot.Deployment.Name, rec.ID, nil); err != nil {
                return err
            }
        }
		return s.bumpForceLevel(ctx, rec, 1, now)
	case 1:
        if snapshot.PVC != nil && s.canForceDeletePVC(rec, snapshot) && exceededDeletionTimeout(rec.DeletionStartedAt, pvcForceCleanupThreshold, now) {
            if err := s.k8sClient.PatchPVCFinalizers(ctx, snapshot.PVC.Name, rec.ID, nil); err != nil {
                return err
            }
            if err := s.k8sClient.DeletePersistentSandbox(ctx, rec.ID, rec.VolumeClaimName, rec.VolumeReclaimPolicy); err != nil {
                return err
            }
        }
		return s.bumpForceLevel(ctx, rec, 2, now)
	case 2:
        if snapshot.PV != nil && exceededDeletionTimeout(rec.DeletionStartedAt, storageAttachmentCleanupAfter, now) {
            if err := s.cleanupStorageAttachments(ctx, rec, snapshot); err != nil {
                return err
            }
        }
        return s.bumpForceLevel(ctx, rec, 3, now)
	default:
		return s.transitionPhase(ctx, rec, store.DeletionPhaseVerifying)
	}
}

func (s *SandboxDeletionService) handleVerifying(ctx context.Context, rec *store.SandboxRecord, snapshot *k8s.SandboxDeletionSnapshot, now time.Time) error {
	if rec.PersistenceEnabled {
		if snapshot.Deployment != nil || len(snapshot.Pods) > 0 {
			return s.transitionPhase(ctx, rec, store.DeletionPhaseForceCleanup)
		}
		if rec.VolumeReclaimPolicy != "Retain" && snapshot.PVC != nil {
			return s.transitionPhase(ctx, rec, store.DeletionPhaseForceCleanup)
		}
		if rec.DeletionForceLevel >= 3 && (snapshot.PV != nil || len(snapshot.VolumeAttachments) > 0) {
			return s.transitionPhase(ctx, rec, store.DeletionPhaseForceCleanup)
		}
	} else if len(snapshot.Pods) > 0 {
		return s.transitionPhase(ctx, rec, store.DeletionPhaseForceCleanup)
	}

	if err := s.sandboxStore.MarkDeletionCompleted(ctx, rec.ID, "delete completed", now); err != nil {
		return err
	}
	_ = s.sandboxStore.AppendStatusHistory(ctx, rec.ID, "system", "terminating", "deleted", "delete completed", nil, now)
	logWithSandboxID(ctx, rec.ID).Info("sandbox deletion completed", "deletion_force_level", rec.DeletionForceLevel, "deletion_attempts", rec.DeletionAttempts)
	return nil
}

func (s *SandboxDeletionService) cleanupStorageAttachments(ctx context.Context, rec *store.SandboxRecord, snapshot *k8s.SandboxDeletionSnapshot) error {
    for i := range snapshot.VolumeAttachments {
        if err := s.k8sClient.DeleteVolumeAttachment(ctx, snapshot.VolumeAttachments[i].Name); err != nil {
            return err
        }
    }
    if snapshot.PV != nil {
        if err := s.k8sClient.PatchPVFinalizers(ctx, snapshot.PV.Name, rec.VolumeClaimName, nil); err != nil {
            return err
        }
        if err := s.k8sClient.DeletePersistentVolume(ctx, snapshot.PV.Name, rec.VolumeClaimName); err != nil {
            return err
        }
    }
    return nil
}

func (s *SandboxDeletionService) canForceDeletePVC(rec *store.SandboxRecord, snapshot *k8s.SandboxDeletionSnapshot) bool {
	if rec.VolumeReclaimPolicy == "Retain" || snapshot.PVC == nil || len(snapshot.Pods) > 0 || snapshot.Deployment != nil {
		return false
	}
	if snapshot.PVC.Name != rec.VolumeClaimName {
		return false
	}
	for i := range snapshot.Pods {
		if podUsesPVC(&snapshot.Pods[i], snapshot.PVC.Name) {
			return false
		}
	}
	return true
}

func (s *SandboxDeletionService) snapshot(ctx context.Context, rec *store.SandboxRecord) (*k8s.SandboxDeletionSnapshot, error) {
	if rec.PersistenceEnabled {
		return s.k8sClient.GetSandboxDeletionSnapshot(ctx, rec.ID, rec.RuntimeName, rec.VolumeClaimName)
	}
	pods, err := s.k8sClient.ListSandboxPods(ctx, rec.ID)
	if err != nil {
		return nil, err
	}
	return &k8s.SandboxDeletionSnapshot{Pods: pods}, nil
}

func (s *SandboxDeletionService) transitionPhase(ctx context.Context, rec *store.SandboxRecord, phase string) error {
	now := time.Now().UTC()
	prev := rec.DeletionPhase
	if prev == "" {
		prev = store.DeletionPhaseRequested
	}
	if err := s.sandboxStore.AdvanceDeletionPhase(ctx, rec.ID, phase, now); err != nil {
		return err
	}
	rec.DeletionPhase = phase
	logWithSandboxID(ctx, rec.ID).Info("sandbox deletion phase advanced", "from_phase", prev, "to_phase", phase, "deletion_force_level", rec.DeletionForceLevel, "deletion_attempts", rec.DeletionAttempts)
	return s.markReadyForImmediateRetry(ctx, rec)
}

func (s *SandboxDeletionService) bumpForceLevel(ctx context.Context, rec *store.SandboxRecord, level int, now time.Time) error {
	if err := s.sandboxStore.RecordDeletionAttempt(ctx, rec.ID, rec.DeletionAttempts, level, &now, &now, "", now); err != nil {
		return err
	}
	rec.DeletionForceLevel = level
	rec.DeletionNextRetryAt = &now
	return nil
}

func (s *SandboxDeletionService) markReadyForImmediateRetry(ctx context.Context, rec *store.SandboxRecord) error {
	now := time.Now().UTC()
	if err := s.sandboxStore.RecordDeletionAttempt(ctx, rec.ID, rec.DeletionAttempts, rec.DeletionForceLevel, rec.DeletionLastAttemptAt, &now, "", now); err != nil {
		return err
	}
	rec.DeletionNextRetryAt = &now
	rec.DeletionLastError = ""
	return nil
}

func computeDeletionRetry(attempts int) time.Duration {
	delay := defaultDeletionRetryInterval * time.Duration(1<<max(0, attempts))
	if delay > maxDeletionRetryInterval {
		return maxDeletionRetryInterval
	}
	return delay
}

func exceededDeletionTimeout(start *time.Time, timeout time.Duration, now time.Time) bool {
	if start == nil {
		return false
	}
	return now.Sub(*start) >= timeout
}

func podUsesPVC(pod *corev1.Pod, claimName string) bool {
	for _, vol := range pod.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.ClaimName == claimName {
			return true
		}
	}
	return false
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *SandboxDeletionService) DebugString(rec *store.SandboxRecord) string {
	return fmt.Sprintf("%s/%s force=%d attempts=%d", rec.ID, rec.DeletionPhase, rec.DeletionForceLevel, rec.DeletionAttempts)
}

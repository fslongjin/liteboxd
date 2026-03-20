package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/k8s"
	"github.com/fslongjin/liteboxd/backend/internal/model"
	"github.com/fslongjin/liteboxd/backend/internal/store"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	reconcileStatusRunning   = "running"
	reconcileStatusCompleted = "completed"
	reconcileStatusFailed    = "failed"
)

// SandboxReconcileService periodically compares DB metadata and K8s runtime state.
type SandboxReconcileService struct {
	k8sClient       *k8s.Client
	sandboxStore    *store.SandboxStore
	lostGracePeriod time.Duration
}

func NewSandboxReconcileService(k8sClient *k8s.Client, sandboxStore *store.SandboxStore) *SandboxReconcileService {
	return &SandboxReconcileService{
		k8sClient:       k8sClient,
		sandboxStore:    sandboxStore,
		lostGracePeriod: 10 * time.Minute,
	}
}

func (s *SandboxReconcileService) Start(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			if _, err := s.Run(context.Background(), "scheduled"); err != nil {
				slog.Default().With("component", "sandbox_reconciler").Error("scheduled reconcile failed", "error", err)
			}
		}
	}()
}

func (s *SandboxReconcileService) Run(ctx context.Context, trigger string) (*model.ReconcileRunDetailResponse, error) {
	runID := "rec-" + uuid.New().String()[:8]
	now := time.Now().UTC()

	run := &store.ReconcileRunRecord{
		ID:          runID,
		TriggerType: trigger,
		StartedAt:   now,
		Status:      reconcileStatusRunning,
	}
	if err := s.sandboxStore.CreateReconcileRun(ctx, run); err != nil {
		return nil, err
	}

	dbRecords, err := s.sandboxStore.ListForReconcile(ctx)
	if err != nil {
		_ = s.sandboxStore.FinishReconcileRun(ctx, runID, reconcileStatusFailed, err.Error(), 0, 0, 0, 0, time.Now().UTC())
		return nil, err
	}

	podList, err := s.k8sClient.ListPods(ctx)
	if err != nil {
		_ = s.sandboxStore.FinishReconcileRun(ctx, runID, reconcileStatusFailed, err.Error(), len(dbRecords), 0, 0, 0, time.Now().UTC())
		return nil, err
	}

	podMap := make(map[string]int, len(podList.Items))
	for i, pod := range podList.Items {
		sandboxID := pod.Labels[k8s.LabelSandboxID]
		if sandboxID == "" {
			continue
		}
		podMap[sandboxID] = i
	}

	driftCount := 0
	fixedCount := 0

	for _, rec := range dbRecords {
		if rec.PersistenceEnabled {
			handled, err := s.reconcilePersistentSandbox(ctx, runID, &rec, podMap)
			if err != nil {
				_ = s.sandboxStore.FinishReconcileRun(ctx, runID, reconcileStatusFailed, err.Error(), len(dbRecords), len(podList.Items), driftCount, fixedCount, time.Now().UTC())
				return nil, err
			}
			if handled.drifted {
				driftCount++
			}
			if handled.fixed {
				fixedCount++
			}
			continue
		}

		if rec.DesiredState == store.DesiredStateDeleted {
			handled, err := s.reconcileDeletedSandbox(ctx, runID, &rec, podMap)
			if err != nil {
				_ = s.sandboxStore.FinishReconcileRun(ctx, runID, reconcileStatusFailed, err.Error(), len(dbRecords), len(podList.Items), driftCount, fixedCount, time.Now().UTC())
				return nil, err
			}
			if handled.drifted {
				driftCount++
			}
			if handled.fixed {
				fixedCount++
			}
			continue
		}

		idx, ok := podMap[rec.ID]
		if !ok {
			driftCount++
			action := "none"
			detail := "sandbox exists in DB but pod not found in cluster"

			switch rec.LifecycleStatus {
			case "terminating", "deleted":
				if rec.LifecycleStatus != "deleted" {
					if err := s.sandboxStore.MarkDeleted(ctx, rec.ID, "reconcile: pod not found", time.Now().UTC()); err == nil {
						fixedCount++
						action = "mark_deleted"
					}
				}
			case "stopped":
				// Pod intentionally absent — not drift
				continue
			default:
				missingSince := rec.CreatedAt
				if rec.LastSeenAt != nil {
					missingSince = *rec.LastSeenAt
				}
				if time.Since(missingSince) >= s.lostGracePeriod {
					if updated, err := s.sandboxStore.UpdateStatusIfActive(ctx, rec.ID, "lost", "reconcile: pod missing beyond grace period", time.Now().UTC()); err == nil && updated {
						fixedCount++
						action = "mark_lost"
					}
				}
			}

			_ = s.sandboxStore.AddReconcileItem(ctx, &store.ReconcileItemRecord{
				RunID:     runID,
				SandboxID: rec.ID,
				DriftType: "missing_in_k8s",
				Action:    action,
				Detail:    detail,
				CreatedAt: time.Now().UTC(),
			})
			continue
		}

		pod := podList.Items[idx]
		delete(podMap, rec.ID)

		podStatus := convertPodStatus(&pod)
		newLifecycle := string(podStatus)
		mismatch := rec.PodUID != string(pod.UID) || rec.PodPhase != string(pod.Status.Phase) || rec.PodIP != pod.Status.PodIP || rec.LifecycleStatus != newLifecycle
		if mismatch {
			driftCount++
			if updated, err := s.sandboxStore.UpdateObservedStateIfActive(
				ctx,
				rec.ID,
				string(pod.UID),
				string(pod.Status.Phase),
				pod.Status.PodIP,
				newLifecycle,
				"",
				time.Now().UTC(),
				time.Now().UTC(),
			); err == nil && updated {
				fixedCount++
			}
			_ = s.sandboxStore.AddReconcileItem(ctx, &store.ReconcileItemRecord{
				RunID:     runID,
				SandboxID: rec.ID,
				DriftType: "status_mismatch",
				Action:    "none",
				Detail:    fmt.Sprintf("db_status=%s, pod_phase=%s", rec.LifecycleStatus, pod.Status.Phase),
				CreatedAt: time.Now().UTC(),
			})
		}
	}

	for sandboxID := range podMap {
		driftCount++
		_ = s.sandboxStore.AddReconcileItem(ctx, &store.ReconcileItemRecord{
			RunID:     runID,
			SandboxID: sandboxID,
			DriftType: "missing_in_db",
			Action:    "alert_only",
			Detail:    "sandbox pod exists in cluster but missing in DB",
			CreatedAt: time.Now().UTC(),
		})
	}

	finishedAt := time.Now().UTC()
	if err := s.sandboxStore.FinishReconcileRun(ctx, runID, reconcileStatusCompleted, "", len(dbRecords), len(podList.Items), driftCount, fixedCount, finishedAt); err != nil {
		return nil, err
	}

	return s.GetRun(ctx, runID)
}

type reconcileResult struct {
	drifted bool
	fixed   bool
}

func (s *SandboxReconcileService) reconcilePersistentSandbox(ctx context.Context, runID string, rec *store.SandboxRecord, podMap map[string]int) (reconcileResult, error) {
	result := reconcileResult{}
	snapshot, err := s.k8sClient.GetPersistentSandboxSnapshot(ctx, rec.ID, rec.RuntimeName, rec.VolumeClaimName)
	if err != nil {
		return result, err
	}
	if snapshot.Pod != nil {
		delete(podMap, rec.ID)
	}

	if rec.DesiredState == store.DesiredStateDeleted {
		return s.reconcileDeletedPersistentSandbox(ctx, runID, rec, snapshot)
	}

	if snapshot.Pod == nil && snapshot.PVC == nil && snapshot.Deployment == nil {
		result.drifted = true
		action := "none"
		detail := "persistent sandbox exists in DB but deployment, pvc and pod not found in cluster"

		switch rec.LifecycleStatus {
		case "terminating", "deleted":
			if rec.LifecycleStatus != "deleted" {
				if err := s.sandboxStore.MarkDeleted(ctx, rec.ID, "reconcile: persistent runtime not found", time.Now().UTC()); err == nil {
					_ = s.sandboxStore.AppendStatusHistory(ctx, rec.ID, "reconcile", rec.LifecycleStatus, "deleted", "reconcile: persistent runtime not found", nil, time.Now().UTC())
					result.fixed = true
					action = "mark_deleted"
				}
			}
		case "stopped":
			return reconcileResult{}, nil
		default:
			missingSince := rec.CreatedAt
			if rec.LastSeenAt != nil {
				missingSince = *rec.LastSeenAt
			}
			if time.Since(missingSince) >= s.lostGracePeriod {
				if updated, err := s.sandboxStore.UpdateStatusIfActive(ctx, rec.ID, "lost", "reconcile: persistent runtime missing beyond grace period", time.Now().UTC()); err == nil && updated {
					_ = s.sandboxStore.AppendStatusHistory(ctx, rec.ID, "reconcile", rec.LifecycleStatus, "lost", "reconcile: persistent runtime missing beyond grace period", nil, time.Now().UTC())
					result.fixed = true
					action = "mark_lost"
				}
			}
		}

		_ = s.sandboxStore.AddReconcileItem(ctx, &store.ReconcileItemRecord{
			RunID:     runID,
			SandboxID: rec.ID,
			DriftType: "missing_in_k8s",
			Action:    action,
			Detail:    detail,
			CreatedAt: time.Now().UTC(),
		})
		return result, nil
	}

	if shouldTreatPersistentSandboxAsStopped(rec, snapshot) {
		return s.reconcileStoppedPersistentSandbox(ctx, runID, rec, snapshot)
	}

	state, reason := classifyPersistentStartup(snapshot)
	switch state {
	case persistentStartupReady:
		if rec.LifecycleStatus != string(model.SandboxStatusRunning) || rec.StatusReason != "" || rec.PodUID != string(snapshot.Pod.UID) || rec.PodPhase != string(snapshot.Pod.Status.Phase) || rec.PodIP != snapshot.Pod.Status.PodIP {
			result.drifted = true
			if updated, err := s.sandboxStore.UpdateObservedStateIfActive(
				ctx,
				rec.ID,
				string(snapshot.Pod.UID),
				string(snapshot.Pod.Status.Phase),
				snapshot.Pod.Status.PodIP,
				string(model.SandboxStatusRunning),
				"",
				time.Now().UTC(),
				time.Now().UTC(),
			); err == nil && updated {
				result.fixed = true
				_ = s.sandboxStore.AppendStatusHistory(ctx, rec.ID, "reconcile", rec.LifecycleStatus, string(model.SandboxStatusRunning), "reconcile: persistent sandbox is ready", nil, time.Now().UTC())
			}
			_ = s.sandboxStore.AddReconcileItem(ctx, &store.ReconcileItemRecord{
				RunID:     runID,
				SandboxID: rec.ID,
				DriftType: "status_mismatch",
				Action:    "none",
				Detail:    fmt.Sprintf("db_status=%s, persistent_state=running", rec.LifecycleStatus),
				CreatedAt: time.Now().UTC(),
			})
		}
	case persistentStartupFailed:
		if rec.LifecycleStatus != string(model.SandboxStatusFailed) || rec.StatusReason != reason || (snapshot.Pod != nil && (rec.PodUID != string(snapshot.Pod.UID) || rec.PodPhase != string(snapshot.Pod.Status.Phase) || rec.PodIP != snapshot.Pod.Status.PodIP)) {
			result.drifted = true
			now := time.Now().UTC()
			if snapshot.Pod != nil {
				if updated, err := s.sandboxStore.UpdateObservedStateIfActive(
					ctx,
					rec.ID,
					string(snapshot.Pod.UID),
					string(snapshot.Pod.Status.Phase),
					snapshot.Pod.Status.PodIP,
					string(model.SandboxStatusFailed),
					reason,
					now,
					now,
				); err == nil && updated {
					result.fixed = true
				}
			} else if updated, err := s.sandboxStore.UpdateStatusIfActive(ctx, rec.ID, string(model.SandboxStatusFailed), reason, now); err == nil && updated {
				result.fixed = true
			}
			if result.fixed {
				_ = s.sandboxStore.AppendStatusHistory(ctx, rec.ID, "reconcile", rec.LifecycleStatus, string(model.SandboxStatusFailed), reason, nil, now)
			}
			_ = s.sandboxStore.AddReconcileItem(ctx, &store.ReconcileItemRecord{
				RunID:     runID,
				SandboxID: rec.ID,
				DriftType: "status_mismatch",
				Action:    "none",
				Detail:    fmt.Sprintf("db_status=%s, persistent_state=failed", rec.LifecycleStatus),
				CreatedAt: now,
			})
		}
	default:
		if rec.LifecycleStatus == string(model.SandboxStatusFailed) {
			return result, nil
		}
		var podUID, podPhase, podIP string
		if snapshot.Pod != nil {
			podUID = string(snapshot.Pod.UID)
			podPhase = string(snapshot.Pod.Status.Phase)
			podIP = snapshot.Pod.Status.PodIP
		}
		if rec.LifecycleStatus != string(model.SandboxStatusPending) || rec.StatusReason != "" || rec.PodUID != podUID || rec.PodPhase != podPhase || rec.PodIP != podIP {
			result.drifted = true
			now := time.Now().UTC()
			if snapshot.Pod != nil {
				if updated, err := s.sandboxStore.UpdateObservedStateIfActive(ctx, rec.ID, podUID, podPhase, podIP, string(model.SandboxStatusPending), "", now, now); err == nil && updated {
					result.fixed = true
				}
			} else if updated, err := s.sandboxStore.UpdateStatusIfActive(ctx, rec.ID, string(model.SandboxStatusPending), "", now); err == nil && updated {
				result.fixed = true
			}
			if result.fixed && rec.LifecycleStatus != string(model.SandboxStatusPending) {
				_ = s.sandboxStore.AppendStatusHistory(ctx, rec.ID, "reconcile", rec.LifecycleStatus, string(model.SandboxStatusPending), "reconcile: persistent sandbox is pending", nil, now)
			}
			_ = s.sandboxStore.AddReconcileItem(ctx, &store.ReconcileItemRecord{
				RunID:     runID,
				SandboxID: rec.ID,
				DriftType: "status_mismatch",
				Action:    "none",
				Detail:    fmt.Sprintf("db_status=%s, persistent_state=pending", rec.LifecycleStatus),
				CreatedAt: now,
			})
		}
	}

	return result, nil
}

func shouldTreatPersistentSandboxAsStopped(rec *store.SandboxRecord, snapshot *k8s.PersistentSandboxSnapshot) bool {
	if rec == nil || snapshot == nil || snapshot.Deployment == nil {
		return false
	}
	if strings.TrimSpace(rec.LifecycleStatus) != string(model.SandboxStatusStopped) && rec.StoppedAt == nil {
		return false
	}
	if snapshot.Deployment.Spec.Replicas == nil {
		return false
	}
	return *snapshot.Deployment.Spec.Replicas == 0
}

func (s *SandboxReconcileService) reconcileStoppedPersistentSandbox(ctx context.Context, runID string, rec *store.SandboxRecord, snapshot *k8s.PersistentSandboxSnapshot) (reconcileResult, error) {
	result := reconcileResult{}
	now := time.Now().UTC()
	statusReason := strings.TrimSpace(rec.StatusReason)
	if statusReason == "" {
		statusReason = "stopped by request"
	}

	if snapshot.Pod != nil && isTerminalOrDeletingPod(snapshot.Pod) {
		result.drifted = true
		action := "cleanup_residual_pod"
		if err := s.k8sClient.DeletePodByName(ctx, snapshot.Pod.Name); err != nil && !apierrors.IsNotFound(err) {
			action = "cleanup_failed"
			_ = s.sandboxStore.AddReconcileItem(ctx, &store.ReconcileItemRecord{
				RunID:     runID,
				SandboxID: rec.ID,
				DriftType: "status_mismatch",
				Action:    action,
				Detail:    fmt.Sprintf("stopped sandbox keeps residual pod %s in phase %s: %v", snapshot.Pod.Name, snapshot.Pod.Status.Phase, err),
				CreatedAt: now,
			})
			return result, err
		}
		result.fixed = true
		_ = s.sandboxStore.AddReconcileItem(ctx, &store.ReconcileItemRecord{
			RunID:     runID,
			SandboxID: rec.ID,
			DriftType: "status_mismatch",
			Action:    action,
			Detail:    fmt.Sprintf("stopped sandbox cleaned residual pod %s in phase %s", snapshot.Pod.Name, snapshot.Pod.Status.Phase),
			CreatedAt: now,
		})
	}

	podUID, podPhase, podIP := "", "", ""
	if rec.LifecycleStatus != string(model.SandboxStatusStopped) || rec.StatusReason != statusReason || rec.PodUID != podUID || rec.PodPhase != podPhase || rec.PodIP != podIP {
		result.drifted = true
		if updated, err := s.sandboxStore.UpdateObservedStateIfActive(
			ctx,
			rec.ID,
			podUID,
			podPhase,
			podIP,
			string(model.SandboxStatusStopped),
			statusReason,
			now,
			now,
		); err == nil && updated {
			if rec.StoppedAt == nil {
				_ = s.sandboxStore.EnsureStoppedAt(ctx, rec.ID, now)
			}
			result.fixed = true
			if rec.LifecycleStatus != string(model.SandboxStatusStopped) {
				_ = s.sandboxStore.AppendStatusHistory(ctx, rec.ID, "reconcile", rec.LifecycleStatus, string(model.SandboxStatusStopped), "reconcile: deployment scaled to zero", nil, now)
			}
		} else if err != nil {
			return result, err
		}
		_ = s.sandboxStore.AddReconcileItem(ctx, &store.ReconcileItemRecord{
			RunID:     runID,
			SandboxID: rec.ID,
			DriftType: "status_mismatch",
			Action:    "preserve_stopped",
			Detail:    "persistent sandbox remains stopped because deployment replicas is 0",
			CreatedAt: now,
		})
	}

	return result, nil
}

func isTerminalOrDeletingPod(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	if pod.DeletionTimestamp != nil {
		return true
	}
	switch pod.Status.Phase {
	case corev1.PodSucceeded, corev1.PodFailed:
		return true
	default:
		return false
	}
}

func (s *SandboxReconcileService) reconcileDeletedSandbox(ctx context.Context, runID string, rec *store.SandboxRecord, podMap map[string]int) (reconcileResult, error) {
	result := reconcileResult{}
	if _, ok := podMap[rec.ID]; ok {
		delete(podMap, rec.ID)
		result.drifted = true
		_ = s.sandboxStore.AddReconcileItem(ctx, &store.ReconcileItemRecord{
			RunID:     runID,
			SandboxID: rec.ID,
			DriftType: "status_mismatch",
			Action:    "deletion_pending",
			Detail:    "delete convergence in progress: residual pod found",
			CreatedAt: time.Now().UTC(),
		})
		return result, nil
	}
	if rec.LifecycleStatus != "deleted" {
		result.drifted = true
		_ = s.sandboxStore.AddReconcileItem(ctx, &store.ReconcileItemRecord{
			RunID:     runID,
			SandboxID: rec.ID,
			DriftType: "status_mismatch",
			Action:    "deletion_pending",
			Detail:    "delete requested with no residual pod; deletion service should complete",
			CreatedAt: time.Now().UTC(),
		})
	}
	return result, nil
}

func (s *SandboxReconcileService) reconcileDeletedPersistentSandbox(ctx context.Context, runID string, rec *store.SandboxRecord, snapshot *k8s.PersistentSandboxSnapshot) (reconcileResult, error) {
	result := reconcileResult{}
	now := time.Now().UTC()

	hasResidualDeployment := snapshot.Deployment != nil
	hasResidualPod := snapshot.Pod != nil
	hasResidualPVC := snapshot.PVC != nil && rec.VolumeReclaimPolicy != "Retain"
	hasResidual := hasResidualDeployment || hasResidualPod || hasResidualPVC

	if hasResidual {
		result.drifted = true
		detail := "delete convergence in progress"
		switch {
		case hasResidualPVC:
			detail = "delete convergence in progress: residual pvc found"
		case hasResidualPod:
			detail = "delete convergence in progress: residual pod found"
		case hasResidualDeployment:
			detail = "delete convergence in progress: residual deployment found"
		}
		_ = s.sandboxStore.AddReconcileItem(ctx, &store.ReconcileItemRecord{
			RunID:     runID,
			SandboxID: rec.ID,
			DriftType: "status_mismatch",
			Action:    "deletion_pending",
			Detail:    detail,
			CreatedAt: now,
		})
		return result, nil
	}

	if rec.LifecycleStatus != "deleted" {
		result.drifted = true
		_ = s.sandboxStore.AddReconcileItem(ctx, &store.ReconcileItemRecord{
			RunID:     runID,
			SandboxID: rec.ID,
			DriftType: "status_mismatch",
			Action:    "deletion_pending",
			Detail:    "delete requested with no residual runtime; deletion service should complete",
			CreatedAt: now,
		})
	}
	return result, nil
}

func (s *SandboxReconcileService) ListRuns(ctx context.Context, limit int) (*model.ReconcileRunListResponse, error) {
	runs, err := s.sandboxStore.ListReconcileRuns(ctx, limit)
	if err != nil {
		return nil, err
	}
	items := make([]model.ReconcileRun, 0, len(runs))
	for _, run := range runs {
		items = append(items, model.ReconcileRun{
			ID:          run.ID,
			TriggerType: run.TriggerType,
			StartedAt:   run.StartedAt,
			FinishedAt:  run.FinishedAt,
			TotalDB:     run.TotalDB,
			TotalK8s:    run.TotalK8s,
			DriftCount:  run.DriftCount,
			FixedCount:  run.FixedCount,
			Status:      run.Status,
			Error:       run.Error,
		})
	}
	return &model.ReconcileRunListResponse{Items: items}, nil
}

func (s *SandboxReconcileService) GetRun(ctx context.Context, runID string) (*model.ReconcileRunDetailResponse, error) {
	run, err := s.sandboxStore.GetReconcileRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, nil
	}
	items, err := s.sandboxStore.ListReconcileItems(ctx, runID)
	if err != nil {
		return nil, err
	}
	respItems := make([]model.ReconcileItem, 0, len(items))
	for _, item := range items {
		respItems = append(respItems, model.ReconcileItem{
			ID:        item.ID,
			RunID:     item.RunID,
			SandboxID: item.SandboxID,
			DriftType: item.DriftType,
			Action:    item.Action,
			Detail:    item.Detail,
			CreatedAt: item.CreatedAt,
		})
	}

	return &model.ReconcileRunDetailResponse{
		Run: model.ReconcileRun{
			ID:          run.ID,
			TriggerType: run.TriggerType,
			StartedAt:   run.StartedAt,
			FinishedAt:  run.FinishedAt,
			TotalDB:     run.TotalDB,
			TotalK8s:    run.TotalK8s,
			DriftCount:  run.DriftCount,
			FixedCount:  run.FixedCount,
			Status:      run.Status,
			Error:       run.Error,
		},
		Items: respItems,
	}, nil
}

package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/k8s"
	"github.com/fslongjin/liteboxd/backend/internal/model"
	"github.com/fslongjin/liteboxd/backend/internal/store"
	"github.com/google/uuid"
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
			default:
				if rec.LastSeenAt != nil && time.Since(*rec.LastSeenAt) >= s.lostGracePeriod {
					if err := s.sandboxStore.UpdateStatus(ctx, rec.ID, "lost", "reconcile: pod missing beyond grace period", time.Now().UTC()); err == nil {
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
			if err := s.sandboxStore.UpdateObservedState(
				ctx,
				rec.ID,
				string(pod.UID),
				string(pod.Status.Phase),
				pod.Status.PodIP,
				newLifecycle,
				"",
				time.Now().UTC(),
				time.Now().UTC(),
			); err == nil {
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

package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func initTestDB(t *testing.T) {
	t.Helper()
	if err := InitDB(filepath.Join(t.TempDir(), "liteboxd.db")); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	t.Cleanup(func() {
		if err := CloseDB(); err != nil {
			t.Fatalf("CloseDB() error = %v", err)
		}
	})
}

func TestSandboxStoreCreateGetAndDeleteFlow(t *testing.T) {
	initTestDB(t)
	ctx := context.Background()
	s := NewSandboxStore()
	now := time.Now().UTC()
	lastSeen := now

	rec := &SandboxRecord{
		ID:                    "sbx12345",
		TemplateName:          "python",
		TemplateVersion:       1,
		Image:                 "python:3.11",
		CPU:                   "500m",
		Memory:                "512Mi",
		TTL:                   3600,
		EnvJSON:               `{"A":"B"}`,
		DesiredState:          DesiredStateActive,
		LifecycleStatus:       "running",
		StatusReason:          "",
		ClusterNamespace:      "liteboxd-sandbox",
		PodName:               "sandbox-sbx12345",
		PodUID:                "uid-1",
		PodPhase:              "Running",
		PodIP:                 "10.0.0.10",
		LastSeenAt:            &lastSeen,
		AccessTokenCiphertext: "cipher",
		AccessTokenNonce:      "nonce",
		AccessTokenKeyID:      "v1",
		AccessTokenSHA256:     "hash",
		AccessURL:             "http://gateway/sbx12345",
		CreatedAt:             now,
		ExpiresAt:             now.Add(time.Hour),
		UpdatedAt:             now,
	}
	if err := s.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := s.GetByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got == nil {
		t.Fatalf("GetByID() returned nil")
	}
	if got.ID != rec.ID || got.LifecycleStatus != "running" || got.PodPhase != "Running" {
		t.Fatalf("unexpected record: %+v", got)
	}
	if v := got.EnvMap()["A"]; v != "B" {
		t.Fatalf("EnvMap() unexpected value: %q", v)
	}

	active, err := s.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("ListActive() len = %d, want 1", len(active))
	}

	if err := s.SetDesiredDeleted(ctx, rec.ID, now.Add(time.Minute)); err != nil {
		t.Fatalf("SetDesiredDeleted() error = %v", err)
	}
	if err := s.MarkDeleted(ctx, rec.ID, "deleted", now.Add(2*time.Minute)); err != nil {
		t.Fatalf("MarkDeleted() error = %v", err)
	}

	active, err = s.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive() after delete error = %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("ListActive() after delete len = %d, want 0", len(active))
	}

	deleted, err := s.GetByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetByID() after delete error = %v", err)
	}
	if deleted.LifecycleStatus != "deleted" || deleted.DesiredState != DesiredStateDeleted || deleted.DeletedAt == nil {
		t.Fatalf("record was not marked deleted correctly: %+v", deleted)
	}
}

func TestSandboxStoreReconcileRunFlow(t *testing.T) {
	initTestDB(t)
	ctx := context.Background()
	s := NewSandboxStore()
	now := time.Now().UTC()

	run := &ReconcileRunRecord{
		ID:          "rec-1234",
		TriggerType: "manual",
		StartedAt:   now,
		Status:      "running",
	}
	if err := s.CreateReconcileRun(ctx, run); err != nil {
		t.Fatalf("CreateReconcileRun() error = %v", err)
	}

	if err := s.AddReconcileItem(ctx, &ReconcileItemRecord{
		RunID:     run.ID,
		SandboxID: "sbx12345",
		DriftType: "missing_in_db",
		Action:    "alert_only",
		Detail:    "pod exists in k8s only",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("AddReconcileItem() error = %v", err)
	}

	if err := s.FinishReconcileRun(ctx, run.ID, "completed", "", 1, 2, 1, 0, now.Add(time.Minute)); err != nil {
		t.Fatalf("FinishReconcileRun() error = %v", err)
	}

	fetchedRun, err := s.GetReconcileRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetReconcileRun() error = %v", err)
	}
	if fetchedRun == nil {
		t.Fatalf("GetReconcileRun() returned nil")
	}
	if fetchedRun.Status != "completed" || fetchedRun.DriftCount != 1 || fetchedRun.TotalK8s != 2 {
		t.Fatalf("unexpected reconcile run: %+v", fetchedRun)
	}

	items, err := s.ListReconcileItems(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListReconcileItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListReconcileItems() len = %d, want 1", len(items))
	}
	if items[0].Action != "alert_only" {
		t.Fatalf("unexpected reconcile action: %+v", items[0])
	}
}

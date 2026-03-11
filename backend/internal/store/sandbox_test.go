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

func TestSandboxStorePurgeHistoricalData(t *testing.T) {
	initTestDB(t)
	ctx := context.Background()
	s := NewSandboxStore()
	now := time.Now().UTC()
	old := now.Add(-10 * 24 * time.Hour)

	// Old deleted sandbox should be purged.
	if err := s.Create(ctx, &SandboxRecord{
		ID:                    "old-deleted",
		TemplateName:          "python",
		TemplateVersion:       1,
		Image:                 "python:3.11",
		CPU:                   "500m",
		Memory:                "512Mi",
		TTL:                   3600,
		EnvJSON:               `{}`,
		DesiredState:          DesiredStateDeleted,
		LifecycleStatus:       "deleted",
		ClusterNamespace:      "liteboxd-sandbox",
		PodName:               "sandbox-old-deleted",
		AccessTokenCiphertext: "cipher",
		AccessTokenNonce:      "nonce",
		AccessTokenKeyID:      "v1",
		AccessTokenSHA256:     "hash",
		AccessURL:             "http://gateway/old-deleted",
		CreatedAt:             old,
		ExpiresAt:             old.Add(time.Hour),
		UpdatedAt:             old,
		DeletedAt:             &old,
	}); err != nil {
		t.Fatalf("Create old deleted sandbox error = %v", err)
	}

	// New deleted sandbox should be kept.
	if err := s.Create(ctx, &SandboxRecord{
		ID:                    "new-deleted",
		TemplateName:          "python",
		TemplateVersion:       1,
		Image:                 "python:3.11",
		CPU:                   "500m",
		Memory:                "512Mi",
		TTL:                   3600,
		EnvJSON:               `{}`,
		DesiredState:          DesiredStateDeleted,
		LifecycleStatus:       "deleted",
		ClusterNamespace:      "liteboxd-sandbox",
		PodName:               "sandbox-new-deleted",
		AccessTokenCiphertext: "cipher",
		AccessTokenNonce:      "nonce",
		AccessTokenKeyID:      "v1",
		AccessTokenSHA256:     "hash",
		AccessURL:             "http://gateway/new-deleted",
		CreatedAt:             now,
		ExpiresAt:             now.Add(time.Hour),
		UpdatedAt:             now,
		DeletedAt:             &now,
	}); err != nil {
		t.Fatalf("Create new deleted sandbox error = %v", err)
	}

	// Old status history should be purged.
	if err := s.AppendStatusHistory(ctx, "new-deleted", "test", "running", "deleted", "old history", nil, old); err != nil {
		t.Fatalf("AppendStatusHistory old error = %v", err)
	}
	// New status history should be kept.
	if err := s.AppendStatusHistory(ctx, "new-deleted", "test", "running", "deleted", "new history", nil, now); err != nil {
		t.Fatalf("AppendStatusHistory new error = %v", err)
	}

	runOld := &ReconcileRunRecord{ID: "rec-old", TriggerType: "manual", StartedAt: old, Status: "completed"}
	if err := s.CreateReconcileRun(ctx, runOld); err != nil {
		t.Fatalf("CreateReconcileRun old error = %v", err)
	}
	if err := s.AddReconcileItem(ctx, &ReconcileItemRecord{
		RunID: runOld.ID, SandboxID: "new-deleted", DriftType: "missing_in_db", Action: "alert_only", Detail: "old", CreatedAt: old,
	}); err != nil {
		t.Fatalf("AddReconcileItem old error = %v", err)
	}

	runNew := &ReconcileRunRecord{ID: "rec-new", TriggerType: "manual", StartedAt: now, Status: "completed"}
	if err := s.CreateReconcileRun(ctx, runNew); err != nil {
		t.Fatalf("CreateReconcileRun new error = %v", err)
	}
	if err := s.AddReconcileItem(ctx, &ReconcileItemRecord{
		RunID: runNew.ID, SandboxID: "new-deleted", DriftType: "missing_in_db", Action: "alert_only", Detail: "new", CreatedAt: now,
	}); err != nil {
		t.Fatalf("AddReconcileItem new error = %v", err)
	}

	cutoff := now.Add(-7 * 24 * time.Hour)
	res, err := s.PurgeHistoricalData(ctx, cutoff)
	if err != nil {
		t.Fatalf("PurgeHistoricalData error = %v", err)
	}
	if res.DeletedSandboxes != 1 {
		t.Fatalf("DeletedSandboxes = %d, want 1", res.DeletedSandboxes)
	}
	if res.DeletedStatusHistory != 1 {
		t.Fatalf("DeletedStatusHistory = %d, want 1", res.DeletedStatusHistory)
	}
	if res.DeletedReconcileRuns != 1 {
		t.Fatalf("DeletedReconcileRuns = %d, want 1", res.DeletedReconcileRuns)
	}
	if res.DeletedReconcileItems < 1 {
		t.Fatalf("DeletedReconcileItems = %d, want >= 1", res.DeletedReconcileItems)
	}

	kept, err := s.GetByID(ctx, "new-deleted")
	if err != nil {
		t.Fatalf("GetByID new-deleted error = %v", err)
	}
	if kept == nil {
		t.Fatalf("new-deleted should be kept")
	}
	removed, err := s.GetByID(ctx, "old-deleted")
	if err != nil {
		t.Fatalf("GetByID old-deleted error = %v", err)
	}
	if removed != nil {
		t.Fatalf("old-deleted should be purged")
	}
}

func TestSandboxStoreListMetadata(t *testing.T) {
	initTestDB(t)
	ctx := context.Background()
	s := NewSandboxStore()
	now := time.Now().UTC()

	mk := func(id, tpl, desired, lifecycle string, created time.Time, deleted *time.Time) *SandboxRecord {
		return &SandboxRecord{
			ID:                    id,
			TemplateName:          tpl,
			TemplateVersion:       1,
			Image:                 "python:3.11",
			CPU:                   "500m",
			Memory:                "512Mi",
			TTL:                   3600,
			EnvJSON:               `{}`,
			DesiredState:          desired,
			LifecycleStatus:       lifecycle,
			ClusterNamespace:      "liteboxd-sandbox",
			PodName:               "sandbox-" + id,
			AccessTokenCiphertext: "cipher",
			AccessTokenNonce:      "nonce",
			AccessTokenKeyID:      "v1",
			AccessTokenSHA256:     "hash",
			AccessURL:             "http://gateway/" + id,
			CreatedAt:             created,
			ExpiresAt:             created.Add(time.Hour),
			UpdatedAt:             created,
			DeletedAt:             deleted,
		}
	}

	delTs := now.Add(-time.Hour)
	records := []*SandboxRecord{
		mk("meta-1", "python", DesiredStateActive, "running", now.Add(-3*time.Hour), nil),
		mk("meta-2", "python", DesiredStateDeleted, "deleted", now.Add(-2*time.Hour), &delTs),
		mk("meta-3", "nodejs", DesiredStateActive, "failed", now.Add(-time.Hour), nil),
	}
	for i := range records {
		if err := s.Create(ctx, records[i]); err != nil {
			t.Fatalf("Create %s error = %v", records[i].ID, err)
		}
	}

	items, total, err := s.ListMetadata(ctx, SandboxMetadataQuery{
		Template:        "python",
		DesiredState:    DesiredStateDeleted,
		LifecycleStatus: "deleted",
		Page:            1,
		PageSize:        20,
	})
	if err != nil {
		t.Fatalf("ListMetadata error = %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != "meta-2" {
		t.Fatalf("ListMetadata unexpected result: total=%d len=%d first=%v", total, len(items), items)
	}

	pageItems, pageTotal, err := s.ListMetadata(ctx, SandboxMetadataQuery{
		Page:     2,
		PageSize: 1,
	})
	if err != nil {
		t.Fatalf("ListMetadata pagination error = %v", err)
	}
	if pageTotal != 3 || len(pageItems) != 1 {
		t.Fatalf("ListMetadata pagination unexpected: total=%d len=%d", pageTotal, len(pageItems))
	}
}

func TestSandboxStoreListExpiredActiveSkipsTTLZero(t *testing.T) {
	initTestDB(t)
	ctx := context.Background()
	s := NewSandboxStore()
	now := time.Now().UTC()

	mk := func(id string, ttl int, expiresAt time.Time) *SandboxRecord {
		return &SandboxRecord{
			ID:                    id,
			TemplateName:          "python",
			TemplateVersion:       1,
			Image:                 "python:3.11",
			CPU:                   "500m",
			Memory:                "512Mi",
			TTL:                   ttl,
			EnvJSON:               `{}`,
			DesiredState:          DesiredStateActive,
			LifecycleStatus:       "running",
			ClusterNamespace:      "liteboxd-sandbox",
			PodName:               "sandbox-" + id,
			AccessTokenCiphertext: "cipher",
			AccessTokenNonce:      "nonce",
			AccessTokenKeyID:      "v1",
			AccessTokenSHA256:     "hash",
			AccessURL:             "http://gateway/" + id,
			CreatedAt:             now.Add(-time.Hour),
			ExpiresAt:             expiresAt,
			UpdatedAt:             now.Add(-time.Hour),
		}
	}

	if err := s.Create(ctx, mk("ttl-zero", 0, now.Add(-10*time.Minute))); err != nil {
		t.Fatalf("Create ttl-zero error = %v", err)
	}
	if err := s.Create(ctx, mk("ttl-expired", 300, now.Add(-10*time.Minute))); err != nil {
		t.Fatalf("Create ttl-expired error = %v", err)
	}

	expired, err := s.ListExpiredActive(ctx, now)
	if err != nil {
		t.Fatalf("ListExpiredActive error = %v", err)
	}
	if len(expired) != 1 {
		t.Fatalf("ListExpiredActive len = %d, want 1", len(expired))
	}
	if expired[0].ID != "ttl-expired" {
		t.Fatalf("unexpected expired sandbox = %s", expired[0].ID)
	}
}

func TestSandboxStoreSetStopped(t *testing.T) {
	initTestDB(t)
	ctx := context.Background()
	s := NewSandboxStore()
	now := time.Now().UTC()

	rec := &SandboxRecord{
		ID:                    "stop-1",
		TemplateName:          "python",
		TemplateVersion:       1,
		Image:                 "python:3.11",
		CPU:                   "500m",
		Memory:                "512Mi",
		TTL:                   3600,
		EnvJSON:               `{}`,
		DesiredState:          DesiredStateActive,
		LifecycleStatus:       "running",
		ClusterNamespace:      "liteboxd-sandbox",
		PodName:               "sandbox-stop-1",
		PodUID:                "uid-orig",
		PodPhase:              "Running",
		PodIP:                 "10.0.0.1",
		AccessTokenCiphertext: "cipher",
		AccessTokenNonce:      "nonce",
		AccessTokenKeyID:      "v1",
		AccessTokenSHA256:     "hash",
		AccessURL:             "http://gateway/stop-1",
		PersistenceEnabled:    true,
		CreatedAt:             now,
		ExpiresAt:             now.Add(time.Hour),
		UpdatedAt:             now,
	}
	if err := s.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	stopTime := now.Add(10 * time.Minute)
	if err := s.SetStopped(ctx, rec.ID, stopTime); err != nil {
		t.Fatalf("SetStopped() error = %v", err)
	}

	got, err := s.GetByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.LifecycleStatus != "stopped" {
		t.Fatalf("LifecycleStatus = %q, want %q", got.LifecycleStatus, "stopped")
	}
	if got.StatusReason != "stopped by request" {
		t.Fatalf("StatusReason = %q, want %q", got.StatusReason, "stopped by request")
	}
	if got.PodPhase != "" {
		t.Fatalf("PodPhase = %q, want empty", got.PodPhase)
	}
	if got.PodIP != "" {
		t.Fatalf("PodIP = %q, want empty", got.PodIP)
	}
	if got.PodUID != "" {
		t.Fatalf("PodUID = %q, want empty", got.PodUID)
	}
	if got.StoppedAt == nil {
		t.Fatalf("StoppedAt is nil, want non-nil")
	}
	if got.StoppedAt.Sub(stopTime).Abs() > time.Second {
		t.Fatalf("StoppedAt = %v, want ~%v", *got.StoppedAt, stopTime)
	}
}

func TestSandboxStoreSetStarted(t *testing.T) {
	initTestDB(t)
	ctx := context.Background()
	s := NewSandboxStore()
	now := time.Now().UTC()

	originalExpires := now.Add(time.Hour)
	rec := &SandboxRecord{
		ID:                    "start-1",
		TemplateName:          "python",
		TemplateVersion:       1,
		Image:                 "python:3.11",
		CPU:                   "500m",
		Memory:                "512Mi",
		TTL:                   3600,
		EnvJSON:               `{}`,
		DesiredState:          DesiredStateActive,
		LifecycleStatus:       "running",
		ClusterNamespace:      "liteboxd-sandbox",
		PodName:               "sandbox-start-1",
		AccessTokenCiphertext: "cipher",
		AccessTokenNonce:      "nonce",
		AccessTokenKeyID:      "v1",
		AccessTokenSHA256:     "hash",
		AccessURL:             "http://gateway/start-1",
		PersistenceEnabled:    true,
		CreatedAt:             now,
		ExpiresAt:             originalExpires,
		UpdatedAt:             now,
	}
	if err := s.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	stopTime := now.Add(10 * time.Minute)
	if err := s.SetStopped(ctx, rec.ID, stopTime); err != nil {
		t.Fatalf("SetStopped() error = %v", err)
	}

	startTime := now.Add(40 * time.Minute)
	newExpires := originalExpires.Add(30 * time.Minute)
	if err := s.SetStarted(ctx, rec.ID, newExpires, startTime); err != nil {
		t.Fatalf("SetStarted() error = %v", err)
	}

	got, err := s.GetByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.LifecycleStatus != "pending" {
		t.Fatalf("LifecycleStatus = %q, want %q", got.LifecycleStatus, "pending")
	}
	if got.StatusReason != "start requested" {
		t.Fatalf("StatusReason = %q, want %q", got.StatusReason, "start requested")
	}
	if got.StoppedAt != nil {
		t.Fatalf("StoppedAt = %v, want nil", got.StoppedAt)
	}
	if got.ExpiresAt.Sub(newExpires).Abs() > time.Second {
		t.Fatalf("ExpiresAt = %v, want ~%v", got.ExpiresAt, newExpires)
	}
}

func TestSandboxStoreListExpiredActiveExcludesStopped(t *testing.T) {
	initTestDB(t)
	ctx := context.Background()
	s := NewSandboxStore()
	now := time.Now().UTC()

	mk := func(id, lifecycle string) *SandboxRecord {
		return &SandboxRecord{
			ID:                    id,
			TemplateName:          "python",
			TemplateVersion:       1,
			Image:                 "python:3.11",
			CPU:                   "500m",
			Memory:                "512Mi",
			TTL:                   300,
			EnvJSON:               `{}`,
			DesiredState:          DesiredStateActive,
			LifecycleStatus:       lifecycle,
			ClusterNamespace:      "liteboxd-sandbox",
			PodName:               "sandbox-" + id,
			AccessTokenCiphertext: "cipher",
			AccessTokenNonce:      "nonce",
			AccessTokenKeyID:      "v1",
			AccessTokenSHA256:     "hash",
			AccessURL:             "http://gateway/" + id,
			PersistenceEnabled:    true,
			CreatedAt:             now.Add(-time.Hour),
			ExpiresAt:             now.Add(-10 * time.Minute),
			UpdatedAt:             now.Add(-time.Hour),
		}
	}

	if err := s.Create(ctx, mk("exp-running", "running")); err != nil {
		t.Fatalf("Create running error = %v", err)
	}
	if err := s.Create(ctx, mk("exp-stopped", "stopped")); err != nil {
		t.Fatalf("Create stopped error = %v", err)
	}

	expired, err := s.ListExpiredActive(ctx, now)
	if err != nil {
		t.Fatalf("ListExpiredActive error = %v", err)
	}
	if len(expired) != 1 {
		t.Fatalf("ListExpiredActive len = %d, want 1", len(expired))
	}
	if expired[0].ID != "exp-running" {
		t.Fatalf("expired sandbox = %s, want exp-running", expired[0].ID)
	}
}

func TestSandboxStoreStoppedAtRoundTrip(t *testing.T) {
	initTestDB(t)
	ctx := context.Background()
	s := NewSandboxStore()
	now := time.Now().UTC()

	stoppedAt := now.Add(-5 * time.Minute)
	rec := &SandboxRecord{
		ID:                    "rt-1",
		TemplateName:          "python",
		TemplateVersion:       1,
		Image:                 "python:3.11",
		CPU:                   "500m",
		Memory:                "512Mi",
		TTL:                   3600,
		EnvJSON:               `{}`,
		DesiredState:          DesiredStateActive,
		LifecycleStatus:       "stopped",
		ClusterNamespace:      "liteboxd-sandbox",
		PodName:               "sandbox-rt-1",
		AccessTokenCiphertext: "cipher",
		AccessTokenNonce:      "nonce",
		AccessTokenKeyID:      "v1",
		AccessTokenSHA256:     "hash",
		AccessURL:             "http://gateway/rt-1",
		PersistenceEnabled:    true,
		CreatedAt:             now,
		ExpiresAt:             now.Add(time.Hour),
		UpdatedAt:             now,
		StoppedAt:             &stoppedAt,
	}
	if err := s.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := s.GetByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.StoppedAt == nil {
		t.Fatalf("StoppedAt is nil, want non-nil")
	}
	if got.StoppedAt.Sub(stoppedAt).Abs() > time.Second {
		t.Fatalf("StoppedAt = %v, want ~%v", *got.StoppedAt, stoppedAt)
	}

	// Verify nil StoppedAt also round-trips correctly
	rec2 := &SandboxRecord{
		ID:                    "rt-2",
		TemplateName:          "python",
		TemplateVersion:       1,
		Image:                 "python:3.11",
		CPU:                   "500m",
		Memory:                "512Mi",
		TTL:                   3600,
		EnvJSON:               `{}`,
		DesiredState:          DesiredStateActive,
		LifecycleStatus:       "running",
		ClusterNamespace:      "liteboxd-sandbox",
		PodName:               "sandbox-rt-2",
		AccessTokenCiphertext: "cipher",
		AccessTokenNonce:      "nonce",
		AccessTokenKeyID:      "v1",
		AccessTokenSHA256:     "hash",
		AccessURL:             "http://gateway/rt-2",
		CreatedAt:             now,
		ExpiresAt:             now.Add(time.Hour),
		UpdatedAt:             now,
	}
	if err := s.Create(ctx, rec2); err != nil {
		t.Fatalf("Create() rec2 error = %v", err)
	}
	got2, err := s.GetByID(ctx, rec2.ID)
	if err != nil {
		t.Fatalf("GetByID() rec2 error = %v", err)
	}
	if got2.StoppedAt != nil {
		t.Fatalf("StoppedAt = %v, want nil", got2.StoppedAt)
	}
}

func TestSandboxStoreListStatusHistory(t *testing.T) {
	initTestDB(t)
	ctx := context.Background()
	s := NewSandboxStore()
	now := time.Now().UTC()

	rec := &SandboxRecord{
		ID:                    "hist-1",
		TemplateName:          "python",
		TemplateVersion:       1,
		Image:                 "python:3.11",
		CPU:                   "500m",
		Memory:                "512Mi",
		TTL:                   3600,
		EnvJSON:               `{}`,
		DesiredState:          DesiredStateActive,
		LifecycleStatus:       "running",
		ClusterNamespace:      "liteboxd-sandbox",
		PodName:               "sandbox-hist-1",
		AccessTokenCiphertext: "cipher",
		AccessTokenNonce:      "nonce",
		AccessTokenKeyID:      "v1",
		AccessTokenSHA256:     "hash",
		AccessURL:             "http://gateway/hist-1",
		CreatedAt:             now,
		ExpiresAt:             now.Add(time.Hour),
		UpdatedAt:             now,
	}
	if err := s.Create(ctx, rec); err != nil {
		t.Fatalf("Create error = %v", err)
	}

	if err := s.AppendStatusHistory(ctx, rec.ID, "api", "", "creating", "create requested", map[string]any{"k": "v"}, now.Add(-2*time.Minute)); err != nil {
		t.Fatalf("AppendStatusHistory 1 error = %v", err)
	}
	if err := s.AppendStatusHistory(ctx, rec.ID, "watcher", "creating", "running", "ready", nil, now.Add(-time.Minute)); err != nil {
		t.Fatalf("AppendStatusHistory 2 error = %v", err)
	}

	items, err := s.ListStatusHistory(ctx, rec.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListStatusHistory error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("ListStatusHistory len=%d, want 2", len(items))
	}
	if items[0].ToStatus != "running" || items[1].ToStatus != "creating" {
		t.Fatalf("ListStatusHistory order unexpected: %+v", items)
	}

	paged, err := s.ListStatusHistory(ctx, rec.ID, 10, items[0].ID)
	if err != nil {
		t.Fatalf("ListStatusHistory before_id error = %v", err)
	}
	if len(paged) != 1 || paged[0].ID != items[1].ID {
		t.Fatalf("ListStatusHistory before_id unexpected: %+v", paged)
	}
}

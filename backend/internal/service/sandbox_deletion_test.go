package service

import (
	"context"
	"testing"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/k8s"
	"github.com/fslongjin/liteboxd/backend/internal/store"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDeleteMarksDeletionRequested(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := makeTestSandboxRecord("del-requested", true, "running")
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	svc := &SandboxService{sandboxStore: sandboxStore}
	resp, err := svc.Delete(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if resp.Deletion == nil || resp.Deletion.Phase != store.DeletionPhaseRequested {
		t.Fatalf("Delete() deletion phase = %+v, want requested", resp.Deletion)
	}

	got, err := sandboxStore.GetByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.LifecycleStatus != "terminating" {
		t.Fatalf("LifecycleStatus = %q, want terminating", got.LifecycleStatus)
	}
	if got.DeletionPhase != store.DeletionPhaseRequested {
		t.Fatalf("DeletionPhase = %q, want requested", got.DeletionPhase)
	}
	if got.DeletedAt != nil {
		t.Fatalf("DeletedAt = %v, want nil", got.DeletedAt)
	}
}

func TestDeleteIsIdempotentWhileTerminating(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := makeTestSandboxRecord("del-idempotent", true, "running")
	now := time.Now().UTC().Add(-2 * time.Minute)
	rec.DesiredState = store.DesiredStateDeleted
	rec.LifecycleStatus = "terminating"
	rec.StatusReason = "delete requested"
	rec.DeletionPhase = store.DeletionPhaseDeletingStorage
	rec.DeletionStartedAt = &now
	rec.DeletionLastAttemptAt = &now
	rec.DeletionNextRetryAt = &now
	rec.DeletionAttempts = 3
	rec.DeletionForceLevel = 1
	rec.DeletionLastError = "pvc still attached"
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := sandboxStore.AppendStatusHistory(ctx, rec.ID, "api", "running", "terminating", "delete requested", nil, now); err != nil {
		t.Fatalf("AppendStatusHistory() error = %v", err)
	}

	svc := &SandboxService{sandboxStore: sandboxStore}
	resp, err := svc.Delete(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if resp.Deletion == nil {
		t.Fatalf("Delete() deletion = nil, want populated deletion progress")
	}
	if resp.Deletion.Phase != store.DeletionPhaseDeletingStorage {
		t.Fatalf("Delete() deletion phase = %q, want %q", resp.Deletion.Phase, store.DeletionPhaseDeletingStorage)
	}
	if resp.Deletion.Attempts != 3 {
		t.Fatalf("Delete() deletion attempts = %d, want 3", resp.Deletion.Attempts)
	}
	if resp.Deletion.ForceLevel != 1 {
		t.Fatalf("Delete() deletion force level = %d, want 1", resp.Deletion.ForceLevel)
	}

	got, err := sandboxStore.GetByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.DeletionPhase != store.DeletionPhaseDeletingStorage {
		t.Fatalf("DeletionPhase = %q, want %q", got.DeletionPhase, store.DeletionPhaseDeletingStorage)
	}
	if got.DeletionAttempts != 3 {
		t.Fatalf("DeletionAttempts = %d, want 3", got.DeletionAttempts)
	}
	if got.DeletionForceLevel != 1 {
		t.Fatalf("DeletionForceLevel = %d, want 1", got.DeletionForceLevel)
	}
	if got.DeletionLastError != "pvc still attached" {
		t.Fatalf("DeletionLastError = %q, want pvc still attached", got.DeletionLastError)
	}

	history, err := sandboxStore.ListStatusHistory(ctx, rec.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListStatusHistory() error = %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("ListStatusHistory() len = %d, want 1", len(history))
	}
}

func TestSandboxDeletionServiceCompletesPersistentDelete(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := makeTestSandboxRecord("del-pvc", true, "running")
	rec.VolumeClaimName = "sandbox-data-del-pvc"
	rec.VolumeReclaimPolicy = "Delete"
	now := time.Now().UTC().Add(-10 * time.Minute)
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := sandboxStore.MarkDeletionRequested(ctx, rec.ID, now); err != nil {
		t.Fatalf("MarkDeletionRequested() error = %v", err)
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rec.VolumeClaimName,
			Namespace: k8s.DefaultSandboxNamespace,
		},
	}
	client := k8s.NewClientForTest(pvc)
	deletionSvc := NewSandboxDeletionService(client, sandboxStore)
	for i := 0; i < 6; i++ {
		if err := deletionSvc.RunPending(ctx); err != nil {
			t.Fatalf("RunPending() error = %v", err)
		}
	}

	got, err := sandboxStore.GetByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.LifecycleStatus != "deleted" {
		t.Fatalf("LifecycleStatus = %q, want deleted", got.LifecycleStatus)
	}
	if got.DeletionPhase != store.DeletionPhaseCompleted {
		t.Fatalf("DeletionPhase = %q, want completed", got.DeletionPhase)
	}
	if _, err := client.GetPersistentVolumeClaim(ctx, rec.VolumeClaimName); err == nil {
		t.Fatalf("PVC still exists, want deleted")
	}
}

func TestSandboxDeletionServiceRetainKeepsPVC(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := makeTestSandboxRecord("del-retain", true, "running")
	rec.VolumeClaimName = "sandbox-data-del-retain"
	rec.VolumeReclaimPolicy = "Retain"
	now := time.Now().UTC().Add(-10 * time.Minute)
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := sandboxStore.MarkDeletionRequested(ctx, rec.ID, now); err != nil {
		t.Fatalf("MarkDeletionRequested() error = %v", err)
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rec.VolumeClaimName,
			Namespace: k8s.DefaultSandboxNamespace,
		},
	}
	client := k8s.NewClientForTest(pvc)
	deletionSvc := NewSandboxDeletionService(client, sandboxStore)
	for i := 0; i < 4; i++ {
		if err := deletionSvc.RunPending(ctx); err != nil {
			t.Fatalf("RunPending() error = %v", err)
		}
	}

	got, err := sandboxStore.GetByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.DeletionPhase != store.DeletionPhaseCompleted {
		t.Fatalf("DeletionPhase = %q, want completed", got.DeletionPhase)
	}
	if _, err := client.GetPersistentVolumeClaim(ctx, rec.VolumeClaimName); err != nil {
		t.Fatalf("PVC get error = %v, want retained", err)
	}
}

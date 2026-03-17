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

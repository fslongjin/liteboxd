package service

import (
	"context"
	"testing"

	"github.com/fslongjin/liteboxd/backend/internal/k8s"
	"github.com/fslongjin/liteboxd/backend/internal/model"
	"github.com/fslongjin/liteboxd/backend/internal/security"
	"github.com/fslongjin/liteboxd/backend/internal/store"
)

func TestCreatePersistentSandboxReturnsPendingLifecycle(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()

	t.Setenv(security.TokenEncryptionKeyEnv, "0123456789abcdef")
	cipher, err := security.NewTokenCipherFromEnv()
	if err != nil {
		t.Fatalf("NewTokenCipherFromEnv() error = %v", err)
	}

	templateSvc := NewTemplateService()
	if _, err := templateSvc.store.Create(ctx, &model.CreateTemplateRequest{
		Name: "persistent-pending",
		Spec: model.TemplateSpec{
			Image:          "busybox:1.36",
			Command:        []string{"sh", "-c", "sleep 30"},
			StartupTimeout: 1,
			Persistence: &model.PersistenceSpec{
				Enabled:          true,
				Mode:             model.PersistenceModeRootFSOverlay,
				Size:             "20Gi",
				StorageClassName: "longhorn",
				ReclaimPolicy:    model.PersistenceReclaimDelete,
			},
		},
	}); err != nil {
		t.Fatalf("Create template error = %v", err)
	}

	svc := NewSandboxService(k8s.NewClientForTest(), store.NewSandboxStore(), cipher)
	svc.SetTemplateService(templateSvc)

	sb, err := svc.Create(ctx, &model.CreateSandboxRequest{Template: "persistent-pending"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if sb.LifecycleStatus != "pending" {
		t.Fatalf("LifecycleStatus = %q, want pending", sb.LifecycleStatus)
	}
	if sb.StatusReason != "" {
		t.Fatalf("StatusReason = %q, want empty", sb.StatusReason)
	}
}

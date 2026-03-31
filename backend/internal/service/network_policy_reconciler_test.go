package service

import (
	"context"
	"testing"

	"github.com/fslongjin/liteboxd/backend/internal/k8s"
	"github.com/fslongjin/liteboxd/backend/internal/model"
	"github.com/fslongjin/liteboxd/backend/internal/store"
)

func TestSandboxNetworkPolicyReconcilerCreatesPolicyForEligibleSandbox(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()

	templateSvc := NewTemplateService()
	if _, err := templateSvc.store.Create(ctx, &model.CreateTemplateRequest{
		Name: "allowlisted",
		Spec: model.TemplateSpec{
			Image: "busybox:1.36",
			Network: &model.NetworkSpec{
				AllowInternetAccess: true,
				AllowedDomains:      []string{"example.com"},
			},
		},
	}); err != nil {
		t.Fatalf("Create template error = %v", err)
	}

	rec := makeTestSandboxRecord("np-eligible", false, "running")
	rec.TemplateName = "allowlisted"
	rec.TemplateVersion = 1
	if err := store.NewSandboxStore().Create(ctx, rec); err != nil {
		t.Fatalf("Create sandbox error = %v", err)
	}

	k8sClient := k8s.NewClientForTestWithDynamic()
	reconciler := NewSandboxNetworkPolicyReconciler(k8sClient, store.NewSandboxStore(), store.NewTemplateStore())
	if err := reconciler.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	mgr := k8s.NewNetworkPolicyManager(k8sClient)
	if _, err := mgr.GetDomainAllowlistPolicy(ctx, rec.ID); err != nil {
		t.Fatalf("GetDomainAllowlistPolicy() error = %v", err)
	}
}

func TestSandboxNetworkPolicyReconcilerDeletesPolicyForTerminatingSandbox(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()

	templateSvc := NewTemplateService()
	if _, err := templateSvc.store.Create(ctx, &model.CreateTemplateRequest{
		Name: "allowlisted",
		Spec: model.TemplateSpec{
			Image: "busybox:1.36",
			Network: &model.NetworkSpec{
				AllowInternetAccess: true,
				AllowedDomains:      []string{"example.com"},
			},
		},
	}); err != nil {
		t.Fatalf("Create template error = %v", err)
	}

	rec := makeTestSandboxRecord("np-terminating", false, "terminating")
	rec.TemplateName = "allowlisted"
	rec.TemplateVersion = 1
	rec.DesiredState = store.DesiredStateDeleted
	if err := store.NewSandboxStore().Create(ctx, rec); err != nil {
		t.Fatalf("Create sandbox error = %v", err)
	}

	k8sClient := k8s.NewClientForTestWithDynamic()
	mgr := k8s.NewNetworkPolicyManager(k8sClient)
	if err := mgr.ApplyDomainAllowlistPolicy(ctx, rec.ID, []string{"example.com"}); err != nil {
		t.Fatalf("ApplyDomainAllowlistPolicy() error = %v", err)
	}

	reconciler := NewSandboxNetworkPolicyReconciler(k8sClient, store.NewSandboxStore(), store.NewTemplateStore())
	if err := reconciler.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if _, err := mgr.GetDomainAllowlistPolicy(ctx, rec.ID); err == nil {
		t.Fatalf("expected policy to be deleted")
	}
}

func TestSandboxNetworkPolicyReconcilerDeletesOrphanPolicy(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()

	k8sClient := k8s.NewClientForTestWithDynamic()
	mgr := k8s.NewNetworkPolicyManager(k8sClient)
	if err := mgr.ApplyDomainAllowlistPolicy(ctx, "orphaned", []string{"example.com"}); err != nil {
		t.Fatalf("ApplyDomainAllowlistPolicy() error = %v", err)
	}

	reconciler := NewSandboxNetworkPolicyReconciler(k8sClient, store.NewSandboxStore(), store.NewTemplateStore())
	if err := reconciler.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if _, err := mgr.GetDomainAllowlistPolicy(ctx, "orphaned"); err == nil {
		t.Fatalf("expected orphan policy to be deleted")
	}
}

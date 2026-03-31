package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/k8s"
	"github.com/fslongjin/liteboxd/backend/internal/store"
)

type SandboxNetworkPolicyReconciler struct {
	k8sClient     *k8s.Client
	sandboxStore  *store.SandboxStore
	templateStore *store.TemplateStore
}

func NewSandboxNetworkPolicyReconciler(k8sClient *k8s.Client, sandboxStore *store.SandboxStore, templateStore *store.TemplateStore) *SandboxNetworkPolicyReconciler {
	return &SandboxNetworkPolicyReconciler{
		k8sClient:     k8sClient,
		sandboxStore:  sandboxStore,
		templateStore: templateStore,
	}
}

func (r *SandboxNetworkPolicyReconciler) Start(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			if err := r.RunOnce(context.Background()); err != nil {
				slog.Default().With("component", "sandbox_network_policy_reconciler").Error("scheduled network policy reconcile failed", "error", err)
			}
		}
	}()
}

func (r *SandboxNetworkPolicyReconciler) RunOnce(ctx context.Context) error {
	if r.k8sClient.GetDynamicClient() == nil {
		return nil
	}

	records, err := r.sandboxStore.ListForReconcile(ctx)
	if err != nil {
		return err
	}

	manager := k8s.NewNetworkPolicyManager(r.k8sClient)
	desiredPolicies := make(map[string][]string, len(records))
	recordsByID := make(map[string]store.SandboxRecord, len(records))

	applied := 0
	deleted := 0
	failures := 0

	for i := range records {
		rec := records[i]
		recordsByID[rec.ID] = rec

		domains, shouldExist, err := r.desiredAllowedDomains(ctx, &rec)
		if err != nil {
			failures++
			logWithSandboxID(ctx, rec.ID).Warn("failed to resolve desired network policy state", "error", err)
			continue
		}
		if !shouldExist {
			continue
		}

		desiredPolicies[rec.ID] = domains
		if err := manager.ApplyDomainAllowlistPolicy(ctx, rec.ID, domains); err != nil {
			failures++
			logWithSandboxID(ctx, rec.ID).Warn("failed to reconcile domain allowlist policy", "error", err)
			continue
		}
		applied++
	}

	policies, err := manager.ListManagedDomainAllowlistPolicies(ctx)
	if err != nil {
		return err
	}

	for i := range policies {
		sandboxID, ok := k8s.ParseDomainAllowlistPolicyName(policies[i].GetName())
		if !ok {
			continue
		}
		if _, ok := desiredPolicies[sandboxID]; ok {
			continue
		}

		rec, exists := recordsByID[sandboxID]
		if exists {
			_, shouldExist, err := r.desiredAllowedDomains(ctx, &rec)
			if err != nil {
				failures++
				logWithSandboxID(ctx, sandboxID).Warn("failed to confirm network policy cleanup state", "error", err)
				continue
			}
			if shouldExist {
				continue
			}
		}

		if err := manager.DeleteDomainAllowlistPolicy(ctx, sandboxID); err != nil {
			failures++
			logWithSandboxID(ctx, sandboxID).Warn("failed to delete domain allowlist policy", "error", err)
			continue
		}
		deleted++
	}

	slog.Default().With("component", "sandbox_network_policy_reconciler").Info(
		"sandbox network policy reconcile completed",
		"sandbox_count", len(records),
		"desired_policy_count", len(desiredPolicies),
		"applied_count", applied,
		"deleted_count", deleted,
		"failure_count", failures,
	)

	return nil
}

func (r *SandboxNetworkPolicyReconciler) desiredAllowedDomains(ctx context.Context, rec *store.SandboxRecord) ([]string, bool, error) {
	if rec.DesiredState == store.DesiredStateDeleted {
		return nil, false, nil
	}
	switch rec.LifecycleStatus {
	case "deleted", "terminating", "failed", "succeeded":
		return nil, false, nil
	}

	version, err := r.templateStore.GetVersionByName(ctx, rec.TemplateName, rec.TemplateVersion)
	if err != nil {
		return nil, false, err
	}
	if version == nil || version.Spec.Network == nil {
		return nil, false, nil
	}
	if !version.Spec.Network.AllowInternetAccess || len(version.Spec.Network.AllowedDomains) == 0 {
		return nil, false, nil
	}
	return version.Spec.Network.AllowedDomains, true, nil
}

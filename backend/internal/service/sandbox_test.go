package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fslongjin/liteboxd/backend/internal/k8s"
	"github.com/fslongjin/liteboxd/backend/internal/model"
	"github.com/fslongjin/liteboxd/backend/internal/store"
)

func initServiceTestDB(t *testing.T) {
	t.Helper()
	if err := store.InitDB(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	t.Cleanup(func() { _ = store.CloseDB() })
}

func makeTestSandboxRecord(id string, persistent bool, lifecycle string) *store.SandboxRecord {
	now := time.Now().UTC()
	return &store.SandboxRecord{
		ID:                    id,
		TemplateName:          "python",
		TemplateVersion:       1,
		Image:                 "python:3.11",
		CPU:                   "500m",
		Memory:                "512Mi",
		TTL:                   3600,
		EnvJSON:               `{}`,
		DesiredState:          store.DesiredStateActive,
		LifecycleStatus:       lifecycle,
		ClusterNamespace:      "liteboxd-sandbox",
		PodName:               "sandbox-" + id,
		PodUID:                "uid-1",
		PodPhase:              "Running",
		PodIP:                 "10.0.0.1",
		AccessTokenCiphertext: "cipher",
		AccessTokenNonce:      "nonce",
		AccessTokenKeyID:      "v1",
		AccessTokenSHA256:     "hash",
		AccessURL:             "http://gateway/" + id,
		PersistenceEnabled:    persistent,
		RuntimeKind:           "deployment",
		RuntimeName:           "sandbox-" + id,
		CreatedAt:             now,
		ExpiresAt:             now.Add(time.Hour),
		UpdatedAt:             now,
	}
}

func makeTestDeploymentForService(sandboxID string, replicas int32) *appsv1.Deployment {
	deployName := fmt.Sprintf("sandbox-%s", sandboxID)
	labels := map[string]string{
		"app":        "liteboxd",
		"sandbox-id": sandboxID,
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName,
			Namespace: "liteboxd-sandbox",
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
		},
	}
}

// --- parseLifecycleStatus tests ---

func TestParseLifecycleStatusStopped(t *testing.T) {
	got := parseLifecycleStatus("stopped")
	if got != model.SandboxStatusStopped {
		t.Fatalf("parseLifecycleStatus(\"stopped\") = %q, want %q", got, model.SandboxStatusStopped)
	}
}

func TestParseLifecycleStatusAll(t *testing.T) {
	cases := []struct {
		input string
		want  model.SandboxStatus
	}{
		{"pending", model.SandboxStatusPending},
		{"creating", model.SandboxStatusPending},
		{"running", model.SandboxStatusRunning},
		{"succeeded", model.SandboxStatusSucceeded},
		{"failed", model.SandboxStatusFailed},
		{"stopped", model.SandboxStatusStopped},
		{"terminating", model.SandboxStatusTerminating},
		{"", model.SandboxStatusUnknown},
		{"bogus", model.SandboxStatusUnknown},
	}
	for _, tc := range cases {
		got := parseLifecycleStatus(tc.input)
		if got != tc.want {
			t.Errorf("parseLifecycleStatus(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- Stop validation tests ---

func TestStopNotFound(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()
	svc := &SandboxService{sandboxStore: sandboxStore}

	err := svc.Stop(ctx, "nonexistent")
	if !errors.Is(err, ErrSandboxNotFound) {
		t.Fatalf("Stop(nonexistent) error = %v, want ErrSandboxNotFound", err)
	}
}

func TestStopNonPersistent(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := makeTestSandboxRecord("stop-np", false, "running")
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	svc := &SandboxService{sandboxStore: sandboxStore}
	err := svc.Stop(ctx, "stop-np")
	if !errors.Is(err, ErrSandboxStopNotSupported) {
		t.Fatalf("Stop(non-persistent) error = %v, want ErrSandboxStopNotSupported", err)
	}
}

func TestStopAlreadyStopped(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := makeTestSandboxRecord("stop-dup", true, "stopped")
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	svc := &SandboxService{sandboxStore: sandboxStore}
	err := svc.Stop(ctx, "stop-dup")
	if !errors.Is(err, ErrSandboxAlreadyStopped) {
		t.Fatalf("Stop(already-stopped) error = %v, want ErrSandboxAlreadyStopped", err)
	}
}

func TestStopTerminating(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := makeTestSandboxRecord("stop-term", true, "terminating")
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	svc := &SandboxService{sandboxStore: sandboxStore}
	err := svc.Stop(ctx, "stop-term")
	if !errors.Is(err, ErrSandboxStopInvalidState) {
		t.Fatalf("Stop(terminating) error = %v, want ErrSandboxStopInvalidState", err)
	}
}

func TestStopDeletedSandbox(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := makeTestSandboxRecord("stop-del", true, "deleted")
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	svc := &SandboxService{sandboxStore: sandboxStore}
	err := svc.Stop(ctx, "stop-del")
	if !errors.Is(err, ErrSandboxNotFound) {
		t.Fatalf("Stop(deleted) error = %v, want ErrSandboxNotFound", err)
	}
}

func TestStopSuccess(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	deploy := makeTestDeploymentForService("stop-ok", 1)
	k8sClient := k8s.NewClientForTest(deploy)

	rec := makeTestSandboxRecord("stop-ok", true, "running")
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	svc := &SandboxService{sandboxStore: sandboxStore, k8sClient: k8sClient}
	if err := svc.Stop(ctx, "stop-ok"); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	got, err := sandboxStore.GetByID(ctx, "stop-ok")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.LifecycleStatus != "stopped" {
		t.Fatalf("LifecycleStatus = %q, want %q", got.LifecycleStatus, "stopped")
	}
	if got.StoppedAt == nil {
		t.Fatalf("StoppedAt is nil, want non-nil")
	}
	if got.PodPhase != "" || got.PodIP != "" || got.PodUID != "" {
		t.Fatalf("pod metadata not cleared: phase=%q ip=%q uid=%q", got.PodPhase, got.PodIP, got.PodUID)
	}

	history, err := sandboxStore.ListStatusHistory(ctx, "stop-ok", 10, 0)
	if err != nil {
		t.Fatalf("ListStatusHistory() error = %v", err)
	}
	if len(history) == 0 {
		t.Fatalf("expected status history entries")
	}
	last := history[0]
	if last.ToStatus != "stopped" {
		t.Fatalf("history ToStatus = %q, want %q", last.ToStatus, "stopped")
	}
}

// --- Start validation tests ---

func TestStartNotFound(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()
	svc := &SandboxService{sandboxStore: sandboxStore}

	err := svc.Start(ctx, "nonexistent")
	if !errors.Is(err, ErrSandboxNotFound) {
		t.Fatalf("Start(nonexistent) error = %v, want ErrSandboxNotFound", err)
	}
}

func TestStartNonPersistent(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := makeTestSandboxRecord("start-np", false, "stopped")
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	svc := &SandboxService{sandboxStore: sandboxStore}
	err := svc.Start(ctx, "start-np")
	if !errors.Is(err, ErrSandboxStartNotSupported) {
		t.Fatalf("Start(non-persistent) error = %v, want ErrSandboxStartNotSupported", err)
	}
}

func TestStartNotStopped(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := makeTestSandboxRecord("start-run", true, "running")
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	svc := &SandboxService{sandboxStore: sandboxStore}
	err := svc.Start(ctx, "start-run")
	if !errors.Is(err, ErrSandboxNotStopped) {
		t.Fatalf("Start(running) error = %v, want ErrSandboxNotStopped", err)
	}
}

func TestStartDeletedSandbox(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := makeTestSandboxRecord("start-del", true, "deleted")
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	svc := &SandboxService{sandboxStore: sandboxStore}
	err := svc.Start(ctx, "start-del")
	if !errors.Is(err, ErrSandboxNotFound) {
		t.Fatalf("Start(deleted) error = %v, want ErrSandboxNotFound", err)
	}
}

func TestStartSuccess(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	deploy := makeTestDeploymentForService("start-ok", 0)
	k8sClient := k8s.NewClientForTest(deploy)

	now := time.Now().UTC()
	stoppedAt := now.Add(-30 * time.Minute)
	originalExpires := now.Add(30 * time.Minute)

	rec := makeTestSandboxRecord("start-ok", true, "stopped")
	rec.StoppedAt = &stoppedAt
	rec.ExpiresAt = originalExpires
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	svc := &SandboxService{sandboxStore: sandboxStore, k8sClient: k8sClient}
	if err := svc.Start(ctx, "start-ok"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	got, err := sandboxStore.GetByID(ctx, "start-ok")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.LifecycleStatus != "pending" {
		t.Fatalf("LifecycleStatus = %q, want %q", got.LifecycleStatus, "pending")
	}
	if got.StoppedAt != nil {
		t.Fatalf("StoppedAt = %v, want nil", got.StoppedAt)
	}
	// TTL should have been extended by ~30 minutes (the stopped duration)
	extension := got.ExpiresAt.Sub(originalExpires)
	if extension < 29*time.Minute || extension > 31*time.Minute {
		t.Fatalf("ExpiresAt extension = %v, want ~30m (original=%v, new=%v)", extension, originalExpires, got.ExpiresAt)
	}

	history, err := sandboxStore.ListStatusHistory(ctx, "start-ok", 10, 0)
	if err != nil {
		t.Fatalf("ListStatusHistory() error = %v", err)
	}
	if len(history) == 0 {
		t.Fatalf("expected status history entries")
	}
	last := history[0]
	if last.FromStatus != "stopped" || last.ToStatus != "pending" {
		t.Fatalf("history from=%q to=%q, want stopped→pending", last.FromStatus, last.ToStatus)
	}
}

func TestStartWithNilStoppedAt(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	deploy := makeTestDeploymentForService("start-nil", 0)
	k8sClient := k8s.NewClientForTest(deploy)

	now := time.Now().UTC()
	originalExpires := now.Add(time.Hour)

	rec := makeTestSandboxRecord("start-nil", true, "stopped")
	rec.StoppedAt = nil // edge case: stopped_at somehow nil
	rec.ExpiresAt = originalExpires
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	svc := &SandboxService{sandboxStore: sandboxStore, k8sClient: k8sClient}
	if err := svc.Start(ctx, "start-nil"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	got, err := sandboxStore.GetByID(ctx, "start-nil")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	// ExpiresAt should stay the same (no extension since StoppedAt was nil)
	if got.ExpiresAt.Sub(originalExpires).Abs() > time.Second {
		t.Fatalf("ExpiresAt = %v, want ~%v (no extension)", got.ExpiresAt, originalExpires)
	}
}

// --- Restart guard tests ---

func TestRestartRejectsStoppedSandbox(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := makeTestSandboxRecord("restart-stopped", true, "stopped")
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	svc := &SandboxService{sandboxStore: sandboxStore}
	err := svc.Restart(ctx, "restart-stopped")
	if !errors.Is(err, ErrSandboxRestartInvalidState) {
		t.Fatalf("Restart(stopped) error = %v, want ErrSandboxRestartInvalidState", err)
	}
}

// --- Full stop→start cycle test ---

func TestStopThenStartCycle(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	deploy := makeTestDeploymentForService("cycle-1", 1)
	k8sClient := k8s.NewClientForTest(deploy)

	now := time.Now().UTC()
	rec := makeTestSandboxRecord("cycle-1", true, "running")
	rec.ExpiresAt = now.Add(time.Hour)
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	svc := &SandboxService{sandboxStore: sandboxStore, k8sClient: k8sClient}

	// Stop
	if err := svc.Stop(ctx, "cycle-1"); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	stopped, _ := sandboxStore.GetByID(ctx, "cycle-1")
	if stopped.LifecycleStatus != "stopped" {
		t.Fatalf("after stop: LifecycleStatus = %q, want stopped", stopped.LifecycleStatus)
	}

	// Try to restart → should fail
	if err := svc.Restart(ctx, "cycle-1"); !errors.Is(err, ErrSandboxRestartInvalidState) {
		t.Fatalf("Restart(stopped) error = %v, want ErrSandboxRestartInvalidState", err)
	}

	// Try to stop again → should fail
	if err := svc.Stop(ctx, "cycle-1"); !errors.Is(err, ErrSandboxAlreadyStopped) {
		t.Fatalf("Stop(already-stopped) error = %v, want ErrSandboxAlreadyStopped", err)
	}

	// Start
	if err := svc.Start(ctx, "cycle-1"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	started, _ := sandboxStore.GetByID(ctx, "cycle-1")
	if started.LifecycleStatus != "pending" {
		t.Fatalf("after start: LifecycleStatus = %q, want pending", started.LifecycleStatus)
	}
	if started.StoppedAt != nil {
		t.Fatalf("after start: StoppedAt should be nil")
	}

	// Try to start again → should fail (not stopped)
	if err := svc.Start(ctx, "cycle-1"); !errors.Is(err, ErrSandboxNotStopped) {
		t.Fatalf("Start(not-stopped) error = %v, want ErrSandboxNotStopped", err)
	}
}

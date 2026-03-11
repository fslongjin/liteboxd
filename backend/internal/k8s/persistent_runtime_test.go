package k8s

import (
	"context"
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func newTestClientWithFakeClientset(objects ...runtime.Object) *Client {
	return &Client{
		clientset: kubefake.NewSimpleClientset(objects...),
		sandboxNS: DefaultSandboxNamespace,
		controlNS: DefaultControlNamespace,
	}
}

func makeTestDeployment(sandboxID string, replicas int32) *appsv1.Deployment {
	deployName := fmt.Sprintf("sandbox-%s", sandboxID)
	labels := map[string]string{
		"app":          LabelApp,
		LabelSandboxID: sandboxID,
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName,
			Namespace: DefaultSandboxNamespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
		},
	}
}

func getDeploymentReplicas(t *testing.T, client *Client, sandboxID string) int32 {
	t.Helper()
	deployName := fmt.Sprintf("sandbox-%s", sandboxID)
	got, err := client.clientset.AppsV1().Deployments(DefaultSandboxNamespace).Get(
		context.Background(), deployName, metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("Get deployment %s error = %v", deployName, err)
	}
	return *got.Spec.Replicas
}

func TestStopPersistentSandbox(t *testing.T) {
	ctx := context.Background()
	client := newTestClientWithFakeClientset(makeTestDeployment("abc123", 1))

	if err := client.StopPersistentSandbox(ctx, "abc123"); err != nil {
		t.Fatalf("StopPersistentSandbox() error = %v", err)
	}
	if r := getDeploymentReplicas(t, client, "abc123"); r != 0 {
		t.Fatalf("Replicas = %d, want 0", r)
	}
}

func TestStopPersistentSandboxAlreadyZero(t *testing.T) {
	ctx := context.Background()
	client := newTestClientWithFakeClientset(makeTestDeployment("zero1", 0))

	if err := client.StopPersistentSandbox(ctx, "zero1"); err != nil {
		t.Fatalf("StopPersistentSandbox() error = %v", err)
	}
	if r := getDeploymentReplicas(t, client, "zero1"); r != 0 {
		t.Fatalf("Replicas = %d, want 0", r)
	}
}

func TestStopPersistentSandboxNotFound(t *testing.T) {
	ctx := context.Background()
	client := newTestClientWithFakeClientset()

	err := client.StopPersistentSandbox(ctx, "nonexistent")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected NotFound error, got %v", err)
	}
}

func TestStartPersistentSandbox(t *testing.T) {
	ctx := context.Background()
	client := newTestClientWithFakeClientset(makeTestDeployment("start1", 0))

	if err := client.StartPersistentSandbox(ctx, "start1"); err != nil {
		t.Fatalf("StartPersistentSandbox() error = %v", err)
	}
	if r := getDeploymentReplicas(t, client, "start1"); r != 1 {
		t.Fatalf("Replicas = %d, want 1", r)
	}
}

func TestStartPersistentSandboxNotFound(t *testing.T) {
	ctx := context.Background()
	client := newTestClientWithFakeClientset()

	err := client.StartPersistentSandbox(ctx, "nonexistent")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected NotFound error, got %v", err)
	}
}

func TestStopThenStartPersistentSandbox(t *testing.T) {
	ctx := context.Background()
	client := newTestClientWithFakeClientset(makeTestDeployment("cycle1", 1))

	if err := client.StopPersistentSandbox(ctx, "cycle1"); err != nil {
		t.Fatalf("StopPersistentSandbox() error = %v", err)
	}
	if r := getDeploymentReplicas(t, client, "cycle1"); r != 0 {
		t.Fatalf("Replicas after stop = %d, want 0", r)
	}

	if err := client.StartPersistentSandbox(ctx, "cycle1"); err != nil {
		t.Fatalf("StartPersistentSandbox() error = %v", err)
	}
	if r := getDeploymentReplicas(t, client, "cycle1"); r != 1 {
		t.Fatalf("Replicas after start = %d, want 1", r)
	}
}

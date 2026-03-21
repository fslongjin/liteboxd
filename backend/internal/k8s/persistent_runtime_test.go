package k8s

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func newTestClientWithFakeClientset(objects ...runtime.Object) *Client {
	return &Client{
		clientset:                   kubefake.NewSimpleClientset(objects...),
		sandboxNS:                   DefaultSandboxNamespace,
		controlNS:                   DefaultControlNamespace,
		persistentRootFSHelperImage: DefaultPersistentRootFSHelperImage,
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

func TestCreatePersistentSandboxUsesHelperSidecarAndControlVolume(t *testing.T) {
	ctx := context.Background()
	client := newTestClientWithFakeClientset()

	_, err := client.CreatePersistentSandbox(ctx, CreatePersistentSandboxOptions{
		CreatePodOptions: CreatePodOptions{
			ID:      "persist1",
			Image:   "busybox:1.36",
			Command: []string{"sh", "-c", "sleep 30"},
		},
		StorageClassName: "longhorn",
		VolumeSize:       "20Gi",
	})
	if err != nil {
		t.Fatalf("CreatePersistentSandbox() error = %v", err)
	}

	deploy, err := client.clientset.AppsV1().Deployments(DefaultSandboxNamespace).Get(ctx, "sandbox-persist1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get deployment error = %v", err)
	}

	if deploy.Spec.Template.Spec.ShareProcessNamespace == nil || !*deploy.Spec.Template.Spec.ShareProcessNamespace {
		t.Fatalf("ShareProcessNamespace not enabled")
	}
	if got := len(deploy.Spec.Template.Spec.InitContainers); got != 1 {
		t.Fatalf("InitContainers len = %d, want 1", got)
	}
	if got := len(deploy.Spec.Template.Spec.Containers); got != 2 {
		t.Fatalf("Containers len = %d, want 2", got)
	}

	main := deploy.Spec.Template.Spec.Containers[0]
	helper := deploy.Spec.Template.Spec.Containers[1]
	if main.Name != "main" {
		t.Fatalf("main container name = %q, want main", main.Name)
	}
	if helper.Name != rootfsOverlayHelperName {
		t.Fatalf("helper container name = %q, want %q", helper.Name, rootfsOverlayHelperName)
	}
	if helper.Image != DefaultPersistentRootFSHelperImage {
		t.Fatalf("helper image = %q, want %q", helper.Image, DefaultPersistentRootFSHelperImage)
	}
	if helper.SecurityContext == nil || helper.SecurityContext.Privileged == nil || !*helper.SecurityContext.Privileged {
		t.Fatalf("helper container must be privileged")
	}
	if main.SecurityContext == nil || main.SecurityContext.AllowPrivilegeEscalation == nil || *main.SecurityContext.AllowPrivilegeEscalation {
		t.Fatalf("main container must keep AllowPrivilegeEscalation=false")
	}
	if len(main.Command) < 3 || main.Command[0] != "sh" || main.Command[1] != "-ec" {
		t.Fatalf("main container command = %v, want sh -ec wrapper", main.Command)
	}
	if len(main.Args) != 3 || main.Args[0] != "sh" || main.Args[1] != "-c" || main.Args[2] != "sleep 30" {
		t.Fatalf("main container args = %v, want original command preserved", main.Args)
	}
	if !hasVolumeMount(main.VolumeMounts, rootfsOverlayVolumeName, rootfsOverlayMountTarget) {
		t.Fatalf("main container rootfs mount not found")
	}
	if !hasVolumeMount(main.VolumeMounts, rootfsOverlayControlVolumeName, rootfsOverlayControlDir) {
		t.Fatalf("main container control mount not found")
	}
	if hasVolumeMount(main.VolumeMounts, "workspace", "/workspace") {
		t.Fatalf("main container should not inherit workspace mount")
	}

	assertMountsHaveNoPropagation(t, main.VolumeMounts)
	assertMountsHaveNoPropagation(t, helper.VolumeMounts)
	assertMountsHaveNoPropagation(t, deploy.Spec.Template.Spec.InitContainers[0].VolumeMounts)

	if !hasVolume(deploy.Spec.Template.Spec.Volumes, rootfsOverlayControlVolumeName, func(v corev1.VolumeSource) bool {
		return v.EmptyDir != nil
	}) {
		t.Fatalf("control EmptyDir volume not found")
	}
	if !hasVolume(deploy.Spec.Template.Spec.Volumes, rootfsOverlayVolumeName, func(v corev1.VolumeSource) bool {
		return v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == "sandbox-data-persist1"
	}) {
		t.Fatalf("rootfs PVC volume not found")
	}
	if hasVolume(deploy.Spec.Template.Spec.Volumes, "workspace", func(v corev1.VolumeSource) bool {
		return true
	}) {
		t.Fatalf("workspace volume should not exist in persistent deployment")
	}
}

func TestCreatePersistentSandboxUsesConfiguredHelperImage(t *testing.T) {
	ctx := context.Background()
	client := newTestClientWithFakeClientset()
	client.persistentRootFSHelperImage = "registry.example.com/rootfs-helper:dev"

	_, err := client.CreatePersistentSandbox(ctx, CreatePersistentSandboxOptions{
		CreatePodOptions: CreatePodOptions{
			ID:      "persist2",
			Image:   "busybox:1.36",
			Command: []string{"sh", "-c", "sleep 30"},
		},
		VolumeSize: "1Gi",
	})
	if err != nil {
		t.Fatalf("CreatePersistentSandbox() error = %v", err)
	}

	deploy, err := client.clientset.AppsV1().Deployments(DefaultSandboxNamespace).Get(ctx, "sandbox-persist2", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get deployment error = %v", err)
	}
	if got := deploy.Spec.Template.Spec.InitContainers[0].Image; got != "registry.example.com/rootfs-helper:dev" {
		t.Fatalf("init image = %q, want configured helper image", got)
	}
	if got := deploy.Spec.Template.Spec.Containers[1].Image; got != "registry.example.com/rootfs-helper:dev" {
		t.Fatalf("helper image = %q, want configured helper image", got)
	}
}

func TestGeneratedRootfsScriptsParseWithSh(t *testing.T) {
	scripts := map[string]string{
		"prep":   buildRootfsOverlayPrepScript(),
		"main":   buildRootfsOverlayMainWrapperScript(),
		"helper": buildRootfsOverlayHelperScript(),
	}
	for name, script := range scripts {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command("sh", "-n")
			cmd.Stdin = strings.NewReader(script)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("sh -n failed: %v\n%s", err, out)
			}
		})
	}
}

func TestRootfsScriptsAvoidKubeletDollarEscaping(t *testing.T) {
	scripts := map[string]string{
		"main":   buildRootfsOverlayMainWrapperScript(),
		"helper": buildRootfsOverlayHelperScript(),
	}
	for name, script := range scripts {
		if strings.Contains(script, "$$") {
			t.Fatalf("%s script must not contain $$ because kubelet collapses it to a literal $ in command/args", name)
		}
	}
}

func TestRootfsHelperScriptTargetsMainContainerRoot(t *testing.T) {
	script := buildRootfsOverlayHelperScript()
	if !strings.Contains(script, `--root="/proc/$target_pid/root"`) {
		t.Fatalf("helper script must switch to target container root before mounting")
	}
	if !strings.Contains(script, `--wd=/`) {
		t.Fatalf("helper script must reset working directory inside target container root")
	}
}

func assertMountsHaveNoPropagation(t *testing.T, mounts []corev1.VolumeMount) {
	t.Helper()
	for _, mount := range mounts {
		if mount.MountPropagation != nil {
			t.Fatalf("mount propagation should be unset for mount %q at %q", mount.Name, mount.MountPath)
		}
	}
}

func hasVolume(volumes []corev1.Volume, name string, predicate func(corev1.VolumeSource) bool) bool {
	for _, volume := range volumes {
		if volume.Name == name && predicate(volume.VolumeSource) {
			return true
		}
	}
	return false
}

func hasVolumeMount(mounts []corev1.VolumeMount, name, path string) bool {
	for _, mount := range mounts {
		if mount.Name == name && mount.MountPath == path {
			return true
		}
	}
	return false
}

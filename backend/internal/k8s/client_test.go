package k8s

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreatePodInjectsLauncher(t *testing.T) {
	ctx := context.Background()
	client := NewClientForTest()

	_, err := client.CreatePod(ctx, CreatePodOptions{
		ID:      "ephemeral1",
		Image:   "busybox:1.36",
		Command: []string{"sh", "-c", "sleep 30"},
	})
	if err != nil {
		t.Fatalf("CreatePod() error = %v", err)
	}

	pod, err := client.clientset.CoreV1().Pods(DefaultSandboxNamespace).Get(ctx, "sandbox-ephemeral1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get pod error = %v", err)
	}
	if got := len(pod.Spec.InitContainers); got != 1 {
		t.Fatalf("InitContainers len = %d, want 1", got)
	}
	if pod.Spec.InitContainers[0].Name != sandboxLauncherInitName {
		t.Fatalf("launcher init name = %q, want %q", pod.Spec.InitContainers[0].Name, sandboxLauncherInitName)
	}
	main := pod.Spec.Containers[0]
	if len(main.Command) != 1 || main.Command[0] != sandboxLauncherBinaryPath {
		t.Fatalf("main command = %v, want launcher", main.Command)
	}
	wantArgs := []string{"--nofile", "16384", "--", "sh", "-c", "sleep 30"}
	if len(main.Args) != len(wantArgs) {
		t.Fatalf("main args len = %d, want %d (%v)", len(main.Args), len(wantArgs), main.Args)
	}
	for i, want := range wantArgs {
		if main.Args[i] != want {
			t.Fatalf("main args[%d] = %q, want %q; args=%v", i, main.Args[i], want, main.Args)
		}
	}
	if !hasVolumeMount(main.VolumeMounts, sandboxLauncherVolumeName, sandboxLauncherMountDir) {
		t.Fatalf("main container launcher mount not found")
	}
	found := false
	for _, volume := range pod.Spec.Volumes {
		if volume.Name == sandboxLauncherVolumeName && volume.EmptyDir != nil {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("sandbox launcher volume not found")
	}
}

func TestResolveSandboxStartCommand(t *testing.T) {
	original := imageEntrypointResolver
	t.Cleanup(func() { imageEntrypointResolver = original })

	imageEntrypointResolver = func(_ context.Context, image string) ([]string, []string, error) {
		if image != "example/image:latest" {
			t.Fatalf("image = %q, want example/image:latest", image)
		}
		return []string{"/entrypoint"}, []string{"serve", "--port", "8080"}, nil
	}

	cmd, args, err := resolveSandboxStartCommand(context.Background(), "example/image:latest", nil, nil)
	if err != nil {
		t.Fatalf("resolveSandboxStartCommand() error = %v", err)
	}
	if len(cmd) != 1 || cmd[0] != "/entrypoint" {
		t.Fatalf("command = %v, want [/entrypoint]", cmd)
	}
	if len(args) != 3 || args[0] != "serve" {
		t.Fatalf("args = %v, want image cmd", args)
	}
}

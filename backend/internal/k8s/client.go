package k8s

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	SandboxNamespace    = "liteboxd"
	LabelApp            = "liteboxd"
	LabelSandboxID      = "sandbox-id"
	AnnotationTTL       = "liteboxd/ttl"
	AnnotationCreatedAt = "liteboxd/created-at"
)

type Client struct {
	clientset *kubernetes.Clientset
	config    *rest.Config
}

func NewClient(kubeconfigPath string) (*Client, error) {
	var config *rest.Config
	var err error

	if kubeconfigPath != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &Client{
		clientset: clientset,
		config:    config,
	}, nil
}

func (c *Client) GetConfig() *rest.Config {
	return c.config
}

func (c *Client) EnsureNamespace(ctx context.Context) error {
	_, err := c.clientset.CoreV1().Namespaces().Get(ctx, SandboxNamespace, metav1.GetOptions{})
	if err == nil {
		return nil
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: SandboxNamespace,
		},
	}
	_, err = c.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	return err
}

type CreatePodOptions struct {
	ID             string
	Image          string
	CPU            string
	Memory         string
	TTL            int
	Env            map[string]string
	Annotations    map[string]string
	StartupScript  string     // Startup script to execute after pod is ready
	StartupFiles   []FileSpec // Files to upload before startup script
	ReadinessProbe *ProbeSpec // Readiness probe configuration
	Network        *NetworkSpec // Network configuration
}

// NetworkSpec defines the network configuration for a pod
type NetworkSpec struct {
	AllowInternetAccess bool     // Enable outbound internet access
	AllowedDomains      []string // Optional domain whitelist (future)
}

// FileSpec defines a file to be uploaded to the sandbox
type FileSpec struct {
	Source      string // Source URL (for future use)
	Destination string // Destination path in the sandbox
	Content     string // File content
}

// ProbeSpec defines a readiness probe
type ProbeSpec struct {
	Exec                []string
	InitialDelaySeconds int
	PeriodSeconds       int
	FailureThreshold    int
}

func (c *Client) CreatePod(ctx context.Context, opts CreatePodOptions) (*corev1.Pod, error) {
	podName := fmt.Sprintf("sandbox-%s", opts.ID)

	cpuLimit := opts.CPU
	if cpuLimit == "" {
		cpuLimit = "500m"
	}
	memLimit := opts.Memory
	if memLimit == "" {
		memLimit = "512Mi"
	}

	var envVars []corev1.EnvVar
	for k, v := range opts.Env {
		envVars = append(envVars, corev1.EnvVar{Name: k, Value: v})
	}

	var runAsUser int64 = 1000

	// Generate access token for sandbox network access
	accessToken := generateAccessToken()

	// Build annotations
	annotations := map[string]string{
		AnnotationTTL:         fmt.Sprintf("%d", opts.TTL),
		AnnotationCreatedAt:    time.Now().UTC().Format(time.RFC3339),
		AnnotationAccessToken:  accessToken,
	}
	// Merge custom annotations
	for k, v := range opts.Annotations {
		annotations[k] = v
	}

	// Build labels
	labels := map[string]string{
		"app":          LabelApp,
		LabelSandboxID: opts.ID,
	}

	// Add internet-access label if network config specifies it
	if opts.Network != nil && opts.Network.AllowInternetAccess {
		labels[LabelInternetAccess] = "true"
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        podName,
			Namespace:   SandboxNamespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Tolerations: []corev1.Toleration{
				{
					Key:      "node.kubernetes.io/disk-pressure",
					Operator: corev1.TolerationOpExists,
				},
				{
					Key:      "node.kubernetes.io/memory-pressure",
					Operator: corev1.TolerationOpExists,
				},
				{
					Key:      "node.kubernetes.io/pid-pressure",
					Operator: corev1.TolerationOpExists,
				},
			},
			SecurityContext: &corev1.PodSecurityContext{
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Containers: []corev1.Container{
				{
					Name:            "main",
					Image:           opts.Image,
					ImagePullPolicy: corev1.PullIfNotPresent,
					Command:         []string{"sleep", "infinity"},
					Env:             envVars,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse(cpuLimit),
							corev1.ResourceMemory: resource.MustParse(memLimit),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: boolPtr(false),
						RunAsNonRoot:             boolPtr(true),
						RunAsUser:                &runAsUser,
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "workspace",
							MountPath: "/workspace",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "workspace",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}

	return c.clientset.CoreV1().Pods(SandboxNamespace).Create(ctx, pod, metav1.CreateOptions{})
}

func (c *Client) GetPod(ctx context.Context, sandboxID string) (*corev1.Pod, error) {
	podName := fmt.Sprintf("sandbox-%s", sandboxID)
	return c.clientset.CoreV1().Pods(SandboxNamespace).Get(ctx, podName, metav1.GetOptions{})
}

func (c *Client) ListPods(ctx context.Context) (*corev1.PodList, error) {
	return c.clientset.CoreV1().Pods(SandboxNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", LabelApp),
	})
}

func (c *Client) DeletePod(ctx context.Context, sandboxID string) error {
	podName := fmt.Sprintf("sandbox-%s", sandboxID)
	return c.clientset.CoreV1().Pods(SandboxNamespace).Delete(ctx, podName, metav1.DeleteOptions{})
}

type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func (c *Client) Exec(ctx context.Context, sandboxID string, command []string) (*ExecResult, error) {
	podName := fmt.Sprintf("sandbox-%s", sandboxID)

	req := c.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(SandboxNamespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "main",
			Command:   command,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.config, "POST", req.URL())
	if err != nil {
		return nil, fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	result := &ExecResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		if exitErr, ok := err.(interface{ ExitStatus() int }); ok {
			result.ExitCode = exitErr.ExitStatus()
		} else {
			result.ExitCode = 1
			result.Stderr = result.Stderr + "\n" + err.Error()
		}
	}

	return result, nil
}

func (c *Client) UploadFile(ctx context.Context, sandboxID string, destPath string, content []byte) error {
	podName := fmt.Sprintf("sandbox-%s", sandboxID)
	dir := filepath.Dir(destPath)
	filename := filepath.Base(destPath)

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	hdr := &tar.Header{
		Name: filename,
		Mode: 0644,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("failed to write tar header: %w", err)
	}
	if _, err := tw.Write(content); err != nil {
		return fmt.Errorf("failed to write tar content: %w", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %w", err)
	}

	req := c.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(SandboxNamespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "main",
			Command:   []string{"tar", "-xf", "-", "-C", dir},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  &tarBuf,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return fmt.Errorf("failed to upload file: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

func (c *Client) DownloadFile(ctx context.Context, sandboxID string, srcPath string) ([]byte, error) {
	podName := fmt.Sprintf("sandbox-%s", sandboxID)
	dir := filepath.Dir(srcPath)
	filename := filepath.Base(srcPath)

	req := c.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(SandboxNamespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "main",
			Command:   []string{"tar", "-cf", "-", "-C", dir, filename},
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.config, "POST", req.URL())
	if err != nil {
		return nil, fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w, stderr: %s", err, stderr.String())
	}

	tr := tar.NewReader(&stdout)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar: %w", err)
		}
		if strings.TrimPrefix(hdr.Name, "./") == filename {
			content, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("failed to read file content: %w", err)
			}
			return content, nil
		}
	}

	return nil, fmt.Errorf("file not found in tar archive")
}

func (c *Client) GetLogs(ctx context.Context, sandboxID string, tailLines int64) (string, error) {
	podName := fmt.Sprintf("sandbox-%s", sandboxID)

	opts := &corev1.PodLogOptions{
		Container: "main",
	}
	if tailLines > 0 {
		opts.TailLines = &tailLines
	}

	req := c.clientset.CoreV1().Pods(SandboxNamespace).GetLogs(podName, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}
	defer stream.Close()

	logs, err := io.ReadAll(stream)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %w", err)
	}

	return string(logs), nil
}

func (c *Client) GetPodEvents(ctx context.Context, sandboxID string) ([]string, error) {
	podName := fmt.Sprintf("sandbox-%s", sandboxID)

	events, err := c.clientset.CoreV1().Events(SandboxNamespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", podName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	var result []string
	for _, event := range events.Items {
		result = append(result, fmt.Sprintf("[%s] %s: %s", event.Type, event.Reason, event.Message))
	}
	return result, nil
}

func boolPtr(b bool) *bool {
	return &b
}

// Job operations for image prepull

const (
	LabelPrepull      = "liteboxd-prepull"
	LabelPrepullImage = "prepull-image-hash"
)

// CreatePrepullDaemonSetOptions defines options for creating a prepull Job
type CreatePrepullDaemonSetOptions struct {
	ID        string // Unique ID for the prepull task
	Image     string // Image to prepull
	ImageHash string // Hash of image name for labeling
}

// CreatePrepullDaemonSet creates a Job to prepull an image on ready nodes
func (c *Client) CreatePrepullDaemonSet(ctx context.Context, opts CreatePrepullDaemonSetOptions) error {
	jobName := fmt.Sprintf("prepull-%s", opts.ID)
	parallelism := int32(1) // Run one pod per ready node at a time

	// Get number of ready nodes to set parallelism
	nodes, err := c.GetNodeCount(ctx)
	if err == nil && nodes > 0 {
		// Count ready nodes
		readyNodes := 0
		nodeList, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, node := range nodeList.Items {
				if isNodeReady(&node) {
					readyNodes++
				}
			}
		}
		if readyNodes > 0 {
			parallelism = int32(readyNodes)
		}
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: SandboxNamespace,
			Labels: map[string]string{
				"app":             LabelPrepull,
				LabelPrepullImage: opts.ImageHash,
				"job-name":        jobName,
			},
			Annotations: map[string]string{
				"liteboxd.io/prepull-id":    opts.ID,
				"liteboxd.io/prepull-image": opts.Image,
			},
		},
		Spec: batchv1.JobSpec{
			Parallelism:             &parallelism,
			Completions:             &parallelism,
			BackoffLimit:            int32Ptr(0),   // Don't retry on failure
			TTLSecondsAfterFinished: int32Ptr(300), // Clean up 5 min after completion
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":             LabelPrepull,
						LabelPrepullImage: opts.ImageHash,
						"job-name":        jobName,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:            "prepull",
							Image:           opts.Image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"sh", "-c", "echo 'Image pulled successfully' && sleep 5"},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("16Mi"),
								},
							},
						},
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "node.kubernetes.io/not-ready",
							Operator: corev1.TolerationOpExists,
						},
						{
							Key:      "node.kubernetes.io/unreachable",
							Operator: corev1.TolerationOpExists,
						},
						{
							Key:      "node.kubernetes.io/memory-pressure",
							Operator: corev1.TolerationOpExists,
						},
					},
				},
			},
		},
	}

	_, err = c.clientset.BatchV1().Jobs(SandboxNamespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create prepull job: %w", err)
	}

	return nil
}

// GetPrepullDaemonSet retrieves a prepull Job by ID
func (c *Client) GetPrepullDaemonSet(ctx context.Context, id string) (*batchv1.Job, error) {
	jobName := fmt.Sprintf("prepull-%s", id)
	return c.clientset.BatchV1().Jobs(SandboxNamespace).Get(ctx, jobName, metav1.GetOptions{})
}

// ListPrepullDaemonSets lists all prepull Jobs
func (c *Client) ListPrepullDaemonSets(ctx context.Context) (*batchv1.JobList, error) {
	return c.clientset.BatchV1().Jobs(SandboxNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", LabelPrepull),
	})
}

// DeletePrepullDaemonSet deletes a prepull Job by ID
func (c *Client) DeletePrepullDaemonSet(ctx context.Context, id string) error {
	jobName := fmt.Sprintf("prepull-%s", id)
	return c.clientset.BatchV1().Jobs(SandboxNamespace).Delete(ctx, jobName, metav1.DeleteOptions{})
}

// PrepullStatus represents the status of a prepull operation
type PrepullStatus struct {
	DesiredNodes int
	ReadyNodes   int
	IsComplete   bool
}

// GetPrepullStatus returns the status of a prepull Job
func (c *Client) GetPrepullStatus(ctx context.Context, id string) (*PrepullStatus, error) {
	job, err := c.GetPrepullDaemonSet(ctx, id)
	if err != nil {
		return nil, err
	}

	status := &PrepullStatus{
		DesiredNodes: int(*job.Spec.Parallelism),
		ReadyNodes:   int(job.Status.Succeeded),
	}

	// Job is complete when Succeeded equals Completions
	status.IsComplete = job.Status.Succeeded > 0 && job.Status.Succeeded >= *job.Spec.Completions

	return status, nil
}

// IsImagePrepulled checks if an image has been prepulled on the cluster
func (c *Client) IsImagePrepulled(ctx context.Context, image string) (bool, error) {
	// List all prepull Jobs
	jobList, err := c.ListPrepullDaemonSets(ctx)
	if err != nil {
		return false, err
	}

	// Check if there's a completed prepull for this image
	for _, job := range jobList.Items {
		// Check annotation for the image
		img := job.Annotations["liteboxd.io/prepull-image"]
		if img == image { // Found matching prepull
			// Job is complete when Succeeded equals Completions
			if job.Status.Succeeded > 0 && job.Status.Succeeded >= *job.Spec.Completions {
				return true, nil
			}
		}
	}
	return false, nil
}

// GetNodeCount returns the total number of nodes in the cluster
func (c *Client) GetNodeCount(ctx context.Context) (int, error) {
	nodes, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, fmt.Errorf("failed to list nodes: %w", err)
	}
	return len(nodes.Items), nil
}

func isNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func int32Ptr(i int32) *int32 {
	return &i
}

func truePtr(b bool) *bool {
	return &b
}

// WaitForReady waits for a pod to be ready with optional custom readiness probe
func (c *Client) WaitForReady(ctx context.Context, sandboxID string, probe *ProbeSpec) error {
	podName := fmt.Sprintf("sandbox-%s", sandboxID)

	// Default polling settings
	pollInterval := 2 * time.Second
	timeout := 5 * time.Minute
	if probe != nil {
		if probe.PeriodSeconds > 0 {
			pollInterval = time.Duration(probe.PeriodSeconds) * time.Second
		}
		if probe.InitialDelaySeconds > 0 {
			// Add initial delay
			select {
			case <-time.After(time.Duration(probe.InitialDelaySeconds) * time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for pod to be ready")
		case <-ticker.C:
			pod, err := c.clientset.CoreV1().Pods(SandboxNamespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("failed to get pod: %w", err)
			}

			// Check if pod has failed
			if pod.Status.Phase == corev1.PodFailed {
				return fmt.Errorf("pod failed")
			}

			// If custom probe is specified, use it
			if probe != nil && len(probe.Exec) > 0 {
				// Use a background context for exec to avoid timeout from parent context
				result, err := c.Exec(context.Background(), sandboxID, probe.Exec)
				if err != nil {
					continue // Try again
				}
				if result.ExitCode == 0 {
					return nil // Ready
				}
				// Exit code non-zero, continue polling
			} else {
				// Default: wait for pod to be running and ready
				if pod.Status.Phase == corev1.PodRunning {
					for _, cond := range pod.Status.Conditions {
						if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
							return nil // Ready
						}
					}
				}
			}
		}
	}
}

// UploadFiles uploads multiple files to a sandbox
func (c *Client) UploadFiles(ctx context.Context, sandboxID string, files []FileSpec) error {
	for _, file := range files {
		content := []byte(file.Content)
		if file.Source != "" {
			// For future: fetch content from URL
			return fmt.Errorf("source URL not yet supported")
		}
		if err := c.UploadFile(ctx, sandboxID, file.Destination, content); err != nil {
			return fmt.Errorf("failed to upload file %s: %w", file.Destination, err)
		}
	}
	return nil
}

// generateAccessToken generates a random access token for sandbox network access
func generateAccessToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback to time-based token if crypto rand fails
		return fmt.Sprintf("%d-%s", time.Now().UnixNano(), randomString(16))
	}
	return hex.EncodeToString(b)
}

// randomString generates a random hex string
func randomString(length int) string {
	b := make([]byte, length)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:length]
}

// GetPodIP returns the IP address of a sandbox pod
func (c *Client) GetPodIP(ctx context.Context, sandboxID string) (string, error) {
	podName := fmt.Sprintf("sandbox-%s", sandboxID)
	pod, err := c.clientset.CoreV1().Pods(SandboxNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get pod: %w", err)
	}

	if len(pod.Status.PodIP) == 0 {
		return "", fmt.Errorf("pod IP not available")
	}

	return pod.Status.PodIP, nil
}

// GetPodAccessToken retrieves the access token from a pod's annotations
func (c *Client) GetPodAccessToken(ctx context.Context, sandboxID string) (string, error) {
	podName := fmt.Sprintf("sandbox-%s", sandboxID)
	pod, err := c.clientset.CoreV1().Pods(SandboxNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get pod: %w", err)
	}

	token, ok := pod.Annotations[AnnotationAccessToken]
	if !ok {
		return "", fmt.Errorf("access token not found")
	}

	return token, nil
}

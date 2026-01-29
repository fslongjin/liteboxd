package service

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/k8s"
	"github.com/fslongjin/liteboxd/backend/internal/model"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
)

type SandboxService struct {
	k8sClient   *k8s.Client
	templateSvc *TemplateService
}

func NewSandboxService(k8sClient *k8s.Client) *SandboxService {
	return &SandboxService{
		k8sClient: k8sClient,
	}
}

// SetTemplateService sets the template service for template-based sandbox creation
func (s *SandboxService) SetTemplateService(templateSvc *TemplateService) {
	s.templateSvc = templateSvc
}

func (s *SandboxService) Create(ctx context.Context, req *model.CreateSandboxRequest) (*model.Sandbox, error) {
	// All sandboxes must be created from a template
	if req.Template == "" {
		return nil, fmt.Errorf("template is required")
	}

	if s.templateSvc == nil {
		return nil, fmt.Errorf("template service not configured")
	}

	// Get template spec
	spec, err := s.templateSvc.GetSpecForSandbox(ctx, req.Template, req.TemplateVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get template: %w", err)
	}

	// Get actual template version
	template, err := s.templateSvc.Get(ctx, req.Template)
	if err != nil {
		return nil, fmt.Errorf("failed to get template info: %w", err)
	}

	templateVersion := req.TemplateVersion
	if templateVersion <= 0 {
		templateVersion = template.LatestVersion
	}

	// Apply template spec as base configuration
	image := spec.Image
	cpu := spec.Resources.CPU
	memory := spec.Resources.Memory
	ttl := spec.TTL
	env := spec.Env

	startupScript := spec.StartupScript
	startupFiles := spec.Files
	readinessProbe := spec.ReadinessProbe
	startupTimeout := spec.StartupTimeout
	if startupTimeout <= 0 {
		startupTimeout = 300 // default 5 minutes
	}

	// Network config from template (cannot be overridden)
	networkConfig := spec.Network

	// Apply overrides (network configuration is NOT allowed to be overridden)
	if req.Overrides != nil {
		if req.Overrides.CPU != "" {
			cpu = req.Overrides.CPU
		}
		if req.Overrides.Memory != "" {
			memory = req.Overrides.Memory
		}
		if req.Overrides.TTL > 0 {
			ttl = req.Overrides.TTL
		}
		if req.Overrides.Env != nil {
			if env == nil {
				env = make(map[string]string)
			}
			for k, v := range req.Overrides.Env {
				env[k] = v
			}
		}
	}

	// Validate required fields
	if image == "" {
		return nil, fmt.Errorf("template spec is invalid: image is required")
	}

	id := generateID()

	if ttl <= 0 {
		ttl = 3600
	}

	// Convert model.FileSpec to k8s.FileSpec
	var files []k8s.FileSpec
	for _, f := range startupFiles {
		files = append(files, k8s.FileSpec{
			Source:      f.Source,
			Destination: f.Destination,
			Content:     f.Content,
		})
	}

	// Convert model.ProbeSpec to k8s.ProbeSpec
	var probe *k8s.ProbeSpec
	if readinessProbe != nil {
		probe = &k8s.ProbeSpec{
			Exec:                readinessProbe.Exec.Command,
			InitialDelaySeconds: readinessProbe.InitialDelaySeconds,
			PeriodSeconds:       readinessProbe.PeriodSeconds,
			FailureThreshold:    readinessProbe.FailureThreshold,
		}
	}

	// Build annotations
	annotations := map[string]string{
		"liteboxd.io/template":         req.Template,
		"liteboxd.io/template-version": strconv.Itoa(templateVersion),
	}

	// Convert model.NetworkSpec to k8s.NetworkSpec
	var k8sNetwork *k8s.NetworkSpec
	if networkConfig != nil {
		k8sNetwork = &k8s.NetworkSpec{
			AllowInternetAccess: networkConfig.AllowInternetAccess,
			AllowedDomains:      networkConfig.AllowedDomains,
		}
	}

	opts := k8s.CreatePodOptions{
		ID:             id,
		Image:          image,
		CPU:            cpu,
		Memory:         memory,
		TTL:            ttl,
		Env:            env,
		StartupScript:  startupScript,
		StartupFiles:   files,
		ReadinessProbe: probe,
		Annotations:    annotations,
		Network:        k8sNetwork,
	}

	pod, err := s.k8sClient.CreatePod(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create pod: %w", err)
	}

	// Get access token from pod annotations
	accessToken := pod.Annotations[k8s.AnnotationAccessToken]

	// Generate access URL
	gatewayURL := os.Getenv("GATEWAY_URL")
	if gatewayURL == "" {
		gatewayURL = "http://localhost:8080" // Default for development
	}
	accessURL := fmt.Sprintf("%s/api/v1/sandbox/%s", gatewayURL, id)

	sandbox := s.podToSandbox(pod)
	sandbox.Template = req.Template
	sandbox.TemplateVersion = templateVersion
	sandbox.AccessToken = accessToken
	sandbox.AccessURL = accessURL

	// Run post-creation tasks asynchronously (wait for ready, upload files, exec startup script)
	go func() {
		s.runPostCreationTasks(context.Background(), id, probe, files, startupScript, startupTimeout)
	}()

	return sandbox, nil
}

// runPostCreationTasks runs tasks after pod creation in the background
func (s *SandboxService) runPostCreationTasks(ctx context.Context, id string, probe *k8s.ProbeSpec, files []k8s.FileSpec, startupScript string, startupTimeout int) {
	// Wait for pod to be ready (with custom probe if specified)
	readyCtx, cancel := context.WithTimeout(ctx, time.Duration(startupTimeout)*time.Second)
	defer cancel()
	if err := s.k8sClient.WaitForReady(readyCtx, id, probe); err != nil {
		// Pod created but not ready - log the error
		fmt.Printf("Post-creation: pod %s not ready: %v\n", id, err)
		return
	}

	// Upload files if specified
	if len(files) > 0 {
		if err := s.k8sClient.UploadFiles(ctx, id, files); err != nil {
			fmt.Printf("Post-creation: failed to upload files to pod %s: %v\n", id, err)
		}
	}

	// Execute startup script if specified
	if startupScript != "" {
		execCtx, execCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer execCancel()
		if _, err := s.k8sClient.Exec(execCtx, id, []string{"sh", "-c", startupScript}); err != nil {
			fmt.Printf("Post-creation: startup script failed for pod %s: %v\n", id, err)
		}
	}

	fmt.Printf("Post-creation tasks completed for pod %s\n", id)
}

func (s *SandboxService) Get(ctx context.Context, id string) (*model.Sandbox, error) {
	pod, err := s.k8sClient.GetPod(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod: %w", err)
	}

	sandbox := s.podToSandbox(pod)

	// Add access URL to get response
	gatewayURL := os.Getenv("GATEWAY_URL")
	if gatewayURL == "" {
		gatewayURL = "http://localhost:8080"
	}
	sandbox.AccessURL = fmt.Sprintf("%s/api/v1/sandbox/%s", gatewayURL, id)

	return sandbox, nil
}

func (s *SandboxService) List(ctx context.Context) (*model.SandboxListResponse, error) {
	pods, err := s.k8sClient.ListPods(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	items := make([]model.Sandbox, 0, len(pods.Items))
	for _, pod := range pods.Items {
		items = append(items, *s.podToSandbox(&pod))
	}

	return &model.SandboxListResponse{Items: items}, nil
}

func (s *SandboxService) Delete(ctx context.Context, id string) error {
	return s.k8sClient.DeletePod(ctx, id)
}

func (s *SandboxService) Exec(ctx context.Context, id string, req *model.ExecRequest) (*model.ExecResponse, error) {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 30
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	result, err := s.k8sClient.Exec(execCtx, id, req.Command)
	if err != nil {
		return nil, fmt.Errorf("failed to exec: %w", err)
	}

	return &model.ExecResponse{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
	}, nil
}

func (s *SandboxService) UploadFile(ctx context.Context, id, path string, content []byte) error {
	return s.k8sClient.UploadFile(ctx, id, path, content)
}

func (s *SandboxService) DownloadFile(ctx context.Context, id, path string) ([]byte, error) {
	return s.k8sClient.DownloadFile(ctx, id, path)
}

func (s *SandboxService) GetLogs(ctx context.Context, id string, tailLines int64) (*model.LogsResponse, error) {
	logs, err := s.k8sClient.GetLogs(ctx, id, tailLines)
	if err != nil {
		logs = ""
	}

	events, err := s.k8sClient.GetPodEvents(ctx, id)
	if err != nil {
		events = nil
	}

	return &model.LogsResponse{
		Logs:   logs,
		Events: events,
	}, nil
}

func (s *SandboxService) StartTTLCleaner(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			s.cleanExpiredSandboxes()
		}
	}()
}

func (s *SandboxService) cleanExpiredSandboxes() {
	ctx := context.Background()
	pods, err := s.k8sClient.ListPods(ctx)
	if err != nil {
		fmt.Printf("TTL cleaner: failed to list pods: %v\n", err)
		return
	}

	for _, pod := range pods.Items {
		if s.isExpired(&pod) {
			sandboxID := pod.Labels[k8s.LabelSandboxID]
			fmt.Printf("TTL cleaner: deleting expired sandbox %s\n", sandboxID)
			if err := s.k8sClient.DeletePod(ctx, sandboxID); err != nil {
				fmt.Printf("TTL cleaner: failed to delete pod %s: %v\n", sandboxID, err)
			}
		}
	}
}

func (s *SandboxService) isExpired(pod *corev1.Pod) bool {
	createdAtStr := pod.Annotations[k8s.AnnotationCreatedAt]
	ttlStr := pod.Annotations[k8s.AnnotationTTL]

	createdAt, err := time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return false
	}

	ttl, err := strconv.Atoi(ttlStr)
	if err != nil {
		return false
	}

	expiresAt := createdAt.Add(time.Duration(ttl) * time.Second)
	return time.Now().After(expiresAt)
}

func (s *SandboxService) podToSandbox(pod *corev1.Pod) *model.Sandbox {
	sandboxID := pod.Labels[k8s.LabelSandboxID]
	createdAtStr := pod.Annotations[k8s.AnnotationCreatedAt]
	ttlStr := pod.Annotations[k8s.AnnotationTTL]

	createdAt, _ := time.Parse(time.RFC3339, createdAtStr)
	ttl, _ := strconv.Atoi(ttlStr)
	expiresAt := createdAt.Add(time.Duration(ttl) * time.Second)

	var image string
	if len(pod.Spec.Containers) > 0 {
		image = pod.Spec.Containers[0].Image
	}

	var cpu, memory string
	if len(pod.Spec.Containers) > 0 {
		limits := pod.Spec.Containers[0].Resources.Limits
		if cpuQty, ok := limits[corev1.ResourceCPU]; ok {
			cpu = cpuQty.String()
		}
		if memQty, ok := limits[corev1.ResourceMemory]; ok {
			memory = memQty.String()
		}
	}

	// Get access token from annotations
	accessToken := pod.Annotations[k8s.AnnotationAccessToken]

	return &model.Sandbox{
		ID:          sandboxID,
		Image:       image,
		CPU:         cpu,
		Memory:      memory,
		TTL:         ttl,
		Status:      convertPodStatus(pod),
		CreatedAt:   createdAt,
		ExpiresAt:   expiresAt,
		AccessToken: accessToken,
	}
}

func convertPodStatus(pod *corev1.Pod) model.SandboxStatus {
	// Check if pod is being terminated (has DeletionTimestamp)
	if pod.DeletionTimestamp != nil {
		return model.SandboxStatusTerminating
	}

	switch pod.Status.Phase {
	case corev1.PodPending:
		return model.SandboxStatusPending
	case corev1.PodRunning:
		return model.SandboxStatusRunning
	case corev1.PodSucceeded:
		return model.SandboxStatusSucceeded
	case corev1.PodFailed:
		return model.SandboxStatusFailed
	default:
		return model.SandboxStatusUnknown
	}
}

func generateID() string {
	id := uuid.New().String()
	return id[:8]
}

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/k8s"
	"github.com/fslongjin/liteboxd/backend/internal/logx"
	"github.com/fslongjin/liteboxd/backend/internal/model"
	"github.com/fslongjin/liteboxd/backend/internal/security"
	"github.com/fslongjin/liteboxd/backend/internal/store"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type SandboxService struct {
	k8sClient    *k8s.Client
	templateSvc  *TemplateService
	sandboxStore *store.SandboxStore
	tokenCipher  *security.TokenCipher
}

func NewSandboxService(k8sClient *k8s.Client, sandboxStore *store.SandboxStore, tokenCipher *security.TokenCipher) *SandboxService {
	return &SandboxService{
		k8sClient:    k8sClient,
		sandboxStore: sandboxStore,
		tokenCipher:  tokenCipher,
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

	accessToken, err := security.GenerateToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}
	ciphertext, nonce, keyID, err := s.tokenCipher.Encrypt(accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt access token: %w", err)
	}
	tokenHash := security.HashToken(accessToken)

	gatewayURL := os.Getenv("GATEWAY_URL")
	if gatewayURL == "" {
		gatewayURL = "http://localhost:8080" // Default for development
	}
	accessURL := fmt.Sprintf("%s/api/v1/sandbox/%s", gatewayURL, id)

	now := time.Now().UTC()
	envJSONBytes, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal env: %w", err)
	}
	record := &store.SandboxRecord{
		ID:                    id,
		TemplateName:          req.Template,
		TemplateVersion:       templateVersion,
		Image:                 image,
		CPU:                   cpu,
		Memory:                memory,
		TTL:                   ttl,
		EnvJSON:               string(envJSONBytes),
		DesiredState:          store.DesiredStateActive,
		LifecycleStatus:       "creating",
		StatusReason:          "",
		ClusterNamespace:      s.k8sClient.SandboxNamespace(),
		PodName:               fmt.Sprintf("sandbox-%s", id),
		AccessTokenCiphertext: ciphertext,
		AccessTokenNonce:      nonce,
		AccessTokenKeyID:      keyID,
		AccessTokenSHA256:     tokenHash,
		AccessURL:             accessURL,
		CreatedAt:             now,
		ExpiresAt:             now.Add(time.Duration(ttl) * time.Second),
		UpdatedAt:             now,
	}
	if err := s.sandboxStore.Create(ctx, record); err != nil {
		return nil, err
	}
	_ = s.sandboxStore.AppendStatusHistory(ctx, id, "api", "", "creating", "create requested", nil, now)

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
		Command:        spec.Command,
		Args:           spec.Args,
		CPU:            cpu,
		Memory:         memory,
		TTL:            ttl,
		Env:            env,
		Annotations:    annotations,
		StartupScript:  startupScript,
		StartupFiles:   files,
		ReadinessProbe: probe,
		Network:        k8sNetwork,
		AccessToken:    accessToken,
	}

	pod, err := s.k8sClient.CreatePod(ctx, opts)
	if err != nil {
		_ = s.sandboxStore.UpdateStatus(ctx, id, string(model.SandboxStatusFailed), err.Error(), time.Now().UTC())
		_ = s.sandboxStore.AppendStatusHistory(ctx, id, "api", "creating", string(model.SandboxStatusFailed), err.Error(), nil, time.Now().UTC())
		return nil, fmt.Errorf("failed to create pod: %w", err)
	}

	lifecycleStatus := string(convertPodStatus(pod))
	if err := s.sandboxStore.UpdateObservedState(
		ctx,
		id,
		string(pod.UID),
		string(pod.Status.Phase),
		pod.Status.PodIP,
		lifecycleStatus,
		"",
		time.Now().UTC(),
		time.Now().UTC(),
	); err != nil {
		return nil, err
	}
	_ = s.sandboxStore.AppendStatusHistory(ctx, id, "api", "creating", lifecycleStatus, "pod created", nil, time.Now().UTC())

	// Only apply domain allowlist policy if internet access is enabled
	if networkConfig != nil && networkConfig.AllowInternetAccess && len(networkConfig.AllowedDomains) > 0 {
		netPolicyMgr := k8s.NewNetworkPolicyManager(s.k8sClient)
		if err := netPolicyMgr.ApplyDomainAllowlistPolicy(ctx, id, networkConfig.AllowedDomains); err != nil {
			_ = s.k8sClient.DeletePod(ctx, id)
			_ = s.sandboxStore.UpdateStatus(ctx, id, string(model.SandboxStatusFailed), err.Error(), time.Now().UTC())
			return nil, fmt.Errorf("failed to apply domain allowlist policy: %w", err)
		}
	}

	sandbox, err := s.recordToSandbox(record)
	if err != nil {
		return nil, err
	}
	// For create response, return original plaintext token to avoid extra decrypt operation.
	sandbox.AccessToken = accessToken

	// Run post-creation tasks asynchronously (wait for ready, upload files, exec startup script)
	bgCtx := logx.WithRequestID(context.Background(), logx.RequestIDFromContext(ctx))
	go func() {
		s.runPostCreationTasks(bgCtx, id, probe, files, startupScript, startupTimeout)
	}()

	return sandbox, nil
}

// runPostCreationTasks runs tasks after pod creation in the background.
// Order: startup script first (so services like nginx can listen), then wait for ready (probe), then upload files.
func (s *SandboxService) runPostCreationTasks(ctx context.Context, id string, probe *k8s.ProbeSpec, files []k8s.FileSpec, startupScript string, startupTimeout int) {
	logger := logx.LoggerWithRequestID(ctx).With("component", "sandbox_service", "sandbox_id", id)

	// Execute startup script first if specified (e.g. start nginx), so readiness probe can succeed
	if startupScript != "" {
		execCtx, execCancel := context.WithTimeout(logx.WithRequestID(context.Background(), logx.RequestIDFromContext(ctx)), 60*time.Second)
		defer execCancel()
		if _, err := s.k8sClient.Exec(execCtx, id, []string{"sh", "-c", startupScript}); err != nil {
			logger.Warn("post-creation startup script failed", "error", err)
		}
	}

	// Wait for pod to be ready (with custom probe if specified)
	readyCtx, cancel := context.WithTimeout(ctx, time.Duration(startupTimeout)*time.Second)
	defer cancel()
	if err := s.k8sClient.WaitForReady(readyCtx, id, probe); err != nil {
		// Pod created but not ready - log the error and update metadata state
		logger.Warn("post-creation pod not ready", "error", err)
		_ = s.sandboxStore.UpdateStatus(context.Background(), id, string(model.SandboxStatusFailed), "startup/readiness failed", time.Now().UTC())
		_ = s.sandboxStore.AppendStatusHistory(context.Background(), id, "system", "pending", string(model.SandboxStatusFailed), "startup/readiness failed", nil, time.Now().UTC())
		return
	}

	_ = s.sandboxStore.UpdateStatus(context.Background(), id, string(model.SandboxStatusRunning), "", time.Now().UTC())
	_ = s.sandboxStore.AppendStatusHistory(context.Background(), id, "system", "pending", string(model.SandboxStatusRunning), "sandbox is ready", nil, time.Now().UTC())

	// Upload files if specified
	if len(files) > 0 {
		if err := s.k8sClient.UploadFiles(ctx, id, files); err != nil {
			logger.Warn("post-creation file upload failed", "error", err)
		}
	}

	logger.Info("post-creation tasks completed")
}

func (s *SandboxService) Get(ctx context.Context, id string) (*model.Sandbox, error) {
	record, err := s.sandboxStore.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if record == nil || record.LifecycleStatus == "deleted" {
		return nil, fmt.Errorf("sandbox not found")
	}
	return s.recordToSandbox(record)
}

func (s *SandboxService) List(ctx context.Context) (*model.SandboxListResponse, error) {
	records, err := s.sandboxStore.ListActive(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]model.Sandbox, 0, len(records))
	for i := range records {
		sandbox, err := s.recordToSandbox(&records[i])
		if err != nil {
			return nil, err
		}
		items = append(items, *sandbox)
	}

	return &model.SandboxListResponse{Items: items}, nil
}

func (s *SandboxService) Delete(ctx context.Context, id string) error {
	record, err := s.sandboxStore.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if record == nil {
		return fmt.Errorf("sandbox not found")
	}

	fromStatus := record.LifecycleStatus
	now := time.Now().UTC()
	if err := s.sandboxStore.SetDesiredDeleted(ctx, id, now); err != nil {
		return err
	}
	_ = s.sandboxStore.AppendStatusHistory(ctx, id, "api", fromStatus, string(model.SandboxStatusTerminating), "delete requested", nil, now)

	netPolicyMgr := k8s.NewNetworkPolicyManager(s.k8sClient)
	if err := netPolicyMgr.DeleteDomainAllowlistPolicy(ctx, id); err != nil {
		logx.LoggerWithRequestID(ctx).With("component", "sandbox_service", "sandbox_id", id).
			Warn("failed to delete domain allowlist policy", "error", err)
	}

	if err := s.k8sClient.DeletePod(ctx, id); err != nil && !apierrors.IsNotFound(err) {
		_ = s.sandboxStore.UpdateStatus(ctx, id, string(model.SandboxStatusFailed), err.Error(), time.Now().UTC())
		return err
	}

	if err := s.sandboxStore.MarkDeleted(ctx, id, "deleted by request", time.Now().UTC()); err != nil {
		return err
	}
	_ = s.sandboxStore.AppendStatusHistory(ctx, id, "api", string(model.SandboxStatusTerminating), "deleted", "delete completed", nil, time.Now().UTC())
	return nil
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

// ExecInteractive bridges a WebSocket connection to an interactive K8s exec session.
func (s *SandboxService) ExecInteractive(ctx context.Context, ws *websocket.Conn, id string, command []string, tty bool, rows, cols int) {
	// Create terminal size queue
	sizeQueue := k8s.NewSizeQueue()
	defer sizeQueue.Close()

	// Push initial size
	sizeQueue.Push(uint16(cols), uint16(rows))

	// Create pipe for stdin
	stdinReader, stdinWriter := io.Pipe()
	defer stdinWriter.Close()

	// Create a writer that sends output to WebSocket
	wsWriter := &wsOutputWriter{ws: ws}

	// Read WebSocket messages in a goroutine (stdin + resize)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		defer stdinWriter.Close()
		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				cancel()
				return
			}

			var msg model.WSMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				continue
			}

			switch msg.Type {
			case "input":
				if _, err := stdinWriter.Write([]byte(msg.Data)); err != nil {
					cancel()
					return
				}
			case "resize":
				sizeQueue.Push(uint16(msg.Cols), uint16(msg.Rows))
			}
		}
	}()

	// Run interactive exec (blocks until process exits)
	var stdinArg io.Reader = stdinReader
	err := s.k8sClient.ExecInteractive(ctx, id, k8s.ExecInteractiveOptions{
		Command:           command,
		TTY:               tty,
		Stdin:             stdinArg,
		Stdout:            wsWriter,
		Stderr:            nil, // merged into stdout when TTY=true
		TerminalSizeQueue: sizeQueue,
	})

	// Send exit message
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(interface{ ExitStatus() int }); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			exitCode = 1
		}
	}

	exitMsg, _ := json.Marshal(model.WSMessage{Type: "exit", ExitCode: exitCode})
	ws.WriteMessage(websocket.TextMessage, exitMsg)
}

// wsOutputWriter wraps a WebSocket connection as an io.Writer.
// Sends terminal output as JSON messages to the client.
type wsOutputWriter struct {
	ws *websocket.Conn
	mu sync.Mutex
}

func (w *wsOutputWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	msg, _ := json.Marshal(model.WSMessage{
		Type: "output",
		Data: string(p),
	})
	err := w.ws.WriteMessage(websocket.TextMessage, msg)
	if err != nil {
		return 0, err
	}
	return len(p), nil
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
	logger := slog.Default().With("component", "sandbox_ttl_cleaner")
	expired, err := s.sandboxStore.ListExpiredActive(ctx, time.Now().UTC())
	if err != nil {
		logger.Error("failed to list expired sandboxes", "error", err)
		return
	}

	for _, rec := range expired {
		sandboxID := rec.ID
		logger.Info("deleting expired sandbox", "sandbox_id", sandboxID)
		netPolicyMgr := k8s.NewNetworkPolicyManager(s.k8sClient)
		if err := netPolicyMgr.DeleteDomainAllowlistPolicy(ctx, sandboxID); err != nil {
			logger.Warn("failed to delete domain allowlist policy", "sandbox_id", sandboxID, "error", err)
		}

		_ = s.sandboxStore.SetDesiredDeleted(ctx, sandboxID, time.Now().UTC())
		if err := s.k8sClient.DeletePod(ctx, sandboxID); err != nil && !apierrors.IsNotFound(err) {
			logger.Warn("failed to delete pod", "sandbox_id", sandboxID, "error", err)
			_ = s.sandboxStore.UpdateStatus(ctx, sandboxID, string(model.SandboxStatusFailed), err.Error(), time.Now().UTC())
			continue
		}
		if err := s.sandboxStore.MarkDeleted(ctx, sandboxID, "ttl expired", time.Now().UTC()); err != nil {
			logger.Warn("failed to mark sandbox deleted", "sandbox_id", sandboxID, "error", err)
		}
		_ = s.sandboxStore.AppendStatusHistory(ctx, sandboxID, "ttl_cleaner", rec.LifecycleStatus, "deleted", "ttl expired", nil, time.Now().UTC())
	}
}

func (s *SandboxService) recordToSandbox(record *store.SandboxRecord) (*model.Sandbox, error) {
	accessToken, err := s.tokenCipher.Decrypt(record.AccessTokenCiphertext, record.AccessTokenNonce)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt access token: %w", err)
	}
	return &model.Sandbox{
		ID:              record.ID,
		Image:           record.Image,
		CPU:             record.CPU,
		Memory:          record.Memory,
		TTL:             record.TTL,
		Env:             record.EnvMap(),
		Status:          parseLifecycleStatus(record.LifecycleStatus),
		Template:        record.TemplateName,
		TemplateVersion: record.TemplateVersion,
		CreatedAt:       record.CreatedAt,
		ExpiresAt:       record.ExpiresAt,
		AccessToken:     accessToken,
		AccessURL:       record.AccessURL,
	}, nil
}

func parseLifecycleStatus(v string) model.SandboxStatus {
	switch v {
	case string(model.SandboxStatusPending), "creating":
		return model.SandboxStatusPending
	case string(model.SandboxStatusRunning):
		return model.SandboxStatusRunning
	case string(model.SandboxStatusSucceeded):
		return model.SandboxStatusSucceeded
	case string(model.SandboxStatusFailed):
		return model.SandboxStatusFailed
	case string(model.SandboxStatusTerminating):
		return model.SandboxStatusTerminating
	default:
		return model.SandboxStatusUnknown
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

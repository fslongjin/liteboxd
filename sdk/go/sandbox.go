package liteboxd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"time"
)

// SandboxService handles sandbox operations.
type SandboxService struct {
	client *Client
}

// Create creates a sandbox from a template.
//
// The template parameter is required. Use overrides to customize CPU, memory, TTL, or env.
// Overrides supported: cpu, memory, ttl, env
// NOT overridable: image, startupScript, files, readinessProbe (from template)
func (s *SandboxService) Create(ctx context.Context, template string, overrides *SandboxOverrides) (*Sandbox, error) {
	req := &CreateSandboxRequest{
		Template:  template,
		Overrides: overrides,
	}
	var result Sandbox
	err := s.client.doJSON(ctx, "POST", s.client.buildPath("sandboxes"), req, &result, nil)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// CreateWithVersion creates a sandbox from a specific template version.
func (s *SandboxService) CreateWithVersion(ctx context.Context, template string, version int, overrides *SandboxOverrides) (*Sandbox, error) {
	req := &CreateSandboxRequest{
		Template:        template,
		TemplateVersion: version,
		Overrides:       overrides,
	}
	var result Sandbox
	err := s.client.doJSON(ctx, "POST", s.client.buildPath("sandboxes"), req, &result, nil)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// List retrieves all sandboxes.
func (s *SandboxService) List(ctx context.Context) ([]Sandbox, error) {
	var result SandboxListResponse
	err := s.client.doJSON(ctx, "GET", s.client.buildPath("sandboxes"), nil, &result, nil)
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

// Get retrieves a specific sandbox by ID.
func (s *SandboxService) Get(ctx context.Context, id string) (*Sandbox, error) {
	var result Sandbox
	err := s.client.doJSON(ctx, "GET", s.client.buildPath("sandboxes", id), nil, &result, nil)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Delete removes a sandbox.
func (s *SandboxService) Delete(ctx context.Context, id string) error {
	return s.client.doEmptyResponse(ctx, "DELETE", s.client.buildPath("sandboxes", id), nil, nil)
}

// Execute runs a command in the sandbox.
func (s *SandboxService) Execute(ctx context.Context, id string, command []string, timeout int) (*ExecResponse, error) {
	req := &ExecRequest{
		Command: command,
		Timeout: timeout,
	}
	var result ExecResponse
	err := s.client.doJSON(ctx, "POST", s.client.buildPath("sandboxes", id, "exec"), req, &result, nil)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetLogs retrieves container logs and Pod events.
func (s *SandboxService) GetLogs(ctx context.Context, id string) (*LogsResponse, error) {
	var result LogsResponse
	err := s.client.doJSON(ctx, "GET", s.client.buildPath("sandboxes", id, "logs"), nil, &result, nil)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// UploadFile uploads a file to the sandbox.
func (s *SandboxService) UploadFile(ctx context.Context, id, filePath string, content []byte, contentType string) error {
	// Create multipart form body
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add file field
	part, err := writer.CreateFormFile("file", "file")
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(content); err != nil {
		return fmt.Errorf("failed to write file content: %w", err)
	}

	// Add path field
	if err := writer.WriteField("path", filePath); err != nil {
		return fmt.Errorf("failed to write path field: %w", err)
	}

	// Close writer to finalize the form
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Make request
	resp, err := s.client.doRequestWithReader(ctx, "POST", s.client.buildPath("sandboxes", id, "files"), &body, writer.FormDataContentType(), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check for error status
	if resp.StatusCode >= 400 {
		return handleErrorResponse(resp)
	}

	// Decode response
	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	return nil
}

// DownloadFile downloads a file from the sandbox.
func (s *SandboxService) DownloadFile(ctx context.Context, id, filePath string) ([]byte, error) {
	queryParams := map[string]string{"path": filePath}

	resp, err := s.client.doRequest(ctx, "GET", s.client.buildPath("sandboxes", id, "files"), nil, queryParams)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check for error status
	if resp.StatusCode >= 400 {
		return nil, handleErrorResponse(resp)
	}

	// Read binary content
	return io.ReadAll(resp.Body)
}

// WaitForReady waits until the sandbox reaches running status.
// pollInterval is the time between checks (default 2s).
// timeout is the maximum wait time (default 5m).
func (s *SandboxService) WaitForReady(ctx context.Context, id string, pollInterval, timeout time.Duration) (*Sandbox, error) {
	if pollInterval == 0 {
		pollInterval = 2 * time.Second
	}
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for sandbox to be ready")
		case <-ticker.C:
			sandbox, err := s.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			if sandbox.Status == SandboxStatusRunning {
				return sandbox, nil
			}
			if sandbox.Status == SandboxStatusFailed {
				return nil, fmt.Errorf("sandbox failed")
			}
		}
	}
}

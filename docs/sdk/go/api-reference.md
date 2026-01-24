# LiteBoxd Go SDK - API Reference

Complete API reference for the LiteBoxd Go SDK.

---

## Table of Contents

1. [Client API](#1-client-api)
2. [SandboxService API](#2-sandboxservice-api)
3. [TemplateService API](#3-templateservice-api)
4. [PrepullService API](#4-prepullservice-api)
5. [ImportExportService API](#5-importexportservice-api)

---

## 1. Client API

### Constructor

```go
package liteboxd

// NewClient creates a new LiteBoxd API client
//
// baseURL: The base URL of the LiteBoxd API (e.g., "http://localhost:8080/api/v1")
// opts: Functional options for configuration
//
// Example:
//   client := liteboxd.NewClient("http://localhost:8080/api/v1",
//       liteboxd.WithTimeout(30*time.Second),
//   )
func NewClient(baseURL string, opts ...Option) *Client
```

### Options

```go
// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(client *http.Client) Option

// WithTimeout sets the default timeout for requests
func WithTimeout(timeout time.Duration) Option

// WithAuthToken sets the authentication token header
func WithAuthToken(token string) Option

// WithUserAgent sets a custom User-Agent header
func WithUserAgent(ua string) Option
```

### Client Fields

```go
type Client struct {
    // Service clients
    Sandbox       *SandboxService
    Template      *TemplateService
    Prepull       *PrepullService
    ImportExport  *ImportExportService
}
```

---

## 2. SandboxService API

```go
type SandboxService struct{}
```

**Important**: Sandbox creation is template-only. Direct sandbox creation (without template) is not supported.

### Create

```go
// Create creates a sandbox from a template
//
// Parameters:
//   - ctx: Context for cancellation/timeout
//   - template: Template name (required)
//   - overrides: Optional overrides for CPU, memory, TTL, and env
//
// Returns:
//   - *Sandbox: The created sandbox
//   - error: API error or request failure
func (s *SandboxService) Create(ctx context.Context, template string, overrides *model.SandboxOverrides) (*model.Sandbox, error)
```

**Example**:
```go
// Create with default template config
sandbox, err := client.Sandbox.Create(ctx, "python-data-science", nil)

// Create with overrides
sandbox, err := client.Sandbox.Create(ctx, "python-data-science", &model.SandboxOverrides{
    TTL: 7200,
    Env: map[string]string{"DEBUG": "true"},
})
```

### CreateWithVersion

```go
// CreateWithVersion creates a sandbox from a specific template version
//
// Parameters:
//   - ctx: Context for cancellation/timeout
//   - template: Template name
//   - version: Template version to use
//   - overrides: Optional overrides
func (s *SandboxService) CreateWithVersion(ctx context.Context, template string, version int, overrides *model.SandboxOverrides) (*model.Sandbox, error)
```

**Example**:
```go
sandbox, err := client.Sandbox.CreateWithVersion(ctx, "python-data-science", 2, nil)
```

### List

```go
// List retrieves all sandboxes
func (s *SandboxService) List(ctx context.Context) ([]model.Sandbox, error)
```

### Get

```go
// Get retrieves a specific sandbox by ID
//
// Returns ErrNotFound if sandbox doesn't exist
func (s *SandboxService) Get(ctx context.Context, id string) (*model.Sandbox, error)
```

### Delete

```go
// Delete removes a sandbox
func (s *SandboxService) Delete(ctx context.Context, id string) error
```

### Execute

```go
// Execute runs a command in the sandbox
//
// Parameters:
//   - ctx: Context for cancellation/timeout
//   - id: Sandbox ID
//   - command: Command arguments (e.g., []string{"python", "-c", "print('hello')"})
//   - timeout: Execution timeout in seconds (0 for default 30s)
//
// Returns:
//   - *ExecResponse: Exit code, stdout, stderr
func (s *SandboxService) Execute(ctx context.Context, id string, command []string, timeout int) (*model.ExecResponse, error)
```

**Example**:
```go
resp, err := client.Sandbox.Execute(ctx, sandbox.ID, []string{"python", "-c", "print('hello')"}, 30)
fmt.Printf("Exit code: %d\n", resp.ExitCode)
fmt.Printf("Stdout: %s\n", resp.Stdout)
```

### GetLogs

```go
// GetLogs retrieves container logs and Pod events
func (s *SandboxService) GetLogs(ctx context.Context, id string) (*model.LogsResponse, error)
```

### UploadFile

```go
// UploadFile uploads a file to the sandbox
//
// Parameters:
//   - ctx: Context for cancellation/timeout
//   - id: Sandbox ID
//   - path: Destination path in sandbox (e.g., "/workspace/file.txt")
//   - content: File content
//   - contentType: MIME type (optional, defaults to application/octet-stream)
func (s *SandboxService) UploadFile(ctx context.Context, id, path string, content []byte, contentType string) error
```

### DownloadFile

```go
// DownloadFile downloads a file from the sandbox
func (s *SandboxService) DownloadFile(ctx context.Context, id, path string) ([]byte, error)
```

### WaitForReady

```go
// WaitForReady waits until the sandbox reaches running status
//
// Parameters:
//   - ctx: Context for cancellation/timeout
//   - id: Sandbox ID
//   - pollInterval: Time between checks (default 2s)
//   - timeout: Maximum wait time (default 5m)
//
// Returns:
//   - *Sandbox: The ready sandbox
//   - error: Timeout or error status
func (s *SandboxService) WaitForReady(ctx context.Context, id string, pollInterval, timeout time.Duration) (*model.Sandbox, error)
```

---

## 3. TemplateService API

```go
type TemplateService struct{}
```

### Create

```go
// Create creates a new template
//
// Returns ErrConflict if template exists
func (t *TemplateService) Create(ctx context.Context, req *model.CreateTemplateRequest) (*model.Template, error)
```

**Example**:
```go
template, err := client.Template.Create(ctx, &model.CreateTemplateRequest{
    Name:        "python-basic",
    DisplayName: "Python Basic",
    Description: "Basic Python environment",
    Tags:        []string{"python", "basic"},
    Spec: model.TemplateSpec{
        Image: "python:3.11-slim",
        Resources: model.ResourceSpec{
            CPU:    "500m",
            Memory: "512Mi",
        },
        TTL: 3600,
    },
    AutoPrepull: true,
})
```

### List

```go
// List retrieves templates with optional filtering
func (t *TemplateService) List(ctx context.Context, opts *model.TemplateListOptions) (*model.TemplateListResponse, error)
```

### Get

```go
// Get retrieves a specific template by name
func (t *TemplateService) Get(ctx context.Context, name string) (*model.Template, error)
```

### Update

```go
// Update updates a template (creates new version)
func (t *TemplateService) Update(ctx context.Context, name string, req *model.UpdateTemplateRequest) (*model.Template, error)
```

### Delete

```go
// Delete removes a template
func (t *TemplateService) Delete(ctx context.Context, name string) error
```

### ListVersions

```go
// ListVersions retrieves all versions of a template
func (t *TemplateService) ListVersions(ctx context.Context, name string) (*model.VersionListResponse, error)
```

### GetVersion

```go
// GetVersion retrieves a specific version of a template
func (t *TemplateService) GetVersion(ctx context.Context, name string, version int) (*model.TemplateVersion, error)
```

### Rollback

```go
// Rollback rolls back a template to a previous version
//
// Parameters:
//   - ctx: Context for cancellation/timeout
//   - name: Template name
//   - targetVersion: Version to rollback to
//   - changelog: Optional changelog for the rollback
func (t *TemplateService) Rollback(ctx context.Context, name string, targetVersion int, changelog string) (*model.RollbackResponse, error)
```

### ExportYAML

```go
// ExportYAML exports a template to YAML format
//
// Parameters:
//   - ctx: Context for cancellation/timeout
//   - name: Template name
//   - version: Optional version (0 for latest)
func (t *TemplateService) ExportYAML(ctx context.Context, name string, version int) ([]byte, error)
```

---

## 4. PrepullService API

```go
type PrepullService struct{}
```

### Create

```go
// Create creates a new prepull task
//
// Returns ErrConflict if prepull already in progress
func (p *PrepullService) Create(ctx context.Context, image string, timeout int) (*model.PrepullResponse, error)
```

### CreateForTemplate

```go
// CreateForTemplate creates a prepull task for a template's image
func (p *PrepullService) CreateForTemplate(ctx context.Context, templateName string) (*model.PrepullResponse, error)
```

### List

```go
// List retrieves prepull tasks
func (p *PrepullService) List(ctx context.Context, image, status string) ([]model.PrepullResponse, error)
```

### Get

```go
// Get retrieves a specific prepull task
func (p *PrepullService) Get(ctx context.Context, id string) (*model.PrepullResponse, error)
```

### Delete

```go
// Delete cancels and removes a prepull task
func (p *PrepullService) Delete(ctx context.Context, id string) error
```

### WaitForCompletion

```go
// WaitForCompletion waits until prepull completes
//
// Parameters:
//   - ctx: Context for cancellation/timeout
//   - id: Prepull task ID
//   - pollInterval: Time between checks (default 5s)
//   - timeout: Maximum wait time (default 30m)
func (p *PrepullService) WaitForCompletion(ctx context.Context, id string, pollInterval, timeout time.Duration) (*model.PrepullResponse, error)
```

---

## 5. ImportExportService API

```go
type ImportExportService struct{}
```

### ImportTemplates

```go
// ImportTemplates imports templates from YAML
//
// Parameters:
//   - ctx: Context for cancellation/timeout
//   - yaml: YAML content
//   - strategy: Import strategy (create-only, update-only, create-or-update)
//   - autoPrepull: Trigger prepull after import
func (ie *ImportExportService) ImportTemplates(ctx context.Context, yaml []byte, strategy string, autoPrepull bool) (*model.ImportTemplatesResponse, error)
```

### ExportAllTemplates

```go
// ExportAllTemplates exports all templates to YAML
//
// Parameters:
//   - ctx: Context for cancellation/timeout
//   - tag: Optional filter by tag
//   - names: Optional comma-separated template names
func (ie *ImportExportService) ExportAllTemplates(ctx context.Context, tag, names string) ([]byte, error)
```

---

## Error Types

```go
// APIError represents an API error response
type APIError struct {
    StatusCode int
    Message    string
    Err        error
}

// Error returns the error message
func (e *APIError) Error() string

// Is checks if the error matches target
func (e *APIError) Is(target error) bool

// Unwrap returns the underlying error
func (e *APIError) Unwrap() error
```

### Common Errors

```go
var (
    ErrNotFound   = &APIError{StatusCode: 404, Message: "resource not found"}
    ErrConflict   = &APIError{StatusCode: 409, Message: "resource already exists"}
    ErrBadRequest = &APIError{StatusCode: 400, Message: "invalid request"}
    ErrInternal   = &APIError{StatusCode: 500, Message: "internal server error"}
)
```

### Usage Example

```go
sandbox, err := client.Sandbox.Get(ctx, id)
if errors.Is(err, liteboxd.ErrNotFound) {
    // Handle not found
    return err
}
if err != nil {
    // Handle other errors
    return err
}
```

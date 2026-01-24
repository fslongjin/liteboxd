# LiteBoxd Go SDK Design Document

## 1. Overview

This document describes the design and implementation plan for the LiteBoxd Go SDK.

### 1.1 Goals

- Provide a type-safe, idiomatic Go SDK for LiteBoxd API integration
- Enable developers to interact with LiteBoxd programmatically
- Support template-centric workflow (all sandboxes created from templates)
- Support all existing API features (templates, sandboxes, prepull, import/export)
- Serve as reference implementation for future SDKs (Python, Node.js, etc.)

### 1.2 Non-Goals

- WebSocket support (not yet in the API)
- Direct sandbox creation without template (not supported by API)
- Advanced streaming operations (future enhancement)
- Retry logic (users can implement on top with context)

---

## 2. Project Structure

```
sdk/go/
├── go.mod                    # Go module definition
├── go.sum
├── README.md                 # SDK documentation
├── client.go                 # Main client struct and configuration
├── options.go                # Client options (WithTimeout, WithHTTPClient, etc.)
├── errors.go                 # Error types and wrapping
├── sandbox.go                # Sandbox service client
├── template.go               # Template service client
├── prepull.go                # Prepull service client
├── import_export.go          # Import/Export service client
└── examples/
    ├── sandbox_create.go     # Example: Create sandbox
    ├── template_create.go    # Example: Create template
    └── exec_command.go       # Example: Execute command
```

**Module Path**: `github.com/fslongjin/liteboxd/sdk/go`

---

## 3. Core Client Design

```go
package liteboxd

// Client is the main API client
type Client struct {
    baseURL    string
    httpClient *http.Client
    authToken  string // Optional API token for future auth

    // Service clients
    Sandbox    *SandboxService
    Template   *TemplateService
    Prepull    *PrepullService
    ImportExport *ImportExportService
}

// Option is a functional option for client configuration
type Option func(*Client)

// NewClient creates a new LiteBoxd API client
func NewClient(baseURL string, opts ...Option) *Client

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(client *http.Client) Option

// WithTimeout sets request timeout
func WithTimeout(timeout time.Duration) Option

// WithAuthToken sets authentication token
func WithAuthToken(token string) Option
```

---

## 4. Service Client Interface

### 4.1 SandboxService

**Important**: Sandbox creation is now template-only. You must specify a template name.

```go
type SandboxService struct {
    client *Client
}

// Create creates a sandbox from a template
//
// The template parameter is required. Use overrides to customize CPU, memory, TTL, or env.
//
// Overrides supported: cpu, memory, ttl, env
// NOT overridable: image, startupScript, files, readinessProbe (from template)
func (s *SandboxService) Create(ctx context.Context, template string, overrides *SandboxOverrides) (*model.Sandbox, error)
func (s *SandboxService) CreateWithVersion(ctx context.Context, template string, version int, overrides *SandboxOverrides) (*model.Sandbox, error)
func (s *SandboxService) List(ctx context.Context) ([]model.Sandbox, error)
func (s *SandboxService) Get(ctx context.Context, id string) (*model.Sandbox, error)
func (s *SandboxService) Delete(ctx context.Context, id string) error
func (s *SandboxService) Execute(ctx context.Context, id string, command []string, timeout int) (*model.ExecResponse, error)
func (s *SandboxService) GetLogs(ctx context.Context, id string) (*model.LogsResponse, error)
func (s *SandboxService) UploadFile(ctx context.Context, id, path string, content []byte, contentType string) error
func (s *SandboxService) DownloadFile(ctx context.Context, id, path string) ([]byte, error)
func (s *SandboxService) WaitForReady(ctx context.Context, id string, pollInterval, timeout time.Duration) (*model.Sandbox, error)
```

**SandboxOverrides**:
```go
type SandboxOverrides struct {
    CPU    string            `json:"cpu,omitempty"`
    Memory string            `json:"memory,omitempty"`
    TTL    int               `json:"ttl,omitempty"`
    Env    map[string]string `json:"env,omitempty"`
}
```

### 4.2 TemplateService

```go
type TemplateService struct {
    client *Client
}

func (t *TemplateService) Create(ctx context.Context, req *CreateTemplateRequest) (*Template, error)
func (t *TemplateService) List(ctx context.Context, opts *TemplateListOptions) (*TemplateListResponse, error)
func (t *TemplateService) Get(ctx context.Context, name string) (*Template, error)
func (t *TemplateService) Update(ctx context.Context, name string, req *UpdateTemplateRequest) (*Template, error)
func (t *TemplateService) Delete(ctx context.Context, name string) error
func (t *TemplateService) ListVersions(ctx context.Context, name string) (*VersionListResponse, error)
func (t *TemplateService) GetVersion(ctx context.Context, name string, version int) (*TemplateVersion, error)
func (t *TemplateService) Rollback(ctx context.Context, name string, targetVersion int, changelog string) (*RollbackResponse, error)
func (t *TemplateService) ExportYAML(ctx context.Context, name string, version int) ([]byte, error)
```

### 4.3 PrepullService

```go
type PrepullService struct {
    client *Client
}

func (p *PrepullService) Create(ctx context.Context, image string, timeout int) (*PrepullResponse, error)
func (p *PrepullService) CreateForTemplate(ctx context.Context, templateName string) (*PrepullResponse, error)
func (p *PrepullService) List(ctx context.Context, image, status string) ([]PrepullResponse, error)
func (p *PrepullService) Get(ctx context.Context, id string) (*PrepullResponse, error)
func (p *PrepullService) Delete(ctx context.Context, id string) error
func (p *PrepullService) WaitForCompletion(ctx context.Context, id string, pollInterval, timeout time.Duration) (*PrepullResponse, error)
```

### 4.4 ImportExportService

```go
type ImportExportService struct {
    client *Client
}

func (ie *ImportExportService) ImportTemplates(ctx context.Context, yaml []byte, strategy string, autoPrepull bool) (*ImportTemplatesResponse, error)
func (ie *ImportExportService) ExportAllTemplates(ctx context.Context, tag, names string) ([]byte, error)
```

---

## 5. Type Reuse Strategy

The SDK will reuse types from the backend where possible:

- Import `github.com/fslongjin/liteboxd/backend/internal/model` for data types
- Create SDK-specific request types only when additional functionality is needed
- This ensures type consistency across the codebase

```go
import "github.com/fslongjin/liteboxd/backend/internal/model"

// Use backend types directly
func (s *SandboxService) Create(ctx context.Context, req *model.CreateSandboxRequest) (*model.Sandbox, error)
```

---

## 6. Error Handling

```go
// APIError represents an API error response
type APIError struct {
    StatusCode int
    Message    string
    Err        error
}

func (e *APIError) Error() string
func (e *APIError) Is(target error) bool
func (e *APIError) Unwrap() error

// Common error variables
var (
    ErrNotFound   = errors.New("resource not found")
    ErrConflict   = errors.New("resource already exists")
    ErrBadRequest = errors.New("invalid request")
    ErrInternal   = errors.New("internal server error")
)
```

---

## 7. Context Support

All service methods accept `context.Context` for:

- Cancellation of long-running requests
- Timeout propagation
- Request-scoped values (tracing, logging)

---

## 8. Implementation Plan

### Phase 1: SDK Foundation

| Task | Description |
|------|-------------|
| 1.1 | Initialize SDK module with `go.mod` |
| 1.2 | Implement `Client` struct with functional options |
| 1.3 | Implement error types and error handling |
| 1.4 | Implement base HTTP request methods |
| 1.5 | Add unit tests for client configuration |

### Phase 2: Sandbox Service

| Task | Description |
|------|-------------|
| 2.1 | Implement `SandboxService` with CRUD operations |
| 2.2 | Implement command execution (`Exec`) |
| 2.3 | Implement log retrieval |
| 2.4 | Implement file upload/download |
| 2.5 | Add integration tests |

### Phase 3: Template Service

| Task | Description |
|------|-------------|
| 3.1 | Implement `TemplateService` with CRUD operations |
| 3.2 | Implement version management (list, get, rollback) |
| 3.3 | Add integration tests |

### Phase 4: Prepull & Import/Export Services

| Task | Description |
|------|-------------|
| 4.1 | Implement `PrepullService` |
| 4.2 | Implement `ImportExportService` |
| 4.3 | Add integration tests |

### Phase 5: Documentation & Examples

| Task | Description |
|------|-------------|
| 5.1 | Write SDK README with usage examples |
| 5.2 | Add Go SDK examples to `examples/` directory |

---

## 9. Open Questions

1. **SDK Versioning**: How will the SDK be versioned?
   - **Recommendation**: Semantic versioning with stable API guarantee

2. **Go Version**: Minimum Go version to support?
   - **Recommendation**: Go 1.23+ (same as backend)

---

## 10. Success Criteria

- [ ] SDK provides complete coverage of all API endpoints
- [ ] SDK has >80% test coverage
- [ ] Documentation includes usage examples for all major features
- [ ] SDK can be imported and used with standard Go tooling

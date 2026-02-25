package model

import "time"

type SandboxStatus string

const (
	SandboxStatusPending     SandboxStatus = "pending"
	SandboxStatusRunning     SandboxStatus = "running"
	SandboxStatusSucceeded   SandboxStatus = "succeeded"
	SandboxStatusFailed      SandboxStatus = "failed"
	SandboxStatusTerminating SandboxStatus = "terminating"
	SandboxStatusUnknown     SandboxStatus = "unknown"
)

type Sandbox struct {
	ID              string            `json:"id"`
	Image           string            `json:"image"`
	CPU             string            `json:"cpu"`
	Memory          string            `json:"memory"`
	TTL             int               `json:"ttl"`
	Env             map[string]string `json:"env,omitempty"`
	Status          SandboxStatus     `json:"status"`
	Template        string            `json:"template,omitempty"`
	TemplateVersion int               `json:"templateVersion,omitempty"`
	DesiredState    string            `json:"desired_state,omitempty"`
	LifecycleStatus string            `json:"lifecycle_status,omitempty"`
	StatusReason    string            `json:"status_reason,omitempty"`
	PodPhase        string            `json:"pod_phase,omitempty"`
	PodIP           string            `json:"pod_ip,omitempty"`
	LastSeenAt      *time.Time        `json:"last_seen_at,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	ExpiresAt       time.Time         `json:"expires_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	DeletedAt       *time.Time        `json:"deleted_at,omitempty"`

	// Network access fields
	AccessToken string `json:"accessToken,omitempty"` // Access token for inbound requests
	AccessURL   string `json:"accessUrl,omitempty"`   // Base URL for accessing the sandbox
}

// CreateSandboxRequest represents a request to create a sandbox from a template.
// All sandboxes must be created from a template.
type CreateSandboxRequest struct {
	// Template is required - all sandboxes must be created from a template
	Template        string            `json:"template" binding:"required"`
	TemplateVersion int               `json:"templateVersion"`
	Overrides       *SandboxOverrides `json:"overrides"`
}

// SandboxOverrides allows overriding template configuration
type SandboxOverrides struct {
	CPU    string            `json:"cpu,omitempty"`
	Memory string            `json:"memory,omitempty"`
	TTL    int               `json:"ttl,omitempty"`
	Env    map[string]string `json:"env,omitempty"`
	// Note: Network configuration cannot be overridden, it must be set in the template spec
}

type ExecRequest struct {
	Command []string `json:"command" binding:"required"`
	Timeout int      `json:"timeout"`
}

type ExecResponse struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

type SandboxListResponse struct {
	Items []Sandbox `json:"items"`
}

type SandboxMetadataListOptions struct {
	ID              string
	Template        string
	DesiredState    string
	LifecycleStatus string
	CreatedFrom     *time.Time
	CreatedTo       *time.Time
	DeletedFrom     *time.Time
	DeletedTo       *time.Time
	Page            int
	PageSize        int
}

type SandboxMetadataListResponse struct {
	Items    []Sandbox `json:"items"`
	Total    int       `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}

type SandboxStatusHistoryItem struct {
	ID         int64     `json:"id"`
	SandboxID  string    `json:"sandbox_id"`
	Source     string    `json:"source"`
	FromStatus string    `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	Reason     string    `json:"reason"`
	PayloadRaw string    `json:"payload_json"`
	CreatedAt  time.Time `json:"created_at"`
}

type SandboxStatusHistoryResponse struct {
	Items []SandboxStatusHistoryItem `json:"items"`
}

type LogsResponse struct {
	Logs   string   `json:"logs"`
	Events []string `json:"events"`
}

// WSMessage represents a WebSocket message for interactive exec
type WSMessage struct {
	Type     string `json:"type"`               // "input", "output", "resize", "exit", "error"
	Data     string `json:"data,omitempty"`     // terminal data (input/output)
	Cols     int    `json:"cols,omitempty"`     // terminal columns (resize)
	Rows     int    `json:"rows,omitempty"`     // terminal rows (resize)
	ExitCode int    `json:"exitCode,omitempty"` // process exit code (exit)
	Message  string `json:"message,omitempty"`  // error message (error)
}

// ExecInteractiveRequest defines parameters for interactive exec
type ExecInteractiveRequest struct {
	Command []string `json:"command"`
	TTY     bool     `json:"tty"`
	Cols    int      `json:"cols"`
	Rows    int      `json:"rows"`
}

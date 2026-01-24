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
	CreatedAt       time.Time         `json:"created_at"`
	ExpiresAt       time.Time         `json:"expires_at"`
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

type LogsResponse struct {
	Logs   string   `json:"logs"`
	Events []string `json:"events"`
}

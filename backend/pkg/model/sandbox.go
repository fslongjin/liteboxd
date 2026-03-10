package model

import "time"

type SandboxStatus string

const (
	SandboxStatusPending     SandboxStatus = "pending"
	SandboxStatusRunning     SandboxStatus = "running"
	SandboxStatusSucceeded   SandboxStatus = "succeeded"
	SandboxStatusFailed      SandboxStatus = "failed"
	SandboxStatusStopped     SandboxStatus = "stopped"
	SandboxStatusTerminating SandboxStatus = "terminating"
	SandboxStatusUnknown     SandboxStatus = "unknown"
)

const (
	PVCMappingStateBound        = "bound"
	PVCMappingStateOrphanPVC    = "orphan_pvc"
	PVCMappingStateDanglingMeta = "dangling_metadata"
	PVCMappingSourceDBAndK8s    = "db+k8s"
	PVCMappingSourceDB          = "db"
	PVCMappingSourceK8s         = "k8s"
)

type Sandbox struct {
	ID              string              `json:"id"`
	Image           string              `json:"image"`
	CPU             string              `json:"cpu"`
	Memory          string              `json:"memory"`
	TTL             int                 `json:"ttl"`
	Env             map[string]string   `json:"env,omitempty"`
	Status          SandboxStatus       `json:"status"`
	Template        string              `json:"template,omitempty"`
	TemplateVersion int                 `json:"templateVersion,omitempty"`
	DesiredState    string              `json:"desired_state,omitempty"`
	LifecycleStatus string              `json:"lifecycle_status,omitempty"`
	StatusReason    string              `json:"status_reason,omitempty"`
	PodPhase        string              `json:"pod_phase,omitempty"`
	PodIP           string              `json:"pod_ip,omitempty"`
	LastSeenAt      *time.Time          `json:"last_seen_at,omitempty"`
	CreatedAt       time.Time           `json:"created_at"`
	ExpiresAt       time.Time           `json:"expires_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
	DeletedAt       *time.Time          `json:"deleted_at,omitempty"`
	Persistence     *SandboxPersistence `json:"persistence,omitempty"`
	RuntimeKind     string              `json:"runtimeKind,omitempty"`
	RuntimeName     string              `json:"runtimeName,omitempty"`

	// Network access fields
	AccessToken string `json:"accessToken,omitempty"` // Access token for inbound requests
	AccessURL   string `json:"accessUrl,omitempty"`   // Base URL for accessing the sandbox
}

// SandboxPersistence describes effective persistence settings on a sandbox.
type SandboxPersistence struct {
	Enabled          bool   `json:"enabled"`
	Mode             string `json:"mode,omitempty"`
	Size             string `json:"size,omitempty"`
	StorageClassName string `json:"storageClassName,omitempty"`
	ReclaimPolicy    string `json:"reclaimPolicy,omitempty"`
	VolumeClaimName  string `json:"volumeClaimName,omitempty"`
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
	CPU         string                       `json:"cpu,omitempty"`
	Memory      string                       `json:"memory,omitempty"`
	TTL         *int                         `json:"ttl,omitempty"`
	Env         map[string]string            `json:"env,omitempty"`
	Persistence *SandboxPersistenceOverrides `json:"persistence,omitempty"`
	// Note: Network configuration cannot be overridden, it must be set in the template spec
}

// SandboxPersistenceOverrides allows selected persistence fields to be overridden per sandbox.
type SandboxPersistenceOverrides struct {
	Size string `json:"size,omitempty"`
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

type PVCMappingListOptions struct {
	SandboxID    string
	StorageClass string
	State        string
	Page         int
	PageSize     int
}

type PVCMapping struct {
	PVCName               string `json:"pvcName"`
	Namespace             string `json:"namespace"`
	StorageClassName      string `json:"storageClassName,omitempty"`
	RequestedSize         string `json:"requestedSize,omitempty"`
	Phase                 string `json:"phase,omitempty"`
	PVName                string `json:"pvName,omitempty"`
	SandboxID             string `json:"sandboxId,omitempty"`
	SandboxLifecycleState string `json:"sandboxLifecycleStatus,omitempty"`
	ReclaimPolicy         string `json:"reclaimPolicy,omitempty"`
	State                 string `json:"state"`
	Source                string `json:"source"`
}

type PVCMappingListResponse struct {
	Items    []PVCMapping `json:"items"`
	Total    int          `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
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

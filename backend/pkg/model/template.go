package model

import (
	"encoding/json"
	"time"
)

// Template represents a sandbox template
type Template struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	DisplayName   string    `json:"displayName"`
	Description   string    `json:"description"`
	Tags          []string  `json:"tags"`
	Author        string    `json:"author"`
	IsPublic      bool      `json:"isPublic"`
	LatestVersion int       `json:"latestVersion"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`

	// Non-persisted field for API response
	Spec *TemplateSpec `json:"spec,omitempty"`
}

// TemplateVersion represents a version of a template
type TemplateVersion struct {
	ID         string       `json:"id"`
	TemplateID string       `json:"templateId"`
	Version    int          `json:"version"`
	Spec       TemplateSpec `json:"spec"`
	Changelog  string       `json:"changelog"`
	CreatedBy  string       `json:"createdBy"`
	CreatedAt  time.Time    `json:"createdAt"`
}

const (
	PersistenceModeRootFSOverlay = "rootfs-overlay"
	PersistenceReclaimDelete     = "Delete"
	PersistenceReclaimRetain     = "Retain"
	PersistenceDefaultSize       = "1Gi"
)

// TemplateSpec defines the specification of a template
type TemplateSpec struct {
	Image          string            `json:"image" yaml:"image"`
	Command        []string          `json:"command,omitempty" yaml:"command,omitempty"` // Override container entrypoint; empty = use image default (OCI CMD)
	Args           []string          `json:"args,omitempty" yaml:"args,omitempty"`       // Override container args; empty = use image default
	Resources      ResourceSpec      `json:"resources" yaml:"resources"`
	TTL            int               `json:"ttl" yaml:"ttl"`
	Env            map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	StartupScript  string            `json:"startupScript,omitempty" yaml:"startupScript,omitempty"`
	StartupTimeout int               `json:"startupTimeout,omitempty" yaml:"startupTimeout,omitempty"`
	Files          []FileSpec        `json:"files,omitempty" yaml:"files,omitempty"`
	ReadinessProbe *ProbeSpec        `json:"readinessProbe,omitempty" yaml:"readinessProbe,omitempty"`
	Network        *NetworkSpec      `json:"network,omitempty" yaml:"network,omitempty"`
	Persistence    *PersistenceSpec  `json:"persistence,omitempty" yaml:"persistence,omitempty"`
}

// ResourceSpec defines resource limits
type ResourceSpec struct {
	CPU    string `json:"cpu" yaml:"cpu"`
	Memory string `json:"memory" yaml:"memory"`
}

// FileSpec defines a file to be uploaded to the sandbox
type FileSpec struct {
	Source      string `json:"source,omitempty" yaml:"source,omitempty"`
	Destination string `json:"destination" yaml:"destination"`
	Content     string `json:"content,omitempty" yaml:"content,omitempty"`
}

// ProbeSpec defines a readiness probe
type ProbeSpec struct {
	Exec                ExecAction `json:"exec" yaml:"exec"`
	InitialDelaySeconds int        `json:"initialDelaySeconds" yaml:"initialDelaySeconds"`
	PeriodSeconds       int        `json:"periodSeconds" yaml:"periodSeconds"`
	FailureThreshold    int        `json:"failureThreshold" yaml:"failureThreshold"`
}

// ExecAction defines an exec action for probe
type ExecAction struct {
	Command []string `json:"command" yaml:"command"`
}

// PersistenceSpec defines persistent disk behavior for sandboxes created from a template.
type PersistenceSpec struct {
	Enabled          bool   `json:"enabled" yaml:"enabled"`
	Mode             string `json:"mode,omitempty" yaml:"mode,omitempty"`
	Size             string `json:"size,omitempty" yaml:"size,omitempty"`
	StorageClassName string `json:"storageClassName,omitempty" yaml:"storageClassName,omitempty"`
	ReclaimPolicy    string `json:"reclaimPolicy,omitempty" yaml:"reclaimPolicy,omitempty"`
}

// MarshalTags serializes Tags to JSON string for database storage
func (t *Template) MarshalTags() string {
	if t.Tags == nil {
		return "[]"
	}
	data, _ := json.Marshal(t.Tags)
	return string(data)
}

// UnmarshalTags deserializes Tags from JSON string
func (t *Template) UnmarshalTags(data string) error {
	if data == "" {
		t.Tags = []string{}
		return nil
	}
	return json.Unmarshal([]byte(data), &t.Tags)
}

// MarshalSpec serializes Spec to JSON string
func (v *TemplateVersion) MarshalSpec() string {
	data, _ := json.Marshal(v.Spec)
	return string(data)
}

// UnmarshalSpec deserializes Spec from JSON string
func (v *TemplateVersion) UnmarshalSpec(data string) error {
	return json.Unmarshal([]byte(data), &v.Spec)
}

// ApplyDefaults applies default values to the spec
func (s *TemplateSpec) ApplyDefaults() {
	if s.Resources.CPU == "" {
		s.Resources.CPU = "500m"
	}
	if s.Resources.Memory == "" {
		s.Resources.Memory = "512Mi"
	}
	if s.StartupTimeout == 0 {
		s.StartupTimeout = 300
	}
	if s.Persistence != nil {
		if s.Persistence.Enabled && s.Persistence.Mode == "" {
			s.Persistence.Mode = PersistenceModeRootFSOverlay
		}
		if s.Persistence.Enabled && s.Persistence.Size == "" {
			s.Persistence.Size = PersistenceDefaultSize
		}
		if s.Persistence.ReclaimPolicy == "" {
			s.Persistence.ReclaimPolicy = PersistenceReclaimDelete
		}
	}
}

// --- Request/Response types ---

// CreateTemplateRequest is the request body for creating a template
type CreateTemplateRequest struct {
	Name        string       `json:"name" binding:"required"`
	DisplayName string       `json:"displayName"`
	Description string       `json:"description"`
	Tags        []string     `json:"tags"`
	IsPublic    *bool        `json:"isPublic"`
	Spec        TemplateSpec `json:"spec" binding:"required"`
	AutoPrepull bool         `json:"autoPrepull"`
}

// UpdateTemplateRequest is the request body for updating a template
type UpdateTemplateRequest struct {
	DisplayName string       `json:"displayName"`
	Description string       `json:"description"`
	Tags        []string     `json:"tags"`
	IsPublic    *bool        `json:"isPublic"`
	Spec        TemplateSpec `json:"spec" binding:"required"`
	Changelog   string       `json:"changelog"`
}

// RollbackRequest is the request body for rolling back a template
type RollbackRequest struct {
	TargetVersion int    `json:"targetVersion" binding:"required"`
	Changelog     string `json:"changelog"`
}

// TemplateListResponse is the response for listing templates
type TemplateListResponse struct {
	Items    []Template `json:"items"`
	Total    int        `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"pageSize"`
}

// VersionListResponse is the response for listing versions
type VersionListResponse struct {
	Items []TemplateVersion `json:"items"`
	Total int               `json:"total"`
}

// TemplateListOptions defines options for listing templates
type TemplateListOptions struct {
	Tag      string
	Search   string
	Page     int
	PageSize int
}

// RollbackResponse is the response for rollback operation
type RollbackResponse struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	LatestVersion  int       `json:"latestVersion"`
	RolledBackFrom int       `json:"rolledBackFrom"`
	RolledBackTo   int       `json:"rolledBackTo"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

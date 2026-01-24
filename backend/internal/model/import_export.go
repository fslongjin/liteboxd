package model

// ImportStrategy defines the template import strategy
type ImportStrategy string

const (
	ImportStrategyCreateOnly     ImportStrategy = "create-only"
	ImportStrategyUpdateOnly     ImportStrategy = "update-only"
	ImportStrategyCreateOrUpdate ImportStrategy = "create-or-update"
)

// ImportTemplatesRequest is the request for importing templates from YAML
type ImportTemplatesRequest struct {
	File     string         `form:"file" binding:"required"`
	Strategy ImportStrategy `form:"strategy"`
	Prepull  bool           `form:"prepull"`
}

// ImportResult represents the result of importing a single template
type ImportResult struct {
	Name    string `json:"name"`
	Action  string `json:"action"` // created, updated, skipped, failed
	Version int    `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ImportTemplatesResponse is the response for import operation
type ImportTemplatesResponse struct {
	Total          int            `json:"total"`
	Created        int            `json:"created"`
	Updated        int            `json:"updated"`
	Skipped        int            `json:"skipped"`
	Failed         int            `json:"failed"`
	Results        []ImportResult `json:"results"`
	PrepullStarted []string       `json:"prepullStarted,omitempty"`
}

// TemplateYAML represents a template in YAML format
type TemplateYAML struct {
	APIVersion string               `yaml:"apiVersion"`
	Kind       string               `yaml:"kind"`
	Metadata   TemplateYAMLMetadata `yaml:"metadata"`
	Spec       TemplateSpec         `yaml:"spec"`
}

// TemplateYAMLMetadata represents metadata section of YAML template
type TemplateYAMLMetadata struct {
	Name        string   `yaml:"name"`
	DisplayName string   `yaml:"displayName"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
}

// TemplateListYAML represents multiple templates in YAML format
type TemplateListYAML struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	ExportedAt string         `yaml:"exportedAt,omitempty"`
	Items      []TemplateYAML `yaml:"items"`
}

// ExportOptions defines options for exporting templates
type ExportOptions struct {
	Tag   string   // Filter by tag
	Names []string // Filter by names
}

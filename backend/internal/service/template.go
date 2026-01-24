package service

import (
	"context"
	"fmt"
	"regexp"

	"github.com/fslongjin/liteboxd/internal/model"
	"github.com/fslongjin/liteboxd/internal/store"
)

// TemplateService handles template business logic
type TemplateService struct {
	store      *store.TemplateStore
	prepullSvc *PrepullService
}

// NewTemplateService creates a new TemplateService
func NewTemplateService() *TemplateService {
	return &TemplateService{
		store: store.NewTemplateStore(),
	}
}

// SetPrepullService sets the prepull service for auto-prepull functionality
func (s *TemplateService) SetPrepullService(prepullSvc *PrepullService) {
	s.prepullSvc = prepullSvc
}

// namePattern validates template names
var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$`)

// validateName validates the template name
func validateName(name string) error {
	if len(name) < 1 || len(name) > 63 {
		return fmt.Errorf("name must be between 1 and 63 characters")
	}
	if !namePattern.MatchString(name) {
		return fmt.Errorf("name must consist of lowercase letters, numbers, and hyphens, and must start and end with a letter or number")
	}
	return nil
}

// validateSpec validates the template spec
func validateSpec(spec *model.TemplateSpec) error {
	if spec.Image == "" {
		return fmt.Errorf("image is required")
	}
	return nil
}

// Create creates a new template
func (s *TemplateService) Create(ctx context.Context, req *model.CreateTemplateRequest) (*model.Template, error) {
	// Validate name
	if err := validateName(req.Name); err != nil {
		return nil, fmt.Errorf("invalid name: %w", err)
	}

	// Validate spec
	if err := validateSpec(&req.Spec); err != nil {
		return nil, fmt.Errorf("invalid spec: %w", err)
	}

	// Check if name already exists
	exists, err := s.store.Exists(ctx, req.Name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("template with name '%s' already exists", req.Name)
	}

	template, err := s.store.Create(ctx, req)
	if err != nil {
		return nil, err
	}

	// Auto prepull if requested
	if req.AutoPrepull && s.prepullSvc != nil {
		go func() {
			// Start prepull asynchronously
			_, _ = s.prepullSvc.PrepullTemplateImage(context.Background(), req.Name, req.Spec.Image)
		}()
	}

	return template, nil
}

// Get retrieves a template by name
func (s *TemplateService) Get(ctx context.Context, name string) (*model.Template, error) {
	return s.store.Get(ctx, name)
}

// List returns a paginated list of templates
func (s *TemplateService) List(ctx context.Context, opts model.TemplateListOptions) (*model.TemplateListResponse, error) {
	return s.store.List(ctx, opts)
}

// Update updates a template
func (s *TemplateService) Update(ctx context.Context, name string, req *model.UpdateTemplateRequest) (*model.Template, error) {
	// Validate spec
	if err := validateSpec(&req.Spec); err != nil {
		return nil, fmt.Errorf("invalid spec: %w", err)
	}

	return s.store.Update(ctx, name, req)
}

// Delete deletes a template
func (s *TemplateService) Delete(ctx context.Context, name string) error {
	return s.store.Delete(ctx, name)
}

// GetVersion retrieves a specific version of a template
func (s *TemplateService) GetVersion(ctx context.Context, name string, version int) (*model.TemplateVersion, error) {
	return s.store.GetVersionByName(ctx, name, version)
}

// ListVersions lists all versions of a template
func (s *TemplateService) ListVersions(ctx context.Context, name string) (*model.VersionListResponse, error) {
	return s.store.ListVersions(ctx, name)
}

// Rollback rolls back a template to a specific version
func (s *TemplateService) Rollback(ctx context.Context, name string, req *model.RollbackRequest) (*model.RollbackResponse, error) {
	if req.TargetVersion < 1 {
		return nil, fmt.Errorf("target version must be at least 1")
	}
	return s.store.Rollback(ctx, name, req.TargetVersion, req.Changelog)
}

// GetSpecForSandbox retrieves the template spec for creating a sandbox
// If version is 0, it returns the latest version
func (s *TemplateService) GetSpecForSandbox(ctx context.Context, name string, version int) (*model.TemplateSpec, error) {
	template, err := s.store.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if template == nil {
		return nil, fmt.Errorf("template '%s' not found", name)
	}

	if version == 0 {
		version = template.LatestVersion
	}

	ver, err := s.store.GetVersion(ctx, template.ID, version)
	if err != nil {
		return nil, err
	}
	if ver == nil {
		return nil, fmt.Errorf("version %d not found for template '%s'", version, name)
	}

	return &ver.Spec, nil
}

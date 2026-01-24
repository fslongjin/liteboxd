package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/model"
	"gopkg.in/yaml.v3"
)

// ImportExportService handles template import/export operations
type ImportExportService struct {
	templateSvc *TemplateService
	prepullSvc  *PrepullService
}

// NewImportExportService creates a new ImportExportService
func NewImportExportService(templateSvc *TemplateService, prepullSvc *PrepullService) *ImportExportService {
	return &ImportExportService{
		templateSvc: templateSvc,
		prepullSvc:  prepullSvc,
	}
}

// ImportFromYAML imports templates from YAML content
func (s *ImportExportService) ImportFromYAML(ctx context.Context, yamlContent []byte, strategy model.ImportStrategy, autoPrepull bool) (*model.ImportTemplatesResponse, error) {
	var response model.ImportTemplatesResponse
	var prepullImages []string

	// Parse YAML to determine if it's a single template or a list
	var templateList []model.TemplateYAML

	// Try to parse as TemplateList first
	var listYAML model.TemplateListYAML
	if err := yaml.Unmarshal(yamlContent, &listYAML); err == nil && listYAML.Kind == "SandboxTemplateList" {
		// It's a list of templates
		templateList = listYAML.Items
	} else {
		// Try to parse as a single template
		var singleTemplate model.TemplateYAML
		if err := yaml.Unmarshal(yamlContent, &singleTemplate); err != nil {
			return nil, fmt.Errorf("invalid YAML format: %w", err)
		}
		if singleTemplate.Kind != "SandboxTemplate" {
			return nil, fmt.Errorf("invalid kind: expected SandboxTemplate, got %s", singleTemplate.Kind)
		}
		templateList = []model.TemplateYAML{singleTemplate}
	}

	response.Total = len(templateList)

	// Process each template
	for _, tpl := range templateList {
		result := s.processTemplate(ctx, tpl, strategy)
		response.Results = append(response.Results, result)

		switch result.Action {
		case "created":
			response.Created++
			if autoPrepull {
				prepullImages = append(prepullImages, tpl.Spec.Image)
			}
		case "updated":
			response.Updated++
			if autoPrepull {
				prepullImages = append(prepullImages, tpl.Spec.Image)
			}
		case "skipped":
			response.Skipped++
		case "failed":
			response.Failed++
		}
	}

	// Trigger prepull for images
	if len(prepullImages) > 0 && s.prepullSvc != nil {
		response.PrepullStarted = prepullImages
		// Trigger prepull asynchronously
		go func() {
			for _, image := range prepullImages {
				_, _ = s.prepullSvc.Create(context.Background(), &model.CreatePrepullRequest{Image: image}, "")
			}
		}()
	}

	return &response, nil
}

// processTemplate processes a single template import according to the strategy
func (s *ImportExportService) processTemplate(ctx context.Context, tpl model.TemplateYAML, strategy model.ImportStrategy) model.ImportResult {
	result := model.ImportResult{
		Name: tpl.Metadata.Name,
	}

	// Validate template name
	if tpl.Metadata.Name == "" {
		result.Action = "failed"
		result.Error = "template name is required"
		return result
	}

	// Check if template exists
	existing, err := s.templateSvc.Get(ctx, tpl.Metadata.Name)
	exists := err == nil && existing != nil

	// Apply strategy
	switch strategy {
	case model.ImportStrategyCreateOnly:
		if exists {
			result.Action = "skipped"
			return result
		}
		result.Action = s.createOrUpdateTemplate(ctx, tpl, false)

	case model.ImportStrategyUpdateOnly:
		if !exists {
			result.Action = "skipped"
			return result
		}
		result.Action = s.createOrUpdateTemplate(ctx, tpl, true)

	case model.ImportStrategyCreateOrUpdate:
		result.Action = s.createOrUpdateTemplate(ctx, tpl, exists)

	default:
		result.Action = "failed"
		result.Error = fmt.Sprintf("invalid strategy: %s", strategy)
	}

	// Get the version for created/updated templates
	if result.Action == "created" || result.Action == "updated" {
		if updated, err := s.templateSvc.Get(ctx, tpl.Metadata.Name); err == nil {
			result.Version = updated.LatestVersion
		}
	}

	return result
}

// createOrUpdateTemplate creates or updates a template
func (s *ImportExportService) createOrUpdateTemplate(ctx context.Context, tpl model.TemplateYAML, isUpdate bool) string {
	// Build CreateTemplateRequest from YAML
	req := &model.CreateTemplateRequest{
		Name:        tpl.Metadata.Name,
		DisplayName: tpl.Metadata.DisplayName,
		Description: tpl.Metadata.Description,
		Tags:        tpl.Metadata.Tags,
		Spec:        tpl.Spec,
		AutoPrepull: false, // Prepull is handled separately for imports
	}

	if isUpdate {
		// For update, use UpdateTemplateRequest structure
		updateReq := &model.UpdateTemplateRequest{
			DisplayName: tpl.Metadata.DisplayName,
			Description: tpl.Metadata.Description,
			Tags:        tpl.Metadata.Tags,
			Spec:        tpl.Spec,
			Changelog:   "Imported from YAML",
		}

		_, err := s.templateSvc.Update(ctx, tpl.Metadata.Name, updateReq)
		if err != nil {
			return "failed"
		}
		return "updated"
	}

	_, err := s.templateSvc.Create(ctx, req)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return "failed"
		}
		return "failed"
	}

	return "created"
}

// ExportToYAML exports templates to YAML format
func (s *ImportExportService) ExportToYAML(ctx context.Context, name string, version int) ([]byte, error) {
	// Get the template
	template, err := s.templateSvc.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if template == nil {
		return nil, fmt.Errorf("template '%s' not found", name)
	}

	// Get spec
	var spec model.TemplateSpec
	if version > 0 {
		ver, err := s.templateSvc.GetVersion(ctx, name, version)
		if err != nil {
			return nil, err
		}
		if ver == nil {
			return nil, fmt.Errorf("version %d not found", version)
		}
		spec = ver.Spec
	} else {
		spec = *template.Spec
	}

	// Build YAML structure
	tplYAML := model.TemplateYAML{
		APIVersion: "liteboxd/v1",
		Kind:       "SandboxTemplate",
		Metadata: model.TemplateYAMLMetadata{
			Name:        template.Name,
			DisplayName: template.DisplayName,
			Description: template.Description,
			Tags:        template.Tags,
		},
		Spec: spec,
	}

	// Export as YAML
	yamlBytes, err := yaml.Marshal(tplYAML)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal YAML: %w", err)
	}

	return yamlBytes, nil
}

// ExportAllToYAML exports all templates to YAML format
func (s *ImportExportService) ExportAllToYAML(ctx context.Context, tag string, names []string) ([]byte, error) {
	// Get templates
	opts := model.TemplateListOptions{
		Page:     1,
		PageSize: 1000, // Get all templates
	}

	templates, err := s.templateSvc.List(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Filter by tag or names
	var items []model.TemplateYAML
	for _, tpl := range templates.Items {
		// Filter by tag
		if tag != "" {
			hasTag := false
			for _, t := range tpl.Tags {
				if t == tag {
					hasTag = true
					break
				}
			}
			if !hasTag {
				continue
			}
		}

		// Filter by names
		if len(names) > 0 {
			found := false
			for _, n := range names {
				if tpl.Name == n {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Get latest version spec
		version, err := s.templateSvc.GetVersion(ctx, tpl.Name, tpl.LatestVersion)
		if err != nil {
			continue
		}
		if version == nil {
			continue
		}

		items = append(items, model.TemplateYAML{
			APIVersion: "liteboxd/v1",
			Kind:       "SandboxTemplate",
			Metadata: model.TemplateYAMLMetadata{
				Name:        tpl.Name,
				DisplayName: tpl.DisplayName,
				Description: tpl.Description,
				Tags:        tpl.Tags,
			},
			Spec: version.Spec,
		})
	}

	// Build list YAML
	listYAML := model.TemplateListYAML{
		APIVersion: "liteboxd/v1",
		Kind:       "SandboxTemplateList",
		ExportedAt: time.Now().Format(time.RFC3339),
		Items:      items,
	}

	// Export as YAML
	yamlBytes, err := yaml.Marshal(listYAML)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal YAML: %w", err)
	}

	return yamlBytes, nil
}

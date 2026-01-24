package liteboxd

import (
	"context"
	"strconv"
)

// TemplateService handles template operations.
type TemplateService struct {
	client *Client
}

// Create creates a new template.
func (t *TemplateService) Create(ctx context.Context, req *CreateTemplateRequest) (*Template, error) {
	var result Template
	err := t.client.doJSON(ctx, "POST", t.client.buildPath("templates"), req, &result, nil)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// List retrieves templates with optional filtering.
func (t *TemplateService) List(ctx context.Context, opts *TemplateListOptions) (*TemplateListResponse, error) {
	queryParams := make(map[string]string)
	if opts != nil {
		if opts.Tag != "" {
			queryParams["tag"] = opts.Tag
		}
		if opts.Search != "" {
			queryParams["search"] = opts.Search
		}
		if opts.Page > 0 {
			queryParams["page"] = strconv.Itoa(opts.Page)
		}
		if opts.PageSize > 0 {
			queryParams["pageSize"] = strconv.Itoa(opts.PageSize)
		}
	}
	var result TemplateListResponse
	err := t.client.doJSON(ctx, "GET", t.client.buildPath("templates"), nil, &result, queryParams)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Get retrieves a specific template by name.
func (t *TemplateService) Get(ctx context.Context, name string) (*Template, error) {
	var result Template
	err := t.client.doJSON(ctx, "GET", t.client.buildPath("templates", name), nil, &result, nil)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Update updates a template (creates new version).
func (t *TemplateService) Update(ctx context.Context, name string, req *UpdateTemplateRequest) (*Template, error) {
	var result Template
	err := t.client.doJSON(ctx, "PUT", t.client.buildPath("templates", name), req, &result, nil)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Delete removes a template.
func (t *TemplateService) Delete(ctx context.Context, name string) error {
	return t.client.doEmptyResponse(ctx, "DELETE", t.client.buildPath("templates", name), nil, nil)
}

// ListVersions retrieves all versions of a template.
func (t *TemplateService) ListVersions(ctx context.Context, name string) (*VersionListResponse, error) {
	var result VersionListResponse
	err := t.client.doJSON(ctx, "GET", t.client.buildPath("templates", name, "versions"), nil, &result, nil)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetVersion retrieves a specific version of a template.
func (t *TemplateService) GetVersion(ctx context.Context, name string, version int) (*TemplateVersion, error) {
	var result TemplateVersion
	err := t.client.doJSON(ctx, "GET", t.client.buildPath("templates", name, "versions", strconv.Itoa(version)), nil, &result, nil)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Rollback rolls back a template to a previous version.
func (t *TemplateService) Rollback(ctx context.Context, name string, targetVersion int, changelog string) (*RollbackResponse, error) {
	req := &RollbackRequest{
		TargetVersion: targetVersion,
		Changelog:     changelog,
	}
	var result RollbackResponse
	err := t.client.doJSON(ctx, "POST", t.client.buildPath("templates", name, "rollback"), req, &result, nil)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ExportYAML exports a template to YAML format.
func (t *TemplateService) ExportYAML(ctx context.Context, name string, version int) ([]byte, error) {
	queryParams := make(map[string]string)
	if version > 0 {
		queryParams["version"] = strconv.Itoa(version)
	}
	return t.client.doText(ctx, "GET", t.client.buildPath("templates", name, "export"), queryParams)
}

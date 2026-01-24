package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/model"
	"github.com/google/uuid"
)

// TemplateStore handles template data persistence
type TemplateStore struct {
	db *sql.DB
}

// NewTemplateStore creates a new TemplateStore
func NewTemplateStore() *TemplateStore {
	return &TemplateStore{db: DB}
}

// generateID generates a unique ID with prefix
func generateID(prefix string) string {
	return prefix + "-" + uuid.New().String()[:8]
}

// Create creates a new template with its first version
func (s *TemplateStore) Create(ctx context.Context, req *model.CreateTemplateRequest) (*model.Template, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now()
	templateID := generateID("tpl")
	versionID := generateID("ver")

	// Apply defaults to spec
	req.Spec.ApplyDefaults()

	// Determine isPublic value
	isPublic := true
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}

	// Create template
	template := &model.Template{
		ID:            templateID,
		Name:          req.Name,
		DisplayName:   req.DisplayName,
		Description:   req.Description,
		Tags:          req.Tags,
		Author:        "",
		IsPublic:      isPublic,
		LatestVersion: 1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO templates (id, name, display_name, description, tags, author, is_public, latest_version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, template.ID, template.Name, template.DisplayName, template.Description,
		template.MarshalTags(), template.Author, template.IsPublic,
		template.LatestVersion, template.CreatedAt, template.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, fmt.Errorf("template with name '%s' already exists", req.Name)
		}
		return nil, fmt.Errorf("failed to insert template: %w", err)
	}

	// Create first version
	version := &model.TemplateVersion{
		ID:         versionID,
		TemplateID: templateID,
		Version:    1,
		Spec:       req.Spec,
		Changelog:  "Initial version",
		CreatedBy:  "",
		CreatedAt:  now,
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO template_versions (id, template_id, version, spec, changelog, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, version.ID, version.TemplateID, version.Version,
		version.MarshalSpec(), version.Changelog, version.CreatedBy, version.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert template version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	template.Spec = &req.Spec
	return template, nil
}

// Get retrieves a template by name
func (s *TemplateStore) Get(ctx context.Context, name string) (*model.Template, error) {
	template := &model.Template{}
	var tagsJSON string

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, display_name, description, tags, author, is_public, latest_version, created_at, updated_at
		FROM templates WHERE name = ?
	`, name).Scan(
		&template.ID, &template.Name, &template.DisplayName, &template.Description,
		&tagsJSON, &template.Author, &template.IsPublic,
		&template.LatestVersion, &template.CreatedAt, &template.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query template: %w", err)
	}

	if err := template.UnmarshalTags(tagsJSON); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
	}

	// Get latest version spec
	version, err := s.GetVersion(ctx, template.ID, template.LatestVersion)
	if err != nil {
		return nil, err
	}
	if version != nil {
		template.Spec = &version.Spec
	}

	return template, nil
}

// GetByID retrieves a template by ID
func (s *TemplateStore) GetByID(ctx context.Context, id string) (*model.Template, error) {
	template := &model.Template{}
	var tagsJSON string

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, display_name, description, tags, author, is_public, latest_version, created_at, updated_at
		FROM templates WHERE id = ?
	`, id).Scan(
		&template.ID, &template.Name, &template.DisplayName, &template.Description,
		&tagsJSON, &template.Author, &template.IsPublic,
		&template.LatestVersion, &template.CreatedAt, &template.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query template: %w", err)
	}

	if err := template.UnmarshalTags(tagsJSON); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
	}

	return template, nil
}

// List returns a paginated list of templates
func (s *TemplateStore) List(ctx context.Context, opts model.TemplateListOptions) (*model.TemplateListResponse, error) {
	// Apply defaults
	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.PageSize < 1 || opts.PageSize > 100 {
		opts.PageSize = 20
	}

	// Build query
	var conditions []string
	var args []interface{}

	if opts.Tag != "" {
		conditions = append(conditions, "tags LIKE ?")
		args = append(args, "%\""+opts.Tag+"\"%")
	}
	if opts.Search != "" {
		conditions = append(conditions, "(name LIKE ? OR display_name LIKE ? OR description LIKE ?)")
		searchPattern := "%" + opts.Search + "%"
		args = append(args, searchPattern, searchPattern, searchPattern)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total
	var total int
	countQuery := "SELECT COUNT(*) FROM templates " + whereClause
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to count templates: %w", err)
	}

	// Query items
	offset := (opts.Page - 1) * opts.PageSize
	query := fmt.Sprintf(`
		SELECT id, name, display_name, description, tags, author, is_public, latest_version, created_at, updated_at
		FROM templates %s
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, whereClause)
	args = append(args, opts.PageSize, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query templates: %w", err)
	}
	defer rows.Close()

	var items []model.Template
	for rows.Next() {
		var t model.Template
		var tagsJSON string
		if err := rows.Scan(
			&t.ID, &t.Name, &t.DisplayName, &t.Description,
			&tagsJSON, &t.Author, &t.IsPublic,
			&t.LatestVersion, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan template: %w", err)
		}
		if err := t.UnmarshalTags(tagsJSON); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}

		// Load spec for each template
		version, err := s.GetVersion(ctx, t.ID, t.LatestVersion)
		if err == nil && version != nil {
			t.Spec = &version.Spec
		}

		items = append(items, t)
	}

	if items == nil {
		items = []model.Template{}
	}

	return &model.TemplateListResponse{
		Items:    items,
		Total:    total,
		Page:     opts.Page,
		PageSize: opts.PageSize,
	}, nil
}

// Update updates a template and creates a new version
func (s *TemplateStore) Update(ctx context.Context, name string, req *model.UpdateTemplateRequest) (*model.Template, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get current template
	template, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if template == nil {
		return nil, nil
	}

	now := time.Now()
	newVersion := template.LatestVersion + 1
	versionID := generateID("ver")

	// Apply defaults to spec
	req.Spec.ApplyDefaults()

	// Update template metadata
	if req.DisplayName != "" {
		template.DisplayName = req.DisplayName
	}
	if req.Description != "" {
		template.Description = req.Description
	}
	if req.Tags != nil {
		template.Tags = req.Tags
	}
	if req.IsPublic != nil {
		template.IsPublic = *req.IsPublic
	}
	template.LatestVersion = newVersion
	template.UpdatedAt = now

	_, err = tx.ExecContext(ctx, `
		UPDATE templates
		SET display_name = ?, description = ?, tags = ?, is_public = ?, latest_version = ?, updated_at = ?
		WHERE id = ?
	`, template.DisplayName, template.Description, template.MarshalTags(),
		template.IsPublic, template.LatestVersion, template.UpdatedAt, template.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update template: %w", err)
	}

	// Create new version
	version := &model.TemplateVersion{
		ID:         versionID,
		TemplateID: template.ID,
		Version:    newVersion,
		Spec:       req.Spec,
		Changelog:  req.Changelog,
		CreatedBy:  "",
		CreatedAt:  now,
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO template_versions (id, template_id, version, spec, changelog, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, version.ID, version.TemplateID, version.Version,
		version.MarshalSpec(), version.Changelog, version.CreatedBy, version.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert template version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	template.Spec = &req.Spec
	return template, nil
}

// Delete deletes a template and all its versions
func (s *TemplateStore) Delete(ctx context.Context, name string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM templates WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("failed to delete template: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("template not found")
	}

	return nil
}

// GetVersion retrieves a specific version of a template
func (s *TemplateStore) GetVersion(ctx context.Context, templateID string, version int) (*model.TemplateVersion, error) {
	v := &model.TemplateVersion{}
	var specJSON string

	err := s.db.QueryRowContext(ctx, `
		SELECT id, template_id, version, spec, changelog, created_by, created_at
		FROM template_versions
		WHERE template_id = ? AND version = ?
	`, templateID, version).Scan(
		&v.ID, &v.TemplateID, &v.Version, &specJSON, &v.Changelog, &v.CreatedBy, &v.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query version: %w", err)
	}

	if err := v.UnmarshalSpec(specJSON); err != nil {
		return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
	}

	return v, nil
}

// GetVersionByName retrieves a specific version by template name
func (s *TemplateStore) GetVersionByName(ctx context.Context, name string, version int) (*model.TemplateVersion, error) {
	template, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if template == nil {
		return nil, nil
	}

	return s.GetVersion(ctx, template.ID, version)
}

// ListVersions lists all versions of a template
func (s *TemplateStore) ListVersions(ctx context.Context, name string) (*model.VersionListResponse, error) {
	template, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if template == nil {
		return nil, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, template_id, version, spec, changelog, created_by, created_at
		FROM template_versions
		WHERE template_id = ?
		ORDER BY version DESC
	`, template.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to query versions: %w", err)
	}
	defer rows.Close()

	var items []model.TemplateVersion
	for rows.Next() {
		var v model.TemplateVersion
		var specJSON string
		if err := rows.Scan(
			&v.ID, &v.TemplateID, &v.Version, &specJSON, &v.Changelog, &v.CreatedBy, &v.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan version: %w", err)
		}
		if err := v.UnmarshalSpec(specJSON); err != nil {
			return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
		}
		items = append(items, v)
	}

	if items == nil {
		items = []model.TemplateVersion{}
	}

	return &model.VersionListResponse{
		Items: items,
		Total: len(items),
	}, nil
}

// Rollback rolls back a template to a specific version
func (s *TemplateStore) Rollback(ctx context.Context, name string, targetVersion int, changelog string) (*model.RollbackResponse, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get current template
	template, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if template == nil {
		return nil, fmt.Errorf("template not found")
	}

	// Get target version
	targetVer, err := s.GetVersion(ctx, template.ID, targetVersion)
	if err != nil {
		return nil, err
	}
	if targetVer == nil {
		return nil, fmt.Errorf("version %d not found", targetVersion)
	}

	now := time.Now()
	previousVersion := template.LatestVersion
	newVersion := previousVersion + 1
	versionID := generateID("ver")

	// Update template
	template.LatestVersion = newVersion
	template.UpdatedAt = now

	_, err = tx.ExecContext(ctx, `
		UPDATE templates SET latest_version = ?, updated_at = ? WHERE id = ?
	`, template.LatestVersion, template.UpdatedAt, template.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update template: %w", err)
	}

	// Create new version with rolled back spec
	if changelog == "" {
		changelog = fmt.Sprintf("Rolled back to version %d", targetVersion)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO template_versions (id, template_id, version, spec, changelog, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, versionID, template.ID, newVersion, targetVer.MarshalSpec(), changelog, "", now)
	if err != nil {
		return nil, fmt.Errorf("failed to insert rollback version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &model.RollbackResponse{
		ID:             template.ID,
		Name:           template.Name,
		LatestVersion:  newVersion,
		RolledBackFrom: previousVersion,
		RolledBackTo:   targetVersion,
		UpdatedAt:      now,
	}, nil
}

// Exists checks if a template with the given name exists
func (s *TemplateStore) Exists(ctx context.Context, name string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM templates WHERE name = ?", name).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check template existence: %w", err)
	}
	return count > 0, nil
}

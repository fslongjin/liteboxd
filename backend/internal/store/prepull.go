package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/model"
	"github.com/google/uuid"
)

// PrepullStore handles image prepull data persistence
type PrepullStore struct {
	db *sql.DB
}

// NewPrepullStore creates a new PrepullStore
func NewPrepullStore() *PrepullStore {
	return &PrepullStore{db: DB}
}

// Create creates a new prepull record
func (s *PrepullStore) Create(ctx context.Context, image, imageHash, template string) (*model.ImagePrepull, error) {
	id := "pp-" + uuid.New().String()[:8]
	now := time.Now()

	prepull := &model.ImagePrepull{
		ID:         id,
		Image:      image,
		ImageHash:  imageHash,
		Status:     model.PrepullStatusPending,
		ReadyNodes: 0,
		TotalNodes: 0,
		Template:   template,
		StartedAt:  now,
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO image_prepulls (id, image, image_hash, status, ready_nodes, total_nodes, template, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, prepull.ID, prepull.Image, prepull.ImageHash, prepull.Status,
		prepull.ReadyNodes, prepull.TotalNodes, prepull.Template, prepull.StartedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create prepull record: %w", err)
	}

	return prepull, nil
}

// Get retrieves a prepull record by ID
func (s *PrepullStore) Get(ctx context.Context, id string) (*model.ImagePrepull, error) {
	prepull := &model.ImagePrepull{}
	var completedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, image, image_hash, status, ready_nodes, total_nodes, error, template, started_at, completed_at
		FROM image_prepulls WHERE id = ?
	`, id).Scan(
		&prepull.ID, &prepull.Image, &prepull.ImageHash, &prepull.Status,
		&prepull.ReadyNodes, &prepull.TotalNodes, &prepull.Error, &prepull.Template,
		&prepull.StartedAt, &completedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get prepull: %w", err)
	}

	if completedAt.Valid {
		prepull.CompletedAt = &completedAt.Time
	}

	return prepull, nil
}

// GetByImage retrieves the latest prepull record for an image
func (s *PrepullStore) GetByImage(ctx context.Context, image string) (*model.ImagePrepull, error) {
	prepull := &model.ImagePrepull{}
	var completedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, image, image_hash, status, ready_nodes, total_nodes, error, template, started_at, completed_at
		FROM image_prepulls WHERE image = ?
		ORDER BY started_at DESC LIMIT 1
	`, image).Scan(
		&prepull.ID, &prepull.Image, &prepull.ImageHash, &prepull.Status,
		&prepull.ReadyNodes, &prepull.TotalNodes, &prepull.Error, &prepull.Template,
		&prepull.StartedAt, &completedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get prepull by image: %w", err)
	}

	if completedAt.Valid {
		prepull.CompletedAt = &completedAt.Time
	}

	return prepull, nil
}

// List returns all prepull records, optionally filtered by status
func (s *PrepullStore) List(ctx context.Context, image, status string) ([]model.ImagePrepull, error) {
	query := "SELECT id, image, image_hash, status, ready_nodes, total_nodes, error, template, started_at, completed_at FROM image_prepulls"
	var conditions []string
	var args []interface{}

	if image != "" {
		conditions = append(conditions, "image = ?")
		args = append(args, image)
	}
	if status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, status)
	}

	if len(conditions) > 0 {
		query += " WHERE "
		for i, cond := range conditions {
			if i > 0 {
				query += " AND "
			}
			query += cond
		}
	}

	query += " ORDER BY started_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list prepulls: %w", err)
	}
	defer rows.Close()

	var items []model.ImagePrepull
	for rows.Next() {
		var p model.ImagePrepull
		var completedAt sql.NullTime
		if err := rows.Scan(
			&p.ID, &p.Image, &p.ImageHash, &p.Status,
			&p.ReadyNodes, &p.TotalNodes, &p.Error, &p.Template,
			&p.StartedAt, &completedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan prepull: %w", err)
		}
		if completedAt.Valid {
			p.CompletedAt = &completedAt.Time
		}
		items = append(items, p)
	}

	if items == nil {
		items = []model.ImagePrepull{}
	}

	return items, nil
}

// UpdateStatus updates the status of a prepull record
func (s *PrepullStore) UpdateStatus(ctx context.Context, id string, status model.PrepullStatus, readyNodes, totalNodes int, errMsg string) error {
	var completedAt interface{}
	if status == model.PrepullStatusCompleted || status == model.PrepullStatusFailed {
		now := time.Now()
		completedAt = now
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE image_prepulls
		SET status = ?, ready_nodes = ?, total_nodes = ?, error = ?, completed_at = ?
		WHERE id = ?
	`, status, readyNodes, totalNodes, errMsg, completedAt, id)
	if err != nil {
		return fmt.Errorf("failed to update prepull status: %w", err)
	}

	return nil
}

// Delete removes a prepull record
func (s *PrepullStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM image_prepulls WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete prepull: %w", err)
	}
	return nil
}

// GetActiveByImage checks if there's an active (pending or pulling) prepull for the image
func (s *PrepullStore) GetActiveByImage(ctx context.Context, image string) (*model.ImagePrepull, error) {
	prepull := &model.ImagePrepull{}
	var completedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, image, image_hash, status, ready_nodes, total_nodes, error, template, started_at, completed_at
		FROM image_prepulls
		WHERE image = ? AND status IN (?, ?)
		ORDER BY started_at DESC LIMIT 1
	`, image, model.PrepullStatusPending, model.PrepullStatusPulling).Scan(
		&prepull.ID, &prepull.Image, &prepull.ImageHash, &prepull.Status,
		&prepull.ReadyNodes, &prepull.TotalNodes, &prepull.Error, &prepull.Template,
		&prepull.StartedAt, &completedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get active prepull: %w", err)
	}

	if completedAt.Valid {
		prepull.CompletedAt = &completedAt.Time
	}

	return prepull, nil
}

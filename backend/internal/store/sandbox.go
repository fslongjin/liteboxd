package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	DesiredStateActive  = "active"
	DesiredStateDeleted = "deleted"
)

// SandboxRecord persists sandbox metadata as control-plane source of truth.
type SandboxRecord struct {
	ID                    string
	TemplateName          string
	TemplateVersion       int
	Image                 string
	CPU                   string
	Memory                string
	TTL                   int
	EnvJSON               string
	DesiredState          string
	LifecycleStatus       string
	StatusReason          string
	ClusterNamespace      string
	PodName               string
	PodUID                string
	PodPhase              string
	PodIP                 string
	LastSeenAt            *time.Time
	AccessTokenCiphertext string
	AccessTokenNonce      string
	AccessTokenKeyID      string
	AccessTokenSHA256     string
	AccessURL             string
	CreatedAt             time.Time
	ExpiresAt             time.Time
	UpdatedAt             time.Time
	DeletedAt             *time.Time
}

func (r *SandboxRecord) EnvMap() map[string]string {
	var env map[string]string
	if r.EnvJSON == "" {
		return map[string]string{}
	}
	if err := json.Unmarshal([]byte(r.EnvJSON), &env); err != nil {
		return map[string]string{}
	}
	if env == nil {
		return map[string]string{}
	}
	return env
}

// ReconcileRunRecord stores one reconcile run.
type ReconcileRunRecord struct {
	ID          string
	TriggerType string
	StartedAt   time.Time
	FinishedAt  *time.Time
	TotalDB     int
	TotalK8s    int
	DriftCount  int
	FixedCount  int
	Status      string
	Error       string
}

// ReconcileItemRecord stores one drift item found in a reconcile run.
type ReconcileItemRecord struct {
	ID        int64
	RunID     string
	SandboxID string
	DriftType string
	Action    string
	Detail    string
	CreatedAt time.Time
}

type SandboxMetadataQuery struct {
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

type SandboxStatusHistoryRecord struct {
	ID         int64
	SandboxID  string
	Source     string
	FromStatus string
	ToStatus   string
	Reason     string
	PayloadRaw string
	CreatedAt  time.Time
}

// SandboxStore handles sandbox metadata persistence.
type SandboxStore struct {
	db *sql.DB
}

func NewSandboxStore() *SandboxStore {
	return &SandboxStore{db: DB}
}

func (s *SandboxStore) Create(ctx context.Context, rec *SandboxRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sandboxes (
			id, template_name, template_version, image, cpu, memory, ttl, env_json,
			desired_state, lifecycle_status, status_reason,
			cluster_namespace, pod_name, pod_uid, pod_phase, pod_ip, last_seen_at,
			access_token_ciphertext, access_token_nonce, access_token_key_id, access_token_sha256, access_url,
			created_at, expires_at, updated_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, rec.ID, rec.TemplateName, rec.TemplateVersion, rec.Image, rec.CPU, rec.Memory, rec.TTL, rec.EnvJSON,
		rec.DesiredState, rec.LifecycleStatus, rec.StatusReason,
		rec.ClusterNamespace, rec.PodName, rec.PodUID, rec.PodPhase, rec.PodIP, toNullTime(rec.LastSeenAt),
		rec.AccessTokenCiphertext, rec.AccessTokenNonce, rec.AccessTokenKeyID, rec.AccessTokenSHA256, rec.AccessURL,
		rec.CreatedAt, rec.ExpiresAt, rec.UpdatedAt, toNullTime(rec.DeletedAt),
	)
	if err != nil {
		return fmt.Errorf("failed to create sandbox record: %w", err)
	}
	return nil
}

func (s *SandboxStore) GetByID(ctx context.Context, id string) (*SandboxRecord, error) {
	row := s.db.QueryRowContext(ctx, sandboxSelectSQL+` WHERE id = ?`, id)
	rec, err := scanSandbox(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get sandbox by id: %w", err)
	}
	return rec, nil
}

func (s *SandboxStore) ListActive(ctx context.Context) ([]SandboxRecord, error) {
	rows, err := s.db.QueryContext(ctx, sandboxSelectSQL+`
		 WHERE desired_state = ? AND lifecycle_status <> ?
		 ORDER BY created_at DESC
	`, DesiredStateActive, "deleted")
	if err != nil {
		return nil, fmt.Errorf("failed to list active sandboxes: %w", err)
	}
	defer rows.Close()
	return scanSandboxRows(rows)
}

func (s *SandboxStore) ListForReconcile(ctx context.Context) ([]SandboxRecord, error) {
	rows, err := s.db.QueryContext(ctx, sandboxSelectSQL+`
		 ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list sandboxes for reconcile: %w", err)
	}
	defer rows.Close()
	return scanSandboxRows(rows)
}

func (s *SandboxStore) ListMetadata(ctx context.Context, query SandboxMetadataQuery) ([]SandboxRecord, int, error) {
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}

	var where []string
	var args []any
	if query.ID != "" {
		where = append(where, "id LIKE ?")
		args = append(args, query.ID+"%")
	}
	if query.Template != "" {
		where = append(where, "template_name = ?")
		args = append(args, query.Template)
	}
	if query.DesiredState != "" {
		where = append(where, "desired_state = ?")
		args = append(args, query.DesiredState)
	}
	if query.LifecycleStatus != "" {
		where = append(where, "lifecycle_status = ?")
		args = append(args, query.LifecycleStatus)
	}
	if query.CreatedFrom != nil {
		where = append(where, "created_at >= ?")
		args = append(args, *query.CreatedFrom)
	}
	if query.CreatedTo != nil {
		where = append(where, "created_at <= ?")
		args = append(args, *query.CreatedTo)
	}
	if query.DeletedFrom != nil {
		where = append(where, "deleted_at IS NOT NULL", "deleted_at >= ?")
		args = append(args, *query.DeletedFrom)
	}
	if query.DeletedTo != nil {
		where = append(where, "deleted_at IS NOT NULL", "deleted_at <= ?")
		args = append(args, *query.DeletedTo)
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	countSQL := "SELECT COUNT(1) FROM sandboxes" + whereSQL
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count metadata sandboxes: %w", err)
	}

	offset := (query.Page - 1) * query.PageSize
	listSQL := sandboxSelectSQL + whereSQL + " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	listArgs := append(append([]any{}, args...), query.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list metadata sandboxes: %w", err)
	}
	defer rows.Close()

	items, err := scanSandboxRows(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *SandboxStore) ListExpiredActive(ctx context.Context, now time.Time) ([]SandboxRecord, error) {
	rows, err := s.db.QueryContext(ctx, sandboxSelectSQL+`
		 WHERE desired_state = ?
		   AND expires_at <= ?
		   AND lifecycle_status NOT IN (?, ?)
		 ORDER BY expires_at ASC
	`, DesiredStateActive, now, "deleted", "terminating")
	if err != nil {
		return nil, fmt.Errorf("failed to list expired sandboxes: %w", err)
	}
	defer rows.Close()
	return scanSandboxRows(rows)
}

func (s *SandboxStore) SetDesiredDeleted(ctx context.Context, id string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sandboxes
		SET desired_state = ?, lifecycle_status = ?, updated_at = ?
		WHERE id = ?
	`, DesiredStateDeleted, "terminating", now, id)
	if err != nil {
		return fmt.Errorf("failed to set desired deleted: %w", err)
	}
	return nil
}

func (s *SandboxStore) MarkDeleted(ctx context.Context, id, reason string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sandboxes
		SET desired_state = ?, lifecycle_status = ?, status_reason = ?, deleted_at = ?, updated_at = ?
		WHERE id = ?
	`, DesiredStateDeleted, "deleted", reason, now, now, id)
	if err != nil {
		return fmt.Errorf("failed to mark deleted: %w", err)
	}
	return nil
}

func (s *SandboxStore) UpdateStatus(ctx context.Context, id, status, reason string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sandboxes
		SET lifecycle_status = ?, status_reason = ?, updated_at = ?
		WHERE id = ?
	`, status, reason, now, id)
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}
	return nil
}

func (s *SandboxStore) UpdateObservedState(ctx context.Context, id, podUID, podPhase, podIP, lifecycleStatus, reason string, lastSeen, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sandboxes
		SET pod_uid = ?, pod_phase = ?, pod_ip = ?, lifecycle_status = ?, status_reason = ?, last_seen_at = ?, updated_at = ?
		WHERE id = ?
	`, podUID, podPhase, podIP, lifecycleStatus, reason, lastSeen, now, id)
	if err != nil {
		return fmt.Errorf("failed to update observed state: %w", err)
	}
	return nil
}

func (s *SandboxStore) AppendStatusHistory(ctx context.Context, sandboxID, source, fromStatus, toStatus, reason string, payload any, now time.Time) error {
	payloadJSON := "{}"
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal status history payload: %w", err)
		}
		payloadJSON = string(b)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sandbox_status_history (sandbox_id, source, from_status, to_status, reason, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sandboxID, source, fromStatus, toStatus, reason, payloadJSON, now)
	if err != nil {
		return fmt.Errorf("failed to append status history: %w", err)
	}
	return nil
}

func (s *SandboxStore) CreateReconcileRun(ctx context.Context, run *ReconcileRunRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sandbox_reconcile_runs (id, trigger_type, started_at, finished_at, total_db, total_k8s, drift_count, fixed_count, status, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, run.ID, run.TriggerType, run.StartedAt, toNullTime(run.FinishedAt), run.TotalDB, run.TotalK8s, run.DriftCount, run.FixedCount, run.Status, run.Error)
	if err != nil {
		return fmt.Errorf("failed to create reconcile run: %w", err)
	}
	return nil
}

func (s *SandboxStore) FinishReconcileRun(ctx context.Context, id, status, errMsg string, totalDB, totalK8s, drift, fixed int, finishedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sandbox_reconcile_runs
		SET finished_at = ?, total_db = ?, total_k8s = ?, drift_count = ?, fixed_count = ?, status = ?, error = ?
		WHERE id = ?
	`, finishedAt, totalDB, totalK8s, drift, fixed, status, errMsg, id)
	if err != nil {
		return fmt.Errorf("failed to finish reconcile run: %w", err)
	}
	return nil
}

func (s *SandboxStore) AddReconcileItem(ctx context.Context, item *ReconcileItemRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sandbox_reconcile_items (run_id, sandbox_id, drift_type, action, detail, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, item.RunID, item.SandboxID, item.DriftType, item.Action, item.Detail, item.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to add reconcile item: %w", err)
	}
	return nil
}

func (s *SandboxStore) ListReconcileRuns(ctx context.Context, limit int) ([]ReconcileRunRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, trigger_type, started_at, finished_at, total_db, total_k8s, drift_count, fixed_count, status, error
		FROM sandbox_reconcile_runs
		ORDER BY started_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list reconcile runs: %w", err)
	}
	defer rows.Close()

	var items []ReconcileRunRecord
	for rows.Next() {
		var r ReconcileRunRecord
		var finishedAt sql.NullTime
		if err := rows.Scan(&r.ID, &r.TriggerType, &r.StartedAt, &finishedAt, &r.TotalDB, &r.TotalK8s, &r.DriftCount, &r.FixedCount, &r.Status, &r.Error); err != nil {
			return nil, fmt.Errorf("failed to scan reconcile run: %w", err)
		}
		if finishedAt.Valid {
			t := finishedAt.Time
			r.FinishedAt = &t
		}
		items = append(items, r)
	}
	if items == nil {
		items = []ReconcileRunRecord{}
	}
	return items, nil
}

func (s *SandboxStore) GetReconcileRun(ctx context.Context, id string) (*ReconcileRunRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, trigger_type, started_at, finished_at, total_db, total_k8s, drift_count, fixed_count, status, error
		FROM sandbox_reconcile_runs
		WHERE id = ?
	`, id)

	var r ReconcileRunRecord
	var finishedAt sql.NullTime
	if err := row.Scan(&r.ID, &r.TriggerType, &r.StartedAt, &finishedAt, &r.TotalDB, &r.TotalK8s, &r.DriftCount, &r.FixedCount, &r.Status, &r.Error); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get reconcile run: %w", err)
	}
	if finishedAt.Valid {
		t := finishedAt.Time
		r.FinishedAt = &t
	}
	return &r, nil
}

func (s *SandboxStore) ListReconcileItems(ctx context.Context, runID string) ([]ReconcileItemRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_id, sandbox_id, drift_type, action, detail, created_at
		FROM sandbox_reconcile_items
		WHERE run_id = ?
		ORDER BY id ASC
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to list reconcile items: %w", err)
	}
	defer rows.Close()

	var items []ReconcileItemRecord
	for rows.Next() {
		var item ReconcileItemRecord
		if err := rows.Scan(&item.ID, &item.RunID, &item.SandboxID, &item.DriftType, &item.Action, &item.Detail, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan reconcile item: %w", err)
		}
		items = append(items, item)
	}
	if items == nil {
		items = []ReconcileItemRecord{}
	}
	return items, nil
}

func (s *SandboxStore) ListStatusHistory(ctx context.Context, sandboxID string, limit int, beforeID int64) ([]SandboxStatusHistoryRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	baseSQL := `
		SELECT id, sandbox_id, source, from_status, to_status, reason, payload_json, created_at
		FROM sandbox_status_history
		WHERE sandbox_id = ?`
	args := []any{sandboxID}
	if beforeID > 0 {
		baseSQL += " AND id < ?"
		args = append(args, beforeID)
	}
	baseSQL += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, baseSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list sandbox status history: %w", err)
	}
	defer rows.Close()

	var items []SandboxStatusHistoryRecord
	for rows.Next() {
		var item SandboxStatusHistoryRecord
		if err := rows.Scan(&item.ID, &item.SandboxID, &item.Source, &item.FromStatus, &item.ToStatus, &item.Reason, &item.PayloadRaw, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan sandbox status history: %w", err)
		}
		items = append(items, item)
	}
	if items == nil {
		items = []SandboxStatusHistoryRecord{}
	}
	return items, nil
}

// PurgeHistoryResult contains deletion stats from history cleanup.
type PurgeHistoryResult struct {
	DeletedSandboxes      int64
	DeletedStatusHistory  int64
	DeletedReconcileRuns  int64
	DeletedReconcileItems int64
}

// PurgeHistoricalData deletes historical records older than cutoff.
// Retention uses one cutoff for all lifecycle-related tables.
func (s *SandboxStore) PurgeHistoricalData(ctx context.Context, cutoff time.Time) (*PurgeHistoryResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin purge transaction: %w", err)
	}
	defer tx.Rollback()

	result := &PurgeHistoryResult{}

	// 1) Purge reconcile items first (also cascaded by reconcile runs, but explicit delete keeps stats clear).
	res, err := tx.ExecContext(ctx, `
		DELETE FROM sandbox_reconcile_items
		WHERE created_at < ?
	`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to purge reconcile items: %w", err)
	}
	result.DeletedReconcileItems, _ = res.RowsAffected()

	// 2) Purge reconcile runs.
	res, err = tx.ExecContext(ctx, `
		DELETE FROM sandbox_reconcile_runs
		WHERE started_at < ?
	`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to purge reconcile runs: %w", err)
	}
	result.DeletedReconcileRuns, _ = res.RowsAffected()

	// 3) Purge status history.
	res, err = tx.ExecContext(ctx, `
		DELETE FROM sandbox_status_history
		WHERE created_at < ?
	`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to purge status history: %w", err)
	}
	result.DeletedStatusHistory, _ = res.RowsAffected()

	// 4) Purge deleted sandboxes only.
	res, err = tx.ExecContext(ctx, `
		DELETE FROM sandboxes
		WHERE lifecycle_status = 'deleted'
		  AND deleted_at IS NOT NULL
		  AND deleted_at < ?
	`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to purge deleted sandboxes: %w", err)
	}
	result.DeletedSandboxes, _ = res.RowsAffected()

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit purge transaction: %w", err)
	}
	return result, nil
}

const sandboxSelectSQL = `
SELECT
	id, template_name, template_version, image, cpu, memory, ttl, env_json,
	desired_state, lifecycle_status, status_reason,
	cluster_namespace, pod_name, pod_uid, pod_phase, pod_ip, last_seen_at,
	access_token_ciphertext, access_token_nonce, access_token_key_id, access_token_sha256, access_url,
	created_at, expires_at, updated_at, deleted_at
FROM sandboxes`

func scanSandbox(row interface{ Scan(dest ...any) error }) (*SandboxRecord, error) {
	var rec SandboxRecord
	var lastSeenAt sql.NullTime
	var deletedAt sql.NullTime
	if err := row.Scan(
		&rec.ID, &rec.TemplateName, &rec.TemplateVersion, &rec.Image, &rec.CPU, &rec.Memory, &rec.TTL, &rec.EnvJSON,
		&rec.DesiredState, &rec.LifecycleStatus, &rec.StatusReason,
		&rec.ClusterNamespace, &rec.PodName, &rec.PodUID, &rec.PodPhase, &rec.PodIP, &lastSeenAt,
		&rec.AccessTokenCiphertext, &rec.AccessTokenNonce, &rec.AccessTokenKeyID, &rec.AccessTokenSHA256, &rec.AccessURL,
		&rec.CreatedAt, &rec.ExpiresAt, &rec.UpdatedAt, &deletedAt,
	); err != nil {
		return nil, err
	}
	if lastSeenAt.Valid {
		t := lastSeenAt.Time
		rec.LastSeenAt = &t
	}
	if deletedAt.Valid {
		t := deletedAt.Time
		rec.DeletedAt = &t
	}
	return &rec, nil
}

func scanSandboxRows(rows *sql.Rows) ([]SandboxRecord, error) {
	var items []SandboxRecord
	for rows.Next() {
		rec, err := scanSandbox(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan sandbox row: %w", err)
		}
		items = append(items, *rec)
	}
	if items == nil {
		items = []SandboxRecord{}
	}
	return items, nil
}

func toNullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

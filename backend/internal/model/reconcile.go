package model

import "time"

// ReconcileRun represents one reconcile task execution.
type ReconcileRun struct {
	ID          string     `json:"id"`
	TriggerType string     `json:"trigger_type"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	TotalDB     int        `json:"total_db"`
	TotalK8s    int        `json:"total_k8s"`
	DriftCount  int        `json:"drift_count"`
	FixedCount  int        `json:"fixed_count"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
}

// ReconcileItem represents one drift item in a run.
type ReconcileItem struct {
	ID        int64     `json:"id"`
	RunID     string    `json:"run_id"`
	SandboxID string    `json:"sandbox_id"`
	DriftType string    `json:"drift_type"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

// ReconcileRunListResponse lists reconcile runs.
type ReconcileRunListResponse struct {
	Items []ReconcileRun `json:"items"`
}

// ReconcileRunDetailResponse shows run and drift items.
type ReconcileRunDetailResponse struct {
	Run   ReconcileRun    `json:"run"`
	Items []ReconcileItem `json:"items"`
}

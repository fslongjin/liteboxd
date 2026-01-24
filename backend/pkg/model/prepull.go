package model

import "time"

// PrepullStatus represents the status of a prepull task
type PrepullStatus string

const (
	PrepullStatusPending   PrepullStatus = "pending"
	PrepullStatusPulling   PrepullStatus = "pulling"
	PrepullStatusCompleted PrepullStatus = "completed"
	PrepullStatusFailed    PrepullStatus = "failed"
)

// ImagePrepull represents an image prepull task
type ImagePrepull struct {
	ID          string        `json:"id"`
	Image       string        `json:"image"`
	ImageHash   string        `json:"imageHash"`
	Status      PrepullStatus `json:"status"`
	ReadyNodes  int           `json:"readyNodes"`
	TotalNodes  int           `json:"totalNodes"`
	Error       string        `json:"error,omitempty"`
	Template    string        `json:"template,omitempty"` // Template name if triggered from template
	StartedAt   time.Time     `json:"startedAt"`
	CompletedAt *time.Time    `json:"completedAt,omitempty"`
}

// PrepullProgress represents the progress of a prepull task
type PrepullProgress struct {
	Ready int `json:"ready"`
	Total int `json:"total"`
}

// PrepullResponse is the API response for a prepull task
type PrepullResponse struct {
	ID          string          `json:"id"`
	Image       string          `json:"image"`
	Status      PrepullStatus   `json:"status"`
	Progress    PrepullProgress `json:"progress,omitempty"`
	Template    string          `json:"template,omitempty"`
	Error       string          `json:"error,omitempty"`
	StartedAt   time.Time       `json:"startedAt"`
	CompletedAt *time.Time      `json:"completedAt,omitempty"`
}

// PrepullListResponse is the response for listing prepull tasks
type PrepullListResponse struct {
	Items []PrepullResponse `json:"items"`
}

// CreatePrepullRequest is the request body for creating a prepull task
type CreatePrepullRequest struct {
	Image   string `json:"image" binding:"required"`
	Timeout int    `json:"timeout"` // Timeout in seconds, default 600
}

// ToPrepullResponse converts ImagePrepull to PrepullResponse
func (p *ImagePrepull) ToPrepullResponse() PrepullResponse {
	return PrepullResponse{
		ID:     p.ID,
		Image:  p.Image,
		Status: p.Status,
		Progress: PrepullProgress{
			Ready: p.ReadyNodes,
			Total: p.TotalNodes,
		},
		Template:    p.Template,
		Error:       p.Error,
		StartedAt:   p.StartedAt,
		CompletedAt: p.CompletedAt,
	}
}

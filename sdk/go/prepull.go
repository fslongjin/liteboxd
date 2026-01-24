package liteboxd

import (
	"context"
	"time"
)

// PrepullService handles image prepull operations.
type PrepullService struct {
	client *Client
}

// Create creates a new prepull task.
func (p *PrepullService) Create(ctx context.Context, image string, timeout int) (*PrepullResponse, error) {
	req := &CreatePrepullRequest{
		Image:   image,
		Timeout: timeout,
	}
	var result PrepullResponse
	err := p.client.doJSON(ctx, "POST", p.client.buildPath("images", "prepull"), req, &result, nil)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// CreateForTemplate creates a prepull task for a template's image.
func (p *PrepullService) CreateForTemplate(ctx context.Context, templateName string) (*PrepullResponse, error) {
	var result PrepullResponse
	err := p.client.doJSON(ctx, "POST", p.client.buildPath("templates", templateName, "prepull"), nil, &result, nil)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// List retrieves prepull tasks.
func (p *PrepullService) List(ctx context.Context, image, status string) ([]PrepullResponse, error) {
	queryParams := make(map[string]string)
	if image != "" {
		queryParams["image"] = image
	}
	if status != "" {
		queryParams["status"] = status
	}
	var result PrepullListResponse
	err := p.client.doJSON(ctx, "GET", p.client.buildPath("images", "prepull"), nil, &result, queryParams)
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

// Get retrieves a specific prepull task.
func (p *PrepullService) Get(ctx context.Context, id string) (*PrepullResponse, error) {
	var result PrepullResponse
	err := p.client.doJSON(ctx, "GET", p.client.buildPath("images", "prepull", id), nil, &result, nil)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Delete cancels and removes a prepull task.
func (p *PrepullService) Delete(ctx context.Context, id string) error {
	return p.client.doEmptyResponse(ctx, "DELETE", p.client.buildPath("images", "prepull", id), nil, nil)
}

// WaitForCompletion waits until prepull completes.
func (p *PrepullService) WaitForCompletion(ctx context.Context, id string, pollInterval, timeout time.Duration) (*PrepullResponse, error) {
	if pollInterval == 0 {
		pollInterval = 5 * time.Second
	}
	if timeout == 0 {
		timeout = 30 * time.Minute
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, context.DeadlineExceeded
		case <-ticker.C:
			task, err := p.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			if task.Status == PrepullStatusCompleted {
				return task, nil
			}
			if task.Status == PrepullStatusFailed {
				return task, nil
			}
		}
	}
}

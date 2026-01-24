package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/fslongjin/liteboxd/internal/k8s"
	"github.com/fslongjin/liteboxd/internal/model"
	"github.com/fslongjin/liteboxd/internal/store"
)

// PrepullService handles image prepull operations
type PrepullService struct {
	k8sClient *k8s.Client
	store     *store.PrepullStore
}

// NewPrepullService creates a new PrepullService
func NewPrepullService(k8sClient *k8s.Client) *PrepullService {
	return &PrepullService{
		k8sClient: k8sClient,
		store:     store.NewPrepullStore(),
	}
}

// hashImage creates a short hash of the image name for labeling
func hashImage(image string) string {
	h := sha256.Sum256([]byte(image))
	return hex.EncodeToString(h[:])[:12]
}

// Create starts a new prepull task for the given image
func (s *PrepullService) Create(ctx context.Context, req *model.CreatePrepullRequest, template string) (*model.ImagePrepull, error) {
	image := strings.TrimSpace(req.Image)
	if image == "" {
		return nil, fmt.Errorf("image is required")
	}

	// Check if there's already an active prepull for this image
	active, err := s.store.GetActiveByImage(ctx, image)
	if err != nil {
		return nil, err
	}
	if active != nil {
		return nil, fmt.Errorf("prepull already in progress for image '%s' (id: %s)", image, active.ID)
	}

	imageHash := hashImage(image)

	// Create database record
	prepull, err := s.store.Create(ctx, image, imageHash, template)
	if err != nil {
		return nil, err
	}

	// Get node count
	nodeCount, err := s.k8sClient.GetNodeCount(ctx)
	if err != nil {
		nodeCount = 0 // Will be updated when DaemonSet is created
	}
	prepull.TotalNodes = nodeCount

	// Create DaemonSet
	err = s.k8sClient.CreatePrepullDaemonSet(ctx, k8s.CreatePrepullDaemonSetOptions{
		ID:        prepull.ID,
		Image:     image,
		ImageHash: imageHash,
	})
	if err != nil {
		// Update status to failed
		s.store.UpdateStatus(ctx, prepull.ID, model.PrepullStatusFailed, 0, nodeCount, err.Error())
		prepull.Status = model.PrepullStatusFailed
		prepull.Error = err.Error()
		return prepull, nil
	}

	// Update status to pulling
	s.store.UpdateStatus(ctx, prepull.ID, model.PrepullStatusPulling, 0, nodeCount, "")
	prepull.Status = model.PrepullStatusPulling

	return prepull, nil
}

// Get retrieves a prepull task by ID and updates its status from K8s
func (s *PrepullService) Get(ctx context.Context, id string) (*model.ImagePrepull, error) {
	prepull, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if prepull == nil {
		return nil, nil
	}

	// If still in progress, update status from K8s
	if prepull.Status == model.PrepullStatusPending || prepull.Status == model.PrepullStatusPulling {
		s.updateStatusFromK8s(ctx, prepull)
	}

	return prepull, nil
}

// List returns all prepull tasks, optionally filtered
func (s *PrepullService) List(ctx context.Context, image, status string) (*model.PrepullListResponse, error) {
	items, err := s.store.List(ctx, image, status)
	if err != nil {
		return nil, err
	}

	// Update status for active tasks
	for i := range items {
		if items[i].Status == model.PrepullStatusPending || items[i].Status == model.PrepullStatusPulling {
			s.updateStatusFromK8s(ctx, &items[i])
		}
	}

	var responses []model.PrepullResponse
	for _, item := range items {
		responses = append(responses, item.ToPrepullResponse())
	}

	if responses == nil {
		responses = []model.PrepullResponse{}
	}

	return &model.PrepullListResponse{Items: responses}, nil
}

// Delete cancels/deletes a prepull task
func (s *PrepullService) Delete(ctx context.Context, id string) error {
	prepull, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if prepull == nil {
		return fmt.Errorf("prepull not found")
	}

	// Delete DaemonSet if it exists
	err = s.k8sClient.DeletePrepullDaemonSet(ctx, id)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to delete daemonset: %w", err)
	}

	// Delete database record
	return s.store.Delete(ctx, id)
}

// updateStatusFromK8s updates the prepull status from K8s DaemonSet status
func (s *PrepullService) updateStatusFromK8s(ctx context.Context, prepull *model.ImagePrepull) {
	status, err := s.k8sClient.GetPrepullStatus(ctx, prepull.ID)
	if err != nil {
		// DaemonSet might have been deleted or not found
		if strings.Contains(err.Error(), "not found") {
			prepull.Status = model.PrepullStatusFailed
			prepull.Error = "DaemonSet not found"
			s.store.UpdateStatus(ctx, prepull.ID, model.PrepullStatusFailed, 0, 0, "DaemonSet not found")
			fmt.Printf("Prepull %s: DaemonSet not found\n", prepull.ID)
		} else {
			fmt.Printf("Prepull %s: failed to get status: %v\n", prepull.ID, err)
		}
		return
	}

	fmt.Printf("Prepull %s: %d/%d nodes ready, complete=%v\n",
		prepull.ID, status.ReadyNodes, status.DesiredNodes, status.IsComplete)

	prepull.ReadyNodes = status.ReadyNodes
	prepull.TotalNodes = status.DesiredNodes

	if status.IsComplete {
		now := time.Now()
		prepull.Status = model.PrepullStatusCompleted
		prepull.CompletedAt = &now
		s.store.UpdateStatus(ctx, prepull.ID, model.PrepullStatusCompleted, status.ReadyNodes, status.DesiredNodes, "")
		fmt.Printf("Prepull %s: completed\n", prepull.ID)
	} else {
		s.store.UpdateStatus(ctx, prepull.ID, model.PrepullStatusPulling, status.ReadyNodes, status.DesiredNodes, "")
	}
}

// PrepullTemplateImage starts a prepull task for a template's image
func (s *PrepullService) PrepullTemplateImage(ctx context.Context, templateName, image string) (*model.ImagePrepull, error) {
	req := &model.CreatePrepullRequest{
		Image: image,
	}
	return s.Create(ctx, req, templateName)
}

// StartStatusUpdater starts a background goroutine that periodically updates prepull statuses
func (s *PrepullService) StartStatusUpdater(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			s.updateAllActiveStatuses()
		}
	}()
}

func (s *PrepullService) updateAllActiveStatuses() {
	ctx := context.Background()

	// Get all pending and pulling prepulls
	pendingItems, err := s.store.List(ctx, "", string(model.PrepullStatusPending))
	if err != nil {
		fmt.Printf("Prepull status updater: failed to list pending prepulls: %v\n", err)
	} else {
		for i := range pendingItems {
			s.updateStatusFromK8s(ctx, &pendingItems[i])
		}
	}

	pullingItems, err := s.store.List(ctx, "", string(model.PrepullStatusPulling))
	if err != nil {
		fmt.Printf("Prepull status updater: failed to list pulling prepulls: %v\n", err)
	} else {
		for i := range pullingItems {
			s.updateStatusFromK8s(ctx, &pullingItems[i])
		}
	}
}

// CleanupCompletedPrepulls deletes DaemonSets for completed prepulls to free resources
func (s *PrepullService) CleanupCompletedPrepulls(ctx context.Context, olderThan time.Duration) error {
	items, err := s.store.List(ctx, "", string(model.PrepullStatusCompleted))
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-olderThan)
	for _, item := range items {
		if item.CompletedAt != nil && item.CompletedAt.Before(cutoff) {
			// Delete DaemonSet but keep database record
			err := s.k8sClient.DeletePrepullDaemonSet(ctx, item.ID)
			if err != nil && !strings.Contains(err.Error(), "not found") {
				fmt.Printf("Failed to cleanup prepull DaemonSet %s: %v\n", item.ID, err)
			}
		}
	}

	return nil
}

// IsImagePrepulled checks if an image has been successfully prepulled
func (s *PrepullService) IsImagePrepulled(ctx context.Context, image string) bool {
	items, err := s.store.List(ctx, image, string(model.PrepullStatusCompleted))
	if err != nil {
		return false
	}
	// Check if any completed prepull exists for this image
	for _, item := range items {
		if item.Image == image && item.ReadyNodes > 0 && item.ReadyNodes >= item.TotalNodes {
			return true
		}
	}
	return false
}

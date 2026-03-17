package service

import (
	"testing"

	"github.com/fslongjin/liteboxd/backend/internal/model"
	"github.com/fslongjin/liteboxd/backend/internal/store"
)

func TestClassifyPVCMappingStateDeletingWhileSandboxTerminating(t *testing.T) {
	rec := &store.SandboxRecord{
		ID:              "sbx1",
		DesiredState:    store.DesiredStateDeleted,
		LifecycleStatus: "terminating",
	}

	got := classifyPVCMappingState(rec, true, false)
	if got != model.PVCMappingStateDeleting {
		t.Fatalf("classifyPVCMappingState() = %q, want %q", got, model.PVCMappingStateDeleting)
	}
}

func TestClassifyPVCMappingStateDanglingMetadataForNonDeletingRecord(t *testing.T) {
	rec := &store.SandboxRecord{
		ID:              "sbx2",
		DesiredState:    store.DesiredStateActive,
		LifecycleStatus: "running",
	}

	got := classifyPVCMappingState(rec, true, false)
	if got != model.PVCMappingStateDanglingMeta {
		t.Fatalf("classifyPVCMappingState() = %q, want %q", got, model.PVCMappingStateDanglingMeta)
	}
}

func TestBuildDBPVCMapSkipsCompletedDeletePolicyRecords(t *testing.T) {
	records := []store.SandboxRecord{
		{
			ID:                  "skip-me",
			VolumeClaimName:     "sandbox-data-skip-me",
			DesiredState:        store.DesiredStateDeleted,
			LifecycleStatus:     "deleted",
			DeletionPhase:       store.DeletionPhaseCompleted,
			VolumeReclaimPolicy: "Delete",
		},
		{
			ID:                  "keep-me",
			VolumeClaimName:     "sandbox-data-keep-me",
			DesiredState:        store.DesiredStateDeleted,
			LifecycleStatus:     "deleted",
			DeletionPhase:       store.DeletionPhaseCompleted,
			VolumeReclaimPolicy: "Retain",
		},
	}

	got := buildDBPVCMap(records)
	if _, exists := got["sandbox-data-skip-me"]; exists {
		t.Fatalf("Delete/completed pvc mapping should be skipped")
	}
	if _, exists := got["sandbox-data-keep-me"]; !exists {
		t.Fatalf("Retain/completed pvc mapping should be kept")
	}
}

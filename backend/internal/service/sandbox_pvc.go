package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/fslongjin/liteboxd/backend/internal/model"
	"github.com/fslongjin/liteboxd/backend/internal/store"
	corev1 "k8s.io/api/core/v1"
)

func (s *SandboxService) ListPVCMappings(ctx context.Context, opts model.PVCMappingListOptions) (*model.PVCMappingListResponse, error) {
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.PageSize <= 0 {
		opts.PageSize = 20
	}
	if opts.PageSize > 100 {
		opts.PageSize = 100
	}
	if opts.State != "" && !isSupportedPVCMappingState(opts.State) {
		return nil, fmt.Errorf("invalid state %q, expected one of [%s, %s, %s]", opts.State, model.PVCMappingStateBound, model.PVCMappingStateOrphanPVC, model.PVCMappingStateDanglingMeta)
	}

	records, err := s.sandboxStore.ListForReconcile(ctx)
	if err != nil {
		return nil, err
	}
	pvcs, err := s.k8sClient.ListSandboxPVCs(ctx)
	if err != nil {
		return nil, err
	}

	dbByPVC := buildDBPVCMap(records)
	k8sByPVC := buildK8sPVCMap(pvcs)

	nameSet := map[string]struct{}{}
	for name := range dbByPVC {
		nameSet[name] = struct{}{}
	}
	for name := range k8sByPVC {
		nameSet[name] = struct{}{}
	}

	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	sort.Strings(names)

	filtered := make([]model.PVCMapping, 0, len(names))
	for _, pvcName := range names {
		rec, hasDB := dbByPVC[pvcName]
		pvc, hasK8s := k8sByPVC[pvcName]

		item := model.PVCMapping{
			PVCName:   pvcName,
			Namespace: s.k8sClient.SandboxNamespace(),
			State:     classifyPVCMappingState(hasDB, hasK8s),
			Source:    classifyPVCMappingSource(hasDB, hasK8s),
		}

		if hasDB {
			item.SandboxID = rec.ID
			item.SandboxLifecycleState = rec.LifecycleStatus
			item.ReclaimPolicy = rec.VolumeReclaimPolicy
			item.StorageClassName = rec.StorageClassName
			item.RequestedSize = rec.PersistenceSize
			if rec.ClusterNamespace != "" {
				item.Namespace = rec.ClusterNamespace
			}
		}

		if hasK8s {
			item.Namespace = pvc.Namespace
			item.Phase = string(pvc.Status.Phase)
			item.PVName = pvc.Spec.VolumeName
			if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != "" {
				item.StorageClassName = *pvc.Spec.StorageClassName
			}
			if q, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok && !q.IsZero() {
				item.RequestedSize = q.String()
			}
		}

		if opts.SandboxID != "" && item.SandboxID != opts.SandboxID {
			continue
		}
		if opts.StorageClass != "" && item.StorageClassName != opts.StorageClass {
			continue
		}
		if opts.State != "" && item.State != opts.State {
			continue
		}
		filtered = append(filtered, item)
	}

	total := len(filtered)
	start := (opts.Page - 1) * opts.PageSize
	if start >= total {
		return &model.PVCMappingListResponse{
			Items:    []model.PVCMapping{},
			Total:    total,
			Page:     opts.Page,
			PageSize: opts.PageSize,
		}, nil
	}
	end := start + opts.PageSize
	if end > total {
		end = total
	}
	pageItems := filtered[start:end]

	return &model.PVCMappingListResponse{
		Items:    pageItems,
		Total:    total,
		Page:     opts.Page,
		PageSize: opts.PageSize,
	}, nil
}

func buildDBPVCMap(records []store.SandboxRecord) map[string]*store.SandboxRecord {
	out := make(map[string]*store.SandboxRecord, len(records))
	for i := range records {
		rec := &records[i]
		if rec.VolumeClaimName == "" {
			continue
		}
		existing, ok := out[rec.VolumeClaimName]
		if !ok || rec.UpdatedAt.After(existing.UpdatedAt) {
			out[rec.VolumeClaimName] = rec
		}
	}
	return out
}

func buildK8sPVCMap(pvcs []corev1.PersistentVolumeClaim) map[string]*corev1.PersistentVolumeClaim {
	out := make(map[string]*corev1.PersistentVolumeClaim, len(pvcs))
	for i := range pvcs {
		pvc := &pvcs[i]
		out[pvc.Name] = pvc
	}
	return out
}

func isSupportedPVCMappingState(v string) bool {
	switch v {
	case model.PVCMappingStateBound, model.PVCMappingStateOrphanPVC, model.PVCMappingStateDanglingMeta:
		return true
	default:
		return false
	}
}

func classifyPVCMappingState(hasDB, hasK8s bool) string {
	switch {
	case hasDB && hasK8s:
		return model.PVCMappingStateBound
	case hasK8s:
		return model.PVCMappingStateOrphanPVC
	default:
		return model.PVCMappingStateDanglingMeta
	}
}

func classifyPVCMappingSource(hasDB, hasK8s bool) string {
	switch {
	case hasDB && hasK8s:
		return model.PVCMappingSourceDBAndK8s
	case hasK8s:
		return model.PVCMappingSourceK8s
	default:
		return model.PVCMappingSourceDB
	}
}

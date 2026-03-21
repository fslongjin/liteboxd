package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/k8s"
	"github.com/fslongjin/liteboxd/backend/internal/store"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func persistentRecord(id, lifecycle, reason string) *store.SandboxRecord {
	rec := makeTestSandboxRecord(id, true, lifecycle)
	rec.RuntimeName = "sandbox-" + id
	rec.VolumeClaimName = "sandbox-data-" + id
	rec.StatusReason = reason
	return rec
}

func persistentObjects(id string, podPhase corev1.PodPhase, ready bool) []runtime.Object {
	return persistentObjectsWithReplicas(id, 1, podPhase, ready)
}

func persistentObjectsWithReplicas(id string, replicas int32, podPhase corev1.PodPhase, ready bool) []runtime.Object {
	conditionStatus := corev1.ConditionFalse
	if ready {
		conditionStatus = corev1.ConditionTrue
	}
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "sandbox-" + id, Namespace: k8s.DefaultSandboxNamespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
				"app":              k8s.LabelApp,
				k8s.LabelSandboxID: id,
			}},
		},
	}
	pvcPhase := corev1.ClaimPending
	if ready {
		pvcPhase = corev1.ClaimBound
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "sandbox-data-" + id, Namespace: k8s.DefaultSandboxNamespace},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: pvcPhase},
	}
	objects := []runtime.Object{deploy, pvc}
	if podPhase != "" {
		objects = append(objects, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sandbox-" + id,
				Namespace: k8s.DefaultSandboxNamespace,
				UID:       types.UID("uid-" + id),
				Labels: map[string]string{
					"app":              k8s.LabelApp,
					k8s.LabelSandboxID: id,
				},
			},
			Status: corev1.PodStatus{
				Phase: podPhase,
				PodIP: "10.0.0.9",
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: conditionStatus},
				},
			},
		})
	}
	return objects
}

func pvcEvent(id, message string) runtime.Object {
	return &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc-fail-" + id, Namespace: k8s.DefaultSandboxNamespace},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "PersistentVolumeClaim",
			Name:      "sandbox-data-" + id,
			Namespace: k8s.DefaultSandboxNamespace,
		},
		Type:          corev1.EventTypeWarning,
		Reason:        "ProvisioningFailed",
		Message:       message,
		LastTimestamp: metav1.NewTime(time.Now().UTC()),
	}
}

func TestReconcilePersistentFailedDoesNotRegressToPending(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()
	now := time.Now().UTC()

	rec := persistentRecord("recon-failed", "failed", "storage provisioning failed: No available disk candidates")
	rec.LastSeenAt = &now
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	objects := append(
		persistentObjects("recon-failed", corev1.PodPending, false),
		pvcEvent("recon-failed", "No available disk candidates to create a new replica"),
	)
	client := k8s.NewClientForTest(objects...)

	svc := NewSandboxReconcileService(client, sandboxStore)
	if _, err := svc.Run(ctx, "manual"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, err := sandboxStore.GetByID(ctx, "recon-failed")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.LifecycleStatus != "failed" {
		t.Fatalf("LifecycleStatus = %q, want failed", got.LifecycleStatus)
	}
	if !strings.Contains(got.StatusReason, "No available disk candidates") {
		t.Fatalf("StatusReason = %q, want longhorn reason", got.StatusReason)
	}
}

func TestReconcilePersistentRecoveryToRunning(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := persistentRecord("recon-ready", "failed", "storage provisioning failed: previous")
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	client := k8s.NewClientForTest(persistentObjects("recon-ready", corev1.PodRunning, true)...)
	svc := NewSandboxReconcileService(client, sandboxStore)
	if _, err := svc.Run(ctx, "manual"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, err := sandboxStore.GetByID(ctx, "recon-ready")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.LifecycleStatus != "running" {
		t.Fatalf("LifecycleStatus = %q, want running", got.LifecycleStatus)
	}
	if got.StatusReason != "" {
		t.Fatalf("StatusReason = %q, want empty", got.StatusReason)
	}
}

func TestReconcilePersistentReadyIgnoresHistoricalSchedulingWarning(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := persistentRecord("recon-scheduled", "pending", "")
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	objects := append(
		persistentObjects("recon-scheduled", corev1.PodRunning, true),
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-failed-scheduling", Namespace: k8s.DefaultSandboxNamespace},
			InvolvedObject: corev1.ObjectReference{
				Kind:      "Pod",
				Name:      "sandbox-recon-scheduled",
				Namespace: k8s.DefaultSandboxNamespace,
			},
			Type:          corev1.EventTypeWarning,
			Reason:        "FailedScheduling",
			Message:       "0/2 nodes are available: pod has unbound immediate PersistentVolumeClaims. preemption: 0/2 nodes are available",
			LastTimestamp: metav1.NewTime(time.Now().UTC()),
		},
	)

	client := k8s.NewClientForTest(objects...)
	svc := NewSandboxReconcileService(client, sandboxStore)
	if _, err := svc.Run(ctx, "manual"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, err := sandboxStore.GetByID(ctx, "recon-scheduled")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.LifecycleStatus != "running" {
		t.Fatalf("LifecycleStatus = %q, want running", got.LifecycleStatus)
	}
	if got.StatusReason != "" {
		t.Fatalf("StatusReason = %q, want empty", got.StatusReason)
	}
}

func TestReconcilePersistentStoppedKeepsStoppedAndCleansResidualCompletedPod(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()
	now := time.Now().UTC().Add(-10 * time.Minute)

	rec := persistentRecord("recon-stop-clean", "stopped", "stopped by request")
	rec.StoppedAt = &now
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	client := k8s.NewClientForTest(persistentObjectsWithReplicas("recon-stop-clean", 0, corev1.PodSucceeded, false)...)
	svc := NewSandboxReconcileService(client, sandboxStore)
	if _, err := svc.Run(ctx, "manual"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, err := sandboxStore.GetByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.LifecycleStatus != "stopped" {
		t.Fatalf("LifecycleStatus = %q, want stopped", got.LifecycleStatus)
	}
	if got.PodPhase != "" || got.PodUID != "" || got.PodIP != "" {
		t.Fatalf("pod metadata should be cleared for stopped sandbox, got phase=%q uid=%q ip=%q", got.PodPhase, got.PodUID, got.PodIP)
	}
	if got.StoppedAt == nil {
		t.Fatalf("StoppedAt is nil, want preserved")
	}
	if _, err := client.GetPod(ctx, rec.ID); !apierrors.IsNotFound(err) {
		t.Fatalf("expected residual pod to be deleted, got err=%v", err)
	}

	run, err := svc.ListRuns(ctx, 1)
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	detail, err := svc.GetRun(ctx, run.Items[0].ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	foundCleanup := false
	for _, item := range detail.Items {
		if item.SandboxID == rec.ID && item.Action == "cleanup_residual_pod" {
			foundCleanup = true
		}
	}
	if !foundCleanup {
		t.Fatalf("expected cleanup_residual_pod reconcile item")
	}
}

func TestReconcilePersistentStoppedRemainsStartableAfterResidualCompletedPod(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()
	now := time.Now().UTC().Add(-15 * time.Minute)

	rec := persistentRecord("recon-stop-start", "stopped", "stopped by request")
	rec.StoppedAt = &now
	rec.ExpiresAt = time.Now().UTC().Add(15 * time.Minute)
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	client := k8s.NewClientForTest(persistentObjectsWithReplicas("recon-stop-start", 0, corev1.PodSucceeded, false)...)
	reconcileSvc := NewSandboxReconcileService(client, sandboxStore)
	if _, err := reconcileSvc.Run(ctx, "manual"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	svc := &SandboxService{sandboxStore: sandboxStore, k8sClient: client}
	if err := svc.Start(ctx, rec.ID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	got, err := sandboxStore.GetByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.LifecycleStatus != "pending" {
		t.Fatalf("LifecycleStatus = %q, want pending", got.LifecycleStatus)
	}
	deploy, err := client.GetDeployment(ctx, "sandbox-"+rec.ID)
	if err != nil {
		t.Fatalf("GetDeployment() error = %v", err)
	}
	if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != 1 {
		t.Fatalf("deployment replicas = %v, want 1", deploy.Spec.Replicas)
	}
}

func TestReconcilePersistentStoppedCleansUnexpectedRunningPod(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()
	now := time.Now().UTC().Add(-5 * time.Minute)

	rec := persistentRecord("recon-stop-running", "stopped", "stopped by request")
	rec.StoppedAt = &now
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	client := k8s.NewClientForTest(persistentObjectsWithReplicas("recon-stop-running", 0, corev1.PodRunning, false)...)
	svc := NewSandboxReconcileService(client, sandboxStore)
	if _, err := svc.Run(ctx, "manual"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, err := sandboxStore.GetByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.LifecycleStatus != "stopped" {
		t.Fatalf("LifecycleStatus = %q, want stopped", got.LifecycleStatus)
	}
	if got.PodPhase != "" || got.PodUID != "" || got.PodIP != "" {
		t.Fatalf("pod metadata should be cleared for stopped sandbox, got phase=%q uid=%q ip=%q", got.PodPhase, got.PodUID, got.PodIP)
	}
	if _, err := client.GetPod(ctx, rec.ID); !apierrors.IsNotFound(err) {
		t.Fatalf("expected unexpected running pod to be deleted, got err=%v", err)
	}

	run, err := svc.ListRuns(ctx, 1)
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	detail, err := svc.GetRun(ctx, run.Items[0].ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	foundCleanup := false
	for _, item := range detail.Items {
		if item.SandboxID == rec.ID && item.Action == "cleanup_unexpected_pod" {
			foundCleanup = true
		}
	}
	if !foundCleanup {
		t.Fatalf("expected cleanup_unexpected_pod reconcile item")
	}
}

func TestReconcilePersistentStoppedBackfillsStoppedAtWithoutOtherMismatch(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := persistentRecord("recon-stop-backfill", "stopped", "stopped by request")
	rec.StoppedAt = nil
	rec.PodUID = ""
	rec.PodPhase = ""
	rec.PodIP = ""
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	client := k8s.NewClientForTest(persistentObjectsWithReplicas("recon-stop-backfill", 0, "", false)...)
	svc := NewSandboxReconcileService(client, sandboxStore)
	if _, err := svc.Run(ctx, "manual"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, err := sandboxStore.GetByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.LifecycleStatus != "stopped" {
		t.Fatalf("LifecycleStatus = %q, want stopped", got.LifecycleStatus)
	}
	if got.StoppedAt == nil {
		t.Fatalf("StoppedAt is nil, want backfilled")
	}

	run, err := svc.ListRuns(ctx, 1)
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	detail, err := svc.GetRun(ctx, run.Items[0].ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	foundBackfill := false
	for _, item := range detail.Items {
		if item.SandboxID == rec.ID && item.Action == "backfill_stopped_at" {
			foundBackfill = true
		}
	}
	if !foundBackfill {
		t.Fatalf("expected backfill_stopped_at reconcile item")
	}
}

func TestReconcilePersistentSucceededPodWithReplicasOneDoesNotPretendStopped(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := persistentRecord("recon-succeed-running", "running", "")
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	client := k8s.NewClientForTest(persistentObjectsWithReplicas("recon-succeed-running", 1, corev1.PodSucceeded, false)...)
	svc := NewSandboxReconcileService(client, sandboxStore)
	if _, err := svc.Run(ctx, "manual"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, err := sandboxStore.GetByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.LifecycleStatus == "stopped" {
		t.Fatalf("LifecycleStatus = %q, should not be forced to stopped when deployment replicas is 1", got.LifecycleStatus)
	}
}

func TestReconcileDeletedPersistentSandboxKeepsDeletedAndRetriesPVCDelete(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := persistentRecord("recon-del-pvc", "deleted", "")
	rec.DesiredState = store.DesiredStateDeleted
	rec.VolumeReclaimPolicy = "Delete"
	now := time.Now().UTC()
	rec.DeletedAt = &now
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "sandbox-data-recon-del-pvc", Namespace: k8s.DefaultSandboxNamespace},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
	client := k8s.NewClientForTest(pvc)

	svc := NewSandboxReconcileService(client, sandboxStore)
	if _, err := svc.Run(ctx, "manual"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, err := sandboxStore.GetByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.LifecycleStatus != "deleted" || got.DesiredState != store.DesiredStateDeleted {
		t.Fatalf("sandbox status regressed: %+v", got)
	}
	if _, err := client.GetPersistentVolumeClaim(ctx, rec.VolumeClaimName); err != nil {
		t.Fatalf("PVC should remain for deletion service, get error = %v", err)
	}

	run, err := svc.ListRuns(ctx, 1)
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	detail, err := svc.GetRun(ctx, run.Items[0].ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	found := false
	for _, item := range detail.Items {
		if item.SandboxID == rec.ID && item.Action == "deletion_pending" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected deletion_pending reconcile item")
	}
}

func TestReconcileDeletedPersistentSandboxRetainKeepsPVC(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := persistentRecord("recon-del-retain", "deleted", "")
	rec.DesiredState = store.DesiredStateDeleted
	rec.VolumeReclaimPolicy = "Retain"
	now := time.Now().UTC()
	rec.DeletedAt = &now
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "sandbox-data-recon-del-retain", Namespace: k8s.DefaultSandboxNamespace},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
	client := k8s.NewClientForTest(pvc)

	svc := NewSandboxReconcileService(client, sandboxStore)
	if _, err := svc.Run(ctx, "manual"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, err := sandboxStore.GetByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.LifecycleStatus != "deleted" {
		t.Fatalf("LifecycleStatus = %q, want deleted", got.LifecycleStatus)
	}
	if _, err := client.GetPersistentVolumeClaim(ctx, rec.VolumeClaimName); err != nil {
		t.Fatalf("PVC should be retained, get error = %v", err)
	}
}

func TestReconcileDeletedNonPersistentSandboxDoesNotRegress(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := makeTestSandboxRecord("recon-del-pod", false, "deleted")
	rec.DesiredState = store.DesiredStateDeleted
	now := time.Now().UTC()
	rec.DeletedAt = &now
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sandbox-recon-del-pod",
			Namespace: k8s.DefaultSandboxNamespace,
			UID:       types.UID("uid-recon-del-pod"),
			Labels: map[string]string{
				"app":              k8s.LabelApp,
				k8s.LabelSandboxID: rec.ID,
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
	}
	client := k8s.NewClientForTest(pod)

	svc := NewSandboxReconcileService(client, sandboxStore)
	if _, err := svc.Run(ctx, "manual"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, err := sandboxStore.GetByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.LifecycleStatus != "deleted" || got.DesiredState != store.DesiredStateDeleted {
		t.Fatalf("sandbox status regressed: %+v", got)
	}
	if _, err := client.GetPod(ctx, rec.ID); err != nil {
		t.Fatalf("pod should remain for deletion service, get error = %v", err)
	}
}

func TestReconcileDeletedNonPersistentSandboxDeletePodNotFoundIsIgnored(t *testing.T) {
	initServiceTestDB(t)
	ctx := context.Background()
	sandboxStore := store.NewSandboxStore()

	rec := makeTestSandboxRecord("recon-del-pod-race", false, "deleted")
	rec.DesiredState = store.DesiredStateDeleted
	now := time.Now().UTC()
	rec.DeletedAt = &now
	if err := sandboxStore.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sandbox-recon-del-pod-race",
			Namespace: k8s.DefaultSandboxNamespace,
			UID:       types.UID("uid-recon-del-pod-race"),
			Labels: map[string]string{
				"app":              k8s.LabelApp,
				k8s.LabelSandboxID: rec.ID,
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
	}
	client := k8s.NewClientForTestWithSetup(func(clientset *k8sfake.Clientset) {
		clientset.PrependReactor("delete", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewNotFound(corev1.Resource("pods"), "sandbox-"+rec.ID)
		})
	}, pod)

	svc := NewSandboxReconcileService(client, sandboxStore)
	if _, err := svc.Run(ctx, "manual"); err != nil {
		t.Fatalf("Run() error = %v, want nil on benign NotFound", err)
	}

	got, err := sandboxStore.GetByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.LifecycleStatus != "deleted" || got.DesiredState != store.DesiredStateDeleted {
		t.Fatalf("sandbox status regressed: %+v", got)
	}

	run, err := svc.ListRuns(ctx, 1)
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(run.Items) != 1 || run.Items[0].Status != reconcileStatusCompleted {
		t.Fatalf("unexpected reconcile run status: %+v", run.Items)
	}
}

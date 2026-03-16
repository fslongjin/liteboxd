package service

import (
	"strings"
	"testing"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func readyPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "sandbox-abc123", UID: "pod-uid"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}
}

func persistentEvent(kind, name, eventType, reason, message string) corev1.Event {
	now := time.Now().UTC()
	return corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:              reason + "-" + strings.ToLower(kind),
			CreationTimestamp: metav1.NewTime(now),
		},
		InvolvedObject: corev1.ObjectReference{
			Kind: kind,
			Name: name,
		},
		Type:          eventType,
		Reason:        reason,
		Message:       message,
		LastTimestamp: metav1.NewTime(now),
	}
}

func TestClassifyPersistentStartupLonghornCapacityFailure(t *testing.T) {
	snapshot := &k8s.PersistentSandboxSnapshot{
		PVCName: "sandbox-data-abc123",
		PVCEvents: []corev1.Event{
			{
				Type:    corev1.EventTypeWarning,
				Reason:  "ProvisioningFailed",
				Message: "No available disk candidates to create a new replica of size 21474836480",
			},
		},
	}

	state, reason := classifyPersistentStartup(snapshot)
	if state != persistentStartupFailed {
		t.Fatalf("state = %q, want failed", state)
	}
	if !strings.Contains(reason, "No available disk candidates") {
		t.Fatalf("reason = %q, want longhorn message", reason)
	}
}

func TestClassifyPersistentStartupPendingWithoutFailure(t *testing.T) {
	snapshot := &k8s.PersistentSandboxSnapshot{
		PVCName: "sandbox-data-abc123",
		PVC: &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "sandbox-data-abc123"},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
		},
		DeploymentName: "sandbox-abc123",
		Deployment:     &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "sandbox-abc123"}},
	}

	state, reason := classifyPersistentStartup(snapshot)
	if state != persistentStartupPending {
		t.Fatalf("state = %q, want pending", state)
	}
	if reason != "waiting for pvc sandbox-data-abc123 to bind" {
		t.Fatalf("reason = %q, want pvc bind wait", reason)
	}
}

func TestClassifyPersistentStartupReady(t *testing.T) {
	state, reason := classifyPersistentStartup(&k8s.PersistentSandboxSnapshot{Pod: readyPod()})
	if state != persistentStartupReady {
		t.Fatalf("state = %q, want ready", state)
	}
	if reason != "" {
		t.Fatalf("reason = %q, want empty", reason)
	}
}

func TestClassifyPersistentStartupTreatsUnboundPVCSchedulingAsPending(t *testing.T) {
	snapshot := &k8s.PersistentSandboxSnapshot{
		PVCName: "sandbox-data-abc123",
		PVC: &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "sandbox-data-abc123"},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "sandbox-abc123"},
			Status: corev1.PodStatus{
				Phase:   corev1.PodPending,
				Message: "pod has unbound immediate PersistentVolumeClaims",
			},
		},
		PodEvents: []corev1.Event{
			persistentEvent("Pod", "sandbox-abc123", corev1.EventTypeWarning, "FailedScheduling", "0/2 nodes are available: pod has unbound immediate PersistentVolumeClaims. preemption: 0/2 nodes are available"),
		},
	}

	state, reason := classifyPersistentStartup(snapshot)
	if state != persistentStartupPending {
		t.Fatalf("state = %q, want pending", state)
	}
	if reason != "waiting for pvc sandbox-data-abc123 to bind" {
		t.Fatalf("reason = %q, want pvc bind wait", reason)
	}
}

func TestClassifyPersistentStartupReadyOverridesHistoricalSchedulingWarning(t *testing.T) {
	snapshot := &k8s.PersistentSandboxSnapshot{
		PVCName: "sandbox-data-abc123",
		PVC: &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "sandbox-data-abc123"},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
		},
		Pod: readyPod(),
		PodEvents: []corev1.Event{
			persistentEvent("Pod", "sandbox-abc123", corev1.EventTypeWarning, "FailedScheduling", "0/2 nodes are available: pod has unbound immediate PersistentVolumeClaims. preemption: 0/2 nodes are available"),
		},
	}

	state, reason := classifyPersistentStartup(snapshot)
	if state != persistentStartupReady {
		t.Fatalf("state = %q, want ready", state)
	}
	if reason != "" {
		t.Fatalf("reason = %q, want empty", reason)
	}
}

func TestClassifyPersistentStartupDoesNotFailOnTransientMountWarning(t *testing.T) {
	snapshot := &k8s.PersistentSandboxSnapshot{
		PVCName: "sandbox-data-abc123",
		PVC: &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "sandbox-data-abc123"},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "sandbox-abc123"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionFalse},
				},
			},
		},
		PodEvents: []corev1.Event{
			persistentEvent("Pod", "sandbox-abc123", corev1.EventTypeWarning, "FailedMount", "MountVolume.SetUp failed for volume \"data\" : timed out waiting for the condition"),
		},
	}

	state, reason := classifyPersistentStartup(snapshot)
	if state != persistentStartupPending {
		t.Fatalf("state = %q, want pending", state)
	}
	if reason != "waiting for pod readiness" {
		t.Fatalf("reason = %q, want readiness wait", reason)
	}
}

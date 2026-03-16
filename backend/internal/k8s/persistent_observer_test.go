package k8s

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func testPodForPersistentSnapshot(sandboxID string, phase corev1.PodPhase, ready bool) *corev1.Pod {
	conditionStatus := corev1.ConditionFalse
	if ready {
		conditionStatus = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sandbox-" + sandboxID,
			Namespace: DefaultSandboxNamespace,
			Labels: map[string]string{
				"app":          LabelApp,
				LabelSandboxID: sandboxID,
			},
			UID: types.UID("pod-uid-" + sandboxID),
		},
		Status: corev1.PodStatus{
			Phase: phase,
			PodIP: "10.0.0.8",
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: conditionStatus},
			},
		},
	}
}

func testPVC(name string, phase corev1.PersistentVolumeClaimPhase) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: DefaultSandboxNamespace},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: phase},
	}
}

func testDeployment(name, sandboxID string) *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: DefaultSandboxNamespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
				"app":          LabelApp,
				LabelSandboxID: sandboxID,
			}},
		},
	}
}

func testEvent(kind, name, eventType, reason, message string, ts time.Time) *corev1.Event {
	return &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:              reason + "-" + strings.ToLower(kind),
			Namespace:         DefaultSandboxNamespace,
			CreationTimestamp: metav1.NewTime(ts),
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:      kind,
			Name:      name,
			Namespace: DefaultSandboxNamespace,
		},
		Type:          eventType,
		Reason:        reason,
		Message:       message,
		LastTimestamp: metav1.NewTime(ts),
	}
}

func TestGetPersistentSandboxSnapshotIncludesEvents(t *testing.T) {
	now := time.Now().UTC()
	client := NewClientForTest(
		testDeployment("sandbox-abc123", "abc123"),
		testPVC("sandbox-data-abc123", corev1.ClaimPending),
		testPodForPersistentSnapshot("abc123", corev1.PodPending, false),
		testEvent("PersistentVolumeClaim", "sandbox-data-abc123", corev1.EventTypeWarning, "ProvisioningFailed", "No available disk candidates", now),
		testEvent("Pod", "sandbox-abc123", corev1.EventTypeWarning, "FailedMount", "MountVolume.SetUp failed for volume", now.Add(time.Second)),
	)

	snapshot, err := client.GetPersistentSandboxSnapshot(context.Background(), "abc123", "sandbox-abc123", "sandbox-data-abc123")
	if err != nil {
		t.Fatalf("GetPersistentSandboxSnapshot() error = %v", err)
	}
	if snapshot.Deployment == nil || snapshot.PVC == nil || snapshot.Pod == nil {
		t.Fatalf("snapshot missing objects: %+v", snapshot)
	}
	if len(snapshot.PVCEvents) != 1 || snapshot.PVCEvents[0].Reason != "ProvisioningFailed" {
		t.Fatalf("PVC events = %+v, want ProvisioningFailed", snapshot.PVCEvents)
	}
	if len(snapshot.PodEvents) != 1 || snapshot.PodEvents[0].Reason != "FailedMount" {
		t.Fatalf("Pod events = %+v, want FailedMount", snapshot.PodEvents)
	}
}

func TestGetSandboxEventsFormatsObjectPrefixes(t *testing.T) {
	now := time.Now().UTC()
	client := NewClientForTest(
		testDeployment("sandbox-abc123", "abc123"),
		testPVC("sandbox-data-abc123", corev1.ClaimPending),
		testPodForPersistentSnapshot("abc123", corev1.PodPending, false),
		testEvent("PersistentVolumeClaim", "sandbox-data-abc123", corev1.EventTypeWarning, "ProvisioningFailed", "No available disk candidates", now),
		testEvent("Deployment", "sandbox-abc123", corev1.EventTypeWarning, "ReplicaFailure", "replica failed", now.Add(time.Second)),
		testEvent("Pod", "sandbox-abc123", corev1.EventTypeWarning, "FailedMount", "MountVolume.SetUp failed", now.Add(2*time.Second)),
	)

	events, err := client.GetSandboxEvents(context.Background(), "abc123", "sandbox-abc123", "sandbox-data-abc123")
	if err != nil {
		t.Fatalf("GetSandboxEvents() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("events len = %d, want 3 (%v)", len(events), events)
	}
	if !strings.HasPrefix(events[0], "[PVC][Warning]") {
		t.Fatalf("first event = %q, want PVC prefix", events[0])
	}
	if !strings.Contains(strings.Join(events, "\n"), "[Deployment][Warning] ReplicaFailure") {
		t.Fatalf("events = %v, want deployment event", events)
	}
	if !strings.Contains(strings.Join(events, "\n"), "[Pod][Warning] FailedMount") {
		t.Fatalf("events = %v, want pod event", events)
	}
}

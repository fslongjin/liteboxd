package k8s

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PersistentSandboxSnapshot captures the current runtime view for a persistence-enabled sandbox.
type PersistentSandboxSnapshot struct {
	SandboxID      string
	DeploymentName string
	PVCName        string

	Deployment *appsv1.Deployment
	PVC        *corev1.PersistentVolumeClaim
	Pod        *corev1.Pod

	DeploymentEvents []corev1.Event
	PVCEvents        []corev1.Event
	PodEvents        []corev1.Event
}

func (c *Client) GetDeployment(ctx context.Context, name string) (*appsv1.Deployment, error) {
	deploy, err := c.clientset.AppsV1().Deployments(c.sandboxNS).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return deploy, nil
}

func (c *Client) GetPersistentVolumeClaim(ctx context.Context, name string) (*corev1.PersistentVolumeClaim, error) {
	pvc, err := c.clientset.CoreV1().PersistentVolumeClaims(c.sandboxNS).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pvc, nil
}

func (c *Client) GetObjectEvents(ctx context.Context, namespace, kind, name string) ([]corev1.Event, error) {
	list, err := c.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	out := make([]corev1.Event, 0, len(list.Items))
	for i := range list.Items {
		event := list.Items[i]
		if !strings.EqualFold(event.InvolvedObject.Kind, kind) {
			continue
		}
		if event.InvolvedObject.Name != name {
			continue
		}
		out = append(out, event)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return eventTimestamp(out[i]).After(eventTimestamp(out[j]))
	})
	return out, nil
}

func (c *Client) GetPersistentSandboxSnapshot(ctx context.Context, sandboxID, deploymentName, pvcName string) (*PersistentSandboxSnapshot, error) {
	snapshot := &PersistentSandboxSnapshot{
		SandboxID:      sandboxID,
		DeploymentName: deploymentName,
		PVCName:        pvcName,
	}

	if deploymentName != "" {
		deploy, err := c.GetDeployment(ctx, deploymentName)
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get deployment %s: %w", deploymentName, err)
		}
		if err == nil {
			snapshot.Deployment = deploy
			events, eventErr := c.GetObjectEvents(ctx, c.sandboxNS, "Deployment", deploymentName)
			if eventErr != nil {
				return nil, fmt.Errorf("failed to get deployment events %s: %w", deploymentName, eventErr)
			}
			snapshot.DeploymentEvents = events
		}
	}

	if pvcName != "" {
		pvc, err := c.GetPersistentVolumeClaim(ctx, pvcName)
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get pvc %s: %w", pvcName, err)
		}
		if err == nil {
			snapshot.PVC = pvc
			events, eventErr := c.GetObjectEvents(ctx, c.sandboxNS, "PersistentVolumeClaim", pvcName)
			if eventErr != nil {
				return nil, fmt.Errorf("failed to get pvc events %s: %w", pvcName, eventErr)
			}
			snapshot.PVCEvents = events
		}
	}

	pod, err := c.getSandboxPod(ctx, sandboxID)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get sandbox pod: %w", err)
	}
	if err == nil {
		snapshot.Pod = pod
		events, eventErr := c.GetObjectEvents(ctx, c.sandboxNS, "Pod", pod.Name)
		if eventErr != nil {
			return nil, fmt.Errorf("failed to get pod events %s: %w", pod.Name, eventErr)
		}
		snapshot.PodEvents = events
	}

	return snapshot, nil
}

func (c *Client) GetSandboxEvents(ctx context.Context, sandboxID, deploymentName, pvcName string) ([]string, error) {
	snapshot, err := c.GetPersistentSandboxSnapshot(ctx, sandboxID, deploymentName, pvcName)
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(snapshot.PVCEvents)+len(snapshot.DeploymentEvents)+len(snapshot.PodEvents))
	for _, event := range snapshot.PVCEvents {
		out = append(out, formatObjectEvent("PVC", event))
	}
	for _, event := range snapshot.DeploymentEvents {
		out = append(out, formatObjectEvent("Deployment", event))
	}
	for _, event := range snapshot.PodEvents {
		out = append(out, formatObjectEvent("Pod", event))
	}
	return out, nil
}

func formatObjectEvent(kind string, event corev1.Event) string {
	return fmt.Sprintf("[%s][%s] %s: %s", kind, event.Type, event.Reason, event.Message)
}

func eventTimestamp(event corev1.Event) time.Time {
	switch {
	case !event.LastTimestamp.IsZero():
		return event.LastTimestamp.Time
	case !event.EventTime.IsZero():
		return event.EventTime.Time
	case !event.FirstTimestamp.IsZero():
		return event.FirstTimestamp.Time
	default:
		return event.CreationTimestamp.Time
	}
}

package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/k8s"
	"github.com/fslongjin/liteboxd/backend/internal/logx"
	"github.com/fslongjin/liteboxd/backend/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

type persistentStartupState string

const (
	persistentStartupPending persistentStartupState = "pending"
	persistentStartupReady   persistentStartupState = "ready"
	persistentStartupFailed  persistentStartupState = "failed"
)

var (
	storageFailureEventReasons = map[string]string{
		"ProvisioningFailed": "storage provisioning failed",
		"FailedBinding":      "storage provisioning failed",
		"FailedAttachVolume": "storage attach/mount failed",
		"FailedMount":        "storage attach/mount failed",
	}
	storageTerminalFailureMessageFragments = []string{
		"no available disk candidates",
		"unable to create new replica",
		"all replicas are failed",
		"failed to auto salvage volume",
		"failed to provision volume",
	}
	explicitPodWaitingReasons = map[string]struct{}{
		"CrashLoopBackOff":           {},
		"CreateContainerConfigError": {},
		"CreateContainerError":       {},
		"ErrImagePull":               {},
		"ImageInspectError":          {},
		"ImagePullBackOff":           {},
		"InvalidImageName":           {},
		"RunContainerError":          {},
	}
)

func classifyPersistentStartup(snapshot *k8s.PersistentSandboxSnapshot) (persistentStartupState, string) {
	if snapshot != nil && podReady(snapshot.Pod) {
		return persistentStartupReady, ""
	}
	if reason := detectStorageFailureReason(snapshot); reason != "" {
		return persistentStartupFailed, reason
	}
	if reason := detectPodStartupFailureReason(snapshot); reason != "" {
		return persistentStartupFailed, reason
	}
	if reason := pendingPersistentStartupReason(snapshot); reason != "" {
		return persistentStartupPending, reason
	}
	return persistentStartupPending, ""
}

func detectStorageFailureReason(snapshot *k8s.PersistentSandboxSnapshot) string {
	if snapshot == nil {
		return ""
	}

	for _, event := range append(append([]corev1.Event{}, snapshot.PVCEvents...), snapshot.PodEvents...) {
		if msg, ok := classifyStorageEvent(event); ok {
			return msg
		}
	}
	for _, event := range snapshot.DeploymentEvents {
		if event.Type != corev1.EventTypeWarning {
			continue
		}
		if hasTerminalStorageFailureMessage(event.Message) {
			return "storage provisioning failed: " + event.Message
		}
	}
	if snapshot.Deployment != nil {
		for _, cond := range snapshot.Deployment.Status.Conditions {
			if cond.Type == appsv1.DeploymentReplicaFailure && cond.Status == corev1.ConditionTrue {
				message := strings.TrimSpace(cond.Message)
				if message == "" {
					message = strings.TrimSpace(cond.Reason)
				}
				if message == "" {
					message = "persistent volume provisioning failed"
				}
				if hasTerminalStorageFailureMessage(message) {
					return "storage provisioning failed: " + message
				}
				return "persistent sandbox deployment failed: " + message
			}
		}
	}
	return ""
}

func classifyStorageEvent(event corev1.Event) (string, bool) {
	if event.Type != corev1.EventTypeWarning {
		return "", false
	}
	if !hasTerminalStorageFailureMessage(event.Message) {
		return "", false
	}
	prefix := "storage provisioning failed"
	if mapped, ok := storageFailureEventReasons[event.Reason]; ok {
		prefix = mapped
	}
	return prefix + ": " + event.Message, true
}

func hasTerminalStorageFailureMessage(msg string) bool {
	value := strings.ToLower(strings.TrimSpace(msg))
	for _, fragment := range storageTerminalFailureMessageFragments {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}

func detectPodStartupFailureReason(snapshot *k8s.PersistentSandboxSnapshot) string {
	if snapshot == nil {
		return ""
	}
	if snapshot.Pod != nil {
		pod := snapshot.Pod
		if pod.Status.Phase == corev1.PodFailed {
			if pod.Status.Message != "" {
				return "persistent sandbox pod failed to start: " + pod.Status.Message
			}
			if pod.Status.Reason != "" {
				return "persistent sandbox pod failed to start: " + pod.Status.Reason
			}
			return "persistent sandbox pod failed to start"
		}
		if reason := containerFailureReason(pod.Status.InitContainerStatuses); reason != "" {
			return "persistent sandbox pod failed to start: " + reason
		}
		if reason := containerFailureReason(pod.Status.ContainerStatuses); reason != "" {
			return "persistent sandbox pod failed to start: " + reason
		}
	}
	if snapshot.Deployment != nil {
		for _, cond := range snapshot.Deployment.Status.Conditions {
			if cond.Type == appsv1.DeploymentProgressing && cond.Status == corev1.ConditionFalse {
				message := strings.TrimSpace(cond.Message)
				if message == "" {
					message = strings.TrimSpace(cond.Reason)
				}
				if message == "" {
					message = "persistent sandbox pod failed to start"
				}
				return "persistent sandbox pod failed to start: " + message
			}
		}
	}
	return ""
}

func containerFailureReason(statuses []corev1.ContainerStatus) string {
	for _, status := range statuses {
		if state := status.State.Terminated; state != nil && state.ExitCode != 0 {
			if state.Message != "" {
				return fmt.Sprintf("container %s terminated: %s", status.Name, state.Message)
			}
			if state.Reason != "" {
				return fmt.Sprintf("container %s terminated: %s", status.Name, state.Reason)
			}
			return fmt.Sprintf("container %s terminated with exit code %d", status.Name, state.ExitCode)
		}
		if state := status.State.Waiting; state != nil {
			if _, ok := explicitPodWaitingReasons[state.Reason]; ok {
				if state.Message != "" {
					return fmt.Sprintf("container %s waiting: %s", status.Name, state.Message)
				}
				return fmt.Sprintf("container %s waiting: %s", status.Name, state.Reason)
			}
		}
	}
	return ""
}

func pendingPersistentStartupReason(snapshot *k8s.PersistentSandboxSnapshot) string {
	if snapshot == nil {
		return ""
	}
	if snapshot.PVC == nil && snapshot.PVCName != "" {
		return fmt.Sprintf("waiting for pvc %s to be created", snapshot.PVCName)
	}
	if snapshot.PVC != nil && snapshot.PVC.Status.Phase != corev1.ClaimBound {
		return fmt.Sprintf("waiting for pvc %s to bind", snapshot.PVC.Name)
	}
	if snapshot.Deployment == nil && snapshot.DeploymentName != "" {
		return fmt.Sprintf("waiting for deployment %s to be created", snapshot.DeploymentName)
	}
	if snapshot.Pod == nil {
		return "waiting for deployment to create pod"
	}
	if snapshot.Pod.Status.Phase == corev1.PodPending {
		if snapshot.Pod.Status.Message != "" {
			return snapshot.Pod.Status.Message
		}
		if snapshot.Pod.Status.Reason != "" {
			return snapshot.Pod.Status.Reason
		}
		return "waiting for pod scheduling"
	}
	if snapshot.Pod.Status.Phase == corev1.PodRunning && !podReady(snapshot.Pod) {
		return "waiting for pod readiness"
	}
	return ""
}

func podReady(pod *corev1.Pod) bool {
	if pod == nil || pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (s *SandboxService) runPersistentStartupMonitor(ctx context.Context, id, deploymentName, pvcName string, startupTimeout int) bool {
	logger := logWithSandboxID(ctx, id)
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(startupTimeout)*time.Second)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastPendingReason := ""
	for {
		snapshot, err := s.k8sClient.GetPersistentSandboxSnapshot(timeoutCtx, id, deploymentName, pvcName)
		if err != nil {
			logger.Warn("persistent startup snapshot failed", "error", err)
		} else {
			state, reason := classifyPersistentStartup(snapshot)
			switch state {
			case persistentStartupReady:
				s.updatePersistentRuntimeState(context.Background(), id, snapshot, string(model.SandboxStatusRunning), "")
				_ = s.sandboxStore.AppendStatusHistory(context.Background(), id, "system", "pending", string(model.SandboxStatusRunning), "persistent sandbox is ready", nil, time.Now().UTC())
				return true
			case persistentStartupFailed:
				s.updatePersistentRuntimeState(context.Background(), id, snapshot, string(model.SandboxStatusFailed), reason)
				_ = s.sandboxStore.AppendStatusHistory(context.Background(), id, "system", "pending", string(model.SandboxStatusFailed), reason, nil, time.Now().UTC())
				return false
			case persistentStartupPending:
				lastPendingReason = reason
			}
		}

		select {
		case <-timeoutCtx.Done():
			reason := "persistent sandbox startup timed out"
			if snapshot, err := s.k8sClient.GetPersistentSandboxSnapshot(context.Background(), id, deploymentName, pvcName); err == nil {
				if state, classified := classifyPersistentStartup(snapshot); state == persistentStartupFailed {
					reason = classified
				} else if classified != "" {
					reason = "persistent sandbox startup timed out: " + classified
				} else if pending := pendingPersistentStartupReason(snapshot); pending != "" {
					reason = "persistent sandbox startup timed out: " + pending
				}
				s.updatePersistentRuntimeState(context.Background(), id, snapshot, string(model.SandboxStatusFailed), reason)
			} else {
				if lastPendingReason != "" {
					reason = "persistent sandbox startup timed out: " + lastPendingReason
				}
				s.updateStatusDurable(id, string(model.SandboxStatusFailed), reason)
			}
			_ = s.sandboxStore.AppendStatusHistory(context.Background(), id, "system", "pending", string(model.SandboxStatusFailed), reason, nil, time.Now().UTC())
			logger.Warn("persistent startup timed out", "reason", reason)
			return false
		case <-ticker.C:
		}
	}
}

func (s *SandboxService) updatePersistentRuntimeState(ctx context.Context, id string, snapshot *k8s.PersistentSandboxSnapshot, lifecycleStatus, reason string) {
	now := time.Now().UTC()
	if snapshot != nil && snapshot.Pod != nil {
		_ = s.sandboxStore.UpdateObservedState(
			ctx,
			id,
			string(snapshot.Pod.UID),
			string(snapshot.Pod.Status.Phase),
			snapshot.Pod.Status.PodIP,
			lifecycleStatus,
			reason,
			now,
			now,
		)
		return
	}
	_ = s.sandboxStore.UpdateStatus(ctx, id, lifecycleStatus, reason, now)
}

func logWithSandboxID(ctx context.Context, id string) *slog.Logger {
	return logx.LoggerWithRequestID(ctx).With("component", "sandbox_service", "sandbox_id", id)
}

package k8s

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type CreatePersistentSandboxOptions struct {
	CreatePodOptions
	StorageClassName string
	VolumeSize       string
	VolumeClaimName  string
}

type SandboxDeletionSnapshot struct {
	Deployment        *appsv1.Deployment
	Pods              []corev1.Pod
	PVC               *corev1.PersistentVolumeClaim
	PV                *corev1.PersistentVolume
	VolumeAttachments []storagev1.VolumeAttachment
}

const (
	DefaultPersistentRootFSHelperImage = "ubuntu:24.04"

	rootfsOverlayStateDir          = ".liteboxd-overlay"
	rootfsOverlayMergedSub         = ".liteboxd-overlay/merged"
	rootfsOverlayPrepInitName      = "rootfs-prepare"
	rootfsOverlayHelperName        = "rootfs-helper"
	rootfsOverlayVolumeName        = "rootfs"
	rootfsOverlayControlVolumeName = "rootfs-control"
	// Hidden mount target to avoid leaking internal implementation details in normal `ls /`.
	rootfsOverlayMountTarget = "/.liteboxd-rootfs"
	rootfsOverlayMergedPath  = rootfsOverlayMountTarget + "/" + rootfsOverlayMergedSub
	rootfsOverlayControlDir  = "/.liteboxd-control"
)

func (c *Client) getSandboxPod(ctx context.Context, sandboxID string) (*corev1.Pod, error) {
	legacyName := fmt.Sprintf("sandbox-%s", sandboxID)
	pod, err := c.clientset.CoreV1().Pods(c.sandboxNS).Get(ctx, legacyName, metav1.GetOptions{})
	if err == nil {
		return pod, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get sandbox pod: %w", err)
	}

	selector := labels.Set{
		"app":          LabelApp,
		LabelSandboxID: sandboxID,
	}.AsSelector().String()
	list, err := c.clientset.CoreV1().Pods(c.sandboxNS).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("failed to list sandbox pods: %w", err)
	}
	if len(list.Items) == 0 {
		return nil, apierrors.NewNotFound(corev1.Resource("pods"), legacyName)
	}

	items := list.Items
	sort.Slice(items, func(i, j int) bool {
		pi := podPriority(&items[i])
		pj := podPriority(&items[j])
		if pi != pj {
			return pi > pj
		}
		return items[i].CreationTimestamp.Time.After(items[j].CreationTimestamp.Time)
	})
	return &items[0], nil
}

func podPriority(p *corev1.Pod) int {
	if p.DeletionTimestamp != nil {
		return 0
	}
	if p.Status.Phase == corev1.PodRunning {
		for _, cond := range p.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				return 3
			}
		}
		return 2
	}
	if p.Status.Phase == corev1.PodPending {
		return 1
	}
	return 0
}

func (c *Client) CreatePersistentSandbox(ctx context.Context, opts CreatePersistentSandboxOptions) (*appsv1.Deployment, error) {
	if opts.VolumeSize == "" {
		return nil, fmt.Errorf("volume size is required for persistent sandbox")
	}
	if _, err := resource.ParseQuantity(opts.VolumeSize); err != nil {
		return nil, fmt.Errorf("invalid volume size: %w", err)
	}
	command, args, err := resolveSandboxStartCommand(ctx, opts.Image, opts.Command, opts.Args)
	if err != nil {
		return nil, err
	}

	claimName := opts.VolumeClaimName
	if claimName == "" {
		claimName = fmt.Sprintf("sandbox-data-%s", opts.ID)
	}
	if err := c.ensurePersistentVolumeClaim(ctx, claimName, opts.StorageClassName, opts.VolumeSize); err != nil {
		return nil, err
	}

	deployName := fmt.Sprintf("sandbox-%s", opts.ID)
	accessToken := opts.AccessToken
	if accessToken == "" {
		accessToken = generateAccessToken()
	}

	annotations := map[string]string{
		AnnotationTTL:         fmt.Sprintf("%d", opts.TTL),
		AnnotationCreatedAt:   time.Now().UTC().Format(time.RFC3339),
		AnnotationAccessToken: accessToken,
	}
	for k, v := range opts.Annotations {
		annotations[k] = v
	}

	labels := map[string]string{
		"app":          LabelApp,
		LabelSandboxID: opts.ID,
		LabelManagedBy: ManagedByServer,
	}
	if opts.Network != nil && opts.Network.AllowInternetAccess && len(opts.Network.AllowedDomains) == 0 {
		labels[LabelInternetAccess] = "true"
	}

	mainContainer := containerWithCommandAndArgs(opts.CreatePodOptions)
	// containerWithCommandAndArgs adds the default /workspace mount for ephemeral sandboxes.
	// Persistent rootfs sandboxes must replace it with rootfs-specific mounts only.
	mainContainer.VolumeMounts = []corev1.VolumeMount{
		{
			Name:      rootfsOverlayVolumeName,
			MountPath: rootfsOverlayMountTarget,
		},
		{
			Name:      rootfsOverlayControlVolumeName,
			MountPath: rootfsOverlayControlDir,
		},
	}
	mainContainer.Env = append(mainContainer.Env,
		corev1.EnvVar{Name: "LITEBOXD_ROOTFS_CONTROL_DIR", Value: rootfsOverlayControlDir},
		corev1.EnvVar{Name: "LITEBOXD_ROOTFS_MOUNT_TARGET", Value: rootfsOverlayMountTarget},
	)
	if c.launcherEnabled() {
		mainContainer.VolumeMounts = append(mainContainer.VolumeMounts, sandboxLauncherMount(true))
		mainContainer.Env = append(mainContainer.Env, corev1.EnvVar{
			Name:  "LITEBOXD_NOFILE_LIMIT",
			Value: strconv.Itoa(c.sandboxNoFileLimit),
		})
	}
	// Kubernetes/containerd cannot mount a volume directly to "/" in container rootfs.
	// Keep the business container unprivileged and let it wait for a privileged helper to
	// mount the overlay inside this container's mount namespace before chrooting.
	mainContainer.Command = []string{"sh", "-ec", buildRootfsOverlayMainWrapperScript(), "rootfs-main"}
	mainContainer.Args = append(append([]string(nil), command...), args...)

	prepInitContainer := corev1.Container{
		Name:            rootfsOverlayPrepInitName,
		Image:           c.persistentRootFSHelperImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"sh", "-ec", buildRootfsOverlayPrepScript()},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
		},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: boolPtr(false),
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      rootfsOverlayVolumeName,
				MountPath: rootfsOverlayMountTarget,
			},
			{
				Name:      rootfsOverlayControlVolumeName,
				MountPath: rootfsOverlayControlDir,
			},
		},
	}
	helperContainer := corev1.Container{
		Name:            rootfsOverlayHelperName,
		Image:           c.persistentRootFSHelperImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"sh", "-ec", buildRootfsOverlayHelperScript()},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("300m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("25m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		},
		SecurityContext: &corev1.SecurityContext{
			Privileged:               boolPtr(true),
			AllowPrivilegeEscalation: boolPtr(true),
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      rootfsOverlayVolumeName,
				MountPath: rootfsOverlayMountTarget,
			},
			{
				Name:      rootfsOverlayControlVolumeName,
				MountPath: rootfsOverlayControlDir,
			},
		},
	}

	replicas := int32(1)
	initContainers := []corev1.Container{prepInitContainer}
	volumes := []corev1.Volume{
		{
			Name: rootfsOverlayVolumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
				},
			},
		},
		{
			Name: rootfsOverlayControlVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}
	if c.launcherEnabled() {
		initContainers = append([]corev1.Container{c.sandboxLauncherInitContainer()}, initContainers...)
		volumes = append(volumes, c.sandboxLauncherVolume())
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        deployName,
			Namespace:   c.sandboxNS,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken:  boolPtr(false),
					TerminationGracePeriodSeconds: int64Ptr(30),
					// Keep a consistent procfs view for chroot runtime by sharing PID namespace
					// between init and main containers in this pod.
					ShareProcessNamespace: boolPtr(true),
					Tolerations: []corev1.Toleration{
						{
							Key:      "node.kubernetes.io/disk-pressure",
							Operator: corev1.TolerationOpExists,
						},
						{
							Key:      "node.kubernetes.io/memory-pressure",
							Operator: corev1.TolerationOpExists,
						},
						{
							Key:      "node.kubernetes.io/pid-pressure",
							Operator: corev1.TolerationOpExists,
						},
					},
					SecurityContext: &corev1.PodSecurityContext{
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					InitContainers: initContainers,
					Containers:     []corev1.Container{mainContainer, helperContainer},
					Volumes:        volumes,
				},
			},
		},
	}

	created, err := c.clientset.AppsV1().Deployments(c.sandboxNS).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		_ = c.clientset.CoreV1().PersistentVolumeClaims(c.sandboxNS).Delete(ctx, claimName, metav1.DeleteOptions{})
		return nil, fmt.Errorf("failed to create persistent sandbox deployment: %w", err)
	}
	return created, nil
}

func (c *Client) ensurePersistentVolumeClaim(ctx context.Context, claimName, storageClass, size string) error {
	_, err := c.clientset.CoreV1().PersistentVolumeClaims(c.sandboxNS).Get(ctx, claimName, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get pvc: %w", err)
	}

	req := corev1.ResourceList{
		corev1.ResourceStorage: resource.MustParse(size),
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      claimName,
			Namespace: c.sandboxNS,
			Labels: map[string]string{
				"app":          LabelApp,
				LabelSandboxID: strings.TrimPrefix(claimName, "sandbox-data-"),
				LabelManagedBy: ManagedByServer,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: req,
			},
		},
	}
	if storageClass != "" {
		pvc.Spec.StorageClassName = &storageClass
	}
	if _, err := c.clientset.CoreV1().PersistentVolumeClaims(c.sandboxNS).Create(ctx, pvc, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create pvc %s: %w", claimName, err)
	}
	return nil
}

func (c *Client) DeletePersistentSandbox(ctx context.Context, sandboxID, claimName, reclaimPolicy string) error {
	deployName := fmt.Sprintf("sandbox-%s", sandboxID)
	if err := c.clientset.AppsV1().Deployments(c.sandboxNS).Delete(ctx, deployName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete deployment %s: %w", deployName, err)
	}
	if reclaimPolicy == "Retain" {
		return nil
	}
	if claimName == "" {
		claimName = fmt.Sprintf("sandbox-data-%s", sandboxID)
	}
	if err := c.clientset.CoreV1().PersistentVolumeClaims(c.sandboxNS).Delete(ctx, claimName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete pvc %s: %w", claimName, err)
	}
	return nil
}

func (c *Client) PatchDeploymentFinalizers(ctx context.Context, name string, finalizers []string) error {
	deploy, err := c.clientset.AppsV1().Deployments(c.sandboxNS).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get deployment %s: %w", name, err)
	}
	deploy.Finalizers = append([]string(nil), finalizers...)
	if _, err := c.clientset.AppsV1().Deployments(c.sandboxNS).Update(ctx, deploy, metav1.UpdateOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to update deployment %s finalizers: %w", name, err)
	}
	return nil
}

func (c *Client) PatchPVCFinalizers(ctx context.Context, name string, finalizers []string) error {
	pvc, err := c.clientset.CoreV1().PersistentVolumeClaims(c.sandboxNS).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get pvc %s: %w", name, err)
	}
	pvc.Finalizers = append([]string(nil), finalizers...)
	if _, err := c.clientset.CoreV1().PersistentVolumeClaims(c.sandboxNS).Update(ctx, pvc, metav1.UpdateOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to update pvc %s finalizers: %w", name, err)
	}
	return nil
}

func (c *Client) GetPersistentVolume(ctx context.Context, name string) (*corev1.PersistentVolume, error) {
	pv, err := c.clientset.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pv, nil
}

func (c *Client) DeletePersistentVolume(ctx context.Context, name string) error {
	err := c.clientset.CoreV1().PersistentVolumes().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete pv %s: %w", name, err)
	}
	return nil
}

func (c *Client) PatchPVFinalizers(ctx context.Context, name string, finalizers []string) error {
	pv, err := c.clientset.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get pv %s: %w", name, err)
	}
	pv.Finalizers = append([]string(nil), finalizers...)
	if _, err := c.clientset.CoreV1().PersistentVolumes().Update(ctx, pv, metav1.UpdateOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to update pv %s finalizers: %w", name, err)
	}
	return nil
}

func (c *Client) ListVolumeAttachmentsByPV(ctx context.Context, pvName string) ([]storagev1.VolumeAttachment, error) {
	list, err := c.clientset.StorageV1().VolumeAttachments().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list volumeattachments: %w", err)
	}
	out := make([]storagev1.VolumeAttachment, 0)
	for i := range list.Items {
		item := list.Items[i]
		if item.Spec.Source.PersistentVolumeName != nil && *item.Spec.Source.PersistentVolumeName == pvName {
			out = append(out, item)
		}
	}
	return out, nil
}

func (c *Client) DeleteVolumeAttachment(ctx context.Context, name string) error {
	err := c.clientset.StorageV1().VolumeAttachments().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete volumeattachment %s: %w", name, err)
	}
	return nil
}

func (c *Client) GetSandboxDeletionSnapshot(ctx context.Context, sandboxID, deploymentName, pvcName string) (*SandboxDeletionSnapshot, error) {
	snapshot := &SandboxDeletionSnapshot{
		Pods: []corev1.Pod{},
	}
	if deploymentName == "" {
		deploymentName = fmt.Sprintf("sandbox-%s", sandboxID)
	}
	if pvcName == "" {
		pvcName = fmt.Sprintf("sandbox-data-%s", sandboxID)
	}
	deploy, err := c.GetDeployment(ctx, deploymentName)
	if err == nil {
		snapshot.Deployment = deploy
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get deployment %s: %w", deploymentName, err)
	}
	pods, err := c.ListSandboxPods(ctx, sandboxID)
	if err != nil {
		return nil, err
	}
	snapshot.Pods = pods
	pvc, err := c.GetPersistentVolumeClaim(ctx, pvcName)
	if err == nil {
		snapshot.PVC = pvc
		if pvc.Spec.VolumeName != "" {
			pv, pvErr := c.GetPersistentVolume(ctx, pvc.Spec.VolumeName)
			if pvErr == nil {
				snapshot.PV = pv
				attachments, attErr := c.ListVolumeAttachmentsByPV(ctx, pv.Name)
				if attErr != nil {
					return nil, attErr
				}
				snapshot.VolumeAttachments = attachments
			} else if !apierrors.IsNotFound(pvErr) {
				return nil, fmt.Errorf("failed to get pv %s: %w", pvc.Spec.VolumeName, pvErr)
			}
		}
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get pvc %s: %w", pvcName, err)
	}
	return snapshot, nil
}

// StopPersistentSandbox scales the sandbox Deployment replicas to 0.
func (c *Client) StopPersistentSandbox(ctx context.Context, sandboxID string) error {
	deployName := fmt.Sprintf("sandbox-%s", sandboxID)
	deploy, err := c.clientset.AppsV1().Deployments(c.sandboxNS).Get(ctx, deployName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return apierrors.NewNotFound(appsv1.Resource("deployments"), deployName)
		}
		return fmt.Errorf("failed to get deployment %s: %w", deployName, err)
	}
	replicas := int32(0)
	deploy.Spec.Replicas = &replicas
	if _, err := c.clientset.AppsV1().Deployments(c.sandboxNS).Update(ctx, deploy, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to scale deployment %s to 0: %w", deployName, err)
	}
	return nil
}

// StartPersistentSandbox scales the sandbox Deployment replicas back to 1.
func (c *Client) StartPersistentSandbox(ctx context.Context, sandboxID string) error {
	deployName := fmt.Sprintf("sandbox-%s", sandboxID)
	deploy, err := c.clientset.AppsV1().Deployments(c.sandboxNS).Get(ctx, deployName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return apierrors.NewNotFound(appsv1.Resource("deployments"), deployName)
		}
		return fmt.Errorf("failed to get deployment %s: %w", deployName, err)
	}
	replicas := int32(1)
	deploy.Spec.Replicas = &replicas
	if _, err := c.clientset.AppsV1().Deployments(c.sandboxNS).Update(ctx, deploy, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to scale deployment %s to 1: %w", deployName, err)
	}
	return nil
}

// RestartPersistentSandbox triggers a restart by deleting the current Pod managed by the sandbox Deployment.
func (c *Client) RestartPersistentSandbox(ctx context.Context, sandboxID string) error {
	deployName := fmt.Sprintf("sandbox-%s", sandboxID)
	if _, err := c.clientset.AppsV1().Deployments(c.sandboxNS).Get(ctx, deployName, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return apierrors.NewNotFound(appsv1.Resource("deployments"), deployName)
		}
		return fmt.Errorf("failed to get deployment %s: %w", deployName, err)
	}

	pod, err := c.getSandboxPod(ctx, sandboxID)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Deployment exists but Pod not observed yet, let controller continue reconciliation.
			return nil
		}
		return fmt.Errorf("failed to resolve sandbox pod: %w", err)
	}
	if err := c.clientset.CoreV1().Pods(c.sandboxNS).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete pod %s for restart: %w", pod.Name, err)
	}
	return nil
}

func (c *Client) ListSandboxPVCs(ctx context.Context) ([]corev1.PersistentVolumeClaim, error) {
	selector := labels.Set{
		"app": LabelApp,
	}.AsSelector().String()
	list, err := c.clientset.CoreV1().PersistentVolumeClaims(c.sandboxNS).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list sandbox pvcs: %w", err)
	}
	if list.Items == nil {
		return []corev1.PersistentVolumeClaim{}, nil
	}
	return list.Items, nil
}

func int64Ptr(v int64) *int64 {
	return &v
}

func buildRootfsOverlayPrepScript() string {
	return fmt.Sprintf(`
set -eu

ROOT=%q
STATE="$ROOT/%s"
CONTROL=%q

mkdir -p "$ROOT" "$STATE" "$CONTROL"
rm -f "$CONTROL"/main.info "$CONTROL"/mounted.* "$CONTROL"/failed.* "$CONTROL"/stop-request.* "$CONTROL"/unmounted.*
`, rootfsOverlayMountTarget, rootfsOverlayStateDir, rootfsOverlayControlDir)
}

func buildRootfsOverlayMainWrapperScript() string {
	return fmt.Sprintf(`
set -eu

ROOT=${LITEBOXD_ROOTFS_MOUNT_TARGET:-%q}
CONTROL=${LITEBOXD_ROOTFS_CONTROL_DIR:-%q}
MERGED="$ROOT/%s"
IFS=' ' read -r SELF_PID _ < /proc/self/stat
GEN="$(date +%%s)-$SELF_PID"
MAIN_INFO="$CONTROL/main.info"
MOUNTED_FILE="$CONTROL/mounted.$GEN"
FAILED_FILE="$CONTROL/failed.$GEN"
STOP_FILE="$CONTROL/stop-request.$GEN"
UNMOUNTED_FILE="$CONTROL/unmounted.$GEN"
CHILD_PID=""

log() {
  echo "[rootfs-main] $*"
}

request_unmount() {
  if [ -f "$STOP_FILE" ]; then
    return 0
  fi
  : > "$STOP_FILE"
  i=0
  while [ "$i" -lt 50 ]; do
    if [ -f "$UNMOUNTED_FILE" ]; then
      return 0
    fi
    i=$((i+1))
    sleep 0.1
  done
  log "timed out waiting for helper unmount acknowledgement"
  return 0
}

forward_term() {
  log "received stop signal"
  if [ -n "$CHILD_PID" ]; then
    kill -TERM "$CHILD_PID" 2>/dev/null || true
  fi
  request_unmount
}

trap forward_term TERM INT

mkdir -p "$CONTROL"
rm -f "$MOUNTED_FILE" "$FAILED_FILE" "$STOP_FILE" "$UNMOUNTED_FILE"
printf '%%s %%s\n' "$GEN" "$SELF_PID" > "$MAIN_INFO.tmp"
mv "$MAIN_INFO.tmp" "$MAIN_INFO"

i=0
while :; do
  if [ -f "$MOUNTED_FILE" ]; then
    break
  fi
  if [ -f "$FAILED_FILE" ]; then
    log "helper failed to mount rootfs"
    cat "$FAILED_FILE" >&2 || true
    exit 1
  fi
  i=$((i+1))
  if [ "$i" -ge 300 ]; then
    log "helper did not mount rootfs in time"
    exit 1
  fi
  sleep 0.2
done

log "rootfs ready, launching sandbox command"
if [ -n "${LITEBOXD_NOFILE_LIMIT:-}" ] && [ -x %q ]; then
  %q --nofile "${LITEBOXD_NOFILE_LIMIT}" -- chroot "$MERGED" "$@" &
else
  chroot "$MERGED" "$@" &
fi
CHILD_PID=$!
set +e
wait "$CHILD_PID"
STATUS=$?
set -e
CHILD_PID=""
request_unmount
exit "$STATUS"
`, rootfsOverlayMountTarget, rootfsOverlayControlDir, rootfsOverlayMergedSub, sandboxLauncherBinaryPath, sandboxLauncherBinaryPath)
}

func buildRootfsOverlayHelperScript() string {
	return fmt.Sprintf(`
set -eu

ROOT=%q
STATE="$ROOT/%s"
UPPER="$STATE/upper"
WORK="$STATE/work"
MERGED="$STATE/merged"
CONTROL=%q
HELPER_ROOT="/proc/self/root"
HELPER_PATH="$HELPER_ROOT/usr/local/sbin:$HELPER_ROOT/usr/local/bin:$HELPER_ROOT/usr/sbin:$HELPER_ROOT/usr/bin:$HELPER_ROOT/sbin:$HELPER_ROOT/bin"
TERM_REQUESTED=0
CURRENT_GEN=""
CURRENT_PID=""

log() {
  echo "[rootfs-helper] $*"
}

require_binary() {
  name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    log "required binary not found: $name"
    exit 1
  fi
}

target_sh() {
  target_pid="$1"
  shift
  nsenter -t "$target_pid" -m --root="/proc/$target_pid/root" --wd=/ -- "$HELPER_ROOT/bin/sh" -eu -c "PATH='$HELPER_PATH'; export PATH; $*"
}

cleanup_generation() {
  target_pid="$1"
  gen="$2"
  if [ -z "$gen" ]; then
    return 0
  fi
  log "unmounting overlay for generation $gen"
  if kill -0 "$target_pid" 2>/dev/null; then
    target_sh "$target_pid" '
for target in $(awk -v base="'"$MERGED"'" '\''$5 == base || index($5, base "/") == 1 { print $5 }'\'' /proc/self/mountinfo | sort -r); do
  umount -l "$target" 2>/dev/null || true
done
'
  fi
  : > "$CONTROL/unmounted.$gen"
  rm -f "$CONTROL/stop-request.$gen"
}

mount_generation() {
  target_pid="$1"
  gen="$2"
  mounted_file="$CONTROL/mounted.$gen"
  failed_file="$CONTROL/failed.$gen"
  rm -f "$mounted_file" "$failed_file" "$CONTROL/unmounted.$gen"
  log "mounting overlay for generation $gen into pid $target_pid"
  if target_sh "$target_pid" '
ROOT="'"$ROOT"'"
STATE="'"$STATE"'"
UPPER="'"$UPPER"'"
WORK="'"$WORK"'"
MERGED="'"$MERGED"'"

is_mounted() {
  target="$1"
  awk -v t="$target" '\''$5 == t { found = 1 } END { exit(found ? 0 : 1) }'\'' /proc/self/mountinfo
}

mkdir -p "$ROOT" "$STATE" "$UPPER" "$WORK" "$MERGED" "$MERGED/proc" "$MERGED/sys" "$MERGED/dev" "$MERGED/run" "$MERGED/etc"
if ! is_mounted "$MERGED"; then
  rm -rf "${WORK:?}/"* || true
  mount -t overlay overlay -o "lowerdir=/,upperdir=$UPPER,workdir=$WORK" "$MERGED"
fi
if ! is_mounted "$MERGED/proc"; then
  mount -t proc proc "$MERGED/proc"
fi
for dir in sys dev run; do
  target="$MERGED/$dir"
  if ! is_mounted "$target"; then
    mount --rbind "/$dir" "$target"
    mount --make-rslave "$target" || true
  fi
done
for file in resolv.conf hosts hostname; do
  target="$MERGED/etc/$file"
  touch "$target"
  if ! is_mounted "$target"; then
    mount --bind "/etc/$file" "$target"
  fi
done
test -d "$MERGED/proc"
test -f "$MERGED/etc/hosts"
'; then
    : > "$mounted_file"
    return 0
  fi

  {
    echo "mount failed for generation $gen"
    echo "--- target mountinfo tail ---"
    if kill -0 "$target_pid" 2>/dev/null; then
      target_sh "$target_pid" "tail -n 100 /proc/self/mountinfo || true"
    fi
  } > "$failed_file" 2>&1 || true
  return 1
}

trap 'TERM_REQUESTED=1' TERM INT

require_binary nsenter
require_binary mount
require_binary umount
require_binary sort
require_binary awk

mkdir -p "$CONTROL"

while :; do
  if [ -f "$CONTROL/main.info" ]; then
    read -r observed_gen observed_pid < "$CONTROL/main.info" || true
    if [ -n "${observed_gen:-}" ] && [ -n "${observed_pid:-}" ] && [ "$observed_gen" != "$CURRENT_GEN" ]; then
      CURRENT_GEN="$observed_gen"
      CURRENT_PID="$observed_pid"
      if ! mount_generation "$CURRENT_PID" "$CURRENT_GEN"; then
        CURRENT_GEN=""
        CURRENT_PID=""
      fi
    fi
  fi

  if [ -n "$CURRENT_GEN" ] && [ -f "$CONTROL/stop-request.$CURRENT_GEN" ]; then
    cleanup_generation "$CURRENT_PID" "$CURRENT_GEN"
    CURRENT_GEN=""
    CURRENT_PID=""
  fi

  if [ -n "$CURRENT_GEN" ] && ! kill -0 "$CURRENT_PID" 2>/dev/null; then
    : > "$CONTROL/unmounted.$CURRENT_GEN"
    CURRENT_GEN=""
    CURRENT_PID=""
  fi

  if [ "$TERM_REQUESTED" -eq 1 ]; then
    if [ -n "$CURRENT_GEN" ]; then
      cleanup_generation "$CURRENT_PID" "$CURRENT_GEN"
    fi
    exit 0
  fi

  sleep 0.2
done
`, rootfsOverlayMountTarget, rootfsOverlayStateDir, rootfsOverlayControlDir)
}

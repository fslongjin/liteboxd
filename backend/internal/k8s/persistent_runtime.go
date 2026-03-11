package k8s

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

const (
	rootfsOverlayStateDir   = ".liteboxd-overlay"
	rootfsOverlayMergedSub  = ".liteboxd-overlay/merged"
	rootfsOverlayInitName   = "rootfs-init"
	rootfsOverlayVolumeName = "rootfs"
	// Hidden mount target to avoid leaking internal implementation details in normal `ls /`.
	rootfsOverlayMountTarget = "/.liteboxd-rootfs"
	rootfsOverlayMergedPath  = rootfsOverlayMountTarget + "/" + rootfsOverlayMergedSub
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
	command, args, err := resolvePersistentStartCommand(ctx, opts)
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
	}
	if opts.Network != nil && opts.Network.AllowInternetAccess && len(opts.Network.AllowedDomains) == 0 {
		labels[LabelInternetAccess] = "true"
	}

	mainContainer := containerWithCommandAndArgs(opts.CreatePodOptions)
	mainContainer.VolumeMounts = []corev1.VolumeMount{
		{
			Name:             rootfsOverlayVolumeName,
			MountPath:        rootfsOverlayMountTarget,
			MountPropagation: mountPropagationPtr(corev1.MountPropagationHostToContainer),
		},
	}
	// Kubernetes/containerd cannot mount a volume directly to "/" in container rootfs.
	// Run the main process in chroot(<merged overlay root>) to preserve rootfs semantics.
	mainContainer.Command = append([]string{"chroot", rootfsOverlayMergedPath}, command...)
	mainContainer.Args = args

	initContainer := corev1.Container{
		Name:            rootfsOverlayInitName,
		Image:           opts.Image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"sh", "-ec", rootfsOverlayInitScript},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("300m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		},
		SecurityContext: &corev1.SecurityContext{
			// Only the helper/init container is privileged to perform overlay mount.
			Privileged:               boolPtr(true),
			AllowPrivilegeEscalation: boolPtr(true),
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:             rootfsOverlayVolumeName,
				MountPath:        rootfsOverlayMountTarget,
				MountPropagation: mountPropagationPtr(corev1.MountPropagationBidirectional),
			},
		},
	}

	replicas := int32(1)
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
					InitContainers: []corev1.Container{initContainer},
					Containers:     []corev1.Container{mainContainer},
					Volumes: []corev1.Volume{
						{
							Name: rootfsOverlayVolumeName,
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: claimName,
								},
							},
						},
					},
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

func resolvePersistentStartCommand(ctx context.Context, opts CreatePersistentSandboxOptions) ([]string, []string, error) {
	if len(opts.Command) > 0 {
		return append([]string(nil), opts.Command...), append([]string(nil), opts.Args...), nil
	}

	entrypoint, imageCmd, err := resolveImageEntrypointAndCmd(ctx, opts.Image)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve image entrypoint/cmd for persistent sandbox: %w", err)
	}

	if len(opts.Args) > 0 {
		if len(entrypoint) > 0 {
			return entrypoint, append([]string(nil), opts.Args...), nil
		}
		return append([]string(nil), opts.Args...), nil, nil
	}
	if len(entrypoint) > 0 {
		return entrypoint, imageCmd, nil
	}
	if len(imageCmd) > 0 {
		return imageCmd, nil, nil
	}

	return nil, nil, fmt.Errorf("unable to determine start command: template command is empty and image %q has no entrypoint/cmd", opts.Image)
}

func resolveImageEntrypointAndCmd(ctx context.Context, image string) ([]string, []string, error) {
	ref, err := name.ParseReference(image, name.WeakValidation)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid image reference %q: %w", image, err)
	}
	img, err := remote.Image(ref, remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch image metadata: %w", err)
	}
	cfg, err := img.ConfigFile()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read image config: %w", err)
	}

	entrypoint := append([]string(nil), cfg.Config.Entrypoint...)
	cmd := append([]string(nil), cfg.Config.Cmd...)
	return entrypoint, cmd, nil
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
				"app": LabelApp,
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

func mountPropagationPtr(v corev1.MountPropagationMode) *corev1.MountPropagationMode {
	return &v
}

const rootfsOverlayInitScript = `
set -eu

ROOT=/.liteboxd-rootfs
STATE="$ROOT/.liteboxd-overlay"
UPPER="$STATE/upper"
WORK="$STATE/work"
MERGED="$STATE/merged"

log() {
  echo "[rootfs-init] $*"
}

is_mounted() {
  target="$1"
  awk -v t="$target" '$5 == t { found = 1 } END { exit(found ? 0 : 1) }' /proc/self/mountinfo
}

ensure_proc_mount() {
  target="$1"
  mkdir -p "$target"
  if is_mounted "$target"; then
    return 0
  fi
  mount -t proc proc "$target"
}

ensure_bind_mount() {
  src="$1"
  target="$2"
  mkdir -p "$target"
  if is_mounted "$target"; then
    return 0
  fi
  mount --rbind "$src" "$target"
  mount --make-rslave "$target" || true
}

mkdir -p "$UPPER" "$WORK" "$MERGED"

# If this pod restart path already mounted overlay, keep existing writable state.
if is_mounted "$MERGED"; then
  log "overlay already mounted: $MERGED"
else
  # Workdir must be empty for overlay mount.
  rm -rf "${WORK:?}/"* || true

  # lowerdir keeps image filesystem readonly, upper/work are persisted on PVC.
  log "mounting overlay lowerdir=/ upperdir=$UPPER workdir=$WORK merged=$MERGED"
  if ! mount -t overlay overlay \
    -o "lowerdir=/,upperdir=$UPPER,workdir=$WORK" \
    "$MERGED"; then
    log "overlay mount failed"
    echo "[rootfs-init] --- /proc/self/mountinfo (tail) ---"
    tail -n 100 /proc/self/mountinfo || true
    echo "[rootfs-init] --- /proc/filesystems ---"
    cat /proc/filesystems || true
    exit 1
  fi
fi

# Mount pseudo filesystems into merged root so chroot runtime behaves like normal container rootfs.
ensure_proc_mount "$MERGED/proc"
ensure_bind_mount /sys "$MERGED/sys"
ensure_bind_mount /dev "$MERGED/dev"
ensure_bind_mount /run "$MERGED/run"

# Basic sanity check to fail fast when mount is not effective.
test -d "$MERGED/etc"
test -d "$MERGED/proc"
log "overlay mount ready"
`

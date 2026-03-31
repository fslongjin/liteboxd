package k8s

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	corev1 "k8s.io/api/core/v1"
)

const (
	DefaultSandboxNoFileLimit = 16384

	sandboxLauncherVolumeName      = "sandbox-launcher"
	sandboxLauncherInitName        = "launcher-init"
	sandboxLauncherMountDir        = "/.liteboxd-injected/launcher"
	sandboxLauncherBinaryPath      = sandboxLauncherMountDir + "/sandbox-launcher"
	sandboxLauncherImageBinaryPath = "/sandbox-launcher/sandbox-launcher"
)

var imageEntrypointResolver = resolveImageEntrypointAndCmd

func (c *Client) launcherEnabled() bool {
	return c.sandboxNoFileLimit > 0
}

func (c *Client) sandboxLauncherVolume() corev1.Volume {
	return corev1.Volume{
		Name: sandboxLauncherVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

func (c *Client) sandboxLauncherInitContainer() corev1.Container {
	return corev1.Container{
		Name:            sandboxLauncherInitName,
		Image:           c.sandboxLauncherImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command: []string{
			"/bin/sh",
			"-ec",
			fmt.Sprintf("cp %s %s && chmod 0755 %s", sandboxLauncherImageBinaryPath, sandboxLauncherBinaryPath, sandboxLauncherBinaryPath),
		},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: boolPtr(false),
			ReadOnlyRootFilesystem:   boolPtr(true),
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      sandboxLauncherVolumeName,
				MountPath: sandboxLauncherMountDir,
			},
		},
	}
}

func sandboxLauncherMount(readOnly bool) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      sandboxLauncherVolumeName,
		MountPath: sandboxLauncherMountDir,
		ReadOnly:  readOnly,
	}
}

func (c *Client) wrapCommandWithLauncher(command []string) []string {
	if !c.launcherEnabled() {
		return append([]string(nil), command...)
	}
	wrapped := []string{
		sandboxLauncherBinaryPath,
		"--nofile",
		strconv.Itoa(c.sandboxNoFileLimit),
		"--",
	}
	return append(wrapped, command...)
}

func (c *Client) buildLauncherWrappedStartCommand(ctx context.Context, opts CreatePodOptions) ([]string, []string, error) {
	command, args, err := resolveSandboxStartCommand(ctx, opts.Image, opts.Command, opts.Args)
	if err != nil {
		return nil, nil, err
	}
	start := append(append([]string(nil), command...), args...)
	wrapped := c.wrapCommandWithLauncher(start)
	return wrapped[:1], wrapped[1:], nil
}

func resolveSandboxStartCommand(ctx context.Context, image string, command []string, args []string) ([]string, []string, error) {
	if len(command) > 0 {
		return append([]string(nil), command...), append([]string(nil), args...), nil
	}

	entrypoint, imageCmd, err := imageEntrypointResolver(ctx, image)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve image entrypoint/cmd: %w", err)
	}

	if len(args) > 0 {
		if len(entrypoint) > 0 {
			return entrypoint, append([]string(nil), args...), nil
		}
		return append([]string(nil), args...), nil, nil
	}
	if len(entrypoint) > 0 {
		return entrypoint, imageCmd, nil
	}
	if len(imageCmd) > 0 {
		return imageCmd, nil, nil
	}

	return nil, nil, fmt.Errorf("unable to determine start command: template command is empty and image %q has no entrypoint/cmd", image)
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
	return append([]string(nil), cfg.Config.Entrypoint...), append([]string(nil), cfg.Config.Cmd...), nil
}

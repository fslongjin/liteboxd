package installer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (i *Installer) buildDeployWorkspace() (string, func(), error) {
	workspace, err := os.MkdirTemp("", "liteboxd-installer-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp workspace: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(workspace) }

	if err := extractEmbeddedDeploy(workspace); err != nil {
		cleanup()
		return "", nil, err
	}

	if err := copyOptionalDir(filepath.Join(i.cfg.LiteBoxd.ConfigDir, "system", "patches"), filepath.Join(workspace, "config", "system", "patches")); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := copyOptionalDir(filepath.Join(i.cfg.LiteBoxd.ConfigDir, "sandbox", "patches"), filepath.Join(workspace, "config", "sandbox", "patches")); err != nil {
		cleanup()
		return "", nil, err
	}

	if err := os.MkdirAll(filepath.Join(workspace, "system-overlay"), 0o755); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := os.WriteFile(filepath.Join(workspace, "system-overlay", "kustomization.yaml"), []byte(i.systemOverlayKustomization()), 0o644); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write system overlay: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "sandbox-overlay"), 0o755); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := os.WriteFile(filepath.Join(workspace, "sandbox-overlay", "kustomization.yaml"), []byte(i.sandboxOverlayKustomization()), 0o644); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write sandbox overlay: %w", err)
	}
	return workspace, cleanup, nil
}

func (i *Installer) systemOverlayKustomization() string {
	sysNS := i.cfg.LiteBoxd.NamespaceSystem
	sandboxNS := i.cfg.LiteBoxd.NamespaceSandbox
	gatewayURL := fmt.Sprintf("http://liteboxd-gateway.%s.svc.cluster.local:8081", sysNS)
	if strings.TrimSpace(i.cfg.LiteBoxd.GatewayURL) != "" {
		gatewayURL = strings.TrimSpace(i.cfg.LiteBoxd.GatewayURL)
	}

	var b strings.Builder
	b.WriteString("apiVersion: kustomize.config.k8s.io/v1beta1\n")
	b.WriteString("kind: Kustomization\n")
	b.WriteString("namespace: " + sysNS + "\n")
	b.WriteString("resources:\n")
	b.WriteString("  - ../base/system\n")
	b.WriteString("patches:\n")
	b.WriteString("  - target:\n      kind: Namespace\n      name: liteboxd-system\n    patch: |-\n      - op: replace\n        path: /metadata/name\n        value: " + sysNS + "\n")
	b.WriteString("  - target:\n      kind: ClusterRoleBinding\n      name: liteboxd-api-cluster\n    patch: |-\n      - op: replace\n        path: /subjects/0/namespace\n        value: " + sysNS + "\n")
	b.WriteString("  - target:\n      kind: ConfigMap\n      name: liteboxd-config\n    patch: |-\n      - op: replace\n        path: /data/SANDBOX_NAMESPACE\n        value: " + sandboxNS + "\n      - op: replace\n        path: /data/CONTROL_NAMESPACE\n        value: " + sysNS + "\n      - op: replace\n        path: /data/GATEWAY_URL\n        value: " + gatewayURL + "\n")
	b.WriteString("  - target:\n      kind: Ingress\n      name: liteboxd\n    patch: |-\n      - op: replace\n        path: /spec/rules/0/host\n        value: " + i.cfg.LiteBoxd.IngressHost + "\n")
	b.WriteString("  - target:\n      kind: Deployment\n      name: liteboxd-api\n    patch: |-\n      - op: replace\n        path: /spec/template/spec/containers/0/image\n        value: " + i.cfg.LiteBoxd.Images.API + "\n")
	b.WriteString("  - target:\n      kind: Deployment\n      name: liteboxd-gateway\n    patch: |-\n      - op: replace\n        path: /spec/template/spec/containers/0/image\n        value: " + i.cfg.LiteBoxd.Images.Gateway + "\n")
	b.WriteString("  - target:\n      kind: Deployment\n      name: liteboxd-web\n    patch: |-\n      - op: replace\n        path: /spec/template/spec/containers/0/image\n        value: " + i.cfg.LiteBoxd.Images.Web + "\n")

	for _, p := range listPatchFiles(filepath.Join(i.cfg.LiteBoxd.ConfigDir, "system", "patches")) {
		b.WriteString("  - path: ../config/system/patches/" + filepath.Base(p) + "\n")
	}
	return b.String()
}

func (i *Installer) sandboxOverlayKustomization() string {
	sysNS := i.cfg.LiteBoxd.NamespaceSystem
	sandboxNS := i.cfg.LiteBoxd.NamespaceSandbox

	var b strings.Builder
	b.WriteString("apiVersion: kustomize.config.k8s.io/v1beta1\n")
	b.WriteString("kind: Kustomization\n")
	b.WriteString("namespace: " + sandboxNS + "\n")
	b.WriteString("resources:\n")
	b.WriteString("  - ../base/sandbox\n")
	b.WriteString("patches:\n")
	b.WriteString("  - target:\n      kind: Namespace\n      name: liteboxd-sandbox\n    patch: |-\n      - op: replace\n        path: /metadata/name\n        value: " + sandboxNS + "\n")
	b.WriteString("  - target:\n      kind: RoleBinding\n      name: liteboxd-api\n    patch: |-\n      - op: replace\n        path: /subjects/0/namespace\n        value: " + sysNS + "\n")
	b.WriteString("  - target:\n      kind: RoleBinding\n      name: liteboxd-gateway\n    patch: |-\n      - op: replace\n        path: /subjects/0/namespace\n        value: " + sysNS + "\n")

	for _, p := range listPatchFiles(filepath.Join(i.cfg.LiteBoxd.ConfigDir, "sandbox", "patches")) {
		b.WriteString("  - path: ../config/sandbox/patches/" + filepath.Base(p) + "\n")
	}
	return b.String()
}

func copyOptionalDir(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", src, err)
	}
	if !st.IsDir() {
		return nil
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dst, err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("readdir %s: %w", src, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasSuffix(n, ".yaml") && !strings.HasSuffix(n, ".yml") {
			continue
		}
		b, readErr := os.ReadFile(filepath.Join(src, n))
		if readErr != nil {
			return fmt.Errorf("read patch file %s: %w", filepath.Join(src, n), readErr)
		}
		if err := os.WriteFile(filepath.Join(dst, n), b, 0o644); err != nil {
			return fmt.Errorf("write patch file %s: %w", filepath.Join(dst, n), err)
		}
	}
	return nil
}

func listPatchFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasSuffix(n, ".yaml") || strings.HasSuffix(n, ".yml") {
			files = append(files, filepath.Join(dir, n))
		}
	}
	sort.Strings(files)
	return files
}

func buildTarGz(root string) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)

		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = rel
		if d.IsDir() {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := io.Copy(tw, f); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		_ = tw.Close()
		_ = gz.Close()
		return nil, fmt.Errorf("build tar.gz: %w", err)
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

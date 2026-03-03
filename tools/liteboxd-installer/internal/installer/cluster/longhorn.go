package cluster

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/fslongjin/liteboxd/tools/liteboxd-installer/internal/config"
)

func (m *Manager) InstallOrUpgradeLonghorn() error {
	longhorn := m.cfg.Storage.Longhorn
	if !longhorn.Enabled {
		m.logf("longhorn disabled, skip")
		return nil
	}

	if err := m.ensureLonghornPrerequisitesOnMaster(); err != nil {
		return err
	}
	if err := m.ensureLonghornPrerequisitesOnAgents(); err != nil {
		return err
	}
	if err := m.ensureHelmCLI(); err != nil {
		return err
	}

	settings := []string{
		fmt.Sprintf("--set defaultSettings.defaultReplicaCount=%d", longhorn.DefaultReplicaCount),
		fmt.Sprintf("--set persistence.defaultClass=%t", m.longhornSetDefaultStorageClass(longhorn)),
	}
	fingerprint := longhornFingerprint(
		longhorn.Namespace,
		longhorn.ReleaseName,
		longhorn.ChartRepoURL,
		longhorn.ChartVersion,
		settings,
	)

	stateNamespace := "kube-system"
	stateConfigMap := "liteboxd-installer-longhorn-state"
	stateKey := "longhornFingerprint"
	upgradeOrInstall := fmt.Sprintf(`
set -euo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
DESIRED=%s
RELEASE=%s
NAMESPACE=%s
CHART_VERSION=%s
STATE_NS=%s
STATE_CM=%s
STATE_KEY=%s
CURRENT="$(kubectl -n "$STATE_NS" get configmap "$STATE_CM" -o jsonpath="{.data.$STATE_KEY}" 2>/dev/null || true)"
INSTALLED=0
if helm -n "$NAMESPACE" status "$RELEASE" >/dev/null 2>&1; then
  INSTALLED=1
fi
if [ "$INSTALLED" = "1" ] && [ "$CURRENT" = "$DESIRED" ]; then
  echo "longhorn_unchanged_skip"
  exit 0
fi
helm repo add longhorn %s >/dev/null 2>&1 || true
helm repo update longhorn >/dev/null
VERSION_ARG=""
if [ -n "$CHART_VERSION" ]; then
  VERSION_ARG="--version $CHART_VERSION"
fi
helm upgrade --install "$RELEASE" longhorn/longhorn \
  -n "$NAMESPACE" \
  --create-namespace \
  $VERSION_ARG \
  %s
for r in deploy/longhorn-driver-deployer deploy/longhorn-ui ds/longhorn-manager ds/longhorn-csi-plugin; do
  if kubectl -n "$NAMESPACE" get "$r" >/dev/null 2>&1; then
    kubectl -n "$NAMESPACE" rollout status "$r" --timeout=15m
  fi
done
kubectl -n "$STATE_NS" create configmap "$STATE_CM" --from-literal="$STATE_KEY=$DESIRED" --dry-run=client -o yaml | kubectl apply -f -
`, shellQuote(fingerprint), shellQuote(longhorn.ReleaseName), shellQuote(longhorn.Namespace), shellQuote(longhorn.ChartVersion), shellQuote(stateNamespace), shellQuote(stateConfigMap), shellQuote(stateKey), shellQuote(longhorn.ChartRepoURL), strings.Join(settings, " \\\n  "))

	out, err := m.runMaster(upgradeOrInstall, true)
	if err != nil {
		return fmt.Errorf("install/upgrade longhorn: %w", err)
	}
	if strings.Contains(out, "longhorn_unchanged_skip") {
		m.logf("longhorn config unchanged, skip install/upgrade")
	}
	return nil
}

func (m *Manager) longhornSetDefaultStorageClass(l config.LonghornConfig) bool {
	if l.SetDefaultStorageClass == nil {
		return true
	}
	return *l.SetDefaultStorageClass
}

func (m *Manager) ensureHelmCLI() error {
	cmd := strings.Join([]string{
		"if ! command -v helm >/dev/null 2>&1; then",
		fmt.Sprintf("  curl -fsSL %s | bash", shellQuote(m.cfg.Storage.Longhorn.HelmInstallScriptURL)),
		"fi",
	}, "\n")
	if _, err := m.runMaster(cmd, true); err != nil {
		return fmt.Errorf("install helm cli: %w", err)
	}
	return nil
}

func (m *Manager) ensureLonghornPrerequisitesOnMaster() error {
	if _, err := m.runMaster(longhornPrerequisitesScript(), true); err != nil {
		return fmt.Errorf("ensure longhorn prerequisites on master: %w", err)
	}
	return nil
}

func (m *Manager) ensureLonghornPrerequisitesOnAgents() error {
	if len(m.cfg.Cluster.Agents) == 0 {
		return nil
	}

	sem := make(chan struct{}, m.cfg.Runtime.Parallelism)
	errCh := make(chan error, len(m.cfg.Cluster.Agents))
	var wg sync.WaitGroup

	for _, agent := range m.cfg.Cluster.Agents {
		agent := agent
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := m.ensureLonghornPrerequisitesOnAgent(agent); err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) ensureLonghornPrerequisitesOnAgent(agent config.NodeConfig) error {
	if m.opts.DryRun {
		m.logf("dry-run ensure longhorn prerequisites host=%s", agent.Host)
		return nil
	}
	client, err := m.dialAgent(agent)
	if err != nil {
		return err
	}
	defer client.Close()

	cmd := longhornPrerequisitesScript()
	res, err := client.Run(cmd, true)
	m.detailf("target=%s sudo=true command=%s", agent.Host, sanitizeCommand(cmd))
	if res.Stdout != "" {
		m.detailf("target=%s stdout=%s", agent.Host, sanitizeOutput(cmd, res.Stdout))
	}
	if res.Stderr != "" {
		m.detailf("target=%s stderr=%s", agent.Host, sanitizeOutput(cmd, res.Stderr))
	}
	if err != nil {
		return fmt.Errorf("ensure longhorn prerequisites host=%s: %w", agent.Host, err)
	}
	return nil
}

func longhornPrerequisitesScript() string {
	return strings.Join([]string{
		"set -euo pipefail",
		"if ! command -v iscsiadm >/dev/null 2>&1; then",
		"  if command -v apt-get >/dev/null 2>&1; then",
		"    export DEBIAN_FRONTEND=noninteractive",
		"    apt-get update -y",
		"    apt-get install -y open-iscsi",
		"  elif command -v dnf >/dev/null 2>&1; then",
		"    dnf install -y iscsi-initiator-utils",
		"  elif command -v yum >/dev/null 2>&1; then",
		"    yum install -y iscsi-initiator-utils",
		"  elif command -v zypper >/dev/null 2>&1; then",
		"    zypper --non-interactive install open-iscsi",
		"  else",
		"    echo 'unsupported package manager for open-iscsi installation' >&2",
		"    exit 1",
		"  fi",
		"fi",
		"if command -v systemctl >/dev/null 2>&1; then",
		"  if systemctl list-unit-files | grep -q '^iscsid\\.service'; then systemctl enable --now iscsid; fi",
		"  if systemctl list-unit-files | grep -q '^open-iscsi\\.service'; then systemctl enable --now open-iscsi; fi",
		"  if systemctl list-unit-files | grep -q '^iscsi\\.service'; then systemctl enable --now iscsi; fi",
		"fi",
	}, "\n")
}

func longhornFingerprint(namespace, release, chartRepoURL, chartVersion string, settings []string) string {
	raw := strings.Join([]string{
		namespace,
		release,
		chartRepoURL,
		chartVersion,
		strings.Join(settings, "\n"),
	}, "\n")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

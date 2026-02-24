package cluster

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func (m *Manager) InstallOrUpgradeCilium() error {
	cilium := m.cfg.Network.Cilium
	installCLI := strings.Join([]string{
		"if ! command -v cilium >/dev/null 2>&1; then",
		fmt.Sprintf("  CILIUM_CLI_VERSION=%s", shellQuote(cilium.CLIVersion)),
		"  CLI_ARCH=amd64",
		"  if [ \"$(uname -m)\" = \"aarch64\" ]; then CLI_ARCH=arm64; fi",
		fmt.Sprintf("  curl -L --fail --remote-name-all %s/${CILIUM_CLI_VERSION}/cilium-linux-${CLI_ARCH}.tar.gz{,.sha256sum}", shellQuote(cilium.CLIDownloadBaseURL)),
		"  sha256sum --check cilium-linux-${CLI_ARCH}.tar.gz.sha256sum",
		"  tar xzvfC cilium-linux-${CLI_ARCH}.tar.gz /usr/local/bin",
		"  rm cilium-linux-${CLI_ARCH}.tar.gz{,.sha256sum}",
		"fi",
	}, "\n")
	if _, err := m.runMaster(installCLI, true); err != nil {
		return fmt.Errorf("install cilium cli: %w", err)
	}

	settings := []string{
		fmt.Sprintf("--set k8sServiceHost=%s", shellQuote(m.cfg.Cluster.Master.NodeIP)),
		"--set k8sServicePort=6443",
		fmt.Sprintf("--set ipam.operator.clusterPoolIPv4PodCIDRList=%s", shellQuote(cilium.PodCIDR)),
		fmt.Sprintf("--set egressGateway.enabled=%t", cilium.EnableEgressGateway),
		"--set bpf.masquerade=true",
		fmt.Sprintf("--set kubeProxyReplacement=%t", cilium.KubeProxyReplacement),
	}
	fingerprint := ciliumFingerprint(cilium.Version, settings)
	stateNamespace := "kube-system"
	stateConfigMap := "liteboxd-installer-state"
	stateKey := "ciliumFingerprint"

	upgradeOrInstall := fmt.Sprintf(`
set -euo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
DESIRED=%s
STATE_NS=%s
STATE_CM=%s
STATE_KEY=%s
INSTALLED=0
if kubectl -n kube-system get ds cilium >/dev/null 2>&1; then
  INSTALLED=1
fi
CURRENT=""
CURRENT="$(kubectl -n "$STATE_NS" get configmap "$STATE_CM" -o jsonpath="{.data.$STATE_KEY}" 2>/dev/null || true)"
if [ "$INSTALLED" = "1" ] && [ "$CURRENT" = "$DESIRED" ]; then
  echo "cilium_unchanged_skip"
  exit 0
fi
if [ "$INSTALLED" = "1" ]; then
  cilium upgrade --version %s --reuse-values %s
else
  cilium install --version %s %s
fi
cilium status --wait
kubectl -n "$STATE_NS" create configmap "$STATE_CM" --from-literal="$STATE_KEY=$DESIRED" --dry-run=client -o yaml | kubectl apply -f -
		`, shellQuote(fingerprint), shellQuote(stateNamespace), shellQuote(stateConfigMap), shellQuote(stateKey), shellQuote(cilium.Version), strings.Join(settings, " "), shellQuote(cilium.Version), strings.Join(settings, " "))

	out, err := m.runMaster(upgradeOrInstall, true)
	if err != nil {
		return fmt.Errorf("install/upgrade cilium: %w", err)
	}
	if strings.Contains(out, "cilium_unchanged_skip") {
		m.logf("cilium config unchanged, skip install/upgrade")
	}
	return nil
}

func ciliumFingerprint(version string, settings []string) string {
	raw := version + "\n" + strings.Join(settings, "\n")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

package cluster

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/fslongjin/liteboxd/tools/liteboxd-installer/internal/config"
)

func (m *Manager) InstallOrCheckMaster() error {
	cmd := "if systemctl is-active --quiet k3s; then echo installed; else echo missing; fi"
	res, err := m.runMaster(cmd, true)
	if err != nil {
		return err
	}
	if strings.Contains(res, "installed") {
		m.logf("k3s server already installed on master=%s", m.cfg.Cluster.Master.Host)
	} else {
		installArgs := append([]string{}, m.cfg.Cluster.Master.K3s.InstallArgs...)
		for _, san := range m.cfg.Cluster.Master.K3s.TLSSAN {
			installArgs = append(installArgs, "--tls-san="+san)
		}
		installCmd := fmt.Sprintf(
			"curl -sfL %s | %sINSTALL_K3S_VERSION=%s INSTALL_K3S_EXEC=%s sh -",
			shellQuote(m.cfg.Cluster.K3sInstall.ScriptURL),
			m.k3sMirrorEnvPrefix(),
			shellQuote(m.cfg.Cluster.Master.K3s.Version),
			shellQuote(strings.Join(installArgs, " ")),
		)
		if _, err := m.runMaster(installCmd, true); err != nil {
			return fmt.Errorf("install k3s server: %w", err)
		}
	}

	fixKubeconfigCmd := fmt.Sprintf(
		"if [ -f /etc/rancher/k3s/k3s.yaml ]; then sed -i -E 's#^(\\s*server:\\s*)https://[^:]+:6443$#\\1https://%s:6443#' /etc/rancher/k3s/k3s.yaml; fi",
		m.cfg.Cluster.Master.NodeIP,
	)
	if _, err := m.runMaster(fixKubeconfigCmd, true); err != nil {
		return fmt.Errorf("update kubeconfig endpoint: %w", err)
	}
	if err := m.ensureRootProfileKubeconfig(); err != nil {
		return err
	}
	return nil
}

func (m *Manager) ReadNodeToken() (string, error) {
	if m.opts.DryRun {
		return "dry-run-token", nil
	}
	out, err := m.runMaster("cat /var/lib/rancher/k3s/server/node-token", true)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(out)
	if token == "" {
		return "", errors.New("empty k3s node token")
	}
	return token, nil
}

func (m *Manager) JoinAgents(token string) error {
	if len(m.cfg.Cluster.Agents) == 0 {
		m.logf("no agents configured, skip")
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
			if err := m.ensureAgentJoined(agent, token); err != nil {
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

func (m *Manager) ensureAgentJoined(agent config.NodeConfig, token string) error {
	if m.opts.DryRun {
		m.logf("dry-run join agent host=%s", agent.Host)
		return nil
	}

	client, err := m.dialAgent(agent)
	if err != nil {
		return err
	}
	defer client.Close()

	res, err := client.Run("if systemctl is-active --quiet k3s-agent; then echo installed; else echo missing; fi", true)
	m.detailf("target=%s sudo=true command=%s", agent.Host, "if systemctl is-active --quiet k3s-agent; then echo installed; else echo missing; fi")
	if res.Stdout != "" {
		m.detailf("target=%s stdout=%s", agent.Host, sanitizeOutput("check-k3s-agent", res.Stdout))
	}
	if res.Stderr != "" {
		m.detailf("target=%s stderr=%s", agent.Host, sanitizeOutput("check-k3s-agent", res.Stderr))
	}
	if err == nil && strings.Contains(res.Stdout, "installed") {
		m.logf("agent already installed host=%s", agent.Host)
		return nil
	}

	cmd := fmt.Sprintf(
		"curl -sfL %s | %sINSTALL_K3S_VERSION=%s K3S_URL=%s K3S_TOKEN=%s sh -",
		shellQuote(m.cfg.Cluster.K3sInstall.ScriptURL),
		m.k3sMirrorEnvPrefix(),
		shellQuote(m.cfg.Cluster.Master.K3s.Version),
		shellQuote("https://"+m.cfg.Cluster.Master.NodeIP+":6443"),
		shellQuote(token),
	)
	res, err = client.Run(cmd, true)
	m.detailf("target=%s sudo=true command=%s", agent.Host, sanitizeCommand(cmd))
	if res.Stdout != "" {
		m.detailf("target=%s stdout=%s", agent.Host, sanitizeOutput(cmd, res.Stdout))
	}
	if res.Stderr != "" {
		m.detailf("target=%s stderr=%s", agent.Host, sanitizeOutput(cmd, res.Stderr))
	}
	if err != nil {
		return fmt.Errorf("install k3s agent %s: %w", agent.Host, err)
	}
	return nil
}

func (m *Manager) WaitNodesReady() error {
	cmd := strings.Join([]string{
		"export KUBECONFIG=/etc/rancher/k3s/k3s.yaml",
		"for i in $(seq 1 90); do",
		"  not_ready=$(kubectl get nodes --no-headers 2>/dev/null | awk '$2 !~ /Ready/ {print $1}')",
		"  if [ -z \"$not_ready\" ]; then exit 0; fi",
		"  sleep 5",
		"done",
		"kubectl get nodes -o wide",
		"exit 1",
	}, "\n")
	_, err := m.runMaster(cmd, true)
	return err
}

func (m *Manager) k3sMirrorEnvPrefix() string {
	if m.cfg.Cluster.K3sInstall.Mirror == "cn" {
		return "INSTALL_K3S_MIRROR=cn "
	}
	return ""
}

func (m *Manager) ensureRootProfileKubeconfig() error {
	cmd := strings.Join([]string{
		"if ! grep -Eq '^\\s*(export\\s+)?KUBECONFIG=' /etc/profile; then",
		"  echo 'export KUBECONFIG=/etc/rancher/k3s/k3s.yaml' >> /etc/profile",
		"fi",
	}, "\n")
	if _, err := m.runMaster(cmd, true); err != nil {
		return fmt.Errorf("ensure /etc/profile has KUBECONFIG: %w", err)
	}
	return nil
}

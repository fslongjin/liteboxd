package cluster

import (
	"encoding/base64"
	"fmt"
	"sync"

	"github.com/fslongjin/liteboxd/tools/liteboxd-installer/internal/config"
	"github.com/fslongjin/liteboxd/tools/liteboxd-installer/internal/sshx"
	"gopkg.in/yaml.v3"
)

func (m *Manager) ConfigureRegistriesAllNodes() error {
	if len(m.cfg.Cluster.Registries.Mirrors) == 0 {
		m.logf("registries config empty, skip")
		return nil
	}

	content, err := yaml.Marshal(m.cfg.Cluster.Registries)
	if err != nil {
		return fmt.Errorf("marshal registries config: %w", err)
	}

	if err := m.configureRegistriesOnMaster(content); err != nil {
		return err
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
			if err := m.configureRegistriesOnAgent(agent, content); err != nil {
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

func (m *Manager) configureRegistriesOnMaster(content []byte) error {
	if m.opts.DryRun {
		m.logf("dry-run configure registries host=%s", m.cfg.Cluster.Master.Host)
		return nil
	}
	client, err := m.dialMaster()
	if err != nil {
		m.detailf("target=%s dial_error=%v", m.cfg.Cluster.Master.Host, err)
		return fmt.Errorf("connect host=%s for registries: %w", m.cfg.Cluster.Master.Host, err)
	}
	defer client.Close()
	return m.applyRegistriesConfig(m.cfg.Cluster.Master.Host, client, content)
}

func (m *Manager) configureRegistriesOnAgent(agent config.NodeConfig, content []byte) error {
	if m.opts.DryRun {
		m.logf("dry-run configure registries host=%s", agent.Host)
		return nil
	}
	client, err := m.dialAgent(agent)
	if err != nil {
		return fmt.Errorf("connect host=%s for registries via master: %w", agent.Host, err)
	}
	defer client.Close()
	return m.applyRegistriesConfig(agent.Host, client, content)
}

func (m *Manager) applyRegistriesConfig(host string, client *sshx.Client, content []byte) error {
	encoded := base64.StdEncoding.EncodeToString(content)
	cmd := fmt.Sprintf(`set -euo pipefail
mkdir -p /etc/rancher/k3s
TMP_FILE=$(mktemp)
printf '%%s' %s | base64 -d > "$TMP_FILE"
CHANGED=0
if [ ! -f /etc/rancher/k3s/registries.yaml ] || ! cmp -s "$TMP_FILE" /etc/rancher/k3s/registries.yaml; then
  cp "$TMP_FILE" /etc/rancher/k3s/registries.yaml
  CHANGED=1
fi
rm -f "$TMP_FILE"
if [ "$CHANGED" = "1" ]; then
  if systemctl list-unit-files | grep -q '^k3s\.service'; then systemctl restart k3s; fi
  if systemctl list-unit-files | grep -q '^k3s-agent\.service'; then systemctl restart k3s-agent; fi
fi
echo "$CHANGED"`, shellQuote(encoded))

	res, err := client.Run(cmd, true)
	m.detailf("target=%s sudo=true command=%s", host, sanitizeCommand(cmd))
	if res.Stdout != "" {
		m.detailf("target=%s stdout=%s", host, sanitizeOutput(cmd, res.Stdout))
	}
	if res.Stderr != "" {
		m.detailf("target=%s stderr=%s", host, sanitizeOutput(cmd, res.Stderr))
	}
	if err != nil {
		return fmt.Errorf("configure registries host=%s: %w", host, err)
	}
	m.logf("configured registries host=%s changed=%s", host, res.Stdout)
	return nil
}

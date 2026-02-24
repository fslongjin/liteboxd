package cluster

import (
	"fmt"
	"sort"
	"strings"
)

func (m *Manager) ReconcileRemoveAbsentAgents() error {
	desired := map[string]struct{}{}
	for _, h := range m.cfg.DesiredAgentNodeIPs() {
		desired[h] = struct{}{}
	}

	hostToNode, err := m.clusterNodeMap()
	if err != nil {
		return err
	}

	var stale []string
	for host := range hostToNode {
		if host == m.cfg.Cluster.Master.NodeIP {
			continue
		}
		if _, ok := desired[host]; !ok {
			stale = append(stale, host)
		}
	}
	sort.Strings(stale)
	if len(stale) == 0 {
		return nil
	}

	for _, host := range stale {
		m.logf("remove absent agent host=%s (cluster node only)", host)
		if err := m.removeNodeByHost(host, false); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) RemoveNodes(hosts []string, uninstallAgent bool) error {
	if len(hosts) == 0 {
		return fmt.Errorf("remove nodes requires at least one host")
	}
	for _, host := range hosts {
		if err := m.removeNodeByHost(host, uninstallAgent); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) removeNodeByHost(host string, uninstallAgent bool) error {
	hostToNode, err := m.clusterNodeMap()
	if err != nil {
		return err
	}
	lookupIP := host
	if agent, ok := m.findAgentByHostOrNodeIP(host); ok {
		lookupIP = agent.NodeIP
	}

	node, ok := hostToNode[lookupIP]
	if !ok {
		return fmt.Errorf("cannot find kubernetes node for host=%s (lookup nodeIP=%s)", host, lookupIP)
	}
	if node == "" {
		return fmt.Errorf("resolved empty node name for host=%s", host)
	}
	if host == m.cfg.Cluster.Master.Host || host == m.cfg.Cluster.Master.NodeIP || lookupIP == m.cfg.Cluster.Master.NodeIP {
		return fmt.Errorf("refuse to remove master host=%s", host)
	}

	for _, cmd := range []string{
		fmt.Sprintf("KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl cordon %s", shellQuote(node)),
		fmt.Sprintf("KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl drain %s --ignore-daemonsets --delete-emptydir-data --force", shellQuote(node)),
		fmt.Sprintf("KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl delete node %s", shellQuote(node)),
	} {
		if _, err := m.runMaster(cmd, true); err != nil {
			return fmt.Errorf("remove node host=%s node=%s: %w", host, node, err)
		}
	}

	if !uninstallAgent {
		return nil
	}

	agent, ok := m.findAgentByHostOrNodeIP(host)
	if !ok {
		m.logf("warn: host=%s not found in config agents, skip remote uninstall", host)
		return nil
	}
	if m.opts.DryRun {
		m.logf("dry-run uninstall k3s-agent host=%s", host)
		return nil
	}

	client, err := m.dialAgent(agent)
	if err != nil {
		return fmt.Errorf("connect removed host=%s via master: %w", host, err)
	}
	defer client.Close()
	if _, err := client.Run("if command -v k3s-agent-uninstall.sh >/dev/null 2>&1; then k3s-agent-uninstall.sh; fi", true); err != nil {
		return fmt.Errorf("uninstall k3s-agent host=%s: %w", host, err)
	}
	return nil
}

func (m *Manager) clusterNodeMap() (map[string]string, error) {
	out, err := m.runMaster(`KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl get nodes -o jsonpath='{range .items[*]}{.metadata.name}{"|"}{range .status.addresses[?(@.type=="InternalIP")]}{.address}{end}{"\n"}{end}'`, true)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 2 {
			continue
		}
		node := strings.TrimSpace(parts[0])
		host := strings.TrimSpace(parts[1])
		if node != "" && host != "" {
			result[host] = node
		}
	}
	return result, nil
}

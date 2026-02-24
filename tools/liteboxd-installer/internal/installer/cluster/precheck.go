package cluster

import (
	"fmt"
	"sync"

	"github.com/fslongjin/liteboxd/tools/liteboxd-installer/internal/config"
)

func (m *Manager) Precheck() error {
	if err := m.checkMasterSSH(); err != nil {
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
			if err := m.checkAgentSSH(agent); err != nil {
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

func (m *Manager) checkMasterSSH() error {
	if m.opts.DryRun {
		m.logf("dry-run ssh check host=%s", m.cfg.Cluster.Master.Host)
		return nil
	}
	c, err := m.dialMaster()
	if err != nil {
		return fmt.Errorf("ssh connect %s: %w", m.cfg.Cluster.Master.Host, err)
	}
	defer c.Close()
	if _, err := c.Run("echo ok", false); err != nil {
		return fmt.Errorf("ssh command check %s: %w", m.cfg.Cluster.Master.Host, err)
	}
	return nil
}

func (m *Manager) checkAgentSSH(agent config.NodeConfig) error {
	if m.opts.DryRun {
		m.logf("dry-run ssh check host=%s", agent.Host)
		return nil
	}
	c, err := m.dialAgent(agent)
	if err != nil {
		return fmt.Errorf("ssh connect %s: %w", agent.Host, err)
	}
	defer c.Close()
	if _, err := c.Run("echo ok", false); err != nil {
		return fmt.Errorf("ssh command check %s: %w", agent.Host, err)
	}
	return nil
}

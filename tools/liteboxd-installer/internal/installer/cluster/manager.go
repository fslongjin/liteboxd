package cluster

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/fslongjin/liteboxd/tools/liteboxd-installer/internal/config"
	"github.com/fslongjin/liteboxd/tools/liteboxd-installer/internal/sshx"
)

type Options struct {
	DryRun         bool
	Verbose        bool
	SSHTimeout     time.Duration
	CommandTimeout time.Duration
}

type Manager struct {
	cfg          *config.Config
	opts         Options
	logger       *log.Logger
	detailLogger *log.Logger
}

func New(cfg *config.Config, opts Options, logger *log.Logger, detailLogger *log.Logger) *Manager {
	if logger == nil {
		logger = log.New(os.Stdout, "[cluster] ", log.LstdFlags)
	}
	return &Manager{cfg: cfg, opts: opts, logger: logger, detailLogger: detailLogger}
}

func (m *Manager) runMaster(command string, useSudo bool) (string, error) {
	if m.opts.DryRun {
		m.logf("dry-run master command: %s", command)
		return "", nil
	}
	start := time.Now()
	m.detailf("target=master host=%s sudo=%t command=%s", m.cfg.Cluster.Master.Host, useSudo, sanitizeCommand(command))
	client, err := m.dialMaster()
	if err != nil {
		m.detailf("target=master dial_error=%v", err)
		return "", fmt.Errorf("connect master: %w", err)
	}
	defer client.Close()
	res, err := client.Run(command, useSudo)
	stdout := sanitizeOutput(command, res.Stdout)
	stderr := sanitizeOutput(command, res.Stderr)
	if stdout != "" {
		m.detailf("target=master stdout=%s", stdout)
	}
	if stderr != "" {
		m.detailf("target=master stderr=%s", stderr)
	}
	m.detailf("target=master duration=%s success=%t", time.Since(start), err == nil)
	if err != nil {
		if res.Stderr != "" {
			return "", fmt.Errorf("%w: %s", err, res.Stderr)
		}
		return "", err
	}
	return res.Stdout, nil
}

func (m *Manager) masterTarget() sshx.Target {
	n := m.cfg.Cluster.Master
	return sshx.Target{
		Host:         n.Host,
		Port:         n.Port,
		User:         n.User,
		Password:     n.Password,
		Sudo:         n.Sudo,
		SudoPassword: n.SudoPassword,
	}
}

func (m *Manager) agentTarget(n config.NodeConfig) sshx.Target {
	return sshx.Target{
		Host:         n.Host,
		Port:         n.Port,
		User:         n.User,
		Password:     n.Password,
		Sudo:         n.Sudo,
		SudoPassword: n.SudoPassword,
	}
}

func (m *Manager) dialMaster() (*sshx.Client, error) {
	return sshx.Dial(m.masterTarget(), m.opts.SSHTimeout, m.opts.CommandTimeout)
}

func (m *Manager) dialAgent(agent config.NodeConfig) (*sshx.Client, error) {
	target := m.agentTarget(agent)
	master := m.masterTarget()
	client, err := sshx.DialVia(target, master, m.opts.SSHTimeout, m.opts.CommandTimeout)
	if err != nil {
		m.detailf("target=%s via_master=%s dial_error=%v", agent.Host, master.Host, err)
		return nil, fmt.Errorf("connect agent %s via master %s: %w", agent.Host, master.Host, err)
	}
	m.detailf("target=%s connect_via_master=%s", agent.Host, master.Host)
	return client, nil
}

func (m *Manager) findAgent(host string) (config.NodeConfig, bool) {
	for _, a := range m.cfg.Cluster.Agents {
		if a.Host == host {
			return a, true
		}
	}
	return config.NodeConfig{}, false
}

func (m *Manager) findAgentByHostOrNodeIP(value string) (config.NodeConfig, bool) {
	for _, a := range m.cfg.Cluster.Agents {
		if a.Host == value || a.NodeIP == value {
			return a, true
		}
	}
	return config.NodeConfig{}, false
}

func (m *Manager) logf(format string, args ...any) {
	if m.opts.Verbose {
		m.logger.Printf(format, args...)
	}
}

func (m *Manager) detailf(format string, args ...any) {
	if m.detailLogger != nil {
		m.detailLogger.Printf(format, args...)
	}
}

func shellQuote(v string) string {
	if v == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'"
}

var (
	reK3STokenWithSpace = regexp.MustCompile(`K3S_TOKEN=[^\s]+`)
	reK3STokenQuoted    = regexp.MustCompile(`K3S_TOKEN='[^']*'`)
)

func sanitizeCommand(command string) string {
	s := reK3STokenQuoted.ReplaceAllString(command, "K3S_TOKEN='***'")
	s = reK3STokenWithSpace.ReplaceAllString(s, "K3S_TOKEN=***")
	return s
}

func sanitizeOutput(command, out string) string {
	if out == "" {
		return ""
	}
	if strings.Contains(command, "node-token") || strings.Contains(command, "K3S_TOKEN") {
		return "[REDACTED]"
	}
	return out
}

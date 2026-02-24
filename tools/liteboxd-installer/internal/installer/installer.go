package installer

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fslongjin/liteboxd/tools/liteboxd-installer/internal/config"
	"github.com/fslongjin/liteboxd/tools/liteboxd-installer/internal/installer/cluster"
	"github.com/fslongjin/liteboxd/tools/liteboxd-installer/internal/state"
)

type Options struct {
	DryRun      bool
	Verbose     bool
	ClusterOnly bool
	LogFile     string
}

type Installer struct {
	cfg            *config.Config
	opts           Options
	stateStore     *state.Store
	sshTimeout     time.Duration
	commandTimeout time.Duration
	logger         *log.Logger
	detailLogger   *log.Logger
	cluster        *cluster.Manager
}

func New(cfg *config.Config, st *state.Store, opts Options) *Installer {
	sshTimeout := time.Duration(cfg.Runtime.SSHTimeoutSeconds) * time.Second
	commandTimeout := time.Duration(cfg.Runtime.CommandTimeoutSeconds) * time.Second
	logger, detailLogger := newLoggers(opts.LogFile)
	clusterMgr := cluster.New(cfg, cluster.Options{
		DryRun:         opts.DryRun,
		Verbose:        opts.Verbose,
		SSHTimeout:     sshTimeout,
		CommandTimeout: commandTimeout,
	}, logger, detailLogger)
	return &Installer{
		cfg:            cfg,
		opts:           opts,
		stateStore:     st,
		sshTimeout:     sshTimeout,
		commandTimeout: commandTimeout,
		logger:         logger,
		detailLogger:   detailLogger,
		cluster:        clusterMgr,
	}
}

func (i *Installer) Apply() error {
	if err := i.runStep("precheck", i.cluster.Precheck); err != nil {
		return err
	}
	if err := i.runStep("configure_registries", i.cluster.ConfigureRegistriesAllNodes); err != nil {
		return err
	}
	if err := i.runStep("install_master", i.cluster.InstallOrCheckMaster); err != nil {
		return err
	}
	if err := i.runStep("install_cilium", i.cluster.InstallOrUpgradeCilium); err != nil {
		return err
	}

	var token string
	if err := i.runStep("read_k3s_token", func() error {
		var err error
		token, err = i.cluster.ReadNodeToken()
		return err
	}); err != nil {
		return err
	}
	if err := i.runStep("join_agents", func() error { return i.cluster.JoinAgents(token) }); err != nil {
		return err
	}
	if err := i.runStep("wait_nodes_ready", i.cluster.WaitNodesReady); err != nil {
		return err
	}
	if i.cfg.Runtime.RemoveAbsentAgents {
		if err := i.runStep("reconcile_remove_absent_agents", i.cluster.ReconcileRemoveAbsentAgents); err != nil {
			return err
		}
	}
	if i.opts.ClusterOnly {
		i.logf("cluster-only mode enabled, skip LiteBoxd deployment")
		_ = i.stateStore.Mark("deploy_liteboxd", state.StatusDone, "skipped: cluster-only mode")
		_ = i.stateStore.Mark("rollout_check", state.StatusDone, "skipped: cluster-only mode")
		return nil
	}
	if err := i.runStep("deploy_liteboxd", i.deployLiteBoxd); err != nil {
		return err
	}
	if err := i.runStep("rollout_check", i.rolloutCheck); err != nil {
		return err
	}
	return nil
}

func (i *Installer) RemoveNodes(hosts []string, uninstallAgent bool) error {
	return i.runStep("node_remove", func() error {
		return i.cluster.RemoveNodes(hosts, uninstallAgent)
	})
}

func (i *Installer) runStep(name string, fn func() error) error {
	i.logf("step=%s start", name)
	if err := i.stateStore.Mark(name, state.StatusRunning, ""); err != nil {
		return err
	}
	if err := fn(); err != nil {
		_ = i.stateStore.Mark(name, state.StatusFailed, err.Error())
		return fmt.Errorf("step %s failed: %w", name, err)
	}
	if err := i.stateStore.Mark(name, state.StatusDone, ""); err != nil {
		return err
	}
	i.logf("step=%s done", name)
	return nil
}

func (i *Installer) logf(format string, args ...any) {
	if i.opts.Verbose || strings.Contains(format, "step=") {
		i.logger.Printf(format, args...)
	}
}

func (i *Installer) detailf(format string, args ...any) {
	if i.detailLogger != nil {
		i.detailLogger.Printf(format, args...)
	}
}

func newLoggers(logFile string) (*log.Logger, *log.Logger) {
	mainLogger := log.New(os.Stdout, "[installer] ", log.LstdFlags)
	if strings.TrimSpace(logFile) == "" {
		return mainLogger, nil
	}

	if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
		mainLogger.Printf("warn: create log dir failed: %v", err)
		return mainLogger, nil
	}
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		mainLogger.Printf("warn: open log file failed: %v", err)
		return mainLogger, nil
	}

	mainLogger.Printf("detailed log file: %s", logFile)
	mainLogger.SetOutput(io.MultiWriter(os.Stdout, f))
	detailLogger := log.New(f, "[detail] ", log.LstdFlags)
	return mainLogger, detailLogger
}

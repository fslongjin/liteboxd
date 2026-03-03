package cmd

import (
	"fmt"

	"github.com/fslongjin/liteboxd/tools/liteboxd-installer/internal/config"
	"github.com/fslongjin/liteboxd/tools/liteboxd-installer/internal/installer"
	"github.com/fslongjin/liteboxd/tools/liteboxd-installer/internal/state"
	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Install/upgrade cluster and/or deploy LiteBoxd",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runApply(false)
	},
}

var clusterOnly bool
var liteboxdOnly bool

func init() {
	rootCmd.AddCommand(applyCmd)
	applyCmd.Flags().BoolVar(&clusterOnly, "cluster-only", false, "Only manage K3s/Cilium/Longhorn/nodes; skip LiteBoxd deployment")
	applyCmd.Flags().BoolVar(&liteboxdOnly, "liteboxd-only", false, "Only deploy LiteBoxd workloads; skip cluster install/upgrade steps")
}

func runApply(isResume bool) error {
	if clusterOnly && liteboxdOnly {
		return fmt.Errorf("--cluster-only and --liteboxd-only are mutually exclusive")
	}

	cfg, err := config.LoadWithOptions(configFile, config.LoadOptions{
		RequireLiteBoxd: !clusterOnly,
	})
	if err != nil {
		return err
	}

	effectiveDryRun := dryRun || cfg.Runtime.DryRun
	path := statePath
	if path == "" {
		path = state.DefaultPath(cfg.Cluster.Name)
	}

	st, err := state.LoadOrCreate(path, cfg.Cluster.Name, configFile)
	if err != nil {
		return err
	}

	if isResume {
		fmt.Printf("Resuming from state file: %s\n", st.Path)
	}

	runner := installer.New(cfg, st, installer.Options{
		DryRun:       effectiveDryRun,
		Verbose:      verbose,
		ClusterOnly:  clusterOnly,
		LiteBoxdOnly: liteboxdOnly,
		LogFile:      logFile,
	})
	return runner.Apply()
}

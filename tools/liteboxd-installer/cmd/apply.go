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
	Short: "Install/upgrade cluster and deploy LiteBoxd",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runApply(false)
	},
}

var clusterOnly bool

func init() {
	rootCmd.AddCommand(applyCmd)
	applyCmd.Flags().BoolVar(&clusterOnly, "cluster-only", false, "Only manage K3s/Cilium/nodes; skip LiteBoxd deployment")
}

func runApply(isResume bool) error {
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
		DryRun:      effectiveDryRun,
		Verbose:     verbose,
		ClusterOnly: clusterOnly,
		LogFile:     logFile,
	})
	return runner.Apply()
}

package cmd

import (
	"strings"

	"github.com/fslongjin/liteboxd/tools/liteboxd-installer/internal/config"
	"github.com/fslongjin/liteboxd/tools/liteboxd-installer/internal/installer"
	"github.com/fslongjin/liteboxd/tools/liteboxd-installer/internal/state"
	"github.com/spf13/cobra"
)

var (
	removeHosts []string
)

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Cluster node operations",
}

var nodeRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Safely remove agent nodes from cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadWithOptions(configFile, config.LoadOptions{
			RequireLiteBoxd: false,
		})
		if err != nil {
			return err
		}
		path := statePath
		if path == "" {
			path = state.DefaultPath(cfg.Cluster.Name)
		}
		st, err := state.LoadOrCreate(path, cfg.Cluster.Name, configFile)
		if err != nil {
			return err
		}
		hosts := make([]string, 0, len(removeHosts))
		for _, h := range removeHosts {
			for _, p := range strings.Split(h, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					hosts = append(hosts, p)
				}
			}
		}

		runner := installer.New(cfg, st, installer.Options{
			DryRun:  dryRun || cfg.Runtime.DryRun,
			Verbose: verbose,
			LogFile: logFile,
		})
		return runner.RemoveNodes(hosts, true)
	},
}

func init() {
	rootCmd.AddCommand(nodeCmd)
	nodeCmd.AddCommand(nodeRemoveCmd)
	nodeRemoveCmd.Flags().StringSliceVar(&removeHosts, "hosts", nil, "Agent host list, supports comma-separated values")
	_ = nodeRemoveCmd.MarkFlagRequired("hosts")
}

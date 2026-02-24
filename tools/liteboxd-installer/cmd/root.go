package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	configFile string
	statePath  string
	dryRun     bool
	verbose    bool
	logFile    string
)

var rootCmd = &cobra.Command{
	Use:   "liteboxd-installer",
	Short: "LiteBoxd one-click cluster installer",
	Long:  "Install K3s + Cilium and deploy LiteBoxd to remote hosts via SSH.",
}

func Execute(version, commit, date string) error {
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, built at: %s)", version, commit, date)
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configFile, "file", "f", "", "Installer config file (YAML)")
	rootCmd.PersistentFlags().StringVar(&statePath, "state", "", "State file path (default: ~/.liteboxd-installer/<cluster>-state.json)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Print planned operations without executing")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().StringVar(&logFile, "log-file", "", "Detailed execution log file path (optional)")
	_ = rootCmd.MarkPersistentFlagRequired("file")
}

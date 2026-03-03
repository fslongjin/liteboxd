package cmd

import "github.com/spf13/cobra"

var resumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume an interrupted apply run",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runApply(true)
	},
}

func init() {
	rootCmd.AddCommand(resumeCmd)
	resumeCmd.Flags().BoolVar(&clusterOnly, "cluster-only", false, "Only manage K3s/Cilium/Longhorn/nodes; skip LiteBoxd deployment")
	resumeCmd.Flags().BoolVar(&liteboxdOnly, "liteboxd-only", false, "Only deploy LiteBoxd workloads; skip cluster install/upgrade steps")
}

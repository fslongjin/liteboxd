package cmd

import (
	"fmt"
	"os"

	liteboxd "github.com/fslongjin/liteboxd/sdk/go"
	"github.com/spf13/cobra"
)

var (
	importFile     string
	importStrategy string
	importPrepull  bool
)

var importCmd = &cobra.Command{
	Use:   "import --file <yaml-file>",
	Short: "Import templates from YAML",
	Example: `  # Import templates
  liteboxd import --file templates.yaml

  # Import with auto-prepull
  liteboxd import --file templates.yaml --prepull`,
	RunE: runImport,
}

func init() {
	rootCmd.AddCommand(importCmd)

	importCmd.Flags().StringVarP(&importFile, "file", "f", "", "YAML file (required)")
	importCmd.Flags().StringVar(&importStrategy, "strategy", "create-or-update", "Import strategy (create-only, update-only, create-or-update)")
	importCmd.Flags().BoolVar(&importPrepull, "prepull", false, "Auto-prepull images after import")
	importCmd.MarkFlagRequired("file")
}

func runImport(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx, _ := getContext()

	// Read YAML file
	content, err := os.ReadFile(importFile)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	strategy := liteboxd.ImportStrategy(importStrategy)
	resp, err := client.ImportExport.ImportTemplates(ctx, content, strategy, importPrepull)
	if err != nil {
		return err
	}

	fmt.Printf("Import complete: %d total\n", resp.Total)
	fmt.Printf("  Created: %d\n", resp.Created)
	fmt.Printf("  Updated: %d\n", resp.Updated)
	fmt.Printf("  Skipped: %d\n", resp.Skipped)
	if resp.Failed > 0 {
		fmt.Printf("  Failed: %d\n", resp.Failed)
	}

	if len(resp.PrepullStarted) > 0 {
		fmt.Printf("\nPrepull started for: %v\n", resp.PrepullStarted)
	}

	return nil
}

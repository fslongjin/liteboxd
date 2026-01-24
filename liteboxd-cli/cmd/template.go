package cmd

import (
	"fmt"
	"os"

	"github.com/fslongjin/liteboxd/liteboxd-cli/internal/output"
	liteboxd "github.com/fslongjin/liteboxd/sdk/go"
	"github.com/spf13/cobra"
)

var (
	forceFlag bool
)

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Manage templates",
	Long:  `Create, list, and manage LiteBoxd sandbox templates.`,
}

var (
	templateName        string
	templateDisplayName string
	templateDescription string
	templateTags        []string
	templateImage       string
	templateCPU         string
	templateMemory      string
	templateTTL         int
	templateFile        string
	templateChangelog   string
)

var templateCreateCmd = &cobra.Command{
	Use:   "create --name <name> --file <yaml-file>",
	Short: "Create a new template",
	Long: `Create a new sandbox template.

You can provide template configuration via --file (YAML) or using individual flags.`,
	Example: `  # Create from YAML file
  liteboxd template create --name python-ds --file template.yaml

  # Create using flags
  liteboxd template create --name node-basic --image node:20-alpine --cpu 500m`,
	RunE: runTemplateCreate,
}

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List templates",
	Example: `  # List all templates
  liteboxd template list

  # Filter by tag
  liteboxd template list --tag python`,
	RunE: runTemplateList,
}

var templateGetCmd = &cobra.Command{
	Use:     "get <name>",
	Short:   "Get template details",
	Args:    cobra.ExactArgs(1),
	Example: `  liteboxd template get python-ds`,
	RunE:    runTemplateGet,
}

var templateUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Update a template",
	Args:  cobra.ExactArgs(1),
	Example: `  # Update from YAML file
  liteboxd template update python-ds --file new-spec.yaml --changelog "Add pandas"`,
	RunE: runTemplateUpdate,
}

var templateDeleteCmd = &cobra.Command{
	Use:     "delete <name>",
	Short:   "Delete a template",
	Args:    cobra.ExactArgs(1),
	Example: `  liteboxd template delete python-ds`,
	RunE:    runTemplateDelete,
}

var templateVersionsCmd = &cobra.Command{
	Use:     "versions <name>",
	Short:   "List template versions",
	Args:    cobra.ExactArgs(1),
	Example: `  liteboxd template versions python-ds`,
	RunE:    runTemplateVersions,
}

var templateRollbackCmd = &cobra.Command{
	Use:     "rollback <name> --to <version>",
	Short:   "Rollback template to version",
	Args:    cobra.ExactArgs(1),
	Example: `  liteboxd template rollback python-ds --to 1`,
	RunE:    runTemplateRollback,
}

var (
	templateExportOutput  string
	templateExportTag     string
	templateExportNames   string
	templateExportVersion int
)

var templateExportCmd = &cobra.Command{
	Use:   "export [name]",
	Short: "Export template(s) to YAML",
	Args:  cobra.MaximumNArgs(1),
	Example: `  # Export single template
  liteboxd template export python-ds --output python-ds.yaml

  # Export all templates
  liteboxd template export --output all-templates.yaml`,
	RunE: runTemplateExport,
}

func init() {
	rootCmd.AddCommand(templateCmd)

	// Create command
	templateCreateCmd.Flags().StringVar(&templateName, "name", "", "Template name (required)")
	templateCreateCmd.Flags().StringVarP(&templateFile, "file", "f", "", "YAML file with template spec")
	templateCreateCmd.Flags().StringVar(&templateDisplayName, "display-name", "", "Display name")
	templateCreateCmd.Flags().StringVarP(&templateDescription, "description", "d", "", "Description")
	templateCreateCmd.Flags().StringSliceVar(&templateTags, "tags", nil, "Tags")
	templateCreateCmd.Flags().StringVar(&templateImage, "image", "", "Container image")
	templateCreateCmd.Flags().StringVar(&templateCPU, "cpu", "", "CPU limit")
	templateCreateCmd.Flags().StringVar(&templateMemory, "memory", "", "Memory limit")
	templateCreateCmd.Flags().IntVar(&templateTTL, "ttl", 0, "Default TTL in seconds")
	templateCreateCmd.MarkFlagRequired("name")
	templateCmd.AddCommand(templateCreateCmd)

	// List command
	templateListCmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml)")
	templateListCmd.Flags().String("tag", "", "Filter by tag")
	templateListCmd.Flags().String("search", "", "Search in name/description")
	templateListCmd.Flags().Int("page", 1, "Page number")
	templateListCmd.Flags().Int("page-size", 20, "Items per page")
	templateCmd.AddCommand(templateListCmd)

	// Get command
	getVersion := 0
	templateGetCmd.Flags().IntVarP(&getVersion, "version", "v", 0, "Template version (default latest)")
	templateCmd.AddCommand(templateGetCmd)

	// Update command
	templateUpdateCmd.Flags().StringVarP(&templateFile, "file", "f", "", "YAML file with new spec")
	templateUpdateCmd.Flags().StringVar(&templateChangelog, "changelog", "", "Changelog for the update")
	templateUpdateCmd.Flags().StringVar(&templateDisplayName, "display-name", "", "New display name")
	templateUpdateCmd.Flags().StringVarP(&templateDescription, "description", "d", "", "New description")
	templateUpdateCmd.Flags().StringSliceVar(&templateTags, "tags", nil, "New tags")
	templateUpdateCmd.Flags().StringVar(&templateImage, "image", "", "New image")
	templateUpdateCmd.Flags().StringVar(&templateCPU, "cpu", "", "New CPU limit")
	templateUpdateCmd.Flags().StringVar(&templateMemory, "memory", "", "New memory limit")
	templateUpdateCmd.Flags().IntVar(&templateTTL, "ttl", 0, "New TTL")
	templateCmd.AddCommand(templateUpdateCmd)

	// Delete command
	templateDeleteCmd.Flags().BoolVarP(&forceFlag, "force", "f", false, "Skip confirmation")
	templateCmd.AddCommand(templateDeleteCmd)

	// Versions command
	templateVersionsCmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml)")
	templateCmd.AddCommand(templateVersionsCmd)

	// Rollback command
	rollbackToVersion := 0
	templateRollbackCmd.Flags().IntVar(&rollbackToVersion, "to", 0, "Target version (required)")
	templateRollbackCmd.Flags().StringVar(&templateChangelog, "changelog", "", "Changelog for the rollback")
	templateRollbackCmd.MarkFlagRequired("to")
	templateCmd.AddCommand(templateRollbackCmd)

	// Export command
	templateExportCmd.Flags().StringVarP(&templateExportOutput, "output", "o", "", "Output file (default: stdout)")
	templateExportCmd.Flags().IntVarP(&templateExportVersion, "version", "v", 0, "Template version for single export")
	templateExportCmd.Flags().StringVar(&templateExportTag, "tag", "", "Filter by tag (for all export)")
	templateExportCmd.Flags().StringVar(&templateExportNames, "names", "", "Comma-separated names (for all export)")
	templateCmd.AddCommand(templateExportCmd)
}

func runTemplateCreate(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx, _ := getContext()

	var req liteboxd.CreateTemplateRequest

	// If file is provided, read from file (for now, require flags)
	// TODO: Add YAML file parsing
	req.Name = templateName
	req.DisplayName = templateDisplayName
	req.Description = templateDescription
	req.Tags = templateTags

	// Build spec from flags
	req.Spec = liteboxd.TemplateSpec{
		Image: templateImage,
		Resources: liteboxd.ResourceSpec{
			CPU:    templateCPU,
			Memory: templateMemory,
		},
		TTL: templateTTL,
	}

	// Validate required fields
	if templateImage == "" {
		return fmt.Errorf("--image is required when not using --file")
	}

	template, err := client.Template.Create(ctx, &req)
	if err != nil {
		return err
	}

	fmt.Printf("Created template: %s (version: %d)\n", template.Name, template.LatestVersion)
	return nil
}

func runTemplateList(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx, _ := getContext()

	tag, _ := cmd.Flags().GetString("tag")
	search, _ := cmd.Flags().GetString("search")
	page, _ := cmd.Flags().GetInt("page")
	pageSize, _ := cmd.Flags().GetInt("page-size")

	opts := &liteboxd.TemplateListOptions{
		Tag:      tag,
		Search:   search,
		Page:     page,
		PageSize: pageSize,
	}

	resp, err := client.Template.List(ctx, opts)
	if err != nil {
		return err
	}

	// Use table formatter with specific fields for list
	format := output.ParseFormat(outputFormat)
	var formatter output.Formatter
	if format == output.FormatTable {
		// Show only basic fields in list view
		formatter = output.NewTableFormatterWithLabels(
			[]string{"id", "name", "spec.image", "latestVersion", "updatedAt"},
			map[string]string{"id": "ID", "name": "NAME", "spec.image": "IMAGE", "latestVersion": "VERSION", "updatedAt": "UPDATED"},
		)
	} else {
		formatter = output.NewFormatter(format)
	}

	return formatter.Write(cmd.OutOrStdout(), resp.Items)
}

func runTemplateGet(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx, _ := getContext()

	version, _ := cmd.Flags().GetInt("version")

	if version > 0 {
		ver, err := client.Template.GetVersion(ctx, args[0], version)
		if err != nil {
			return err
		}
		formatter := output.NewFormatter(output.ParseFormat(outputFormat))
		return formatter.Write(cmd.OutOrStdout(), ver)
	}

	template, err := client.Template.Get(ctx, args[0])
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(output.ParseFormat(outputFormat))
	return formatter.Write(cmd.OutOrStdout(), template)
}

func runTemplateUpdate(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx, _ := getContext()

	name := args[0]

	// For now, require --file for updates
	// TODO: Add flag-based updates
	if templateFile == "" {
		return fmt.Errorf("--file is required for updates")
	}

	req := &liteboxd.UpdateTemplateRequest{
		Changelog: templateChangelog,
	}

	template, err := client.Template.Update(ctx, name, req)
	if err != nil {
		return err
	}

	fmt.Printf("Updated template: %s (version: %d)\n", template.Name, template.LatestVersion)
	return nil
}

func runTemplateDelete(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx, _ := getContext()

	name := args[0]

	force, _ := cmd.Flags().GetBool("force")
	if !force {
		fmt.Printf("Delete template %s? [y/N]: ", name)
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Cancelled")
			return nil
		}
	}

	if err := client.Template.Delete(ctx, name); err != nil {
		return err
	}

	fmt.Printf("Deleted template: %s\n", name)
	return nil
}

func runTemplateVersions(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx, _ := getContext()

	resp, err := client.Template.ListVersions(ctx, args[0])
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(output.ParseFormat(outputFormat))
	return formatter.Write(cmd.OutOrStdout(), resp.Items)
}

func runTemplateRollback(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx, _ := getContext()

	name := args[0]
	version, _ := cmd.Flags().GetInt("to")
	changelog, _ := cmd.Flags().GetString("changelog")

	resp, err := client.Template.Rollback(ctx, name, version, changelog)
	if err != nil {
		return err
	}

	fmt.Printf("Rolled back template: %s from version %d to %d\n", name, resp.RolledBackFrom, resp.RolledBackTo)
	fmt.Printf("Current version: %d\n", resp.LatestVersion)
	return nil
}

func runTemplateExport(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx, _ := getContext()

	var yamlData []byte
	var err error

	if len(args) == 1 {
		// Export single template
		version, _ := cmd.Flags().GetInt("version")
		yamlData, err = client.Template.ExportYAML(ctx, args[0], version)
	} else {
		// Export all templates
		tag, _ := cmd.Flags().GetString("tag")
		names, _ := cmd.Flags().GetString("names")
		yamlData, err = client.ImportExport.ExportAllTemplates(ctx, tag, names)
	}

	if err != nil {
		return err
	}

	outputPath, _ := cmd.Flags().GetString("output")
	if outputPath != "" {
		if err := os.WriteFile(outputPath, yamlData, 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		fmt.Printf("Exported to: %s\n", outputPath)
	} else {
		fmt.Print(string(yamlData))
	}

	return nil
}

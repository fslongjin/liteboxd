package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/fslongjin/liteboxd/liteboxd-cli/internal/output"
	liteboxd "github.com/fslongjin/liteboxd/sdk/go"
	"github.com/spf13/cobra"
)

var outputFormat string

var sandboxCmd = &cobra.Command{
	Use:   "sandbox",
	Short: "Manage sandboxes",
	Long:  `Create, list, and manage LiteBoxd sandboxes.`,
}

var (
	templateFlag        string
	templateVersionFlag int
	cpuFlag             string
	memoryFlag          string
	ttlFlag             int
	envFlag             []string
	waitFlag            bool
	quietFlag           bool
)

var sandboxCreateCmd = &cobra.Command{
	Use:   "create --template <name>",
	Short: "Create a new sandbox",
	Long: `Create a new sandbox from a template.

All sandboxes must be created from a template. Use --cpu, --memory, --ttl, and --env
to override template values.`,
	Example: `  # Create from template with defaults
  liteboxd sandbox create --template python-data-science

  # Create with overrides
  liteboxd sandbox create --template python-ds --ttl 7200 --env DEBUG=true

  # Create and wait for ready
  liteboxd sandbox create --template nodejs --wait`,
	RunE: runSandboxCreate,
}

var sandboxListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all sandboxes",
	Example: `  # List all sandboxes
  liteboxd sandbox list

  # List with JSON output
  liteboxd sandbox list --output json`,
	RunE: runSandboxList,
}

var sandboxGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get sandbox details",
	Args:  cobra.ExactArgs(1),
	Example: `  # Get sandbox details
  liteboxd sandbox get <sandbox-id>`,
	RunE: runSandboxGet,
}

var sandboxDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a sandbox",
	Args:  cobra.ExactArgs(1),
	Example: `  # Delete with confirmation
  liteboxd sandbox delete <sandbox-id>

  # Force delete without confirmation
  liteboxd sandbox delete <id> --force`,
	RunE: runSandboxDelete,
}

var (
	execTimeout  int
	exitCodeFlag bool
)

var sandboxExecCmd = &cobra.Command{
	Use:   "exec <id> -- <command> [args...]",
	Short: "Execute command in sandbox",
	Args:  cobra.MinimumNArgs(1),
	Example: `  # Python script
  liteboxd sandbox exec <id> -- python -c "print('hello')"

  # Long-running command
  liteboxd sandbox exec <id> --timeout 5m -- npm test`,
	RunE: runSandboxExec,
}

var (
	logsTailFlag   int
	logsEventsFlag bool
)

var sandboxLogsCmd = &cobra.Command{
	Use:   "logs <id>",
	Short: "Get sandbox logs",
	Args:  cobra.ExactArgs(1),
	Example: `  # Get logs
  liteboxd sandbox logs <sandbox-id>

  # Get with events
  liteboxd sandbox logs <id> --events`,
	RunE: runSandboxLogs,
}

var sandboxUploadCmd = &cobra.Command{
	Use:     "upload <id> <local-path> <remote-path>",
	Short:   "Upload file to sandbox",
	Args:    cobra.ExactArgs(3),
	Example: `  liteboxd sandbox upload <id> ./main.py /workspace/main.py`,
	RunE:    runSandboxUpload,
}

var sandboxDownloadCmd = &cobra.Command{
	Use:   "download <id> <remote-path> [local-path]",
	Short: "Download file from sandbox",
	Args:  cobra.RangeArgs(2, 3),
	Example: `  # Download to stdout
  liteboxd sandbox download <id> /workspace/output.txt

  # Download to file
  liteboxd sandbox download <id> /workspace/output.txt ./output.txt`,
	RunE: runSandboxDownload,
}

var (
	pollIntervalFlag time.Duration
	waitTimeoutFlag  time.Duration
)

var sandboxWaitCmd = &cobra.Command{
	Use:     "wait <id>",
	Short:   "Wait for sandbox to be ready",
	Args:    cobra.ExactArgs(1),
	Example: `  liteboxd sandbox wait <sandbox-id>`,
	RunE:    runSandboxWait,
}

func init() {
	rootCmd.AddCommand(sandboxCmd)

	// Create command
	sandboxCreateCmd.Flags().StringVar(&templateFlag, "template", "", "Template name (required)")
	sandboxCreateCmd.Flags().IntVar(&templateVersionFlag, "template-version", 0, "Template version (default latest)")
	sandboxCreateCmd.Flags().StringVar(&cpuFlag, "cpu", "", "Override CPU limit")
	sandboxCreateCmd.Flags().StringVar(&memoryFlag, "memory", "", "Override memory limit")
	sandboxCreateCmd.Flags().IntVar(&ttlFlag, "ttl", 0, "Override TTL in seconds")
	sandboxCreateCmd.Flags().StringSliceVar(&envFlag, "env", nil, "Environment variables (KEY=VALUE)")
	sandboxCreateCmd.Flags().BoolVar(&waitFlag, "wait", false, "Wait for sandbox to be ready")
	sandboxCreateCmd.Flags().BoolVarP(&quietFlag, "quiet", "q", false, "Only print sandbox ID")
	sandboxCreateCmd.MarkFlagRequired("template")
	sandboxCmd.AddCommand(sandboxCreateCmd)

	// List command
	sandboxListCmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml)")
	sandboxCmd.AddCommand(sandboxListCmd)

	// Get command
	sandboxGetCmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml)")
	sandboxCmd.AddCommand(sandboxGetCmd)

	// Delete command
	forceFlag := false
	sandboxDeleteCmd.Flags().BoolVarP(&forceFlag, "force", "f", false, "Skip confirmation")
	sandboxCmd.AddCommand(sandboxDeleteCmd)

	// Exec command
	sandboxExecCmd.Flags().IntVar(&execTimeout, "timeout", 30, "Execution timeout in seconds")
	sandboxExecCmd.Flags().BoolVar(&quietFlag, "quiet", false, "Only print stdout")
	sandboxExecCmd.Flags().BoolVar(&exitCodeFlag, "exit-code", false, "Print exit code")
	sandboxCmd.AddCommand(sandboxExecCmd)

	// Logs command
	sandboxLogsCmd.Flags().IntVar(&logsTailFlag, "tail", 100, "Number of lines")
	sandboxLogsCmd.Flags().BoolVar(&logsEventsFlag, "events", false, "Show Pod events")
	sandboxCmd.AddCommand(sandboxLogsCmd)

	sandboxCmd.AddCommand(sandboxUploadCmd)
	sandboxCmd.AddCommand(sandboxDownloadCmd)

	// Wait command
	sandboxWaitCmd.Flags().DurationVar(&pollIntervalFlag, "poll-interval", 2*time.Second, "Poll interval")
	sandboxWaitCmd.Flags().DurationVar(&waitTimeoutFlag, "timeout", 5*time.Minute, "Max wait time")
	sandboxWaitCmd.Flags().BoolVar(&quietFlag, "quiet", false, "Only print status")
	sandboxCmd.AddCommand(sandboxWaitCmd)
}

func runSandboxCreate(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx, _ := getContext()

	// Build overrides
	var overrides *liteboxd.SandboxOverrides
	if cpuFlag != "" || memoryFlag != "" || ttlFlag != 0 || len(envFlag) > 0 {
		overrides = &liteboxd.SandboxOverrides{}
		if cpuFlag != "" {
			overrides.CPU = cpuFlag
		}
		if memoryFlag != "" {
			overrides.Memory = memoryFlag
		}
		if ttlFlag != 0 {
			overrides.TTL = ttlFlag
		}
		if len(envFlag) > 0 {
			overrides.Env = parseEnvVars(envFlag)
		}
	}

	// Create sandbox
	var sandbox *liteboxd.Sandbox
	var err error
	if templateVersionFlag > 0 {
		sandbox, err = client.Sandbox.CreateWithVersion(ctx, templateFlag, templateVersionFlag, overrides)
	} else {
		sandbox, err = client.Sandbox.Create(ctx, templateFlag, overrides)
	}
	if err != nil {
		return err
	}

	if quietFlag {
		fmt.Println(sandbox.ID)
	} else {
		fmt.Printf("Created sandbox: %s\n", sandbox.ID)
		fmt.Printf("Status: %s\n", sandbox.Status)
		fmt.Printf("Expires: %s\n", sandbox.ExpiresAt.Format(time.RFC3339))
	}

	// Wait for ready if requested
	if waitFlag {
		if err := waitForSandbox(client, ctx, sandbox.ID); err != nil {
			return err
		}
	}

	return nil
}

func runSandboxList(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx, _ := getContext()

	sandboxes, err := client.Sandbox.List(ctx)
	if err != nil {
		return err
	}

	// Use table formatter with specific fields for list
	format := output.ParseFormat(outputFormat)
	var formatter output.Formatter
	if format == output.FormatTable {
		// Show only basic fields in list view
		formatter = output.NewTableFormatter([]string{"id", "image", "status", "createdAt", "expiresAt"})
	} else {
		formatter = output.NewFormatter(format)
	}

	return formatter.Write(cmd.OutOrStdout(), sandboxes)
}

func runSandboxGet(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx, _ := getContext()

	sandbox, err := client.Sandbox.Get(ctx, args[0])
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(output.ParseFormat(outputFormat))
	return formatter.Write(cmd.OutOrStdout(), sandbox)
}

func runSandboxDelete(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx, _ := getContext()

	id := args[0]

	// Check for force flag
	force, _ := cmd.Flags().GetBool("force")
	if !force {
		fmt.Printf("Delete sandbox %s? [y/N]: ", id)
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Cancelled")
			return nil
		}
	}

	if err := client.Sandbox.Delete(ctx, id); err != nil {
		return err
	}

	fmt.Printf("Deleted sandbox: %s\n", id)
	return nil
}

func runSandboxExec(cmd *cobra.Command, args []string) error {
	client := getAPIClient()

	id := args[0]
	// Find the "--" separator to split command from flags
	cmdArgs := args[1:]

	resp, err := client.Sandbox.Execute(context.Background(), id, cmdArgs, execTimeout)
	if err != nil {
		return err
	}

	quiet, _ := cmd.Flags().GetBool("quiet")
	if !quiet && resp.Stderr != "" {
		fmt.Fprint(os.Stderr, resp.Stderr)
	}
	fmt.Print(resp.Stdout)

	if exitCodeFlag {
		os.Exit(resp.ExitCode)
	}

	if resp.ExitCode != 0 {
		return fmt.Errorf("command exited with code %d", resp.ExitCode)
	}

	return nil
}

func runSandboxLogs(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx, _ := getContext()

	resp, err := client.Sandbox.GetLogs(ctx, args[0])
	if err != nil {
		return err
	}

	if logsEventsFlag {
		fmt.Println("\n--- Events ---")
		for _, event := range resp.Events {
			fmt.Println(event)
		}
		fmt.Println("\n--- Logs ---")
	}

	fmt.Println(resp.Logs)
	return nil
}

func runSandboxUpload(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx, _ := getContext()

	id := args[0]
	localPath := args[1]
	remotePath := args[2]

	// Read file
	content, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	if err := client.Sandbox.UploadFile(ctx, id, remotePath, content, ""); err != nil {
		return err
	}

	fmt.Printf("Uploaded: %s -> %s\n", localPath, remotePath)
	return nil
}

func runSandboxDownload(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx, _ := getContext()

	id := args[0]
	remotePath := args[1]
	localPath := ""

	if len(args) == 3 {
		localPath = args[2]
	}

	content, err := client.Sandbox.DownloadFile(ctx, id, remotePath)
	if err != nil {
		return err
	}

	if localPath == "" {
		// Write to stdout
		fmt.Print(string(content))
	} else {
		if err := os.WriteFile(localPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		fmt.Printf("Downloaded: %s -> %s\n", remotePath, localPath)
	}

	return nil
}

func runSandboxWait(cmd *cobra.Command, args []string) error {
	client := getAPIClient()

	quiet, _ := cmd.Flags().GetBool("quiet")

	sandbox, err := client.Sandbox.WaitForReady(context.Background(), args[0], pollIntervalFlag, waitTimeoutFlag)
	if err != nil {
		return err
	}

	if !quiet {
		fmt.Printf("Sandbox %s is ready\n", sandbox.ID)
		fmt.Printf("Status: %s\n", sandbox.Status)
	} else {
		fmt.Println(sandbox.Status)
	}

	return nil
}

// Helper functions

func parseEnvVars(envs []string) map[string]string {
	result := make(map[string]string)
	for _, env := range envs {
		// Simple KEY=VALUE parsing
		for i, c := range env {
			if c == '=' {
				result[env[:i]] = env[i+1:]
				break
			}
		}
	}
	return result
}

func waitForSandbox(client *liteboxd.Client, ctx context.Context, id string) error {
	sandbox, err := client.Sandbox.WaitForReady(context.Background(), id, 2*time.Second, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("timeout waiting for sandbox to be ready: %w", err)
	}

	fmt.Printf("Sandbox ready: %s (status: %s)\n", sandbox.ID, sandbox.Status)
	return nil
}

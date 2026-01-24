package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/fslongjin/liteboxd/liteboxd-cli/internal/output"
	liteboxd "github.com/fslongjin/liteboxd/sdk/go"
	"github.com/spf13/cobra"
)

var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "Manage image prepull",
	Long:  `Prepull container images to all nodes for faster sandbox creation.`,
}

var (
	prepullImage    string
	prepullTemplate string
	prepullTimeout  time.Duration
	prepullWait     bool
)

var imagePrepullCmd = &cobra.Command{
	Use:   "prepull <image>",
	Short: "Trigger image prepull",
	Example: `  # Prepull an image
  liteboxd image prepull python:3.11-slim

  # Prepull from template
  liteboxd image prepull --template python-ds

  # Wait for completion
  liteboxd image prepull python:3.11-slim --wait`,
	RunE: runImagePrepull,
}

var imageListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List prepull tasks",
	Example: `  liteboxd image list`,
	RunE:    runImageList,
}

var imageDeleteCmd = &cobra.Command{
	Use:     "delete <id>",
	Short:   "Delete prepull task",
	Args:    cobra.ExactArgs(1),
	Example: `  liteboxd image delete <task-id>`,
	RunE:    runImageDelete,
}

func init() {
	rootCmd.AddCommand(imageCmd)

	// Prepull command
	imagePrepullCmd.Flags().StringVar(&prepullTemplate, "template", "", "Prepull image from template")
	imagePrepullCmd.Flags().DurationVar(&prepullTimeout, "timeout", 10*time.Minute, "Prepull timeout")
	imagePrepullCmd.Flags().BoolVar(&prepullWait, "wait", false, "Wait for completion")
	imageCmd.AddCommand(imagePrepullCmd)

	// List command
	imageListCmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml)")
	imageListCmd.Flags().String("image", "", "Filter by image")
	imageListCmd.Flags().String("status", "", "Filter by status")
	imageCmd.AddCommand(imageListCmd)

	// Delete command
	imageCmd.AddCommand(imageDeleteCmd)
}

func runImagePrepull(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx := context.Background()

	var resp *liteboxd.PrepullResponse
	var err error

	if prepullTemplate != "" {
		resp, err = client.Prepull.CreateForTemplate(ctx, prepullTemplate)
	} else {
		if len(args) == 0 {
			return fmt.Errorf("image argument or --template flag is required")
		}
		resp, err = client.Prepull.Create(ctx, args[0], int(prepullTimeout.Seconds()))
	}

	if err != nil {
		return err
	}

	fmt.Printf("Prepull task created: %s\n", resp.ID)
	fmt.Printf("Image: %s\n", resp.Image)
	fmt.Printf("Status: %s\n", resp.Status)

	if prepullWait {
		fmt.Println("Waiting for prepull to complete...")
		resp, err = client.Prepull.WaitForCompletion(context.Background(), resp.ID, 5*time.Second, 30*time.Minute)
		if err != nil {
			return err
		}
		fmt.Printf("Prepull completed: %s\n", resp.Status)
		if resp.Status == liteboxd.PrepullStatusCompleted {
			fmt.Printf("Progress: %d/%d nodes ready\n", resp.Progress.Ready, resp.Progress.Total)
		}
	}

	return nil
}

func runImageList(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx, _ := getContext()

	image, _ := cmd.Flags().GetString("image")
	status, _ := cmd.Flags().GetString("status")

	tasks, err := client.Prepull.List(ctx, image, status)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(output.ParseFormat(outputFormat))
	return formatter.Write(cmd.OutOrStdout(), tasks)
}

func runImageDelete(cmd *cobra.Command, args []string) error {
	client := getAPIClient()
	ctx, _ := getContext()

	if err := client.Prepull.Delete(ctx, args[0]); err != nil {
		return err
	}

	fmt.Printf("Deleted prepull task: %s\n", args[0])
	return nil
}

package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/fslongjin/liteboxd/sdk/go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	apiURL  string
	timeout time.Duration
	verbose bool
)

var rootCmd = &cobra.Command{
	Use:   "liteboxd",
	Short: "LiteBoxd CLI - Manage lightweight Kubernetes sandboxes",
	Long: `LiteBoxd CLI is a command-line tool for managing LiteBoxd sandboxes.

LiteBoxd is a lightweight Kubernetes-based sandbox system that provides
isolated container environments with lifecycle management.`,
	Version: "dev",
}

// Execute runs the root command
func Execute(version, commit, date string) error {
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, built at: %s)", version, commit, date)
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "Config file path (default: ~/.config/liteboxd/config.yaml)")
	rootCmd.PersistentFlags().StringVarP(&apiURL, "api-server", "s", "http://localhost:8080/api/v1", "API server address")
	rootCmd.PersistentFlags().DurationVar(&timeout, "timeout", 30*time.Second, "Request timeout")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	// Bind to viper
	viper.BindPFlag("api-server", rootCmd.PersistentFlags().Lookup("api-server"))
	viper.BindPFlag("timeout", rootCmd.PersistentFlags().Lookup("timeout"))
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
}

// initConfig reads in config file and ENV variables if set
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".liteboxd" (without extension)
		viper.AddConfigPath(home + "/.config/liteboxd")
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	// Read environment variables
	viper.SetEnvPrefix("LITEBOXD")
	viper.AutomaticEnv()

	// If a config file is found, read it in
	if err := viper.ReadInConfig(); err == nil && verbose {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

// getAPIClient creates and returns an API client
func getAPIClient() *liteboxd.Client {
	url := viper.GetString("api-server")
	if url == "" {
		url = "http://localhost:8080/api/v1"
	}

	timeout := viper.GetDuration("timeout")
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return liteboxd.NewClient(
		url,
		liteboxd.WithTimeout(timeout),
	)
}

// getContext returns a context with timeout
func getContext() (context.Context, context.CancelFunc) {
	timeout := viper.GetDuration("timeout")
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return context.WithTimeout(context.Background(), timeout)
}

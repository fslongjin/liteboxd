package gateway

import (
	"os"
	"time"
)

// Config holds the configuration for the gateway service
type Config struct {
	// Port is the port the gateway listens on
	Port string
	// KubeconfigPath is the path to the kubeconfig file
	KubeconfigPath string
	// RequestTimeout is the timeout for proxying requests
	RequestTimeout time.Duration
	// ShutdownTimeout is the timeout for graceful shutdown
	ShutdownTimeout time.Duration
	// UseK8sProxy enables using K8s API server proxy instead of direct pod connection
	// This is useful for local development with remote cluster
	UseK8sProxy bool
}

// LoadConfig loads configuration from environment variables with defaults
func LoadConfig() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081" // Default to 8081 to avoid conflict with main API
	}

	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		home := os.Getenv("HOME")
		kubeconfigPath = home + "/.kube/config"
	}

	useK8sProxy := os.Getenv("DEV_USE_K8S_PROXY") == "true"

	return &Config{
		Port:           port,
		KubeconfigPath: kubeconfigPath,
		RequestTimeout: 5 * time.Minute,
		ShutdownTimeout: 30 * time.Second,
		UseK8sProxy:    useK8sProxy,
	}
}

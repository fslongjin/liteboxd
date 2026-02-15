package gateway

import (
	"os"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/k8s"
)

// Config holds the configuration for the gateway service
type Config struct {
	// Port is the port the gateway listens on
	Port string
	// KubeconfigPath is the path to the kubeconfig file
	KubeconfigPath string
	// SandboxNamespace is the namespace where sandboxes run
	SandboxNamespace string
	// ControlNamespace is the namespace where control-plane services run
	ControlNamespace string
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
	sandboxNamespace := os.Getenv("SANDBOX_NAMESPACE")
	if sandboxNamespace == "" {
		sandboxNamespace = k8s.DefaultSandboxNamespace
	}
	controlNamespace := os.Getenv("CONTROL_NAMESPACE")
	if controlNamespace == "" {
		controlNamespace = k8s.DefaultControlNamespace
	}

	useK8sProxy := os.Getenv("DEV_USE_K8S_PROXY") == "true"

	return &Config{
		Port:             port,
		KubeconfigPath:   kubeconfigPath,
		SandboxNamespace: sandboxNamespace,
		ControlNamespace: controlNamespace,
		RequestTimeout:   5 * time.Minute,
		ShutdownTimeout:  30 * time.Second,
		UseK8sProxy:      useK8sProxy,
	}
}

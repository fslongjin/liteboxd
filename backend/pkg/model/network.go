package model

// NetworkSpec defines the network configuration for a sandbox or template
type NetworkSpec struct {
	// AllowInternetAccess enables outbound internet access for the sandbox.
	// When false (default), the sandbox can only access DNS and internal services.
	// When true, the sandbox can access the internet (HTTP/HTTPS only).
	AllowInternetAccess bool `json:"allowInternetAccess"`

	// AllowedDomains is an optional list of domains that the sandbox is allowed to access.
	// This is a future enhancement for domain whitelist functionality.
	// When empty, no domain filtering is applied (beyond the internet access setting).
	AllowedDomains []string `json:"allowedDomains,omitempty"`
}

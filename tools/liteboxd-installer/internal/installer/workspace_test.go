package installer

import (
	"strings"
	"testing"

	"github.com/fslongjin/liteboxd/tools/liteboxd-installer/internal/config"
)

func TestSystemOverlayKustomizationIncludesSecurityConfigPatch(t *testing.T) {
	i := &Installer{
		cfg: &config.Config{
			LiteBoxd: config.LiteBoxdConfig{
				NamespaceSystem:    "liteboxd-system",
				NamespaceSandbox:   "liteboxd-sandbox",
				IngressHost:        "liteboxd.local",
				GatewayIngressHost: "gateway.liteboxd.local",
				GatewayURL:         "http://liteboxd.local",
				Images: config.ImageConfig{
					API:     "img-api",
					Gateway: "img-gateway",
					Web:     "img-web",
				},
				Security: config.SecurityConfig{
					SandboxTokenEncryptionKey:   "my-secret-key",
					SandboxTokenEncryptionKeyID: "v2",
				},
			},
		},
	}

	out := i.systemOverlayKustomization()

	if !strings.Contains(out, "/data/SANDBOX_TOKEN_ENCRYPTION_KEY") {
		t.Fatalf("expected security key patch path in kustomization")
	}
	if !strings.Contains(out, "value: my-secret-key") {
		t.Fatalf("expected security key patch value in kustomization")
	}
	if !strings.Contains(out, "/data/SANDBOX_TOKEN_ENCRYPTION_KEY_ID") {
		t.Fatalf("expected security key id patch path in kustomization")
	}
	if !strings.Contains(out, "value: v2") {
		t.Fatalf("expected security key id patch value in kustomization")
	}
}

func TestSystemOverlayKustomizationPatchesTwoIngressHosts(t *testing.T) {
	i := &Installer{
		cfg: &config.Config{
			LiteBoxd: config.LiteBoxdConfig{
				NamespaceSystem:    "liteboxd-system",
				NamespaceSandbox:   "liteboxd-sandbox",
				IngressHost:        "myapp.example.com",
				GatewayIngressHost: "gateway.example.com",
				Images: config.ImageConfig{
					API:     "img-api",
					Gateway: "img-gateway",
					Web:     "img-web",
				},
			},
		},
	}

	out := i.systemOverlayKustomization()

	// Verify gateway host is patched at rules/0
	if !strings.Contains(out, "path: /spec/rules/0/host\n        value: gateway.example.com") {
		t.Fatalf("expected rules[0] host to be gateway.example.com, got:\n%s", out)
	}
	// Verify API+Web host is patched at rules/1
	if !strings.Contains(out, "path: /spec/rules/1/host\n        value: myapp.example.com") {
		t.Fatalf("expected rules[1] host to be myapp.example.com, got:\n%s", out)
	}
	// Verify there's no rules/2 patch (only two hosts now)
	if strings.Contains(out, "/spec/rules/2/host") {
		t.Fatalf("expected only two ingress rules (no rules[2]), got:\n%s", out)
	}
}

func TestSystemOverlayKustomizationGatewayURLDefault(t *testing.T) {
	i := &Installer{
		cfg: &config.Config{
			LiteBoxd: config.LiteBoxdConfig{
				NamespaceSystem:    "liteboxd-system",
				NamespaceSandbox:   "liteboxd-sandbox",
				IngressHost:        "myapp.example.com",
				GatewayIngressHost: "gateway.example.com",
				Images: config.ImageConfig{
					API:     "img-api",
					Gateway: "img-gateway",
					Web:     "img-web",
				},
			},
		},
	}

	out := i.systemOverlayKustomization()

	// When GatewayURL is empty, the default should be http://<GatewayIngressHost>
	if !strings.Contains(out, "value: http://gateway.example.com") {
		t.Fatalf("expected default GATEWAY_URL to be http://gateway.example.com, got:\n%s", out)
	}
}

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
				NamespaceSystem:  "liteboxd-system",
				NamespaceSandbox: "liteboxd-sandbox",
				IngressHost:      "liteboxd.local",
				GatewayURL:       "http://liteboxd.local",
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

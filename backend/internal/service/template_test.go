package service

import (
	"reflect"
	"testing"

	"github.com/fslongjin/liteboxd/backend/internal/model"
)

func TestNormalizeAllowedDomains(t *testing.T) {
	input := []string{" Example.COM ", "example.com.", "*.Sub.Example.COM", "EXAMPLE.com"}
	normalized, err := normalizeAllowedDomains(input)
	if err != nil {
		t.Fatalf("normalizeAllowedDomains error: %v", err)
	}

	expected := []string{"example.com", "*.sub.example.com"}
	if !reflect.DeepEqual(normalized, expected) {
		t.Fatalf("unexpected normalized domains: %v", normalized)
	}
}

func TestNormalizeAllowedDomainsRejectsInvalid(t *testing.T) {
	cases := []string{
		"http://example.com",
		"exa_mple.com",
		"example.com/path",
		"example.com:443",
		"ex*ample.com",
	}
	for _, value := range cases {
		if _, err := normalizeAllowedDomains([]string{value}); err == nil {
			t.Fatalf("expected error for %s", value)
		}
	}
}

func TestValidateNetworkSpecRequiresInternetAccess(t *testing.T) {
	spec := &model.NetworkSpec{
		AllowInternetAccess: false,
		AllowedDomains:      []string{"example.com"},
	}
	// After decoupling, this should NOT return an error.
	// The domains are stored but only applied when AllowInternetAccess is true.
	if err := validateNetworkSpec(spec); err != nil {
		t.Fatalf("unexpected error when allowInternetAccess is false with domains: %v", err)
	}
	// Domains should still be normalized
	if len(spec.AllowedDomains) != 1 || spec.AllowedDomains[0] != "example.com" {
		t.Fatalf("domains should be normalized: %v", spec.AllowedDomains)
	}
}

func TestValidateNetworkSpecWithInternetAccessAndDomains(t *testing.T) {
	spec := &model.NetworkSpec{
		AllowInternetAccess: true,
		AllowedDomains:      []string{" EXAMPLE.COM ", "*.test.org"},
	}
	if err := validateNetworkSpec(spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Check normalization
	expected := []string{"example.com", "*.test.org"}
	if len(spec.AllowedDomains) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(spec.AllowedDomains))
	}
	if spec.AllowedDomains[0] != expected[0] || spec.AllowedDomains[1] != expected[1] {
		t.Fatalf("unexpected normalized domains: %v", spec.AllowedDomains)
	}
}

func TestValidateNetworkSpecWithInternetAccessOnly(t *testing.T) {
	spec := &model.NetworkSpec{
		AllowInternetAccess: true,
		AllowedDomains:      []string{},
	}
	if err := validateNetworkSpec(spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateNetworkSpecDisabled(t *testing.T) {
	spec := &model.NetworkSpec{
		AllowInternetAccess: false,
		AllowedDomains:      []string{},
	}
	if err := validateNetworkSpec(spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePersistenceSpecValid(t *testing.T) {
	spec := &model.PersistenceSpec{
		Enabled:          true,
		Mode:             model.PersistenceModeRootFSOverlay,
		Size:             "1Gi",
		StorageClassName: "longhorn",
		ReclaimPolicy:    model.PersistenceReclaimDelete,
	}
	if err := validatePersistenceSpec(spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePersistenceSpecDefaultSize(t *testing.T) {
	spec := &model.PersistenceSpec{
		Enabled:          true,
		Mode:             model.PersistenceModeRootFSOverlay,
		StorageClassName: "longhorn",
	}
	if err := validatePersistenceSpec(spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Size != model.PersistenceDefaultSize {
		t.Fatalf("expected default size %q, got %q", model.PersistenceDefaultSize, spec.Size)
	}
}

func TestValidatePersistenceSpecRejectsLocalPath(t *testing.T) {
	spec := &model.PersistenceSpec{
		Enabled:          true,
		Mode:             model.PersistenceModeRootFSOverlay,
		Size:             "1Gi",
		StorageClassName: "local-path",
	}
	if err := validatePersistenceSpec(spec); err == nil {
		t.Fatalf("expected local-path to be rejected")
	}
}

func TestValidateSpecPersistentTemplateWithoutCommand(t *testing.T) {
	spec := &model.TemplateSpec{
		Image: "alpine:3.20",
		Resources: model.ResourceSpec{
			CPU:    "500m",
			Memory: "512Mi",
		},
		TTL: 0,
		Persistence: &model.PersistenceSpec{
			Enabled:          true,
			Mode:             model.PersistenceModeRootFSOverlay,
			Size:             "1Gi",
			StorageClassName: "longhorn",
		},
	}
	if err := validateSpec(spec); err != nil {
		t.Fatalf("expected validateSpec success for persistent template without command, got: %v", err)
	}
}

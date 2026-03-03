package model

import "testing"

func TestTemplateSpecApplyDefaults_KeepTTLZeroForPersistentTemplate(t *testing.T) {
	spec := TemplateSpec{
		Image: "alpine:3.20",
		TTL:   0,
		Persistence: &PersistenceSpec{
			Enabled: true,
			Mode:    PersistenceModeRootFSOverlay,
			Size:    "1Gi",
		},
	}

	spec.ApplyDefaults()

	if spec.TTL != 0 {
		t.Fatalf("expected ttl to remain 0 for persistent template, got %d", spec.TTL)
	}
}

func TestTemplateSpecApplyDefaults_KeepTTLZeroForEphemeralTemplate(t *testing.T) {
	spec := TemplateSpec{
		Image: "alpine:3.20",
		TTL:   0,
	}

	spec.ApplyDefaults()

	if spec.TTL != 0 {
		t.Fatalf("expected ttl to remain 0, got %d", spec.TTL)
	}
}

func TestTemplateSpecApplyDefaults_DefaultPersistenceSize(t *testing.T) {
	spec := TemplateSpec{
		Image: "alpine:3.20",
		Persistence: &PersistenceSpec{
			Enabled: true,
		},
	}

	spec.ApplyDefaults()

	if spec.Persistence == nil || spec.Persistence.Size != PersistenceDefaultSize {
		t.Fatalf("expected persistence size default to be %q, got %#v", PersistenceDefaultSize, spec.Persistence)
	}
}

package model

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTemplateYAMLPersistenceCamelCaseFields(t *testing.T) {
	const doc = `
apiVersion: liteboxd/v1
kind: SandboxTemplate
metadata:
  name: t1
spec:
  image: alpine:3.20
  resources:
    cpu: "500m"
    memory: "512Mi"
  ttl: 0
  persistence:
    enabled: true
    mode: rootfs-overlay
    size: 1Gi
    storageClassName: longhorn
    reclaimPolicy: Retain
`

	var tpl TemplateYAML
	if err := yaml.Unmarshal([]byte(doc), &tpl); err != nil {
		t.Fatalf("unmarshal yaml failed: %v", err)
	}
	if tpl.Spec.Persistence == nil {
		t.Fatalf("expected persistence to be parsed")
	}
	if tpl.Spec.Persistence.StorageClassName != "longhorn" {
		t.Fatalf("unexpected storageClassName: %q", tpl.Spec.Persistence.StorageClassName)
	}
	if tpl.Spec.Persistence.ReclaimPolicy != "Retain" {
		t.Fatalf("unexpected reclaimPolicy: %q", tpl.Spec.Persistence.ReclaimPolicy)
	}
}

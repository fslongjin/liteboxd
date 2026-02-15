package k8s

import (
	"context"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newTestClient() *Client {
	scheme := runtime.NewScheme()
	return &Client{
		clientset:     nil,
		dynamicClient: dynamicfake.NewSimpleDynamicClient(scheme),
		sandboxNS:     DefaultSandboxNamespace,
		controlNS:     DefaultControlNamespace,
	}
}

func TestApplyDomainAllowlistPolicyCreatesPolicy(t *testing.T) {
	ctx := context.Background()
	client := newTestClient()
	manager := NewNetworkPolicyManager(client)

	domains := []string{"example.com", "*.example.org"}
	if err := manager.ApplyDomainAllowlistPolicy(ctx, "abc123", domains); err != nil {
		t.Fatalf("ApplyDomainAllowlistPolicy error: %v", err)
	}

	resource := client.dynamicClient.Resource(ciliumPolicyGVR).Namespace(client.sandboxNS)
	policy, err := resource.Get(ctx, domainAllowlistPolicyName("abc123"), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get policy error: %v", err)
	}

	if policy.GetName() != domainAllowlistPolicyName("abc123") {
		t.Fatalf("unexpected policy name: %s", policy.GetName())
	}

	matchLabels, found, err := unstructured.NestedMap(policy.Object, "spec", "endpointSelector", "matchLabels")
	if err != nil || !found {
		t.Fatalf("missing endpointSelector.matchLabels")
	}
	if matchLabels[LabelSandboxID] != "abc123" {
		t.Fatalf("missing sandbox label in endpointSelector")
	}

	egress, found, err := unstructured.NestedSlice(policy.Object, "spec", "egress")
	if err != nil || !found || len(egress) == 0 {
		t.Fatalf("missing egress rules")
	}

	rule, ok := egress[0].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid egress rule format")
	}

	toFQDNs, found, err := unstructured.NestedSlice(rule, "toFQDNs")
	if err != nil || !found {
		t.Fatalf("missing toFQDNs")
	}

	foundMatchName := false
	foundMatchPattern := false
	for _, item := range toFQDNs {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if entry["matchName"] == "example.com" {
			foundMatchName = true
		}
		if entry["matchPattern"] == "*.example.org" {
			foundMatchPattern = true
		}
	}

	if !foundMatchName || !foundMatchPattern {
		t.Fatalf("toFQDNs missing expected entries: matchName=%v matchPattern=%v", foundMatchName, foundMatchPattern)
	}
}

func TestDeleteDomainAllowlistPolicy(t *testing.T) {
	ctx := context.Background()
	client := newTestClient()
	manager := NewNetworkPolicyManager(client)

	if err := manager.ApplyDomainAllowlistPolicy(ctx, "abc123", []string{"example.com"}); err != nil {
		t.Fatalf("ApplyDomainAllowlistPolicy error: %v", err)
	}

	if err := manager.DeleteDomainAllowlistPolicy(ctx, "abc123"); err != nil {
		t.Fatalf("DeleteDomainAllowlistPolicy error: %v", err)
	}

	resource := client.dynamicClient.Resource(ciliumPolicyGVR).Namespace(client.sandboxNS)
	_, err := resource.Get(ctx, domainAllowlistPolicyName("abc123"), metav1.GetOptions{})
	if err == nil || !apierrors.IsNotFound(err) {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestDomainAllowlistPolicyWithWildcard(t *testing.T) {
	ctx := context.Background()
	client := newTestClient()
	manager := NewNetworkPolicyManager(client)

	domains := []string{"*.example.com", "*.api.example.org"}
	if err := manager.ApplyDomainAllowlistPolicy(ctx, "wildcard-test", domains); err != nil {
		t.Fatalf("ApplyDomainAllowlistPolicy error: %v", err)
	}

	resource := client.dynamicClient.Resource(ciliumPolicyGVR).Namespace(client.sandboxNS)
	policy, err := resource.Get(ctx, domainAllowlistPolicyName("wildcard-test"), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get policy error: %v", err)
	}

	egress, found, err := unstructured.NestedSlice(policy.Object, "spec", "egress")
	if err != nil || !found || len(egress) == 0 {
		t.Fatalf("missing egress rules")
	}

	rule, ok := egress[0].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid egress rule format")
	}

	toFQDNs, found, err := unstructured.NestedSlice(rule, "toFQDNs")
	if err != nil || !found {
		t.Fatalf("missing toFQDNs")
	}

	if len(toFQDNs) != 2 {
		t.Fatalf("expected 2 toFQDNs entries, got %d", len(toFQDNs))
	}
}

func TestDomainAllowlistPolicyWithEmptyDomains(t *testing.T) {
	ctx := context.Background()
	client := newTestClient()
	manager := NewNetworkPolicyManager(client)

	// Empty domains should return nil (no policy applied)
	if err := manager.ApplyDomainAllowlistPolicy(ctx, "empty-test", []string{}); err != nil {
		t.Fatalf("ApplyDomainAllowlistPolicy with empty domains should not error: %v", err)
	}

	// Verify no policy was created
	resource := client.dynamicClient.Resource(ciliumPolicyGVR).Namespace(client.sandboxNS)
	_, err := resource.Get(ctx, domainAllowlistPolicyName("empty-test"), metav1.GetOptions{})
	if err == nil {
		t.Fatalf("expected policy not to be created with empty domains")
	}
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected not found error, got %v", err)
	}
}

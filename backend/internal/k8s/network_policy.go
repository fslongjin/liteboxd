package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// Annotation and label constants for network access
	AnnotationAccessToken = "liteboxd.io/access-token"
	LabelInternetAccess   = "liteboxd.io/internet-access"
)

// NetworkPolicyManager manages network policies for sandboxes
type NetworkPolicyManager struct {
	client *Client
}

// NewNetworkPolicyManager creates a new NetworkPolicyManager
func NewNetworkPolicyManager(client *Client) *NetworkPolicyManager {
	return &NetworkPolicyManager{client: client}
}

// EnsureDefaultPolicies ensures all base network policies are applied
func (m *NetworkPolicyManager) EnsureDefaultPolicies(ctx context.Context) error {
	// Ensure namespace exists first
	if err := m.client.EnsureNamespace(ctx); err != nil {
		return fmt.Errorf("failed to ensure namespace: %w", err)
	}

	policies := []struct {
		name string
		spec *networkingv1.NetworkPolicy
	}{
		{
			name: "default-deny-all",
			spec: m.defaultDenyAllPolicy(),
		},
		{
			name: "allow-dns",
			spec: m.allowDNSPolicy(),
		},
		{
			name: "deny-k8s-api",
			spec: m.denyK8sAPIPolicy(),
		},
		{
			name: "allow-gateway-ingress",
			spec: m.allowGatewayIngressPolicy(),
		},
		{
			name: "allow-internet-egress",
			spec: m.allowInternetEgressPolicy(),
		},
	}

	for _, p := range policies {
		if err := m.ensurePolicy(ctx, p.spec); err != nil {
			return fmt.Errorf("failed to ensure policy %s: %w", p.name, err)
		}
	}

	return nil
}

// ensurePolicy creates or updates a network policy
func (m *NetworkPolicyManager) ensurePolicy(ctx context.Context, policy *networkingv1.NetworkPolicy) error {
	existing, err := m.client.clientset.NetworkingV1().NetworkPolicies(SandboxNamespace).Get(ctx, policy.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = m.client.clientset.NetworkingV1().NetworkPolicies(SandboxNamespace).Create(ctx, policy, metav1.CreateOptions{})
			return err
		}
		return err
	}

	// Update existing policy
	policy.ResourceVersion = existing.ResourceVersion
	_, err = m.client.clientset.NetworkingV1().NetworkPolicies(SandboxNamespace).Update(ctx, policy, metav1.UpdateOptions{})
	return err
}

// defaultDenyAllPolicy creates a policy that denies all ingress and egress traffic
func (m *NetworkPolicyManager) defaultDenyAllPolicy() *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default-deny-all",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": LabelApp,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
		},
	}
}

// allowDNSPolicy creates a policy that allows DNS queries
func (m *NetworkPolicyManager) allowDNSPolicy() *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "allow-dns",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": LabelApp,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					To: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "kube-system",
								},
							},
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"k8s-app": "kube-dns",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolUDP}[0],
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 53},
						},
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 53},
						},
					},
				},
			},
		},
	}
}

// denyK8sAPIPolicy creates a policy that only allows DNS (implicitly denies K8s API)
func (m *NetworkPolicyManager) denyK8sAPIPolicy() *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "deny-k8s-api",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": LabelApp,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					// Only allow DNS, everything else is implicitly denied by default-deny-all
					To: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "kube-system",
								},
							},
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"k8s-app": "kube-dns",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolUDP}[0],
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 53},
						},
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 53},
						},
					},
				},
			},
		},
	}
}

// allowGatewayIngressPolicy creates a policy that allows ingress from gateway
func (m *NetworkPolicyManager) allowGatewayIngressPolicy() *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "allow-gateway-ingress",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": LabelApp,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							// Allow from gateway pods in the same namespace
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "liteboxd-gateway",
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"name": SandboxNamespace,
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 3000},
							EndPort:  func() *int32 { i := int32(65535); return &i }(),
						},
					},
				},
			},
		},
	}
}

// allowInternetEgressPolicy creates a policy that allows internet egress for pods with internet-access label
func (m *NetworkPolicyManager) allowInternetEgressPolicy() *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "allow-internet-egress",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":               LabelApp,
					LabelInternetAccess: "true",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					// Allow internet access (excluding private IPs)
					To: []networkingv1.NetworkPolicyPeer{
						{
							IPBlock: &networkingv1.IPBlock{
								CIDR: "0.0.0.0/0",
								Except: []string{
									"10.0.0.0/8",     // Private network
									"172.16.0.0/12",  // Private network
									"192.168.0.0/16", // Private network
									"127.0.0.0/8",    // Loopback
									"169.254.0.0/16", // Link-local
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 443},
						},
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 80},
						},
					},
				},
				{
					// Also allow DNS
					To: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "kube-system",
								},
							},
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"k8s-app": "kube-dns",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolUDP}[0],
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 53},
						},
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 53},
						},
					},
				},
			},
		},
	}
}

// AllowInternetAccess adds the internet-access label to a sandbox pod
func (m *NetworkPolicyManager) AllowInternetAccess(ctx context.Context, sandboxID string) error {
	return m.setInternetAccessLabel(ctx, sandboxID, "true")
}

// DenyInternetAccess removes the internet-access label from a sandbox pod
func (m *NetworkPolicyManager) DenyInternetAccess(ctx context.Context, sandboxID string) error {
	return m.setInternetAccessLabel(ctx, sandboxID, "false")
}

// setInternetAccessLabel sets or removes the internet-access label on a pod
func (m *NetworkPolicyManager) setInternetAccessLabel(ctx context.Context, sandboxID, value string) error {
	podName := fmt.Sprintf("sandbox-%s", sandboxID)

	pod, err := m.client.clientset.CoreV1().Pods(SandboxNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get pod: %w", err)
	}

	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}

	if value == "false" {
		delete(pod.Labels, LabelInternetAccess)
	} else {
		pod.Labels[LabelInternetAccess] = value
	}

	_, err = m.client.clientset.CoreV1().Pods(SandboxNamespace).Update(ctx, pod, metav1.UpdateOptions{})
	return err
}

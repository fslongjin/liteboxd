package k8s

import "k8s.io/apimachinery/pkg/runtime"
import kubefake "k8s.io/client-go/kubernetes/fake"

// NewClientForTest creates a Client with a fake Kubernetes clientset for use in tests.
func NewClientForTest(objects ...runtime.Object) *Client {
	return &Client{
		clientset: kubefake.NewSimpleClientset(objects...),
		sandboxNS: DefaultSandboxNamespace,
		controlNS: DefaultControlNamespace,
	}
}

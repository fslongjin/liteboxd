package k8s

import "k8s.io/apimachinery/pkg/runtime"
import kubefake "k8s.io/client-go/kubernetes/fake"

// NewClientForTest creates a Client with a fake Kubernetes clientset for use in tests.
func NewClientForTest(objects ...runtime.Object) *Client {
	return NewClientForTestWithSetup(nil, objects...)
}

// NewClientForTestWithSetup creates a fake client and lets tests customize clientset reactions.
func NewClientForTestWithSetup(setup func(*kubefake.Clientset), objects ...runtime.Object) *Client {
	clientset := kubefake.NewSimpleClientset(objects...)
	if setup != nil {
		setup(clientset)
	}
	return &Client{
		clientset: clientset,
		sandboxNS: DefaultSandboxNamespace,
		controlNS: DefaultControlNamespace,
	}
}

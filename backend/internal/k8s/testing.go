package k8s

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

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
		clientset:                   clientset,
		sandboxNS:                   DefaultSandboxNamespace,
		controlNS:                   DefaultControlNamespace,
		persistentRootFSHelperImage: DefaultPersistentRootFSHelperImage,
	}
}

// NewClientForTestWithDynamic creates a Client with fake typed and dynamic clients.
func NewClientForTestWithDynamic(objects ...runtime.Object) *Client {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		ciliumPolicyGVR: "CiliumNetworkPolicyList",
	}
	return &Client{
		clientset:                   kubefake.NewSimpleClientset(objects...),
		dynamicClient:               dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...),
		sandboxNS:                   DefaultSandboxNamespace,
		controlNS:                   DefaultControlNamespace,
		persistentRootFSHelperImage: DefaultPersistentRootFSHelperImage,
	}
}

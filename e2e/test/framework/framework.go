package framework

import (
	"github.com/appscode/go/crypto/rand"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	Image    = "linode/linode-cloud-controller-manager:latest"
	ApiToken = ""

	KubeConfigFile         = ""
	testServerResourceName = "e2e-test-server-" + rand.Characters(5)
)

const (
	MaxRetry        = 100
	TestServerImage = "appscode/test-server:2.3"
)

type Framework struct {
	restConfig *rest.Config
	kubeClient kubernetes.Interface
	namespace  string
	name       string
}

func New(
	restConfig *rest.Config,
	kubeClient kubernetes.Interface,
) *Framework {
	return &Framework{
		restConfig: restConfig,
		kubeClient: kubeClient,

		name:      "cloud-controller-manager",
		namespace: rand.WithUniqSuffix("ccm"),
	}
}

func (f *Framework) Invoke() *Invocation {
	r := &rootInvocation{
		Framework: f,
		app:       rand.WithUniqSuffix("csi-driver-e2e"),
	}
	return &Invocation{
		rootInvocation: r,
		LoadBalancer:   &lbInvocation{rootInvocation: r},
	}
}

type Invocation struct {
	*rootInvocation
	LoadBalancer *lbInvocation
}

type rootInvocation struct {
	*Framework
	app string
}

type lbInvocation struct {
	*rootInvocation
}

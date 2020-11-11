package framework

import (
	"fmt"

	"github.com/appscode/go/crypto/rand"
	"github.com/linode/linodego"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	Image    = "linode/linode-cloud-controller-manager:latest"
	ApiToken = ""

	KubeConfigFile         = ""
	TestServerResourceName = "e2e-test-server-" + rand.Characters(5)
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

	linodeClient linodego.Client
}

func generateNamespaceName() string {
	return rand.WithUniqSuffix("ccm")
}

func New(
	restConfig *rest.Config,
	kubeClient kubernetes.Interface,
	linodeClient linodego.Client,
) *Framework {
	return &Framework{
		restConfig:   restConfig,
		kubeClient:   kubeClient,
		linodeClient: linodeClient,

		name:      "cloud-controller-manager",
		namespace: generateNamespaceName(),
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

func (f *Framework) Recycle() error {
	if err := f.DeleteNamespace(); err != nil {
		return fmt.Errorf("failed to delete namespace (%s)", f.namespace)
	}

	f.namespace = generateNamespaceName()
	if err := f.CreateNamespace(); err != nil {
		return fmt.Errorf("failed to create namespace (%s)", f.namespace)
	}
	return nil
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

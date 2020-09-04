package linode

import (
	"fmt"
	"io"
	"os"

	"github.com/linode/linodego"
	"github.com/spf13/pflag"
	"k8s.io/client-go/informers"
	"k8s.io/kubernetes/pkg/cloudprovider"
	"k8s.io/kubernetes/pkg/controller"
)

const (
	// The name of this cloudprovider
	ProviderName   = "linode"
	accessTokenEnv = "LINODE_API_TOKEN"
	regionEnv      = "LINODE_REGION"
)

// Options is a configuration object for this cloudprovider implementation.
// We expect it to be initialized with flags external to this package, likely in
// main.go
var Options struct {
	KubeconfigFlag *pflag.Flag
	LinodeGoDebug  bool
}

type linodeCloud struct {
	client        *linodego.Client
	instances     cloudprovider.Instances
	zones         cloudprovider.Zones
	loadbalancers cloudprovider.LoadBalancer
}

func init() {
	cloudprovider.RegisterCloudProvider(
		ProviderName,
		func(io.Reader) (cloudprovider.Interface, error) {
			return newCloud()
		})
}

func newCloud() (cloudprovider.Interface, error) {
	// Read environment variables (from secrets)
	apiToken := os.Getenv(accessTokenEnv)
	if apiToken == "" {
		return nil, fmt.Errorf("%s must be set in the environment (use a k8s secret)", accessTokenEnv)
	}

	region := os.Getenv(regionEnv)
	if region == "" {
		return nil, fmt.Errorf("%s must be set in the environment (use a k8s secret)", regionEnv)
	}

	linodeClient := linodego.NewClient(nil)
	linodeClient.SetToken(apiToken)
	if Options.LinodeGoDebug {
		linodeClient.SetDebug(true)
	}
	linodeClient.SetUserAgent(fmt.Sprintf("linode-cloud-controller-manager %s", linodego.DefaultUserAgent))

	// Return struct that satisfies cloudprovider.Interface
	return &linodeCloud{
		client:        &linodeClient,
		instances:     newInstances(&linodeClient),
		zones:         newZones(&linodeClient, region),
		loadbalancers: newLoadbalancers(&linodeClient, region),
	}, nil
}

func (c *linodeCloud) Initialize(clientBuilder controller.ControllerClientBuilder) {
	kubeclient := clientBuilder.ClientOrDie("linode-shared-informers")
	sharedInformer := informers.NewSharedInformerFactory(kubeclient, 0)
	serviceInformer := sharedInformer.Core().V1().Services()

	serviceController := newServiceController(c.loadbalancers.(*loadbalancers), serviceInformer)

	// in future version of the cloudprovider package, we should use the stopCh provided to
	// (cloudprovider.Interface).Initialize instead
	forever := make(chan struct{})
	go serviceController.Run(forever)
}

func (c *linodeCloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	return c.loadbalancers, true
}

func (c *linodeCloud) Instances() (cloudprovider.Instances, bool) {
	return c.instances, true
}

func (c *linodeCloud) Zones() (cloudprovider.Zones, bool) {
	return c.zones, true
}

func (c *linodeCloud) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

func (c *linodeCloud) Routes() (cloudprovider.Routes, bool) {
	return nil, false
}

func (c *linodeCloud) ProviderName() string {
	return ProviderName
}

func (c *linodeCloud) ScrubDNS(nameservers, searches []string) (nsOut, srchOut []string) {
	return nil, nil
}

func (c *linodeCloud) HasClusterID() bool {
	return true
}

package linode

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/spf13/pflag"
	"golang.org/x/exp/slices"
	"k8s.io/client-go/informers"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client"
)

const (
	// The name of this cloudprovider
	ProviderName       = "linode"
	accessTokenEnv     = "LINODE_API_TOKEN"
	regionEnv          = "LINODE_REGION"
	ciliumLBType       = "cilium-bgp"
	nodeBalancerLBType = "nodebalancer"
)

var supportedLoadBalancerTypes = []string{ciliumLBType, nodeBalancerLBType}

// Options is a configuration object for this cloudprovider implementation.
// We expect it to be initialized with flags external to this package, likely in
// main.go
var Options struct {
	KubeconfigFlag        *pflag.Flag
	LinodeGoDebug         bool
	EnableRouteController bool
	// deprecated: use VPCNames instead
	VPCName               string
	VPCNames              string
	LoadBalancerType      string
	BGPNodeSelector       string
	LinodeExternalNetwork *net.IPNet
}

type linodeCloud struct {
	client        client.Client
	instances     cloudprovider.InstancesV2
	loadbalancers cloudprovider.LoadBalancer
	routes        cloudprovider.Routes
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

	// set timeout used by linodeclient for API calls
	timeout := client.DefaultClientTimeout
	if raw, ok := os.LookupEnv("LINODE_REQUEST_TIMEOUT_SECONDS"); ok {
		if t, err := strconv.Atoi(raw); err == nil && t > 0 {
			timeout = time.Duration(t) * time.Second
		}
	}

	linodeClient, err := client.New(apiToken, timeout)
	if err != nil {
		return nil, fmt.Errorf("client was not created succesfully: %w", err)
	}

	if Options.LinodeGoDebug {
		linodeClient.SetDebug(true)
	}

	if Options.VPCName != "" && Options.VPCNames != "" {
		return nil, fmt.Errorf("cannot have both vpc-name and vpc-names set")
	}

	if Options.VPCName != "" {
		klog.Warningf("vpc-name flag is deprecated. Use vpc-names instead")
		Options.VPCNames = Options.VPCName
	}

	routes, err := newRoutes(linodeClient)
	if err != nil {
		return nil, fmt.Errorf("routes client was not created successfully: %w", err)
	}

	if Options.LoadBalancerType != "" && !slices.Contains(supportedLoadBalancerTypes, Options.LoadBalancerType) {
		return nil, fmt.Errorf(
			"unsupported default load-balancer type %s. Options are %v",
			Options.LoadBalancerType,
			supportedLoadBalancerTypes,
		)
	}

	// create struct that satisfies cloudprovider.Interface
	lcloud := &linodeCloud{
		client:        linodeClient,
		instances:     newInstances(linodeClient),
		loadbalancers: newLoadbalancers(linodeClient, region),
		routes:        routes,
	}
	return lcloud, nil
}

func (c *linodeCloud) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stopCh <-chan struct{}) {
	kubeclient := clientBuilder.ClientOrDie("linode-shared-informers")
	sharedInformer := informers.NewSharedInformerFactory(kubeclient, 0)
	serviceInformer := sharedInformer.Core().V1().Services()
	nodeInformer := sharedInformer.Core().V1().Nodes()

	serviceController := newServiceController(c.loadbalancers.(*loadbalancers), serviceInformer)
	go serviceController.Run(stopCh)

	nodeController := newNodeController(kubeclient, c.client, nodeInformer)
	go nodeController.Run(stopCh)
}

func (c *linodeCloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	return c.loadbalancers, true
}

func (c *linodeCloud) Instances() (cloudprovider.Instances, bool) {
	return nil, false
}

func (c *linodeCloud) InstancesV2() (cloudprovider.InstancesV2, bool) {
	return c.instances, true
}

func (c *linodeCloud) Zones() (cloudprovider.Zones, bool) {
	return nil, false
}

func (c *linodeCloud) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

func (c *linodeCloud) Routes() (cloudprovider.Routes, bool) {
	if Options.EnableRouteController {
		return c.routes, true
	}
	return nil, false
}

func (c *linodeCloud) ProviderName() string {
	return ProviderName
}

func (c *linodeCloud) ScrubDNS(_, _ []string) (nsOut, srchOut []string) {
	return nil, nil
}

func (c *linodeCloud) HasClusterID() bool {
	return true
}

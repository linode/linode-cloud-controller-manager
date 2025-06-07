package linode

import (
	"context"
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
	ProviderName           = "linode"
	accessTokenEnv         = "LINODE_API_TOKEN"
	regionEnv              = "LINODE_REGION"
	ciliumLBType           = "cilium-bgp"
	nodeBalancerLBType     = "nodebalancer"
	tokenHealthCheckPeriod = 5 * time.Minute
)

var supportedLoadBalancerTypes = []string{ciliumLBType, nodeBalancerLBType}

// Options is a configuration object for this cloudprovider implementation.
// We expect it to be initialized with flags external to this package, likely in
// main.go
var Options struct {
	KubeconfigFlag           *pflag.Flag
	LinodeGoDebug            bool
	EnableRouteController    bool
	EnableTokenHealthChecker bool
	// Deprecated: use VPCNames instead
	VPCName                           string
	VPCNames                          string
	SubnetNames                       string
	LoadBalancerType                  string
	BGPNodeSelector                   string
	IpHolderSuffix                    string
	LinodeExternalNetwork             *net.IPNet
	NodeBalancerTags                  []string
	DefaultNBType                     string
	NodeBalancerBackendIPv4Subnet     string
	NodeBalancerBackendIPv4SubnetID   int
	NodeBalancerBackendIPv4SubnetName string
	DisableNodeBalancerVPCBackends    bool
	GlobalStopChannel                 chan<- struct{}
	EnableIPv6ForLoadBalancers        bool
	AllocateNodeCIDRs                 bool
	ClusterCIDRIPv4                   string
	NodeCIDRMaskSizeIPv4              int
	NodeCIDRMaskSizeIPv6              int
}

type linodeCloud struct {
	client                   client.Client
	instances                cloudprovider.InstancesV2
	loadbalancers            cloudprovider.LoadBalancer
	routes                   cloudprovider.Routes
	linodeTokenHealthChecker *healthChecker
}

var (
	instanceCache     *instances
	ipHolderCharLimit int = 23
)

func init() {
	registerMetrics()
	cloudprovider.RegisterCloudProvider(
		ProviderName,
		func(io.Reader) (cloudprovider.Interface, error) {
			return newCloud()
		})
}

// newLinodeClientWithPrometheus creates a new client kept in its own local
// scope and returns an instrumented one that should be used and passed around
func newLinodeClientWithPrometheus(apiToken string, timeout time.Duration) (client.Client, error) {
	linodeClient, err := client.New(apiToken, timeout)
	if err != nil {
		return nil, fmt.Errorf("client was not created successfully: %w", err)
	}

	if Options.LinodeGoDebug {
		linodeClient.SetDebug(true)
	}

	return client.NewClientWithPrometheus(linodeClient), nil
}

func newCloud() (cloudprovider.Interface, error) {
	region := os.Getenv(regionEnv)
	if region == "" {
		return nil, fmt.Errorf("%s must be set in the environment (use a k8s secret)", regionEnv)
	}

	// Read environment variables (from secrets)
	apiToken := os.Getenv(accessTokenEnv)
	if apiToken == "" {
		return nil, fmt.Errorf("%s must be set in the environment (use a k8s secret)", accessTokenEnv)
	}

	// set timeout used by linodeclient for API calls
	timeout := client.DefaultClientTimeout
	if raw, ok := os.LookupEnv("LINODE_REQUEST_TIMEOUT_SECONDS"); ok {
		if t, err := strconv.Atoi(raw); err == nil && t > 0 {
			timeout = time.Duration(t) * time.Second
		}
	}

	linodeClient, err := newLinodeClientWithPrometheus(apiToken, timeout)
	if err != nil {
		return nil, err
	}

	var healthChecker *healthChecker

	if Options.EnableTokenHealthChecker {
		var authenticated bool
		authenticated, err = client.CheckClientAuthenticated(context.TODO(), linodeClient)
		if err != nil {
			return nil, fmt.Errorf("linode client authenticated connection error: %w", err)
		}

		if !authenticated {
			return nil, fmt.Errorf("linode api token %q is invalid", accessTokenEnv)
		}

		healthChecker = newHealthChecker(linodeClient, tokenHealthCheckPeriod, Options.GlobalStopChannel)
	}

	if Options.VPCName != "" && Options.VPCNames != "" {
		return nil, fmt.Errorf("cannot have both vpc-name and vpc-names set")
	}

	if Options.VPCName != "" {
		klog.Warningf("vpc-name flag is deprecated. Use vpc-names instead")
		Options.VPCNames = Options.VPCName
	}

	// SubnetNames can't be used without VPCNames also being set
	if Options.SubnetNames != "" && Options.VPCNames == "" {
		klog.Warningf("failed to set flag subnet-names: vpc-names must be set to a non-empty value")
		Options.SubnetNames = ""
	}

	if Options.NodeBalancerBackendIPv4SubnetID != 0 && Options.NodeBalancerBackendIPv4SubnetName != "" {
		return nil, fmt.Errorf("cannot have both --nodebalancer-backend-ipv4-subnet-id and --nodebalancer-backend-ipv4-subnet-name set")
	}

	if Options.DisableNodeBalancerVPCBackends {
		klog.Infof("NodeBalancer VPC backends are disabled, no VPC backends will be created for NodeBalancers")
		Options.NodeBalancerBackendIPv4SubnetID = 0
		Options.NodeBalancerBackendIPv4SubnetName = ""
	} else if Options.NodeBalancerBackendIPv4SubnetName != "" {
		Options.NodeBalancerBackendIPv4SubnetID, err = getNodeBalancerBackendIPv4SubnetID(linodeClient)
		if err != nil {
			return nil, fmt.Errorf("failed to get backend IPv4 subnet ID for subnet name %s: %w", Options.NodeBalancerBackendIPv4SubnetName, err)
		}
		klog.Infof("Using NodeBalancer backend IPv4 subnet ID %d for subnet name %s", Options.NodeBalancerBackendIPv4SubnetID, Options.NodeBalancerBackendIPv4SubnetName)
	}

	instanceCache = newInstances(linodeClient)
	routes, err := newRoutes(linodeClient, instanceCache)
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

	if Options.IpHolderSuffix != "" {
		klog.Infof("Using IP holder suffix '%s'\n", Options.IpHolderSuffix)
	}

	if len(Options.IpHolderSuffix) > ipHolderCharLimit {
		msg := fmt.Sprintf("ip-holder-suffix must be %d characters or less: %s is %d characters\n", ipHolderCharLimit, Options.IpHolderSuffix, len(Options.IpHolderSuffix))
		klog.Error(msg)
		return nil, fmt.Errorf("%s", msg)
	}

	// create struct that satisfies cloudprovider.Interface
	lcloud := &linodeCloud{
		client:                   linodeClient,
		instances:                instanceCache,
		loadbalancers:            newLoadbalancers(linodeClient, region),
		routes:                   routes,
		linodeTokenHealthChecker: healthChecker,
	}
	return lcloud, nil
}

func (c *linodeCloud) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stopCh <-chan struct{}) {
	kubeclient := clientBuilder.ClientOrDie("linode-shared-informers")
	sharedInformer := informers.NewSharedInformerFactory(kubeclient, 0)
	serviceInformer := sharedInformer.Core().V1().Services()
	nodeInformer := sharedInformer.Core().V1().Nodes()

	if err := startNodeIpamController(stopCh, c, nodeInformer, kubeclient); err != nil {
		klog.Fatal("starting of node ipam controller failed", err)
	}

	if c.linodeTokenHealthChecker != nil {
		go c.linodeTokenHealthChecker.Run(stopCh)
	}

	lb, assertion := c.loadbalancers.(*loadbalancers)
	if !assertion {
		klog.Error("type assertion during Initialize() failed")
		return
	}
	serviceController := newServiceController(lb, serviceInformer)
	go serviceController.Run(stopCh)

	nodeController := newNodeController(kubeclient, c.client, nodeInformer, instanceCache)
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

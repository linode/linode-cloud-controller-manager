package linode

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/exp/slices"
	"k8s.io/client-go/informers"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/options"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/services"
)

const (
	// The name of this cloudprovider
	ProviderName             = "linode"
	accessTokenEnv           = "LINODE_API_TOKEN"
	regionEnv                = "LINODE_REGION"
	tokenFilePathEnv         = "LINODE_API_TOKEN_FILE"
	defaultTokenFilePath     = "/var/run/secrets/linode/api-token"
	tokenCacheTTLEnv         = "LINODE_API_TOKEN_CACHE_TTL_SECONDS"
	defaultTokenFileCacheTTL = time.Minute
	ciliumLBType             = "cilium-bgp"
	nodeBalancerLBType       = "nodebalancer"
	tokenHealthCheckPeriod   = 5 * time.Minute
)

var supportedLoadBalancerTypes = []string{ciliumLBType, nodeBalancerLBType}

type linodeCloud struct {
	client                   client.Client
	instances                cloudprovider.InstancesV2
	loadbalancers            cloudprovider.LoadBalancer
	routes                   cloudprovider.Routes
	linodeTokenHealthChecker *healthChecker
}

var (
	instanceCache               *services.Instances
	ipHolderCharLimit           int = 23
	NodeBalancerPrefixCharLimit int = 19
)

type tokenFileProvider struct {
	path     string
	now      func() time.Time
	cacheTTL time.Duration

	mu          sync.RWMutex
	cachedToken string
	expiresAt   time.Time
}

type staticTokenProvider struct {
	token string
}

func (t staticTokenProvider) GetToken(context.Context) (string, error) {
	if t.token == "" {
		return "", fmt.Errorf("%s must be set in the environment (use a k8s secret)", accessTokenEnv)
	}

	return t.token, nil
}

func (t *tokenFileProvider) String() string {
	return t.path
}

func (t *tokenFileProvider) nowTime() time.Time {
	if t.now != nil {
		return t.now()
	}

	return time.Now()
}

func (t *tokenFileProvider) GetToken(_ context.Context) (string, error) {
	now := t.nowTime()
	cacheTTL := t.cacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultTokenFileCacheTTL
	}

	t.mu.RLock()
	if t.cachedToken != "" && now.Before(t.expiresAt) {
		token := t.cachedToken
		t.mu.RUnlock()
		return token, nil
	}
	t.mu.RUnlock()

	rawToken, err := os.ReadFile(t.path)
	if err != nil {
		return "", fmt.Errorf("failed to read token file %q: %w", t.String(), err)
	}

	token := strings.TrimSpace(string(rawToken))
	if token == "" {
		return "", fmt.Errorf("token file %q is empty", t.String())
	}

	t.mu.Lock()
	t.cachedToken = token
	t.expiresAt = t.nowTime().Add(cacheTTL)
	t.mu.Unlock()

	return token, nil
}

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
func newLinodeClientWithPrometheus(apiToken string, timeout time.Duration, tokenProvider client.TokenProvider) (client.Client, error) {
	linodeClient, err := client.New(apiToken, timeout, tokenProvider)
	if err != nil {
		return nil, fmt.Errorf("client was not created successfully: %w", err)
	}

	if options.Options.LinodeGoDebug {
		linodeClient.SetDebug(true)
	}

	return client.NewClientWithPrometheus(linodeClient), nil
}
func tokenFileCacheTTLFromEnv() time.Duration {
	tokenCacheTTL := defaultTokenFileCacheTTL
	if raw, ok := os.LookupEnv(tokenCacheTTLEnv); ok {
		if ttlSeconds, err := strconv.Atoi(raw); err == nil && ttlSeconds > 0 {
			tokenCacheTTL = time.Duration(ttlSeconds) * time.Second
		}
	}

	return tokenCacheTTL
}

func tokenProviderFromFileOrEnv() (string, client.TokenProvider, string, error) {
	tokenFilePath := strings.TrimSpace(os.Getenv(tokenFilePathEnv))
	if tokenFilePath == "" {
		tokenFilePath = defaultTokenFilePath
	}

	fileProvider := tokenFileProvider{
		path:     tokenFilePath,
		cacheTTL: tokenFileCacheTTLFromEnv(),
	}

	apiToken, fileErr := fileProvider.GetToken(context.Background())
	if fileErr == nil {
		return apiToken, fileProvider.GetToken, fmt.Sprintf("file %q", fileProvider.String()), nil
	}

	envToken := strings.TrimSpace(os.Getenv(accessTokenEnv))
	if envToken != "" {
		envProvider := staticTokenProvider{token: envToken}
		return envToken, envProvider.GetToken, fmt.Sprintf("environment variable %q", accessTokenEnv), nil
	}

	return "", nil, "", fmt.Errorf("failed to load linode api token from %s=%q: %w; fallback %s is not set", tokenFilePathEnv, tokenFilePath, fileErr, accessTokenEnv)
}

func newCloud() (cloudprovider.Interface, error) {
	region := os.Getenv(regionEnv)
	if region == "" {
		return nil, fmt.Errorf("%s must be set in the environment (use a k8s secret)", regionEnv)
	}

	apiToken, tokenProvider, tokenSourceDescription, err := tokenProviderFromFileOrEnv()
	if err != nil {
		return nil, err
	}

	// set timeout used by linodeclient for API calls
	timeout := client.DefaultClientTimeout
	if raw, ok := os.LookupEnv("LINODE_REQUEST_TIMEOUT_SECONDS"); ok {
		if t, atoiErr := strconv.Atoi(raw); atoiErr == nil && t > 0 {
			timeout = time.Duration(t) * time.Second
		}
	}

	linodeClient, err := newLinodeClientWithPrometheus(apiToken, timeout, tokenProvider)
	if err != nil {
		return nil, err
	}

	var healthChecker *healthChecker

	if options.Options.EnableTokenHealthChecker {
		var authenticated bool
		authenticated, err = client.CheckClientAuthenticated(context.TODO(), linodeClient)
		if err != nil {
			return nil, fmt.Errorf("linode client authenticated connection error: %w", err)
		}

		if !authenticated {
			return nil, fmt.Errorf("linode api token from %s is invalid", tokenSourceDescription)
		}

		healthChecker = newHealthChecker(linodeClient, tokenHealthCheckPeriod, options.Options.GlobalStopChannel)
	}

	err = services.ValidateAndSetVPCSubnetFlags(linodeClient)
	if err != nil {
		return nil, fmt.Errorf("failed to validate VPC and subnet flags: %w", err)
	}

	if options.Options.NodeBalancerBackendIPv4SubnetID != 0 && options.Options.NodeBalancerBackendIPv4SubnetName != "" {
		return nil, fmt.Errorf("cannot have both --nodebalancer-backend-ipv4-subnet-id and --nodebalancer-backend-ipv4-subnet-name set")
	}

	if options.Options.DisableNodeBalancerVPCBackends {
		klog.Infof("NodeBalancer VPC backends are disabled, no VPC backends will be created for NodeBalancers")
		options.Options.NodeBalancerBackendIPv4SubnetID = 0
		options.Options.NodeBalancerBackendIPv4SubnetName = ""
	} else if options.Options.NodeBalancerBackendIPv4SubnetName != "" {
		options.Options.NodeBalancerBackendIPv4SubnetID, err = services.GetNodeBalancerBackendIPv4SubnetID(linodeClient)
		if err != nil {
			return nil, fmt.Errorf("failed to get backend IPv4 subnet ID for subnet name %s: %w", options.Options.NodeBalancerBackendIPv4SubnetName, err)
		}
		klog.Infof("Using NodeBalancer backend IPv4 subnet ID %d for subnet name %s", options.Options.NodeBalancerBackendIPv4SubnetID, options.Options.NodeBalancerBackendIPv4SubnetName)
	}

	instanceCache = services.NewInstances(linodeClient)
	routes, err := newRoutes(linodeClient, instanceCache)
	if err != nil {
		return nil, fmt.Errorf("routes client was not created successfully: %w", err)
	}

	if options.Options.LoadBalancerType != "" && !slices.Contains(supportedLoadBalancerTypes, options.Options.LoadBalancerType) {
		return nil, fmt.Errorf(
			"unsupported default load-balancer type %s. options.Options are %v",
			options.Options.LoadBalancerType,
			supportedLoadBalancerTypes,
		)
	}

	if options.Options.IpHolderSuffix != "" {
		klog.Infof("Using IP holder suffix '%s'\n", options.Options.IpHolderSuffix)
	}

	if len(options.Options.IpHolderSuffix) > ipHolderCharLimit {
		msg := fmt.Sprintf("ip-holder-suffix must be %d characters or less: %s is %d characters\n", ipHolderCharLimit, options.Options.IpHolderSuffix, len(options.Options.IpHolderSuffix))
		klog.Error(msg)
		return nil, fmt.Errorf("%s", msg)
	}

	if len(options.Options.NodeBalancerPrefix) > NodeBalancerPrefixCharLimit {
		msg := fmt.Sprintf("nodebalancer-prefix must be %d characters or less: %s is %d characters\n", NodeBalancerPrefixCharLimit, options.Options.NodeBalancerPrefix, len(options.Options.NodeBalancerPrefix))
		klog.Error(msg)
		return nil, fmt.Errorf("%s", msg)
	}

	validPrefix := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validPrefix.MatchString(options.Options.NodeBalancerPrefix) {
		msg := fmt.Sprintf("nodebalancer-prefix must be no empty and use only letters, numbers, underscores, and dashes: %s\n", options.Options.NodeBalancerPrefix)
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
	if options.Options.EnableRouteController {
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

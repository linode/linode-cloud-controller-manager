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
	accessTokenEnv           = "LINODE_API_TOKEN" //gosec:disable G101 -- These are not hardcoded credentials.
	regionEnv                = "LINODE_REGION"
	tokenFilePathEnv         = "LINODE_API_TOKEN_FILE"              //gosec:disable G101 -- These are not hardcoded credentials.
	defaultTokenFilePath     = "/var/run/secrets/linode/api-token"  //gosec:disable G101 -- These are not hardcoded credentials.
	tokenCacheTTLEnv         = "LINODE_API_TOKEN_CACHE_TTL_SECONDS" //gosec:disable G101 -- These are not hardcoded credentials.
	defaultTokenFileCacheTTL = time.Minute
	ciliumLBType             = "cilium-bgp"
	nodeBalancerLBType       = "nodebalancer"
	tokenHealthCheckPeriod   = 5 * time.Minute
)

var supportedLoadBalancerTypes = []string{nodeBalancerLBType}

type linodeCloud struct {
	client                   client.Client
	instances                cloudprovider.InstancesV2
	loadbalancers            cloudprovider.LoadBalancer
	routes                   cloudprovider.Routes
	linodeTokenHealthChecker *healthChecker
}

var (
	instanceCache               *services.Instances
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
func newLinodeClientWithPrometheus(timeout time.Duration, tokenProvider client.TokenProvider) (client.Client, error) {
	linodeClient, err := client.New(timeout, tokenProvider)
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

func tokenProviderFromFileOrEnv() (client.TokenProvider, string, error) {
	tokenFilePath := strings.TrimSpace(os.Getenv(tokenFilePathEnv))
	if tokenFilePath == "" {
		tokenFilePath = defaultTokenFilePath
	}

	fileProvider := tokenFileProvider{
		path:     tokenFilePath,
		cacheTTL: tokenFileCacheTTLFromEnv(),
	}

	_, fileErr := fileProvider.GetToken(context.Background())
	if fileErr == nil {
		return fileProvider.GetToken, fmt.Sprintf("file %q", fileProvider.String()), nil
	}

	if envToken := strings.TrimSpace(os.Getenv(accessTokenEnv)); envToken != "" {
		envProvider := staticTokenProvider{token: envToken}
		return envProvider.GetToken, fmt.Sprintf("environment variable %q", accessTokenEnv), nil
	}

	return nil, "", fmt.Errorf("failed to load linode api token from %s=%q: %w; fallback %s is not set", tokenFilePathEnv, tokenFilePath, fileErr, accessTokenEnv)
}

// requestTimeoutFromEnv resolves the client timeout used for Linode API calls,
// honoring LINODE_REQUEST_TIMEOUT_SECONDS if set to a valid positive integer.
func requestTimeoutFromEnv() time.Duration {
	timeout := client.DefaultClientTimeout
	if raw, ok := os.LookupEnv("LINODE_REQUEST_TIMEOUT_SECONDS"); ok {
		if t, atoiErr := strconv.Atoi(raw); atoiErr == nil && t > 0 {
			timeout = time.Duration(t) * time.Second
		}
	}
	return timeout
}

// setupTokenHealthChecker validates the Linode API token (when enabled) and
// returns a healthChecker to be run by the cloud provider, if applicable.
func setupTokenHealthChecker(linodeClient client.Client, tokenSourceDescription string) (*healthChecker, error) {
	if !options.Options.EnableTokenHealthChecker {
		return nil, nil //nolint:nilnil // nil, nil indicates the health checker is disabled, not an error
	}

	authenticated, err := client.CheckClientAuthenticated(context.TODO(), linodeClient)
	if err != nil {
		return nil, fmt.Errorf("linode client authenticated connection error: %w", err)
	}

	if !authenticated {
		return nil, fmt.Errorf("linode api token from %s is invalid", tokenSourceDescription)
	}

	return newHealthChecker(linodeClient, tokenHealthCheckPeriod, options.Options.GlobalStopChannel), nil
}

// setupNodeBalancerBackendSubnet resolves and validates the NodeBalancer
// backend IPv4 subnet options, mutating options.Options as needed.
func setupNodeBalancerBackendSubnet(linodeClient client.Client) error {
	if options.Options.NodeBalancerBackendIPv4SubnetID != 0 && options.Options.NodeBalancerBackendIPv4SubnetName != "" {
		return fmt.Errorf("cannot have both --nodebalancer-backend-ipv4-subnet-id and --nodebalancer-backend-ipv4-subnet-name set")
	}

	switch {
	case options.Options.DisableNodeBalancerVPCBackends:
		klog.Infof("NodeBalancer VPC backends are disabled, no VPC backends will be created for NodeBalancers")
		options.Options.NodeBalancerBackendIPv4SubnetID = 0
		options.Options.NodeBalancerBackendIPv4SubnetName = ""
	case options.Options.NodeBalancerBackendIPv4SubnetName != "":
		subnetID, err := services.GetNodeBalancerBackendIPv4SubnetID(linodeClient)
		if err != nil {
			return fmt.Errorf("failed to get backend IPv4 subnet ID for subnet name %s: %w", options.Options.NodeBalancerBackendIPv4SubnetName, err)
		}
		options.Options.NodeBalancerBackendIPv4SubnetID = subnetID
		klog.Infof("Using NodeBalancer backend IPv4 subnet ID %d for subnet name %s", options.Options.NodeBalancerBackendIPv4SubnetID, options.Options.NodeBalancerBackendIPv4SubnetName)
	}

	return nil
}

// applyDeprecatedOptionWarnings logs warnings for, and normalizes, deprecated
// CLI options that are retained only for backwards compatibility.
func applyDeprecatedOptionWarnings() {
	if options.Options.LoadBalancerType == ciliumLBType {
		klog.Warningf("--load-balancer-type=%s is deprecated and has no effect; using %s", ciliumLBType, nodeBalancerLBType)
		options.Options.LoadBalancerType = nodeBalancerLBType
	}

	if options.Options.BGPNodeSelector != "" {
		klog.Warning("--bgp-node-selector is deprecated and has no effect; it is retained for backwards compatibility")
	}

	if options.Options.IpHolderSuffix != "" {
		klog.Warning("--ip-holder-suffix is deprecated and has no effect; it is retained for backwards compatibility")
	}
}

// validateNodeBalancerOptions validates the configured load-balancer type and
// NodeBalancer prefix options.
func validateNodeBalancerOptions() error {
	if options.Options.LoadBalancerType != "" && !slices.Contains(supportedLoadBalancerTypes, options.Options.LoadBalancerType) {
		return fmt.Errorf(
			"unsupported default load-balancer type %s. options.Options are %v",
			options.Options.LoadBalancerType,
			supportedLoadBalancerTypes,
		)
	}

	if len(options.Options.NodeBalancerPrefix) > NodeBalancerPrefixCharLimit {
		msg := fmt.Sprintf("nodebalancer-prefix must be %d characters or less: %s is %d characters\n", NodeBalancerPrefixCharLimit, options.Options.NodeBalancerPrefix, len(options.Options.NodeBalancerPrefix))
		klog.Error(msg)
		return fmt.Errorf("%s", msg)
	}

	validPrefix := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validPrefix.MatchString(options.Options.NodeBalancerPrefix) {
		msg := fmt.Sprintf("nodebalancer-prefix must be no empty and use only letters, numbers, underscores, and dashes: %s\n", options.Options.NodeBalancerPrefix)
		klog.Error(msg)
		return fmt.Errorf("%s", msg)
	}

	return nil
}

func newCloud() (cloudprovider.Interface, error) {
	region := os.Getenv(regionEnv)
	if region == "" {
		return nil, fmt.Errorf("%s must be set in the environment (use a k8s secret)", regionEnv)
	}

	tokenProvider, tokenSourceDescription, err := tokenProviderFromFileOrEnv()
	if err != nil {
		return nil, err
	}

	linodeClient, err := newLinodeClientWithPrometheus(requestTimeoutFromEnv(), tokenProvider)
	if err != nil {
		return nil, err
	}

	healthChecker, err := setupTokenHealthChecker(linodeClient, tokenSourceDescription)
	if err != nil {
		return nil, err
	}

	if vpcErr := services.ValidateAndSetVPCSubnetFlags(linodeClient); vpcErr != nil {
		return nil, fmt.Errorf("failed to validate VPC and subnet flags: %w", vpcErr)
	}

	if subnetErr := setupNodeBalancerBackendSubnet(linodeClient); subnetErr != nil {
		return nil, subnetErr
	}

	instanceCache = services.NewInstances(linodeClient)
	routes, err := newRoutes(linodeClient, instanceCache)
	if err != nil {
		return nil, fmt.Errorf("routes client was not created successfully: %w", err)
	}

	applyDeprecatedOptionWarnings()

	if err := validateNodeBalancerOptions(); err != nil {
		return nil, err
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

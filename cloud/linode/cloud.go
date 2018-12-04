package linode

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"

	"github.com/linode/linodego"
	"golang.org/x/oauth2"
	"k8s.io/kubernetes/pkg/cloudprovider"
	"k8s.io/kubernetes/pkg/controller"
)

const (
	ProviderName   = "linode"
	accessTokenEnv = "LINODE_API_TOKEN"
	nbPrefixEnv    = "LINODE_NB_PREFIX"
	regionEnv      = "LINODE_REGION"
)

type linodeCloud struct {
	client        *linodego.Client
	nbPrefix      string
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

	nbPrefix := os.Getenv(nbPrefixEnv)
	matched, _ := regexp.MatchString(`^[0-9A-Za-z_-]{,9}$`, nbPrefixEnv)
	if !matched {
		return nil, fmt.Errorf("%s must be up to 9 alphanumeric characters, including hyphen and underscore", nbPrefixEnv)
	}

	// Initialize Linode API Client
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: apiToken,
	})

	oauth2Client := &http.Client{
		Transport: &oauth2.Transport{
			Source: tokenSource,
		},
	}

	linodeClient := linodego.NewClient(oauth2Client)
	linodeClient.SetDebug(true)

	// Return struct that satisfies cloudprovider.Interface
	return &linodeCloud{
		client:        &linodeClient,
		nbPrefix:      nbPrefix,
		instances:     newInstances(&linodeClient),
		zones:         newZones(&linodeClient, region),
		loadbalancers: newLoadbalancers(&linodeClient, region, nbPrefix),
	}, nil
}

func (c *linodeCloud) Initialize(clientBuilder controller.ControllerClientBuilder) {
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

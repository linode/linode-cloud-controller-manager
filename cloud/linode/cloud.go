package linode

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/ghodss/yaml"
	"github.com/linode/linodego"
	"golang.org/x/oauth2"
	"k8s.io/kubernetes/pkg/cloudprovider"
	"k8s.io/kubernetes/pkg/controller"
)

const (
	ProviderName = "linode"
)

type Credential struct {
	Token string `json:"token" yaml:"token"`
	Zone  string `json:"zone" yaml:"zone"`
}
type Cloud struct {
	client        *linodego.Client
	instances     cloudprovider.Instances
	zones         cloudprovider.Zones
	loadbalancers cloudprovider.LoadBalancer
}

func init() {
	cloudprovider.RegisterCloudProvider(
		ProviderName,
		func(config io.Reader) (cloudprovider.Interface, error) {
			return newCloud(config)
		})
}

func newCloud(config io.Reader) (*Cloud, error) {
	cred := &Credential{}
	contents, err := ioutil.ReadAll(config)
	if err != nil {
		return nil, err
	}
	fmt.Println(string(contents))

	err = yaml.Unmarshal(contents, cred)
	if err != nil {
		return nil, err
	}

	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: cred.Token,
	})

	oauth2Client := &http.Client{
		Transport: &oauth2.Transport{
			Source: tokenSource,
		},
	}

	linodeClient := linodego.NewClient(oauth2Client)
	linodeClient.SetDebug(true)

	return &Cloud{
		client:        &linodeClient,
		instances:     newInstances(&linodeClient),
		zones:         newZones(&linodeClient, cred.Zone),
		loadbalancers: newLoadbalancers(&linodeClient, cred.Zone),
	}, nil
}

func (c *Cloud) Initialize(clientBuilder controller.ControllerClientBuilder) {
}

func (c *Cloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	return c.loadbalancers, true
}

func (c *Cloud) Instances() (cloudprovider.Instances, bool) {
	return c.instances, true
}

func (c *Cloud) Zones() (cloudprovider.Zones, bool) {
	return c.zones, true
}

func (c *Cloud) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

func (c *Cloud) Routes() (cloudprovider.Routes, bool) {
	return nil, false
}

func (c *Cloud) ProviderName() string {
	return ProviderName
}

func (c *Cloud) ScrubDNS(nameservers, searches []string) (nsOut, srchOut []string) {
	return nil, nil
}

func (c *Cloud) HasClusterID() bool {
	return true
}

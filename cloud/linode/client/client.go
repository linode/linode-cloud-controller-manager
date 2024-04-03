package client

//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination mock_client_test.go -package client github.com/linode/linode-cloud-controller-manager/cloud/linode/client Client

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/linode/linodego"
)

type Client interface {
	GetInstance(context.Context, int) (*linodego.Instance, error)
	ListInstances(context.Context, *linodego.ListOptions) ([]linodego.Instance, error)
	GetInstanceIPAddresses(context.Context, int) (*linodego.InstanceIPAddressResponse, error)

	CreateNodeBalancer(context.Context, linodego.NodeBalancerCreateOptions) (*linodego.NodeBalancer, error)
	GetNodeBalancer(context.Context, int) (*linodego.NodeBalancer, error)
	UpdateNodeBalancer(context.Context, int, linodego.NodeBalancerUpdateOptions) (*linodego.NodeBalancer, error)
	DeleteNodeBalancer(context.Context, int) error
	ListNodeBalancers(context.Context, *linodego.ListOptions) ([]linodego.NodeBalancer, error)
	ListNodeBalancerNodes(context.Context, int, int, *linodego.ListOptions) ([]linodego.NodeBalancerNode, error)

	CreateNodeBalancerConfig(context.Context, int, linodego.NodeBalancerConfigCreateOptions) (*linodego.NodeBalancerConfig, error)
	DeleteNodeBalancerConfig(context.Context, int, int) error
	ListNodeBalancerConfigs(context.Context, int, *linodego.ListOptions) ([]linodego.NodeBalancerConfig, error)
	RebuildNodeBalancerConfig(context.Context, int, int, linodego.NodeBalancerConfigRebuildOptions) (*linodego.NodeBalancerConfig, error)
	ListNodeBalancerFirewalls(ctx context.Context, nodebalancerID int, opts *linodego.ListOptions) ([]linodego.Firewall, error)
	ListFirewallDevices(ctx context.Context, firewallID int, opts *linodego.ListOptions) ([]linodego.FirewallDevice, error)
	DeleteFirewallDevice(ctx context.Context, firewallID, deviceID int) error
	CreateFirewallDevice(ctx context.Context, firewallID int, opts linodego.FirewallDeviceCreateOptions) (*linodego.FirewallDevice, error)
	CreateFirewall(ctx context.Context, opts linodego.FirewallCreateOptions) (*linodego.Firewall, error)
	DeleteFirewall(ctx context.Context, fwid int) error
	GetFirewall(context.Context, int) (*linodego.Firewall, error)
	UpdateFirewallRules(context.Context, int, linodego.FirewallRuleSet) (*linodego.FirewallRuleSet, error)
}

// linodego.Client implements Client
var _ Client = (*linodego.Client)(nil)

// New creates a new linode client with a given token, userAgent, and API URL
func New(token, userAgent, apiURL string) (*linodego.Client, error) {
	linodeClient := linodego.NewClient(nil)
	linodeClient.SetUserAgent(userAgent)
	linodeClient.SetToken(token)

	// Validate apiURL
	parsedURL, err := url.Parse(apiURL)
	if err != nil {
		return nil, err
	}

	validatedURL := &url.URL{
		Host:   parsedURL.Host,
		Scheme: parsedURL.Scheme,
	}

	linodeClient.SetBaseURL(validatedURL.String())

	version := ""
	matches := regexp.MustCompile(`/v\d+`).FindAllString(parsedURL.Path, -1)

	if len(matches) > 0 {
		version = strings.Trim(matches[len(matches)-1], "/")
	}

	linodeClient.SetAPIVersion(version)

	return &linodeClient, nil
}

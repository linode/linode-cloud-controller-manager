package client

//go:generate go run github.com/golang/mock/mockgen -destination mocks/mock_client.go -package mocks github.com/linode/linode-cloud-controller-manager/cloud/linode/client Client

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/linode/linodego"
	"golang.org/x/oauth2"
	"k8s.io/klog/v2"
)

const (
	// DefaultClientTimeout is the default timeout for a client Linode API call
	DefaultClientTimeout = 120 * time.Second
)

type Client interface {
	GetInstance(context.Context, int) (*linodego.Instance, error)
	ListInstances(context.Context, *linodego.ListOptions) ([]linodego.Instance, error)
	CreateInstance(ctx context.Context, opts linodego.InstanceCreateOptions) (*linodego.Instance, error)

	GetInstanceIPAddresses(context.Context, int) (*linodego.InstanceIPAddressResponse, error)
	AddInstanceIPAddress(ctx context.Context, linodeID int, public bool) (*linodego.InstanceIP, error)
	DeleteInstanceIPAddress(ctx context.Context, linodeID int, ipAddress string) error
	ShareIPAddresses(ctx context.Context, opts linodego.IPAddressesShareOptions) error

	UpdateInstanceConfigInterface(context.Context, int, int, int, linodego.InstanceConfigInterfaceUpdateOptions) (*linodego.InstanceConfigInterface, error)

	ListVPCs(context.Context, *linodego.ListOptions) ([]linodego.VPC, error)
	ListVPCIPAddresses(context.Context, int, *linodego.ListOptions) ([]linodego.VPCIP, error)

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

// New creates a new linode client with a given token and default timeout
func New(token string, timeout time.Duration) (*linodego.Client, error) {
	userAgent := fmt.Sprintf("linode-cloud-controller-manager %s", linodego.DefaultUserAgent)
	apiURL := os.Getenv("LINODE_URL")

	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	oauth2Client := &http.Client{
		Transport: &oauth2.Transport{
			Source: tokenSource,
		},
		Timeout: timeout,
	}
	linodeClient := linodego.NewClient(oauth2Client)
	client, err := linodeClient.UseURL(apiURL)
	if err != nil {
		return nil, err
	}
	client.SetUserAgent(userAgent)

	klog.V(3).Infof("Linode client created with default timeout of %v", timeout)
	return client, nil
}

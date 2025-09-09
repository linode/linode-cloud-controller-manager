package client

//go:generate go run github.com/golang/mock/mockgen -destination mocks/mock_client.go -package mocks github.com/linode/linode-cloud-controller-manager/cloud/linode/client Client
//go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -p github.com/linode/linode-cloud-controller-manager/cloud/linode/client -i Client -t ../../../hack/templates/prometheus.go.gotpl -o client_with_metrics.go -l ""

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/linode/linodego"
	"k8s.io/klog/v2"

	_ "github.com/hexdigest/gowrap"
)

const (
	// DefaultClientTimeout is the default timeout for a client Linode API call
	DefaultClientTimeout = 120 * time.Second
	DefaultLinodeAPIURL  = "https://api.linode.com"
)

type Client interface {
	GetInstance(context.Context, int) (*linodego.Instance, error)
	ListInstances(context.Context, *linodego.ListOptions) ([]linodego.Instance, error)
	CreateInstance(ctx context.Context, opts linodego.InstanceCreateOptions) (*linodego.Instance, error)
	ListInstanceConfigs(ctx context.Context, linodeID int, opts *linodego.ListOptions) ([]linodego.InstanceConfig, error)

	GetInstanceIPAddresses(context.Context, int) (*linodego.InstanceIPAddressResponse, error)
	AddInstanceIPAddress(ctx context.Context, linodeID int, public bool) (*linodego.InstanceIP, error)
	DeleteInstanceIPAddress(ctx context.Context, linodeID int, ipAddress string) error
	ShareIPAddresses(ctx context.Context, opts linodego.IPAddressesShareOptions) error

	UpdateInstanceConfigInterface(context.Context, int, int, int, linodego.InstanceConfigInterfaceUpdateOptions) (*linodego.InstanceConfigInterface, error)

	ListInterfaces(ctx context.Context, linodeID int, opts *linodego.ListOptions) ([]linodego.LinodeInterface, error)
	UpdateInterface(ctx context.Context, linodeID int, interfaceID int, opts linodego.LinodeInterfaceUpdateOptions) (*linodego.LinodeInterface, error)

	GetVPC(context.Context, int) (*linodego.VPC, error)
	GetVPCSubnet(context.Context, int, int) (*linodego.VPCSubnet, error)
	ListVPCs(context.Context, *linodego.ListOptions) ([]linodego.VPC, error)
	ListVPCIPAddresses(context.Context, int, *linodego.ListOptions) ([]linodego.VPCIP, error)
	ListVPCIPv6Addresses(context.Context, int, *linodego.ListOptions) ([]linodego.VPCIP, error)
	ListVPCSubnets(context.Context, int, *linodego.ListOptions) ([]linodego.VPCSubnet, error)

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

	ReserveIPAddress(ctx context.Context, opts linodego.ReserveIPOptions) (*linodego.InstanceIP, error)
	DeleteReservedIPAddress(ctx context.Context, ipAddress string) error

	GetProfile(ctx context.Context) (*linodego.Profile, error)
}

// linodego.Client implements Client
var _ Client = (*linodego.Client)(nil)

// New creates a new linode client with a given token and default timeout
func New(token string, timeout time.Duration) (*linodego.Client, error) {
	userAgent := fmt.Sprintf("linode-cloud-controller-manager %s", linodego.DefaultUserAgent)
	apiURL := os.Getenv("LINODE_URL")
	if apiURL == "" {
		apiURL = DefaultLinodeAPIURL
	}

	linodeClient := linodego.NewClient(&http.Client{Timeout: timeout})
	client, err := linodeClient.UseURL(apiURL)
	if err != nil {
		return nil, err
	}
	client.SetUserAgent(userAgent)
	client.SetToken(token)

	klog.V(3).Infof("Linode client created with default timeout of %v", timeout)
	return client, nil
}

func CheckClientAuthenticated(ctx context.Context, client Client) (bool, error) {
	_, err := client.GetProfile(ctx)
	if err == nil {
		return true, nil
	}

	var linodeErr *linodego.Error
	if !errors.As(err, &linodeErr) {
		return false, err
	}

	if linodego.ErrHasStatus(err, http.StatusUnauthorized) {
		return false, nil
	}

	return false, err
}

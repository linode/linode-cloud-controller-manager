// Code generated by gowrap. DO NOT EDIT.
// template: ../../../hack/templates/prometheus.go.gotpl
// gowrap: http://github.com/hexdigest/gowrap

package client

import (
	"context"

	_ "github.com/hexdigest/gowrap"
	"github.com/linode/linodego"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ClientWithPrometheus implements Client interface with all methods wrapped
// with Prometheus counters
type ClientWithPrometheus struct {
	base Client
}

var ClientMethodCounterVec = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "ccm_linode_client_requests_total",
		Help: "client counters for each operation and its result",
	},
	[]string{"method", "result"})

// NewClientWithPrometheus returns an instance of the Client decorated with prometheus metrics
func NewClientWithPrometheus(base Client) ClientWithPrometheus {
	return ClientWithPrometheus{
		base: base,
	}
}

// AddInstanceIPAddress implements Client
func (_d ClientWithPrometheus) AddInstanceIPAddress(ctx context.Context, linodeID int, public bool) (ip1 *linodego.InstanceIP, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("AddInstanceIPAddress", result).Inc()
	}()
	return _d.base.AddInstanceIPAddress(ctx, linodeID, public)
}

// CreateFirewall implements Client
func (_d ClientWithPrometheus) CreateFirewall(ctx context.Context, opts linodego.FirewallCreateOptions) (fp1 *linodego.Firewall, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("CreateFirewall", result).Inc()
	}()
	return _d.base.CreateFirewall(ctx, opts)
}

// CreateFirewallDevice implements Client
func (_d ClientWithPrometheus) CreateFirewallDevice(ctx context.Context, firewallID int, opts linodego.FirewallDeviceCreateOptions) (fp1 *linodego.FirewallDevice, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("CreateFirewallDevice", result).Inc()
	}()
	return _d.base.CreateFirewallDevice(ctx, firewallID, opts)
}

// CreateInstance implements Client
func (_d ClientWithPrometheus) CreateInstance(ctx context.Context, opts linodego.InstanceCreateOptions) (ip1 *linodego.Instance, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("CreateInstance", result).Inc()
	}()
	return _d.base.CreateInstance(ctx, opts)
}

// CreateNodeBalancer implements Client
func (_d ClientWithPrometheus) CreateNodeBalancer(ctx context.Context, n1 linodego.NodeBalancerCreateOptions) (np1 *linodego.NodeBalancer, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("CreateNodeBalancer", result).Inc()
	}()
	return _d.base.CreateNodeBalancer(ctx, n1)
}

// CreateNodeBalancerConfig implements Client
func (_d ClientWithPrometheus) CreateNodeBalancerConfig(ctx context.Context, i1 int, n1 linodego.NodeBalancerConfigCreateOptions) (np1 *linodego.NodeBalancerConfig, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("CreateNodeBalancerConfig", result).Inc()
	}()
	return _d.base.CreateNodeBalancerConfig(ctx, i1, n1)
}

// DeleteFirewall implements Client
func (_d ClientWithPrometheus) DeleteFirewall(ctx context.Context, fwid int) (err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("DeleteFirewall", result).Inc()
	}()
	return _d.base.DeleteFirewall(ctx, fwid)
}

// DeleteFirewallDevice implements Client
func (_d ClientWithPrometheus) DeleteFirewallDevice(ctx context.Context, firewallID int, deviceID int) (err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("DeleteFirewallDevice", result).Inc()
	}()
	return _d.base.DeleteFirewallDevice(ctx, firewallID, deviceID)
}

// DeleteInstanceIPAddress implements Client
func (_d ClientWithPrometheus) DeleteInstanceIPAddress(ctx context.Context, linodeID int, ipAddress string) (err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("DeleteInstanceIPAddress", result).Inc()
	}()
	return _d.base.DeleteInstanceIPAddress(ctx, linodeID, ipAddress)
}

// DeleteNodeBalancer implements Client
func (_d ClientWithPrometheus) DeleteNodeBalancer(ctx context.Context, i1 int) (err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("DeleteNodeBalancer", result).Inc()
	}()
	return _d.base.DeleteNodeBalancer(ctx, i1)
}

// DeleteNodeBalancerConfig implements Client
func (_d ClientWithPrometheus) DeleteNodeBalancerConfig(ctx context.Context, i1 int, i2 int) (err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("DeleteNodeBalancerConfig", result).Inc()
	}()
	return _d.base.DeleteNodeBalancerConfig(ctx, i1, i2)
}

// GetFirewall implements Client
func (_d ClientWithPrometheus) GetFirewall(ctx context.Context, i1 int) (fp1 *linodego.Firewall, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("GetFirewall", result).Inc()
	}()
	return _d.base.GetFirewall(ctx, i1)
}

// GetInstance implements Client
func (_d ClientWithPrometheus) GetInstance(ctx context.Context, i1 int) (ip1 *linodego.Instance, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("GetInstance", result).Inc()
	}()
	return _d.base.GetInstance(ctx, i1)
}

// GetInstanceIPAddresses implements Client
func (_d ClientWithPrometheus) GetInstanceIPAddresses(ctx context.Context, i1 int) (ip1 *linodego.InstanceIPAddressResponse, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("GetInstanceIPAddresses", result).Inc()
	}()
	return _d.base.GetInstanceIPAddresses(ctx, i1)
}

// GetNodeBalancer implements Client
func (_d ClientWithPrometheus) GetNodeBalancer(ctx context.Context, i1 int) (np1 *linodego.NodeBalancer, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("GetNodeBalancer", result).Inc()
	}()
	return _d.base.GetNodeBalancer(ctx, i1)
}

// GetProfile implements Client
func (_d ClientWithPrometheus) GetProfile(ctx context.Context) (pp1 *linodego.Profile, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("GetProfile", result).Inc()
	}()
	return _d.base.GetProfile(ctx)
}

// ListFirewallDevices implements Client
func (_d ClientWithPrometheus) ListFirewallDevices(ctx context.Context, firewallID int, opts *linodego.ListOptions) (fa1 []linodego.FirewallDevice, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("ListFirewallDevices", result).Inc()
	}()
	return _d.base.ListFirewallDevices(ctx, firewallID, opts)
}

// ListIPv6Ranges implements Client
func (_d ClientWithPrometheus) ListIPv6Ranges(ctx context.Context, opts *linodego.ListOptions) (ia1 []linodego.IPv6Range, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("ListIPv6Ranges", result).Inc()
	}()
	return _d.base.ListIPv6Ranges(ctx, opts)
}

// ListInstances implements Client
func (_d ClientWithPrometheus) ListInstances(ctx context.Context, lp1 *linodego.ListOptions) (ia1 []linodego.Instance, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("ListInstances", result).Inc()
	}()
	return _d.base.ListInstances(ctx, lp1)
}

// ListNodeBalancerConfigs implements Client
func (_d ClientWithPrometheus) ListNodeBalancerConfigs(ctx context.Context, i1 int, lp1 *linodego.ListOptions) (na1 []linodego.NodeBalancerConfig, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("ListNodeBalancerConfigs", result).Inc()
	}()
	return _d.base.ListNodeBalancerConfigs(ctx, i1, lp1)
}

// ListNodeBalancerFirewalls implements Client
func (_d ClientWithPrometheus) ListNodeBalancerFirewalls(ctx context.Context, nodebalancerID int, opts *linodego.ListOptions) (fa1 []linodego.Firewall, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("ListNodeBalancerFirewalls", result).Inc()
	}()
	return _d.base.ListNodeBalancerFirewalls(ctx, nodebalancerID, opts)
}

// ListNodeBalancerNodes implements Client
func (_d ClientWithPrometheus) ListNodeBalancerNodes(ctx context.Context, i1 int, i2 int, lp1 *linodego.ListOptions) (na1 []linodego.NodeBalancerNode, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("ListNodeBalancerNodes", result).Inc()
	}()
	return _d.base.ListNodeBalancerNodes(ctx, i1, i2, lp1)
}

// ListNodeBalancers implements Client
func (_d ClientWithPrometheus) ListNodeBalancers(ctx context.Context, lp1 *linodego.ListOptions) (na1 []linodego.NodeBalancer, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("ListNodeBalancers", result).Inc()
	}()
	return _d.base.ListNodeBalancers(ctx, lp1)
}

// ListVPCIPAddresses implements Client
func (_d ClientWithPrometheus) ListVPCIPAddresses(ctx context.Context, i1 int, lp1 *linodego.ListOptions) (va1 []linodego.VPCIP, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("ListVPCIPAddresses", result).Inc()
	}()
	return _d.base.ListVPCIPAddresses(ctx, i1, lp1)
}

// ListVPCSubnets implements Client
func (_d ClientWithPrometheus) ListVPCSubnets(ctx context.Context, i1 int, lp1 *linodego.ListOptions) (va1 []linodego.VPCSubnet, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("ListVPCSubnets", result).Inc()
	}()
	return _d.base.ListVPCSubnets(ctx, i1, lp1)
}

// ListVPCs implements Client
func (_d ClientWithPrometheus) ListVPCs(ctx context.Context, lp1 *linodego.ListOptions) (va1 []linodego.VPC, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("ListVPCs", result).Inc()
	}()
	return _d.base.ListVPCs(ctx, lp1)
}

// RebuildNodeBalancerConfig implements Client
func (_d ClientWithPrometheus) RebuildNodeBalancerConfig(ctx context.Context, i1 int, i2 int, n1 linodego.NodeBalancerConfigRebuildOptions) (np1 *linodego.NodeBalancerConfig, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("RebuildNodeBalancerConfig", result).Inc()
	}()
	return _d.base.RebuildNodeBalancerConfig(ctx, i1, i2, n1)
}

// ShareIPAddresses implements Client
func (_d ClientWithPrometheus) ShareIPAddresses(ctx context.Context, opts linodego.IPAddressesShareOptions) (err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("ShareIPAddresses", result).Inc()
	}()
	return _d.base.ShareIPAddresses(ctx, opts)
}

// UpdateFirewallRules implements Client
func (_d ClientWithPrometheus) UpdateFirewallRules(ctx context.Context, i1 int, f1 linodego.FirewallRuleSet) (fp1 *linodego.FirewallRuleSet, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("UpdateFirewallRules", result).Inc()
	}()
	return _d.base.UpdateFirewallRules(ctx, i1, f1)
}

// UpdateInstanceConfigInterface implements Client
func (_d ClientWithPrometheus) UpdateInstanceConfigInterface(ctx context.Context, i1 int, i2 int, i3 int, i4 linodego.InstanceConfigInterfaceUpdateOptions) (ip1 *linodego.InstanceConfigInterface, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("UpdateInstanceConfigInterface", result).Inc()
	}()
	return _d.base.UpdateInstanceConfigInterface(ctx, i1, i2, i3, i4)
}

// UpdateNodeBalancer implements Client
func (_d ClientWithPrometheus) UpdateNodeBalancer(ctx context.Context, i1 int, n1 linodego.NodeBalancerUpdateOptions) (np1 *linodego.NodeBalancer, err error) {
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ClientMethodCounterVec.WithLabelValues("UpdateNodeBalancer", result).Inc()
	}()
	return _d.base.UpdateNodeBalancer(ctx, i1, n1)
}

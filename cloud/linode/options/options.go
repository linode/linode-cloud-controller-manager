package options

import (
	"net"

	"github.com/spf13/pflag"
)

// Options is a configuration object for this cloudprovider implementation.
// We expect it to be initialized with flags external to this package, likely in
// main.go
var Options struct {
	KubeconfigFlag                    *pflag.Flag
	LinodeGoDebug                     bool
	EnableRouteController             bool
	EnableTokenHealthChecker          bool
	VPCNames                          []string
	VPCIDs                            []int
	SubnetNames                       []string
	SubnetIDs                         []int
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
	DisableIPv6NodeCIDRAllocation     bool
	ClusterCIDRIPv4                   string
	NodeCIDRMaskSizeIPv4              int
	NodeCIDRMaskSizeIPv6              int
	NodeBalancerPrefix                string
}

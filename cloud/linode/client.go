package linode

//go:generate go run github.com/golang/mock/mockgen -destination mock_client_test.go -package linode github.com/linode/linode-cloud-controller-manager/cloud/linode LinodeClient

import (
	"context"

	"github.com/linode/linodego"
)

type LinodeClient interface {
	GetInstance(context.Context, int) (*linodego.Instance, error)
	ListInstances(context.Context, *linodego.ListOptions) ([]linodego.Instance, error)
	GetInstanceIPAddresses(context.Context, int) (*linodego.InstanceIPAddressResponse, error)

	CreateNodeBalancer(context.Context, linodego.NodeBalancerCreateOptions) (*linodego.NodeBalancer, error)
	GetNodeBalancer(context.Context, int) (*linodego.NodeBalancer, error)
	UpdateNodeBalancer(context.Context, int, linodego.NodeBalancerUpdateOptions) (*linodego.NodeBalancer, error)
	DeleteNodeBalancer(context.Context, int) error
	ListNodeBalancers(context.Context, *linodego.ListOptions) ([]linodego.NodeBalancer, error)

	CreateNodeBalancerConfig(context.Context, int, linodego.NodeBalancerConfigCreateOptions) (*linodego.NodeBalancerConfig, error)
	DeleteNodeBalancerConfig(context.Context, int, int) error
	ListNodeBalancerConfigs(context.Context, int, *linodego.ListOptions) ([]linodego.NodeBalancerConfig, error)
	RebuildNodeBalancerConfig(context.Context, int, int, linodego.NodeBalancerConfigRebuildOptions) (*linodego.NodeBalancerConfig, error)
}

// linodego.Client implements LinodeClient
var _ LinodeClient = (*linodego.Client)(nil)

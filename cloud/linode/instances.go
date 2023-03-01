package linode

import (
	"context"
	"fmt"
	"net/http"

	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
)

type instances struct {
	client Client
}

func newInstances(client Client) cloudprovider.InstancesV2 {
	return &instances{client}
}

type instanceNoIPAddressesError struct {
	id int
}

func (e instanceNoIPAddressesError) Error() string {
	return fmt.Sprintf("instance %d has no IP addresses", e.id)
}

func (i *instances) nodeAddresses(ctx context.Context, linode *linodego.Instance) ([]v1.NodeAddress, error) {
	var addresses []v1.NodeAddress
	addresses = append(addresses, v1.NodeAddress{Type: v1.NodeHostName, Address: linode.Label})

	ips, err := i.client.GetInstanceIPAddresses(ctx, linode.ID)
	if err != nil {
		return nil, err
	}

	if (len(ips.IPv4.Public) == 0) && (len(ips.IPv4.Private) == 0) {
		return nil, instanceNoIPAddressesError{linode.ID}
	}

	if len(ips.IPv4.Public) > 0 {
		addresses = append(addresses, v1.NodeAddress{Type: v1.NodeExternalIP, Address: ips.IPv4.Public[0].Address})
	}
	// Allow Nodes to not have an ExternalIP, if this proves problematic this will be reverted

	if len(ips.IPv4.Private) > 0 {
		addresses = append(addresses, v1.NodeAddress{Type: v1.NodeInternalIP, Address: ips.IPv4.Private[0].Address})
	}
	// Allow Nodes to not have an InternalIP, if this proves problematic this will be reverted

	return addresses, nil
}

func (i *instances) lookupLinode(ctx context.Context, node *v1.Node) (*linodego.Instance, error) {
	providerID := node.Spec.ProviderID
	nodeName := types.NodeName(node.Name)

	if providerID != "" {
		id, err := parseProviderID(providerID)
		if err != nil {
			return nil, err
		}

		return linodeByID(ctx, i.client, id)
	}

	return linodeByName(ctx, i.client, nodeName)
}

func (i *instances) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	if _, err := i.lookupLinode(ctx, node); err != nil {
		if apiError, ok := err.(*linodego.Error); ok && apiError.Code == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (i *instances) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	instance, err := i.lookupLinode(ctx, node)
	if err != nil {
		return false, err
	}

	// An instance is considered to be "shutdown" when it is
	// in the process of shutting down, or already offline.
	if instance.Status == linodego.InstanceOffline ||
		instance.Status == linodego.InstanceShuttingDown {
		return true, nil
	}

	return false, nil
}

func (i *instances) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	linode, err := i.lookupLinode(ctx, node)
	if err != nil {
		return nil, err
	}

	addresses, err := i.nodeAddresses(ctx, linode)
	if err != nil {
		return nil, err
	}

	// note that Zone is omitted as it's not a thing in Linode
	meta := &cloudprovider.InstanceMetadata{
		ProviderID:    node.Spec.ProviderID, // TODO(okokes): this is circular... should we instead set it to a known prefix + linode.ID?
		NodeAddresses: addresses,
		InstanceType:  linode.Type,
		Region:        linode.Region,
	}

	return meta, nil
}

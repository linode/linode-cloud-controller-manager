package linode

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/linode/linode-cloud-controller-manager/sentry"
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

func (i *instances) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	providerID := node.Spec.ProviderID

	ctx = sentry.SetHubOnContext(ctx)
	sentry.SetTag(ctx, "provider_id", providerID)

	id, err := parseProviderID(providerID)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return false, err
	}

	sentry.SetTag(ctx, "linode_id", strconv.Itoa(id))

	_, err = linodeByID(ctx, i.client, id)
	if err != nil {
		if apiError, ok := err.(*linodego.Error); ok && apiError.Code == http.StatusNotFound {
			return false, nil
		}
		sentry.CaptureError(ctx, err)
		return false, err
	}

	return true, nil
}

func (i *instances) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	providerID := node.Spec.ProviderID

	ctx = sentry.SetHubOnContext(ctx)
	sentry.SetTag(ctx, "provider_id", providerID)

	id, err := parseProviderID(providerID)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return false, err
	}

	sentry.SetTag(ctx, "linode_id", strconv.Itoa(id))

	instance, err := linodeByID(ctx, i.client, id)
	if err != nil {
		sentry.CaptureError(ctx, err)
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
	var (
		providerID = node.Spec.ProviderID
		nodeName   = types.NodeName(node.Name)
		linode     *linodego.Instance
		err        error
	)

	// TODO(okokes): abstract this away and reuse it in our other methods here
	if providerID != "" {
		id, err := parseProviderID(providerID)
		if err != nil {
			return nil, err
		}

		linode, err = linodeByID(ctx, i.client, id)
		if err != nil {
			return nil, err
		}
	} else {
		linode, err = linodeByName(ctx, i.client, nodeName)
		if err != nil {
			return nil, err
		}
	}

	addresses, err := i.nodeAddresses(ctx, linode)
	if err != nil {
		return nil, err
	}

	// note that Zone is omitted as it's not a thing in Linode
	meta := &cloudprovider.InstanceMetadata{
		ProviderID:    providerID,
		NodeAddresses: addresses,
		InstanceType:  linode.Type,
		Region:        linode.Region,
	}

	return meta, nil
}

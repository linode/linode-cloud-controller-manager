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

func (i *instances) lookupLinode(ctx context.Context, node *v1.Node) (*linodego.Instance, error) {
	providerID := node.Spec.ProviderID
	nodeName := types.NodeName(node.Name)

	sentry.SetTag(ctx, "provider_id", providerID)
	sentry.SetTag(ctx, "node_name", node.Name)

	if providerID != "" {
		id, err := parseProviderID(providerID)
		if err != nil {
			sentry.CaptureError(ctx, err)
			return nil, err
		}
		sentry.SetTag(ctx, "linode_id", strconv.Itoa(id))

		return linodeByID(ctx, i.client, id)
	}

	return linodeByName(ctx, i.client, nodeName)
}

func (i *instances) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	ctx = sentry.SetHubOnContext(ctx)
	if _, err := i.lookupLinode(ctx, node); err != nil {
		if apiError, ok := err.(*linodego.Error); ok && apiError.Code == http.StatusNotFound {
			return false, nil
		}
		sentry.CaptureError(ctx, err)
		return false, err
	}

	return true, nil
}

func (i *instances) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	ctx = sentry.SetHubOnContext(ctx)
	instance, err := i.lookupLinode(ctx, node)
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
	ctx = sentry.SetHubOnContext(ctx)
	linode, err := i.lookupLinode(ctx, node)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return nil, err
	}

	if len(linode.IPv4) == 0 {
		err := instanceNoIPAddressesError{linode.ID}
		sentry.CaptureError(ctx, err)
		return nil, err
	}

	addresses := []v1.NodeAddress{{Type: v1.NodeHostName, Address: linode.Label}}

	for _, ip := range linode.IPv4 {
		ipType := v1.NodeExternalIP
		if ip.IsPrivate() {
			ipType = v1.NodeInternalIP
		}
		addresses = append(addresses, v1.NodeAddress{Type: ipType, Address: ip.String()})
	}

	// note that Zone is omitted as it's not a thing in Linode
	meta := &cloudprovider.InstanceMetadata{
		ProviderID:    fmt.Sprintf("%v%v", providerIDPrefix, linode.ID),
		NodeAddresses: addresses,
		InstanceType:  linode.Type,
		Region:        linode.Region,
	}

	return meta, nil
}

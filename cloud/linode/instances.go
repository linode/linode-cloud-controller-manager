package linode

import (
	"context"
	"fmt"
	"strconv"

	"github.com/linode/linode-cloud-controller-manager/sentry"
	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
)

type instances struct {
	client LinodeClient
}

func newInstances(client LinodeClient) cloudprovider.Instances {
	return &instances{client}
}

type instanceNoIPAddressesError struct {
	id int
}

func (e instanceNoIPAddressesError) Error() string {
	return fmt.Sprintf("instance %d has no IP addresses", e.id)
}

func (i *instances) NodeAddresses(ctx context.Context, name types.NodeName) ([]v1.NodeAddress, error) {
	ctx = sentry.SetHubOnContext(ctx)
	sentry.SetTag(ctx, "node_name", string(name))

	linode, err := linodeByName(ctx, i.client, name)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return nil, err
	}

	addresses, err := i.nodeAddresses(ctx, linode)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return nil, err
	}

	return addresses, nil
}

func (i *instances) NodeAddressesByProviderID(ctx context.Context, providerID string) ([]v1.NodeAddress, error) {
	ctx = sentry.SetHubOnContext(ctx)
	sentry.SetTag(ctx, "provider_id", providerID)

	id, err := parseProviderID(providerID)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return nil, err
	}

	linode, err := linodeByID(ctx, i.client, id)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return nil, err
	}

	addresses, err := i.nodeAddresses(ctx, linode)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return nil, err
	}

	return addresses, nil
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

func (i *instances) InstanceID(ctx context.Context, nodeName types.NodeName) (string, error) {
	ctx = sentry.SetHubOnContext(ctx)
	sentry.SetTag(ctx, "node_name", string(nodeName))

	linode, err := linodeByName(ctx, i.client, nodeName)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return "", err
	}
	return strconv.Itoa(linode.ID), nil
}

func (i *instances) InstanceType(ctx context.Context, nodeName types.NodeName) (string, error) {
	ctx = sentry.SetHubOnContext(ctx)
	sentry.SetTag(ctx, "node_name", string(nodeName))

	linode, err := linodeByName(ctx, i.client, nodeName)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return "", err
	}
	return linode.Type, nil
}

func (i *instances) InstanceTypeByProviderID(ctx context.Context, providerID string) (string, error) {
	ctx = sentry.SetHubOnContext(ctx)
	sentry.SetTag(ctx, "provider_id", providerID)

	id, err := parseProviderID(providerID)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return "", err
	}

	sentry.SetTag(ctx, "linode_id", strconv.Itoa(id))

	linode, err := linodeByID(ctx, i.client, id)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return "", err
	}
	return linode.Type, nil
}

func (i *instances) AddSSHKeyToAllInstances(_ context.Context, user string, keyData []byte) error {
	return cloudprovider.NotImplemented
}

func (i *instances) CurrentNodeName(_ context.Context, hostname string) (types.NodeName, error) {
	return types.NodeName(hostname), nil
}

func (i *instances) InstanceExistsByProviderID(ctx context.Context, providerID string) (bool, error) {
	ctx = sentry.SetHubOnContext(ctx)
	sentry.SetTag(ctx, "provider_id", providerID)

	id, err := parseProviderID(providerID)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return false, err
	}

	sentry.SetTag(ctx, "linode_id", strconv.Itoa(id))

	_, err = linodeByID(ctx, i.client, id)
	if err == nil {
		return true, nil
	}

	sentry.CaptureError(ctx, err)

	return false, nil
}

func (i *instances) InstanceShutdownByProviderID(ctx context.Context, providerID string) (bool, error) {
	return false, cloudprovider.NotImplemented
}

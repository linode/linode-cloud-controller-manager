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

// TODO(PR): providing both APIs to keep the tests green and to enable
// gradual migration
type hybridInstances interface {
	cloudprovider.Instances
	cloudprovider.InstancesV2
}

func newInstances(client Client) hybridInstances {
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
	if err != nil {
		if apiError, ok := err.(*linodego.Error); ok && apiError.Code == http.StatusNotFound {
			return false, nil
		}
		sentry.CaptureError(ctx, err)
		return false, err
	}

	return true, nil
}

func (i *instances) InstanceShutdownByProviderID(ctx context.Context, providerID string) (bool, error) {
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

// TODO(PR): move code from instancesv1 over here
func (i *instances) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	return false, nil // TODO(PR): fix
}

func (i *instances) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	return false, nil // TODO(PR): fix
}

func (i *instances) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	return nil, nil
}

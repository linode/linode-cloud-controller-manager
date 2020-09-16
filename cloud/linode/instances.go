package linode

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/linode/linode-cloud-controller-manager/cloud"
	"github.com/linode/linode-cloud-controller-manager/sentry"
	"github.com/linode/linodego"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubernetes/pkg/cloudprovider"
)

type instances struct {
	client *linodego.Client
}

func newInstances(client *linodego.Client) cloudprovider.Instances {
	return &instances{client}
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

	id, err := linodeIDFromProviderID(providerID)
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
		return nil, fmt.Errorf("instance has no IP addresses")
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

	id, err := linodeIDFromProviderID(providerID)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return "", err
	}

	sentry.SetTag(ctx, "linode_id", id)

	linode, err := linodeByID(ctx, i.client, id)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return "", err
	}
	return linode.Type, nil
}

func (i *instances) AddSSHKeyToAllInstances(_ context.Context, user string, keyData []byte) error {
	return cloud.ErrNotImplemented
}

func (i *instances) CurrentNodeName(_ context.Context, hostname string) (types.NodeName, error) {
	return types.NodeName(hostname), nil
}

func (i *instances) InstanceExistsByProviderID(ctx context.Context, providerID string) (bool, error) {
	ctx = sentry.SetHubOnContext(ctx)
	sentry.SetTag(ctx, "provider_id", providerID)

	id, err := linodeIDFromProviderID(providerID)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return false, err
	}

	sentry.SetTag(ctx, "linode_id", id)

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

func linodeByID(ctx context.Context, client *linodego.Client, id string) (*linodego.Instance, error) {
	linodeID, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}

	instance, err := client.GetInstance(ctx, linodeID)
	if err != nil {
		return nil, err
	}
	if instance == nil {
		return nil, fmt.Errorf("linode not found with id %v", linodeID)
	}
	return instance, nil
}

func linodeByName(ctx context.Context, client *linodego.Client, nodeName types.NodeName) (*linodego.Instance, error) {
	jsonFilter, err := json.Marshal(map[string]string{"label": string(nodeName)})
	if err != nil {
		return nil, err
	}

	linodes, err := client.ListInstances(ctx, linodego.NewListOptions(0, string(jsonFilter)))
	if err != nil {
		return nil, err
	}

	if len(linodes) == 0 {
		return nil, cloudprovider.InstanceNotFound
	} else if len(linodes) > 1 {
		return nil, errors.New(fmt.Sprintf("Multiple instances found with name %v", nodeName))
	}

	return &linodes[0], nil
}

// serverIDFromProviderID returns a Linode ID from a providerID.
//
// The providerID can be seen on the Kubernetes Node object. The expected
// format is: linode://linodeID
func linodeIDFromProviderID(providerID string) (string, error) {
	if providerID == "" {
		return "", errors.New("providerID cannot be empty string")
	}

	split := strings.Split(providerID, "://")
	if len(split) != 2 {
		return "", fmt.Errorf("unexpected providerID format: %s, format should be: linode://12345", providerID)
	}

	if split[0] != ProviderName {
		return "", fmt.Errorf("provider scheme from providerID should be 'linode://', %s", providerID)
	}

	return split[1], nil
}

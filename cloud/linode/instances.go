package linode

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/linode/linode-cloud-controller-manager/cloud"
	"github.com/linode/linodego"
	"github.com/pkg/errors"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubernetes/pkg/cloudprovider"
)

type instances struct {
	client *linodego.Client
}

func newInstances(client *linodego.Client) cloudprovider.Instances {
	return &instances{client}
}

func (i *instances) NodeAddresses(_ context.Context, name types.NodeName) ([]v1.NodeAddress, error) {
	linode, err := linodeByName(i.client, name)
	if err != nil {
		return nil, err
	}
	return i.nodeAddresses(linode)
}

func (i *instances) NodeAddressesByProviderID(_ context.Context, providerID string) ([]v1.NodeAddress, error) {
	id, err := serverIDFromProviderID(providerID)
	if err != nil {
		return nil, err
	}

	linode, err := linodeByID(i.client, id)
	if err != nil {
		return nil, err
	}

	return i.nodeAddresses(linode)
}

func (i *instances) nodeAddresses(linode *linodego.Instance) ([]v1.NodeAddress, error) {
	var addresses []v1.NodeAddress
	addresses = append(addresses, v1.NodeAddress{Type: v1.NodeHostName, Address: linode.Label})

	ips, err := i.client.GetInstanceIPAddresses(context.TODO(), linode.ID)
	if err != nil {
		return nil, err
	}

	if len(ips.IPv4.Public) > 0 {
		addresses = append(addresses, v1.NodeAddress{Type: v1.NodeExternalIP, Address: ips.IPv4.Public[0].Address})
	} else {
		return nil, fmt.Errorf("could not get public ip")
	}

	if len(ips.IPv4.Private) > 0 {
		addresses = append(addresses, v1.NodeAddress{Type: v1.NodeInternalIP, Address: ips.IPv4.Private[0].Address})
	} else {
		return nil, fmt.Errorf("could not get private ip")
	}

	return addresses, nil
}

func (i *instances) InstanceID(_ context.Context, nodeName types.NodeName) (string, error) {
	linode, err := linodeByName(i.client, nodeName)
	if err != nil {
		return "", err
	}
	return strconv.Itoa(linode.ID), nil
}

func (i *instances) InstanceType(_ context.Context, nodeName types.NodeName) (string, error) {
	linode, err := linodeByName(i.client, nodeName)
	if err != nil {
		return "", err
	}
	return linode.Type, nil
}

func (i *instances) InstanceTypeByProviderID(_ context.Context, providerID string) (string, error) {
	id, err := serverIDFromProviderID(providerID)
	if err != nil {
		return "", err
	}
	linode, err := linodeByID(i.client, id)
	if err != nil {
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

func (i *instances) InstanceExistsByProviderID(_ context.Context, providerID string) (bool, error) {
	id, err := serverIDFromProviderID(providerID)
	if err != nil {
		return false, err
	}
	_, err = linodeByID(i.client, id)
	if err == nil {
		return true, nil
	}

	return false, nil
}

func (i *instances) InstanceShutdownByProviderID(ctx context.Context, providerID string) (bool, error) {
	return false, cloudprovider.NotImplemented
}

func linodeByID(client *linodego.Client, id string) (*linodego.Instance, error) {
	linodeID, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}

	instance, err := client.GetInstance(context.TODO(), linodeID)
	if err != nil {
		return nil, err
	}
	if instance == nil {
		return nil, fmt.Errorf("linode not found with id %v", linodeID)
	}
	return instance, nil

}
func linodeByName(client *linodego.Client, nodeName types.NodeName) (*linodego.Instance, error) {
	jsonFilter, err := json.Marshal(map[string]string{"label": string(nodeName)})
	if err != nil {
		return nil, err
	}

	linodes, err := client.ListInstances(context.TODO(), linodego.NewListOptions(0, string(jsonFilter)))
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

// serverIDFromProviderID returns a server's ID from providerID.
//
// The providerID spec should be retrievable from the Kubernetes
// node object. The expected format is: linode://server-id

func serverIDFromProviderID(providerID string) (string, error) {
	if providerID == "" {
		return "", errors.New("providerID cannot be empty string")
	}

	split := strings.Split(providerID, "/")
	if len(split) != 3 {
		return "", fmt.Errorf("unexpected providerID format: %s, format should be: linode://12345", providerID)
	}

	// since split[0] is actually "linode:"
	if strings.TrimSuffix(split[0], ":") != ProviderName {
		return "", fmt.Errorf("provider name from providerID should be linode: %s", providerID)
	}

	return split[2], nil
}

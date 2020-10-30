package linode

import (
	"context"

	"github.com/linode/linodego"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
)

type zones struct {
	client *linodego.Client
	region string
}

func newZones(client *linodego.Client, zone string) cloudprovider.Zones {
	return zones{client, zone}
}

func (z zones) GetZone(_ context.Context) (cloudprovider.Zone, error) {
	return cloudprovider.Zone{Region: z.region}, nil
}

func (z zones) GetZoneByProviderID(ctx context.Context, providerID string) (cloudprovider.Zone, error) {
	id, err := linodeIDFromProviderID(providerID)
	if err != nil {
		return cloudprovider.Zone{}, err
	}
	linode, err := linodeByID(ctx, z.client, id)
	if err != nil {
		return cloudprovider.Zone{}, err
	}

	return cloudprovider.Zone{Region: linode.Region}, nil
}

func (z zones) GetZoneByNodeName(ctx context.Context, nodeName types.NodeName) (cloudprovider.Zone, error) {
	linode, err := linodeByName(ctx, z.client, nodeName)
	if err != nil {
		return cloudprovider.Zone{}, err
	}
	return cloudprovider.Zone{Region: linode.Region}, nil
}

package linode

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/linode/linodego"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
)

func TestGetZone(t *testing.T) {
	region := "us-east"
	zones := newZones(nil, region)
	zone, err := zones.GetZone(context.TODO())

	assert.NoError(t, err)
	assert.Equal(t, zone, cloudprovider.Zone{Region: region})
}

func TestGetZoneByProviderID(t *testing.T) {
	ctx := context.TODO()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockLinodeClient(ctrl)
	zones := newZones(client, "us-east")

	t.Run("fail when providerID is malformed", func(t *testing.T) {
		providerID := "bogus://123"
		zone, err := zones.GetZoneByProviderID(ctx, providerID)

		assert.Error(t, err, invalidProviderIDError{providerID}.Error())
		assert.Equal(t, cloudprovider.Zone{}, zone)
	})

	t.Run("fail on api error", func(t *testing.T) {
		id := 29182
		providerID := providerIDPrefix + strconv.Itoa(id)
		getErr := &linodego.Error{Code: http.StatusServiceUnavailable}
		client.EXPECT().GetInstance(gomock.Any(), id).Times(1).Return(nil, getErr)
		zone, err := zones.GetZoneByProviderID(ctx, providerID)

		assert.ErrorIs(t, err, getErr)
		assert.Equal(t, cloudprovider.Zone{}, zone)
	})

	t.Run("get region when linode exists", func(t *testing.T) {
		id := 29818
		region := "eu-west"
		providerID := providerIDPrefix + strconv.Itoa(id)
		client.EXPECT().GetInstance(gomock.Any(), id).Times(1).Return(&linodego.Instance{
			ID: id, Region: region,
		}, nil)
		zone, err := zones.GetZoneByProviderID(ctx, providerID)

		assert.NoError(t, err)
		assert.Equal(t, cloudprovider.Zone{Region: region}, zone)
	})
}

func TestGetZoneByNodeName(t *testing.T) {
	ctx := context.TODO()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockLinodeClient(ctrl)
	zones := newZones(client, "us-east")

	t.Run("fail on api error", func(t *testing.T) {
		name := "a-very-nice-linode"
		listErr := &linodego.Error{Code: http.StatusInternalServerError}
		client.EXPECT().ListInstances(gomock.Any(), linodeFilterListOptions(name)).Times(1).Return(nil, listErr)
		zone, err := zones.GetZoneByNodeName(ctx, types.NodeName(name))

		assert.ErrorIs(t, err, listErr)
		assert.Equal(t, cloudprovider.Zone{}, zone)
	})

	t.Run("get region when linode exists", func(t *testing.T) {
		name := "some-linode"
		region := "eu-west"
		client.EXPECT().ListInstances(gomock.Any(), linodeFilterListOptions(name)).Times(1).Return([]linodego.Instance{
			{ID: 291828, Label: name, Region: region},
		}, nil)
		zone, err := zones.GetZoneByNodeName(ctx, types.NodeName(name))

		assert.NoError(t, err)
		assert.Equal(t, cloudprovider.Zone{Region: region}, zone)
	})
}

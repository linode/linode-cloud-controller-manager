package linode

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/linode/linodego"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cloudprovider "k8s.io/cloud-provider"
)

func nodeWithProviderID(providerID string) *v1.Node {
	return &v1.Node{Spec: v1.NodeSpec{
		ProviderID: providerID,
	}}
}

func nodeWithName(name string) *v1.Node {
	return &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func TestInstanceExists(t *testing.T) {
	ctx := context.TODO()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockClient(ctrl)
	instances := newInstances(client)

	t.Run("should propagate generic api error", func(t *testing.T) {
		node := nodeWithProviderID(providerIDPrefix + "123")
		expectedErr := errors.New("some error")
		client.EXPECT().GetInstance(gomock.Any(), 123).Times(1).Return(nil, expectedErr)

		exists, err := instances.InstanceExists(ctx, node)
		assert.ErrorIs(t, err, expectedErr)
		assert.False(t, exists)
	})

	t.Run("should return false if linode does not exist (by providerID)", func(t *testing.T) {
		node := nodeWithProviderID(providerIDPrefix + "123")
		client.EXPECT().GetInstance(gomock.Any(), 123).Times(1).Return(nil, &linodego.Error{
			Code: http.StatusNotFound,
		})

		exists, err := instances.InstanceExists(ctx, node)
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("should return false if linode does not exist (by name)", func(t *testing.T) {
		name := "some-name"
		node := nodeWithName(name)
		notFound := &linodego.Error{
			Code: http.StatusNotFound,
		}
		filter := linodeFilterListOptions(name)
		client.EXPECT().ListInstances(gomock.Any(), filter).Times(1).Return(nil, notFound)

		exists, err := instances.InstanceExists(ctx, node)
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("should return true if linode exists (by provider)", func(t *testing.T) {
		node := nodeWithProviderID(providerIDPrefix + "123")
		client.EXPECT().GetInstance(gomock.Any(), 123).Times(1).Return(&linodego.Instance{
			ID:     123,
			Label:  "mock",
			Region: "us-east",
			Type:   "g6-standard-2",
		}, nil)

		exists, err := instances.InstanceExists(ctx, node)
		assert.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("should return true if linode exists (by name)", func(t *testing.T) {
		name := "some-name"
		node := nodeWithName(name)

		client.EXPECT().ListInstances(gomock.Any(), linodeFilterListOptions(name)).Times(1).Return([]linodego.Instance{
			{ID: 123, Label: name},
		}, nil)

		exists, err := instances.InstanceExists(ctx, node)
		assert.NoError(t, err)
		assert.True(t, exists)
	})
}

func TestNodeAddresses(t *testing.T) {
	ctx := context.TODO()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockClient(ctrl)
	instances := newInstances(client)

	t.Run("errors when linode does not exist", func(t *testing.T) {
		name := "does-not-exist"
		node := nodeWithName(name)
		filter := linodeFilterListOptions(name)
		client.EXPECT().ListInstances(gomock.Any(), filter).Times(1).Return([]linodego.Instance{}, nil)

		meta, err := instances.InstanceMetadata(ctx, node)
		assert.ErrorIs(t, err, cloudprovider.InstanceNotFound)
		assert.Nil(t, meta)
	})

	t.Run("errors when linode does not have any ips", func(t *testing.T) {
		id := 29392
		name := "an-instance"
		node := nodeWithName(name)
		filter := linodeFilterListOptions(name)
		client.EXPECT().ListInstances(gomock.Any(), filter).Times(1).Return([]linodego.Instance{
			{ID: id, Label: name},
		}, nil)
		client.EXPECT().GetInstanceIPAddresses(gomock.Any(), id).Times(1).Return(&linodego.InstanceIPAddressResponse{
			IPv4: &linodego.InstanceIPv4Response{
				Public:  []*linodego.InstanceIP{},
				Private: []*linodego.InstanceIP{},
			},
		}, nil)

		meta, err := instances.InstanceMetadata(ctx, node)
		assert.Error(t, err, instanceNoIPAddressesError{id})
		assert.Nil(t, meta)
	})

	t.Run("should return addresses when linode is found", func(t *testing.T) {
		id := 123
		name := "mock-instance"
		node := nodeWithName(name)
		publicIPv4 := "45.76.101.25"
		privateIPv4 := "192.168.133.65"
		filter := linodeFilterListOptions(name)
		client.EXPECT().ListInstances(gomock.Any(), filter).Times(1).Return([]linodego.Instance{
			{ID: id, Label: name},
		}, nil)
		client.EXPECT().GetInstanceIPAddresses(gomock.Any(), id).Times(1).Return(&linodego.InstanceIPAddressResponse{
			IPv4: &linodego.InstanceIPv4Response{
				Public:  []*linodego.InstanceIP{{Address: publicIPv4}},
				Private: []*linodego.InstanceIP{{Address: privateIPv4}},
			},
		}, nil)

		meta, err := instances.InstanceMetadata(ctx, node)
		assert.NoError(t, err)
		assert.Equal(t, meta.NodeAddresses, []v1.NodeAddress{
			{
				Type:    v1.NodeHostName,
				Address: name,
			},
			{
				Type:    v1.NodeExternalIP,
				Address: publicIPv4,
			},
			{
				Type:    v1.NodeInternalIP,
				Address: privateIPv4,
			},
		})
	})
}

func TestNodeAddressesByProviderID(t *testing.T) {
	ctx := context.TODO()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockClient(ctrl)
	instances := newInstances(client)

	t.Run("fails on malformed providerID", func(t *testing.T) {
		providerID := "bogus://bogus"
		node := nodeWithProviderID(providerID)
		meta, err := instances.InstanceMetadata(ctx, node)
		assert.ErrorIs(t, err, invalidProviderIDError{providerID})
		assert.Nil(t, meta)
	})

	t.Run("fails on non-numeric providerID", func(t *testing.T) {
		providerID := providerIDPrefix + "abc"
		node := nodeWithProviderID(providerID)
		meta, err := instances.InstanceMetadata(ctx, node)

		assert.ErrorIs(t, err, invalidProviderIDError{providerID})
		assert.Nil(t, meta)
	})

	t.Run("fails when linode does not exist", func(t *testing.T) {
		id := 456302
		providerID := providerIDPrefix + strconv.Itoa(id)
		node := nodeWithProviderID(providerID)
		getInstanceErr := &linodego.Error{Code: http.StatusNotFound}
		client.EXPECT().GetInstance(gomock.Any(), id).Times(1).Return(nil, getInstanceErr)
		meta, err := instances.InstanceMetadata(ctx, node)

		assert.ErrorIs(t, err, getInstanceErr)
		assert.Nil(t, meta)
	})

	t.Run("should return addresses when linode is found", func(t *testing.T) {
		id := 192910
		name := "my-instance"
		providerID := providerIDPrefix + strconv.Itoa(id)
		node := nodeWithProviderID(providerID)
		publicIPv4 := "32.74.121.25"
		privateIPv4 := "192.168.121.42"
		client.EXPECT().GetInstance(gomock.Any(), id).Times(1).Return(&linodego.Instance{
			ID: id, Label: name,
		}, nil)
		client.EXPECT().GetInstanceIPAddresses(gomock.Any(), id).Times(1).Return(&linodego.InstanceIPAddressResponse{
			IPv4: &linodego.InstanceIPv4Response{
				Public:  []*linodego.InstanceIP{{Address: publicIPv4}},
				Private: []*linodego.InstanceIP{{Address: privateIPv4}},
			},
		}, nil)

		meta, err := instances.InstanceMetadata(ctx, node)

		assert.NoError(t, err)
		assert.Equal(t, meta.NodeAddresses, []v1.NodeAddress{
			{
				Type:    v1.NodeHostName,
				Address: name,
			},
			{
				Type:    v1.NodeExternalIP,
				Address: publicIPv4,
			},
			{
				Type:    v1.NodeInternalIP,
				Address: privateIPv4,
			},
		})
	})
}

func TestInstanceType(t *testing.T) {
	ctx := context.TODO()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockClient(ctrl)
	instances := newInstances(client)

	t.Run("fails when instance not found", func(t *testing.T) {
		name := "bogus"
		node := nodeWithName(name)
		filter := linodeFilterListOptions(name)
		client.EXPECT().ListInstances(gomock.Any(), filter).Times(1).Return([]linodego.Instance{}, nil)
		meta, err := instances.InstanceMetadata(ctx, node)

		assert.ErrorIs(t, err, cloudprovider.InstanceNotFound)
		assert.Nil(t, meta)
	})

	t.Run("succeeds when instance exists", func(t *testing.T) {
		id := 39399
		name := "my-nanode"
		node := nodeWithName(name)
		linodeType := "g6-standard-1"
		filter := linodeFilterListOptions(name)
		client.EXPECT().ListInstances(gomock.Any(), filter).Times(1).Return([]linodego.Instance{
			{ID: id, Label: name, Type: linodeType},
		}, nil)
		client.EXPECT().GetInstanceIPAddresses(gomock.Any(), id).Times(1).Return(&linodego.InstanceIPAddressResponse{
			IPv4: &linodego.InstanceIPv4Response{Public: []*linodego.InstanceIP{{Address: "1.2.3.4"}}},
			IPv6: &linodego.InstanceIPv6Response{},
		}, nil)
		meta, err := instances.InstanceMetadata(ctx, node)

		assert.NoError(t, err)
		assert.Equal(t, linodeType, meta.InstanceType)
	})
}

func TestInstanceTypeByProviderID(t *testing.T) {
	ctx := context.TODO()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockClient(ctrl)
	instances := newInstances(client)

	t.Run("fails when instance not found", func(t *testing.T) {
		id := 39281
		providerID := providerIDPrefix + strconv.Itoa(id)
		node := nodeWithProviderID(providerID)
		client.EXPECT().GetInstance(gomock.Any(), id).Times(1).Return(nil, linodego.Error{Code: http.StatusNotFound})
		meta, err := instances.InstanceMetadata(ctx, node)

		assert.Error(t, err)
		assert.Nil(t, meta)
	})

	t.Run("succeeds when instance exists", func(t *testing.T) {
		id := 39281
		providerID := providerIDPrefix + strconv.Itoa(id)
		node := nodeWithProviderID(providerID)
		linodeType := "g6-standard-2"
		client.EXPECT().GetInstance(gomock.Any(), id).Times(1).Return(&linodego.Instance{
			ID: id, Label: "some-linode", Type: linodeType,
		}, nil)
		client.EXPECT().GetInstanceIPAddresses(gomock.Any(), id).Times(1).Return(&linodego.InstanceIPAddressResponse{
			IPv4: &linodego.InstanceIPv4Response{Public: []*linodego.InstanceIP{{Address: "1.2.3.4"}}},
			IPv6: &linodego.InstanceIPv6Response{},
		}, nil)
		meta, err := instances.InstanceMetadata(ctx, node)

		assert.NoError(t, err)
		assert.Equal(t, linodeType, meta.InstanceType)
	})
}

func TestInstanceShutdownByProviderID(t *testing.T) {
	ctx := context.TODO()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockClient(ctrl)
	instances := newInstances(client)

	t.Run("fails when instance not found", func(t *testing.T) {
		id := 12345
		node := nodeWithProviderID(providerIDPrefix + strconv.Itoa(id))
		client.EXPECT().GetInstance(gomock.Any(), id).Times(1).Return(nil, linodego.Error{Code: http.StatusNotFound})
		shutdown, err := instances.InstanceShutdown(ctx, node)

		assert.Error(t, err)
		assert.False(t, shutdown)
	})

	t.Run("returns true when instance is shut down", func(t *testing.T) {
		id := 12345
		node := nodeWithProviderID(providerIDPrefix + strconv.Itoa(id))
		client.EXPECT().GetInstance(gomock.Any(), id).Times(1).Return(&linodego.Instance{
			ID: id, Label: "offline-linode", Status: linodego.InstanceOffline,
		}, nil)
		shutdown, err := instances.InstanceShutdown(ctx, node)

		assert.NoError(t, err)
		assert.True(t, shutdown)
	})

	t.Run("returns true when instance is shutting down", func(t *testing.T) {
		id := 12345
		node := nodeWithProviderID(providerIDPrefix + strconv.Itoa(id))
		client.EXPECT().GetInstance(gomock.Any(), id).Times(1).Return(&linodego.Instance{
			ID: id, Label: "shutting-down-linode", Status: linodego.InstanceShuttingDown,
		}, nil)
		shutdown, err := instances.InstanceShutdown(ctx, node)

		assert.NoError(t, err)
		assert.True(t, shutdown)
	})

	t.Run("returns false when instance is running", func(t *testing.T) {
		id := 12345
		node := nodeWithProviderID(providerIDPrefix + strconv.Itoa(id))
		client.EXPECT().GetInstance(gomock.Any(), id).Times(1).Return(&linodego.Instance{
			ID: id, Label: "running-linode", Status: linodego.InstanceRunning,
		}, nil)
		shutdown, err := instances.InstanceShutdown(ctx, node)

		assert.NoError(t, err)
		assert.False(t, shutdown)
	})
}

// TODO: consider folding all of these tests into the InstanceMetadata tests above
// they have the same setup, we're just testing different properties
func TestMetadataRegion(t *testing.T) {
	ctx := context.TODO()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockClient(ctrl)
	instances := newInstances(client)

	t.Run("Test region retrieval", func(t *testing.T) {
		id := 123
		region := "us-east"
		node := nodeWithProviderID(providerIDPrefix + strconv.Itoa(id))
		client.EXPECT().GetInstance(gomock.Any(), 123).Times(1).Return(&linodego.Instance{
			ID:     123,
			Label:  "mock",
			Region: region,
			Type:   "g6-standard-2",
		}, nil)
		client.EXPECT().GetInstanceIPAddresses(gomock.Any(), id).Times(1).Return(&linodego.InstanceIPAddressResponse{
			IPv4: &linodego.InstanceIPv4Response{Public: []*linodego.InstanceIP{{Address: "1.2.3.4"}}},
			IPv6: &linodego.InstanceIPv6Response{},
		}, nil)
		meta, err := instances.InstanceMetadata(ctx, node)

		assert.NoError(t, err)
		assert.Equal(t, region, meta.Region)
		assert.Empty(t, meta.Zone)
	})
}

func TestGetZoneByProviderID(t *testing.T) {
	ctx := context.TODO()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockClient(ctrl)
	instances := newInstances(client)

	t.Run("fail when providerID is malformed", func(t *testing.T) {
		providerID := "bogus://123"
		node := nodeWithProviderID(providerID)
		meta, err := instances.InstanceMetadata(ctx, node)

		assert.Error(t, err, invalidProviderIDError{providerID}.Error())
		assert.Nil(t, meta)
	})

	t.Run("fail on api error", func(t *testing.T) {
		id := 29182
		providerID := providerIDPrefix + strconv.Itoa(id)
		node := nodeWithProviderID(providerID)
		getErr := &linodego.Error{Code: http.StatusServiceUnavailable}
		client.EXPECT().GetInstance(gomock.Any(), id).Times(1).Return(nil, getErr)
		meta, err := instances.InstanceMetadata(ctx, node)

		assert.ErrorIs(t, err, getErr)
		assert.Nil(t, meta)
	})

	t.Run("get region when linode exists", func(t *testing.T) {
		id := 29818
		region := "eu-west"
		providerID := providerIDPrefix + strconv.Itoa(id)
		node := nodeWithProviderID(providerID)
		client.EXPECT().GetInstance(gomock.Any(), id).Times(1).Return(&linodego.Instance{
			ID: id, Region: region,
		}, nil)
		client.EXPECT().GetInstanceIPAddresses(gomock.Any(), id).Times(1).Return(&linodego.InstanceIPAddressResponse{
			IPv4: &linodego.InstanceIPv4Response{Public: []*linodego.InstanceIP{{Address: "1.2.3.4"}}},
			IPv6: &linodego.InstanceIPv6Response{},
		}, nil)
		meta, err := instances.InstanceMetadata(ctx, node)

		assert.NoError(t, err)
		assert.Equal(t, meta.Region, region)
	})
}

func TestGetZoneByNodeName(t *testing.T) {
	ctx := context.TODO()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockClient(ctrl)
	instances := newInstances(client)

	t.Run("fail on api error", func(t *testing.T) {
		name := "a-very-nice-linode"
		node := nodeWithName(name)
		listErr := &linodego.Error{Code: http.StatusInternalServerError}
		client.EXPECT().ListInstances(gomock.Any(), linodeFilterListOptions(name)).Times(1).Return(nil, listErr)

		meta, err := instances.InstanceMetadata(ctx, node)
		assert.ErrorIs(t, err, listErr)
		assert.Nil(t, meta)
	})

	t.Run("get region when linode exists", func(t *testing.T) {
		name := "some-linode"
		id := 291828
		node := nodeWithName(name)
		region := "eu-west"
		client.EXPECT().ListInstances(gomock.Any(), linodeFilterListOptions(name)).Times(1).Return([]linodego.Instance{
			{ID: id, Label: name, Region: region},
		}, nil)
		client.EXPECT().GetInstanceIPAddresses(gomock.Any(), id).Times(1).Return(&linodego.InstanceIPAddressResponse{
			IPv4: &linodego.InstanceIPv4Response{Public: []*linodego.InstanceIP{{Address: "1.2.3.4"}}},
			IPv6: &linodego.InstanceIPv6Response{},
		}, nil)
		meta, err := instances.InstanceMetadata(ctx, node)

		assert.NoError(t, err)
		assert.Equal(t, region, meta.Region)
	})
}

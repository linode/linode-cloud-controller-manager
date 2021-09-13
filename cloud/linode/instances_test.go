package linode

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/linode/linodego"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
)

func TestInstanceExistsByProviderID(t *testing.T) {
	ctx := context.TODO()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockLinodeClient(ctrl)
	instances := newInstances(client)

	t.Run("should propagate generic api error", func(t *testing.T) {
		providerID := providerIDPrefix + "123"
		expectedErr := errors.New("some error")
		client.EXPECT().GetInstance(gomock.Any(), 123).Times(1).Return(nil, expectedErr)

		exists, err := instances.InstanceExistsByProviderID(ctx, providerID)
		assert.ErrorIs(t, err, expectedErr)
		assert.False(t, exists)
	})

	t.Run("should return false if linode does not exist", func(t *testing.T) {
		providerID := providerIDPrefix + "123"
		client.EXPECT().GetInstance(gomock.Any(), 123).Times(1).Return(nil, &linodego.Error{
			Code: http.StatusNotFound,
		})

		exists, err := instances.InstanceExistsByProviderID(ctx, providerID)
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("should return true if linode exists", func(t *testing.T) {
		providerID := providerIDPrefix + "123"
		client.EXPECT().GetInstance(gomock.Any(), 123).Times(1).Return(&linodego.Instance{
			ID:     123,
			Label:  "mock",
			Region: "us-east",
			Type:   "g6-standard-2",
		}, nil)

		exists, err := instances.InstanceExistsByProviderID(ctx, providerID)
		assert.NoError(t, err)
		assert.True(t, exists)
	})
}

func TestNodeAddresses(t *testing.T) {
	ctx := context.TODO()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockLinodeClient(ctrl)
	instances := newInstances(client)

	t.Run("errors when linode does not exist", func(t *testing.T) {
		name := "does-not-exist"
		filter := linodeFilterListOptions(name)
		client.EXPECT().ListInstances(gomock.Any(), filter).Times(1).Return([]linodego.Instance{}, nil)

		addrs, err := instances.NodeAddresses(ctx, types.NodeName(name))
		assert.ErrorIs(t, err, cloudprovider.InstanceNotFound)
		assert.Nil(t, addrs)
	})

	t.Run("errors when linode does not have any ips", func(t *testing.T) {
		id := 29392
		name := "an-instance"
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

		addrs, err := instances.NodeAddresses(ctx, types.NodeName(name))
		assert.Error(t, err, instanceNoIPAddressesError{id})
		assert.Nil(t, addrs)
	})

	t.Run("should return addresses when linode is found", func(t *testing.T) {
		id := 123
		name := "mock-instance"
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

		addrs, err := instances.NodeAddresses(ctx, types.NodeName(name))
		assert.NoError(t, err)
		assert.Equal(t, addrs, []v1.NodeAddress{
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

	client := NewMockLinodeClient(ctrl)
	instances := newInstances(client)

	t.Run("fails on malformed providerID", func(t *testing.T) {
		providerID := "bogus://bogus"
		addrs, err := instances.NodeAddressesByProviderID(ctx, providerID)

		assert.ErrorIs(t, err, invalidProviderIDError{providerID})
		assert.Nil(t, addrs)
	})

	t.Run("fails on non-numeric providerID", func(t *testing.T) {
		providerID := providerIDPrefix + "abc"
		addrs, err := instances.NodeAddressesByProviderID(ctx, providerID)

		assert.ErrorIs(t, err, invalidProviderIDError{providerID})
		assert.Nil(t, addrs)
	})

	t.Run("fails when linode does not exist", func(t *testing.T) {
		id := 456302
		providerID := providerIDPrefix + strconv.Itoa(id)
		getInstanceErr := &linodego.Error{Code: http.StatusNotFound}
		client.EXPECT().GetInstance(gomock.Any(), id).Times(1).Return(nil, getInstanceErr)
		addrs, err := instances.NodeAddressesByProviderID(ctx, providerID)

		assert.ErrorIs(t, err, getInstanceErr)
		assert.Nil(t, addrs)
	})

	t.Run("should return addresses when linode is found", func(t *testing.T) {
		id := 192910
		name := "my-instance"
		providerID := providerIDPrefix + strconv.Itoa(id)
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

		addrs, err := instances.NodeAddressesByProviderID(ctx, providerID)

		assert.NoError(t, err)
		assert.Equal(t, addrs, []v1.NodeAddress{
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

func TestInstanceID(t *testing.T) {
	ctx := context.TODO()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockLinodeClient(ctrl)
	instances := newInstances(client)

	t.Run("fails when instance not found", func(t *testing.T) {
		name := "non-existant-instance"
		filter := linodeFilterListOptions(name)
		client.EXPECT().ListInstances(gomock.Any(), filter).Times(1).Return([]linodego.Instance{}, nil)
		instanceID, err := instances.InstanceID(ctx, types.NodeName(name))

		assert.ErrorIs(t, err, cloudprovider.InstanceNotFound)
		assert.Zero(t, instanceID)
	})

	t.Run("succeeds when instance exists", func(t *testing.T) {
		id := 129289
		name := "existing-instance"
		filter := linodeFilterListOptions(name)
		client.EXPECT().ListInstances(gomock.Any(), filter).Times(1).Return([]linodego.Instance{
			{ID: id, Label: name},
		}, nil)
		instanceID, err := instances.InstanceID(ctx, types.NodeName(name))

		assert.NoError(t, err)
		assert.Equal(t, strconv.Itoa(id), instanceID)
	})
}

func TestInstanceType(t *testing.T) {
	ctx := context.TODO()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockLinodeClient(ctrl)
	instances := newInstances(client)

	t.Run("fails when instance not found", func(t *testing.T) {
		name := "bogus"
		filter := linodeFilterListOptions(name)
		client.EXPECT().ListInstances(gomock.Any(), filter).Times(1).Return([]linodego.Instance{}, nil)
		instanceType, err := instances.InstanceType(ctx, types.NodeName(name))

		assert.ErrorIs(t, err, cloudprovider.InstanceNotFound)
		assert.Empty(t, instanceType)
	})

	t.Run("succeeds when instance exists", func(t *testing.T) {
		id := 39399
		name := "my-nanode"
		linodeType := "g6-standard-1"
		filter := linodeFilterListOptions(name)
		client.EXPECT().ListInstances(gomock.Any(), filter).Times(1).Return([]linodego.Instance{
			{ID: id, Label: name, Type: linodeType},
		}, nil)
		instanceType, err := instances.InstanceType(ctx, types.NodeName(name))

		assert.NoError(t, err)
		assert.Equal(t, linodeType, instanceType)
	})
}

func TestInstanceTypeByProviderID(t *testing.T) {
	ctx := context.TODO()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockLinodeClient(ctrl)
	instances := newInstances(client)

	t.Run("fails when instance not found", func(t *testing.T) {
		id := 39281
		providerID := providerIDPrefix + strconv.Itoa(id)
		client.EXPECT().GetInstance(gomock.Any(), id).Times(1).Return(nil, linodego.Error{Code: http.StatusNotFound})
		instanceType, err := instances.InstanceTypeByProviderID(ctx, providerID)

		assert.Error(t, err)
		assert.Empty(t, instanceType)
	})

	t.Run("succeeds when instance exists", func(t *testing.T) {
		id := 39281
		providerID := providerIDPrefix + strconv.Itoa(id)
		linodeType := "g6-standard-2"
		client.EXPECT().GetInstance(gomock.Any(), id).Times(1).Return(&linodego.Instance{
			ID: id, Label: "some-linode", Type: linodeType,
		}, nil)
		instanceType, err := instances.InstanceTypeByProviderID(ctx, providerID)

		assert.NoError(t, err)
		assert.Equal(t, linodeType, instanceType)
	})
}

func TestCurrentNodeName(t *testing.T) {
	instances := newInstances(nil)
	hostname := "lke2901-2920fa-3838f-28a21"
	nodeName, err := instances.CurrentNodeName(context.TODO(), hostname)
	assert.NoError(t, err)
	assert.Equal(t, types.NodeName(hostname), nodeName)
}

func TestAddSSHKeyToAllInstances(t *testing.T) {
	instances := newInstances(nil)
	err := instances.AddSSHKeyToAllInstances(context.TODO(), "root", []byte{})
	assert.ErrorIs(t, err, cloudprovider.NotImplemented)
}

func TestInstanceShutdownByProviderID(t *testing.T) {
	instances := newInstances(nil)
	_, err := instances.InstanceShutdownByProviderID(context.TODO(), providerIDPrefix+"12345")
	assert.ErrorIs(t, err, cloudprovider.NotImplemented)
}

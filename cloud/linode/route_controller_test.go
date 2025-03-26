package linode

import (
	"net"
	"strconv"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/linode/linodego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/utils/ptr"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client/mocks"
)

func TestListRoutes(t *testing.T) {
	Options.VPCNames = "test,abc"
	vpcIDs["test"] = 1
	vpcIDs["abc"] = 2
	Options.EnableRouteController = true

	nodeID := 123
	name := "mock-instance"
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: v1.NodeSpec{ProviderID: providerIDPrefix + strconv.Itoa(nodeID)},
	}
	publicIPv4 := net.ParseIP("45.76.101.25")
	privateIPv4 := net.ParseIP("192.168.133.65")
	linodeType := "g6-standard-1"
	region := "us-east"

	t.Run("should return empty if no instance exists in cluster", func(t *testing.T) {
		ctx := t.Context()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		instanceCache := newInstances(client)
		routeController, err := newRoutes(client, instanceCache)
		require.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.Instance{}, nil)
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return([]linodego.VPCIP{}, nil)
		routes, err := routeController.ListRoutes(ctx, "test")
		require.NoError(t, err)
		assert.Empty(t, routes)
	})

	validInstance := linodego.Instance{
		ID:     nodeID,
		Label:  name,
		Type:   linodeType,
		Region: region,
		IPv4:   []*net.IP{&publicIPv4, &privateIPv4},
	}

	t.Run("should return no routes if instance exists but is not connected to VPC", func(t *testing.T) {
		ctx := t.Context()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		instanceCache := newInstances(client)
		routeController, err := newRoutes(client, instanceCache)
		require.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return([]linodego.VPCIP{}, nil)
		routes, err := routeController.ListRoutes(ctx, "test")
		require.NoError(t, err)
		assert.Empty(t, routes)
	})

	vpcIP := "10.0.0.2"
	noRoutesInVPC := []linodego.VPCIP{
		{
			Address:      &vpcIP,
			AddressRange: nil,
			VPCID:        vpcIDs["test"],
			NAT1To1:      nil,
			LinodeID:     nodeID,
		},
	}

	t.Run("should return no routes if instance exists, connected to VPC but no ip_ranges configured", func(t *testing.T) {
		ctx := t.Context()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		instanceCache := newInstances(client)
		routeController, err := newRoutes(client, instanceCache)
		require.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(4).Return(noRoutesInVPC, nil)
		routes, err := routeController.ListRoutes(ctx, "test")
		require.NoError(t, err)
		assert.Empty(t, routes)
	})

	addressRange1 := "10.192.0.0/24"
	addressRange2 := "10.192.10.0/24"
	routesInVPC := []linodego.VPCIP{
		{
			Address:      &vpcIP,
			AddressRange: nil,
			VPCID:        vpcIDs["test"],
			NAT1To1:      nil,
			LinodeID:     nodeID,
		},
		{
			Address:      nil,
			AddressRange: &addressRange1,
			VPCID:        vpcIDs["test"],
			NAT1To1:      nil,
			LinodeID:     nodeID,
		},
		{
			Address:      nil,
			AddressRange: &addressRange2,
			VPCID:        vpcIDs["test"],
			NAT1To1:      nil,
			LinodeID:     nodeID,
		},
	}

	t.Run("should return routes if instance exists, connected to VPC and ip_ranges configured", func(t *testing.T) {
		ctx := t.Context()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		instanceCache := newInstances(client)
		existingK8sCache := registeredK8sNodeCache
		defer func() {
			registeredK8sNodeCache = existingK8sCache
		}()
		registeredK8sNodeCache = newK8sNodeCache()
		registeredK8sNodeCache.addNodeToCache(node)
		routeController, err := newRoutes(client, instanceCache)
		require.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(4).Return(routesInVPC, nil)
		routes, err := routeController.ListRoutes(ctx, "test")
		require.NoError(t, err)
		assert.NotEmpty(t, routes)
		assert.Equal(t, addressRange1, routes[0].DestinationCIDR)
		assert.Equal(t, addressRange2, routes[1].DestinationCIDR)
	})

	routesInDifferentVPC := []linodego.VPCIP{
		{
			Address:      &vpcIP,
			AddressRange: nil,
			VPCID:        100,
			NAT1To1:      nil,
			LinodeID:     nodeID,
		},
		{
			Address:      nil,
			AddressRange: &addressRange1,
			VPCID:        100,
			NAT1To1:      nil,
			LinodeID:     nodeID,
		},
		{
			Address:      nil,
			AddressRange: &addressRange2,
			VPCID:        100,
			NAT1To1:      nil,
			LinodeID:     nodeID,
		},
	}

	t.Run("should return no routes if instance exists, connected to VPC and ip_ranges configured but vpc id doesn't match", func(t *testing.T) {
		ctx := t.Context()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		instanceCache := newInstances(client)
		existingK8sCache := registeredK8sNodeCache
		defer func() {
			registeredK8sNodeCache = existingK8sCache
		}()
		registeredK8sNodeCache = newK8sNodeCache()
		registeredK8sNodeCache.addNodeToCache(node)
		routeController, err := newRoutes(client, instanceCache)
		require.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(4).Return(routesInDifferentVPC, nil)
		routes, err := routeController.ListRoutes(ctx, "test")
		require.NoError(t, err)
		assert.Empty(t, routes)
	})

	t.Run("should return routes if multiple instances exists, connected to VPCs and ip_ranges configured", func(t *testing.T) {
		ctx := t.Context()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		instanceCache := newInstances(client)
		existingK8sCache := registeredK8sNodeCache
		defer func() {
			registeredK8sNodeCache = existingK8sCache
		}()
		registeredK8sNodeCache = newK8sNodeCache()
		registeredK8sNodeCache.addNodeToCache(node)
		routeController, err := newRoutes(client, instanceCache)
		require.NoError(t, err)

		vpcIP2 := "10.0.0.3"
		addressRange3 := "10.192.40.0/24"
		addressRange4 := "10.192.50.0/24"
		addressRange5 := "10.192.60.0/24"

		instance2Label := "mock-instance2"
		instance3Label := "mock-instance3"
		instance2ID := 124
		instance3ID := 125
		validInstance2 := linodego.Instance{
			ID:     instance2ID,
			Label:  instance2Label,
			Type:   linodeType,
			Region: region,
			IPv4:   []*net.IP{&publicIPv4, &privateIPv4},
		}
		validInstance3 := linodego.Instance{
			ID:     instance3ID,
			Label:  instance3Label,
			Type:   linodeType,
			Region: region,
			IPv4:   []*net.IP{&publicIPv4, &privateIPv4},
		}

		routesInVPC2 := []linodego.VPCIP{
			{
				Address:      &vpcIP2,
				AddressRange: nil,
				VPCID:        vpcIDs["abc"],
				NAT1To1:      nil,
				LinodeID:     instance2ID,
			},
			{
				Address:      nil,
				AddressRange: &addressRange3,
				VPCID:        vpcIDs["abc"],
				NAT1To1:      nil,
				LinodeID:     instance2ID,
			},
			{
				Address:      nil,
				AddressRange: &addressRange4,
				VPCID:        vpcIDs["abc"],
				NAT1To1:      nil,
				LinodeID:     instance2ID,
			},
			{
				Address:      &vpcIP2,
				AddressRange: nil,
				VPCID:        vpcIDs["abc"],
				NAT1To1:      nil,
				LinodeID:     instance3ID,
			},
			{
				Address:      nil,
				AddressRange: &addressRange5,
				VPCID:        vpcIDs["abc"],
				NAT1To1:      nil,
				LinodeID:     instance3ID,
			},
		}

		node2 := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:        instance2Label,
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			},
			Spec: v1.NodeSpec{ProviderID: providerIDPrefix + strconv.Itoa(instance2ID)},
		}

		registeredK8sNodeCache.addNodeToCache(node2)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{validInstance, validInstance2, validInstance3}, nil)
		c1 := client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(routesInVPC, nil)
		c2 := client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).After(c1).Times(1).Return(routesInVPC2, nil)
		c3 := client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).After(c2).Times(1).Return(routesInVPC, nil)
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).After(c3).Times(1).Return(routesInVPC2, nil)
		routes, err := routeController.ListRoutes(ctx, "test")
		require.NoError(t, err)
		assert.NotEmpty(t, routes)
		cidrs := make([]string, len(routes))
		for i, value := range routes {
			cidrs[i] = value.DestinationCIDR
		}
		assert.Contains(t, cidrs, addressRange1)
		assert.Contains(t, cidrs, addressRange2)
		assert.Contains(t, cidrs, addressRange3)
		assert.Contains(t, cidrs, addressRange4)
		assert.NotContains(t, cidrs, addressRange5)
	})
}

func TestCreateRoute(t *testing.T) {
	ctx := t.Context()
	Options.VPCNames = "dummy"
	vpcIDs["dummy"] = 1
	Options.EnableRouteController = true

	nodeID := 123
	name := "mock-instance"
	publicIPv4 := net.ParseIP("45.76.101.25")
	privateIPv4 := net.ParseIP("192.168.133.65")
	linodeType := "g6-standard-1"
	region := "us-east"
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: v1.NodeSpec{ProviderID: providerIDPrefix + strconv.Itoa(nodeID)},
	}
	validInstance := linodego.Instance{
		ID:     nodeID,
		Label:  name,
		Type:   linodeType,
		Region: region,
		IPv4:   []*net.IP{&publicIPv4, &privateIPv4},
	}

	vpcIP := "10.0.0.2"
	noRoutesInVPC := []linodego.VPCIP{
		{
			Address:      &vpcIP,
			AddressRange: nil,
			VPCID:        vpcIDs["dummy"],
			NAT1To1:      nil,
			LinodeID:     nodeID,
		},
	}

	instanceConfigIntfWithVPCAndRoute := linodego.InstanceConfigInterface{
		VPCID:    ptr.To(vpcIDs["dummy"]),
		IPv4:     &linodego.VPCIPv4{VPC: vpcIP},
		IPRanges: []string{"10.10.10.0/24"},
	}
	route := &cloudprovider.Route{
		Name:            "route1",
		TargetNode:      types.NodeName(name),
		DestinationCIDR: "10.10.10.0/24",
	}

	t.Run("should return no error if instance exists, connected to VPC we add a route", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		instanceCache := newInstances(client)
		existingK8sCache := registeredK8sNodeCache
		defer func() {
			registeredK8sNodeCache = existingK8sCache
		}()
		registeredK8sNodeCache = newK8sNodeCache()
		registeredK8sNodeCache.addNodeToCache(node)
		routeController, err := newRoutes(client, instanceCache)
		require.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(noRoutesInVPC, nil)
		client.EXPECT().UpdateInstanceConfigInterface(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(&instanceConfigIntfWithVPCAndRoute, nil)
		err = routeController.CreateRoute(ctx, "dummy", "dummy", route)
		assert.NoError(t, err)
	})

	addressRange1 := "10.10.10.0/24"
	routesInVPC := []linodego.VPCIP{
		{
			Address:      &vpcIP,
			AddressRange: nil,
			VPCID:        vpcIDs["dummy"],
			NAT1To1:      nil,
			LinodeID:     nodeID,
		},
		{
			Address:      nil,
			AddressRange: &addressRange1,
			VPCID:        vpcIDs["dummy"],
			NAT1To1:      nil,
			LinodeID:     nodeID,
		},
	}

	t.Run("should return no error if instance exists, connected to VPC and route already exists", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		instanceCache := newInstances(client)
		routeController, err := newRoutes(client, instanceCache)
		require.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(routesInVPC, nil)
		err = routeController.CreateRoute(ctx, "dummy", "dummy", route)
		assert.NoError(t, err)
	})

	t.Run("should return error if instance doesn't exist", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		instanceCache := newInstances(client)
		routeController, err := newRoutes(client, instanceCache)
		require.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{}, nil)
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCIP{}, nil)
		err = routeController.CreateRoute(ctx, "dummy", "dummy", route)
		assert.Error(t, err)
	})
}

func TestDeleteRoute(t *testing.T) {
	Options.VPCNames = "dummy"
	vpcIDs["dummy"] = 1
	Options.EnableRouteController = true

	ctx := t.Context()

	nodeID := 123
	name := "mock-instance"
	publicIPv4 := net.ParseIP("45.76.101.25")
	privateIPv4 := net.ParseIP("192.168.133.65")
	linodeType := "g6-standard-1"
	region := "us-east"
	validInstance := linodego.Instance{
		ID:     nodeID,
		Label:  name,
		Type:   linodeType,
		Region: region,
		IPv4:   []*net.IP{&publicIPv4, &privateIPv4},
	}

	vpcIP := "10.0.0.2"
	route := &cloudprovider.Route{
		Name:            "route1",
		TargetNode:      types.NodeName(name),
		DestinationCIDR: "10.10.10.0/24",
	}

	t.Run("should return error if instance doesn't exist", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		instanceCache := newInstances(client)
		routeController, err := newRoutes(client, instanceCache)
		require.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{}, nil)
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCIP{}, nil)
		err = routeController.DeleteRoute(ctx, "dummy", route)
		assert.Error(t, err)
	})

	addressRange1 := "10.10.10.0/24"
	noRoutesInVPC := []linodego.VPCIP{
		{
			Address:      &vpcIP,
			AddressRange: nil,
			VPCID:        vpcIDs["dummy"],
			NAT1To1:      nil,
			LinodeID:     nodeID,
		},
	}

	instanceConfigIntfWithVPCAndNoRoute := linodego.InstanceConfigInterface{
		VPCID:    ptr.To(vpcIDs["dummy"]),
		IPv4:     &linodego.VPCIPv4{VPC: vpcIP},
		IPRanges: []string{},
	}

	t.Run("should return no error if instance exists, connected to VPC, route doesn't exist and we try to delete route", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		instanceCache := newInstances(client)
		routeController, err := newRoutes(client, instanceCache)
		require.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(noRoutesInVPC, nil)
		client.EXPECT().UpdateInstanceConfigInterface(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(&instanceConfigIntfWithVPCAndNoRoute, nil)
		err = routeController.DeleteRoute(ctx, "dummy", route)
		assert.NoError(t, err)
	})

	routesInVPC := []linodego.VPCIP{
		{
			Address:      &vpcIP,
			AddressRange: nil,
			VPCID:        vpcIDs["dummy"],
			NAT1To1:      nil,
			LinodeID:     nodeID,
		},
		{
			Address:      nil,
			AddressRange: &addressRange1,
			VPCID:        vpcIDs["dummy"],
			NAT1To1:      nil,
			LinodeID:     nodeID,
		},
	}

	t.Run("should return no error if instance exists, connected to VPC and route is deleted", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		instanceCache := newInstances(client)
		routeController, err := newRoutes(client, instanceCache)
		require.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(routesInVPC, nil)
		client.EXPECT().UpdateInstanceConfigInterface(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(&instanceConfigIntfWithVPCAndNoRoute, nil)
		err = routeController.DeleteRoute(ctx, "dummy", route)
		assert.NoError(t, err)
	})
}

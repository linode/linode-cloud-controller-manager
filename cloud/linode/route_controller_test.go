package linode

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/linode/linodego"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
)

func TestRouteControllerDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockClient(ctrl)
	vpcID := 1

	t.Run("should not fail if vpc is empty and routecontroller is disabled", func(t *testing.T) {
		Options.VPCName = ""
		Options.EnableRouteController = false
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(0)
		_, err := newRoutes(client)
		assert.NoError(t, err)
	})

	t.Run("should not fail if vpc is not empty and routecontroller is disabled", func(t *testing.T) {
		Options.VPCName = "abc"
		Options.EnableRouteController = false
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(0)
		_, err := newRoutes(client)
		assert.NoError(t, err)
	})

	t.Run("fail if vpc is empty and routecontroller is enabled", func(t *testing.T) {
		Options.VPCName = ""
		Options.EnableRouteController = true
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{{ID: vpcID, Label: "abc"}}, nil)
		_, err := newRoutes(client)
		assert.Error(t, err)
	})
}

func TestNewRoutes(t *testing.T) {
	Options.VPCName = "test"
	Options.EnableRouteController = true

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockClient(ctrl)
	vpcID := 1

	t.Run("should return error if vpc not found", func(t *testing.T) {
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{{ID: vpcID, Label: "abc"}}, nil)
		_, err := newRoutes(client)
		assert.Error(t, err)
	})

	t.Run("should return no error if vpc exists", func(t *testing.T) {
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{{ID: vpcID, Label: "test"}}, nil)
		routeController, err := newRoutes(client)
		assert.NoError(t, err)
		assert.NotNil(t, routeController)
	})

	t.Run("should return error if ListVPCs error out", func(t *testing.T) {
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return(nil, errors.New("Mock Error"))
		_, err := newRoutes(client)
		assert.Error(t, err)
	})
}

func TestListRoutes(t *testing.T) {
	Options.VPCName = "test"
	Options.EnableRouteController = true

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockClient(ctrl)

	nodeID := 123
	vpcID := 1
	vpcName := "test"
	name := "mock-instance"
	publicIPv4 := net.ParseIP("45.76.101.25")
	privateIPv4 := net.ParseIP("192.168.133.65")
	linodeType := "g6-standard-1"
	region := "us-east"

	t.Run("should return empty if no instance exists in cluster", func(t *testing.T) {
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{{ID: vpcID, Label: vpcName}}, nil)
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.Instance{}, nil)
		routes, err := routeController.ListRoutes(ctx, "abc")
		assert.NoError(t, err)
		assert.Empty(t, routes)
	})

	validInstance := linodego.Instance{ID: nodeID, Label: name, Type: linodeType, Region: region, IPv4: []*net.IP{&publicIPv4, &privateIPv4}}
	validInstanceAddrs := &linodego.InstanceIPAddressResponse{
		IPv4: &linodego.InstanceIPv4Response{
			Public: []*linodego.InstanceIP{
				{Address: "45.76.101.25"},
			},
			Private: []*linodego.InstanceIP{
				{Address: "192.168.133.65"},
			},
		},
		IPv6: nil,
	}
	instanceConfigWithoutVPC := linodego.InstanceConfig{
		ID: 123456,
		Interfaces: []linodego.InstanceConfigInterface{
			{
				VPCID: nil,
				IPv4:  linodego.VPCIPv4{},
			},
		},
	}

	t.Run("should return no routes if instance exists but is not connected to VPC", func(t *testing.T) {
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{{ID: vpcID, Label: vpcName}}, nil)
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(2).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().GetInstanceIPAddresses(gomock.Any(), nodeID).Times(1).Return(validInstanceAddrs, nil)
		client.EXPECT().ListInstanceConfigs(gomock.Any(), 123, gomock.Any()).Times(2).Return([]linodego.InstanceConfig{instanceConfigWithoutVPC}, nil)
		routes, err := routeController.ListRoutes(ctx, "abc")
		assert.NoError(t, err)
		assert.Empty(t, routes)
	})

	instanceConfigWithVPCNoRoutes := linodego.InstanceConfig{
		ID: 123456,
		Interfaces: []linodego.InstanceConfigInterface{
			{
				VPCID: &vpcID,
				IPv4:  linodego.VPCIPv4{VPC: "1.1.1.1"},
			},
		},
	}

	t.Run("should return no routes if instance exists, connected to VPC but no ip_ranges configured", func(t *testing.T) {
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{{ID: vpcID, Label: vpcName}}, nil)
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(2).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().GetInstanceIPAddresses(gomock.Any(), nodeID).Times(1).Return(validInstanceAddrs, nil)
		client.EXPECT().ListInstanceConfigs(gomock.Any(), 123, gomock.Any()).Times(2).Return([]linodego.InstanceConfig{instanceConfigWithVPCNoRoutes}, nil)
		routes, err := routeController.ListRoutes(ctx, "abc")
		assert.NoError(t, err)
		assert.Empty(t, routes)
	})

	instanceConfigWithVPCAndRoutes := linodego.InstanceConfig{
		ID: 123456,
		Interfaces: []linodego.InstanceConfigInterface{
			{
				VPCID:    &vpcID,
				IPv4:     linodego.VPCIPv4{VPC: "1.1.1.1"},
				IPRanges: []string{"10.10.10.0/24", "10.11.11.0/24"},
			},
		},
	}

	t.Run("should return routes if instance exists, connected to VPC and ip_ranges configured", func(t *testing.T) {
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{{ID: vpcID, Label: vpcName}}, nil)
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(2).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().GetInstanceIPAddresses(gomock.Any(), nodeID).Times(1).Return(validInstanceAddrs, nil)
		client.EXPECT().ListInstanceConfigs(gomock.Any(), 123, gomock.Any()).Times(2).Return([]linodego.InstanceConfig{instanceConfigWithVPCAndRoutes}, nil)
		routes, err := routeController.ListRoutes(ctx, "abc")
		assert.NoError(t, err)
		assert.NotEmpty(t, routes)
		assert.Equal(t, "10.10.10.0/24", routes[0].DestinationCIDR)
		assert.Equal(t, "10.11.11.0/24", routes[1].DestinationCIDR)
	})

	instanceConfigWithDifferentVPCAndRoutes := linodego.InstanceConfig{
		ID: 123456,
		Interfaces: []linodego.InstanceConfigInterface{
			{
				VPCID:    &nodeID,
				IPv4:     linodego.VPCIPv4{VPC: "1.1.1.1"},
				IPRanges: []string{"10.10.10.1"},
			},
		},
	}

	t.Run("should return no routes if instance exists, connected to VPC and ip_ranges configured but vpc id doesn't match", func(t *testing.T) {
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{{ID: vpcID, Label: vpcName}}, nil)
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(2).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().GetInstanceIPAddresses(gomock.Any(), nodeID).Times(1).Return(validInstanceAddrs, nil)
		client.EXPECT().ListInstanceConfigs(gomock.Any(), 123, gomock.Any()).Times(2).Return([]linodego.InstanceConfig{instanceConfigWithDifferentVPCAndRoutes}, nil)
		routes, err := routeController.ListRoutes(ctx, "abc")
		assert.NoError(t, err)
		assert.Empty(t, routes)
	})
}

func TestCreateRoute(t *testing.T) {
	Options.VPCName = "test"
	Options.EnableRouteController = true

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockClient(ctrl)

	nodeID := 123
	vpcID := 1
	vpcName := "test"
	name := "mock-instance"
	publicIPv4 := net.ParseIP("45.76.101.25")
	privateIPv4 := net.ParseIP("192.168.133.65")
	linodeType := "g6-standard-1"
	region := "us-east"
	validInstance := linodego.Instance{ID: nodeID, Label: name, Type: linodeType, Region: region, IPv4: []*net.IP{&publicIPv4, &privateIPv4}}
	validInstanceAddrs := &linodego.InstanceIPAddressResponse{
		IPv4: &linodego.InstanceIPv4Response{
			Public: []*linodego.InstanceIP{
				{Address: "45.76.101.25"},
			},
			Private: []*linodego.InstanceIP{
				{Address: "192.168.133.65"},
			},
		},
		IPv6: nil,
	}
	instanceConfigWithVPCNoRoutes := linodego.InstanceConfig{
		ID: 123456,
		Interfaces: []linodego.InstanceConfigInterface{
			{
				VPCID: &vpcID,
				IPv4:  linodego.VPCIPv4{VPC: "1.1.1.1"},
			},
		},
	}
	instanceConfigIntfWithVPCAndRoute := linodego.InstanceConfigInterface{
		VPCID:    &vpcID,
		IPv4:     linodego.VPCIPv4{VPC: "1.1.1.1"},
		IPRanges: []string{"10.10.10.0/24"},
	}
	route := &cloudprovider.Route{
		Name:            "route1",
		TargetNode:      types.NodeName(name),
		DestinationCIDR: "10.10.10.0/24",
	}

	t.Run("should return no error if instance exists, connected to VPC and route not present yet", func(t *testing.T) {
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{{ID: vpcID, Label: vpcName}}, nil)
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(2).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().GetInstanceIPAddresses(gomock.Any(), nodeID).Times(1).Return(validInstanceAddrs, nil)
		client.EXPECT().ListInstanceConfigs(gomock.Any(), 123, gomock.Any()).Times(2).Return([]linodego.InstanceConfig{instanceConfigWithVPCNoRoutes}, nil)
		client.EXPECT().UpdateInstanceConfigInterface(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(&instanceConfigIntfWithVPCAndRoute, nil)
		err = routeController.CreateRoute(ctx, "dummy", "dummy", route)
		assert.NoError(t, err)
	})

	instanceConfigWithVPCAndRoutes := linodego.InstanceConfig{
		ID:         123456,
		Interfaces: []linodego.InstanceConfigInterface{instanceConfigIntfWithVPCAndRoute},
	}

	t.Run("should return no error if instance exists, connected to VPC and route already exists", func(t *testing.T) {
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{{ID: vpcID, Label: vpcName}}, nil)
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(2).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().GetInstanceIPAddresses(gomock.Any(), nodeID).Times(1).Return(validInstanceAddrs, nil)
		client.EXPECT().ListInstanceConfigs(gomock.Any(), 123, gomock.Any()).Times(2).Return([]linodego.InstanceConfig{instanceConfigWithVPCAndRoutes}, nil)
		err = routeController.CreateRoute(ctx, "dummy", "dummy", route)
		assert.NoError(t, err)
	})

	t.Run("should return error if instance doesn't exist", func(t *testing.T) {
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{{ID: vpcID, Label: vpcName}}, nil)
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return(nil, errors.New("Mock Error"))
		err = routeController.CreateRoute(ctx, "dummy", "dummy", route)
		assert.Error(t, err)
	})
}

func TestDeleteRoute(t *testing.T) {
	Options.VPCName = "test"
	Options.EnableRouteController = true

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockClient(ctrl)

	nodeID := 123
	vpcID := 1
	vpcName := "test"
	name := "mock-instance"
	publicIPv4 := net.ParseIP("45.76.101.25")
	privateIPv4 := net.ParseIP("192.168.133.65")
	linodeType := "g6-standard-1"
	region := "us-east"
	validInstance := linodego.Instance{ID: nodeID, Label: name, Type: linodeType, Region: region, IPv4: []*net.IP{&publicIPv4, &privateIPv4}}
	validInstanceAddrs := &linodego.InstanceIPAddressResponse{
		IPv4: &linodego.InstanceIPv4Response{
			Public: []*linodego.InstanceIP{
				{Address: "45.76.101.25"},
			},
			Private: []*linodego.InstanceIP{
				{Address: "192.168.133.65"},
			},
		},
		IPv6: nil,
	}
	route := &cloudprovider.Route{
		Name:            "route1",
		TargetNode:      types.NodeName(name),
		DestinationCIDR: "10.10.10.0/24",
	}
	instanceConfigIntfWithVPCAndRoute := linodego.InstanceConfigInterface{
		VPCID:    &vpcID,
		IPv4:     linodego.VPCIPv4{VPC: "1.1.1.1"},
		IPRanges: []string{"10.10.10.0/24"},
	}
	instanceConfigIntfWithVPCAndNoRoute := linodego.InstanceConfigInterface{
		VPCID:    &vpcID,
		IPv4:     linodego.VPCIPv4{VPC: "1.1.1.1"},
		IPRanges: []string{},
	}
	instanceConfigWithVPCAndRoutes := linodego.InstanceConfig{
		ID:         123456,
		Interfaces: []linodego.InstanceConfigInterface{instanceConfigIntfWithVPCAndRoute},
	}

	t.Run("should return error if instance doesn't exist", func(t *testing.T) {
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{{ID: vpcID, Label: vpcName}}, nil)
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{}, errors.New("no instance"))
		err = routeController.DeleteRoute(ctx, "dummy", route)
		assert.Error(t, err)
	})

	t.Run("should return no error if instance exists, connected to VPC and route is deleted", func(t *testing.T) {
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{{ID: vpcID, Label: vpcName}}, nil)
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(2).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().GetInstanceIPAddresses(gomock.Any(), nodeID).Times(1).Return(validInstanceAddrs, nil)
		client.EXPECT().ListInstanceConfigs(gomock.Any(), 123, gomock.Any()).Times(2).Return([]linodego.InstanceConfig{instanceConfigWithVPCAndRoutes}, nil)
		client.EXPECT().UpdateInstanceConfigInterface(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(&instanceConfigIntfWithVPCAndNoRoute, nil)
		err = routeController.DeleteRoute(ctx, "dummy", route)
		assert.NoError(t, err)
	})
}

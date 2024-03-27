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

func TestListRoutes(t *testing.T) {
	Options.VPCName = "test"
	Options.EnableRouteController = true

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := NewMockClient(ctrl)
	vpcInfo.id = 1

	nodeID := 123
	name := "mock-instance"
	publicIPv4 := net.ParseIP("45.76.101.25")
	privateIPv4 := net.ParseIP("192.168.133.65")
	linodeType := "g6-standard-1"
	region := "us-east"

	t.Run("should return empty if no instance exists in cluster", func(t *testing.T) {
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.Instance{}, nil)
		client.EXPECT().GetVPC(gomock.Any(), gomock.Any()).Times(1).Return(&linodego.VPC{}, nil)
		routes, err := routeController.ListRoutes(ctx, "abc")
		assert.NoError(t, err)
		assert.Empty(t, routes)
	})

	validInstance := linodego.Instance{ID: nodeID, Label: name, Type: linodeType, Region: region, IPv4: []*net.IP{&publicIPv4, &privateIPv4}}

	t.Run("should return no routes if instance exists but is not connected to VPC", func(t *testing.T) {
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().GetVPC(gomock.Any(), gomock.Any()).Times(1).Return(&linodego.VPC{}, nil)
		routes, err := routeController.ListRoutes(ctx, "abc")
		assert.NoError(t, err)
		assert.Empty(t, routes)
	})

	instanceConfigWithVPCNoRoutes := linodego.InstanceConfig{
		ID: 123456,
		Interfaces: []linodego.InstanceConfigInterface{
			{
				VPCID: &vpcInfo.id,
				IPv4:  linodego.VPCIPv4{VPC: "1.1.1.1"},
			},
		},
	}
	vpcWithInstance := linodego.VPC{
		Subnets: []linodego.VPCSubnet{
			{
				Linodes: []linodego.VPCSubnetLinode{
					{
						ID:         nodeID,
						Interfaces: []linodego.VPCSubnetLinodeInterface{},
					},
				},
			},
		},
	}

	t.Run("should return no routes if instance exists, connected to VPC but no ip_ranges configured", func(t *testing.T) {
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().GetVPC(gomock.Any(), gomock.Any()).Times(2).Return(&vpcWithInstance, nil)
		client.EXPECT().ListInstanceConfigs(gomock.Any(), 123, gomock.Any()).Times(2).Return([]linodego.InstanceConfig{instanceConfigWithVPCNoRoutes}, nil)
		routes, err := routeController.ListRoutes(ctx, "abc")
		assert.NoError(t, err)
		assert.Empty(t, routes)
	})

	instanceConfigWithVPCAndRoutes := linodego.InstanceConfig{
		ID: 123456,
		Interfaces: []linodego.InstanceConfigInterface{
			{
				VPCID:    &vpcInfo.id,
				IPv4:     linodego.VPCIPv4{VPC: "1.1.1.1"},
				IPRanges: []string{"10.10.10.0/24", "10.11.11.0/24"},
			},
		},
	}

	t.Run("should return routes if instance exists, connected to VPC and ip_ranges configured", func(t *testing.T) {
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().GetVPC(gomock.Any(), gomock.Any()).Times(2).Return(&vpcWithInstance, nil)
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
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().GetVPC(gomock.Any(), gomock.Any()).Times(2).Return(&vpcWithInstance, nil)
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
	vpcInfo.id = 1

	nodeID := 123
	name := "mock-instance"
	publicIPv4 := net.ParseIP("45.76.101.25")
	privateIPv4 := net.ParseIP("192.168.133.65")
	linodeType := "g6-standard-1"
	region := "us-east"
	validInstance := linodego.Instance{ID: nodeID, Label: name, Type: linodeType, Region: region, IPv4: []*net.IP{&publicIPv4, &privateIPv4}}

	vpcWithInstance := linodego.VPC{
		Subnets: []linodego.VPCSubnet{
			{
				Linodes: []linodego.VPCSubnetLinode{
					{
						ID:         nodeID,
						Interfaces: []linodego.VPCSubnetLinodeInterface{},
					},
				},
			},
		},
	}

	instanceConfigWithVPCNoRoutes := linodego.InstanceConfig{
		ID: 123456,
		Interfaces: []linodego.InstanceConfigInterface{
			{
				VPCID: &vpcInfo.id,
				IPv4:  linodego.VPCIPv4{VPC: "1.1.1.1"},
			},
		},
	}
	instanceConfigIntfWithVPCAndRoute := linodego.InstanceConfigInterface{
		VPCID:    &vpcInfo.id,
		IPv4:     linodego.VPCIPv4{VPC: "1.1.1.1"},
		IPRanges: []string{"10.10.10.0/24"},
	}
	route := &cloudprovider.Route{
		Name:            "route1",
		TargetNode:      types.NodeName(name),
		DestinationCIDR: "10.10.10.0/24",
	}

	t.Run("should return no error if instance exists, connected to VPC and route not present yet", func(t *testing.T) {
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().GetVPC(gomock.Any(), gomock.Any()).Times(2).Return(&vpcWithInstance, nil)
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
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().GetVPC(gomock.Any(), gomock.Any()).Times(2).Return(&vpcWithInstance, nil)
		client.EXPECT().ListInstanceConfigs(gomock.Any(), 123, gomock.Any()).Times(2).Return([]linodego.InstanceConfig{instanceConfigWithVPCAndRoutes}, nil)
		err = routeController.CreateRoute(ctx, "dummy", "dummy", route)
		assert.NoError(t, err)
	})

	t.Run("should return error if instance doesn't exist", func(t *testing.T) {
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
	vpcInfo.id = 1

	nodeID := 123
	name := "mock-instance"
	publicIPv4 := net.ParseIP("45.76.101.25")
	privateIPv4 := net.ParseIP("192.168.133.65")
	linodeType := "g6-standard-1"
	region := "us-east"
	validInstance := linodego.Instance{ID: nodeID, Label: name, Type: linodeType, Region: region, IPv4: []*net.IP{&publicIPv4, &privateIPv4}}
	vpcWithInstance := linodego.VPC{
		Subnets: []linodego.VPCSubnet{
			{
				Linodes: []linodego.VPCSubnetLinode{
					{
						ID:         nodeID,
						Interfaces: []linodego.VPCSubnetLinodeInterface{},
					},
				},
			},
		},
	}
	route := &cloudprovider.Route{
		Name:            "route1",
		TargetNode:      types.NodeName(name),
		DestinationCIDR: "10.10.10.0/24",
	}
	instanceConfigIntfWithVPCAndRoute := linodego.InstanceConfigInterface{
		VPCID:    &vpcInfo.id,
		IPv4:     linodego.VPCIPv4{VPC: "1.1.1.1"},
		IPRanges: []string{"10.10.10.0/24"},
	}
	instanceConfigIntfWithVPCAndNoRoute := linodego.InstanceConfigInterface{
		VPCID:    &vpcInfo.id,
		IPv4:     linodego.VPCIPv4{VPC: "1.1.1.1"},
		IPRanges: []string{},
	}
	instanceConfigWithVPCAndRoutes := linodego.InstanceConfig{
		ID:         123456,
		Interfaces: []linodego.InstanceConfigInterface{instanceConfigIntfWithVPCAndRoute},
	}

	t.Run("should return error if instance doesn't exist", func(t *testing.T) {
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{}, errors.New("no instance"))
		err = routeController.DeleteRoute(ctx, "dummy", route)
		assert.Error(t, err)
	})

	t.Run("should return no error if instance exists, connected to VPC and route is deleted", func(t *testing.T) {
		routeController, err := newRoutes(client)
		assert.NoError(t, err)

		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{validInstance}, nil)
		client.EXPECT().GetVPC(gomock.Any(), gomock.Any()).Times(2).Return(&vpcWithInstance, nil)
		client.EXPECT().ListInstanceConfigs(gomock.Any(), 123, gomock.Any()).Times(2).Return([]linodego.InstanceConfig{instanceConfigWithVPCAndRoutes}, nil)
		client.EXPECT().UpdateInstanceConfigInterface(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(&instanceConfigIntfWithVPCAndNoRoute, nil)
		err = routeController.DeleteRoute(ctx, "dummy", route)
		assert.NoError(t, err)
	})
}

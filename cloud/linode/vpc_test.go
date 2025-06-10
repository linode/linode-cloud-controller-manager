package linode

import (
	"errors"
	"net/http"
	"reflect"
	"sort"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/linode/linodego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client/mocks"
)

func TestGetAllVPCIDs(t *testing.T) {
	tests := []struct {
		name   string
		vpcIDs map[string]int
		want   []int
	}{
		{
			name:   "multiple vpcs present",
			vpcIDs: map[string]int{"test1": 1, "test2": 2, "test3": 3},
			want:   []int{1, 2, 3},
		},
		{
			name:   "no vpc present",
			vpcIDs: map[string]int{},
			want:   []int{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vpcIDs = tt.vpcIDs
			got := GetAllVPCIDs()
			sort.Ints(got)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetAllVPCIDs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetVPCID(t *testing.T) {
	t.Run("vpcID in cache", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		vpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		got, err := GetVPCID(t.Context(), client, "test3")
		if err != nil {
			t.Errorf("GetVPCID() error = %v", err)
			return
		}
		if got != vpcIDs["test3"] {
			t.Errorf("GetVPCID() = %v, want %v", got, vpcIDs["test3"])
		}
	})

	t.Run("vpcID not in cache and listVPCs return error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		vpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{}, errors.New("error"))
		got, err := GetVPCID(t.Context(), client, "test4")
		require.Error(t, err)
		if got != 0 {
			t.Errorf("GetVPCID() = %v, want %v", got, 0)
		}
	})

	t.Run("vpcID not in cache and listVPCs return nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		vpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{}, nil)
		got, err := GetVPCID(t.Context(), client, "test4")
		require.ErrorIs(t, err, vpcLookupError{"test4"})
		if got != 0 {
			t.Errorf("GetVPCID() = %v, want %v", got, 0)
		}
	})

	t.Run("vpcID not in cache and listVPCs return vpc info", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		vpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{{ID: 4, Label: "test4"}}, nil)
		got, err := GetVPCID(t.Context(), client, "test4")
		require.NoError(t, err)
		if got != 4 {
			t.Errorf("GetVPCID() = %v, want %v", got, 4)
		}
	})
}

func TestGetVPCIPAddresses(t *testing.T) {
	t.Run("vpc id not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		vpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{}, nil)
		_, err := GetVPCIPAddresses(t.Context(), client, "test4")
		require.Error(t, err)
	})

	t.Run("vpc id found but listing ip addresses fails with 404 error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		vpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCIP{}, &linodego.Error{Code: http.StatusNotFound, Message: "[404] [label] VPC not found"})
		_, err := GetVPCIPAddresses(t.Context(), client, "test3")
		require.Error(t, err)
		_, exists := vpcIDs["test3"]
		assert.False(t, exists, "test3 key should get deleted from vpcIDs map")
	})

	t.Run("vpc id found but listing ip addresses fails with 500 error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		vpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCIP{}, &linodego.Error{Code: http.StatusInternalServerError, Message: "[500] [label] Internal Server Error"})
		_, err := GetVPCIPAddresses(t.Context(), client, "test1")
		require.Error(t, err)
		_, exists := vpcIDs["test1"]
		assert.True(t, exists, "test1 key should not get deleted from vpcIDs map")
	})

	t.Run("vpc id found and listing vpc ipaddresses succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		vpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{{ID: 10, Label: "test10"}}, nil)
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCIP{}, nil)
		_, err := GetVPCIPAddresses(t.Context(), client, "test10")
		require.NoError(t, err)
		_, exists := vpcIDs["test10"]
		assert.True(t, exists, "test10 key should be present in vpcIDs map")
	})

	t.Run("vpc id found and ip addresses found with subnet filtering", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		sn := Options.SubnetNames
		defer func() { Options.SubnetNames = sn }()
		Options.SubnetNames = "subnet4"
		vpcIDs = map[string]int{"test1": 1}
		subnetIDs = map[string]int{"subnet1": 1}
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{{ID: 10, Label: "test10"}}, nil)
		client.EXPECT().ListVPCSubnets(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCSubnet{{ID: 4, Label: "subnet4"}}, nil)
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCIP{}, nil)
		_, err := GetVPCIPAddresses(t.Context(), client, "test10")
		require.NoError(t, err)
		_, exists := subnetIDs["subnet4"]
		assert.True(t, exists, "subnet4 should be present in subnetIDs map")
	})
}

func TestGetSubnetID(t *testing.T) {
	t.Run("subnet in cache", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		subnetIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		got, err := GetSubnetID(t.Context(), client, 0, "test3")
		if err != nil {
			t.Errorf("GetSubnetID() error = %v", err)
			return
		}
		if got != subnetIDs["test3"] {
			t.Errorf("GetSubnetID() = %v, want %v", got, subnetIDs["test3"])
		}
	})

	t.Run("subnetID not in cache and listVPCSubnets return error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		subnetIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCSubnets(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCSubnet{}, errors.New("error"))
		got, err := GetSubnetID(t.Context(), client, 0, "test4")
		require.Error(t, err)
		if got != 0 {
			t.Errorf("GetSubnetID() = %v, want %v", got, 0)
		}
		_, exists := subnetIDs["test4"]
		assert.False(t, exists, "subnet4 should not be present in subnetIDs")
	})

	t.Run("subnetID not in cache and listVPCSubnets return nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		subnetIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCSubnets(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCSubnet{}, nil)
		got, err := GetSubnetID(t.Context(), client, 0, "test4")
		require.ErrorIs(t, err, subnetLookupError{"test4"})
		if got != 0 {
			t.Errorf("GetSubnetID() = %v, want %v", got, 0)
		}
	})

	t.Run("subnetID not in cache and listVPCSubnets return subnet info", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		subnetIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCSubnets(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCSubnet{{ID: 4, Label: "test4"}}, nil)
		got, err := GetSubnetID(t.Context(), client, 0, "test4")
		require.NoError(t, err)
		if got != 4 {
			t.Errorf("GetSubnetID() = %v, want %v", got, 4)
		}
	})
}

func TestGetNodeBalancerBackendIPv4SubnetID(t *testing.T) {
	t.Run("VPC not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		currVPCNames := Options.VPCNames
		currVPCIDs := vpcIDs
		currSubnetIDs := subnetIDs
		defer func() {
			Options.VPCNames = currVPCNames
			vpcIDs = currVPCIDs
			subnetIDs = currSubnetIDs
		}()
		Options.VPCNames = "vpc-test1,vpc-test2,vpc-test3"
		vpcIDs = map[string]int{"vpc-test2": 2, "vpc-test3": 3}
		subnetIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{}, errors.New("error"))
		subnetID, err := getNodeBalancerBackendIPv4SubnetID(client)
		require.Error(t, err)
		if subnetID != 0 {
			t.Errorf("getNodeBalancerBackendIPv4SubnetID() = %v, want %v", subnetID, 0)
		}
	})

	t.Run("Subnet not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		currVPCNames := Options.VPCNames
		defer func() { Options.VPCNames = currVPCNames }()
		Options.VPCNames = "vpc-test1,vpc-test2,vpc-test3"
		vpcIDs = map[string]int{"vpc-test1": 1, "vpc-test2": 2, "vpc-test3": 3}
		subnetIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCSubnets(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCSubnet{}, errors.New("error"))
		subnetID, err := getNodeBalancerBackendIPv4SubnetID(client)
		require.Error(t, err)
		if subnetID != 0 {
			t.Errorf("getNodeBalancerBackendIPv4SubnetID() = %v, want %v", subnetID, 0)
		}
	})

	t.Run("Subnet found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		currVPCNames := Options.VPCNames
		currNodeBalancerBackendIPv4SubnetName := Options.NodeBalancerBackendIPv4SubnetName
		defer func() {
			Options.VPCNames = currVPCNames
			Options.NodeBalancerBackendIPv4SubnetName = currNodeBalancerBackendIPv4SubnetName
		}()
		Options.VPCNames = "vpc-test1,vpc-test2,vpc-test3"
		Options.NodeBalancerBackendIPv4SubnetName = "test4"
		vpcIDs = map[string]int{"vpc-test1": 1, "vpc-test2": 2, "vpc-test3": 3}
		subnetIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCSubnets(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCSubnet{{ID: 4, Label: "test4"}}, nil)
		subnetID, err := getNodeBalancerBackendIPv4SubnetID(client)
		require.NoError(t, err)
		if subnetID != 4 {
			t.Errorf("getNodeBalancerBackendIPv4SubnetID() = %v, want %v", subnetID, 4)
		}
	})
}

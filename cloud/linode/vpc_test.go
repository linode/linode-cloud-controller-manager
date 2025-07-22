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
		Options.SubnetNames = []string{"subnet4"}
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
		Options.VPCNames = []string{"vpc-test1", "vpc-test2", "vpc-test3"}
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
		Options.VPCNames = []string{"vpc-test1", "vpc-test2", "vpc-test3"}
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
		Options.VPCNames = []string{"vpc-test1", "vpc-test2", "vpc-test3"}
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

func Test_validateVPCSubnetFlags(t *testing.T) {
	tests := []struct {
		name        string
		vpcIDs      []int
		vpcNames    []string
		subnetIDs   []int
		subnetNames []string
		wantErr     bool
	}{
		{
			name:        "invalid flags with vpc-names and vpc-ids set",
			vpcIDs:      []int{1, 2},
			vpcNames:    []string{"vpc1", "vpc2"},
			subnetIDs:   []int{1},
			subnetNames: []string{},
			wantErr:     true,
		},
		{
			name:        "invalid flags with subnet-names and subnet-ids set",
			vpcIDs:      []int{},
			vpcNames:    []string{"vpc1", "vpc2"},
			subnetIDs:   []int{1, 2},
			subnetNames: []string{"subnet1", "subnet2"},
			wantErr:     true,
		},
		{
			name:        "invalid flags with subnet-names and no vpc-names",
			vpcIDs:      []int{},
			vpcNames:    []string{},
			subnetIDs:   []int{},
			subnetNames: []string{"subnet1", "subnet2"},
			wantErr:     true,
		},
		{
			name:        "invalid flags with subnet-ids and no vpc-ids",
			vpcIDs:      []int{},
			vpcNames:    []string{},
			subnetIDs:   []int{1, 2},
			subnetNames: []string{},
			wantErr:     true,
		},
		{
			name:        "invalid flags with vpc-ids and no subnet-ids",
			vpcIDs:      []int{1, 2},
			vpcNames:    []string{},
			subnetIDs:   []int{},
			subnetNames: []string{},
			wantErr:     true,
		},
		{
			name:        "valid flags with vpc-names and subnet-names",
			vpcIDs:      []int{},
			vpcNames:    []string{"vpc1", "vpc2"},
			subnetIDs:   []int{},
			subnetNames: []string{"subnet1", "subnet2"},
			wantErr:     false,
		},
		{
			name:        "valid flags with vpc-ids and subnet-ids",
			vpcIDs:      []int{1, 2},
			vpcNames:    []string{},
			subnetIDs:   []int{1, 2},
			subnetNames: []string{},
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Options.VPCIDs = tt.vpcIDs
			Options.VPCNames = tt.vpcNames
			Options.SubnetIDs = tt.subnetIDs
			Options.SubnetNames = tt.subnetNames
			if err := validateVPCSubnetFlags(); (err != nil) != tt.wantErr {
				t.Errorf("validateVPCSubnetFlags() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_resolveSubnetNames(t *testing.T) {
	t.Run("empty subnet ids", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		optionsSubnetIDs := Options.SubnetIDs
		currSubnetIDs := subnetIDs
		defer func() {
			Options.SubnetIDs = optionsSubnetIDs
			subnetIDs = currSubnetIDs
		}()
		Options.SubnetIDs = []int{}
		// client.EXPECT().GetVPCSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(&linodego.VPCSubnet{}, errors.New("error"))
		subnetNames, err := resolveSubnetNames(client, 10)
		if err != nil {
			t.Errorf("resolveSubnetNames() error = %v", err)
			return
		}
		if len(subnetNames) != 0 {
			t.Errorf("resolveSubnetNames() = %v, want %v", subnetNames, []string{})
		}
	})

	t.Run("fails getting subnet ids", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		optionsSubnetIDs := Options.SubnetIDs
		currSubnetIDs := subnetIDs
		defer func() {
			Options.SubnetIDs = optionsSubnetIDs
			subnetIDs = currSubnetIDs
		}()
		Options.SubnetIDs = []int{1}
		client.EXPECT().GetVPCSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(&linodego.VPCSubnet{}, errors.New("error"))
		_, err := resolveSubnetNames(client, 10)
		require.Error(t, err)
	})

	t.Run("correctly resolves subnet names", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		optionsSubnetIDs := Options.SubnetIDs
		currSubnetIDs := subnetIDs
		defer func() {
			Options.SubnetIDs = optionsSubnetIDs
			subnetIDs = currSubnetIDs
		}()
		Options.SubnetIDs = []int{1}
		client.EXPECT().GetVPCSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(&linodego.VPCSubnet{ID: 1, Label: "subnet1"}, nil)
		subnet, err := resolveSubnetNames(client, 10)
		require.NoError(t, err)
		require.Equal(t, []string{"subnet1"}, subnet, "Expected subnet names to match")
	})
}

func Test_validateAndSetVPCSubnetFlags(t *testing.T) {
	t.Run("invalid flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		Options.VPCIDs = []int{1, 2}
		Options.SubnetIDs = []int{}
		err := validateAndSetVPCSubnetFlags(client)
		require.Error(t, err)
	})

	t.Run("valid flags with vpc-ids and subnet-ids", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		currVPCIDs := Options.VPCIDs
		currSubnetIDs := Options.SubnetIDs
		defer func() {
			Options.VPCIDs = currVPCIDs
			Options.SubnetIDs = currSubnetIDs
			vpcIDs = map[string]int{}
			subnetIDs = map[string]int{}
		}()
		Options.VPCIDs = []int{1}
		Options.SubnetIDs = []int{1, 2}
		client.EXPECT().GetVPC(gomock.Any(), gomock.Any()).Times(1).Return(&linodego.VPC{ID: 1, Label: "test"}, nil)
		client.EXPECT().GetVPCSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(&linodego.VPCSubnet{ID: 1, Label: "subnet1"}, nil)
		err := validateAndSetVPCSubnetFlags(client)
		require.NoError(t, err)
	})

	t.Run("error while making linode api call", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		currVPCIDs := Options.VPCIDs
		currSubnetIDs := Options.SubnetIDs
		defer func() {
			Options.VPCIDs = currVPCIDs
			Options.SubnetIDs = currSubnetIDs
			vpcIDs = map[string]int{}
			subnetIDs = map[string]int{}
		}()
		Options.VPCIDs = []int{1}
		Options.SubnetIDs = []int{1, 2}
		Options.VPCNames = []string{}
		Options.SubnetNames = []string{}
		client.EXPECT().GetVPC(gomock.Any(), gomock.Any()).Times(1).Return(nil, errors.New("error"))
		err := validateAndSetVPCSubnetFlags(client)
		require.Error(t, err)
	})

	t.Run("valid flags with vpc-names and subnet-names", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		currVPCNames := Options.VPCNames
		currSubnetNames := Options.SubnetNames
		defer func() {
			Options.VPCNames = currVPCNames
			Options.SubnetNames = currSubnetNames
			vpcIDs = map[string]int{}
			subnetIDs = map[string]int{}
		}()
		Options.VPCNames = []string{"vpc1"}
		Options.SubnetNames = []string{"subnet1", "subnet2"}
		Options.VPCIDs = []int{}
		Options.SubnetIDs = []int{}
		err := validateAndSetVPCSubnetFlags(client)
		require.NoError(t, err)
	})
}

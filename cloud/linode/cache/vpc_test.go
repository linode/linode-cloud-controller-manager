package cache

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
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/options"
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
			VpcIDs = tt.vpcIDs
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
		VpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		got, err := GetVPCID(t.Context(), client, "test3")
		if err != nil {
			t.Errorf("GetVPCID() error = %v", err)
			return
		}
		if got != VpcIDs["test3"] {
			t.Errorf("GetVPCID() = %v, want %v", got, VpcIDs["test3"])
		}
	})

	t.Run("vpcID not in cache and listVPCs return error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		VpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
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
		VpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
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
		VpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
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
		VpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{}, nil)
		_, err := GetVPCIPAddresses(t.Context(), client, "test4")
		require.Error(t, err)
	})

	t.Run("vpc id found but listing ip addresses fails with 404 error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		VpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCIP{}, &linodego.Error{Code: http.StatusNotFound, Message: "[404] [label] VPC not found"})
		_, err := GetVPCIPAddresses(t.Context(), client, "test3")
		require.Error(t, err)
		_, exists := VpcIDs["test3"]
		assert.False(t, exists, "test3 key should get deleted from vpcIDs map")
	})

	t.Run("vpc id found but listing ip addresses fails with 500 error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		VpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCIP{}, &linodego.Error{Code: http.StatusInternalServerError, Message: "[500] [label] Internal Server Error"})
		_, err := GetVPCIPAddresses(t.Context(), client, "test1")
		require.Error(t, err)
		_, exists := VpcIDs["test1"]
		assert.True(t, exists, "test1 key should not get deleted from vpcIDs map")
	})

	t.Run("vpc id found and listing vpc ipaddresses succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		VpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{{ID: 10, Label: "test10"}}, nil)
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCIP{}, nil)
		_, err := GetVPCIPAddresses(t.Context(), client, "test10")
		require.NoError(t, err)
		_, exists := VpcIDs["test10"]
		assert.True(t, exists, "test10 key should be present in vpcIDs map")
	})

	t.Run("vpc id found and ip addresses found with subnet filtering", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		sn := options.Options.SubnetNames
		defer func() { options.Options.SubnetNames = sn }()
		options.Options.SubnetNames = []string{"subnet4"}
		VpcIDs = map[string]int{"test1": 1}
		SubnetIDs = map[string]int{"subnet1": 1}
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{{ID: 10, Label: "test10"}}, nil)
		client.EXPECT().ListVPCSubnets(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCSubnet{{ID: 4, Label: "subnet4"}}, nil)
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCIP{}, nil)
		_, err := GetVPCIPAddresses(t.Context(), client, "test10")
		require.NoError(t, err)
		_, exists := SubnetIDs["subnet4"]
		assert.True(t, exists, "subnet4 should be present in subnetIDs map")
	})
}

func TestGetSubnetID(t *testing.T) {
	t.Run("subnet in cache", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		SubnetIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		got, err := GetSubnetID(t.Context(), client, 0, "test3")
		if err != nil {
			t.Errorf("GetSubnetID() error = %v", err)
			return
		}
		if got != SubnetIDs["test3"] {
			t.Errorf("GetSubnetID() = %v, want %v", got, SubnetIDs["test3"])
		}
	})

	t.Run("subnetID not in cache and listVPCSubnets return error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		SubnetIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCSubnets(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCSubnet{}, errors.New("error"))
		got, err := GetSubnetID(t.Context(), client, 0, "test4")
		require.Error(t, err)
		if got != 0 {
			t.Errorf("GetSubnetID() = %v, want %v", got, 0)
		}
		_, exists := SubnetIDs["test4"]
		assert.False(t, exists, "subnet4 should not be present in subnetIDs")
	})

	t.Run("subnetID not in cache and listVPCSubnets return nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		SubnetIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
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
		SubnetIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
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
		currVPCNames := options.Options.VPCNames
		currVPCIDs := VpcIDs
		currSubnetIDs := SubnetIDs
		defer func() {
			options.Options.VPCNames = currVPCNames
			VpcIDs = currVPCIDs
			SubnetIDs = currSubnetIDs
		}()
		options.Options.VPCNames = []string{"vpc-test1", "vpc-test2", "vpc-test3"}
		VpcIDs = map[string]int{"vpc-test2": 2, "vpc-test3": 3}
		SubnetIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{}, errors.New("error"))
		subnetID, err := GetNodeBalancerBackendIPv4SubnetID(client)
		require.Error(t, err)
		if subnetID != 0 {
			t.Errorf("getNodeBalancerBackendIPv4SubnetID() = %v, want %v", subnetID, 0)
		}
	})

	t.Run("Subnet not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		currVPCNames := options.Options.VPCNames
		defer func() { options.Options.VPCNames = currVPCNames }()
		options.Options.VPCNames = []string{"vpc-test1", "vpc-test2", "vpc-test3"}
		VpcIDs = map[string]int{"vpc-test1": 1, "vpc-test2": 2, "vpc-test3": 3}
		SubnetIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCSubnets(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCSubnet{}, errors.New("error"))
		subnetID, err := GetNodeBalancerBackendIPv4SubnetID(client)
		require.Error(t, err)
		if subnetID != 0 {
			t.Errorf("getNodeBalancerBackendIPv4SubnetID() = %v, want %v", subnetID, 0)
		}
	})

	t.Run("Subnet found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		currVPCNames := options.Options.VPCNames
		currNodeBalancerBackendIPv4SubnetName := options.Options.NodeBalancerBackendIPv4SubnetName
		defer func() {
			options.Options.VPCNames = currVPCNames
			options.Options.NodeBalancerBackendIPv4SubnetName = currNodeBalancerBackendIPv4SubnetName
		}()
		options.Options.VPCNames = []string{"vpc-test1", "vpc-test2", "vpc-test3"}
		options.Options.NodeBalancerBackendIPv4SubnetName = "test4"
		VpcIDs = map[string]int{"vpc-test1": 1, "vpc-test2": 2, "vpc-test3": 3}
		SubnetIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCSubnets(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCSubnet{{ID: 4, Label: "test4"}}, nil)
		subnetID, err := GetNodeBalancerBackendIPv4SubnetID(client)
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
			options.Options.VPCIDs = tt.vpcIDs
			options.Options.VPCNames = tt.vpcNames
			options.Options.SubnetIDs = tt.subnetIDs
			options.Options.SubnetNames = tt.subnetNames
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
		optionsSubnetIDs := options.Options.SubnetIDs
		currSubnetIDs := SubnetIDs
		defer func() {
			options.Options.SubnetIDs = optionsSubnetIDs
			SubnetIDs = currSubnetIDs
		}()
		options.Options.SubnetIDs = []int{}
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
		optionsSubnetIDs := options.Options.SubnetIDs
		currSubnetIDs := SubnetIDs
		defer func() {
			options.Options.SubnetIDs = optionsSubnetIDs
			SubnetIDs = currSubnetIDs
		}()
		options.Options.SubnetIDs = []int{1}
		client.EXPECT().GetVPCSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(&linodego.VPCSubnet{}, errors.New("error"))
		_, err := resolveSubnetNames(client, 10)
		require.Error(t, err)
	})

	t.Run("correctly resolves subnet names", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		optionsSubnetIDs := options.Options.SubnetIDs
		currSubnetIDs := SubnetIDs
		defer func() {
			options.Options.SubnetIDs = optionsSubnetIDs
			SubnetIDs = currSubnetIDs
		}()
		options.Options.SubnetIDs = []int{1}
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
		options.Options.VPCIDs = []int{1, 2}
		options.Options.SubnetIDs = []int{}
		err := ValidateAndSetVPCSubnetFlags(client)
		require.Error(t, err)
	})

	t.Run("valid flags with vpc-ids and subnet-ids", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		currVPCIDs := options.Options.VPCIDs
		currSubnetIDs := options.Options.SubnetIDs
		defer func() {
			options.Options.VPCIDs = currVPCIDs
			options.Options.SubnetIDs = currSubnetIDs
			VpcIDs = map[string]int{}
			SubnetIDs = map[string]int{}
		}()
		options.Options.VPCIDs = []int{1}
		options.Options.SubnetIDs = []int{1, 2}
		client.EXPECT().GetVPC(gomock.Any(), gomock.Any()).Times(1).Return(&linodego.VPC{ID: 1, Label: "test"}, nil)
		client.EXPECT().GetVPCSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(&linodego.VPCSubnet{ID: 1, Label: "subnet1"}, nil)
		err := ValidateAndSetVPCSubnetFlags(client)
		require.NoError(t, err)
	})

	t.Run("error while making linode api call", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		currVPCIDs := options.Options.VPCIDs
		currSubnetIDs := options.Options.SubnetIDs
		defer func() {
			options.Options.VPCIDs = currVPCIDs
			options.Options.SubnetIDs = currSubnetIDs
			VpcIDs = map[string]int{}
			SubnetIDs = map[string]int{}
		}()
		options.Options.VPCIDs = []int{1}
		options.Options.SubnetIDs = []int{1, 2}
		options.Options.VPCNames = []string{}
		options.Options.SubnetNames = []string{}
		client.EXPECT().GetVPC(gomock.Any(), gomock.Any()).Times(1).Return(nil, errors.New("error"))
		err := ValidateAndSetVPCSubnetFlags(client)
		require.Error(t, err)
	})

	t.Run("valid flags with vpc-names and subnet-names", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		currVPCNames := options.Options.VPCNames
		currSubnetNames := options.Options.SubnetNames
		defer func() {
			options.Options.VPCNames = currVPCNames
			options.Options.SubnetNames = currSubnetNames
			VpcIDs = map[string]int{}
			SubnetIDs = map[string]int{}
		}()
		options.Options.VPCNames = []string{"vpc1"}
		options.Options.SubnetNames = []string{"subnet1", "subnet2"}
		options.Options.VPCIDs = []int{}
		options.Options.SubnetIDs = []int{}
		err := ValidateAndSetVPCSubnetFlags(client)
		require.NoError(t, err)
	})
}

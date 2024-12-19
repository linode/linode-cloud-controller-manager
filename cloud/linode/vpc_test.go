package linode

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sort"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client/mocks"
	"github.com/linode/linodego"
	"github.com/stretchr/testify/assert"
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
	t.Parallel()

	t.Run("vpcID in cache", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		vpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		got, err := GetVPCID(context.TODO(), client, "test3")
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
		got, err := GetVPCID(context.TODO(), client, "test4")
		assert.Error(t, err)
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
		got, err := GetVPCID(context.TODO(), client, "test4")
		assert.ErrorIs(t, err, vpcLookupError{"test4"})
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
		got, err := GetVPCID(context.TODO(), client, "test4")
		assert.NoError(t, err)
		if got != 4 {
			t.Errorf("GetVPCID() = %v, want %v", got, 4)
		}
	})
}

func TestGetVPCIPAddresses(t *testing.T) {
	t.Parallel()

	t.Run("vpc id not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		vpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCs(gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPC{}, nil)
		_, err := GetVPCIPAddresses(context.TODO(), client, "test4")
		assert.Error(t, err)
	})

	t.Run("vpc id found but listing ip addresses fails with 404 error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		vpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCIP{}, &linodego.Error{Code: http.StatusNotFound, Message: "[404] [label] VPC not found"})
		_, err := GetVPCIPAddresses(context.TODO(), client, "test3")
		assert.Error(t, err)
		_, exists := vpcIDs["test3"]
		assert.False(t, exists, "test3 key should get deleted from vpcIDs map")
	})

	t.Run("vpc id found but listing ip addresses fails with 500 error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockClient(ctrl)
		vpcIDs = map[string]int{"test1": 1, "test2": 2, "test3": 3}
		client.EXPECT().ListVPCIPAddresses(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]linodego.VPCIP{}, &linodego.Error{Code: http.StatusInternalServerError, Message: "[500] [label] Internal Server Error"})
		_, err := GetVPCIPAddresses(context.TODO(), client, "test1")
		assert.Error(t, err)
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
		_, err := GetVPCIPAddresses(context.TODO(), client, "test10")
		assert.NoError(t, err)
		_, exists := vpcIDs["test10"]
		assert.True(t, exists, "test10 key should be present in vpcIDs map")
	})
}

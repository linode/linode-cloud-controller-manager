package linode

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/linode/linode-cloud-controller-manager/cloud/annotations"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client/mocks"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/options"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/services"
	"github.com/linode/linodego/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAllocateNodeBalancerBackendIPv4Range(t *testing.T) {
	tests := []struct {
		name        string
		backendCIDR string
		subnet      *linodego.VPCSubnet
		want        string
		wantErr     string
	}{
		{
			name:        "selects the first available range",
			backendCIDR: "10.63.88.0/21",
			subnet: &linodego.VPCSubnet{
				IPv4: "10.63.80.0/20",
			},
			want: "10.63.88.0/30",
		},
		{
			name:        "skips assigned ranges",
			backendCIDR: "10.63.88.0/21",
			subnet: &linodego.VPCSubnet{
				IPv4: "10.63.80.0/20",
				Nodebalancers: []linodego.VPCSubnetNodebalancers{
					{ID: 1, Ipv4Range: "10.63.88.0/30"},
					{ID: 2, Ipv4Range: "10.63.88.4/30"},
				},
			},
			want: "10.63.88.8/30",
		},
		{
			name:        "supports non RFC1918 prefixes",
			backendCIDR: "100.96.0.0/26",
			subnet: &linodego.VPCSubnet{
				IPv4: "100.96.0.0/24",
			},
			want: "100.96.0.0/30",
		},
		{
			name:        "rejects a reserved range already in use",
			backendCIDR: "10.63.88.0/21",
			subnet: &linodego.VPCSubnet{
				IPv4: "10.63.80.0/20",
				Nodebalancers: []linodego.VPCSubnetNodebalancers{
					{ID: 42, Ipv4Range: "10.63.95.252/30"},
				},
			},
			wantErr: "reserved NodeBalancer backend range 10.63.95.252/30 is already assigned to NodeBalancer 42",
		},
		{
			name:        "rejects a range outside the VPC subnet",
			backendCIDR: "10.64.0.0/21",
			subnet: &linodego.VPCSubnet{
				IPv4: "10.63.80.0/20",
			},
			wantErr: "NodeBalancer backend CIDR 10.64.0.0/21 is not within VPC subnet 10.63.80.0/20",
		},
		{
			name:        "rejects a backend with only the reserved range",
			backendCIDR: "10.63.95.252/30",
			subnet: &linodego.VPCSubnet{
				IPv4: "10.63.80.0/20",
			},
			wantErr: "NodeBalancer backend CIDR 10.63.95.252/30 has no allocable /30 after reserving its highest /30",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reserved, reservedErr := highestNodeBalancerBackendIPv4Range(tt.backendCIDR)
			if reservedErr != nil {
				if tt.wantErr != "" {
					require.EqualError(t, reservedErr, tt.wantErr)
					return
				}
				require.NoError(t, reservedErr)
			}
			got, err := allocateNodeBalancerBackendIPv4Range(tt.backendCIDR, reserved.String(), tt.subnet)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHighestNodeBalancerBackendIPv4Range(t *testing.T) {
	got, err := highestNodeBalancerBackendIPv4Range("10.63.88.0/21")
	require.NoError(t, err)
	assert.Equal(t, "10.63.95.252/30", got.String())
}

func TestValidateNodeBalancerBackendIPv4Reservation(t *testing.T) {
	require.NoError(t, validateNodeBalancerBackendIPv4Reservation("10.63.88.0/21", "10.63.95.252/30"))
	require.EqualError(
		t,
		validateNodeBalancerBackendIPv4Reservation("10.63.88.0/21", "10.63.95.248/30"),
		"reserved NodeBalancer backend range 10.63.95.248/30 must be the highest /30 in 10.63.88.0/21 (10.63.95.252/30)",
	)
}

func TestGetVPCCreateOptionsWithReservedBackendRange(t *testing.T) {
	previousVPCNames := options.Options.VPCNames
	previousSubnetNames := options.Options.SubnetNames
	previousBackendSubnet := options.Options.NodeBalancerBackendIPv4Subnet
	previousReservedRange := options.Options.NodeBalancerBackendIPv4ReservedRange
	previousBackendSubnetID := options.Options.NodeBalancerBackendIPv4SubnetID
	t.Cleanup(func() {
		options.Options.VPCNames = previousVPCNames
		options.Options.SubnetNames = previousSubnetNames
		options.Options.NodeBalancerBackendIPv4Subnet = previousBackendSubnet
		options.Options.NodeBalancerBackendIPv4ReservedRange = previousReservedRange
		options.Options.NodeBalancerBackendIPv4SubnetID = previousBackendSubnetID
	})

	tests := []struct {
		name        string
		annotations map[string]string
		subnetID    int
	}{
		{
			name: "default subnet",
		},
		{
			name: "service subnet override",
			annotations: map[string]string{
				annotations.NodeBalancerBackendSubnetName: "test-subnet",
			},
		},
		{
			name:     "configured subnet ID",
			subnetID: 456,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			linodeClient := mocks.NewMockClient(ctrl)

			options.Options.VPCNames = []string{"test-vpc"}
			options.Options.SubnetNames = []string{"test-subnet"}
			options.Options.NodeBalancerBackendIPv4Subnet = "10.63.88.0/21"
			options.Options.NodeBalancerBackendIPv4ReservedRange = "10.63.95.252/30"
			options.Options.NodeBalancerBackendIPv4SubnetID = tt.subnetID

			services.Mu.Lock()
			services.VpcIDs = map[string]int{"test-vpc": 123}
			services.SubnetIDs = map[string]int{"test-subnet": 456}
			services.Mu.Unlock()

			linodeClient.EXPECT().
				GetVPCSubnet(gomock.Any(), 123, 456).
				Return(&linodego.VPCSubnet{
					ID:   456,
					IPv4: "10.63.80.0/20",
					Nodebalancers: []linodego.VPCSubnetNodebalancers{
						{ID: 1, Ipv4Range: "10.63.88.0/30"},
					},
				}, nil)

			l := &loadbalancers{client: linodeClient}
			got, err := l.getVPCCreateOptions(context.Background(), &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotations,
				},
			})
			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, 456, got[0].SubnetID)
			assert.Equal(t, "10.63.88.4/30", got[0].IPv4Range)
			assert.False(t, got[0].IPv4RangeAutoAssign)
		})
	}
}

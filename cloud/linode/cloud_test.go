package linode

import (
	"reflect"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cloudprovider "k8s.io/cloud-provider"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client/mocks"
)

func TestNewCloudRouteControllerDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Setenv("LINODE_API_TOKEN", "dummyapitoken")
	t.Setenv("LINODE_REGION", "us-east")
	t.Setenv("LINODE_REQUEST_TIMEOUT_SECONDS", "10")

	t.Run("should not fail if vpc is empty and routecontroller is disabled", func(t *testing.T) {
		Options.VPCName = ""
		Options.EnableRouteController = false
		_, err := newCloud()
		assert.NoError(t, err)
	})

	t.Run("fail if vpcname is empty and routecontroller is enabled", func(t *testing.T) {
		Options.VPCName = ""
		Options.EnableRouteController = true
		_, err := newCloud()
		assert.Error(t, err)
	})
}

func TestNewCloud(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Setenv("LINODE_API_TOKEN", "dummyapitoken")
	t.Setenv("LINODE_REGION", "us-east")
	t.Setenv("LINODE_REQUEST_TIMEOUT_SECONDS", "10")
	t.Setenv("LINODE_ROUTES_CACHE_TTL_SECONDS", "60")
	Options.LinodeGoDebug = true

	t.Run("should fail if api token is empty", func(t *testing.T) {
		t.Setenv("LINODE_API_TOKEN", "")
		_, err := newCloud()
		assert.Error(t, err, "expected error when api token is empty")
	})

	t.Run("should fail if region is empty", func(t *testing.T) {
		t.Setenv("LINODE_REGION", "")
		_, err := newCloud()
		assert.Error(t, err, "expected error when linode region is empty")
	})

	t.Run("should fail if both vpcname and vpcnames are set", func(t *testing.T) {
		Options.VPCName = "tt"
		Options.VPCNames = "tt"
		defer func() {
			Options.VPCName = ""
			Options.VPCNames = ""
		}()
		_, err := newCloud()
		assert.Error(t, err, "expected error when both vpcname and vpcnames are set")
	})

	t.Run("should not fail if deprecated vpcname is set", func(t *testing.T) {
		Options.VPCName = "tt"
		defer func() {
			Options.VPCName = ""
			Options.VPCNames = ""
		}()
		_, err := newCloud()
		require.NoError(t, err, "expected no error if deprecated flag vpcname is set")
		assert.Equal(t, "tt", Options.VPCNames, "expected vpcnames to be set to vpcname")
	})

	t.Run("should fail if both nodeBalancerBackendIPv4SubnetID and nodeBalancerBackendIPv4SubnetName are set", func(t *testing.T) {
		Options.VPCNames = "tt"
		Options.NodeBalancerBackendIPv4SubnetID = 12345
		Options.NodeBalancerBackendIPv4SubnetName = "test-subnet"
		defer func() {
			Options.VPCNames = ""
			Options.NodeBalancerBackendIPv4SubnetID = 0
			Options.NodeBalancerBackendIPv4SubnetName = ""
		}()
		_, err := newCloud()
		assert.Error(t, err, "expected error when both nodeBalancerBackendIPv4SubnetID and nodeBalancerBackendIPv4SubnetName are set")
	})

	t.Run("should fail if incorrect loadbalancertype is set", func(t *testing.T) {
		rtEnabled := Options.EnableRouteController
		Options.EnableRouteController = false
		Options.LoadBalancerType = "test"
		Options.VPCNames = "vpc-test1,vpc-test2"
		Options.NodeBalancerBackendIPv4SubnetName = "t1"
		vpcIDs = map[string]int{"vpc-test1": 1, "vpc-test2": 2, "vpc-test3": 3}
		subnetIDs = map[string]int{"t1": 1, "t2": 2, "t3": 3}
		defer func() {
			Options.LoadBalancerType = ""
			Options.EnableRouteController = rtEnabled
			Options.VPCNames = ""
			Options.NodeBalancerBackendIPv4SubnetID = 0
			Options.NodeBalancerBackendIPv4SubnetName = ""
			vpcIDs = map[string]int{}
			subnetIDs = map[string]int{}
		}()
		_, err := newCloud()
		assert.Error(t, err, "expected error if incorrect loadbalancertype is set")
	})

	t.Run("should fail if ipholdersuffix is longer than 23 chars", func(t *testing.T) {
		suffix := Options.IpHolderSuffix
		Options.IpHolderSuffix = strings.Repeat("a", 24)
		rtEnabled := Options.EnableRouteController
		Options.EnableRouteController = false
		defer func() {
			Options.IpHolderSuffix = suffix
			Options.EnableRouteController = rtEnabled
		}()
		_, err := newCloud()
		assert.Error(t, err, "expected error if ipholdersuffix is longer than 23 chars")
	})
}

func Test_linodeCloud_LoadBalancer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)
	type fields struct {
		client        *mocks.MockClient
		instances     cloudprovider.InstancesV2
		loadbalancers cloudprovider.LoadBalancer
		routes        cloudprovider.Routes
	}
	tests := []struct {
		name   string
		fields fields
		want   cloudprovider.LoadBalancer
		want1  bool
	}{
		{
			name: "should return loadbalancer interface",
			fields: fields{
				client:        client,
				instances:     newInstances(client),
				loadbalancers: newLoadbalancers(client, "us-east"),
				routes:        nil,
			},
			want:  newLoadbalancers(client, "us-east"),
			want1: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &linodeCloud{
				client:        tt.fields.client,
				instances:     tt.fields.instances,
				loadbalancers: tt.fields.loadbalancers,
				routes:        tt.fields.routes,
			}
			got, got1 := c.LoadBalancer()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("linodeCloud.LoadBalancer() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("linodeCloud.LoadBalancer() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_linodeCloud_InstancesV2(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)
	type fields struct {
		client        *mocks.MockClient
		instances     cloudprovider.InstancesV2
		loadbalancers cloudprovider.LoadBalancer
		routes        cloudprovider.Routes
	}
	tests := []struct {
		name   string
		fields fields
		want   cloudprovider.InstancesV2
		want1  bool
	}{
		{
			name: "should return instances interface",
			fields: fields{
				client:        client,
				instances:     newInstances(client),
				loadbalancers: newLoadbalancers(client, "us-east"),
				routes:        nil,
			},
			want:  newInstances(client),
			want1: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &linodeCloud{
				client:        tt.fields.client,
				instances:     tt.fields.instances,
				loadbalancers: tt.fields.loadbalancers,
				routes:        tt.fields.routes,
			}
			got, got1 := c.InstancesV2()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("linodeCloud.InstancesV2() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("linodeCloud.InstancesV2() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_linodeCloud_Instances(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)
	type fields struct {
		client        *mocks.MockClient
		instances     cloudprovider.InstancesV2
		loadbalancers cloudprovider.LoadBalancer
		routes        cloudprovider.Routes
	}
	tests := []struct {
		name   string
		fields fields
		want   cloudprovider.Instances
		want1  bool
	}{
		{
			name: "should return nil",
			fields: fields{
				client:        client,
				instances:     newInstances(client),
				loadbalancers: newLoadbalancers(client, "us-east"),
				routes:        nil,
			},
			want:  nil,
			want1: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &linodeCloud{
				client:        tt.fields.client,
				instances:     tt.fields.instances,
				loadbalancers: tt.fields.loadbalancers,
				routes:        tt.fields.routes,
			}
			got, got1 := c.Instances()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("linodeCloud.Instances() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("linodeCloud.Instances() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_linodeCloud_Zones(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)
	type fields struct {
		client        *mocks.MockClient
		instances     cloudprovider.InstancesV2
		loadbalancers cloudprovider.LoadBalancer
		routes        cloudprovider.Routes
	}
	tests := []struct {
		name   string
		fields fields
		want   cloudprovider.Zones
		want1  bool
	}{
		{
			name: "should return nil",
			fields: fields{
				client:        client,
				instances:     newInstances(client),
				loadbalancers: newLoadbalancers(client, "us-east"),
				routes:        nil,
			},
			want:  nil,
			want1: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &linodeCloud{
				client:        tt.fields.client,
				instances:     tt.fields.instances,
				loadbalancers: tt.fields.loadbalancers,
				routes:        tt.fields.routes,
			}
			got, got1 := c.Zones()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("linodeCloud.Zones() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("linodeCloud.Zones() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_linodeCloud_Clusters(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)
	type fields struct {
		client        *mocks.MockClient
		instances     cloudprovider.InstancesV2
		loadbalancers cloudprovider.LoadBalancer
		routes        cloudprovider.Routes
	}
	tests := []struct {
		name   string
		fields fields
		want   cloudprovider.Clusters
		want1  bool
	}{
		{
			name: "should return nil",
			fields: fields{
				client:        client,
				instances:     newInstances(client),
				loadbalancers: newLoadbalancers(client, "us-east"),
				routes:        nil,
			},
			want:  nil,
			want1: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &linodeCloud{
				client:        tt.fields.client,
				instances:     tt.fields.instances,
				loadbalancers: tt.fields.loadbalancers,
				routes:        tt.fields.routes,
			}
			got, got1 := c.Clusters()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("linodeCloud.Clusters() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("linodeCloud.Clusters() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_linodeCloud_Routes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)
	r := &routes{}
	type fields struct {
		client                *mocks.MockClient
		instances             cloudprovider.InstancesV2
		loadbalancers         cloudprovider.LoadBalancer
		routes                cloudprovider.Routes
		EnableRouteController bool
	}
	tests := []struct {
		name   string
		fields fields
		want   cloudprovider.Routes
		want1  bool
	}{
		{
			name: "should return nil",
			fields: fields{
				client:                client,
				instances:             newInstances(client),
				loadbalancers:         newLoadbalancers(client, "us-east"),
				routes:                r,
				EnableRouteController: false,
			},
			want:  nil,
			want1: false,
		},
		{
			name: "should return routes interface",
			fields: fields{
				client:                client,
				instances:             newInstances(client),
				loadbalancers:         newLoadbalancers(client, "us-east"),
				routes:                r,
				EnableRouteController: true,
			},
			want:  r,
			want1: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &linodeCloud{
				client:        tt.fields.client,
				instances:     tt.fields.instances,
				loadbalancers: tt.fields.loadbalancers,
				routes:        tt.fields.routes,
			}
			rt := Options.EnableRouteController
			defer func() { Options.EnableRouteController = rt }()
			Options.EnableRouteController = tt.fields.EnableRouteController
			got, got1 := c.Routes()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("linodeCloud.Routes() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("linodeCloud.Routes() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_linodeCloud_ProviderName(t *testing.T) {
	type fields struct {
		client        *mocks.MockClient
		instances     cloudprovider.InstancesV2
		loadbalancers cloudprovider.LoadBalancer
		routes        cloudprovider.Routes
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "should return linode",
			fields: fields{
				client:        nil,
				instances:     nil,
				loadbalancers: nil,
				routes:        nil,
			},
			want: ProviderName,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &linodeCloud{
				client:        tt.fields.client,
				instances:     tt.fields.instances,
				loadbalancers: tt.fields.loadbalancers,
				routes:        tt.fields.routes,
			}
			if got := c.ProviderName(); got != tt.want {
				t.Errorf("linodeCloud.ProviderName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_linodeCloud_ScrubDNS(t *testing.T) {
	type fields struct {
		client        *mocks.MockClient
		instances     cloudprovider.InstancesV2
		loadbalancers cloudprovider.LoadBalancer
		routes        cloudprovider.Routes
	}
	type args struct {
		in0 []string
		in1 []string
	}
	tests := []struct {
		name        string
		fields      fields
		args        args
		wantNsOut   []string
		wantSrchOut []string
	}{
		{
			name: "should return linode",
			fields: fields{
				client:        nil,
				instances:     nil,
				loadbalancers: nil,
				routes:        nil,
			},
			wantNsOut:   nil,
			wantSrchOut: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &linodeCloud{
				client:        tt.fields.client,
				instances:     tt.fields.instances,
				loadbalancers: tt.fields.loadbalancers,
				routes:        tt.fields.routes,
			}
			gotNsOut, gotSrchOut := c.ScrubDNS(tt.args.in0, tt.args.in1)
			if !reflect.DeepEqual(gotNsOut, tt.wantNsOut) {
				t.Errorf("linodeCloud.ScrubDNS() gotNsOut = %v, want %v", gotNsOut, tt.wantNsOut)
			}
			if !reflect.DeepEqual(gotSrchOut, tt.wantSrchOut) {
				t.Errorf("linodeCloud.ScrubDNS() gotSrchOut = %v, want %v", gotSrchOut, tt.wantSrchOut)
			}
		})
	}
}

func Test_linodeCloud_HasClusterID(t *testing.T) {
	type fields struct {
		client        *mocks.MockClient
		instances     cloudprovider.InstancesV2
		loadbalancers cloudprovider.LoadBalancer
		routes        cloudprovider.Routes
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "should return true",
			fields: fields{
				client:        nil,
				instances:     nil,
				loadbalancers: nil,
				routes:        nil,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &linodeCloud{
				client:        tt.fields.client,
				instances:     tt.fields.instances,
				loadbalancers: tt.fields.loadbalancers,
				routes:        tt.fields.routes,
			}
			if got := c.HasClusterID(); got != tt.want {
				t.Errorf("linodeCloud.HasClusterID() = %v, want %v", got, tt.want)
			}
		})
	}
}

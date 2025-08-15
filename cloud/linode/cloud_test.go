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
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/options"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/services"
)

func TestNewCloudRouteControllerDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Setenv("LINODE_API_TOKEN", "dummyapitoken")
	t.Setenv("LINODE_REGION", "us-east")
	t.Setenv("LINODE_REQUEST_TIMEOUT_SECONDS", "10")
	options.Options.NodeBalancerPrefix = "ccm"

	t.Run("should not fail if vpc is empty and routecontroller is disabled", func(t *testing.T) {
		options.Options.VPCNames = []string{}
		options.Options.EnableRouteController = false
		_, err := newCloud()
		assert.NoError(t, err)
	})

	t.Run("fail if vpcname is empty and routecontroller is enabled", func(t *testing.T) {
		options.Options.VPCNames = []string{}
		options.Options.EnableRouteController = true
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
	t.Setenv("LINODE_URL", "https://api.linode.com/v4")
	options.Options.LinodeGoDebug = true
	options.Options.NodeBalancerPrefix = "ccm"

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

	t.Run("should fail if both nodeBalancerBackendIPv4SubnetID and nodeBalancerBackendIPv4SubnetName are set", func(t *testing.T) {
		options.Options.VPCNames = []string{"tt"}
		options.Options.NodeBalancerBackendIPv4SubnetID = 12345
		options.Options.NodeBalancerBackendIPv4SubnetName = "test-subnet"
		defer func() {
			options.Options.VPCNames = []string{}
			options.Options.NodeBalancerBackendIPv4SubnetID = 0
			options.Options.NodeBalancerBackendIPv4SubnetName = ""
		}()
		_, err := newCloud()
		assert.Error(t, err, "expected error when both nodeBalancerBackendIPv4SubnetID and nodeBalancerBackendIPv4SubnetName are set")
	})

	t.Run("should fail if incorrect loadbalancertype is set", func(t *testing.T) {
		rtEnabled := options.Options.EnableRouteController
		options.Options.EnableRouteController = false
		options.Options.LoadBalancerType = "test"
		options.Options.VPCNames = []string{"vpc-test1", "vpc-test2"}
		options.Options.NodeBalancerBackendIPv4SubnetName = "t1"
		services.VpcIDs = map[string]int{"vpc-test1": 1, "vpc-test2": 2, "vpc-test3": 3}
		services.SubnetIDs = map[string]int{"t1": 1, "t2": 2, "t3": 3}
		defer func() {
			options.Options.LoadBalancerType = ""
			options.Options.EnableRouteController = rtEnabled
			options.Options.VPCNames = []string{}
			options.Options.NodeBalancerBackendIPv4SubnetID = 0
			options.Options.NodeBalancerBackendIPv4SubnetName = ""
			services.VpcIDs = map[string]int{}
			services.SubnetIDs = map[string]int{}
		}()
		_, err := newCloud()
		assert.Error(t, err, "expected error if incorrect loadbalancertype is set")
	})

	t.Run("should fail if ipholdersuffix is longer than 23 chars", func(t *testing.T) {
		suffix := options.Options.IpHolderSuffix
		options.Options.IpHolderSuffix = strings.Repeat("a", 24)
		rtEnabled := options.Options.EnableRouteController
		options.Options.EnableRouteController = false
		defer func() {
			options.Options.IpHolderSuffix = suffix
			options.Options.EnableRouteController = rtEnabled
		}()
		_, err := newCloud()
		assert.Error(t, err, "expected error if ipholdersuffix is longer than 23 chars")
	})

	t.Run("should fail if nodebalancer-prefix is longer than 19 chars", func(t *testing.T) {
		prefix := options.Options.NodeBalancerPrefix
		rtEnabled := options.Options.EnableRouteController
		options.Options.EnableRouteController = false
		options.Options.LoadBalancerType = "nodebalancer"
		options.Options.NodeBalancerPrefix = strings.Repeat("a", 21)
		defer func() {
			options.Options.NodeBalancerPrefix = prefix
			options.Options.LoadBalancerType = ""
			options.Options.EnableRouteController = rtEnabled
		}()
		_, err := newCloud()
		t.Log(err)
		require.Error(t, err, "expected error if nodebalancer-prefix is longer than 19 chars")
		require.ErrorContains(t, err, "nodebalancer-prefix")
	})

	t.Run("should fail if nodebalancer-prefix is empty", func(t *testing.T) {
		prefix := options.Options.NodeBalancerPrefix
		rtEnabled := options.Options.EnableRouteController
		options.Options.EnableRouteController = false
		options.Options.LoadBalancerType = "nodebalancer"
		options.Options.NodeBalancerPrefix = ""
		defer func() {
			options.Options.NodeBalancerPrefix = prefix
			options.Options.LoadBalancerType = ""
			options.Options.EnableRouteController = rtEnabled
		}()
		_, err := newCloud()
		t.Log(err)
		require.Error(t, err, "expected error if nodebalancer-prefix is empty")
		require.ErrorContains(t, err, "nodebalancer-prefix must be no empty")
	})

	t.Run("should fail if not validated nodebalancer-prefix", func(t *testing.T) {
		prefix := options.Options.NodeBalancerPrefix
		rtEnabled := options.Options.EnableRouteController
		options.Options.EnableRouteController = false
		options.Options.LoadBalancerType = "nodebalancer"
		options.Options.NodeBalancerPrefix = "\\+x"
		defer func() {
			options.Options.NodeBalancerPrefix = prefix
			options.Options.LoadBalancerType = ""
			options.Options.EnableRouteController = rtEnabled
		}()
		_, err := newCloud()
		t.Log(err)
		require.Error(t, err, "expected error if not validated nodebalancer-prefix")
		require.ErrorContains(t, err, "nodebalancer-prefix must be no empty and use only letters, numbers, underscores, and dashes")
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
				instances:     services.NewInstances(client),
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
				instances:     services.NewInstances(client),
				loadbalancers: newLoadbalancers(client, "us-east"),
				routes:        nil,
			},
			want:  services.NewInstances(client),
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
				instances:     services.NewInstances(client),
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
				instances:     services.NewInstances(client),
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
				instances:     services.NewInstances(client),
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
				instances:             services.NewInstances(client),
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
				instances:             services.NewInstances(client),
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
			rt := options.Options.EnableRouteController
			defer func() { options.Options.EnableRouteController = rt }()
			options.Options.EnableRouteController = tt.fields.EnableRouteController
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

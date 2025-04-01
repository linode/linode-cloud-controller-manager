/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// This file holds the code related with the sample nodeipamcontroller
// which demonstrates how cloud providers add external controllers to cloud-controller-manager

package linode

import (
	"net"
	"reflect"
	"testing"

	"k8s.io/client-go/informers"
	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	cloudprovider "k8s.io/cloud-provider"
)

func Test_setNodeCIDRMaskSizes(t *testing.T) {
	type args struct {
		clusterCIDRs []*net.IPNet
		ipv4NetMask  int
		ipv6NetMask  int
	}
	_, ipv4Net, _ := net.ParseCIDR("10.192.0.0/10")
	_, ipv6Net, _ := net.ParseCIDR("fd00::/56")
	tests := []struct {
		name string
		args args
		want []int
	}{
		{
			name: "empty cluster cidrs",
			args: args{
				clusterCIDRs: []*net.IPNet{},
			},
			want: []int{},
		},
		{
			name: "single cidr",
			args: args{
				clusterCIDRs: []*net.IPNet{
					{
						IP:   ipv4Net.IP,
						Mask: ipv4Net.Mask,
					},
				},
			},
			want: []int{defaultNodeMaskCIDRIPv4},
		},
		{
			name: "two cidrs",
			args: args{
				clusterCIDRs: []*net.IPNet{
					{
						IP:   ipv4Net.IP,
						Mask: ipv4Net.Mask,
					},
					{
						IP:   ipv6Net.IP,
						Mask: ipv6Net.Mask,
					},
				},
			},
			want: []int{defaultNodeMaskCIDRIPv4, defaultNodeMaskCIDRIPv6},
		},
		{
			name: "two cidrs with custom mask sizes",
			args: args{
				clusterCIDRs: []*net.IPNet{
					{
						IP:   ipv4Net.IP,
						Mask: ipv4Net.Mask,
					},
					{
						IP:   ipv6Net.IP,
						Mask: ipv6Net.Mask,
					},
				},
				ipv4NetMask: 25,
				ipv6NetMask: 80,
			},
			want: []int{25, 80},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldCIDRMaskSizeIPv4 := Options.NodeCIDRMaskSizeIPv4
			oldCIDRMaskSizeIPv6 := Options.NodeCIDRMaskSizeIPv6
			defer func() {
				Options.NodeCIDRMaskSizeIPv4 = oldCIDRMaskSizeIPv4
				Options.NodeCIDRMaskSizeIPv6 = oldCIDRMaskSizeIPv6
			}()
			if tt.args.ipv4NetMask != 0 {
				Options.NodeCIDRMaskSizeIPv4 = tt.args.ipv4NetMask
			}
			if tt.args.ipv6NetMask != 0 {
				Options.NodeCIDRMaskSizeIPv6 = tt.args.ipv6NetMask
			}
			got := setNodeCIDRMaskSizes(tt.args.clusterCIDRs)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("setNodeCIDRMaskSizes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_processCIDRs(t *testing.T) {
	type args struct {
		cidrsList string
	}
	_, ipv4Net, _ := net.ParseCIDR("10.192.0.0/10")
	_, ipv6Net, _ := net.ParseCIDR("fd00::/56")
	tests := []struct {
		name        string
		args        args
		want        []*net.IPNet
		ipv6Enabled bool
		wantErr     bool
	}{
		{
			name: "empty cidr list",
			args: args{
				cidrsList: "",
			},
			want:        nil,
			ipv6Enabled: false,
			wantErr:     true,
		},
		{
			name: "valid ipv4 cidr",
			args: args{
				cidrsList: "10.192.0.0/10",
			},
			want: []*net.IPNet{
				{
					IP:   ipv4Net.IP,
					Mask: ipv4Net.Mask,
				},
			},
			ipv6Enabled: false,
			wantErr:     false,
		},
		{
			name: "valid ipv4 and ipv6 cidrs",
			args: args{
				cidrsList: "10.192.0.0/10,fd00::/56",
			},
			want: []*net.IPNet{
				{
					IP:   ipv4Net.IP,
					Mask: ipv4Net.Mask,
				},
				{
					IP:   ipv6Net.IP,
					Mask: ipv6Net.Mask,
				},
			},
			ipv6Enabled: true,
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := processCIDRs(tt.args.cidrsList)
			if (err != nil) != tt.wantErr {
				t.Errorf("processCIDRs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("processCIDRs() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.ipv6Enabled {
				t.Errorf("processCIDRs() got1 = %v, want %v", got1, tt.ipv6Enabled)
			}
		})
	}
}

func Test_startNodeIpamController(t *testing.T) {
	type args struct {
		stopCh            <-chan struct{}
		cloud             cloudprovider.Interface
		nodeInformer      v1.NodeInformer
		kubeclient        kubernetes.Interface
		allocateNodeCIDRs bool
		clusterCIDR       string
	}
	kubeClient := fake.NewSimpleClientset()
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "allocate-node-cidrs not set",
			args: args{
				stopCh:            make(<-chan struct{}),
				cloud:             nil,
				nodeInformer:      nil,
				kubeclient:        nil,
				allocateNodeCIDRs: false,
				clusterCIDR:       "",
			},
			wantErr: false,
		},
		{
			name: "incorrect cluster-cidrs specified",
			args: args{
				stopCh:            make(<-chan struct{}),
				cloud:             nil,
				nodeInformer:      nil,
				kubeclient:        nil,
				allocateNodeCIDRs: true,
				clusterCIDR:       "10.10.10.10",
			},
			wantErr: true,
		},
		{
			name: "more than one ipv4 cidrs specified",
			args: args{
				stopCh:            make(<-chan struct{}),
				cloud:             nil,
				nodeInformer:      nil,
				kubeclient:        nil,
				allocateNodeCIDRs: true,
				clusterCIDR:       "10.192.0.0/10,192.168.0.0/16",
			},
			wantErr: true,
		},
		{
			name: "more than two cidrs specified",
			args: args{
				stopCh:            make(<-chan struct{}),
				cloud:             nil,
				nodeInformer:      nil,
				kubeclient:        nil,
				allocateNodeCIDRs: true,
				clusterCIDR:       "10.192.0.0/10,fd00::/80,192.168.0.0/16",
			},
			wantErr: true,
		},
		{
			name: "correct cidrs specified",
			args: args{
				stopCh:            make(<-chan struct{}),
				cloud:             nil,
				nodeInformer:      informers.NewSharedInformerFactory(kubeClient, 0).Core().V1().Nodes(),
				kubeclient:        kubeClient,
				allocateNodeCIDRs: true,
				clusterCIDR:       "10.192.0.0/10",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		currAllocateNodeCIDRs := Options.AllocateNodeCIDRs
		currClusterCIDR := Options.ClusterCIDRIPv4
		defer func() {
			Options.AllocateNodeCIDRs = currAllocateNodeCIDRs
			Options.ClusterCIDRIPv4 = currClusterCIDR
		}()
		Options.AllocateNodeCIDRs = tt.args.allocateNodeCIDRs
		Options.ClusterCIDRIPv4 = tt.args.clusterCIDR
		t.Run(tt.name, func(t *testing.T) {
			if err := startNodeIpamController(tt.args.stopCh, tt.args.cloud, tt.args.nodeInformer, tt.args.kubeclient); (err != nil) != tt.wantErr {
				t.Errorf("startNodeIpamController() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

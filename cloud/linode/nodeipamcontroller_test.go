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

	"github.com/linode/linode-cloud-controller-manager/cloud/linode/options"
)

func Test_setNodeCIDRMaskSizes(t *testing.T) {
	type args struct {
		ipv4NetMask int
		ipv6NetMask int
	}
	tests := []struct {
		name string
		args args
		want []int
	}{
		{
			name: "default cidr mask sizes",
			args: args{},
			want: []int{defaultNodeMaskCIDRIPv4, defaultNodeMaskCIDRIPv6},
		},
		{
			name: "two cidrs with custom mask sizes",
			args: args{
				ipv4NetMask: 25,
				ipv6NetMask: 80,
			},
			want: []int{25, 80},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldCIDRMaskSizeIPv4 := options.Options.NodeCIDRMaskSizeIPv4
			oldCIDRMaskSizeIPv6 := options.Options.NodeCIDRMaskSizeIPv6
			defer func() {
				options.Options.NodeCIDRMaskSizeIPv4 = oldCIDRMaskSizeIPv4
				options.Options.NodeCIDRMaskSizeIPv6 = oldCIDRMaskSizeIPv6
			}()
			if tt.args.ipv4NetMask != 0 {
				options.Options.NodeCIDRMaskSizeIPv4 = tt.args.ipv4NetMask
			}
			if tt.args.ipv6NetMask != 0 {
				options.Options.NodeCIDRMaskSizeIPv6 = tt.args.ipv6NetMask
			}
			got := setNodeCIDRMaskSizes()
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
	tests := []struct {
		name    string
		args    args
		want    []*net.IPNet
		wantErr bool
	}{
		{
			name: "empty cidr list",
			args: args{
				cidrsList: "",
			},
			want:    nil,
			wantErr: true,
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
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processCIDRs(tt.args.cidrsList)
			if (err != nil) != tt.wantErr {
				t.Errorf("processCIDRs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("processCIDRs() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_startNodeIpamController(t *testing.T) {
	type args struct {
		stopCh            <-chan struct{}
		cloud             linodeCloud
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
				cloud:             linodeCloud{},
				nodeInformer:      nil,
				kubeclient:        nil,
				allocateNodeCIDRs: false,
				clusterCIDR:       "",
			},
			wantErr: false,
		},
		{
			name: "allocate-node-cidrs set but cluster-cidr not set",
			args: args{
				stopCh:            make(<-chan struct{}),
				cloud:             linodeCloud{},
				nodeInformer:      nil,
				kubeclient:        nil,
				allocateNodeCIDRs: true,
				clusterCIDR:       "",
			},
			wantErr: true,
		},
		{
			name: "incorrect cluster-cidr specified",
			args: args{
				stopCh:            make(<-chan struct{}),
				cloud:             linodeCloud{},
				nodeInformer:      nil,
				kubeclient:        nil,
				allocateNodeCIDRs: true,
				clusterCIDR:       "10.10.10.10",
			},
			wantErr: true,
		},
		{
			name: "ipv6 cidr specified",
			args: args{
				stopCh:            make(<-chan struct{}),
				cloud:             linodeCloud{},
				nodeInformer:      nil,
				kubeclient:        nil,
				allocateNodeCIDRs: true,
				clusterCIDR:       "fd00::/80",
			},
			wantErr: true,
		},
		{
			name: "more than one cidr specified",
			args: args{
				stopCh:            make(<-chan struct{}),
				cloud:             linodeCloud{},
				nodeInformer:      nil,
				kubeclient:        nil,
				allocateNodeCIDRs: true,
				clusterCIDR:       "10.192.0.0/10,fd00::/80",
			},
			wantErr: true,
		},
		{
			name: "correct cidrs specified",
			args: args{
				stopCh:            make(<-chan struct{}),
				cloud:             linodeCloud{},
				nodeInformer:      informers.NewSharedInformerFactory(kubeClient, 0).Core().V1().Nodes(),
				kubeclient:        kubeClient,
				allocateNodeCIDRs: true,
				clusterCIDR:       "10.192.0.0/10",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		currAllocateNodeCIDRs := options.Options.AllocateNodeCIDRs
		currClusterCIDR := options.Options.ClusterCIDRIPv4
		defer func() {
			options.Options.AllocateNodeCIDRs = currAllocateNodeCIDRs
			options.Options.ClusterCIDRIPv4 = currClusterCIDR
		}()
		options.Options.AllocateNodeCIDRs = tt.args.allocateNodeCIDRs
		options.Options.ClusterCIDRIPv4 = tt.args.clusterCIDR
		t.Run(tt.name, func(t *testing.T) {
			if err := startNodeIpamController(tt.args.stopCh, &tt.args.cloud, tt.args.nodeInformer, tt.args.kubeclient); (err != nil) != tt.wantErr {
				t.Errorf("startNodeIpamController() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

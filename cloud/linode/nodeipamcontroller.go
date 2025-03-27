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
	"fmt"
	"net"
	"strings"

	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	cloudprovider "k8s.io/cloud-provider"
	netutils "k8s.io/utils/net"

	nodeipamcontroller "github.com/linode/linode-cloud-controller-manager/cloud/nodeipam"
	"github.com/linode/linode-cloud-controller-manager/cloud/nodeipam/ipam"
)

const (
	// defaultNodeMaskCIDRIPv4 is default mask size for IPv4 node cidr
	defaultNodeMaskCIDRIPv4 = 24
	// defaultNodeMaskCIDRIPv6 is default mask size for IPv6 node cidr
	defaultNodeMaskCIDRIPv6 = 64
)

func startNodeIpamController(stopCh <-chan struct{}, cloud cloudprovider.Interface, nodeInformer v1.NodeInformer, kubeclient kubernetes.Interface) (bool, error) {
	var serviceCIDR *net.IPNet
	var secondaryServiceCIDR *net.IPNet

	// should we start nodeIPAM
	if !Options.EnableNodeCIDRAllocation {
		return false, nil
	}

	// failure: bad cidrs in config
	clusterCIDRs, dualStack, err := processCIDRs(Options.ClusterIPv4CIDR)
	if err != nil {
		return false, fmt.Errorf("processCIDRs failed: %v", err)
	}

	// failure: more than one cidr but they are not configured as dual stack
	if len(clusterCIDRs) > 1 && !dualStack {
		return false, fmt.Errorf("len of ClusterCIDRs==%v and they are not configured as dual stack (at least one from each IPFamily", len(clusterCIDRs))
	}

	// failure: more than cidrs is not allowed even with dual stack
	if len(clusterCIDRs) > 2 {
		return false, fmt.Errorf("len of clusters is:%v > more than max allowed of 2", len(clusterCIDRs))
	}

	nodeCIDRMaskSizes, err := setNodeCIDRMaskSizes(clusterCIDRs)
	if err != nil {
		return false, fmt.Errorf("setNodeCIDRMaskSizes failed: %v", err)
	}

	nodeIpamController, err := nodeipamcontroller.NewNodeIpamController(
		nodeInformer,
		cloud,
		kubeclient,
		clusterCIDRs,
		serviceCIDR,
		secondaryServiceCIDR,
		nodeCIDRMaskSizes,
		ipam.CIDRAllocatorType(ipam.RangeAllocatorType),
	)
	if err != nil {
		return true, err
	}
	go nodeIpamController.Run(stopCh)
	return true, nil
}

// processCIDRs is a helper function that works on a comma separated cidrs and returns
// a list of typed cidrs
// a flag if cidrs represents a dual stack
// error if failed to parse any of the cidrs
func processCIDRs(cidrsList string) ([]*net.IPNet, bool, error) {
	cidrsSplit := strings.Split(strings.TrimSpace(cidrsList), ",")

	cidrs, err := netutils.ParseCIDRs(cidrsSplit)
	if err != nil {
		return nil, false, err
	}

	// if cidrs has an error then the previous call will fail
	// safe to ignore error checking on next call
	dualstack, _ := netutils.IsDualStackCIDRs(cidrs)

	return cidrs, dualstack, nil
}

func setNodeCIDRMaskSizes(clusterCIDRs []*net.IPNet) ([]int, error) {
	if Options.NodeCIDRMaskSizeIPv4 != 0 {
		return []int{Options.NodeCIDRMaskSizeIPv4, defaultNodeMaskCIDRIPv6}, nil
	}
	return []int{defaultNodeMaskCIDRIPv4, defaultNodeMaskCIDRIPv6}, nil
}

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

	"k8s.io/apimachinery/pkg/util/wait"
	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	cloudprovider "k8s.io/cloud-provider"
	nodeipamcontroller "k8s.io/kubernetes/pkg/controller/nodeipam"
	"k8s.io/kubernetes/pkg/controller/nodeipam/ipam"
	netutils "k8s.io/utils/net"
)

const (
	maxAllowedNodeCIDRs = 2
)

var (
	// defaultNodeMaskCIDRIPv4 is default mask size for IPv4 node cidr
	defaultNodeMaskCIDRIPv4 = 24
	// defaultNodeMaskCIDRIPv6 is default mask size for IPv6 node cidr
	defaultNodeMaskCIDRIPv6 = 64
)

func startNodeIpamController(stopCh <-chan struct{}, cloud cloudprovider.Interface, nodeInformer v1.NodeInformer, kubeclient kubernetes.Interface) error {
	var serviceCIDR *net.IPNet
	var secondaryServiceCIDR *net.IPNet

	// should we start nodeIPAM
	if !Options.AllocateNodeCIDRs {
		return nil
	}

	// failure: bad cidrs in config
	clusterCIDRs, dualStack, err := processCIDRs(Options.ClusterCIDRIPv4)
	if err != nil {
		return fmt.Errorf("processCIDRs failed: %w", err)
	}

	// failure: more than one cidr but they are not configured as dual stack
	if len(clusterCIDRs) > 1 && !dualStack {
		return fmt.Errorf("len of ClusterCIDRs==%v and they are not configured as dual stack (at least one from each IPFamily", len(clusterCIDRs))
	}

	// failure: more than cidrs is not allowed even with dual stack
	if len(clusterCIDRs) > maxAllowedNodeCIDRs {
		return fmt.Errorf("len of clusters is:%v > more than max allowed of %d", len(clusterCIDRs), maxAllowedNodeCIDRs)
	}

	/* TODO: uncomment and fix if we want to support service cidr overlap with nodecidr
	// service cidr processing
	if len(strings.TrimSpace(nodeIPAMConfig.ServiceCIDR)) != 0 {
		_, serviceCIDR, err = netutils.ParseCIDRSloppy(nodeIPAMConfig.ServiceCIDR)
		if err != nil {
			klog.ErrorS(err, "Unsuccessful parsing of service CIDR", "CIDR", nodeIPAMConfig.ServiceCIDR)
		}
	}

	if len(strings.TrimSpace(nodeIPAMConfig.SecondaryServiceCIDR)) != 0 {
		_, secondaryServiceCIDR, err = netutils.ParseCIDRSloppy(nodeIPAMConfig.SecondaryServiceCIDR)
		if err != nil {
			klog.ErrorS(err, "Unsuccessful parsing of service CIDR", "CIDR", nodeIPAMConfig.SecondaryServiceCIDR)
		}
	}

	// the following checks are triggered if both serviceCIDR and secondaryServiceCIDR are provided
	if serviceCIDR != nil && secondaryServiceCIDR != nil {
		// should be dual stack (from different IPFamilies)
		dualstackServiceCIDR, err := netutils.IsDualStackCIDRs([]*net.IPNet{serviceCIDR, secondaryServiceCIDR})
		if err != nil {
			return nil, false, fmt.Errorf("failed to perform dualstack check on serviceCIDR and secondaryServiceCIDR error:%v", err)
		}
		if !dualstackServiceCIDR {
			return nil, false, fmt.Errorf("serviceCIDR and secondaryServiceCIDR are not dualstack (from different IPfamiles)")
		}
	}
	*/

	nodeCIDRMaskSizes := setNodeCIDRMaskSizes(clusterCIDRs)

	ctx := wait.ContextForChannel(stopCh)

	nodeIpamController, err := nodeipamcontroller.NewNodeIpamController(
		ctx,
		nodeInformer,
		cloud,
		kubeclient,
		clusterCIDRs,
		serviceCIDR,
		secondaryServiceCIDR,
		nodeCIDRMaskSizes,
		ipam.RangeAllocatorType,
	)
	if err != nil {
		return err
	}

	go nodeIpamController.Run(ctx)
	return nil
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
	dualstack, err := netutils.IsDualStackCIDRs(cidrs)
	if err != nil {
		return nil, false, fmt.Errorf("failed to perform dualstack check on cidrs: %w", err)
	}

	return cidrs, dualstack, nil
}

func setNodeCIDRMaskSizes(clusterCIDRs []*net.IPNet) []int {
	sortedSizes := func(maskSizeIPv4, maskSizeIPv6 int) []int {
		nodeMaskCIDRs := make([]int, len(clusterCIDRs))

		for idx, clusterCIDR := range clusterCIDRs {
			if netutils.IsIPv6CIDR(clusterCIDR) {
				nodeMaskCIDRs[idx] = maskSizeIPv6
			} else {
				nodeMaskCIDRs[idx] = maskSizeIPv4
			}
		}
		return nodeMaskCIDRs
	}

	if Options.NodeCIDRMaskSizeIPv4 != 0 {
		defaultNodeMaskCIDRIPv4 = Options.NodeCIDRMaskSizeIPv4
	}
	if Options.NodeCIDRMaskSizeIPv6 != 0 {
		defaultNodeMaskCIDRIPv6 = Options.NodeCIDRMaskSizeIPv6
	}
	return sortedSizes(defaultNodeMaskCIDRIPv4, defaultNodeMaskCIDRIPv6)
}

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
	netutils "k8s.io/utils/net"

	nodeipamcontroller "github.com/linode/linode-cloud-controller-manager/cloud/nodeipam"
	"github.com/linode/linode-cloud-controller-manager/cloud/nodeipam/ipam"
)

const (
	maxAllowedNodeCIDRsIPv4 = 1
)

var (
	// defaultNodeMaskCIDRIPv4 is default mask size for IPv4 node cidr
	defaultNodeMaskCIDRIPv4 = 24
	// defaultNodeMaskCIDRIPv6 is default mask size for IPv6 node cidr
	defaultNodeMaskCIDRIPv6 = 112
)

func startNodeIpamController(stopCh <-chan struct{}, cloud cloudprovider.Interface, nodeInformer v1.NodeInformer, kubeclient kubernetes.Interface) error {
	var serviceCIDR *net.IPNet
	var secondaryServiceCIDR *net.IPNet

	// should we start nodeIPAM
	if !Options.AllocateNodeCIDRs {
		return nil
	}

	// failure: bad cidrs in config
	clusterCIDRs, err := processCIDRs(Options.ClusterCIDRIPv4)
	if err != nil {
		return fmt.Errorf("processCIDRs failed: %w", err)
	}

	if len(clusterCIDRs) > maxAllowedNodeCIDRsIPv4 {
		return fmt.Errorf("too many clusterCIDRs specified for ipv4, max allowed is %d", maxAllowedNodeCIDRsIPv4)
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

	nodeCIDRMaskSizes := setNodeCIDRMaskSizes()

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
		ipam.CloudAllocatorType,
	)
	if err != nil {
		return err
	}

	go nodeIpamController.Run(ctx)
	return nil
}

// processCIDR is a helper function that works on cidr and returns a list of typed cidrs
// error if failed to parse the cidr
func processCIDRs(cidrsList string) ([]*net.IPNet, error) {
	cidrsSplit := strings.Split(strings.TrimSpace(cidrsList), ",")

	cidrs, err := netutils.ParseCIDRs(cidrsSplit)
	if err != nil {
		return nil, err
	}

	return cidrs, nil
}

func setNodeCIDRMaskSizes() []int {
	if Options.NodeCIDRMaskSizeIPv4 != 0 {
		defaultNodeMaskCIDRIPv4 = Options.NodeCIDRMaskSizeIPv4
	}
	if Options.NodeCIDRMaskSizeIPv6 != 0 {
		defaultNodeMaskCIDRIPv6 = Options.NodeCIDRMaskSizeIPv6
	}
	return []int{defaultNodeMaskCIDRIPv4, defaultNodeMaskCIDRIPv6}
}

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

func startNodeIpamController(stopCh <-chan struct{}, cloud *linodeCloud, nodeInformer v1.NodeInformer, kubeclient kubernetes.Interface) error {
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

	if len(clusterCIDRs) == 0 {
		return fmt.Errorf("no clusterCIDR specified. Must specify --cluster-cidr if --allocate-node-cidrs is set")
	}

	if len(clusterCIDRs) > maxAllowedNodeCIDRsIPv4 {
		return fmt.Errorf("too many clusterCIDRs specified for ipv4, max allowed is %d", maxAllowedNodeCIDRsIPv4)
	}

	if clusterCIDRs[0].IP.To4() == nil {
		return fmt.Errorf("clusterCIDR %s is not ipv4", clusterCIDRs[0].String())
	}

	nodeCIDRMaskSizes := setNodeCIDRMaskSizes()

	ctx := wait.ContextForChannel(stopCh)

	nodeIpamController, err := nodeipamcontroller.NewNodeIpamController(
		ctx,
		nodeInformer,
		cloud,
		cloud.client,
		kubeclient,
		clusterCIDRs,
		serviceCIDR,
		secondaryServiceCIDR,
		nodeCIDRMaskSizes,
		ipam.CloudAllocatorType,
		Options.DisableIPv6NodeCIDRAllocation,
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

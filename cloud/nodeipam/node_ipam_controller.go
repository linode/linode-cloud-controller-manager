/*
Copyright 2014 The Kubernetes Authors.

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

package nodeipam

import (
	"context"
	"fmt"
	"net"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	cloudprovider "k8s.io/cloud-provider"
	controllersmetrics "k8s.io/component-base/metrics/prometheus/controllers"
	"k8s.io/klog/v2"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client"
	"github.com/linode/linode-cloud-controller-manager/cloud/nodeipam/ipam"
)

// Controller is the controller that manages node ipam state.
type Controller struct {
	allocatorType ipam.CIDRAllocatorType

	cloud                cloudprovider.Interface
	linodeClient         client.Client
	clusterCIDRs         []*net.IPNet
	serviceCIDR          *net.IPNet
	secondaryServiceCIDR *net.IPNet
	kubeClient           clientset.Interface
	eventBroadcaster     record.EventBroadcaster

	nodeLister         corelisters.NodeLister
	nodeInformerSynced cache.InformerSynced

	cidrAllocator ipam.CIDRAllocator
}

// NewNodeIpamController returns a new node IP Address Management controller to
// sync instances from cloudprovider.
// This method returns an error if it is unable to initialize the CIDR bitmap with
// podCIDRs it has already allocated to nodes. Since we don't allow podCIDR changes
// currently, this should be handled as a fatal error.
func NewNodeIpamController(
	ctx context.Context,
	nodeInformer coreinformers.NodeInformer,
	cloud cloudprovider.Interface,
	linodeClient client.Client,
	kubeClient clientset.Interface,
	clusterCIDRs []*net.IPNet,
	serviceCIDR *net.IPNet,
	secondaryServiceCIDR *net.IPNet,
	nodeCIDRMaskSizes []int,
	allocatorType ipam.CIDRAllocatorType,
	disableIPv6NodeCIDRAllocation bool,
) (*Controller, error) {
	if kubeClient == nil {
		return nil, fmt.Errorf("kubeClient is nil when starting Controller")
	}

	if len(clusterCIDRs) == 0 {
		return nil, fmt.Errorf("Controller: Must specify --cluster-cidr if --allocate-node-cidrs is set")
	}

	for idx, cidr := range clusterCIDRs {
		mask := cidr.Mask
		if maskSize, _ := mask.Size(); maskSize > nodeCIDRMaskSizes[idx] {
			return nil, fmt.Errorf("Controller: Invalid --cluster-cidr, mask size of cluster CIDR must be less than or equal to --node-cidr-mask-size configured for CIDR family")
		}
	}

	ic := &Controller{
		cloud:                cloud,
		linodeClient:         linodeClient,
		kubeClient:           kubeClient,
		eventBroadcaster:     record.NewBroadcaster(record.WithContext(ctx)),
		clusterCIDRs:         clusterCIDRs,
		serviceCIDR:          serviceCIDR,
		secondaryServiceCIDR: secondaryServiceCIDR,
		allocatorType:        allocatorType,
	}

	var err error

	allocatorParams := ipam.CIDRAllocatorParams{
		ClusterCIDRs:                  clusterCIDRs,
		ServiceCIDR:                   ic.serviceCIDR,
		SecondaryServiceCIDR:          ic.secondaryServiceCIDR,
		NodeCIDRMaskSizes:             nodeCIDRMaskSizes,
		DisableIPv6NodeCIDRAllocation: disableIPv6NodeCIDRAllocation,
	}

	ic.cidrAllocator, err = ipam.New(ctx, ic.linodeClient, kubeClient, cloud, nodeInformer, ic.allocatorType, allocatorParams)
	if err != nil {
		return nil, err
	}

	ic.nodeLister = nodeInformer.Lister()
	ic.nodeInformerSynced = nodeInformer.Informer().HasSynced

	return ic, nil
}

// Run starts an asynchronous loop that monitors the status of cluster nodes.
func (nc *Controller) Run(ctx context.Context) {
	defer utilruntime.HandleCrash()

	// Start event processing pipeline.
	nc.eventBroadcaster.StartStructuredLogging(3)
	nc.eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: nc.kubeClient.CoreV1().Events("")})
	defer nc.eventBroadcaster.Shutdown()
	klog.FromContext(ctx).Info("Starting ipam controller")
	defer klog.FromContext(ctx).Info("Shutting down ipam controller")

	if !cache.WaitForNamedCacheSync("node", ctx.Done(), nc.nodeInformerSynced) {
		return
	}

	go nc.cidrAllocator.Run(ctx)

	<-ctx.Done()
}

// RunWithMetrics is a wrapper for Run that also tracks starting and stopping of the nodeipam controller with additional metric
func (nc *Controller) RunWithMetrics(ctx context.Context, controllerManagerMetrics *controllersmetrics.ControllerManagerMetrics) {
	controllerManagerMetrics.ControllerStarted("nodeipam")
	defer controllerManagerMetrics.ControllerStopped("nodeipam")
	nc.Run(ctx)
}

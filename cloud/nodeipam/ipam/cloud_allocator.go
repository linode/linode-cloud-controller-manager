/*
Copyright 2016 The Kubernetes Authors.

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

package ipam

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	informers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	nodeutil "k8s.io/component-helpers/node/util"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/controller/nodeipam/ipam/cidrset"
	controllerutil "k8s.io/kubernetes/pkg/controller/util/node"
	netutils "k8s.io/utils/net"

	linode "github.com/linode/linode-cloud-controller-manager/cloud/linode/client"
)

type cloudAllocator struct {
	client clientset.Interface

	linodeClient linode.Client
	// cluster cidr as passed in during controller creation for ipv4 addresses
	clusterCIDR *net.IPNet
	// for clusterCIDR we maintain what is used and what is not
	cidrSet *cidrset.CidrSet
	// nodeLister is able to list/get nodes and is populated by the shared informer passed to controller
	nodeLister corelisters.NodeLister
	// nodesSynced returns true if the node shared informer has been synced at least once.
	nodesSynced cache.InformerSynced
	broadcaster record.EventBroadcaster
	recorder    record.EventRecorder

	// queues are where incoming work is placed to de-dup and to allow "easy"
	// rate limited requeues on errors
	queue workqueue.TypedRateLimitingInterface[any]

	// nodeCIDRMaskSizeIPv6 is the mask size for the IPv6 CIDR assigned to nodes.
	nodeCIDRMaskSizeIPv6 int
	// disableIPv6NodeCIDRAllocation is true if we should not allocate IPv6 CIDRs for nodes.
	disableIPv6NodeCIDRAllocation bool
}

const (
	providerIDPrefix    = "linode://"
	ipv6BitLen          = 128
	ipv6PodCIDRMaskSize = 112
)

var _ CIDRAllocator = &cloudAllocator{}

// NewLinodeCIDRAllocator returns a CIDRAllocator to allocate CIDRs for node
// Caller must ensure subNetMaskSize is not less than cluster CIDR mask size.
// Caller must always pass in a list of existing nodes so the new allocator.
// Caller must ensure that ClusterCIDR is semantically correct
// can initialize its CIDR map. NodeList is only nil in testing.
func NewLinodeCIDRAllocator(ctx context.Context, linodeClient linode.Client, client clientset.Interface, nodeInformer informers.NodeInformer, allocatorParams CIDRAllocatorParams, nodeList *v1.NodeList) (CIDRAllocator, error) {
	logger := klog.FromContext(ctx)
	if client == nil {
		logger.Error(nil, "kubeClient is nil when starting CIDRAllocator")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	eventBroadcaster := record.NewBroadcaster(record.WithContext(ctx))
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "cidrAllocator"})

	// create a cidrSet for ipv4 cidr we operate on
	cidrSet, err := cidrset.NewCIDRSet(allocatorParams.ClusterCIDRs[0], allocatorParams.NodeCIDRMaskSizes[0])
	if err != nil {
		return nil, err
	}

	ca := &cloudAllocator{
		client:                        client,
		linodeClient:                  linodeClient,
		clusterCIDR:                   allocatorParams.ClusterCIDRs[0],
		cidrSet:                       cidrSet,
		nodeLister:                    nodeInformer.Lister(),
		nodesSynced:                   nodeInformer.Informer().HasSynced,
		broadcaster:                   eventBroadcaster,
		recorder:                      recorder,
		queue:                         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[any](), "cidrallocator_node"),
		nodeCIDRMaskSizeIPv6:          allocatorParams.NodeCIDRMaskSizes[1],
		disableIPv6NodeCIDRAllocation: allocatorParams.DisableIPv6NodeCIDRAllocation,
	}

	if allocatorParams.ServiceCIDR != nil {
		ca.filterOutServiceRange(logger, allocatorParams.ServiceCIDR)
	} else {
		logger.Info("No Service CIDR provided. Skipping filtering out service addresses")
	}

	if allocatorParams.SecondaryServiceCIDR != nil {
		ca.filterOutServiceRange(logger, allocatorParams.SecondaryServiceCIDR)
	} else {
		logger.Info("No Secondary Service CIDR provided. Skipping filtering out secondary service addresses")
	}

	if nodeList != nil {
		for _, node := range nodeList.Items {
			if len(node.Spec.PodCIDRs) == 0 {
				logger.V(4).Info("Node has no CIDR, ignoring", "node", klog.KObj(&node))
				continue
			}
			logger.V(4).Info("Node has CIDR, occupying it in CIDR map", "node", klog.KObj(&node), "podCIDR", node.Spec.PodCIDR)
			if err := ca.occupyCIDRs(ctx, &node); err != nil {
				// This will happen if:
				// 1. We find garbage in the podCIDRs field. Retrying is useless.
				// 2. CIDR out of range: This means a node CIDR has changed.
				// This error will keep crashing controller-manager.
				return nil, err
			}
		}
	}

	if _, err := nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				ca.queue.Add(key)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err == nil {
				ca.queue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			// The informer cache no longer has the object, and since Node doesn't have a finalizer,
			// we don't see the Update with DeletionTimestamp != 0.
			node, ok := obj.(*v1.Node)
			if !ok {
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					utilruntime.HandleError(fmt.Errorf("unexpected object type: %v", obj))
					return
				}
				node, ok = tombstone.Obj.(*v1.Node)
				if !ok {
					utilruntime.HandleError(fmt.Errorf("unexpected object types: %v", obj))
					return
				}
			}
			if err := ca.ReleaseCIDR(logger, node); err != nil {
				utilruntime.HandleError(fmt.Errorf("error while processing CIDR Release: %w", err))
			}
		},
	}); err != nil {
		logger.Error(err, "Failed to add event handler to node informer")
		return nil, err
	}

	return ca, nil
}

func (c *cloudAllocator) Run(ctx context.Context) {
	defer utilruntime.HandleCrash()

	// Start event processing pipeline.
	c.broadcaster.StartStructuredLogging(3)
	logger := klog.FromContext(ctx)
	logger.Info("Sending events to api server")
	c.broadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: c.client.CoreV1().Events("")})
	defer c.broadcaster.Shutdown()

	defer c.queue.ShutDown()

	logger.Info("Starting linode's cloud CIDR allocator")
	defer logger.Info("Shutting down linode's cloud CIDR allocator")

	if !cache.WaitForNamedCacheSync("cidrallocator", ctx.Done(), c.nodesSynced) {
		return
	}

	for i := 0; i < cidrUpdateWorkers; i++ {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}

	<-ctx.Done()
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// queue.
func (c *cloudAllocator) runWorker(ctx context.Context) {
	for c.processNextNodeWorkItem(ctx) {
	}
}

// processNextWorkItem will read a single work item off the queue and
// attempt to process it, by calling the syncHandler.
func (c *cloudAllocator) processNextNodeWorkItem(ctx context.Context) bool {
	obj, shutdown := c.queue.Get()
	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer r.queue.Done.
	err := func(logger klog.Logger, obj interface{}) error {
		// We call Done here so the workNodeQueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the queue and attempted again after a back-off
		// period.
		defer c.queue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workNodeQueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workNodeQueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workNodeQueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workNodeQueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.queue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workNodeQueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// Foo resource to be synced.
		if err := c.syncNode(ctx, key); err != nil {
			// Put the item back on the queue to handle any transient errors.
			c.queue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queue again until another change happens.
		c.queue.Forget(obj)
		logger.V(4).Info("Successfully synced", "key", key)
		return nil
	}(klog.FromContext(ctx), obj)
	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

func (c *cloudAllocator) syncNode(ctx context.Context, key string) error {
	logger := klog.FromContext(ctx)
	startTime := time.Now()
	defer func() {
		logger.V(4).Info("Finished syncing Node request", "node", key, "elapsed", time.Since(startTime))
	}()

	node, err := c.nodeLister.Get(key)
	if apierrors.IsNotFound(err) {
		logger.V(3).Info("node has been deleted", "node", key)
		// TODO: obtain the node object information to call ReleaseCIDR from here
		// and retry if there is an error.
		return nil
	}
	if err != nil {
		return err
	}
	// Check the DeletionTimestamp to determine if object is under deletion.
	if !node.DeletionTimestamp.IsZero() {
		logger.V(3).Info("node is being deleted", "node", key)
		return nil
	}
	return c.AllocateOrOccupyCIDR(ctx, node)
}

// marks node.PodCIDRs[...] as used in allocator's tracked cidrSet
func (c *cloudAllocator) occupyCIDRs(ctx context.Context, node *v1.Node) error {
	if len(node.Spec.PodCIDRs) == 0 {
		return nil
	}
	logger := klog.FromContext(ctx)
	for idx, cidr := range node.Spec.PodCIDRs {
		_, podCIDR, err := netutils.ParseCIDRSloppy(cidr)
		if err != nil {
			return fmt.Errorf("failed to parse node %s, CIDR %s", node.Name, cidr)
		}
		// IPv6 CIDRs are allocated from node-specific ranges
		// We don't track them in the cidrSet
		if podCIDR.IP.To4() == nil {
			logger.V(4).Info("Nothing to occupy for IPv6 CIDR", "cidr", podCIDR)
			return nil
		}
		// If node has a pre allocate cidr that does not exist in our cidrs.
		// This will happen if cluster went from dualstack(multi cidrs) to non-dualstack
		// then we have now way of locking it
		if idx >= 1 {
			return fmt.Errorf("node:%s has an allocated cidr: %v at index:%v that does not exist in cluster cidrs configuration", node.Name, cidr, idx)
		}

		if err := c.cidrSet.Occupy(podCIDR); err != nil {
			return fmt.Errorf("failed to mark cidr[%v] at idx [%v] as occupied for node: %v: %w", podCIDR, idx, node.Name, err)
		}
	}
	return nil
}

// getIPv6RangeFromInterface extracts the IPv6 range from a Linode instance configuration interface.
func getIPv6RangeFromInterface(iface linodego.InstanceConfigInterface) string {
	if ipv6 := iface.IPv6; ipv6 != nil {
		if len(ipv6.SLAAC) > 0 {
			return ipv6.SLAAC[0].Range
		}
		if len(ipv6.Ranges) > 0 {
			return ipv6.Ranges[0].Range
		}
	}
	return ""
}

func getIPv6RangeFromLinodeInterface(iface linodego.LinodeInterface) string {
	if len(iface.VPC.IPv6.SLAAC) > 0 {
		return iface.VPC.IPv6.SLAAC[0].Range
	}
	if len(iface.VPC.IPv6.Ranges) > 0 {
		return iface.VPC.IPv6.Ranges[0].Range
	}
	return ""
}

// getIPv6PodCIDR attempts to compute a stable IPv6 /112 PodCIDR
// within the provided base IPv6 range using the mnemonic subprefix :0:c::/112.
//
// The mnemonic subprefix :0:c::/112 is constructed by setting hextets 5..7 to 0, c, 0
// (i.e., bytes 8-13 set to 00 00 00 0c 00 00) within the /64 base, as implemented below.
//
// Rules:
//   - For a /64 base, return {base64}:0:c::/112
//   - Only applies when desiredMask is /112 and the result is fully contained
//     in the base range. Otherwise, returns (nil, false) to signal fallback.
func getIPv6PodCIDR(ip net.IP, desiredMask int) (*net.IPNet, bool) {
	// Some validation checks
	if ip == nil || desiredMask != ipv6PodCIDRMaskSize {
		return nil, false
	}

	// We need to make a copy so we don't mutate caller's backing array (net.IP is a slice)
	ipCopy := make(net.IP, len(ip))
	copy(ipCopy, ip)

	// Keep first 64 bits (bytes 0..7) and set hextets 5..7 to 0, c, 0 respectively
	// Hextet index to byte mapping: h5->[8,9], h6->[10,11], h7->[12,13]
	ipCopy[8], ipCopy[9] = 0x00, 0x00   // :0
	ipCopy[10], ipCopy[11] = 0x00, 0x0c // :c
	ipCopy[12], ipCopy[13] = 0x00, 0x00 // :0
	// last hextet (bytes 14..15) will be zeroed by mask below

	podMask := net.CIDRMask(desiredMask, ipv6BitLen)
	// Ensure the address is the network address for the desired mask
	ipCopy = ipCopy.Mask(podMask)
	podCIDR := &net.IPNet{IP: ipCopy, Mask: podMask}

	return podCIDR, true
}

// allocateIPv6CIDR allocates an IPv6 CIDR for the given node.
// It retrieves the instance configuration for the node and extracts the IPv6 range.
// It then creates a new net.IPNet with the IPv6 address and mask size defined
// by nodeCIDRMaskSizeIPv6. The function returns an error if it fails to retrieve
// the instance configuration or parse the IPv6 range.
func (c *cloudAllocator) allocateIPv6CIDR(ctx context.Context, node *v1.Node) (*net.IPNet, error) {
	logger := klog.FromContext(ctx)

	if node.Spec.ProviderID == "" {
		return nil, fmt.Errorf("node %s has no ProviderID set, cannot calculate ipv6 range for it", node.Name)
	}
	// Extract the Linode ID from the ProviderID
	if !strings.HasPrefix(node.Spec.ProviderID, providerIDPrefix) {
		return nil, fmt.Errorf("node %s has invalid ProviderID %s, expected prefix '%s'", node.Name, node.Spec.ProviderID, providerIDPrefix)
	}
	// Parse the Linode ID from the ProviderID
	id, err := strconv.Atoi(strings.TrimPrefix(node.Spec.ProviderID, providerIDPrefix))
	if err != nil {
		return nil, fmt.Errorf("failed to parse Linode ID from ProviderID %s: %w", node.Spec.ProviderID, err)
	}

	// fetch the instance so we can determine which interface generation to use
	instance, err := c.linodeClient.GetInstance(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed get linode with id %d: %w", id, err)
	}
	ipv6Range := ""
	if instance.InterfaceGeneration == linodego.GenerationLinode {
		ifaces, listErr := c.linodeClient.ListInterfaces(ctx, id, &linodego.ListOptions{})
		if listErr != nil || len(ifaces) == 0 {
			return nil, fmt.Errorf("failed to list interfaces: %w", listErr)
		}
		for _, iface := range ifaces {
			if iface.VPC != nil {
				ipv6Range = getIPv6RangeFromLinodeInterface(iface)
				if ipv6Range != "" {
					break
				}
			}
		}

		if ipv6Range == "" {
			return nil, fmt.Errorf("failed to find ipv6 range in Linode interfaces: %v", ifaces)
		}
	} else {
		// Retrieve the instance configuration for the Linode ID
		configs, listErr := c.linodeClient.ListInstanceConfigs(ctx, id, &linodego.ListOptions{})
		if listErr != nil || len(configs) == 0 {
			return nil, fmt.Errorf("failed to list instance configs: %w", listErr)
		}

		for _, iface := range configs[0].Interfaces {
			if iface.Purpose == linodego.InterfacePurposeVPC {
				ipv6Range = getIPv6RangeFromInterface(iface)
				if ipv6Range != "" {
					break
				}
			}
		}

		if ipv6Range == "" {
			return nil, fmt.Errorf("failed to find ipv6 range in instance config: %v", configs[0])
		}
	}

	ip, base, err := net.ParseCIDR(ipv6Range)
	if err != nil {
		return nil, fmt.Errorf("failed parsing ipv6 range %s: %w", ipv6Range, err)
	}

	// get pod cidr using stable mnemonic subprefix :0:c::/112
	if podCIDR, ok := getIPv6PodCIDR(ip, c.nodeCIDRMaskSizeIPv6); ok {
		logger.V(4).Info("Using stable IPv6 PodCIDR subprefix :0:c::/112", "ip", ip, "podCIDR", podCIDR)
		// Verify the /112 PodCIDR is fully contained within the base /64 range
		if !base.Contains(podCIDR.IP) {
			return nil, fmt.Errorf("stable IPv6 PodCIDR %s is not contained in base range %s", podCIDR, base)
		}
		return podCIDR, nil
	}

	// Fallback to the original behavior: mask base IP directly to desired size
	podMask := net.CIDRMask(c.nodeCIDRMaskSizeIPv6, 128)
	fallbackPodCIDR := &net.IPNet{IP: ip.Mask(podMask), Mask: podMask}
	logger.V(4).Info("Falling back to start-of-range IPv6 PodCIDR", "ip", ip, "podCIDR", fallbackPodCIDR)
	return fallbackPodCIDR, nil
}

// WARNING: If you're adding any return calls or defer any more work from this
// function you have to make sure to update nodesInProcessing properly with the
// disposition of the node when the work is done.
func (c *cloudAllocator) AllocateOrOccupyCIDR(ctx context.Context, node *v1.Node) error {
	if node == nil {
		return nil
	}

	if len(node.Spec.PodCIDRs) > 0 {
		return c.occupyCIDRs(ctx, node)
	}

	logger := klog.FromContext(ctx)
	allocatedCIDRs := make([]*net.IPNet, 2)

	podCIDR, err := c.cidrSet.AllocateNext()
	if err != nil {
		controllerutil.RecordNodeStatusChange(logger, c.recorder, node, "CIDRNotAvailable")
		return fmt.Errorf("failed to allocate cidr from cluster cidr: %w", err)
	}
	allocatedCIDRs[0] = podCIDR

	// If IPv6 CIDR allocation is disabled, log and return early.
	if c.disableIPv6NodeCIDRAllocation {
		logger.V(4).Info("IPv6 CIDR allocation disabled; using only IPv4", "node", klog.KObj(node))
		// remove the second CIDR from the allocatedCIDRs slice
		// since we are not allocating IPv6 CIDR
		allocatedCIDRs = allocatedCIDRs[:1]
		return c.enqueueCIDRUpdate(ctx, node.Name, allocatedCIDRs)
	}
	// Allocate IPv6 CIDR for the node.
	logger.V(4).Info("Allocating IPv6 CIDR", "node", klog.KObj(node))
	if allocatedCIDRs[1], err = c.allocateIPv6CIDR(ctx, node); err != nil {
		return fmt.Errorf("failed to assign IPv6 CIDR: %w", err)
	}

	return c.enqueueCIDRUpdate(ctx, node.Name, allocatedCIDRs)
}

// enqueueCIDRUpdate adds the node name and CIDRs to the work queue for processing.
func (c *cloudAllocator) enqueueCIDRUpdate(ctx context.Context, nodeName string, cidrs []*net.IPNet) error {
	logger := klog.FromContext(ctx)
	logger.V(4).Info("Putting node with CIDR into the work queue", "node", nodeName, "CIDR", cidrs)
	return c.updateCIDRsAllocation(ctx, nodeName, cidrs)
}

// ReleaseCIDR marks node.podCIDRs[...] as unused in our tracked cidrSets
func (c *cloudAllocator) ReleaseCIDR(logger klog.Logger, node *v1.Node) error {
	if node == nil || len(node.Spec.PodCIDRs) == 0 {
		return nil
	}

	for idx, cidr := range node.Spec.PodCIDRs {
		_, podCIDR, err := netutils.ParseCIDRSloppy(cidr)
		if err != nil {
			return fmt.Errorf("failed to parse CIDR %s on Node %v: %w", cidr, node.Name, err)
		}
		if podCIDR.IP.To4() == nil {
			logger.V(4).Info("Nothing to release for IPv6 CIDR", "cidr", podCIDR)
			continue
		}

		// If node has a pre allocate cidr that does not exist in our cidrs.
		// This will happen if cluster went from dualstack(multi cidrs) to non-dualstack
		// then we have now way of locking it
		if idx >= 1 {
			return fmt.Errorf("node:%s has an allocated cidr: %v at index:%v that does not exist in cluster cidrs configuration", node.Name, cidr, idx)
		}

		logger.V(4).Info("Release CIDR for node", "CIDR", cidr, "node", klog.KObj(node))
		if err = c.cidrSet.Release(podCIDR); err != nil {
			return fmt.Errorf("error when releasing CIDR %v: %w", cidr, err)
		}
	}
	return nil
}

// Marks all CIDRs with subNetMaskSize that belongs to serviceCIDR as used across all cidrs
// so that they won't be assignable.
func (c *cloudAllocator) filterOutServiceRange(logger klog.Logger, serviceCIDR *net.IPNet) {
	// Checks if service CIDR has a nonempty intersection with cluster
	// CIDR. It is the case if either clusterCIDR contains serviceCIDR with
	// clusterCIDR's Mask applied (this means that clusterCIDR contains
	// serviceCIDR) or vice versa (which means that serviceCIDR contains
	// clusterCIDR).
	// if they don't overlap then ignore the filtering
	if !c.clusterCIDR.Contains(serviceCIDR.IP.Mask(c.clusterCIDR.Mask)) && !serviceCIDR.Contains(c.clusterCIDR.IP.Mask(serviceCIDR.Mask)) {
		return
	}

	// at this point, len(cidrSet) == len(clusterCidr)
	if err := c.cidrSet.Occupy(serviceCIDR); err != nil {
		logger.Error(err, "Error filtering out service cidr out cluster cidr", "CIDR", c.clusterCIDR, "serviceCIDR", serviceCIDR)
	}
}

// updateCIDRsAllocation assigns CIDR to Node and sends an update to the API server.
func (c *cloudAllocator) updateCIDRsAllocation(ctx context.Context, nodeName string, allocatedCIDRs []*net.IPNet) error {
	var err error
	var node *v1.Node
	logger := klog.FromContext(ctx)
	cidrsString := ipnetToStringList(allocatedCIDRs)
	node, err = c.nodeLister.Get(nodeName)
	if err != nil {
		logger.Error(err, "Failed while getting node for updating Node.Spec.PodCIDRs", "node", klog.KRef("", nodeName))
		return err
	}

	// if cidr list matches the proposed.
	// then we possibly updated this node
	// and just failed to ack the success.
	if len(node.Spec.PodCIDRs) == len(allocatedCIDRs) {
		match := true
		for idx, cidr := range cidrsString {
			if node.Spec.PodCIDRs[idx] != cidr {
				match = false
				break
			}
		}
		if match {
			logger.V(4).Info("Node already has allocated CIDR. It matches the proposed one", "node", klog.KObj(node), "CIDRs", allocatedCIDRs)
			return nil
		}
	}

	// node has cidrs, release the reserved
	if len(node.Spec.PodCIDRs) != 0 {
		logger.Error(nil, "Node already has a CIDR allocated. Releasing the new one", "node", klog.KObj(node), "podCIDRs", node.Spec.PodCIDRs)
		if releaseErr := c.cidrSet.Release(allocatedCIDRs[0]); releaseErr != nil {
			logger.Error(releaseErr, "Error when releasing CIDR", "CIDR", allocatedCIDRs[0])
		}
		return nil
	}

	// If we reached here, it means that the node has no CIDR currently assigned. So we set it.
	for i := 0; i < cidrUpdateRetries; i++ {
		if err = nodeutil.PatchNodeCIDRs(ctx, c.client, types.NodeName(node.Name), cidrsString); err == nil {
			logger.Info("Set node PodCIDR", "node", klog.KObj(node), "podCIDRs", cidrsString)
			return nil
		}
	}
	// failed release back to the pool
	logger.Error(err, "Failed to update node PodCIDR after multiple attempts", "node", klog.KObj(node), "podCIDRs", cidrsString)
	controllerutil.RecordNodeStatusChange(logger, c.recorder, node, "CIDRAssignmentFailed")
	// We accept the fact that we may leak CIDRs here. This is safer than releasing
	// them in case when we don't know if request went through.
	// NodeController restart will return all falsely allocated CIDRs to the pool.
	if !apierrors.IsServerTimeout(err) {
		logger.Error(err, "CIDR assignment for node failed. Releasing allocated CIDR", "node", klog.KObj(node))
		if releaseErr := c.cidrSet.Release(allocatedCIDRs[0]); releaseErr != nil {
			logger.Error(releaseErr, "Error releasing allocated CIDR for node", "node", klog.KObj(node))
		}
	}
	return err
}

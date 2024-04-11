package linode

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client"
	"github.com/linode/linode-cloud-controller-manager/sentry"
)

type nodeIP struct {
	ip     string
	ipType v1.NodeAddressType
}

type linodeInstance struct {
	instance *linodego.Instance
	ips      []nodeIP
}

type nodeCache struct {
	sync.RWMutex
	nodes      map[int]linodeInstance
	lastUpdate time.Time
	ttl        time.Duration
}

// getInstanceIPv4Addresses returns all ipv4 addresses configured on a linode.
func (nc *nodeCache) getInstanceIPv4Addresses(instance linodego.Instance, vpcips []string) []nodeIP {
	ips := []nodeIP{}

	// If vpc ips are present, list them first
	for _, ip := range vpcips {
		ipType := v1.NodeInternalIP
		ips = append(ips, nodeIP{ip: ip, ipType: ipType})
	}

	for _, ip := range instance.IPv4 {
		ipType := v1.NodeExternalIP
		if ip.IsPrivate() {
			ipType = v1.NodeInternalIP
		}
		ips = append(ips, nodeIP{ip: ip.String(), ipType: ipType})
	}

	return ips
}

// refreshInstances conditionally loads all instances from the Linode API and caches them.
// It does not refresh if the last update happened less than `nodeCache.ttl` ago.
func (nc *nodeCache) refreshInstances(ctx context.Context, client client.Client) error {
	nc.Lock()
	defer nc.Unlock()

	if time.Since(nc.lastUpdate) < nc.ttl {
		return nil
	}

	instances, err := client.ListInstances(ctx, nil)
	if err != nil {
		return err
	}

	// If running within VPC, find instances and store their ips
	vpcNodes := map[int][]string{}
	vpcID := vpcInfo.getID()
	if vpcID != 0 {
		resp, err := client.ListVPCIPAddresses(ctx, vpcID, linodego.NewListOptions(0, ""))
		if err != nil {
			return err
		}
		for _, r := range resp {
			if r.Address == nil {
				continue
			}
			vpcNodes[r.LinodeID] = append(vpcNodes[r.LinodeID], *r.Address)
		}
	}

	newNodes := make(map[int]linodeInstance, len(instances))
	for i, instance := range instances {

		// if running within VPC, only store instances in cache which are part of VPC
		if vpcID != 0 && len(vpcNodes[instance.ID]) == 0 {
			continue
		}
		node := linodeInstance{
			instance: &instances[i],
			ips:      nc.getInstanceIPv4Addresses(instance, vpcNodes[instance.ID]),
		}
		newNodes[instance.ID] = node
	}

	nc.nodes = newNodes
	nc.lastUpdate = time.Now()
	return nil
}

type instances struct {
	client client.Client

	nodeCache *nodeCache
}

func newInstances(client client.Client) *instances {
	timeout := 15
	if raw, ok := os.LookupEnv("LINODE_INSTANCE_CACHE_TTL"); ok {
		if t, _ := strconv.Atoi(raw); t > 0 {
			timeout = t
		}
	}
	klog.V(3).Infof("TTL for nodeCache set to %d", timeout)

	return &instances{client, &nodeCache{
		nodes: make(map[int]linodeInstance, 0),
		ttl:   time.Duration(timeout) * time.Second,
	}}
}

type instanceNoIPAddressesError struct {
	id int
}

func (e instanceNoIPAddressesError) Error() string {
	return fmt.Sprintf("instance %d has no IP addresses", e.id)
}

func (i *instances) linodeByIP(kNode *v1.Node) (*linodego.Instance, error) {
	i.nodeCache.RLock()
	defer i.nodeCache.RUnlock()
	var kNodeAddresses []string
	for _, address := range kNode.Status.Addresses {
		if address.Type == "ExternalIP" || address.Type == "InternalIP" {
			kNodeAddresses = append(kNodeAddresses, address.Address)
		}
	}
	if kNodeAddresses == nil {
		return nil, fmt.Errorf("no IP address found on node %s", kNode.Name)
	}
	for _, node := range i.nodeCache.nodes {
		for _, nodeIP := range node.instance.IPv4 {
			if !nodeIP.IsPrivate() && slices.Contains(kNodeAddresses, nodeIP.String()) {
				return node.instance, nil
			}
		}
	}

	return nil, cloudprovider.InstanceNotFound
}

func (i *instances) linodeByName(nodeName types.NodeName) *linodego.Instance {
	i.nodeCache.RLock()
	defer i.nodeCache.RUnlock()
	for _, node := range i.nodeCache.nodes {
		if node.instance.Label == string(nodeName) {
			return node.instance
		}
	}

	return nil
}

func (i *instances) linodeByID(id int) (*linodego.Instance, error) {
	i.nodeCache.RLock()
	defer i.nodeCache.RUnlock()
	linodeInstance, ok := i.nodeCache.nodes[id]
	if !ok {
		return nil, cloudprovider.InstanceNotFound
	}
	return linodeInstance.instance, nil
}

// listAllInstances returns all instances in nodeCache
func (i *instances) listAllInstances(ctx context.Context) ([]linodego.Instance, error) {
	if err := i.nodeCache.refreshInstances(ctx, i.client); err != nil {
		return nil, err
	}

	instances := []linodego.Instance{}
	for _, linodeInstance := range i.nodeCache.nodes {
		instances = append(instances, *linodeInstance.instance)
	}
	return instances, nil
}

func (i *instances) lookupLinode(ctx context.Context, node *v1.Node) (*linodego.Instance, error) {
	if err := i.nodeCache.refreshInstances(ctx, i.client); err != nil {
		return nil, err
	}

	providerID := node.Spec.ProviderID
	nodeName := types.NodeName(node.Name)

	sentry.SetTag(ctx, "provider_id", providerID)
	sentry.SetTag(ctx, "node_name", node.Name)

	if providerID != "" && isLinodeProviderID(providerID) {
		id, err := parseProviderID(providerID)
		if err != nil {
			sentry.CaptureError(ctx, err)
			return nil, err
		}
		sentry.SetTag(ctx, "linode_id", strconv.Itoa(id))

		return i.linodeByID(id)
	}
	instance := i.linodeByName(nodeName)
	if instance != nil {
		return instance, nil
	}

	return i.linodeByIP(node)
}

func (i *instances) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	ctx = sentry.SetHubOnContext(ctx)
	if _, err := i.lookupLinode(ctx, node); err != nil {
		if err == cloudprovider.InstanceNotFound {
			return false, nil
		}
		sentry.CaptureError(ctx, err)
		return false, err
	}

	return true, nil
}

func (i *instances) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	ctx = sentry.SetHubOnContext(ctx)
	instance, err := i.lookupLinode(ctx, node)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return false, err
	}

	// An instance is considered to be "shutdown" when it is
	// in the process of shutting down, or already offline.
	if instance.Status == linodego.InstanceOffline ||
		instance.Status == linodego.InstanceShuttingDown {
		return true, nil
	}

	return false, nil
}

func (i *instances) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	ctx = sentry.SetHubOnContext(ctx)
	linode, err := i.lookupLinode(ctx, node)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return nil, err
	}

	ips, err := i.getLinodeIPv4Addresses(ctx, node)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return nil, err
	}

	if len(ips) == 0 {
		err := instanceNoIPAddressesError{linode.ID}
		sentry.CaptureError(ctx, err)
		return nil, err
	}

	addresses := []v1.NodeAddress{{Type: v1.NodeHostName, Address: linode.Label}}

	for _, ip := range ips {
		addresses = append(addresses, v1.NodeAddress{Type: ip.ipType, Address: ip.ip})
	}

	// note that Zone is omitted as it's not a thing in Linode
	meta := &cloudprovider.InstanceMetadata{
		ProviderID:    fmt.Sprintf("%v%v", providerIDPrefix, linode.ID),
		NodeAddresses: addresses,
		InstanceType:  linode.Type,
		Region:        linode.Region,
	}

	return meta, nil
}

func (i *instances) getLinodeIPv4Addresses(ctx context.Context, node *v1.Node) ([]nodeIP, error) {
	ctx = sentry.SetHubOnContext(ctx)
	instance, err := i.lookupLinode(ctx, node)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return nil, err
	}

	i.nodeCache.RLock()
	defer i.nodeCache.RUnlock()
	linodeInstance, ok := i.nodeCache.nodes[instance.ID]
	if !ok || len(linodeInstance.ips) == 0 {
		err := instanceNoIPAddressesError{instance.ID}
		sentry.CaptureError(ctx, err)
		return nil, err
	}

	return linodeInstance.ips, nil
}

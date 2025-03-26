package linode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
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

// getInstanceAddresses returns all addresses configured on a linode.
func (nc *nodeCache) getInstanceAddresses(instance linodego.Instance, vpcips []string) []nodeIP {
	ips := []nodeIP{}

	// If vpc ips are present, list them first
	for _, ip := range vpcips {
		ipType := v1.NodeInternalIP
		ips = append(ips, nodeIP{ip: ip, ipType: ipType})
	}

	for _, ip := range instance.IPv4 {
		ipType := v1.NodeExternalIP
		if isPrivate(ip) {
			ipType = v1.NodeInternalIP
		}
		ips = append(ips, nodeIP{ip: ip.String(), ipType: ipType})
	}

	if instance.IPv6 != "" {
		ips = append(ips, nodeIP{ip: strings.TrimSuffix(instance.IPv6, "/128"), ipType: v1.NodeExternalIP})
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
	vpcNames := strings.Split(Options.VPCNames, ",")
	for _, v := range vpcNames {
		vpcName := strings.TrimSpace(v)
		if vpcName == "" {
			continue
		}
		resp, err := GetVPCIPAddresses(ctx, client, vpcName)
		if err != nil {
			klog.Errorf("failed updating instances cache for VPC %s. Error: %s", vpcName, err.Error())
			continue
		}
		for _, r := range resp {
			if r.Address == nil {
				continue
			}
			vpcNodes[r.LinodeID] = append(vpcNodes[r.LinodeID], *r.Address)
		}
	}

	newNodes := make(map[int]linodeInstance, len(instances))
	for index, instance := range instances {
		// if running within VPC, only store instances in cache which are part of VPC
		if Options.VPCNames != "" && len(vpcNodes[instance.ID]) == 0 {
			continue
		}
		node := linodeInstance{
			instance: &instances[index],
			ips:      nc.getInstanceAddresses(instance, vpcNodes[instance.ID]),
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
		if t, err := strconv.Atoi(raw); t > 0 && err == nil {
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
		if address.Type == v1.NodeExternalIP || address.Type == v1.NodeInternalIP {
			kNodeAddresses = append(kNodeAddresses, address.Address)
		}
	}
	if kNodeAddresses == nil {
		return nil, fmt.Errorf("no IP address found on node %s", kNode.Name)
	}
	for _, node := range i.nodeCache.nodes {
		for _, nodeIP := range node.instance.IPv4 {
			if !isPrivate(nodeIP) && slices.Contains(kNodeAddresses, nodeIP.String()) {
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
		if errors.Is(err, cloudprovider.InstanceNotFound) {
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

	ips, err := i.getLinodeAddresses(ctx, node)
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

	// create temporary uniqueAddrs cache just for reference
	uniqueAddrs := make(map[string]v1.NodeAddressType, len(node.Status.Addresses)+len(ips))
	for _, ip := range addresses {
		if _, ok := uniqueAddrs[ip.Address]; ok {
			continue
		}
		uniqueAddrs[ip.Address] = ip.Type
	}

	// include IPs set by kubelet for internal node IP
	for _, addr := range node.Status.Addresses {
		if _, ok := uniqueAddrs[addr.Address]; ok {
			continue
		}
		if addr.Type == v1.NodeInternalIP {
			uniqueAddrs[addr.Address] = v1.NodeInternalIP
			addresses = append(addresses, v1.NodeAddress{Type: addr.Type, Address: addr.Address})
		}
	}

	klog.Infof("Instance %s, assembled IP addresses: %v", node.Name, addresses)
	// note that Zone is omitted as it's not a thing in Linode
	meta := &cloudprovider.InstanceMetadata{
		ProviderID:    fmt.Sprintf("%v%v", providerIDPrefix, linode.ID),
		NodeAddresses: addresses,
		InstanceType:  linode.Type,
		Region:        linode.Region,
	}

	return meta, nil
}

func (i *instances) getLinodeAddresses(ctx context.Context, node *v1.Node) ([]nodeIP, error) {
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

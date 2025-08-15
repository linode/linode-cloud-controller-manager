package services

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
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/options"
	ccmUtils "github.com/linode/linode-cloud-controller-manager/cloud/linode/utils"
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
func (nc *nodeCache) getInstanceAddresses(instance linodego.Instance, vpcips []string, vpcIPv6AddrTypes map[string]v1.NodeAddressType) []nodeIP {
	ips := []nodeIP{}

	// We store vpc IPv6 addrs separately so that we can list them after IPv4 addresses.
	// Ordering matters in k8s, first address marked as externalIP will be used as external IP for node.
	// We prefer to use IPv4 address as external IP if possible, so we list them first.
	vpcIPv6Addrs := []nodeIP{}

	// If vpc ips are present, list them first
	for _, ip := range vpcips {
		ipType := v1.NodeInternalIP
		if _, ok := vpcIPv6AddrTypes[ip]; ok {
			ipType = vpcIPv6AddrTypes[ip]
			vpcIPv6Addrs = append(vpcIPv6Addrs, nodeIP{ip: ip, ipType: ipType})
			continue
		}
		ips = append(ips, nodeIP{ip: ip, ipType: ipType})
	}

	for _, ip := range instance.IPv4 {
		ipType := v1.NodeExternalIP
		if ccmUtils.IsPrivate(ip, options.Options.LinodeExternalNetwork) {
			ipType = v1.NodeInternalIP
		}
		ips = append(ips, nodeIP{ip: ip.String(), ipType: ipType})
	}

	// Add vpc IPv6 addresses after IPv4 addresses
	ips = append(ips, vpcIPv6Addrs...)

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
	vpcIPv6AddrTypes := map[string]v1.NodeAddressType{}
	for _, name := range options.Options.VPCNames {
		vpcName := strings.TrimSpace(name)
		if vpcName == "" {
			continue
		}
		resp, err := GetVPCIPAddresses(ctx, client, vpcName)
		if err != nil {
			klog.Errorf("failed updating instances cache for VPC %s. Error: %s", vpcName, err.Error())
			continue
		}
		for _, vpcip := range resp {
			if vpcip.Address == nil {
				continue
			}
			vpcNodes[vpcip.LinodeID] = append(vpcNodes[vpcip.LinodeID], *vpcip.Address)
		}

		resp, err = GetVPCIPv6Addresses(ctx, client, vpcName)
		if err != nil {
			klog.Errorf("failed updating instances cache for VPC %s. Error: %s", vpcName, err.Error())
			continue
		}
		for _, vpcip := range resp {
			if len(vpcip.IPv6Addresses) == 0 {
				continue
			}
			vpcIPv6AddrType := v1.NodeInternalIP
			if vpcip.IPv6IsPublic != nil && *vpcip.IPv6IsPublic {
				vpcIPv6AddrType = v1.NodeExternalIP
			}
			for _, ipv6 := range vpcip.IPv6Addresses {
				vpcNodes[vpcip.LinodeID] = append(vpcNodes[vpcip.LinodeID], ipv6.SLAACAddress)
				vpcIPv6AddrTypes[ipv6.SLAACAddress] = vpcIPv6AddrType
			}
		}
	}

	newNodes := make(map[int]linodeInstance, len(instances))
	for index, instance := range instances {
		// if running within VPC, only store instances in cache which are part of VPC
		if len(options.Options.VPCNames) > 0 && len(vpcNodes[instance.ID]) == 0 {
			continue
		}
		node := linodeInstance{
			instance: &instances[index],
			ips:      nc.getInstanceAddresses(instance, vpcNodes[instance.ID], vpcIPv6AddrTypes),
		}
		newNodes[instance.ID] = node
	}

	nc.nodes = newNodes
	nc.lastUpdate = time.Now()
	return nil
}

type Instances struct {
	client client.Client

	nodeCache *nodeCache
}

// NewInstances creates a new Instances cache with a specified TTL for the nodeCache.
func NewInstances(client client.Client) *Instances {
	timeout := 15
	if raw, ok := os.LookupEnv("LINODE_INSTANCE_CACHE_TTL"); ok {
		if t, err := strconv.Atoi(raw); t > 0 && err == nil {
			timeout = t
		}
	}
	klog.V(3).Infof("TTL for nodeCache set to %d", timeout)

	return &Instances{client, &nodeCache{
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

func (i *Instances) linodeByIP(kNode *v1.Node) (*linodego.Instance, error) {
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
			if !ccmUtils.IsPrivate(nodeIP, options.Options.LinodeExternalNetwork) && slices.Contains(kNodeAddresses, nodeIP.String()) {
				return node.instance, nil
			}
		}
	}

	return nil, cloudprovider.InstanceNotFound
}

func (i *Instances) linodeByName(nodeName types.NodeName) *linodego.Instance {
	i.nodeCache.RLock()
	defer i.nodeCache.RUnlock()
	for _, node := range i.nodeCache.nodes {
		if node.instance.Label == string(nodeName) {
			return node.instance
		}
	}

	return nil
}

func (i *Instances) linodeByID(id int) (*linodego.Instance, error) {
	i.nodeCache.RLock()
	defer i.nodeCache.RUnlock()
	linodeInstance, ok := i.nodeCache.nodes[id]
	if !ok {
		return nil, cloudprovider.InstanceNotFound
	}
	return linodeInstance.instance, nil
}

// ListAllInstances returns all instances in nodeCache
func (i *Instances) ListAllInstances(ctx context.Context) ([]linodego.Instance, error) {
	if err := i.nodeCache.refreshInstances(ctx, i.client); err != nil {
		return nil, err
	}

	instances := []linodego.Instance{}
	for _, linodeInstance := range i.nodeCache.nodes {
		instances = append(instances, *linodeInstance.instance)
	}
	return instances, nil
}

// LookupLinode looks up a Linode instance by its ProviderID or NodeName.
func (i *Instances) LookupLinode(ctx context.Context, node *v1.Node) (*linodego.Instance, error) {
	if err := i.nodeCache.refreshInstances(ctx, i.client); err != nil {
		return nil, err
	}

	providerID := node.Spec.ProviderID
	nodeName := types.NodeName(node.Name)

	sentry.SetTag(ctx, "provider_id", providerID)
	sentry.SetTag(ctx, "node_name", node.Name)

	if providerID != "" && ccmUtils.IsLinodeProviderID(providerID) {
		id, err := ccmUtils.ParseProviderID(providerID)
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

func (i *Instances) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	ctx = sentry.SetHubOnContext(ctx)
	if _, err := i.LookupLinode(ctx, node); err != nil {
		if errors.Is(err, cloudprovider.InstanceNotFound) {
			return false, nil
		}
		sentry.CaptureError(ctx, err)
		return false, err
	}

	return true, nil
}

func (i *Instances) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	ctx = sentry.SetHubOnContext(ctx)
	instance, err := i.LookupLinode(ctx, node)
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

func (i *Instances) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	ctx = sentry.SetHubOnContext(ctx)
	linode, err := i.LookupLinode(ctx, node)
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
		ProviderID:    fmt.Sprintf("%v%v", ccmUtils.ProviderIDPrefix, linode.ID),
		NodeAddresses: addresses,
		InstanceType:  linode.Type,
		Region:        linode.Region,
	}

	return meta, nil
}

func (i *Instances) getLinodeAddresses(ctx context.Context, node *v1.Node) ([]nodeIP, error) {
	ctx = sentry.SetHubOnContext(ctx)
	instance, err := i.LookupLinode(ctx, node)
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

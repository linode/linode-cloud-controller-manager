package linode

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/linode/linodego"
	"golang.org/x/sync/errgroup"
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
func (nc *nodeCache) getInstanceIPv4Addresses(ctx context.Context, id int, client client.Client) ([]nodeIP, error) {
	// Retrieve ipaddresses for the linode
	addresses, err := client.GetInstanceIPAddresses(ctx, id)
	if err != nil {
		return nil, err
	}

	var ips []nodeIP
	if len(addresses.IPv4.Public) != 0 {
		for _, ip := range addresses.IPv4.Public {
			ips = append(ips, nodeIP{ip: ip.Address, ipType: v1.NodeExternalIP})
		}
	}

	// Retrieve instance configs for the linode
	configs, err := client.ListInstanceConfigs(ctx, id, &linodego.ListOptions{})
	if err != nil || len(configs) == 0 {
		return nil, err
	}

	// Iterate over interfaces in config and find VPC specific ips
	for _, iface := range configs[0].Interfaces {
		if iface.VPCID != nil && iface.IPv4.VPC != "" {
			ips = append(ips, nodeIP{ip: iface.IPv4.VPC, ipType: v1.NodeInternalIP})
		}
	}

	// NOTE: We specifically store VPC ips first so that if they exist, they are
	//       used as internal ip for the nodes than the private ip
	if len(addresses.IPv4.Private) != 0 {
		for _, ip := range addresses.IPv4.Private {
			ips = append(ips, nodeIP{ip: ip.Address, ipType: v1.NodeInternalIP})
		}
	}

	return ips, nil
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

	nc.nodes = make(map[int]linodeInstance, len(instances))

	mtx := sync.Mutex{}
	g := new(errgroup.Group)
	for _, instance := range instances {
		instance := instance
		g.Go(func() error {
			addresses, err := nc.getInstanceIPv4Addresses(ctx, instance.ID, client)
			if err != nil {
				klog.Errorf("Failed fetching ip addresses for instance id %d. Error: %s", instance.ID, err.Error())
				return err
			}
			// take lock on map so that concurrent writes are safe
			mtx.Lock()
			defer mtx.Unlock()
			node := linodeInstance{instance: &instance, ips: addresses}
			nc.nodes[instance.ID] = node
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

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

func (i *instances) linodeByName(nodeName types.NodeName) (*linodego.Instance, error) {
	i.nodeCache.RLock()
	defer i.nodeCache.RUnlock()
	for _, node := range i.nodeCache.nodes {
		if node.instance.Label == string(nodeName) {
			return node.instance, nil
		}
	}

	return nil, cloudprovider.InstanceNotFound
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

	return i.linodeByName(nodeName)
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

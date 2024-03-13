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
	"golang.org/x/exp/slices"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client"
)

type routeCache struct {
	sync.RWMutex
	routes     map[int][]linodego.InstanceConfig
	lastUpdate time.Time
	ttl        time.Duration
}

func (rc *routeCache) refreshRoutes(ctx context.Context, client client.Client) error {
	rc.Lock()
	defer rc.Unlock()

	if time.Since(rc.lastUpdate) < rc.ttl {
		return nil
	}

	instances, err := client.ListInstances(ctx, nil)
	if err != nil {
		return err
	}

	rc.routes = make(map[int][]linodego.InstanceConfig, len(instances))

	mtx := sync.Mutex{}
	g := new(errgroup.Group)
	for _, instance := range instances {
		id := instance.ID
		g.Go(func() error {
			configs, err := client.ListInstanceConfigs(ctx, id, &linodego.ListOptions{})
			if err != nil {
				klog.Errorf("Failed fetching instance configs for instance id %d. Error: %s", id, err.Error())
				return err
			}
			// take lock on map so that concurrent writes are safe
			mtx.Lock()
			defer mtx.Unlock()
			rc.routes[id] = configs
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	rc.lastUpdate = time.Now()
	return nil
}

type routes struct {
	vpcid      int
	client     client.Client
	instances  *instances
	routeCache *routeCache
}

func newRoutes(client client.Client) (cloudprovider.Routes, error) {
	timeout := 60
	if raw, ok := os.LookupEnv("LINODE_ROUTES_CACHE_TTL"); ok {
		if t, _ := strconv.Atoi(raw); t > 0 {
			timeout = t
		}
	}
	klog.V(3).Infof("TTL for routeCache set to %d", timeout)

	vpcid, err := getVPCID(client, Options.VPCName)
	if err != nil {
		return nil, err
	}

	return &routes{
		vpcid:     vpcid,
		client:    client,
		instances: newInstances(client),
		routeCache: &routeCache{
			routes: make(map[int][]linodego.InstanceConfig, 0),
			ttl:    time.Duration(timeout) * time.Second,
		},
	}, nil
}

// instanceConfigsByID returns InstanceConfigs for given instance id
func (r *routes) instanceConfigsByID(id int) ([]linodego.InstanceConfig, error) {
	r.routeCache.RLock()
	defer r.routeCache.RUnlock()
	instanceConfigs, ok := r.routeCache.routes[id]
	if !ok {
		return nil, fmt.Errorf("no configs found for instance %d", id)
	}
	return instanceConfigs, nil
}

// getInstanceConfigs returns InstanceConfigs for given instance id
// It refreshes routeCache if it has expired
func (r *routes) getInstanceConfigs(ctx context.Context, id int) ([]linodego.InstanceConfig, error) {
	if err := r.routeCache.refreshRoutes(ctx, r.client); err != nil {
		return nil, err
	}

	return r.instanceConfigsByID(id)
}

// getInstanceFromName returns linode instance with given name if it exists
func (r *routes) getInstanceFromName(ctx context.Context, name string) (*linodego.Instance, error) {
	// create node object
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	// fetch instance with specified node name
	instance, err := r.instances.lookupLinode(ctx, node)
	if err != nil {
		klog.Errorf("failed getting linode %s", name)
		return nil, err
	}
	return instance, nil
}

// CreateRoute adds route's subnet to ip_ranges of target node's VPC interface
func (r *routes) CreateRoute(ctx context.Context, clusterName string, nameHint string, route *cloudprovider.Route) error {
	instance, err := r.getInstanceFromName(ctx, string(route.TargetNode))
	if err != nil {
		return err
	}

	// fetch instance configs
	configs, err := r.getInstanceConfigs(ctx, instance.ID)
	if err != nil {
		return err
	}

	// find VPC interface and add route to it
	for _, iface := range configs[0].Interfaces {
		if iface.VPCID == nil || r.vpcid != *iface.VPCID || iface.IPv4.VPC == "" {
			continue
		}

		if slices.Contains(iface.IPRanges, route.DestinationCIDR) {
			klog.V(4).Infof("Route already exists for node %s", route.TargetNode)
			return nil
		}

		ipRanges := append(iface.IPRanges, route.DestinationCIDR)
		interfaceUpdateOptions := linodego.InstanceConfigInterfaceUpdateOptions{
			IPRanges: ipRanges,
		}
		resp, err := r.client.UpdateInstanceConfigInterface(ctx, instance.ID, configs[0].ID, iface.ID, interfaceUpdateOptions)
		if err != nil {
			return err
		}
		klog.V(4).Infof("Added routes for node %s. Current routes: %v", route.TargetNode, resp.IPRanges)
		return nil
	}

	return fmt.Errorf("unable to add route %s for node %s. no valid interface found", route.DestinationCIDR, route.TargetNode)
}

// DeleteRoute removes route's subnet from ip_ranges of target node's VPC interface
func (r *routes) DeleteRoute(ctx context.Context, clusterName string, route *cloudprovider.Route) error {
	instance, err := r.getInstanceFromName(ctx, string(route.TargetNode))
	if err != nil {
		return err
	}

	configs, err := r.getInstanceConfigs(ctx, instance.ID)
	if err != nil {
		return err
	}

	for _, iface := range configs[0].Interfaces {
		if iface.VPCID == nil || r.vpcid != *iface.VPCID || iface.IPv4.VPC == "" {
			continue
		}

		ipRanges := []string{}
		for _, configured_route := range iface.IPRanges {
			if configured_route != route.DestinationCIDR {
				ipRanges = append(ipRanges, configured_route)
			}
		}

		interfaceUpdateOptions := linodego.InstanceConfigInterfaceUpdateOptions{
			IPRanges: ipRanges,
		}
		resp, err := r.client.UpdateInstanceConfigInterface(ctx, instance.ID, configs[0].ID, iface.ID, interfaceUpdateOptions)
		if err != nil {
			return err
		}
		klog.V(4).Infof("Deleted route for node %s. Current routes: %v", route.TargetNode, resp.IPRanges)
		return nil
	}
	return fmt.Errorf("unable to remove route %s for node %s", route.DestinationCIDR, route.TargetNode)
}

// ListRoutes fetches routes configured on all instances which have VPC interfaces
func (r *routes) ListRoutes(ctx context.Context, clusterName string) ([]*cloudprovider.Route, error) {
	klog.V(4).Infof("Fetching routes configured on the cluster")
	instances, err := r.instances.listAllInstances(ctx)
	if err != nil {
		return nil, err
	}

	var routes []*cloudprovider.Route
	for _, instance := range instances {
		configs, err := r.getInstanceConfigs(ctx, instance.ID)
		if err != nil {
			klog.Errorf("Failed finding routes for instance id %d. Error: %s", instance.ID, err.Error())
			continue
		}

		for _, iface := range configs[0].Interfaces {
			if iface.VPCID == nil || r.vpcid != *iface.VPCID || iface.IPv4.VPC == "" {
				continue
			}

			for _, ipsubnet := range iface.IPRanges {
				route := &cloudprovider.Route{
					TargetNode:      types.NodeName(instance.Label),
					DestinationCIDR: ipsubnet,
				}
				klog.V(4).Infof("Found route: node %s, route %s", instance.Label, route.DestinationCIDR)
				routes = append(routes, route)
			}
		}
	}
	return routes, nil
}

package linode

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client"
)

type routeCache struct {
	Mu         sync.RWMutex
	routes     map[int][]linodego.VPCIP
	lastUpdate time.Time
	ttl        time.Duration
}

// RefreshCache checks if cache has expired and updates it accordingly
func (rc *routeCache) refreshRoutes(ctx context.Context, client client.Client) {
	rc.Mu.Lock()
	defer rc.Mu.Unlock()

	if time.Since(rc.lastUpdate) < rc.ttl {
		return
	}

	vpcNodes := map[int][]linodego.VPCIP{}
	vpcNames := strings.Split(Options.VPCNames, ",")
	for _, v := range vpcNames {
		vpcName := strings.TrimSpace(v)
		if vpcName == "" {
			continue
		}
		resp, err := GetVPCIPAddresses(ctx, client, vpcName)
		if err != nil {
			klog.Errorf("failed updating cache for VPC %s. Error: %s", vpcName, err.Error())
			continue
		}
		for _, r := range resp {
			vpcNodes[r.LinodeID] = append(vpcNodes[r.LinodeID], r)
		}
	}

	rc.routes = vpcNodes
	rc.lastUpdate = time.Now()
}

type routes struct {
	client     client.Client
	instances  *instances
	routeCache *routeCache
}

func newRoutes(client client.Client, instanceCache *instances) (cloudprovider.Routes, error) {
	timeout := 60
	if raw, ok := os.LookupEnv("LINODE_ROUTES_CACHE_TTL_SECONDS"); ok {
		if t, err := strconv.Atoi(raw); t > 0 && err == nil {
			timeout = t
		}
	}
	klog.V(3).Infof("TTL for routeCache set to %d seconds", timeout)

	if Options.EnableRouteController && Options.VPCNames == "" {
		return nil, fmt.Errorf("cannot enable route controller as vpc-names is empty")
	}

	return &routes{
		client:    client,
		instances: instanceCache,
		routeCache: &routeCache{
			routes: make(map[int][]linodego.VPCIP, 0),
			ttl:    time.Duration(timeout) * time.Second,
		},
	}, nil
}

// instanceRoutesByID returns routes for given instance id
func (r *routes) instanceRoutesByID(id int) ([]linodego.VPCIP, error) {
	r.routeCache.Mu.RLock()
	defer r.routeCache.Mu.RUnlock()
	instanceRoutes, ok := r.routeCache.routes[id]
	if !ok {
		return nil, fmt.Errorf("no routes found for instance %d", id)
	}
	return instanceRoutes, nil
}

// getInstanceRoutes returns routes for given instance id
// It refreshes routeCache if it has expired
func (r *routes) getInstanceRoutes(ctx context.Context, id int) ([]linodego.VPCIP, error) {
	r.routeCache.refreshRoutes(ctx, r.client)
	return r.instanceRoutesByID(id)
}

// getInstanceFromName returns linode instance with given name if it exists
func (r *routes) getInstanceFromName(ctx context.Context, name string) (*linodego.Instance, error) {
	// create node object
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	// fetch providerID from k8s node cache if it exists
	if id, ok := registeredK8sNodeCache.getProviderID(name); ok {
		node.Spec.ProviderID = id
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

	// fetch instance routes
	instanceRoutes, err := r.getInstanceRoutes(ctx, instance.ID)
	if err != nil {
		return err
	}

	// check already configured routes
	intfRoutes := []string{}
	intfVPCIP := linodego.VPCIP{}

	for _, vpcid := range GetAllVPCIDs() {
		for _, ir := range instanceRoutes {
			if ir.VPCID != vpcid {
				continue
			}

			if ir.Address != nil {
				intfVPCIP = ir
				continue
			}

			if ir.AddressRange != nil && *ir.AddressRange == route.DestinationCIDR {
				klog.V(4).Infof("Route already exists for node %s", route.TargetNode)
				return nil
			}

			intfRoutes = append(intfRoutes, *ir.AddressRange)
		}
	}

	if intfVPCIP.Address == nil {
		return fmt.Errorf("unable to add route %s for node %s. no valid interface found", route.DestinationCIDR, route.TargetNode)
	}

	intfRoutes = append(intfRoutes, route.DestinationCIDR)
	interfaceUpdateOptions := linodego.InstanceConfigInterfaceUpdateOptions{
		IPRanges: &intfRoutes,
	}

	resp, err := r.client.UpdateInstanceConfigInterface(ctx, instance.ID, intfVPCIP.ConfigID, intfVPCIP.InterfaceID, interfaceUpdateOptions)
	if err != nil {
		return err
	}
	klog.V(4).Infof("Added routes for node %s. Current routes: %v", route.TargetNode, resp.IPRanges)
	return nil
}

// DeleteRoute removes route's subnet from ip_ranges of target node's VPC interface
func (r *routes) DeleteRoute(ctx context.Context, clusterName string, route *cloudprovider.Route) error {
	instance, err := r.getInstanceFromName(ctx, string(route.TargetNode))
	if err != nil {
		return err
	}

	instanceRoutes, err := r.getInstanceRoutes(ctx, instance.ID)
	if err != nil {
		return err
	}

	// check already configured routes
	intfRoutes := []string{}
	intfVPCIP := linodego.VPCIP{}

	for _, vpcid := range GetAllVPCIDs() {
		for _, ir := range instanceRoutes {
			if ir.VPCID != vpcid {
				continue
			}

			if ir.Address != nil {
				intfVPCIP = ir
				continue
			}

			if ir.AddressRange != nil && *ir.AddressRange == route.DestinationCIDR {
				continue
			}

			intfRoutes = append(intfRoutes, *ir.AddressRange)
		}
	}

	if intfVPCIP.Address == nil {
		return fmt.Errorf("unable to remove route %s for node %s. no valid interface found", route.DestinationCIDR, route.TargetNode)
	}

	interfaceUpdateOptions := linodego.InstanceConfigInterfaceUpdateOptions{
		IPRanges: &intfRoutes,
	}
	resp, err := r.client.UpdateInstanceConfigInterface(ctx, instance.ID, intfVPCIP.ConfigID, intfVPCIP.InterfaceID, interfaceUpdateOptions)
	if err != nil {
		return err
	}
	klog.V(4).Infof("Deleted route for node %s. Current routes: %v", route.TargetNode, resp.IPRanges)
	return nil
}

// ListRoutes fetches routes configured on all instances which have VPC interfaces
func (r *routes) ListRoutes(ctx context.Context, clusterName string) ([]*cloudprovider.Route, error) {
	klog.V(4).Infof("Fetching routes configured on the cluster")
	instances, err := r.instances.listAllInstances(ctx)
	if err != nil {
		return nil, err
	}

	var configuredRoutes []*cloudprovider.Route
	for _, instance := range instances {
		providerID := providerIDPrefix + strconv.Itoa(instance.ID)
		label, found := registeredK8sNodeCache.getNodeLabel(providerID, instance.Label)
		if !found {
			klog.V(4).Infof("Node %s not found in k8s node cache, skipping listing its routes", instance.Label)
			continue
		}
		// Update label to match with k8s registered label
		instance.Label = label

		instanceRoutes, err := r.getInstanceRoutes(ctx, instance.ID)
		if err != nil {
			klog.Errorf("Failed finding routes for instance id %d. Error: %v", instance.ID, err)
			continue
		}

		// check for configured routes
		for _, vpcid := range GetAllVPCIDs() {
			for _, ir := range instanceRoutes {
				if ir.Address != nil || ir.VPCID != vpcid {
					continue
				}

				if ir.AddressRange != nil {
					route := &cloudprovider.Route{
						TargetNode:      types.NodeName(instance.Label),
						DestinationCIDR: *ir.AddressRange,
					}
					configuredRoutes = append(configuredRoutes, route)
				}
			}
		}
	}
	return configuredRoutes, nil
}

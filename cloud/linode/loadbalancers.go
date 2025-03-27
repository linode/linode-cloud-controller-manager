package linode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2alpha1"
	ciliumclient "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/typed/cilium.io/v2alpha1"
	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"

	"github.com/linode/linode-cloud-controller-manager/cloud/annotations"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/firewall"
	"github.com/linode/linode-cloud-controller-manager/sentry"
)

var (
	errNoNodesAvailable          = errors.New("no nodes available for nodebalancer")
	maxConnThrottleStringLen int = 20
)

type lbNotFoundError struct {
	serviceNn      string
	nodeBalancerID int
}

func (e lbNotFoundError) Error() string {
	if e.nodeBalancerID != 0 {
		return fmt.Sprintf("LoadBalancer (%d) not found for service (%s)", e.nodeBalancerID, e.serviceNn)
	}
	return fmt.Sprintf("LoadBalancer not found for service (%s)", e.serviceNn)
}

type loadbalancers struct {
	client           client.Client
	zone             string
	kubeClient       kubernetes.Interface
	ciliumClient     ciliumclient.CiliumV2alpha1Interface
	loadBalancerType string
}

type portConfigAnnotation struct {
	TLSSecretName string `json:"tls-secret-name"`
	Protocol      string `json:"protocol"`
	ProxyProtocol string `json:"proxy-protocol"`
}

type portConfig struct {
	TLSSecretName string
	Protocol      linodego.ConfigProtocol
	ProxyProtocol linodego.ConfigProxyProtocol
	Port          int
}

// newLoadbalancers returns a cloudprovider.LoadBalancer whose concrete type is a *loadbalancer.
func newLoadbalancers(client client.Client, zone string) cloudprovider.LoadBalancer {
	return &loadbalancers{client: client, zone: zone, loadBalancerType: Options.LoadBalancerType}
}

func (l *loadbalancers) getNodeBalancerForService(ctx context.Context, service *v1.Service) (*linodego.NodeBalancer, error) {
	rawID := service.GetAnnotations()[annotations.AnnLinodeNodeBalancerID]
	id, idErr := strconv.Atoi(rawID)
	hasIDAnn := idErr == nil && id != 0

	if hasIDAnn {
		sentry.SetTag(ctx, "load_balancer_id", rawID)
		return l.getNodeBalancerByID(ctx, service, id)
	}
	return l.getNodeBalancerByStatus(ctx, service)
}

func (l *loadbalancers) getLatestServiceLoadBalancerStatus(ctx context.Context, service *v1.Service) (v1.LoadBalancerStatus, error) {
	err := l.retrieveKubeClient()
	if err != nil {
		return v1.LoadBalancerStatus{}, err
	}

	service, err = l.kubeClient.CoreV1().Services(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
	if err != nil {
		return v1.LoadBalancerStatus{}, err
	}
	return service.Status.LoadBalancer, nil
}

// getNodeBalancerByStatus attempts to get the NodeBalancer from the IPv4 specified in the
// most recent LoadBalancer status.
func (l *loadbalancers) getNodeBalancerByStatus(ctx context.Context, service *v1.Service) (nb *linodego.NodeBalancer, err error) {
	for _, ingress := range service.Status.LoadBalancer.Ingress {
		if ingress.IP != "" {
			return l.getNodeBalancerByIPv4(ctx, service, ingress.IP)
		}
		if ingress.Hostname != "" {
			return l.getNodeBalancerByHostname(ctx, service, ingress.Hostname)
		}
	}
	return nil, lbNotFoundError{serviceNn: getServiceNn(service)}
}

// cleanupOldNodeBalancer removes the service's disowned NodeBalancer if there is one.
//
// The current NodeBalancer from getNodeBalancerForService is compared to the most recent
// LoadBalancer status; if they are different (because of an updated NodeBalancerID
// annotation), the old one is deleted.
func (l *loadbalancers) cleanupOldNodeBalancer(ctx context.Context, service *v1.Service) error {
	// unless there's an annotation, we can never get a past and current NB to differ,
	// because they're looked up the same way
	if _, ok := service.GetAnnotations()[annotations.AnnLinodeNodeBalancerID]; !ok {
		return nil
	}

	previousNB, err := l.getNodeBalancerByStatus(ctx, service)
	//nolint: errorlint //conversion to errors.Is() may break chainsaw tests
	switch err.(type) {
	case nil:
		// continue execution
		break
	case lbNotFoundError:
		return nil
	default:
		return err
	}

	nb, err := l.getNodeBalancerForService(ctx, service)
	if err != nil {
		return err
	}

	if previousNB.ID == nb.ID {
		return nil
	}

	if err := l.client.DeleteNodeBalancer(ctx, previousNB.ID); err != nil {
		return err
	}

	klog.Infof("successfully deleted old NodeBalancer (%d) for service (%s)", previousNB.ID, getServiceNn(service))
	return nil
}

// GetLoadBalancerName returns the name of the load balancer.
//
// GetLoadBalancer will not modify service.
func (l *loadbalancers) GetLoadBalancerName(_ context.Context, _ string, _ *v1.Service) string {
	unixNano := strconv.FormatInt(time.Now().UnixNano(), 16)
	return fmt.Sprintf("ccm-%s", unixNano[len(unixNano)-12:])
}

// GetLoadBalancer returns the *v1.LoadBalancerStatus of service.
//
// GetLoadBalancer will not modify service.
func (l *loadbalancers) GetLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (*v1.LoadBalancerStatus, bool, error) {
	ctx = sentry.SetHubOnContext(ctx)
	sentry.SetTag(ctx, "cluster_name", clusterName)
	sentry.SetTag(ctx, "service", service.Name)

	// Handle LoadBalancers backed by Cilium
	if l.loadBalancerType == ciliumLBType {
		return &v1.LoadBalancerStatus{
			Ingress: service.Status.LoadBalancer.Ingress,
		}, true, nil
	}

	nb, err := l.getNodeBalancerForService(ctx, service)
	//nolint: errorlint //conversion to errors.Is() may break chainsaw tests
	switch err.(type) {
	case nil:
		break

	case lbNotFoundError:
		return nil, false, nil

	default:
		sentry.CaptureError(ctx, err)
		return nil, false, err
	}

	return makeLoadBalancerStatus(service, nb), true, nil
}

// EnsureLoadBalancer ensures that the cluster is running a load balancer for
// service.
//
// EnsureLoadBalancer will not modify service or nodes.
func (l *loadbalancers) EnsureLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (lbStatus *v1.LoadBalancerStatus, err error) {
	ctx = sentry.SetHubOnContext(ctx)
	sentry.SetTag(ctx, "cluster_name", clusterName)
	sentry.SetTag(ctx, "service", service.Name)
	serviceNn := getServiceNn(service)

	// Handle LoadBalancers backed by Cilium
	if l.loadBalancerType == ciliumLBType {
		klog.Infof("handling LoadBalancer Service %s as %s", serviceNn, ciliumLBClass)

		if err = l.ensureCiliumBGPPeeringPolicy(ctx); err != nil {
			klog.Infof("Failed to ensure CiliumBGPPeeringPolicy: %v", err)
			return nil, err
		}

		// check for existing CiliumLoadBalancerIPPool for service
		var pool *v2alpha1.CiliumLoadBalancerIPPool
		pool, err = l.getCiliumLBIPPool(ctx, service)
		if err != nil && !k8serrors.IsNotFound(err) {
			klog.Infof("Failed to get CiliumLoadBalancerIPPool: %s", err.Error())
			return nil, err
		}
		// if the CiliumLoadBalancerIPPool doesn't exist, it's not nil, instead an empty struct
		// gets returned, so we check if this is so via the Name being empty
		if pool != nil && pool.Name != "" {
			klog.Infof("Cilium LB IP pool %s for Service %s ensured", pool.Name, serviceNn)
			// ingress will be set by Cilium
			return &v1.LoadBalancerStatus{
				Ingress: service.Status.LoadBalancer.Ingress,
			}, nil
		}

		var ipHolderSuffix string
		if Options.IpHolderSuffix != "" {
			ipHolderSuffix = Options.IpHolderSuffix
			klog.Infof("using parameter-based IP Holder suffix %s for Service %s", ipHolderSuffix, serviceNn)
		}

		// CiliumLoadBalancerIPPool does not yet exist for the service
		var sharedIP string
		if sharedIP, err = l.createSharedIP(ctx, nodes, ipHolderSuffix); err != nil {
			klog.Errorf("Failed to request shared instance IP: %s", err.Error())
			return nil, err
		}
		if _, err = l.createCiliumLBIPPool(ctx, service, sharedIP); err != nil {
			klog.Infof("Failed to create CiliumLoadBalancerIPPool: %s", err.Error())
			return nil, err
		}

		// ingress will be set by Cilium
		return &v1.LoadBalancerStatus{
			Ingress: service.Status.LoadBalancer.Ingress,
		}, nil
	}

	// Handle LoadBalancers backed by NodeBalancers
	var nb *linodego.NodeBalancer

	nb, err = l.getNodeBalancerForService(ctx, service)
	//nolint: errorlint //conversion to errors.Is() may break chainsaw tests
	switch err.(type) {
	case lbNotFoundError:
		if service.GetAnnotations()[annotations.AnnLinodeNodeBalancerID] != "" {
			// a load balancer annotation has been created so a NodeBalancer is coming, error out and retry later
			klog.Infof("NodeBalancer created but not available yet, waiting...")
			sentry.CaptureError(ctx, err)
			return nil, err
		}

		if nb, err = l.buildLoadBalancerRequest(ctx, clusterName, service, nodes); err != nil {
			sentry.CaptureError(ctx, err)
			return nil, err
		}
		klog.Infof("created new NodeBalancer (%d) for service (%s)", nb.ID, serviceNn)

	case nil:
		if err = l.updateNodeBalancer(ctx, clusterName, service, nodes, nb); err != nil {
			sentry.CaptureError(ctx, err)
			return nil, err
		}

	default:
		sentry.CaptureError(ctx, err)
		return nil, err
	}

	klog.Infof("NodeBalancer (%d) has been ensured for service (%s)", nb.ID, serviceNn)
	lbStatus = makeLoadBalancerStatus(service, nb)

	if !l.shouldPreserveNodeBalancer(service) {
		if err := l.cleanupOldNodeBalancer(ctx, service); err != nil {
			sentry.CaptureError(ctx, err)
			return nil, err
		}
	}

	return lbStatus, nil
}

func (l *loadbalancers) updateNodeBalancer(
	ctx context.Context,
	clusterName string,
	service *v1.Service,
	nodes []*v1.Node,
	nb *linodego.NodeBalancer,
) (err error) {
	if len(nodes) == 0 {
		return fmt.Errorf("%w: service %s", errNoNodesAvailable, getServiceNn(service))
	}

	connThrottle := getConnectionThrottle(service)
	if connThrottle != nb.ClientConnThrottle {
		update := nb.GetUpdateOptions()
		update.ClientConnThrottle = &connThrottle
		nb, err = l.client.UpdateNodeBalancer(ctx, nb.ID, update)
		if err != nil {
			sentry.CaptureError(ctx, err)
			return err
		}
	}

	tags := l.GetLoadBalancerTags(ctx, clusterName, service)
	if !reflect.DeepEqual(nb.Tags, tags) {
		update := nb.GetUpdateOptions()
		update.Tags = &tags
		nb, err = l.client.UpdateNodeBalancer(ctx, nb.ID, update)
		if err != nil {
			sentry.CaptureError(ctx, err)
			return err
		}
	}

	fwClient := firewall.LinodeClient{Client: l.client}
	err = fwClient.UpdateNodeBalancerFirewall(ctx, l.GetLoadBalancerName(ctx, clusterName, service), tags, service, nb)
	if err != nil {
		return err
	}

	// Get all of the NodeBalancer's configs
	nbCfgs, err := l.client.ListNodeBalancerConfigs(ctx, nb.ID, nil)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return err
	}

	// Delete any configs for ports that have been removed from the Service
	if err = l.deleteUnusedConfigs(ctx, nbCfgs, service.Spec.Ports); err != nil {
		sentry.CaptureError(ctx, err)
		return err
	}

	// Add or overwrite configs for each of the Service's ports
	for _, port := range service.Spec.Ports {
		if port.Protocol == v1.ProtocolUDP {
			err := fmt.Errorf("error updating NodeBalancer Config: ports with the UDP protocol are not supported")
			sentry.CaptureError(ctx, err)
			return err
		}

		// Construct a new config for this port
		newNBCfg, err := l.buildNodeBalancerConfig(ctx, service, int(port.Port))
		if err != nil {
			sentry.CaptureError(ctx, err)
			return err
		}

		// Look for an existing config for this port
		var currentNBCfg *linodego.NodeBalancerConfig
		for i := range nbCfgs {
			nbc := nbCfgs[i]
			if nbc.Port == int(port.Port) {
				currentNBCfg = &nbc
				break
			}
		}
		oldNBNodeIDs := make(map[string]int)
		if currentNBCfg != nil {
			// Obtain list of current NB nodes and convert it to map of node IDs
			var currentNBNodes []linodego.NodeBalancerNode
			currentNBNodes, err = l.client.ListNodeBalancerNodes(ctx, nb.ID, currentNBCfg.ID, nil)
			if err != nil {
				// This error can be ignored, because if we fail to get nodes we can anyway rebuild the config from scratch,
				// it would just cause the NB to reload config even if the node list did not change, so we prefer to send IDs when it is posible.
				klog.Warningf("Unable to list existing nodebalancer nodes for NB %d config %d, error: %s", nb.ID, newNBCfg.ID, err)
			}
			for _, node := range currentNBNodes {
				oldNBNodeIDs[node.Address] = node.ID
			}
			klog.Infof("Nodebalancer %d had nodes %v", nb.ID, oldNBNodeIDs)
		} else {
			klog.Infof("No preexisting nodebalancer for port %v found.", port.Port)
		}
		// Add all of the Nodes to the config
		newNBNodes := make([]linodego.NodeBalancerConfigRebuildNodeOptions, 0, len(nodes))
		subnetID := 0
		backendIPv4Range, ok := service.GetAnnotations()[annotations.NodeBalancerBackendIPv4Range]
		if ok {
			if err = validateNodeBalancerBackendIPv4Range(backendIPv4Range); err != nil {
				return err
			}

			var id int
			id, err = l.getSubnetIDForSVC(ctx, service)
			if err != nil {
				sentry.CaptureError(ctx, err)
				return fmt.Errorf("Error getting subnet ID for service %s: %w", service.Name, err)
			}
			subnetID = id
		}
		for _, node := range nodes {
			newNodeOpts := l.buildNodeBalancerNodeConfigRebuildOptions(node, port.NodePort, subnetID)
			oldNodeID, ok := oldNBNodeIDs[newNodeOpts.Address]
			if ok {
				newNodeOpts.ID = oldNodeID
			} else {
				klog.Infof("No preexisting node id for %v found.", newNodeOpts.Address)
			}
			newNBNodes = append(newNBNodes, newNodeOpts)
		}

		// If there's no existing config, create it
		var rebuildOpts linodego.NodeBalancerConfigRebuildOptions
		if currentNBCfg == nil {
			createOpts := newNBCfg.GetCreateOptions()

			currentNBCfg, err = l.client.CreateNodeBalancerConfig(ctx, nb.ID, createOpts)
			if err != nil {
				sentry.CaptureError(ctx, err)
				return fmt.Errorf("[port %d] error creating NodeBalancer config: %w", int(port.Port), err)
			}
			rebuildOpts = currentNBCfg.GetRebuildOptions()

			// SSLCert and SSLKey return <REDACTED> from the API, so copy the
			// value that we sent in create for the rebuild
			rebuildOpts.SSLCert = newNBCfg.SSLCert
			rebuildOpts.SSLKey = newNBCfg.SSLKey
		} else {
			rebuildOpts = newNBCfg.GetRebuildOptions()
		}

		rebuildOpts.Nodes = newNBNodes

		if _, err = l.client.RebuildNodeBalancerConfig(ctx, nb.ID, currentNBCfg.ID, rebuildOpts); err != nil {
			sentry.CaptureError(ctx, err)
			return fmt.Errorf("[port %d] error rebuilding NodeBalancer config: %w", int(port.Port), err)
		}
	}

	return nil
}

// UpdateLoadBalancer updates the NodeBalancer to have configs that match the Service's ports
func (l *loadbalancers) UpdateLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (err error) {
	ctx = sentry.SetHubOnContext(ctx)
	sentry.SetTag(ctx, "cluster_name", clusterName)
	sentry.SetTag(ctx, "service", service.Name)

	// handle LoadBalancers backed by Cilium
	if l.loadBalancerType == ciliumLBType {
		klog.Infof("handling update for LoadBalancer Service %s/%s as %s", service.Namespace, service.Name, ciliumLBClass)
		serviceNn := getServiceNn(service)
		var ipHolderSuffix string
		if Options.IpHolderSuffix != "" {
			ipHolderSuffix = Options.IpHolderSuffix
			klog.V(3).Infof("using parameter-based IP Holder suffix %s for Service %s", ipHolderSuffix, serviceNn)
		}

		// make sure that IPs are shared properly on the Node if using load-balancers not backed by NodeBalancers
		for _, node := range nodes {
			if err = l.handleIPSharing(ctx, node, ipHolderSuffix); err != nil {
				return err
			}
		}
		return nil
	}

	// UpdateLoadBalancer is invoked with a nil LoadBalancerStatus; we must fetch the latest
	// status for NodeBalancer discovery.
	serviceWithStatus := service.DeepCopy()
	serviceWithStatus.Status.LoadBalancer, err = l.getLatestServiceLoadBalancerStatus(ctx, service)
	if err != nil {
		return fmt.Errorf("failed to get latest LoadBalancer status for service (%s): %w", getServiceNn(service), err)
	}

	nb, err := l.getNodeBalancerForService(ctx, serviceWithStatus)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return err
	}

	if !l.shouldPreserveNodeBalancer(service) {
		if err := l.cleanupOldNodeBalancer(ctx, service); err != nil {
			sentry.CaptureError(ctx, err)
			return err
		}
	}

	return l.updateNodeBalancer(ctx, clusterName, serviceWithStatus, nodes, nb)
}

// Delete any NodeBalancer configs for ports that no longer exist on the Service
// Note: Don't build a map or other lookup structure here, it is not worth the overhead
func (l *loadbalancers) deleteUnusedConfigs(ctx context.Context, nbConfigs []linodego.NodeBalancerConfig, servicePorts []v1.ServicePort) error {
	for _, nbc := range nbConfigs {
		found := false
		for _, sp := range servicePorts {
			if nbc.Port == int(sp.Port) {
				found = true
			}
		}
		if !found {
			if err := l.client.DeleteNodeBalancerConfig(ctx, nbc.NodeBalancerID, nbc.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// shouldPreserveNodeBalancer determines whether a NodeBalancer should be deleted based on the
// service's preserve annotation.
func (l *loadbalancers) shouldPreserveNodeBalancer(service *v1.Service) bool {
	return getServiceBoolAnnotation(service, annotations.AnnLinodeLoadBalancerPreserve)
}

// EnsureLoadBalancerDeleted deletes the specified loadbalancer if it exists.
// nil is returned if the load balancer for service does not exist or is
// successfully deleted.
//
// EnsureLoadBalancerDeleted will not modify service.
func (l *loadbalancers) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *v1.Service) error {
	ctx = sentry.SetHubOnContext(ctx)
	sentry.SetTag(ctx, "cluster_name", clusterName)
	sentry.SetTag(ctx, "service", service.Name)

	// Handle LoadBalancers backed by Cilium
	if l.loadBalancerType == ciliumLBType {
		klog.Infof("handling LoadBalancer Service %s/%s as %s", service.Namespace, service.Name, ciliumLBClass)
		if err := l.deleteSharedIP(ctx, service); err != nil {
			return err
		}
		// delete CiliumLoadBalancerIPPool for service
		if err := l.deleteCiliumLBIPPool(ctx, service); err != nil && !k8serrors.IsNotFound(err) {
			klog.Infof("Failed to delete CiliumLoadBalancerIPPool")
			return err
		}

		return nil
	}

	// Handle LoadBalancers backed by NodeBalancers

	serviceNn := getServiceNn(service)

	if len(service.Status.LoadBalancer.Ingress) == 0 {
		klog.Infof("short-circuiting deletion of NodeBalancer for service(%s) as LoadBalancer ingress is not present", serviceNn)
		return nil
	}

	nb, err := l.getNodeBalancerForService(ctx, service)
	//nolint: errorlint //conversion to errors.Is() may break chainsaw tests
	switch getErr := err.(type) {
	case nil:
		break

	case lbNotFoundError:
		klog.Infof("short-circuiting deletion for NodeBalancer for service (%s) as one does not exist: %s", serviceNn, err)
		return nil

	default:
		klog.Errorf("failed to get NodeBalancer for service (%s): %s", serviceNn, err)
		sentry.CaptureError(ctx, getErr)
		return err
	}

	if l.shouldPreserveNodeBalancer(service) {
		klog.Infof(
			"short-circuiting deletion of NodeBalancer (%d) for service (%s) as annotated with %s",
			nb.ID,
			serviceNn,
			annotations.AnnLinodeLoadBalancerPreserve,
		)
		return nil
	}

	fwClient := firewall.LinodeClient{Client: l.client}
	if err = fwClient.DeleteNodeBalancerFirewall(ctx, service, nb); err != nil {
		return err
	}

	if err = l.client.DeleteNodeBalancer(ctx, nb.ID); err != nil {
		klog.Errorf("failed to delete NodeBalancer (%d) for service (%s): %s", nb.ID, serviceNn, err)
		sentry.CaptureError(ctx, err)
		return err
	}

	klog.Infof("successfully deleted NodeBalancer (%d) for service (%s)", nb.ID, serviceNn)
	return nil
}

func (l *loadbalancers) getNodeBalancerByHostname(ctx context.Context, service *v1.Service, hostname string) (*linodego.NodeBalancer, error) {
	lbs, err := l.client.ListNodeBalancers(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, lb := range lbs {
		if *lb.Hostname == hostname {
			klog.V(2).Infof("found NodeBalancer (%d) for service (%s) via hostname (%s)", lb.ID, getServiceNn(service), hostname)
			return &lb, nil
		}
	}
	return nil, lbNotFoundError{serviceNn: getServiceNn(service)}
}

func (l *loadbalancers) getNodeBalancerByIPv4(ctx context.Context, service *v1.Service, ipv4 string) (*linodego.NodeBalancer, error) {
	filter := fmt.Sprintf(`{"ipv4": "%v"}`, ipv4)
	lbs, err := l.client.ListNodeBalancers(ctx, &linodego.ListOptions{Filter: filter})
	if err != nil {
		return nil, err
	}
	if len(lbs) == 0 {
		return nil, lbNotFoundError{serviceNn: getServiceNn(service)}
	}
	klog.V(2).Infof("found NodeBalancer (%d) for service (%s) via IPv4 (%s)", lbs[0].ID, getServiceNn(service), ipv4)
	return &lbs[0], nil
}

func (l *loadbalancers) getNodeBalancerByID(ctx context.Context, service *v1.Service, id int) (*linodego.NodeBalancer, error) {
	nb, err := l.client.GetNodeBalancer(ctx, id)
	if err != nil {
		//nolint: errorlint //need type assertion for code field to work
		if apiErr, ok := err.(*linodego.Error); ok && apiErr.Code == http.StatusNotFound {
			return nil, lbNotFoundError{serviceNn: getServiceNn(service), nodeBalancerID: id}
		}
		return nil, err
	}
	return nb, nil
}

func (l *loadbalancers) GetLoadBalancerTags(_ context.Context, clusterName string, service *v1.Service) []string {
	tags := []string{}
	if clusterName != "" {
		tags = append(tags, clusterName)
	}

	tags = append(tags, Options.NodeBalancerTags...)

	tagStr, ok := service.GetAnnotations()[annotations.AnnLinodeLoadBalancerTags]
	if ok {
		return append(tags, strings.Split(tagStr, ",")...)
	}

	return tags
}

// GetLinodeNBType returns the NodeBalancer type for the service.
func (l *loadbalancers) GetLinodeNBType(service *v1.Service) linodego.NodeBalancerPlanType {
	typeStr, ok := service.GetAnnotations()[annotations.AnnLinodeNodeBalancerType]
	if ok && linodego.NodeBalancerPlanType(typeStr) == linodego.NBTypePremium {
		return linodego.NBTypePremium
	}

	return linodego.NodeBalancerPlanType(Options.DefaultNBType)
}

func (l *loadbalancers) createNodeBalancer(ctx context.Context, clusterName string, service *v1.Service, configs []*linodego.NodeBalancerConfigCreateOptions) (lb *linodego.NodeBalancer, err error) {
	connThrottle := getConnectionThrottle(service)

	label := l.GetLoadBalancerName(ctx, clusterName, service)
	tags := l.GetLoadBalancerTags(ctx, clusterName, service)
	nbType := l.GetLinodeNBType(service)
	createOpts := linodego.NodeBalancerCreateOptions{
		Label:              &label,
		Region:             l.zone,
		ClientConnThrottle: &connThrottle,
		Configs:            configs,
		Tags:               tags,
		Type:               nbType,
	}

	backendIPv4Range, ok := service.GetAnnotations()[annotations.NodeBalancerBackendIPv4Range]
	if ok {
		if err := validateNodeBalancerBackendIPv4Range(backendIPv4Range); err != nil {
			return nil, err
		}
		subnetID, err := l.getSubnetIDForSVC(ctx, service)
		if err != nil {
			return nil, err
		}
		createOpts.VPCs = []linodego.NodeBalancerVPCOptions{
			{
				SubnetID:  subnetID,
				IPv4Range: backendIPv4Range,
			},
		}
	}

	fwid, ok := service.GetAnnotations()[annotations.AnnLinodeCloudFirewallID]
	if ok {
		firewallID, err := strconv.Atoi(fwid)
		if err != nil {
			return nil, err
		}
		createOpts.FirewallID = firewallID
	} else {
		// There's no firewallID already set, see if we need to create a new fw, look for the acl annotation.
		_, ok := service.GetAnnotations()[annotations.AnnLinodeCloudFirewallACL]
		if ok {
			fwcreateOpts, err := firewall.CreateFirewallOptsForSvc(label, tags, service)
			if err != nil {
				return nil, err
			}

			fw, err := l.client.CreateFirewall(ctx, *fwcreateOpts)
			if err != nil {
				return nil, err
			}
			createOpts.FirewallID = fw.ID
		}
		// no need to deal with firewalls, continue creating nb's
	}

	return l.client.CreateNodeBalancer(ctx, createOpts)
}

func (l *loadbalancers) buildNodeBalancerConfig(ctx context.Context, service *v1.Service, port int) (linodego.NodeBalancerConfig, error) {
	portConfigResult, err := getPortConfig(service, port)
	if err != nil {
		return linodego.NodeBalancerConfig{}, err
	}

	health, err := getHealthCheckType(service)
	if err != nil {
		return linodego.NodeBalancerConfig{}, err
	}

	config := linodego.NodeBalancerConfig{
		Port:          port,
		Protocol:      portConfigResult.Protocol,
		ProxyProtocol: portConfigResult.ProxyProtocol,
		Check:         health,
	}

	if health == linodego.CheckHTTP || health == linodego.CheckHTTPBody {
		path := service.GetAnnotations()[annotations.AnnLinodeCheckPath]
		if path == "" {
			path = "/"
		}
		config.CheckPath = path
	}

	if health == linodego.CheckHTTPBody {
		body := service.GetAnnotations()[annotations.AnnLinodeCheckBody]
		if body == "" {
			return config, fmt.Errorf("for health check type http_body need body regex annotation %v", annotations.AnnLinodeCheckBody)
		}
		config.CheckBody = body
	}
	checkInterval := 5
	if ci, ok := service.GetAnnotations()[annotations.AnnLinodeHealthCheckInterval]; ok {
		if checkInterval, err = strconv.Atoi(ci); err != nil {
			return config, err
		}
	}
	config.CheckInterval = checkInterval

	checkTimeout := 3
	if ct, ok := service.GetAnnotations()[annotations.AnnLinodeHealthCheckTimeout]; ok {
		if checkTimeout, err = strconv.Atoi(ct); err != nil {
			return config, err
		}
	}
	config.CheckTimeout = checkTimeout

	checkAttempts := 2
	if ca, ok := service.GetAnnotations()[annotations.AnnLinodeHealthCheckAttempts]; ok {
		if checkAttempts, err = strconv.Atoi(ca); err != nil {
			return config, err
		}
	}
	config.CheckAttempts = checkAttempts

	checkPassive := true
	if cp, ok := service.GetAnnotations()[annotations.AnnLinodeHealthCheckPassive]; ok {
		if checkPassive, err = strconv.ParseBool(cp); err != nil {
			return config, err
		}
	}
	config.CheckPassive = checkPassive

	if portConfigResult.Protocol == linodego.ProtocolHTTPS {
		if err = l.addTLSCert(ctx, service, &config, portConfigResult); err != nil {
			return config, err
		}
	}

	return config, nil
}

func (l *loadbalancers) addTLSCert(ctx context.Context, service *v1.Service, nbConfig *linodego.NodeBalancerConfig, config portConfig) error {
	err := l.retrieveKubeClient()
	if err != nil {
		return err
	}

	nbConfig.SSLCert, nbConfig.SSLKey, err = getTLSCertInfo(ctx, l.kubeClient, service.Namespace, config)
	if err != nil {
		return err
	}
	return nil
}

// getSubnetIDForSVC returns the subnet ID for the service's VPC and subnet.
// By default, first VPCName and SubnetName are used to calculate subnet id for the service.
// If the service has annotations specifying VPCName and SubnetName, they are used instead.
func (l *loadbalancers) getSubnetIDForSVC(ctx context.Context, service *v1.Service) (int, error) {
	if Options.VPCNames == "" {
		return 0, fmt.Errorf("CCM not configured with VPC, cannot create NodeBalancer with specified annotation")
	}
	vpcName := strings.Split(Options.VPCNames, ",")[0]
	if specifiedVPCName, ok := service.GetAnnotations()[annotations.NodeBalancerBackendVPCName]; ok {
		vpcName = specifiedVPCName
	}
	vpcID, err := GetVPCID(ctx, l.client, vpcName)
	if err != nil {
		return 0, err
	}
	subnetName := strings.Split(Options.SubnetNames, ",")[0]
	if specifiedSubnetName, ok := service.GetAnnotations()[annotations.NodeBalancerBackendSubnetName]; ok {
		subnetName = specifiedSubnetName
	}
	return GetSubnetID(ctx, l.client, vpcID, subnetName)
}

// buildLoadBalancerRequest returns a linodego.NodeBalancer
// requests for service across nodes.
func (l *loadbalancers) buildLoadBalancerRequest(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (*linodego.NodeBalancer, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("%w: cluster %s, service %s", errNoNodesAvailable, clusterName, getServiceNn(service))
	}
	ports := service.Spec.Ports
	configs := make([]*linodego.NodeBalancerConfigCreateOptions, 0, len(ports))

	subnetID := 0
	backendIPv4Range, ok := service.GetAnnotations()[annotations.NodeBalancerBackendIPv4Range]
	if ok {
		if err := validateNodeBalancerBackendIPv4Range(backendIPv4Range); err != nil {
			return nil, err
		}
		id, err := l.getSubnetIDForSVC(ctx, service)
		if err != nil {
			return nil, err
		}
		subnetID = id
	}

	for _, port := range ports {
		if port.Protocol == v1.ProtocolUDP {
			return nil, fmt.Errorf("error creating NodeBalancer Config: ports with the UDP protocol are not supported")
		}

		config, err := l.buildNodeBalancerConfig(ctx, service, int(port.Port))
		if err != nil {
			return nil, err
		}
		createOpt := config.GetCreateOptions()

		for _, n := range nodes {
			createOpt.Nodes = append(createOpt.Nodes, l.buildNodeBalancerNodeConfigRebuildOptions(n, port.NodePort, subnetID).NodeBalancerNodeCreateOptions)
		}

		configs = append(configs, &createOpt)
	}
	return l.createNodeBalancer(ctx, clusterName, service, configs)
}

func coerceString(str string, minLen, maxLen int, padding string) string {
	if len(padding) == 0 {
		padding = "x"
	}
	if len(str) > maxLen {
		return str[:maxLen]
	} else if len(str) < minLen {
		return coerceString(fmt.Sprintf("%s%s", padding, str), minLen, maxLen, padding)
	}
	return str
}

func (l *loadbalancers) buildNodeBalancerNodeConfigRebuildOptions(node *v1.Node, nodePort int32, subnetID int) linodego.NodeBalancerConfigRebuildNodeOptions {
	nodeOptions := linodego.NodeBalancerConfigRebuildNodeOptions{
		NodeBalancerNodeCreateOptions: linodego.NodeBalancerNodeCreateOptions{
			Address: fmt.Sprintf("%v:%v", getNodePrivateIP(node, subnetID), nodePort),
			// NodeBalancer backends must be 3-32 chars in length
			// If < 3 chars, pad node name with "node-" prefix
			Label:  coerceString(node.Name, 3, 32, "node-"),
			Mode:   "accept",
			Weight: 100,
		},
	}
	if subnetID != 0 {
		nodeOptions.NodeBalancerNodeCreateOptions.SubnetID = subnetID
	}
	return nodeOptions
}

func (l *loadbalancers) retrieveKubeClient() error {
	if l.kubeClient != nil {
		return nil
	}

	var (
		kubeConfig *rest.Config
		err        error
	)

	// Check to see if --kubeconfig was set. If it was, build a kubeconfig from the given file.
	// Otherwise, use the in-cluster config.
	kubeconfigFlag := Options.KubeconfigFlag
	if kubeconfigFlag == nil || kubeconfigFlag.Value.String() == "" {
		kubeConfig, err = rest.InClusterConfig()
	} else {
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigFlag.Value.String())
	}

	if err != nil {
		return err
	}

	l.kubeClient, err = kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return err
	}

	return nil
}

func getPortConfig(service *v1.Service, port int) (portConfig, error) {
	portConfigResult := portConfig{}
	portConfigAnnotationResult, err := getPortConfigAnnotation(service, port)
	if err != nil {
		return portConfigResult, err
	}
	protocol := portConfigAnnotationResult.Protocol
	if protocol == "" {
		protocol = "tcp"
		if p, ok := service.GetAnnotations()[annotations.AnnLinodeDefaultProtocol]; ok {
			protocol = p
		}
	}
	protocol = strings.ToLower(protocol)

	proxyProtocol := portConfigAnnotationResult.ProxyProtocol
	if proxyProtocol == "" {
		proxyProtocol = string(linodego.ProxyProtocolNone)
		for _, ann := range []string{annotations.AnnLinodeDefaultProxyProtocol, annLinodeProxyProtocolDeprecated} {
			if pp, ok := service.GetAnnotations()[ann]; ok {
				proxyProtocol = pp
				break
			}
		}
	}

	if protocol != "tcp" && protocol != "http" && protocol != "https" {
		return portConfigResult, fmt.Errorf("invalid protocol: %q specified", protocol)
	}

	switch proxyProtocol {
	case string(linodego.ProxyProtocolNone), string(linodego.ProxyProtocolV1), string(linodego.ProxyProtocolV2):
		break
	default:
		return portConfigResult, fmt.Errorf("invalid NodeBalancer proxy protocol value '%s'", proxyProtocol)
	}

	portConfigResult.Port = port
	portConfigResult.Protocol = linodego.ConfigProtocol(protocol)
	portConfigResult.ProxyProtocol = linodego.ConfigProxyProtocol(proxyProtocol)
	portConfigResult.TLSSecretName = portConfigAnnotationResult.TLSSecretName

	return portConfigResult, nil
}

func getHealthCheckType(service *v1.Service) (linodego.ConfigCheck, error) {
	hType, ok := service.GetAnnotations()[annotations.AnnLinodeHealthCheckType]
	if !ok {
		return linodego.CheckConnection, nil
	}
	if hType != "none" && hType != "connection" && hType != "http" && hType != "http_body" {
		return "", fmt.Errorf("invalid health check type: %q specified in annotation: %q", hType, annotations.AnnLinodeHealthCheckType)
	}
	return linodego.ConfigCheck(hType), nil
}

func getPortConfigAnnotation(service *v1.Service, port int) (portConfigAnnotation, error) {
	annotation := portConfigAnnotation{}
	annotationKey := annotations.AnnLinodePortConfigPrefix + strconv.Itoa(port)
	annotationJSON, ok := service.GetAnnotations()[annotationKey]

	if !ok {
		return annotation, nil
	}

	err := json.Unmarshal([]byte(annotationJSON), &annotation)
	if err != nil {
		return annotation, err
	}

	return annotation, nil
}

// getNodePrivateIP provides the Linode Backend IP the NodeBalancer will communicate with.
// If a service specifies NodeBalancerBackendIPv4Range annotation, it will
// use NodeInternalIP of node.
// For services which don't have NodeBalancerBackendIPv4Range annotation,
// Backend IP can be overwritten to the one specified using AnnLinodeNodePrivateIP
// annotation over the NodeInternalIP.
func getNodePrivateIP(node *v1.Node, subnetID int) string {
	if subnetID == 0 {
		if address, exists := node.Annotations[annotations.AnnLinodeNodePrivateIP]; exists {
			return address
		}
	}

	klog.Infof("Node %s, assigned IP addresses: %v", node.Name, node.Status.Addresses)
	for _, addr := range node.Status.Addresses {
		if addr.Type == v1.NodeInternalIP {
			return addr.Address
		}
	}
	return ""
}

func getTLSCertInfo(ctx context.Context, kubeClient kubernetes.Interface, namespace string, config portConfig) (string, string, error) {
	if config.TLSSecretName == "" {
		return "", "", fmt.Errorf("TLS secret name for port %v is not specified", config.Port)
	}

	secret, err := kubeClient.CoreV1().Secrets(namespace).Get(ctx, config.TLSSecretName, metav1.GetOptions{})
	if err != nil {
		return "", "", err
	}

	cert := string(secret.Data[v1.TLSCertKey])
	cert = strings.TrimSpace(cert)

	key := string(secret.Data[v1.TLSPrivateKeyKey])

	key = strings.TrimSpace(key)

	return cert, key, nil
}

func getConnectionThrottle(service *v1.Service) int {
	connThrottle := 0 // disable throttle if nothing is specified

	if connThrottleString := service.GetAnnotations()[annotations.AnnLinodeThrottle]; connThrottleString != "" {
		parsed, err := strconv.Atoi(connThrottleString)
		if err == nil {
			if parsed < 0 {
				parsed = 0
			}

			if parsed > maxConnThrottleStringLen {
				parsed = maxConnThrottleStringLen
			}
			connThrottle = parsed
		}
	}

	return connThrottle
}

func makeLoadBalancerStatus(service *v1.Service, nb *linodego.NodeBalancer) *v1.LoadBalancerStatus {
	ingress := v1.LoadBalancerIngress{
		Hostname: *nb.Hostname,
	}

	// Return hostname-only if annotation is set or environment variable is set
	if getServiceBoolAnnotation(service, annotations.AnnLinodeHostnameOnlyIngress) {
		return &v1.LoadBalancerStatus{
			Ingress: []v1.LoadBalancerIngress{ingress},
		}
	}

	if val := envBoolOptions("LINODE_HOSTNAME_ONLY_INGRESS"); val {
		klog.Infof("LINODE_HOSTNAME_ONLY_INGRESS:  (%v)", val)
		return &v1.LoadBalancerStatus{
			Ingress: []v1.LoadBalancerIngress{ingress},
		}
	}

	// Check for per-service IPv6 annotation first, then fall back to global setting
	useIPv6 := getServiceBoolAnnotation(service, annotations.AnnLinodeEnableIPv6Ingress) || Options.EnableIPv6ForLoadBalancers

	// When IPv6 is enabled (either per-service or globally), include both IPv4 and IPv6
	if useIPv6 && nb.IPv6 != nil && *nb.IPv6 != "" {
		ingresses := []v1.LoadBalancerIngress{
			{
				Hostname: *nb.Hostname,
				IP:       *nb.IPv4,
			},
			{
				Hostname: *nb.Hostname,
				IP:       *nb.IPv6,
			},
		}
		klog.V(4).Infof("Using both IPv4 and IPv6 addresses for NodeBalancer (%d): %s, %s", nb.ID, *nb.IPv4, *nb.IPv6)
		return &v1.LoadBalancerStatus{
			Ingress: ingresses,
		}
	}

	// Default case - just use IPv4
	ingress.IP = *nb.IPv4
	return &v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{ingress},
	}
}

// Checks for a truth value in an environment variable
func envBoolOptions(o string) bool {
	boolValue, err := strconv.ParseBool(os.Getenv(o))
	if err != nil {
		return false
	}
	return boolValue
}

// getServiceNn returns the services namespaced name.
func getServiceNn(service *v1.Service) string {
	return fmt.Sprintf("%s/%s", service.Namespace, service.Name)
}

func getServiceBoolAnnotation(service *v1.Service, name string) bool {
	value, ok := service.GetAnnotations()[name]
	if !ok {
		return false
	}
	boolValue, err := strconv.ParseBool(value)
	return err == nil && boolValue
}

// validateNodeBalancerBackendIPv4Range validates the NodeBalancerBackendIPv4Range
// annotation to be within the NodeBalancerBackendIPv4Subnet if it is set.
func validateNodeBalancerBackendIPv4Range(backendIPv4Range string) error {
	if Options.NodeBalancerBackendIPv4Subnet == "" {
		return nil
	}
	withinCIDR, err := isCIDRWithinCIDR(Options.NodeBalancerBackendIPv4Subnet, backendIPv4Range)
	if err != nil {
		return fmt.Errorf("invalid IPv4 range: %w", err)
	}
	if !withinCIDR {
		return fmt.Errorf("IPv4 range %s is not within the subnet %s", backendIPv4Range, Options.NodeBalancerBackendIPv4Subnet)
	}
	return nil
}

// isCIDRWithinCIDR returns true if the inner CIDR is within the outer CIDR.
func isCIDRWithinCIDR(outer, inner string) (bool, error) {
	_, ipNet1, err := net.ParseCIDR(outer)
	if err != nil {
		return false, fmt.Errorf("invalid CIDR: %w", err)
	}
	_, ipNet2, err := net.ParseCIDR(inner)
	if err != nil {
		return false, fmt.Errorf("invalid CIDR: %w", err)
	}
	return ipNet1.Contains(ipNet2.IP), nil
}

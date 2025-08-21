package linode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
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
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/options"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/services"
	"github.com/linode/linode-cloud-controller-manager/sentry"
)

var (
	errNoNodesAvailable             = errors.New("no nodes available for nodebalancer")
	maxConnThrottleStringLen    int = 20
	eventIPChangeIgnoredWarning     = "nodebalancer-ipv4-change-ignored"

	// validProtocols is a map of valid protocols
	validProtocols = map[string]bool{
		string(linodego.ProtocolTCP):   true,
		string(linodego.ProtocolUDP):   true,
		string(linodego.ProtocolHTTP):  true,
		string(linodego.ProtocolHTTPS): true,
	}
	// validProxyProtocols is a map of valid proxy protocols
	validProxyProtocols = map[string]bool{
		string(linodego.ProxyProtocolNone): true,
		string(linodego.ProxyProtocolV1):   true,
		string(linodego.ProxyProtocolV2):   true,
	}
	// validTCPAlgorithms is a map of valid TCP algorithms
	validTCPAlgorithms = map[string]bool{
		string(linodego.AlgorithmRoundRobin): true,
		string(linodego.AlgorithmLeastConn):  true,
		string(linodego.AlgorithmSource):     true,
	}
	// validUDPAlgorithms is a map of valid UDP algorithms
	validUDPAlgorithms = map[string]bool{
		string(linodego.AlgorithmRoundRobin): true,
		string(linodego.AlgorithmRingHash):   true,
		string(linodego.AlgorithmLeastConn):  true,
	}
	// validHTTPStickiness is a map of valid HTTP stickiness options
	validHTTPStickiness = map[string]bool{
		string(linodego.StickinessNone):       true,
		string(linodego.StickinessHTTPCookie): true,
		string(linodego.StickinessTable):      true,
	}
	// validHTTPSStickiness is the same as validHTTPStickiness, but for HTTPS
	validHTTPSStickiness = map[string]bool{
		string(linodego.StickinessNone):       true,
		string(linodego.StickinessHTTPCookie): true,
		string(linodego.StickinessTable):      true,
	}
	// validUDPStickiness is a map of valid UDP stickiness options
	validUDPStickiness = map[string]bool{
		string(linodego.StickinessNone):     true,
		string(linodego.StickinessSession):  true,
		string(linodego.StickinessSourceIP): true,
	}
	// validNBConfigChecks is a map of valid NodeBalancer config checks
	validNBConfigChecks = map[string]bool{
		string(linodego.CheckNone):       true,
		string(linodego.CheckHTTP):       true,
		string(linodego.CheckHTTPBody):   true,
		string(linodego.CheckConnection): true,
	}
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
	Algorithm     string `json:"algorithm"`
	Stickiness    string `json:"stickiness"`
	UDPCheckPort  string `json:"udp-check-port"`
}

type portConfig struct {
	TLSSecretName string
	Protocol      linodego.ConfigProtocol
	ProxyProtocol linodego.ConfigProxyProtocol
	Port          int
	Algorithm     linodego.ConfigAlgorithm
	Stickiness    linodego.ConfigStickiness
	UDPCheckPort  int
}

// newLoadbalancers returns a cloudprovider.LoadBalancer whose concrete type is a *loadbalancer.
func newLoadbalancers(client client.Client, zone string) cloudprovider.LoadBalancer {
	return &loadbalancers{client: client, zone: zone, loadBalancerType: options.Options.LoadBalancerType}
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

// getNodeBalancerByStatus attempts to get the NodeBalancer from the IP or hostname specified in the
// most recent LoadBalancer status.
func (l *loadbalancers) getNodeBalancerByStatus(ctx context.Context, service *v1.Service) (nb *linodego.NodeBalancer, err error) {
	lb := service.Status.LoadBalancer
	updatedLb, err := l.getLatestServiceLoadBalancerStatus(ctx, service)
	if err != nil {
		klog.V(3).Infof("failed to get latest LoadBalancer status for service (%s): %v", getServiceNn(service), err)
	} else {
		lb = updatedLb
	}
	for _, ingress := range lb.Ingress {
		if ingress.IP != "" {
			address, err := netip.ParseAddr(ingress.IP)
			if err != nil {
				klog.Warningf("failed to parse IP address %s from service %s/%s status, error: %s", ingress.IP, service.Namespace, service.Name, err)
			} else {
				return l.getNodeBalancerByIP(ctx, service, address)
			}
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
	if err != nil {
		var targetError lbNotFoundError
		if errors.As(err, &targetError) {
			return nil
		} else {
			return err
		}
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
	return fmt.Sprintf("%s-%s", options.Options.NodeBalancerPrefix, unixNano[len(unixNano)-12:])
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
	if err != nil {
		var targetError lbNotFoundError
		if errors.As(err, &targetError) {
			return nil, false, nil
		} else {
			sentry.CaptureError(ctx, err)
			return nil, false, err
		}
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
		if options.Options.IpHolderSuffix != "" {
			ipHolderSuffix = options.Options.IpHolderSuffix
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
	if err == nil {
		if err = l.updateNodeBalancer(ctx, clusterName, service, nodes, nb); err != nil {
			sentry.CaptureError(ctx, err)
			return nil, err
		}
	} else {
		var targetError lbNotFoundError
		if errors.As(err, &targetError) {
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
		} else {
			sentry.CaptureError(ctx, err)
			return nil, err
		}
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

func (l *loadbalancers) createIPChangeWarningEvent(ctx context.Context, service *v1.Service, nb *linodego.NodeBalancer, newIP string) {
	_, err := l.kubeClient.CoreV1().Events(service.Namespace).Create(ctx, &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      eventIPChangeIgnoredWarning,
			Namespace: service.Namespace,
		},
		InvolvedObject: v1.ObjectReference{
			Kind:      "Service",
			Namespace: service.Namespace,
			Name:      service.Name,
			UID:       service.UID,
		},
		Type:    "Warning",
		Reason:  "NodeBalancerIPChangeIgnored",
		Message: fmt.Sprintf("IPv4 annotation changed to %s, but NodeBalancer (%d) IP cannot be updated after creation. It will remain %s", newIP, nb.ID, *nb.IPv4),
		Source: v1.EventSource{
			Component: "linode-cloud-controller-manager",
		},
	}, metav1.CreateOptions{})
	if err != nil {
		klog.Errorf("failed to create NodeBalancerIPChangeIgnored event for service %s: %s", getServiceNn(service), err)
	}
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

	// Check for IPv4 annotation change
	if ipv4, ok := service.GetAnnotations()[annotations.AnnLinodeLoadBalancerIPv4]; ok && ipv4 != *nb.IPv4 {
		// Log the error in the CCM's logfile
		klog.Warningf("IPv4 annotation has changed for service (%s) from %s to %s, but NodeBalancer (%d) IP cannot be updated after creation",
			getServiceNn(service), *nb.IPv4, ipv4, nb.ID)

		// Issue a k8s cluster event warning
		l.createIPChangeWarningEvent(ctx, service, nb, ipv4)
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

	fwClient := services.LinodeClient{Client: l.client}
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
		// Construct a new config for this port
		newNBCfg, err := l.buildNodeBalancerConfig(ctx, service, port)
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
				// it would just cause the NB to reload config even if the node list did not change, so we prefer to send IDs when it is possible.
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
		if options.Options.NodeBalancerBackendIPv4SubnetID != 0 {
			subnetID = options.Options.NodeBalancerBackendIPv4SubnetID
		}
		backendIPv4Range, ok := service.GetAnnotations()[annotations.NodeBalancerBackendIPv4Range]
		if ok {
			if err = validateNodeBalancerBackendIPv4Range(backendIPv4Range); err != nil {
				return err
			}
		}
		if len(options.Options.VPCNames) > 0 && !options.Options.DisableNodeBalancerVPCBackends {
			var id int
			id, err = l.getSubnetIDForSVC(ctx, service)
			if err != nil {
				sentry.CaptureError(ctx, err)
				return fmt.Errorf("Error getting subnet ID for service %s: %w", service.Name, err)
			}
			subnetID = id
		}
		for _, node := range nodes {
			if _, ok := node.Annotations[annotations.AnnExcludeNodeFromNb]; ok {
				klog.Infof("Node %s is excluded from NodeBalancer by annotation, skipping", node.Name)
				continue
			}
			var newNodeOpts *linodego.NodeBalancerConfigRebuildNodeOptions
			newNodeOpts, err = l.buildNodeBalancerNodeConfigRebuildOptions(node, port.NodePort, subnetID, newNBCfg.Protocol)
			if err != nil {
				sentry.CaptureError(ctx, err)
				return fmt.Errorf("failed to build NodeBalancer node config options for node %s: %w", node.Name, err)
			}
			oldNodeID, ok := oldNBNodeIDs[newNodeOpts.Address]
			if ok {
				newNodeOpts.ID = oldNodeID
			} else {
				klog.Infof("No preexisting node id for %v found.", newNodeOpts.Address)
			}
			newNBNodes = append(newNBNodes, *newNodeOpts)
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
		if options.Options.IpHolderSuffix != "" {
			ipHolderSuffix = options.Options.IpHolderSuffix
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
	if err != nil {
		var targetError lbNotFoundError
		if errors.As(err, &targetError) {
			klog.Infof("short-circuiting deletion for NodeBalancer for service (%s) as one does not exist: %s", serviceNn, err)
			return nil
		} else {
			klog.Errorf("failed to get NodeBalancer for service (%s): %s", serviceNn, err)
			sentry.CaptureError(ctx, err)
			return err
		}
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

	fwClient := services.LinodeClient{Client: l.client}
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

func (l *loadbalancers) getNodeBalancerByIP(ctx context.Context, service *v1.Service, ip netip.Addr) (*linodego.NodeBalancer, error) {
	var filter string
	if ip.Is6() {
		filter = fmt.Sprintf(`{"ipv6": "%v"}`, ip.String())
	} else {
		filter = fmt.Sprintf(`{"ipv4": "%v"}`, ip.String())
	}

	lbs, err := l.client.ListNodeBalancers(ctx, &linodego.ListOptions{Filter: filter})
	if err != nil {
		return nil, err
	}
	if len(lbs) == 0 {
		return nil, lbNotFoundError{serviceNn: getServiceNn(service)}
	}
	klog.V(2).Infof("found NodeBalancer (%d) for service (%s) via IP (%s)", lbs[0].ID, getServiceNn(service), ip.String())
	return &lbs[0], nil
}

func (l *loadbalancers) getNodeBalancerByID(ctx context.Context, service *v1.Service, id int) (*linodego.NodeBalancer, error) {
	nb, err := l.client.GetNodeBalancer(ctx, id)
	if err != nil {
		var targetError *linodego.Error
		if errors.As(err, &targetError) && targetError.Code == http.StatusNotFound {
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

	tags = append(tags, options.Options.NodeBalancerTags...)

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

	return linodego.NodeBalancerPlanType(options.Options.DefaultNBType)
}

// getVPCCreateOptions returns the VPC options for the NodeBalancer creation.
// Order of precedence:
// 1. NodeBalancerBackendIPv4Range annotation
// 2. NodeBalancerBackendVPCName and NodeBalancerBackendSubnetName annotation
// 3. NodeBalancerBackendIPv4SubnetID/NodeBalancerBackendIPv4SubnetName flag
// 4. NodeBalancerBackendIPv4Subnet flag
// 5. Default to using the subnet ID of the service's VPC
func (l *loadbalancers) getVPCCreateOptions(ctx context.Context, service *v1.Service) ([]linodego.NodeBalancerVPCOptions, error) {
	// Evaluate subnetID based on annotations or flags
	subnetID, err := l.getSubnetIDForSVC(ctx, service)
	if err != nil {
		return nil, err
	}

	// Precedence 1: If the user has specified a NodeBalancerBackendIPv4Range, use that
	backendIPv4Range, ok := service.GetAnnotations()[annotations.NodeBalancerBackendIPv4Range]
	if ok {
		if err := validateNodeBalancerBackendIPv4Range(backendIPv4Range); err != nil {
			return nil, err
		}
		// If the user has specified a NodeBalancerBackendIPv4Range, use that
		// for the NodeBalancer backend ipv4 range
		if backendIPv4Range != "" {
			vpcCreateOpts := []linodego.NodeBalancerVPCOptions{
				{
					SubnetID:  subnetID,
					IPv4Range: backendIPv4Range,
				},
			}
			return vpcCreateOpts, nil
		}
	}

	// Precedence 2: If the user wants to overwrite the default VPC name or subnet name
	// and have specified it in the annotations, use it to set subnetID
	// and auto-allocate subnets from it for the NodeBalancer
	_, vpcInAnnotation := service.GetAnnotations()[annotations.NodeBalancerBackendVPCName]
	_, subnetInAnnotation := service.GetAnnotations()[annotations.NodeBalancerBackendSubnetName]
	if vpcInAnnotation || subnetInAnnotation {
		vpcCreateOpts := []linodego.NodeBalancerVPCOptions{
			{
				SubnetID: subnetID,
			},
		}
		return vpcCreateOpts, nil
	}

	// Precedence 3: If the user has specified a NodeBalancerBackendIPv4SubnetID, use that
	// and auto-allocate subnets from it for the NodeBalancer
	if options.Options.NodeBalancerBackendIPv4SubnetID != 0 {
		vpcCreateOpts := []linodego.NodeBalancerVPCOptions{
			{
				SubnetID: options.Options.NodeBalancerBackendIPv4SubnetID,
			},
		}
		return vpcCreateOpts, nil
	}

	// Precedence 4: If the user has specified a NodeBalancerBackendIPv4Subnet, use that
	// and auto-allocate subnets from it for the NodeBalancer
	if options.Options.NodeBalancerBackendIPv4Subnet != "" {
		vpcCreateOpts := []linodego.NodeBalancerVPCOptions{
			{
				SubnetID:            subnetID,
				IPv4Range:           options.Options.NodeBalancerBackendIPv4Subnet,
				IPv4RangeAutoAssign: true,
			},
		}
		return vpcCreateOpts, nil
	}

	// Default to using the subnet ID of the service's VPC
	vpcCreateOpts := []linodego.NodeBalancerVPCOptions{
		{
			SubnetID: subnetID,
		},
	}
	return vpcCreateOpts, nil
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

	if len(options.Options.VPCNames) > 0 && !options.Options.DisableNodeBalancerVPCBackends {
		createOpts.VPCs, err = l.getVPCCreateOptions(ctx, service)
		if err != nil {
			return nil, err
		}
	}

	// Check for static IPv4 address annotation
	if ipv4, ok := service.GetAnnotations()[annotations.AnnLinodeLoadBalancerIPv4]; ok {
		createOpts.IPv4 = &ipv4
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
			fwcreateOpts, err := services.CreateFirewallOptsForSvc(label, tags, service)
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

func (l *loadbalancers) buildNodeBalancerConfig(ctx context.Context, service *v1.Service, port v1.ServicePort) (linodego.NodeBalancerConfig, error) {
	portConfigResult, err := getPortConfig(service, port)
	if err != nil {
		return linodego.NodeBalancerConfig{}, err
	}

	health, err := getHealthCheckType(service, port)
	if err != nil {
		return linodego.NodeBalancerConfig{}, err
	}

	config := linodego.NodeBalancerConfig{
		Port:          int(port.Port),
		Protocol:      portConfigResult.Protocol,
		ProxyProtocol: portConfigResult.ProxyProtocol,
		Check:         health,
		Algorithm:     portConfigResult.Algorithm,
	}

	if portConfigResult.Stickiness != "" {
		config.Stickiness = portConfigResult.Stickiness
	}

	if portConfigResult.UDPCheckPort != 0 {
		config.UDPCheckPort = portConfigResult.UDPCheckPort
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
	if config.Protocol == linodego.ProtocolUDP {
		checkPassive = false
	} else if cp, ok := service.GetAnnotations()[annotations.AnnLinodeHealthCheckPassive]; ok {
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

// getSubnetIDForSVC returns the subnet ID for the service when running within VPC.
// Following precedence rules are applied:
// 1. If the service has an annotation for NodeBalancerBackendSubnetID, use that.
// 2. If the service has annotations specifying VPCName or SubnetName, use them.
// 3. If CCM is configured with --nodebalancer-backend-ipv4-subnet-id, it will be used as the subnet ID.
// 4. Else, use first VPCName and SubnetName to calculate subnet id for the service.
func (l *loadbalancers) getSubnetIDForSVC(ctx context.Context, service *v1.Service) (int, error) {
	if len(options.Options.VPCNames) == 0 {
		return 0, fmt.Errorf("CCM not configured with VPC, cannot create NodeBalancer with specified annotation")
	}
	// Check if the service has an annotation for NodeBalancerBackendSubnetID
	if specifiedSubnetID, ok := service.GetAnnotations()[annotations.NodeBalancerBackendSubnetID]; ok {
		subnetID, err := strconv.Atoi(specifiedSubnetID)
		if err != nil {
			return 0, err
		}
		return subnetID, nil
	}

	specifiedVPCName, vpcOk := service.GetAnnotations()[annotations.NodeBalancerBackendVPCName]
	specifiedSubnetName, subnetOk := service.GetAnnotations()[annotations.NodeBalancerBackendSubnetName]

	// If no VPCName or SubnetName is specified in annotations, but NodeBalancerBackendIPv4SubnetID is set,
	// use the NodeBalancerBackendIPv4SubnetID as the subnet ID.
	if !vpcOk && !subnetOk && options.Options.NodeBalancerBackendIPv4SubnetID != 0 {
		return options.Options.NodeBalancerBackendIPv4SubnetID, nil
	}

	vpcName := options.Options.VPCNames[0]
	if vpcOk {
		vpcName = specifiedVPCName
	}
	vpcID, err := services.GetVPCID(ctx, l.client, vpcName)
	if err != nil {
		return 0, err
	}

	subnetName := options.Options.SubnetNames[0]
	if subnetOk {
		subnetName = specifiedSubnetName
	}

	// Use the VPC ID and Subnet Name to get the subnet ID
	return services.GetSubnetID(ctx, l.client, vpcID, subnetName)
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
	if options.Options.NodeBalancerBackendIPv4SubnetID != 0 {
		subnetID = options.Options.NodeBalancerBackendIPv4SubnetID
	}
	// Check for the NodeBalancerBackendIPv4Range annotation
	backendIPv4Range, ok := service.GetAnnotations()[annotations.NodeBalancerBackendIPv4Range]
	if ok {
		if err := validateNodeBalancerBackendIPv4Range(backendIPv4Range); err != nil {
			return nil, err
		}
	}
	if len(options.Options.VPCNames) > 0 && !options.Options.DisableNodeBalancerVPCBackends {
		id, err := l.getSubnetIDForSVC(ctx, service)
		if err != nil {
			return nil, err
		}
		subnetID = id
	}

	for _, port := range ports {
		config, err := l.buildNodeBalancerConfig(ctx, service, port)
		if err != nil {
			return nil, err
		}
		createOpt := config.GetCreateOptions()

		for _, node := range nodes {
			if _, ok := node.Annotations[annotations.AnnExcludeNodeFromNb]; ok {
				klog.Infof("Node %s is excluded from NodeBalancer by annotation, skipping", node.Name)
				continue
			}
			newNodeOpts, err := l.buildNodeBalancerNodeConfigRebuildOptions(node, port.NodePort, subnetID, config.Protocol)
			if err != nil {
				sentry.CaptureError(ctx, err)
				return nil, fmt.Errorf("failed to build NodeBalancer node config options for node %s: %w", node.Name, err)
			}
			createOpt.Nodes = append(createOpt.Nodes, newNodeOpts.NodeBalancerNodeCreateOptions)
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

func (l *loadbalancers) buildNodeBalancerNodeConfigRebuildOptions(node *v1.Node, nodePort int32, subnetID int, protocol linodego.ConfigProtocol) (*linodego.NodeBalancerConfigRebuildNodeOptions, error) {
	nodeIP, err := getNodePrivateIP(node, subnetID)
	if err != nil {
		return nil, fmt.Errorf("node %s does not have a private IP address: %w", node.Name, err)
	}
	nodeOptions := &linodego.NodeBalancerConfigRebuildNodeOptions{
		NodeBalancerNodeCreateOptions: linodego.NodeBalancerNodeCreateOptions{
			Address: fmt.Sprintf("%v:%v", nodeIP, nodePort),
			// NodeBalancer backends must be 3-32 chars in length
			// If < 3 chars, pad node name with "node-" prefix
			Label:  coerceString(node.Name, 3, 32, "node-"),
			Weight: 100,
		},
	}
	// Mode is not set for UDP protocol
	if protocol != linodego.ProtocolUDP {
		nodeOptions.Mode = "accept"
	}
	if subnetID != 0 {
		nodeOptions.SubnetID = subnetID
	}
	return nodeOptions, nil
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
	kubeconfigFlag := options.Options.KubeconfigFlag
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

func getPortConfig(service *v1.Service, port v1.ServicePort) (portConfig, error) {
	portConfigResult := portConfig{}
	portConfigResult.Port = int(port.Port)

	portConfigAnnotationResult, err := getPortConfigAnnotation(service, int(port.Port))
	if err != nil {
		return portConfigResult, err
	}

	// validate and set protocol
	protocol, err := getPortProtocol(portConfigAnnotationResult, service, port)
	if err != nil {
		return portConfigResult, err
	}
	portConfigResult.Protocol = linodego.ConfigProtocol(protocol)

	// validate and set proxy protocol
	proxyProtocol, err := getPortProxyProtocol(portConfigAnnotationResult, service, portConfigResult.Protocol)
	if err != nil {
		return portConfigResult, err
	}
	portConfigResult.ProxyProtocol = linodego.ConfigProxyProtocol(proxyProtocol)

	// validate and set algorithm
	algorithm, err := getPortAlgorithm(portConfigAnnotationResult, service, portConfigResult.Protocol)
	if err != nil {
		return portConfigResult, err
	}
	portConfigResult.Algorithm = linodego.ConfigAlgorithm(algorithm)

	// set TLS secret name. Its only used for TCP and HTTPS protocols
	if protocol == string(linodego.ProtocolUDP) && portConfigAnnotationResult.TLSSecretName != "" {
		return portConfigResult, fmt.Errorf("specifying TLS secret name is not supported for UDP")
	}
	portConfigResult.TLSSecretName = portConfigAnnotationResult.TLSSecretName

	// validate and set udp check port
	udpCheckPort, err := getPortUDPCheckPort(portConfigAnnotationResult, service, portConfigResult.Protocol)
	if err != nil {
		return portConfigResult, err
	}
	if protocol == string(linodego.ProtocolUDP) {
		portConfigResult.UDPCheckPort = udpCheckPort
	}

	// validate and set stickiness
	stickiness, err := getPortStickiness(portConfigAnnotationResult, service, portConfigResult.Protocol)
	if err != nil {
		return portConfigResult, err
	}
	// Stickiness is not supported for TCP protocol
	if protocol != string(linodego.ProtocolTCP) {
		portConfigResult.Stickiness = linodego.ConfigStickiness(stickiness)
	}

	return portConfigResult, nil
}

func getHealthCheckType(service *v1.Service, port v1.ServicePort) (linodego.ConfigCheck, error) {
	hType, ok := service.GetAnnotations()[annotations.AnnLinodeHealthCheckType]
	if !ok {
		if port.Protocol == v1.ProtocolUDP {
			return linodego.CheckNone, nil
		}
		return linodego.CheckConnection, nil
	}
	if !validNBConfigChecks[hType] {
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
// If CCM runs within VPC and DisableNodeBalancerVPCBackends is set to false, it will
// use NodeInternalIP of node.
// For services outside of VPC, it will use linode specific private IP address
// Backend IP can be overwritten to the one specified using AnnLinodeNodePrivateIP
// annotation over the NodeInternalIP.
func getNodePrivateIP(node *v1.Node, subnetID int) (string, error) {
	if subnetID == 0 {
		if address, exists := node.Annotations[annotations.AnnLinodeNodePrivateIP]; exists {
			return address, nil
		}
	}

	klog.Infof("Node %s, assigned IP addresses: %v", node.Name, node.Status.Addresses)
	for _, addr := range node.Status.Addresses {
		if addr.Type == v1.NodeInternalIP {
			return addr.Address, nil
		}
	}

	// If no private/internal IP is found, return error.
	klog.V(4).Infof("No internal IP found for node %s", node.Name)
	return "", fmt.Errorf("no internal IP found for node %s", node.Name)
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
	useIPv6 := getServiceBoolAnnotation(service, annotations.AnnLinodeEnableIPv6Ingress) || options.Options.EnableIPv6ForLoadBalancers

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
	if options.Options.NodeBalancerBackendIPv4Subnet == "" {
		return nil
	}
	withinCIDR, err := isCIDRWithinCIDR(options.Options.NodeBalancerBackendIPv4Subnet, backendIPv4Range)
	if err != nil {
		return fmt.Errorf("invalid IPv4 range: %w", err)
	}
	if !withinCIDR {
		return fmt.Errorf("IPv4 range %s is not within the subnet %s", backendIPv4Range, options.Options.NodeBalancerBackendIPv4Subnet)
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

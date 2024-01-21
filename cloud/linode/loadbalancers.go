package linode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"

	"github.com/linode/linode-cloud-controller-manager/sentry"
	"github.com/linode/linodego"
)

const (
	// annLinodeDefaultProtocol is the annotation used to specify the default protocol
	// for Linode load balancers. Options are tcp, http and https. Defaults to tcp.
	annLinodeDefaultProtocol      = "service.beta.kubernetes.io/linode-loadbalancer-default-protocol"
	annLinodePortConfigPrefix     = "service.beta.kubernetes.io/linode-loadbalancer-port-"
	annLinodeDefaultProxyProtocol = "service.beta.kubernetes.io/linode-loadbalancer-default-proxy-protocol"

	annLinodeCheckPath       = "service.beta.kubernetes.io/linode-loadbalancer-check-path"
	annLinodeCheckBody       = "service.beta.kubernetes.io/linode-loadbalancer-check-body"
	annLinodeHealthCheckType = "service.beta.kubernetes.io/linode-loadbalancer-check-type"

	annLinodeHealthCheckInterval = "service.beta.kubernetes.io/linode-loadbalancer-check-interval"
	annLinodeHealthCheckTimeout  = "service.beta.kubernetes.io/linode-loadbalancer-check-timeout"
	annLinodeHealthCheckAttempts = "service.beta.kubernetes.io/linode-loadbalancer-check-attempts"
	annLinodeHealthCheckPassive  = "service.beta.kubernetes.io/linode-loadbalancer-check-passive"

	// annLinodeThrottle is the annotation specifying the value of the Client Connection
	// Throttle, which limits the number of subsequent new connections per second from the
	// same client IP. Options are a number between 1-20, or 0 to disable. Defaults to 20.
	annLinodeThrottle = "service.beta.kubernetes.io/linode-loadbalancer-throttle"

	annLinodeLoadBalancerPreserve = "service.beta.kubernetes.io/linode-loadbalancer-preserve"
	annLinodeNodeBalancerID       = "service.beta.kubernetes.io/linode-loadbalancer-nodebalancer-id"

	annLinodeHostnameOnlyIngress = "service.beta.kubernetes.io/linode-loadbalancer-hostname-only-ingress"
	annLinodeLoadBalancerTags    = "service.beta.kubernetes.io/linode-loadbalancer-tags"
	annLinodeCloudFirewallID     = "service.beta.kubernetes.io/linode-loadbalancer-firewall-id"

	annLinodeNodePrivateIP = "node.k8s.linode.com/private-ip"
)

var (
	errNoNodesAvailable = errors.New("no nodes available for nodebalancer")
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
	client Client
	zone   string

	kubeClient kubernetes.Interface
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
func newLoadbalancers(client Client, zone string) cloudprovider.LoadBalancer {
	return &loadbalancers{client: client, zone: zone}
}

func (l *loadbalancers) getNodeBalancerForService(ctx context.Context, service *v1.Service) (*linodego.NodeBalancer, error) {
	rawID, _ := getServiceAnnotation(service, annLinodeNodeBalancerID)
	id, idErr := strconv.Atoi(rawID)
	hasIDAnn := idErr == nil && id != 0

	if hasIDAnn {
		sentry.SetTag(ctx, "load_balancer_id", rawID)
		nb, err := l.getNodeBalancerByID(ctx, service, id)
		switch err.(type) {
		case nil:
			return nb, nil

		case lbNotFoundError:
			return nil, fmt.Errorf("%s annotation points to a NodeBalancer that does not exist: %s", annLinodeNodeBalancerID, err)

		default:
			return nil, err
		}
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
	if _, ok := getServiceAnnotation(service, annLinodeNodeBalancerID); !ok {
		return nil
	}

	previousNB, err := l.getNodeBalancerByStatus(ctx, service)
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

	nb, err := l.getNodeBalancerForService(ctx, service)
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

	var nb *linodego.NodeBalancer
	serviceNn := getServiceNn(service)

	nb, err = l.getNodeBalancerForService(ctx, service)
	switch err.(type) {
	case lbNotFoundError:
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

func (l *loadbalancers) getNodeBalancerDeviceId(ctx context.Context, firewallID, nbID, page int) (int, bool, error) {
	devices, err := l.client.ListFirewallDevices(ctx, firewallID, &linodego.ListOptions{PageSize: 500, PageOptions: &linodego.PageOptions{Page: page}})
	if err != nil {
		return 0, false, err
	}

	if len(devices) == 0 {
		return 0, false, nil
	}

	for _, device := range devices {
		if device.Entity.ID == nbID {
			return device.ID, true, nil
		}
	}

	return l.getNodeBalancerDeviceId(ctx, firewallID, nbID, page+1)
}

func (l *loadbalancers) updateNodeBalancerFirewall(ctx context.Context, service *v1.Service, nb *linodego.NodeBalancer) error {
	var newFirewallID, existingFirewallID int
	var err error
	fwid, ok := getServiceAnnotation(service, annLinodeCloudFirewallID)
	if ok {
		newFirewallID, err = strconv.Atoi(fwid)
		if err != nil {
			return err
		}
	}

	// get the attached firewall
	firewalls, err := l.client.ListNodeBalancerFirewalls(ctx, nb.ID, &linodego.ListOptions{})
	if err != nil {
		if !ok {
			if err.Error() != "[404] Not Found" {
				return err
			} else {
				return nil
			}
		}
	}

	if !ok && len(firewalls) == 0 {
		return nil
	}

	if len(firewalls) != 0 {
		existingFirewallID = firewalls[0].ID
	}

	if existingFirewallID != newFirewallID {
		// remove the existing firewall

		if existingFirewallID != 0 {

			deviceID, deviceExists, err := l.getNodeBalancerDeviceId(ctx, existingFirewallID, nb.ID, 1)
			if err != nil {
				return err
			}

			if !deviceExists {
				return fmt.Errorf("Error in fetching attached nodeBalancer device")
			}

			err = l.client.DeleteFirewallDevice(ctx, existingFirewallID, deviceID)
			if err != nil {
				return err
			}
		}

		// attach new firewall if ID != 0
		if newFirewallID != 0 {
			_, err = l.client.CreateFirewallDevice(ctx, newFirewallID, linodego.FirewallDeviceCreateOptions{
				ID:   nb.ID,
				Type: "nodebalancer",
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

//nolint:funlen
func (l *loadbalancers) updateNodeBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node, nb *linodego.NodeBalancer) (err error) {
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

	tags := l.getLoadBalancerTags(ctx, clusterName, service)
	if !reflect.DeepEqual(nb.Tags, tags) {
		update := nb.GetUpdateOptions()
		update.Tags = &tags
		nb, err = l.client.UpdateNodeBalancer(ctx, nb.ID, update)
		if err != nil {
			sentry.CaptureError(ctx, err)
			return err
		}
	}

	// update node-balancer firewall
	err = l.updateNodeBalancerFirewall(ctx, service, nb)
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

		// Add all of the Nodes to the config
		var newNBNodes []linodego.NodeBalancerNodeCreateOptions
		for _, node := range nodes {
			newNBNodes = append(newNBNodes, l.buildNodeBalancerNodeCreateOptions(node, port.NodePort))
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

		// If there's no existing config, create it
		var rebuildOpts linodego.NodeBalancerConfigRebuildOptions
		if currentNBCfg == nil {
			createOpts := newNBCfg.GetCreateOptions()

			currentNBCfg, err = l.client.CreateNodeBalancerConfig(ctx, nb.ID, createOpts)
			if err != nil {
				sentry.CaptureError(ctx, err)
				return fmt.Errorf("[port %d] error creating NodeBalancer config: %v", int(port.Port), err)
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
			return fmt.Errorf("[port %d] error rebuilding NodeBalancer config: %v", int(port.Port), err)
		}
	}
	return nil
}

// UpdateLoadBalancer updates the NodeBalancer to have configs that match the Service's ports
func (l *loadbalancers) UpdateLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (err error) {
	ctx = sentry.SetHubOnContext(ctx)
	sentry.SetTag(ctx, "cluster_name", clusterName)
	sentry.SetTag(ctx, "service", service.Name)

	// UpdateLoadBalancer is invoked with a nil LoadBalancerStatus; we must fetch the latest
	// status for NodeBalancer discovery.
	serviceWithStatus := service.DeepCopy()
	serviceWithStatus.Status.LoadBalancer, err = l.getLatestServiceLoadBalancerStatus(ctx, service)
	if err != nil {
		return fmt.Errorf("failed to get latest LoadBalancer status for service (%s): %s", getServiceNn(service), err)
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
	return getServiceBoolAnnotation(service, annLinodeLoadBalancerPreserve)
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

	serviceNn := getServiceNn(service)

	if len(service.Status.LoadBalancer.Ingress) == 0 {
		klog.Infof("short-circuiting deletion of NodeBalancer for service(%s) as LoadBalancer ingress is not present", serviceNn)
		return nil
	}

	nb, err := l.getNodeBalancerForService(ctx, service)
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
		klog.Infof("short-circuiting deletion of NodeBalancer (%d) for service (%s) as annotated with %s", nb.ID, serviceNn, annLinodeLoadBalancerPreserve)
		return nil
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
		if apiErr, ok := err.(*linodego.Error); ok && apiErr.Code == http.StatusNotFound {
			return nil, lbNotFoundError{serviceNn: getServiceNn(service), nodeBalancerID: id}
		}
		return nil, err
	}
	return nb, nil
}

func (l *loadbalancers) getLoadBalancerTags(_ context.Context, clusterName string, service *v1.Service) []string {
	tags := []string{clusterName}
	tagStr, ok := getServiceAnnotation(service, annLinodeLoadBalancerTags)
	if ok {
		return append(tags, strings.Split(tagStr, ",")...)
	}
	return tags
}

func (l *loadbalancers) createNodeBalancer(ctx context.Context, clusterName string, service *v1.Service, configs []*linodego.NodeBalancerConfigCreateOptions) (lb *linodego.NodeBalancer, err error) {
	connThrottle := getConnectionThrottle(service)

	label := l.GetLoadBalancerName(ctx, clusterName, service)
	tags := l.getLoadBalancerTags(ctx, clusterName, service)
	createOpts := linodego.NodeBalancerCreateOptions{
		Label:              &label,
		Region:             l.zone,
		ClientConnThrottle: &connThrottle,
		Configs:            configs,
		Tags:               tags,
	}

	fwid, ok := getServiceAnnotation(service, annLinodeCloudFirewallID)
	if ok {
		firewallID, err := strconv.Atoi(fwid)
		if err != nil {
			return nil, err
		}
		createOpts.FirewallID = firewallID
	}

	return l.client.CreateNodeBalancer(ctx, createOpts)
}

func (l *loadbalancers) createEmptyFirewall(ctx context.Context, service *v1.Service, opts linodego.FirewallCreateOptions) (fw *linodego.Firewall, err error) {
	return l.client.CreateFirewall(ctx, opts)
}

func (l *loadbalancers) deleteFirewall(ctx context.Context, firewall *linodego.Firewall) error {
	return l.client.DeleteFirewall(ctx, firewall.ID)
}

//nolint:funlen
func (l *loadbalancers) buildNodeBalancerConfig(ctx context.Context, service *v1.Service, port int) (linodego.NodeBalancerConfig, error) {
	portConfig, err := getPortConfig(service, port)
	if err != nil {
		return linodego.NodeBalancerConfig{}, err
	}

	health, err := getHealthCheckType(service)
	if err != nil {
		return linodego.NodeBalancerConfig{}, nil
	}

	config := linodego.NodeBalancerConfig{
		Port:          port,
		Protocol:      portConfig.Protocol,
		ProxyProtocol: portConfig.ProxyProtocol,
		Check:         health,
	}

	if health == linodego.CheckHTTP || health == linodego.CheckHTTPBody {
		path := service.Annotations[annLinodeCheckPath]
		if path == "" {
			path = "/"
		}
		config.CheckPath = path
	}

	if health == linodego.CheckHTTPBody {
		body := service.Annotations[annLinodeCheckBody]
		if body == "" {
			return config, fmt.Errorf("for health check type http_body need body regex annotation %v", annLinodeCheckBody)
		}
		config.CheckBody = body
	}
	checkInterval := 5
	if ci, ok := service.Annotations[annLinodeHealthCheckInterval]; ok {
		if checkInterval, err = strconv.Atoi(ci); err != nil {
			return config, err
		}
	}
	config.CheckInterval = checkInterval

	checkTimeout := 3
	if ct, ok := service.Annotations[annLinodeHealthCheckTimeout]; ok {
		if checkTimeout, err = strconv.Atoi(ct); err != nil {
			return config, err
		}
	}
	config.CheckTimeout = checkTimeout

	checkAttempts := 2
	if ca, ok := service.Annotations[annLinodeHealthCheckAttempts]; ok {
		if checkAttempts, err = strconv.Atoi(ca); err != nil {
			return config, err
		}
	}
	config.CheckAttempts = checkAttempts

	checkPassive := true
	if cp, ok := service.Annotations[annLinodeHealthCheckPassive]; ok {
		if checkPassive, err = strconv.ParseBool(cp); err != nil {
			return config, err
		}
	}
	config.CheckPassive = checkPassive

	if portConfig.Protocol == linodego.ProtocolHTTPS {
		if err = l.addTLSCert(ctx, service, &config, portConfig); err != nil {
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

// buildLoadBalancerRequest returns a linodego.NodeBalancer
// requests for service across nodes.
func (l *loadbalancers) buildLoadBalancerRequest(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (*linodego.NodeBalancer, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("%w: cluster %s, service %s", errNoNodesAvailable, clusterName, getServiceNn(service))
	}
	ports := service.Spec.Ports
	configs := make([]*linodego.NodeBalancerConfigCreateOptions, 0, len(ports))

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
			createOpt.Nodes = append(createOpt.Nodes, l.buildNodeBalancerNodeCreateOptions(n, port.NodePort))
		}

		configs = append(configs, &createOpt)
	}
	return l.createNodeBalancer(ctx, clusterName, service, configs)
}

func coerceString(s string, minLen, maxLen int, padding string) string {
	if len(padding) == 0 {
		padding = "x"
	}
	if len(s) > maxLen {
		return s[:maxLen]
	} else if len(s) < minLen {
		return coerceString(fmt.Sprintf("%s%s", padding, s), minLen, maxLen, padding)
	}
	return s
}

func (l *loadbalancers) buildNodeBalancerNodeCreateOptions(node *v1.Node, nodePort int32) linodego.NodeBalancerNodeCreateOptions {
	return linodego.NodeBalancerNodeCreateOptions{
		Address: fmt.Sprintf("%v:%v", getNodePrivateIP(node), nodePort),
		// NodeBalancer backends must be 3-32 chars in length
		// If < 3 chars, pad node name with "node-" prefix
		Label:  coerceString(node.Name, 3, 32, "node-"),
		Mode:   "accept",
		Weight: 100,
	}
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
	portConfig := portConfig{}
	portConfigAnnotation, err := getPortConfigAnnotation(service, port)
	if err != nil {
		return portConfig, err
	}
	protocol := portConfigAnnotation.Protocol
	if protocol == "" {
		protocol = "tcp"
		if p, ok := service.Annotations[annLinodeDefaultProtocol]; ok {
			protocol = p
		}
	}
	protocol = strings.ToLower(protocol)

	proxyProtocol := portConfigAnnotation.ProxyProtocol
	if proxyProtocol == "" {
		proxyProtocol = string(linodego.ProxyProtocolNone)
		for _, ann := range []string{annLinodeDefaultProxyProtocol, annLinodeProxyProtocolDeprecated} {
			if pp, ok := service.Annotations[ann]; ok {
				proxyProtocol = pp
				break
			}
		}
	}

	if protocol != "tcp" && protocol != "http" && protocol != "https" {
		return portConfig, fmt.Errorf("invalid protocol: %q specified", protocol)
	}

	switch proxyProtocol {
	case string(linodego.ProxyProtocolNone), string(linodego.ProxyProtocolV1), string(linodego.ProxyProtocolV2):
		break
	default:
		return portConfig, fmt.Errorf("invalid NodeBalancer proxy protocol value '%s'", proxyProtocol)
	}

	portConfig.Port = port
	portConfig.Protocol = linodego.ConfigProtocol(protocol)
	portConfig.ProxyProtocol = linodego.ConfigProxyProtocol(proxyProtocol)
	portConfig.TLSSecretName = portConfigAnnotation.TLSSecretName

	return portConfig, nil
}

func getHealthCheckType(service *v1.Service) (linodego.ConfigCheck, error) {
	hType, ok := service.Annotations[annLinodeHealthCheckType]
	if !ok {
		return linodego.CheckConnection, nil
	}
	if hType != "none" && hType != "connection" && hType != "http" && hType != "http_body" {
		return "", fmt.Errorf("invalid health check type: %q specified in annotation: %q", hType, annLinodeHealthCheckType)
	}
	return linodego.ConfigCheck(hType), nil
}

func getPortConfigAnnotation(service *v1.Service, port int) (portConfigAnnotation, error) {
	annotation := portConfigAnnotation{}
	annotationKey := annLinodePortConfigPrefix + strconv.Itoa(port)
	annotationJSON, ok := service.Annotations[annotationKey]

	if !ok {
		return annotation, nil
	}

	err := json.Unmarshal([]byte(annotationJSON), &annotation)
	if err != nil {
		return annotation, err
	}

	return annotation, nil
}

// getNodePrivateIP should provide the Linode Private IP the NodeBalance
// will communicate with. When using a VLAN or VPC for the Kubernetes cluster
// network, this will not be the NodeInternalIP, so this prefers an annotation
// cluster operators may specify in such a situation.
func getNodePrivateIP(node *v1.Node) string {
	if address, exists := node.Annotations[annLinodeNodePrivateIP]; exists {
		return address
	}

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
	connThrottle := 20

	if connThrottleString := service.Annotations[annLinodeThrottle]; connThrottleString != "" {
		parsed, err := strconv.Atoi(connThrottleString)
		if err == nil {
			if parsed < 0 {
				parsed = 0
			}

			if parsed > 20 {
				parsed = 20
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
	if !getServiceBoolAnnotation(service, annLinodeHostnameOnlyIngress) {
		if val := envBoolOptions("LINODE_HOSTNAME_ONLY_INGRESS"); val {
			klog.Infof("LINODE_HOSTNAME_ONLY_INGRESS:  (%v)", val)
		} else {
			ingress.IP = *nb.IPv4
		}
	}
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

func getServiceAnnotation(service *v1.Service, name string) (string, bool) {
	if service.Annotations == nil {
		return "", false
	}
	val, ok := service.Annotations[name]
	return val, ok
}

func getServiceBoolAnnotation(service *v1.Service, name string) bool {
	value, ok := getServiceAnnotation(service, name)
	if !ok {
		return false
	}
	boolValue, err := strconv.ParseBool(value)
	return err == nil && boolValue
}

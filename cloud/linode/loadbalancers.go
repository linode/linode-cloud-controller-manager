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

	"golang.org/x/exp/slices"

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
	maxFirewallRuleLabelLen = 32
	maxIPsPerFirewall       = 255
)

var (
	errNoNodesAvailable = errors.New("No nodes available for nodebalancer")
	errInvalidFWConfig  = errors.New("Specify either an allowList or a denyList for a firewall")
	errTooManyFirewalls = errors.New("Too many firewalls attached to a nodebalancer")
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
	client     Client
	zone       string
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
	rawID := service.GetAnnotations()[annLinodeNodeBalancerID]
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
	if _, ok := service.GetAnnotations()[annLinodeNodeBalancerID]; !ok {
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
		if service.GetAnnotations()[annLinodeNodeBalancerID] != "" {
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

// getNodeBalancerDeviceID gets the deviceID of the nodeBalancer that is attached to the firewall.
func (l *loadbalancers) getNodeBalancerDeviceID(ctx context.Context, firewallID, nbID int) (int, bool, error) {
	devices, err := l.client.ListFirewallDevices(ctx, firewallID, &linodego.ListOptions{})
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

	return 0, false, nil
}

// Updates a service that has a firewallID annotation set.
// If an annotation is set, and the nodebalancer has a firewall that matches the ID, nothing to do
// If there's more than one firewall attached to the node-balancer, an error is returned as its not a supported use case.
// If there's only one firewall attached and it doesn't match what's in the annotation, the new firewall is attached and the old one removed
func (l *loadbalancers) updateFirewallwithID(ctx context.Context, service *v1.Service, nb *linodego.NodeBalancer) error {
	var newFirewallID int
	var err error

	fwID := service.GetAnnotations()[annLinodeCloudFirewallID]
	newFirewallID, err = strconv.Atoi(fwID)
	if err != nil {
		return err
	}

	// See if a firewall is attached to the nodebalancer first.
	firewalls, err := l.client.ListNodeBalancerFirewalls(ctx, nb.ID, &linodego.ListOptions{})
	if err != nil {
		return err
	}
	if len(firewalls) > 1 {
		klog.Errorf("Found more than one firewall attached to nodebalancer: %d, firewall IDs: %v", nb.ID, firewalls)
		return errTooManyFirewalls
	}

	// get the ID of the firewall that is already attached to the nodeBalancer, if we have one.
	var existingFirewallID int
	if len(firewalls) == 1 {
		existingFirewallID = firewalls[0].ID
	}

	// if existing firewall and new firewall differs, attach the new firewall and remove the old.
	if existingFirewallID != newFirewallID {
		// attach new firewall.
		_, err = l.client.CreateFirewallDevice(ctx, newFirewallID, linodego.FirewallDeviceCreateOptions{
			ID:   nb.ID,
			Type: "nodebalancer",
		})
		if err != nil {
			return err
		}
		// remove the existing firewall if it exists
		if existingFirewallID != 0 {
			deviceID, deviceExists, err := l.getNodeBalancerDeviceID(ctx, existingFirewallID, nb.ID)
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
	}
	return nil
}

func ipsChanged(ips *linodego.NetworkAddresses, rules []linodego.FirewallRule) bool {
	var ruleIPv4s []string
	var ruleIPv6s []string

	for _, rule := range rules {
		if rule.Addresses.IPv4 != nil {
			ruleIPv4s = append(ruleIPv4s, *rule.Addresses.IPv4...)
		}
		if rule.Addresses.IPv6 != nil {
			ruleIPv6s = append(ruleIPv6s, *rule.Addresses.IPv6...)
		}
	}

	if len(ruleIPv4s) > 0 && ips.IPv4 == nil {
		return true
	}

	if len(ruleIPv6s) > 0 && ips.IPv6 == nil {
		return true
	}

	if ips.IPv4 != nil {
		for _, ipv4 := range *ips.IPv4 {
			if !slices.Contains(ruleIPv4s, ipv4) {
				return true
			}
		}
	}

	if ips.IPv6 != nil {
		for _, ipv6 := range *ips.IPv6 {
			if !slices.Contains(ruleIPv6s, ipv6) {
				return true
			}
		}
	}

	return false
}

func firewallRuleChanged(old linodego.FirewallRuleSet, newACL aclConfig) bool {
	var ips *linodego.NetworkAddresses
	if newACL.AllowList != nil {
		// this is a allowList, this means that the rules should have `DROP` as inboundpolicy
		if old.InboundPolicy != "DROP" {
			return true
		}
		if (newACL.AllowList.IPv4 != nil || newACL.AllowList.IPv6 != nil) && len(old.Inbound) == 0 {
			return true
		}
		ips = newACL.AllowList
	}

	if newACL.DenyList != nil {
		if old.InboundPolicy != "ACCEPT" {
			return true
		}

		if (newACL.DenyList.IPv4 != nil || newACL.DenyList.IPv6 != nil) && len(old.Inbound) == 0 {
			return true
		}
		ips = newACL.DenyList
	}

	return ipsChanged(ips, old.Inbound)
}

func (l *loadbalancers) updateFWwithACL(ctx context.Context, service *v1.Service, nb *linodego.NodeBalancer) error {
	// See if a firewall is attached to the nodebalancer first.
	firewalls, err := l.client.ListNodeBalancerFirewalls(ctx, nb.ID, &linodego.ListOptions{})
	if err != nil {
		return err
	}

	switch len(firewalls) {
	case 0:
		{
			// need to create a fw and attach it to our nb
			fwcreateOpts, err := l.createFirewallOptsForSvc(l.GetLoadBalancerName(ctx, "", service), l.getLoadBalancerTags(ctx, "", service), service)
			if err != nil {
				return err
			}

			fw, err := l.client.CreateFirewall(ctx, *fwcreateOpts)
			if err != nil {
				return err
			}
			// attach new firewall.
			_, err = l.client.CreateFirewallDevice(ctx, fw.ID, linodego.FirewallDeviceCreateOptions{
				ID:   nb.ID,
				Type: "nodebalancer",
			})
			if err != nil {
				return err
			}
		}
	case 1:
		{
			// We do not want to get into the complexity of reconciling differences, might as well just pull what's in the svc annotation now and update the fw.
			var acl aclConfig
			err := json.Unmarshal([]byte(service.GetAnnotations()[annLinodeCloudFirewallACL]), &acl)
			if err != nil {
				return err
			}

			changed := firewallRuleChanged(firewalls[0].Rules, acl)
			if !changed {
				return nil
			}

			fwCreateOpts, err := l.createFirewallOptsForSvc(service.Name, []string{""}, service)
			if err != nil {
				return err
			}
			_, err = l.client.UpdateFirewallRules(ctx, firewalls[0].ID, fwCreateOpts.Rules)
			if err != nil {
				return err
			}
		}
	default:
		klog.Errorf("Found more than one firewall attached to nodebalancer: %d, firewall IDs: %v", nb.ID, firewalls)
		return errTooManyFirewalls
	}
	return nil
}

// updateNodeBalancerFirewall reconciles the firewall attached to the nodebalancer
//
// This function does the following
//  1. If a firewallID annotation is present, it checks if the nodebalancer has a firewall attached, and if it matches the annotationID
//     a. If the IDs match, nothing to do here.
//     b. If they don't match, the nb is attached to the new firewall and removed from the old one.
//  2. If a firewallACL annotation is present,
//     a. it checks if the nodebalancer has a firewall attached, if a fw exists, it updates rules
//     b. if a fw does not exist, it creates one
//  3. If neither of these annotations are present,
//	  a. AND if no firewalls are attached to the nodebalancer, nothing to do.
//	  b. if the NB has ONE firewall attached, remove it from nb, and clean up if nothing else is attached to it
//	  c. If there are more than one fw attached to it, then its a problem, return an err
//  4. If both these annotations are present, the firewallID takes precedence, and the ACL annotation is ignored.
// IF a user creates a fw ID externally, and then switches to using a ACL, the CCM will take over the fw that's attached to the nodebalancer.

func (l *loadbalancers) updateNodeBalancerFirewall(ctx context.Context, service *v1.Service, nb *linodego.NodeBalancer) error {
	// get the new firewall id from the annotation (if any).
	_, fwIDExists := service.GetAnnotations()[annLinodeCloudFirewallID]
	if fwIDExists { // If an ID exists, we ignore everything else and handle just that
		return l.updateFirewallwithID(ctx, service, nb)
	}

	// See if a acl exists
	_, fwACLExists := service.GetAnnotations()[annLinodeCloudFirewallACL]
	if fwACLExists { // if an ACL exists, but no ID, just update the ACL on the fw.
		return l.updateFWwithACL(ctx, service, nb)
	}

	// No firewall ID or ACL annotation, see if there are firewalls attached to our nb
	firewalls, err := l.client.ListNodeBalancerFirewalls(ctx, nb.ID, &linodego.ListOptions{})
	if err != nil {
		return err
	}

	if len(firewalls) == 0 {
		return nil
	}
	if len(firewalls) > 1 {
		klog.Errorf("Found more than one firewall attached to nodebalancer: %d, firewall IDs: %v", nb.ID, firewalls)
		return errTooManyFirewalls
	}

	err = l.client.DeleteFirewallDevice(ctx, firewalls[0].ID, nb.ID)
	if err != nil {
		return err
	}
	// once we delete the device, we should see if there's anything attached to that firewall
	devices, err := l.client.ListFirewallDevices(ctx, firewalls[0].ID, &linodego.ListOptions{})
	if err != nil {
		return err
	}

	if len(devices) == 0 {
		// nothing attached to it, clean it up
		return l.client.DeleteFirewall(ctx, firewalls[0].ID)
	}
	// else let that firewall linger, don't mess with it.

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
	tags := []string{}
	if clusterName != "" {
		tags = append(tags, clusterName)
	}

	tagStr, ok := service.GetAnnotations()[annLinodeLoadBalancerTags]
	if ok {
		return append(tags, strings.Split(tagStr, ",")...)
	}

	return tags
}

func chunkIPs(ips []string) [][]string {
	chunks := [][]string{}
	ipCount := len(ips)

	// If the number of IPs is less than or equal to maxIPsPerFirewall,
	// return a single chunk containing all IPs.
	if ipCount <= maxIPsPerFirewall {
		return [][]string{ips}
	}

	// Otherwise, break the IPs into chunks with maxIPsPerFirewall IPs per chunk.
	chunkCount := 0
	for ipCount > maxIPsPerFirewall {
		start := chunkCount * maxIPsPerFirewall
		end := (chunkCount + 1) * maxIPsPerFirewall
		chunks = append(chunks, ips[start:end])
		chunkCount++
		ipCount -= maxIPsPerFirewall
	}

	// Append the remaining IPs as a chunk.
	chunks = append(chunks, ips[chunkCount*maxIPsPerFirewall:])

	return chunks
}

// processACL takes the IPs, aclType, label etc and formats them into the passed linodego.FirewallCreateOptions pointer.
func processACL(fwcreateOpts *linodego.FirewallCreateOptions, aclType, label, svcName, ports string, ips linodego.NetworkAddresses) {
	ruleLabel := fmt.Sprintf("%s-%s", aclType, svcName)
	if len(ruleLabel) > maxFirewallRuleLabelLen {
		newLabel := ruleLabel[0:maxFirewallRuleLabelLen]
		klog.Infof("Firewall label '%s' is too long. Stripping to '%s'", ruleLabel, newLabel)
		ruleLabel = newLabel
	}

	// Linode has a limitation of firewall rules with a max of 255 IPs per rule
	var ipv4s, ipv6s []string // doing this to avoid dereferencing a nil pointer
	if ips.IPv6 != nil {
		ipv6s = *ips.IPv6
	}
	if ips.IPv4 != nil {
		ipv4s = *ips.IPv4
	}

	if len(ipv4s)+len(ipv6s) > maxIPsPerFirewall {
		ipv4chunks := chunkIPs(ipv4s)
		for i, v4chunk := range ipv4chunks {
			fwcreateOpts.Rules.Inbound = append(fwcreateOpts.Rules.Inbound, linodego.FirewallRule{
				Action:      aclType,
				Label:       ruleLabel,
				Description: fmt.Sprintf("Rule %d, Created by linode-ccm: %s, for %s", i, label, svcName),
				Protocol:    linodego.TCP, // Nodebalancers support only TCP.
				Ports:       ports,
				Addresses:   linodego.NetworkAddresses{IPv4: &v4chunk},
			})
		}

		ipv6chunks := chunkIPs(ipv6s)
		for i, v6chunk := range ipv6chunks {
			fwcreateOpts.Rules.Inbound = append(fwcreateOpts.Rules.Inbound, linodego.FirewallRule{
				Action:      aclType,
				Label:       ruleLabel,
				Description: fmt.Sprintf("Rule %d, Created by linode-ccm: %s, for %s", i, label, svcName),
				Protocol:    linodego.TCP, // Nodebalancers support only TCP.
				Ports:       ports,
				Addresses:   linodego.NetworkAddresses{IPv6: &v6chunk},
			})
		}
	} else {
		fwcreateOpts.Rules.Inbound = append(fwcreateOpts.Rules.Inbound, linodego.FirewallRule{
			Action:      aclType,
			Label:       ruleLabel,
			Description: fmt.Sprintf("Created by linode-ccm: %s, for %s", label, svcName),
			Protocol:    linodego.TCP, // Nodebalancers support only TCP.
			Ports:       ports,
			Addresses:   ips,
		})
	}

	fwcreateOpts.Rules.OutboundPolicy = "ACCEPT"
	if aclType == "ACCEPT" {
		// if an allowlist is present, we drop everything else.
		fwcreateOpts.Rules.InboundPolicy = "DROP"
	} else {
		// if a denylist is present, we accept everything else.
		fwcreateOpts.Rules.InboundPolicy = "ACCEPT"
	}
}

type aclConfig struct {
	AllowList *linodego.NetworkAddresses `json:"allowList"`
	DenyList  *linodego.NetworkAddresses `json:"denyList"`
}

func (l *loadbalancers) createFirewallOptsForSvc(label string, tags []string, svc *v1.Service) (*linodego.FirewallCreateOptions, error) {
	// Fetch acl from annotation
	aclString := svc.GetAnnotations()[annLinodeCloudFirewallACL]
	fwcreateOpts := linodego.FirewallCreateOptions{
		Label: label,
		Tags:  tags,
	}
	servicePorts := make([]string, 0, len(svc.Spec.Ports))
	for _, port := range svc.Spec.Ports {
		servicePorts = append(servicePorts, strconv.Itoa(int(port.Port)))
	}

	portsString := strings.Join(servicePorts[:], ",")
	var acl aclConfig
	err := json.Unmarshal([]byte(aclString), &acl)
	if err != nil {
		return nil, err
	}
	// it is a problem if both are set, or if both are not set
	if (acl.AllowList != nil && acl.DenyList != nil) || (acl.AllowList == nil && acl.DenyList == nil) {
		return nil, errInvalidFWConfig
	}

	aclType := "ACCEPT"
	allowedIPs := acl.AllowList
	if acl.DenyList != nil {
		aclType = "DROP"
		allowedIPs = acl.DenyList
	}

	processACL(&fwcreateOpts, aclType, label, svc.Name, portsString, *allowedIPs)
	return &fwcreateOpts, nil
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

	fwid, ok := service.GetAnnotations()[annLinodeCloudFirewallID]
	if ok {
		firewallID, err := strconv.Atoi(fwid)
		if err != nil {
			return nil, err
		}
		createOpts.FirewallID = firewallID
	} else {
		// There's no firewallID already set, see if we need to create a new fw, look for the acl annotation.
		_, ok := service.GetAnnotations()[annLinodeCloudFirewallACL]
		if ok {
			fwcreateOpts, err := l.createFirewallOptsForSvc(label, tags, service)
			if err != nil {
				return nil, err
			}

			firewall, err := l.client.CreateFirewall(ctx, *fwcreateOpts)
			if err != nil {
				return nil, err
			}
			createOpts.FirewallID = firewall.ID
		}
		// no need to deal with firewalls, continue creating nb's
	}

	return l.client.CreateNodeBalancer(ctx, createOpts)
}

func (l *loadbalancers) createFirewall(ctx context.Context, opts linodego.FirewallCreateOptions) (fw *linodego.Firewall, err error) {
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
		return linodego.NodeBalancerConfig{}, err
	}

	config := linodego.NodeBalancerConfig{
		Port:          port,
		Protocol:      portConfig.Protocol,
		ProxyProtocol: portConfig.ProxyProtocol,
		Check:         health,
	}

	if health == linodego.CheckHTTP || health == linodego.CheckHTTPBody {
		path := service.GetAnnotations()[annLinodeCheckPath]
		if path == "" {
			path = "/"
		}
		config.CheckPath = path
	}

	if health == linodego.CheckHTTPBody {
		body := service.GetAnnotations()[annLinodeCheckBody]
		if body == "" {
			return config, fmt.Errorf("for health check type http_body need body regex annotation %v", annLinodeCheckBody)
		}
		config.CheckBody = body
	}
	checkInterval := 5
	if ci, ok := service.GetAnnotations()[annLinodeHealthCheckInterval]; ok {
		if checkInterval, err = strconv.Atoi(ci); err != nil {
			return config, err
		}
	}
	config.CheckInterval = checkInterval

	checkTimeout := 3
	if ct, ok := service.GetAnnotations()[annLinodeHealthCheckTimeout]; ok {
		if checkTimeout, err = strconv.Atoi(ct); err != nil {
			return config, err
		}
	}
	config.CheckTimeout = checkTimeout

	checkAttempts := 2
	if ca, ok := service.GetAnnotations()[annLinodeHealthCheckAttempts]; ok {
		if checkAttempts, err = strconv.Atoi(ca); err != nil {
			return config, err
		}
	}
	config.CheckAttempts = checkAttempts

	checkPassive := true
	if cp, ok := service.GetAnnotations()[annLinodeHealthCheckPassive]; ok {
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
		if p, ok := service.GetAnnotations()[annLinodeDefaultProtocol]; ok {
			protocol = p
		}
	}
	protocol = strings.ToLower(protocol)

	proxyProtocol := portConfigAnnotation.ProxyProtocol
	if proxyProtocol == "" {
		proxyProtocol = string(linodego.ProxyProtocolNone)
		for _, ann := range []string{annLinodeDefaultProxyProtocol, annLinodeProxyProtocolDeprecated} {
			if pp, ok := service.GetAnnotations()[ann]; ok {
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
	hType, ok := service.GetAnnotations()[annLinodeHealthCheckType]
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

	if connThrottleString := service.GetAnnotations()[annLinodeThrottle]; connThrottleString != "" {
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

func getServiceBoolAnnotation(service *v1.Service, name string) bool {
	value, ok := service.GetAnnotations()[name]
	if !ok {
		return false
	}
	boolValue, err := strconv.ParseBool(value)
	return err == nil && boolValue
}

package linode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/cloudprovider"

	"github.com/linode/linode-cloud-controller-manager/sentry"
	"github.com/linode/linodego"
	"github.com/pkg/errors"
)

const (
	// annLinodeDefaultProtocol is the annotation used to specify the default protocol
	// for Linode load balancers. Options are tcp, http and https. Defaults to tcp.
	annLinodeDefaultProtocol  = "service.beta.kubernetes.io/linode-loadbalancer-default-protocol"
	annLinodePortConfigPrefix = "service.beta.kubernetes.io/linode-loadbalancer-port-"

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
	annLinodeNodeBalancerID       = "service.beta.kubernetes.io/linode-nodebalancer-id"
)

var errLbNotFound = errors.New("loadbalancer not found")

type loadbalancers struct {
	client *linodego.Client
	zone   string

	kubeClient kubernetes.Interface
}

type portConfigAnnotation struct {
	TLSSecretName string `json:"tls-secret-name"`
	Protocol      string `json:"protocol"`
}

type portConfig struct {
	TLSSecretName string
	Protocol      linodego.ConfigProtocol
	Port          int
}

// newLoadbalancers returns a cloudprovider.LoadBalancer whose concrete type is a *loadbalancer.
func newLoadbalancers(client *linodego.Client, zone string) cloudprovider.LoadBalancer {
	return &loadbalancers{client: client, zone: zone}
}

func (l *loadbalancers) getNodeBalancerForService(ctx context.Context, service *v1.Service) (*linodego.NodeBalancer, error) {
	rawID, _ := getServiceAnnotation(service, annLinodeNodeBalancerID)
	id, idErr := strconv.Atoi(rawID)
	hasIDAnn := idErr == nil && id != 0

	if hasIDAnn {
		sentry.SetTag(ctx, "load_balancer_id", rawID)
		nb, err := l.getNodeBalancerByID(ctx, service, id)
		if err != errLbNotFound {
			return nb, err
		}

		// If the specified NodeBalancer does not exist, try to resolve the current
		// one from the status.
		klog.Warningf("could not find NodeBalancer from annotation on service %s; attempting to find from status", getServiceSlug(service))
	}
	return l.getNodeBalancerByStatus(ctx, service)
}

// getNodeBalancerByStatus attempts to get the NodeBalancer from the IPv4 specified in the
// most recent LoadBalancer status.
func (l *loadbalancers) getNodeBalancerByStatus(ctx context.Context, service *v1.Service) (*linodego.NodeBalancer, error) {
	for _, ingress := range service.Status.LoadBalancer.Ingress {
		ipv4 := ingress.IP
		if nb, err := l.getNodeBalancerByIPv4(ctx, service, ipv4); err == nil {
			return nb, err
		}
	}
	return nil, errLbNotFound
}

// cleanupOldNodeBalancer removes the service's disowned NodeBalancer if there is one.
//
// The current NodeBalancer from getNodeBalancerForService is compared to the most recent
// LoadBalancer status; if they are different (because of an updated NodeBalancerID
// annotation), the old one is deleted.
func (l *loadbalancers) cleanupOldNodeBalancer(ctx context.Context, service *v1.Service) error {
	previousNB, err := l.getNodeBalancerByStatus(ctx, service)
	if err != nil {
		if err == errLbNotFound {
			return nil
		}
		return err
	}

	nb, err := l.getNodeBalancerForService(ctx, service)
	if err != nil {
		return err
	}

	if previousNB.ID == nb.ID {
		return nil
	}
	return l.client.DeleteNodeBalancer(ctx, previousNB.ID)
}

// GetLoadBalancer returns the *v1.LoadBalancerStatus of service.
//
// GetLoadBalancer will not modify service.
func (l *loadbalancers) GetLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (*v1.LoadBalancerStatus, bool, error) {
	ctx = sentry.SetHubOnContext(ctx)
	sentry.SetTag(ctx, "cluster_name", clusterName)
	sentry.SetTag(ctx, "service", service.Name)

	nb, err := l.getNodeBalancerForService(ctx, service)
	if err != nil {
		if err == errLbNotFound {
			return nil, false, nil
		}

		sentry.CaptureError(ctx, err)
		return nil, false, err
	}

	return getLoadBalancerStatus(nb), true, nil
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

	nb, err = l.getNodeBalancerForService(ctx, service)
	switch err {
	case errLbNotFound:
		if nb, err = l.buildLoadBalancerRequest(ctx, service, nodes); err != nil {
			sentry.CaptureError(ctx, err)
			return nil, err
		}

	case nil:
		if err = l.UpdateLoadBalancer(ctx, clusterName, service, nodes); err != nil {
			sentry.CaptureError(ctx, err)
			return nil, err
		}

	default:
		sentry.CaptureError(ctx, err)
		return nil, err
	}

	lbStatus = getLoadBalancerStatus(nb)

	if !l.shouldPreserveNodeBalancer(service) {
		if err := l.cleanupOldNodeBalancer(ctx, service); err != nil {
			sentry.CaptureError(ctx, err)
			return nil, err
		}
	}

	return lbStatus, nil
}

// UpdateLoadBalancer updates the NodeBalancer to have configs that match the Service's ports
//nolint:funlen
func (l *loadbalancers) UpdateLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) error {
	ctx = sentry.SetHubOnContext(ctx)
	sentry.SetTag(ctx, "cluster_name", clusterName)
	sentry.SetTag(ctx, "service", service.Name)

	lb, err := l.getNodeBalancerForService(ctx, service)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return err
	}

	connThrottle := getConnectionThrottle(service)
	if connThrottle != lb.ClientConnThrottle {
		update := lb.GetUpdateOptions()
		update.ClientConnThrottle = &connThrottle

		lb, err = l.client.UpdateNodeBalancer(ctx, lb.ID, update)
		if err != nil {
			sentry.CaptureError(ctx, err)
			return err
		}
	}

	// Get all of the NodeBalancer's configs
	nbCfgs, err := l.client.ListNodeBalancerConfigs(ctx, lb.ID, nil)
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
		newNBCfg, err := l.buildNodeBalancerConfig(service, int(port.Port))
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

			currentNBCfg, err = l.client.CreateNodeBalancerConfig(ctx, lb.ID, createOpts)
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

		if _, err = l.client.RebuildNodeBalancerConfig(ctx, lb.ID, currentNBCfg.ID, rebuildOpts); err != nil {
			sentry.CaptureError(ctx, err)
			return fmt.Errorf("[port %d] error rebuilding NodeBalancer config: %v", int(port.Port), err)
		}
	}
	return nil
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
	preserveRaw, ok := getServiceAnnotation(service, annLinodeLoadBalancerPreserve)
	if !ok {
		return false
	}
	preserve, err := strconv.ParseBool(preserveRaw)
	return err == nil && preserve
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

	// Don't delete the underlying nodebalancer if the service has the preserve annotation.
	if l.shouldPreserveNodeBalancer(service) {
		return nil
	}

	// GetLoadBalancer will capture any errors it gets for Sentry, so it's unnecessary to capture
	// them again here.
	_, exists, err := l.GetLoadBalancer(ctx, clusterName, service)
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}

	lb, err := l.getNodeBalancerForService(ctx, service)
	if err != nil {
		sentry.CaptureError(ctx, err)
		return err
	}

	if err = l.client.DeleteNodeBalancer(ctx, lb.ID); err != nil {
		sentry.CaptureError(ctx, err)
		return err
	}

	return nil
}

func (l *loadbalancers) getNodeBalancerByIPv4(ctx context.Context, service *v1.Service, ipv4 string) (*linodego.NodeBalancer, error) {
	lbs, err := l.client.ListNodeBalancers(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, lb := range lbs {
		if *lb.IPv4 == ipv4 {
			klog.Infof("found NodeBalancer (%d) for service %s/%s via IPv4 (%s)", lb.ID, service.Namespace, service.Name, ipv4)
			return &lb, nil
		}
	}
	return nil, errLbNotFound
}

func (l *loadbalancers) getNodeBalancerByID(ctx context.Context, service *v1.Service, id int) (lb *linodego.NodeBalancer, err error) {
	lb, err = l.client.GetNodeBalancer(ctx, id)
	if err != nil {
		if apiErr, ok := err.(*linodego.Error); ok && apiErr.Code == http.StatusNotFound {
			err = errLbNotFound
		}
	} else {
		klog.Infof("found NodeBalancer (%d) for service %s/%s via %s annotation", id, service.Namespace, service.Name, annLinodeNodeBalancerID)
	}
	return
}

// The returned error will be errLbNotFound if the load balancer does not exist.
func (l *loadbalancers) lbByName(ctx context.Context, name string) (*linodego.NodeBalancer, error) {
	jsonFilter, err := json.Marshal(map[string]string{"label": name})
	if err != nil {
		return nil, err
	}
	lbs, err := l.client.ListNodeBalancers(ctx, linodego.NewListOptions(0, string(jsonFilter)))
	if err != nil {
		return nil, err
	}

	if len(lbs) > 0 {
		return &lbs[0], nil
	}

	return nil, errLbNotFound
}

func (l *loadbalancers) createNodeBalancer(ctx context.Context, service *v1.Service, configs []*linodego.NodeBalancerConfigCreateOptions) (lb *linodego.NodeBalancer, err error) {
	connThrottle := getConnectionThrottle(service)
	createOpts := linodego.NodeBalancerCreateOptions{
		Region:             l.zone,
		ClientConnThrottle: &connThrottle,
		Configs:            configs,
	}
	return l.client.CreateNodeBalancer(ctx, createOpts)
}

//nolint:funlen
func (l *loadbalancers) buildNodeBalancerConfig(service *v1.Service, port int) (linodego.NodeBalancerConfig, error) {
	portConfig, err := getPortConfig(service, port)
	if err != nil {
		return linodego.NodeBalancerConfig{}, err
	}

	health, err := getHealthCheckType(service)
	if err != nil {
		return linodego.NodeBalancerConfig{}, nil
	}

	config := linodego.NodeBalancerConfig{
		Port:     port,
		Protocol: portConfig.Protocol,
		Check:    health,
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
		if err = l.addTLSCert(service, &config, portConfig); err != nil {
			return config, err
		}
	}

	return config, nil
}

func (l *loadbalancers) addTLSCert(service *v1.Service, nbConfig *linodego.NodeBalancerConfig, config portConfig) error {
	err := l.retrieveKubeClient()
	if err != nil {
		return err
	}

	nbConfig.SSLCert, nbConfig.SSLKey, err = getTLSCertInfo(l.kubeClient, service.Namespace, config)
	if err != nil {
		return err
	}
	return nil
}

// buildLoadBalancerRequest returns a linodego.NodeBalancer
// requests for service across nodes.
func (l *loadbalancers) buildLoadBalancerRequest(ctx context.Context, service *v1.Service, nodes []*v1.Node) (*linodego.NodeBalancer, error) {
	ports := service.Spec.Ports
	configs := make([]*linodego.NodeBalancerConfigCreateOptions, 0, len(ports))

	for _, port := range ports {
		if port.Protocol == v1.ProtocolUDP {
			return nil, fmt.Errorf("error creating NodeBalancer Config: ports with the UDP protocol are not supported")
		}

		config, err := l.buildNodeBalancerConfig(service, int(port.Port))
		if err != nil {
			return nil, err
		}
		createOpt := config.GetCreateOptions()

		for _, n := range nodes {
			createOpt.Nodes = append(createOpt.Nodes, l.buildNodeBalancerNodeCreateOptions(n, port.NodePort))
		}

		configs = append(configs, &createOpt)
	}
	return l.createNodeBalancer(ctx, service, configs)
}

func (l *loadbalancers) buildNodeBalancerNodeCreateOptions(node *v1.Node, nodePort int32) linodego.NodeBalancerNodeCreateOptions {
	return linodego.NodeBalancerNodeCreateOptions{
		Address: fmt.Sprintf("%v:%v", getNodeInternalIP(node), nodePort),
		Label:   node.Name,
		Mode:    "accept",
		Weight:  100,
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
		var ok bool
		protocol, ok = service.Annotations[annLinodeDefaultProtocol]
		if !ok {
			protocol = "tcp"
		}
	}

	protocol = strings.ToLower(protocol)

	if protocol != "tcp" && protocol != "http" && protocol != "https" {
		return portConfig, fmt.Errorf("invalid protocol: %q specified", protocol)
	}

	portConfig.Port = port
	portConfig.Protocol = linodego.ConfigProtocol(protocol)
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
	annotationKey := annLinodePortConfigPrefix + strconv.Itoa(port)
	annotationJSON, ok := service.Annotations[annotationKey]
	if !ok {
		return tryDeprecatedTLSAnnotation(service, port)
	}

	annotation := portConfigAnnotation{}
	err := json.Unmarshal([]byte(annotationJSON), &annotation)
	if err != nil {
		return annotation, err
	}

	return annotation, nil
}

func getNodeInternalIP(node *v1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == v1.NodeInternalIP {
			return addr.Address
		}
	}
	return ""
}

func getTLSCertInfo(kubeClient kubernetes.Interface, namespace string, config portConfig) (string, string, error) {
	if config.TLSSecretName == "" {
		return "", "", fmt.Errorf("TLS secret name for port %v is not specified", config.Port)
	}

	secret, err := kubeClient.CoreV1().Secrets(namespace).Get(config.TLSSecretName, metav1.GetOptions{})
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

func getLoadBalancerStatus(nb *linodego.NodeBalancer) *v1.LoadBalancerStatus {
	return &v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{{
			IP:       *nb.IPv4,
			Hostname: *nb.Hostname,
		}},
	}
}

func getServiceSlug(service *v1.Service) string {
	return fmt.Sprintf("%s/%s", service.Namespace, service.Name)
}

func getServiceAnnotation(service *v1.Service, name string) (id string, ok bool) {
	if service.Annotations == nil {
		id, ok = "", false
	}
	id, ok = service.Annotations[name]
	return
}

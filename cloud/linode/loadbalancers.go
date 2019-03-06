package linode

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/linode/linodego"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/cloudprovider"
)

const (
	// annLinodeProtocol is the annotation used to specify the default protocol
	// for Linode load balancers. For ports specifed in annLinodeTLSPorts, this protocol
	// is overwritten to https. Options are tcp, http and https. Defaults to tcp.
	annLinodeProtocol = "service.beta.kubernetes.io/linode-loadbalancer-protocol"

	// annLinodeTLSPorts is the annotation used to specify which ports of the loadbalancer
	// should use the https protocol. This is a comma separated list of ports
	// (e.g. 443,6443,7443).
	annLinodeTLSPorts = "service.beta.kubernetes.io/linode-loadbalancer-tls-ports"

	// annLinodeTLSPassThrough is the annotation used to specify whether the
	// Linode loadbalancer should pass encrypted data to backend Linodes.
	// This is optional and defaults to false.
	annLinodeCheckPath       = "service.beta.kubernetes.io/linode-loadbalancer-check-path"
	annLinodeCheckBody       = "service.beta.kubernetes.io/linode-loadbalancer-check-body"
	annLinodeHealthCheckType = "service.beta.kubernetes.io/linode-loadbalancer-check-type"

	// annLinodeCertificateID is the annotation specifying the certificate ID
	// used for https protocol. This annoataion is required if annLinodeTLSPorts
	// is passed.
	annLinodeSSLCertificate = "service.beta.kubernetes.io/linode-loadbalancer-ssl-cert"
	annLinodeSSLKey         = "service.beta.kubernetes.io/linode-loadbalancer-ssl-key"

	annLinodeHealthCheckInterval = "service.beta.kubernetes.io/linode-loadbalancer-check-interval"
	annLinodeHealthCheckTimeout  = "service.beta.kubernetes.io/linode-loadbalancer-check-timeout"
	annLinodeHealthCheckAttempts = "service.beta.kubernetes.io/linode-loadbalancer-check-attempts"
	annLinodeHealthCheckPassive  = "service.beta.kubernetes.io/linode-loadbalancer-check-passive"

	annLinodeSessionPersistence = "service.beta.kubernetes.io/linode-loadbalancer-stickiness"

	// annLinodeAlgorithm is the annotation specifying which algorithm Linode loadbalancer
	// should use. Options are round_robin and least_connections. Defaults
	// to round_robin.
	annLinodeAlgorithm = "service.beta.kubernetes.io/linode-loadbalancer-algorithm"
)

var lbNotFound = errors.New("loadbalancer not found")

type loadbalancers struct {
	client *linodego.Client
	zone   string
}

// newLoadbalancers returns a cloudprovider.LoadBalancer whose concrete type is a *loadbalancer.
func newLoadbalancers(client *linodego.Client, zone string) cloudprovider.LoadBalancer {
	return &loadbalancers{client: client, zone: zone}
}

// GetLoadBalancer returns the *v1.LoadBalancerStatus of service.
//
// GetLoadBalancer will not modify service.
func (l *loadbalancers) GetLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (*v1.LoadBalancerStatus, bool, error) {
	lbName := cloudprovider.GetLoadBalancerName(service)
	lb, err := l.lbByName(ctx, l.client, lbName)
	if err != nil {
		if err == lbNotFound {
			return nil, false, nil
		}

		return nil, false, err
	}

	return &v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{
			{
				IP:       *lb.IPv4,
				Hostname: *lb.Hostname,
			},
		},
	}, true, nil
}

// EnsureLoadBalancer ensures that the cluster is running a load balancer for
// service.
//
// EnsureLoadBalancer will not modify service or nodes.
func (l *loadbalancers) EnsureLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	_, exists, err := l.GetLoadBalancer(ctx, clusterName, service)
	if err != nil {
		return nil, err
	}

	if !exists {
		lb, err := l.buildLoadBalancerRequest(ctx, service, nodes)
		if err != nil {
			return nil, err
		}

		return &v1.LoadBalancerStatus{
			Ingress: []v1.LoadBalancerIngress{
				{
					IP:       *lb.IPv4,
					Hostname: *lb.Hostname,
				},
			},
		}, nil
	}

	err = l.UpdateLoadBalancer(ctx, clusterName, service, nodes)
	if err != nil {
		return nil, err
	}

	lbStatus, _, err := l.GetLoadBalancer(ctx, clusterName, service)
	if err != nil {
		return nil, err
	}

	return lbStatus, nil
}

// UpdateLoadBalancer updates the NodeBalancer to have configs that match the Service's ports
func (l *loadbalancers) UpdateLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) error {
	// Get the NodeBalancer
	lbName := cloudprovider.GetLoadBalancerName(service)
	lb, err := l.lbByName(ctx, l.client, lbName)
	if err != nil {
		return err
	}

	// Get all of the NodeBalancer's configs
	nbCfgs, err := l.client.ListNodeBalancerConfigs(ctx, lb.ID, nil)
	if err != nil {
		return err
	}

	// Delete any configs for ports that have been removed from the Service
	if err = l.deleteUnusedConfigs(ctx, nbCfgs, service.Spec.Ports); err != nil {
		return err
	}

	// Add or overwrite configs for each of the Service's ports
	for _, port := range service.Spec.Ports {
		// Construct a new config for this port
		newNBCfg, err := l.buildNodeBalancerConfig(service, int(port.Port))
		if err != nil {
			return err
		}

		// Add all of the Nodes to the config
		var newNBNodes []linodego.NodeBalancerNodeCreateOptions
		for _, node := range nodes {
			newNBNodes = append(newNBNodes, l.buildNodeBalancerNodeCreateOptions(node, port.NodePort))
		}

		// Look for an existing config for this port
		var existingNBCfg *linodego.NodeBalancerConfig
		for _, nbc := range nbCfgs {
			if nbc.Port == int(port.Port) {
				existingNBCfg = &nbc
				break
			}
		}

		// If there's an existing config, rebuild it, otherwise, create it
		if existingNBCfg != nil {
			rebuildOpts := newNBCfg.GetRebuildOptions()
			rebuildOpts.Nodes = newNBNodes

			if _, err = l.client.RebuildNodeBalancerConfig(ctx, lb.ID, existingNBCfg.ID, rebuildOpts); err != nil {
				return fmt.Errorf("Error rebuilding NodeBalancer config: %v", err)
			}
		} else {
			createOpts := newNBCfg.GetCreateOptions()
			createOpts.Nodes = newNBNodes

			_, err := l.client.CreateNodeBalancerConfig(ctx, lb.ID, createOpts)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Delete any NodeBalancer configs for ports that no longer exist on the Service
// Note: Don't build a map or other lookup structure here, it is not worth the overhead
func (l *loadbalancers) deleteUnusedConfigs(ctx context.Context, nbConfigs []linodego.NodeBalancerConfig, servicePorts []v1.ServicePort) error {
	for _, nbc := range nbConfigs {
		for _, sp := range servicePorts {
			if nbc.Port == int(sp.Port) {
				continue
			}
		}
		if err := l.client.DeleteNodeBalancerConfig(ctx, nbc.NodeBalancerID, nbc.ID); err != nil {
			return err
		}
	}
	return nil
}

// EnsureLoadBalancerDeleted deletes the specified loadbalancer if it exists.
// nil is returned if the load balancer for service does not exist or is
// successfully deleted.
//
// EnsureLoadBalancerDeleted will not modify service.
func (l *loadbalancers) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *v1.Service) error {
	_, exists, err := l.GetLoadBalancer(ctx, clusterName, service)
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}
	lbName := cloudprovider.GetLoadBalancerName(service)
	lb, err := l.lbByName(ctx, l.client, lbName)
	if err != nil {
		return err
	}
	return l.client.DeleteNodeBalancer(ctx, lb.ID)
}

// The returned error will be lbNotFound if the load balancer does not exist.
func (l *loadbalancers) lbByName(ctx context.Context, client *linodego.Client, name string) (*linodego.NodeBalancer, error) {
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

	return nil, lbNotFound
}

func (l *loadbalancers) createNodeBalancer(ctx context.Context, service *v1.Service, configs []*linodego.NodeBalancerConfigCreateOptions) (*linodego.NodeBalancer, error) {
	lbName := cloudprovider.GetLoadBalancerName(service)

	connThrottle := 20
	createOpts := linodego.NodeBalancerCreateOptions{
		Label:              &lbName,
		Region:             l.zone,
		ClientConnThrottle: &connThrottle,
		Configs:            configs,
	}
	return l.client.CreateNodeBalancer(ctx, createOpts)
}

func (l *loadbalancers) buildNodeBalancerConfig(service *v1.Service, port int) (linodego.NodeBalancerConfig, error) {
	protocol, err := getProtocol(service)
	if err != nil {
		return linodego.NodeBalancerConfig{}, err
	}
	health, err := getHealthCheckType(service)
	if err != nil {
		return linodego.NodeBalancerConfig{}, nil
	}

	config := linodego.NodeBalancerConfig{
		Port:       port,
		Protocol:   protocol,
		Algorithm:  getAlgorithm(service),
		Stickiness: getStickiness(service),
		Check:      health,
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

	if protocol == linodego.ProtocolHTTPS {
		isTLS, err := isTLSPort(service, port)
		if err != nil {
			return config, err
		}
		if isTLS {
			cert, key := getSSLCertInfo(service)
			if cert == "" && key == "" {
				return config, fmt.Errorf("must set %v and %v annotation for https protocol", annLinodeSSLCertificate, annLinodeSSLKey)
			}
			if cert != "" {
				config.SSLCert = cert
			}
			if key != "" {
				config.SSLKey = key
			}
		}
	}

	return config, nil
}

// buildLoadBalancerRequest returns a linodego.NodeBalancer
// requests for service across nodes.
func (l *loadbalancers) buildLoadBalancerRequest(ctx context.Context, service *v1.Service, nodes []*v1.Node) (*linodego.NodeBalancer, error) {
	var configs []*linodego.NodeBalancerConfigCreateOptions

	ports := service.Spec.Ports
	for _, port := range ports {
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
		Address: fmt.Sprintf("%v:%v", getNodeInternalIp(node), nodePort),
		Label:   node.Name,
		Mode:    "accept",
		Weight:  100,
	}
}

// getProtocol returns the desired protocol of service.
func getProtocol(service *v1.Service) (linodego.ConfigProtocol, error) {
	protocol, ok := service.Annotations[annLinodeProtocol]
	if !ok {
		return linodego.ProtocolTCP, nil
	}

	if protocol != "tcp" && protocol != "http" && protocol != "https" {
		return "", fmt.Errorf("invalid protocol: %q specifed in annotation: %q", protocol, annLinodeProtocol)
	}

	return linodego.ConfigProtocol(protocol), nil
}

func getHealthCheckType(service *v1.Service) (linodego.ConfigCheck, error) {
	hType, ok := service.Annotations[annLinodeHealthCheckType]
	if !ok {
		return linodego.CheckConnection, nil
	}
	if hType != "connection" && hType != "http" && hType != "http_body" {
		return "", fmt.Errorf("invalid health check type: %q specifed in annotation: %q", hType, annLinodeHealthCheckType)
	}
	return linodego.ConfigCheck(hType), nil
}

func isTLSPort(service *v1.Service, port int) (bool, error) {
	tlsPorts, ok := service.Annotations[annLinodeTLSPorts]
	if !ok {
		return false, nil
	}
	tlsPortsSlice := strings.Split(tlsPorts, ",")
	for _, p := range tlsPortsSlice {
		tlsPort, err := strconv.Atoi(p)
		if err != nil {
			return false, err
		}
		if port == tlsPort {
			return true, nil
		}
	}
	return false, nil

}

// getTLSPorts returns the ports of service that are set to use TLS.
func getTLSPorts(service *v1.Service) ([]int, error) {
	tlsPorts, ok := service.Annotations[annLinodeTLSPorts]
	if !ok {
		return nil, nil
	}

	tlsPortsSlice := strings.Split(tlsPorts, ",")

	tlsPortsInt := make([]int, len(tlsPortsSlice))
	for i, tlsPort := range tlsPortsSlice {
		port, err := strconv.Atoi(tlsPort)
		if err != nil {
			return nil, err
		}

		tlsPortsInt[i] = port
	}

	return tlsPortsInt, nil
}

func getNodeInternalIp(node *v1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == v1.NodeInternalIP {
			return addr.Address
		}
	}
	return ""
}

// getAlgorithm returns the load balancing algorithm to use for service.
// round_robin is returned when service does not specify an algorithm.
func getAlgorithm(service *v1.Service) linodego.ConfigAlgorithm {
	algo := service.Annotations[annLinodeAlgorithm]

	switch algo {
	case "least_connections":
		return linodego.AlgorithmLeastConn
	case "source":
		return linodego.AlgorithmSource
	default:
		return linodego.AlgorithmRoundRobin
	}
}

func getStickiness(service *v1.Service) linodego.ConfigStickiness {
	stickiness := service.Annotations[annLinodeSessionPersistence]

	switch stickiness {
	case "none":
		return linodego.StickinessNone
	case "http_cookie":
		return linodego.StickinessHTTPCookie
	default:
		return linodego.StickinessTable

	}
}

func getSSLCertInfo(service *v1.Service) (string, string) {
	cert := service.Annotations[annLinodeSSLCertificate]
	if cert != "" {
		cb, _ := base64.StdEncoding.DecodeString(cert)
		cert = string(cb)
	}
	key := service.Annotations[annLinodeSSLKey]
	if key != "" {
		kb, _ := base64.StdEncoding.DecodeString(key)
		key = string(kb)
	}
	return cert, key
}

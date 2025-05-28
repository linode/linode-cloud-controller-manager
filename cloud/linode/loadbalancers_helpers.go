package linode

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"

	"github.com/linode/linode-cloud-controller-manager/cloud/annotations"
)

const (
	udpCheckPortDefault = 80
	udpCheckPortMin     = 1
	udpCheckPortMax     = 65535
)

// getPortProtocol returns the protocol for a given service port.
// It checks the portConfigAnnotationResult for a specific port.
// If not found, it checks the service annotations for the service.
// It also validates the protocol against a list of valid protocols.
func getPortProtocol(portConfigAnnotationResult portConfigAnnotation, service *v1.Service, port v1.ServicePort) (string, error) {
	protocol := portConfigAnnotationResult.Protocol
	if protocol == "" {
		protocol = string(port.Protocol)
		if p, ok := service.GetAnnotations()[annotations.AnnLinodeDefaultProtocol]; ok {
			protocol = p
		}
	}
	protocol = strings.ToLower(protocol)
	if !validProtocols[protocol] {
		return "", fmt.Errorf("invalid protocol: %q specified", protocol)
	}
	return protocol, nil
}

// getPortProxyProtocol returns the proxy protocol for a given service port.
// It checks the portConfigAnnotationResult for a specific port.
// If not found, it checks the service annotations for the service.
// It also validates the proxy protocol against a list of valid proxy protocols.
// If the protocol is UDP, it checks if the proxy protocol is set to none.
// It also checks if a TLS secret name is specified for UDP, which is not allowed.
// It returns the proxy protocol as a string.
func getPortProxyProtocol(portConfigAnnotationResult portConfigAnnotation, service *v1.Service, protocol linodego.ConfigProtocol) (string, error) {
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
	proxyProtocol = strings.ToLower(proxyProtocol)

	if !validProxyProtocols[proxyProtocol] {
		return "", fmt.Errorf("invalid NodeBalancer proxy protocol value '%s'", proxyProtocol)
	}

	if protocol == linodego.ProtocolUDP {
		if proxyProtocol != string(linodego.ProxyProtocolNone) {
			return "", fmt.Errorf("proxy protocol [%s] is not supported for UDP", proxyProtocol)
		}
	}
	return proxyProtocol, nil
}

// getPortAlgorithm returns the algorithm for a given service port.
// It checks the portConfigAnnotationResult for a specific port.
// If not found, it checks the service annotations for the service.
// It also validates the algorithm against a list of valid algorithms.
// If the protocol is UDP, it checks if the algorithm is valid for UDP.
func getPortAlgorithm(portConfigAnnotationResult portConfigAnnotation, service *v1.Service, protocol linodego.ConfigProtocol) (string, error) {
	algorithm := portConfigAnnotationResult.Algorithm
	if algorithm == "" {
		algorithm = string(linodego.AlgorithmRoundRobin)
		if a, ok := service.GetAnnotations()[annotations.AnnLinodeDefaultAlgorithm]; ok {
			algorithm = a
		}
	}
	algorithm = strings.ToLower(algorithm)

	if protocol == linodego.ProtocolUDP {
		if !validUDPAlgorithms[algorithm] {
			return "", fmt.Errorf("invalid algorithm: %q specified for UDP protocol", algorithm)
		}
	} else {
		if !validTCPAlgorithms[algorithm] {
			return "", fmt.Errorf("invalid algorithm: %q specified for TCP/HTTP/HTTPS protocol", algorithm)
		}
	}
	return algorithm, nil
}

// getPortUDPCheckPort returns the UDP check port for a given service port.
// It checks the portConfigAnnotationResult for a specific port.
// If not found, it checks the service annotations for the service.
// It also validates the UDP check port against a range of valid ports (1-65535).
func getPortUDPCheckPort(portConfigAnnotationResult portConfigAnnotation, service *v1.Service, protocol linodego.ConfigProtocol) (int, error) {
	udpCheckPort := udpCheckPortDefault
	if protocol != linodego.ProtocolUDP {
		return udpCheckPort, nil
	}

	if portConfigAnnotationResult.UDPCheckPort != "" {
		cp, err := strconv.Atoi(portConfigAnnotationResult.UDPCheckPort)
		if err != nil {
			return udpCheckPort, err
		}
		udpCheckPort = cp
	} else if udpPort, ok := service.GetAnnotations()[annotations.AnnLinodeUDPCheckPort]; ok {
		cp, err := strconv.Atoi(udpPort)
		if err != nil {
			return udpCheckPort, err
		}
		udpCheckPort = cp
	}

	// Validate the UDP check port to be between 1 and 65535
	if udpCheckPort < udpCheckPortMin || udpCheckPort > udpCheckPortMax {
		return udpCheckPort, fmt.Errorf("UDPCheckPort must be between 1 and 65535, got %d", udpCheckPort)
	}
	return udpCheckPort, nil
}

// getDefaultStickiness returns the default stickiness for a given protocol.
// For UDP, it returns StickinessSession, and for other protocols, it returns StickinessTable.
func getDefaultStickiness(protocol string) linodego.ConfigStickiness {
	if protocol == string(linodego.ProtocolUDP) {
		return linodego.StickinessSession
	} else {
		return linodego.StickinessTable
	}
}

// getPortStickiness returns the stickiness for a given service port.
// It checks the portConfigAnnotationResult for a specific port.
// If not found, it checks the service annotations for the service.
// It also validates the stickiness against a list of valid stickiness options.
func getPortStickiness(portConfigAnnotationResult portConfigAnnotation, service *v1.Service, protocol linodego.ConfigProtocol) (string, error) {
	stickiness := portConfigAnnotationResult.Stickiness
	if stickiness == "" {
		stickiness = string(getDefaultStickiness(string(protocol)))
		if s, ok := service.GetAnnotations()[annotations.AnnLinodeDefaultStickiness]; ok {
			stickiness = s
		}
	}
	stickiness = strings.ToLower(stickiness)

	switch protocol {
	case linodego.ProtocolHTTP:
		if !validHTTPStickiness[stickiness] {
			return "", fmt.Errorf("invalid stickiness: %q specified for HTTP protocol", stickiness)
		}
	case linodego.ProtocolHTTPS:
		if !validHTTPSStickiness[stickiness] {
			return "", fmt.Errorf("invalid stickiness: %q specified for HTTPS protocol", stickiness)
		}
	case linodego.ProtocolUDP:
		if !validUDPStickiness[stickiness] {
			return "", fmt.Errorf("invalid stickiness: %q specified for UDP protocol", stickiness)
		}
	case linodego.ProtocolTCP:
		// For TCP, we don't validate stickiness as it is not applicable.
	}

	return stickiness, nil
}

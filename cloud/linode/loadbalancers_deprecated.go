package linode

import (
	"encoding/json"

	v1 "k8s.io/api/core/v1"
)

const (
	annLinodeProtocolDeprecated        = "service.beta.kubernetes.io/linode-loadbalancer-protocol"
	annLinodeLoadBalancerTLSDeprecated = "service.beta.kubernetes.io/linode-loadbalancer-tls"
)

type tlsAnnotationDeprecated struct {
	TLSSecretName string `json:"tls-secret-name"`
	Port          int    `json:"port"`
}

func tryDeprecatedTLSAnnotation(service *v1.Service, port int) (portConfigAnnotation, error) {
	annotation := portConfigAnnotation{}
	tlsAnnotation, err := getTLSAnnotationDeprecated(service, port)
	if err != nil {
		return annotation, err
	}

	if tlsAnnotation != nil {
		annotation.Protocol = "https"
		annotation.TLSSecretName = tlsAnnotation.TLSSecretName
	} else if protocol, ok := service.Annotations[annLinodeProtocolDeprecated]; ok {
		annotation.Protocol = protocol
	}
	return annotation, nil
}

func getTLSAnnotationDeprecated(service *v1.Service, port int) (*tlsAnnotationDeprecated, error) {
	annotationJSON, ok := service.Annotations[annLinodeLoadBalancerTLSDeprecated]
	if !ok {
		return nil, nil
	}
	tlsAnnotations := make([]*tlsAnnotationDeprecated, 0)
	err := json.Unmarshal([]byte(annotationJSON), &tlsAnnotations)
	if err != nil {
		return nil, err
	}
	for _, tlsAnnotation := range tlsAnnotations {
		if tlsAnnotation.Port == port {
			return tlsAnnotation, nil
		}
	}
	return nil, nil
}

package framework

import (
	"context"
	"fmt"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/util/retry"
)

func (i *lbInvocation) createOrUpdateService(selector, annotations map[string]string, ports []core.ServicePort, isSessionAffinityClientIP bool, isCreate bool) error {
	var sessionAffinity core.ServiceAffinity = "None"
	if isSessionAffinityClientIP {
		sessionAffinity = "ClientIP"
	}
	svc := &core.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        TestServerResourceName,
			Namespace:   i.Namespace(),
			Annotations: annotations,
			Labels: map[string]string{
				"app": "test-server-" + i.app,
			},
		},
		Spec: core.ServiceSpec{
			Ports:           ports,
			Selector:        selector,
			Type:            core.ServiceTypeLoadBalancer,
			SessionAffinity: sessionAffinity,
		},
	}

	service := i.kubeClient.CoreV1().Services(i.Namespace())
	if isCreate {
		_, err := service.Create(context.TODO(), svc, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	} else {
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			options := metav1.GetOptions{}
			resource, err := service.Get(context.TODO(), TestServerResourceName, options)
			if err != nil {
				return err
			}
			svc.ObjectMeta.ResourceVersion = resource.ResourceVersion
			svc.Spec.ClusterIP = resource.Spec.ClusterIP
			_, err = service.Update(context.TODO(), svc, metav1.UpdateOptions{})
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

func (i *lbInvocation) GetServiceWatcher() (watch.Interface, error) {
	var timeoutSeconds int64 = 30
	watcher, err := i.kubeClient.CoreV1().Events(i.Namespace()).Watch(context.TODO(), metav1.ListOptions{
		FieldSelector:  "involvedObject.kind=Service",
		Watch:          true,
		TimeoutSeconds: &timeoutSeconds,
	})
	if err != nil {
		return nil, err
	}
	return watcher, nil
}

func (i *lbInvocation) CreateService(selector, annotations map[string]string, ports []core.ServicePort, isSessionAffinityClientIP bool) error {
	return i.createOrUpdateService(selector, annotations, ports, isSessionAffinityClientIP, true)
}

func (i *lbInvocation) UpdateService(selector, annotations map[string]string, ports []core.ServicePort, isSessionAffinityClientIP bool) error {
	err := i.deleteEvents()
	if err != nil {
		return err
	}
	return i.createOrUpdateService(selector, annotations, ports, isSessionAffinityClientIP, false)
}

func (i *lbInvocation) DeleteService() error {
	return i.kubeClient.CoreV1().Services(i.Namespace()).Delete(context.TODO(), TestServerResourceName, metav1.DeleteOptions{})
}

func (i *lbInvocation) GetServiceEndpoints() ([]core.EndpointAddress, error) {
	ep, err := i.kubeClient.CoreV1().Endpoints(i.Namespace()).Get(context.TODO(), TestServerResourceName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if len(ep.Subsets) == 0 {
		return nil, fmt.Errorf("No service endpoints found for %s", TestServerResourceName)
	}
	return ep.Subsets[0].Addresses, err
}

func (i *lbInvocation) deleteEvents() error {
	return i.kubeClient.CoreV1().Events(i.Namespace()).DeleteCollection(context.TODO(), metav1.DeleteOptions{}, metav1.ListOptions{FieldSelector: "involvedObject.kind=Service"})
}

func (i *lbInvocation) GetLoadBalancerIps() ([]string, error) {
	svc, err := i.kubeClient.CoreV1().Services(i.Namespace()).Get(context.TODO(), TestServerResourceName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	var serverAddr []string
	for _, ingress := range svc.Status.LoadBalancer.Ingress {
		if len(svc.Spec.Ports) > 0 {
			for _, port := range svc.Spec.Ports {
				if port.NodePort > 0 {
					serverAddr = append(serverAddr, fmt.Sprintf("%s:%d", ingress.IP, port.Port))
				}
			}
		}
	}
	if serverAddr == nil {
		return nil, fmt.Errorf("failed to get Status.LoadBalancer.Ingress for service %s/%s", TestServerResourceName, i.Namespace())
	}
	return serverAddr, nil
}

func (i *lbInvocation) getServiceIngress(name, namespace string) ([]core.LoadBalancerIngress, error) {
	svc, err := i.kubeClient.CoreV1().Services(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if svc.Status.LoadBalancer.Ingress == nil {
		return nil, fmt.Errorf("Status.LoadBalancer.Ingress is empty for %s", name)
	}
	return svc.Status.LoadBalancer.Ingress, nil
}

func (i *lbInvocation) testServerServicePorts() []core.ServicePort {
	return []core.ServicePort{
		{
			Name:       "http-1",
			Port:       80,
			TargetPort: intstr.FromInt(8080),
			Protocol:   "TCP",
		},
	}
}

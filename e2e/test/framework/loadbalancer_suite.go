package framework

import (
	"context"
	"fmt"
	"time"

	"github.com/appscode/go/wait"
	"github.com/linode/linodego"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (i *lbInvocation) GetHTTPEndpoints() ([]string, error) {
	return i.getLoadBalancerURLs()
}

func (i *lbInvocation) GetNodeBalancerID(svcName string) (int, error) {
	hostname, err := i.waitForLoadBalancerHostname(svcName)
	if err != nil {
		return -1, err
	}

	nbList, errListNodeBalancers := i.linodeClient.ListNodeBalancers(context.Background(), nil)

	if errListNodeBalancers != nil {
		return -1, errListNodeBalancers
	}

	for _, nb := range nbList {
		if *nb.Hostname == hostname {
			return nb.ID, nil
		}
	}
	return -1, fmt.Errorf("no NodeBalancer Found for service %v", svcName)
}

func (i *lbInvocation) WaitForNodeBalancerReady(svcName string, expectedID int) error {
	return wait.PollImmediate(time.Millisecond*500, RetryTimeout, func() (bool, error) {
		nbID, err := i.GetNodeBalancerID(svcName)
		if err != nil {
			return false, err
		}
		return nbID == expectedID, nil
	})
}

func (i *lbInvocation) GetNodeBalancerConfig(svcName string) (*linodego.NodeBalancerConfig, error) {
	id, err := i.GetNodeBalancerID(svcName)
	if err != nil {
		return nil, err
	}
	nbcList, err := i.linodeClient.ListNodeBalancerConfigs(context.Background(), id, nil)
	if err != nil {
		return nil, err
	}
	return &nbcList[0], nil
}

func (i *lbInvocation) GetNodeBalancerConfigForPort(svcName string, port int) (*linodego.NodeBalancerConfig, error) {
	id, err := i.GetNodeBalancerID(svcName)
	if err != nil {
		return nil, err
	}
	nbConfigs, err := i.linodeClient.ListNodeBalancerConfigs(context.Background(), id, nil)
	if err != nil {
		return nil, err
	}

	for _, config := range nbConfigs {
		if config.Port == port {
			return &config, nil
		}
	}
	return nil, fmt.Errorf("NodeBalancerConfig for port %d was not found", port)
}

func (i *lbInvocation) waitForLoadBalancerHostname(svcName string) (string, error) {
	var ip string
	err := wait.PollImmediate(RetryInterval, RetryTimeout, func() (bool, error) {
		svc, err := i.kubeClient.CoreV1().Services(i.Namespace()).Get(context.TODO(), svcName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if svc.Status.LoadBalancer.Ingress == nil {
			return false, nil
		}
		ip = svc.Status.LoadBalancer.Ingress[0].Hostname
		return true, nil
	})

	return ip, err
}

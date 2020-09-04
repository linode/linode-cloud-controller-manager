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
	ip, err := i.waitForLoadBalancerIP(svcName)
	if err != nil {
		return -1, err
	}

	nbList, errListNodeBalancers := i.linodeClient.ListNodeBalancers(context.Background(), nil)

	if errListNodeBalancers != nil {
		return -1, errListNodeBalancers
	}

	for _, nb := range nbList {
		if *nb.IPv4 == ip {
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

func (i *lbInvocation) waitForLoadBalancerIP(svcName string) (string, error) {
	var ip string
	err := wait.PollImmediate(RetryInterval, RetryTimeout, func() (bool, error) {
		svc, err := i.kubeClient.CoreV1().Services(i.Namespace()).Get(svcName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if svc.Status.LoadBalancer.Ingress == nil {
			return false, nil
		}
		ip = svc.Status.LoadBalancer.Ingress[0].IP
		return true, nil
	})

	return ip, err
}

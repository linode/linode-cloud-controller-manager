package framework

import (
	"context"
	"fmt"

	"github.com/linode/linodego"
)

func (i *lbInvocation) GetNodeBalancerFromService(svcName string, checkIP bool) (*linodego.NodeBalancer, error) {
	ingress, err := i.getServiceIngress(svcName, i.Namespace())
	if err != nil {
		return nil, err
	}
	hostname := ingress[0].Hostname
	ip := ingress[0].IP
	nbList, errListNodeBalancers := i.linodeClient.ListNodeBalancers(context.Background(), nil)
	if errListNodeBalancers != nil {
		return nil, fmt.Errorf("Error listingNodeBalancer for hostname %s: %s", hostname, errListNodeBalancers.Error())
	}

	for _, nb := range nbList {
		if *nb.Hostname == hostname {
			if checkIP {
				if *nb.IPv4 == ip {
					return &nb, nil
				} else {
					return nil, fmt.Errorf("IPv4 for Nodebalancer (%s) does not match IP (%s) for service %v", *nb.IPv4, ip, svcName)
				}
			}
			return &nb, nil
		}
	}
	return nil, fmt.Errorf("no NodeBalancer Found for service %v", svcName)
}

func (i *lbInvocation) GetNodeBalancerID(svcName string) (int, error) {
	nb, err := i.GetNodeBalancerFromService(svcName, false)
	if err != nil {
		return -1, err
	}
	return nb.ID, nil
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

func (i *lbInvocation) GetNodeBalancerUpNodes(svcName string) (int, error) {
	id, err := i.GetNodeBalancerID(svcName)
	if err != nil {
		return 0, err
	}
	nbcList, err := i.linodeClient.ListNodeBalancerConfigs(context.Background(), id, nil)
	if err != nil {
		return 0, err
	}
	nb := &nbcList[0]
	return nb.NodesStatus.Up, nil
}

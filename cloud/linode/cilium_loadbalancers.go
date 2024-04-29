package linode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2alpha1"
	ciliumclient "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/typed/cilium.io/v2alpha1"
	slimv1 "github.com/cilium/cilium/pkg/k8s/slim/k8s/apis/meta/v1"
	"github.com/google/uuid"
	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

const (
	ciliumLBClass = "io.cilium/bgp-control-plane"
	ipHolderLabel = "linode-ccm-ip-holder"
)

var noBGPSelector = errors.New("no BGP node selector set to configure IP sharing")

// createSharedIP requests an additional IP that can be shared on Nodes to support
// loadbalancing via Cilium LB IPAM + BGP Control Plane.
func (l *loadbalancers) createSharedIP(ctx context.Context, nodes []*v1.Node) (string, error) {
	if Options.BGPNodeSelector == "" {
		return "", noBGPSelector
	}

	ipHolder, err := l.ensureIPHolder(ctx)
	if err != nil {
		return "", err
	}

	newSharedIP, err := l.client.AddInstanceIPAddress(ctx, ipHolder.ID, true)
	if err != nil {
		return "", err
	}

	// need to retrieve existing public IPs on the IP holder since ShareIPAddresses
	// expects the full list of IPs to be shared
	ipv4PublicAddrs := []string{}
	addrResp, err := l.client.GetInstanceIPAddresses(ctx, ipHolder.ID)
	if err != nil {
		return "", err
	}
	for _, addr := range addrResp.IPv4.Public {
		ipv4PublicAddrs = append(ipv4PublicAddrs, addr.Address)
	}

	// share the IPs with nodes participating in Cilium BGP peering
	kv := strings.Split(Options.BGPNodeSelector, "=")
	for _, node := range nodes {
		if val, ok := node.Labels[kv[0]]; ok && len(kv) == 2 && val == kv[1] {
			nodeLinodeID, err := parseProviderID(node.Spec.ProviderID)
			if err != nil {
				return "", err
			}

			if err = l.client.ShareIPAddresses(ctx, linodego.IPAddressesShareOptions{
				IPs:      ipv4PublicAddrs,
				LinodeID: nodeLinodeID,
			}); err != nil {
				return "", err
			}
			klog.Infof("shared IPs %v on Linode %d", ipv4PublicAddrs, nodeLinodeID)
		}
	}

	return newSharedIP.Address, nil
}

// deleteSharedIP cleans up the shared IP for a LoadBalancer Service if it was assigned
// by Cilium LB IPAM, removing it from the ip-holder
func (l *loadbalancers) deleteSharedIP(ctx context.Context, service *v1.Service) error {
	if Options.BGPNodeSelector == "" {
		return errors.New("no BGP node label set to configure IP sharing")
	}
	err := l.retrieveKubeClient()
	if err != nil {
		return err
	}
	nodeList, err := l.kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: Options.BGPNodeSelector,
	})
	if err != nil {
		return err
	}
	bgpNodes := nodeList.Items
	ipHolder, err := l.getIPHolder(ctx)
	if err != nil {
		// return error or nil if not found since no IP holder means there
		// is no IP to reclaim
		return IgnoreLinodeAPIError(err, http.StatusNotFound)
	}
	svcIngress := service.Status.LoadBalancer.Ingress
	if len(svcIngress) > 0 && ipHolder != nil {
		for _, ingress := range svcIngress {
			// delete the shared IP on the Linodes it's shared on
			for _, node := range bgpNodes {
				nodeLinodeID, err := parseProviderID(node.Spec.ProviderID)
				if err != nil {
					return err
				}
				err = l.client.DeleteInstanceIPAddress(ctx, nodeLinodeID, ingress.IP)
				if IgnoreLinodeAPIError(err, http.StatusNotFound) != nil {
					return err
				}
			}

			// finally delete the shared IP on the ip-holder
			err = l.client.DeleteInstanceIPAddress(ctx, ipHolder.ID, ingress.IP)
			if IgnoreLinodeAPIError(err, http.StatusNotFound) != nil {
				return err
			}
		}
	}

	return nil
}

// To hold the IP in lieu of a proper IP reservation system, a special Nanode is
// created but not booted and used to hold all shared IPs.
func (l *loadbalancers) ensureIPHolder(ctx context.Context) (*linodego.Instance, error) {
	ipHolder, err := l.getIPHolder(ctx)
	if err != nil {
		return nil, err
	}
	if ipHolder != nil {
		return ipHolder, nil
	}

	ipHolder, err = l.client.CreateInstance(ctx, linodego.InstanceCreateOptions{
		Region:   l.zone,
		Type:     "g6-nanode-1",
		Label:    ipHolderLabel,
		RootPass: uuid.NewString(),
		Image:    "linode/ubuntu22.04",
		Booted:   Pointer(false),
	})
	if err != nil {
		return nil, err
	}

	return ipHolder, nil
}

func (l *loadbalancers) getIPHolder(ctx context.Context) (*linodego.Instance, error) {
	filter := map[string]string{"label": ipHolderLabel}
	rawFilter, err := json.Marshal(filter)
	if err != nil {
		panic("this should not have failed")
	}
	var ipHolder *linodego.Instance
	linodes, err := l.client.ListInstances(ctx, linodego.NewListOptions(1, string(rawFilter)))
	if err != nil {
		return nil, err
	}
	if len(linodes) > 0 {
		ipHolder = &linodes[0]
	}

	return ipHolder, nil
}

func (l *loadbalancers) retrieveCiliumClientset() error {
	if l.ciliumClient != nil {
		return nil
	}
	var (
		kubeConfig *rest.Config
		err        error
	)
	kubeconfigFlag := Options.KubeconfigFlag
	if kubeconfigFlag == nil || kubeconfigFlag.Value.String() == "" {
		kubeConfig, err = rest.InClusterConfig()
	} else {
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigFlag.Value.String())
	}
	if err != nil {
		return err
	}
	l.ciliumClient, err = ciliumclient.NewForConfig(kubeConfig)

	return err
}

// for LoadBalancer Services not backed by a NodeBalancer, a CiliumLoadBalancerIPPool resource
// will be created specifically for the Service with the requested shared IP
// NOTE: Cilium CRDs must be installed for this to work
func (l *loadbalancers) createCiliumLBIPPool(ctx context.Context, service *v1.Service, sharedIP string) (*v2alpha1.CiliumLoadBalancerIPPool, error) {
	if err := l.retrieveCiliumClientset(); err != nil {
		return nil, err
	}
	ciliumLBIPPool := &v2alpha1.CiliumLoadBalancerIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s-pool", service.Namespace, service.Name),
		},
		Spec: v2alpha1.CiliumLoadBalancerIPPoolSpec{
			ServiceSelector: &slimv1.LabelSelector{
				MatchLabels: map[string]slimv1.MatchLabelsValue{
					"io.kubernetes.service.namespace": service.Namespace,
					"io.kubernetes.service.name":      service.Name,
				},
			},
			Blocks: []v2alpha1.CiliumLoadBalancerIPPoolIPBlock{{
				Cidr: v2alpha1.IPv4orIPv6CIDR(fmt.Sprintf("%s/32", sharedIP)),
			}},
			Disabled: false,
		},
	}

	return l.ciliumClient.CiliumLoadBalancerIPPools().Create(ctx, ciliumLBIPPool, metav1.CreateOptions{})
}

// NOTE: Cilium CRDs must be installed for this to work
func (l *loadbalancers) deleteCiliumLBIPPool(ctx context.Context, service *v1.Service) error {
	if err := l.retrieveCiliumClientset(); err != nil {
		return err
	}

	return l.ciliumClient.CiliumLoadBalancerIPPools().Delete(
		ctx,
		fmt.Sprintf("%s-%s-pool", service.Namespace, service.Name),
		metav1.DeleteOptions{},
	)
}

// NOTE: Cilium CRDs must be installed for this to work
func (l *loadbalancers) getCiliumLBIPPool(ctx context.Context, service *v1.Service) (*v2alpha1.CiliumLoadBalancerIPPool, error) {
	if err := l.retrieveCiliumClientset(); err != nil {
		return nil, err
	}

	return l.ciliumClient.CiliumLoadBalancerIPPools().Get(
		ctx,
		fmt.Sprintf("%s-%s-pool", service.Namespace, service.Name),
		metav1.GetOptions{},
	)
}

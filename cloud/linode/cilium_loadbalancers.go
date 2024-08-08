package linode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2alpha1"
	ciliumclient "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/typed/cilium.io/v2alpha1"
	slimv1 "github.com/cilium/cilium/pkg/k8s/slim/k8s/apis/meta/v1"
	"github.com/google/uuid"
	"github.com/linode/linode-cloud-controller-manager/cloud/annotations"
	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

const (
	ciliumLBClass              = "io.cilium/bgp-control-plane"
	ipHolderLabelPrefix        = "linode-ccm-ip-holder"
	ciliumBGPPeeringPolicyName = "linode-ccm-bgp-peering"
)

// This mapping is unfortunately necessary since there is no way to get the
// numeric ID for a data center from the API.
// These values come from https://www.linode.com/docs/products/compute/compute-instances/guides/failover/#ip-sharing-availability
var (
	regionIDMap = map[string]int{
		"us-southeast": 4,  // Atlanta, GA (USA)
		"us-ord":       18, // Chicago, IL (USA)
		"us-central":   2,  // Dallas, TX (USA)
		// "us-west"   : 3,  // Fremont, CA (USA) UNDERGOING NETWORK UPGRADES
		"us-lax":     30, // Los Angeles, CA (USA)
		"us-mia":     28, // Miami, FL (USA)
		"us-east":    6,  // Newark, NJ (USA)
		"us-sea":     20, // Seattle, WA (USA)
		"us-iad":     17, // Washington, DC (USA)
		"ca-central": 15, // Toronto (Canada)
		"br-gru":     21, // São Paulo (Brazil)
		// EMEA
		"nl-ams":     22, // Amsterdam (Netherlands)
		"eu-central": 10, // Frankfurt (Germany)
		"eu-west":    7,  // London (United Kingdom)
		"it-mil":     27, // Milan (Italy)
		"ap-west":    14, // Mumbai (India)
		"fr-par":     19, // Paris (France)
		"se-sto":     23, // Stockholm (Sweden)
		// APAC
		"in-maa":       25, // Chennai (India)
		"id-cgk":       29, // Jakarta (Indonesia)
		"jp-osa":       26, // Osaka (Japan)
		"ap-south":     9,  // Singapore
		"ap-southeast": 16, // Sydney (Australia)
		"ap-northeast": 11, // Tokyo (Japan)
	}
)

// getExistingSharedIPsInCluster determines the list of addresses to share on nodes by checking the
// CiliumLoadBalancerIPPools created by the CCM in createCiliumLBIPPool
// NOTE: Cilium CRDs must be installed for this to work
func (l *loadbalancers) getExistingSharedIPsInCluster(ctx context.Context) ([]string, error) {
	addrs := []string{}
	if err := l.retrieveCiliumClientset(); err != nil {
		return addrs, err
	}
	pools, err := l.ciliumClient.CiliumLoadBalancerIPPools().List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/managed-by=linode-ccm",
	})
	if err != nil {
		return addrs, err
	}
	for _, pool := range pools.Items {
		for _, block := range pool.Spec.Blocks {
			addrs = append(addrs, strings.TrimSuffix(string(block.Cidr), "/32"))
		}
	}
	return addrs, nil
}

func (l *loadbalancers) getExistingSharedIPs(ctx context.Context, ipHolder *linodego.Instance) ([]string, error) {
	if ipHolder == nil {
		return nil, nil
	}
	ipHolderAddrs, err := l.client.GetInstanceIPAddresses(ctx, ipHolder.ID)
	if err != nil {
		return nil, err
	}
	addrs := make([]string, 0, len(ipHolderAddrs.IPv4.Shared))
	for _, addr := range ipHolderAddrs.IPv4.Shared {
		addrs = append(addrs, addr.Address)
	}
	return addrs, nil
}

// shareIPs shares the given list of IP addresses on the given Node
func (l *loadbalancers) shareIPs(ctx context.Context, addrs []string, node *v1.Node) error {
	nodeLinodeID, err := parseProviderID(node.Spec.ProviderID)
	if err != nil {
		return err
	}
	if err = l.retrieveKubeClient(); err != nil {
		return err
	}
	if err = l.client.ShareIPAddresses(ctx, linodego.IPAddressesShareOptions{
		IPs:      addrs,
		LinodeID: nodeLinodeID,
	}); err != nil {
		return err
	}
	// need to make sure node is up-to-date
	node, err = l.kubeClient.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}
	node.Labels[annotations.AnnLinodeNodeIPSharingUpdated] = "true"
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := l.kubeClient.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
		return err
	})
	if retryErr != nil {
		klog.Infof("could not update Node: %s", retryErr.Error())
		return retryErr
	}

	klog.Infof("shared IPs %v on Linode %d", addrs, nodeLinodeID)

	return nil
}

// handleIPSharing makes sure that the appropriate Nodes that are labeled to
// perform IP sharing (via a specified node selector) have the expected IPs shared
// in the event that a Node joins the cluster after the LoadBalancer Service already
// exists
func (l *loadbalancers) handleIPSharing(ctx context.Context, node *v1.Node) error {
	// ignore cases where the provider ID has been set
	if node.Spec.ProviderID == "" {
		klog.Info("skipping IP while providerID is unset")
		return nil
	}
	// If performing Service load-balancing via IP sharing + BGP, check for a special annotation
	// added by the CCM gets set when load-balancer IPs have been successfully shared on the node
	if Options.BGPNodeSelector != "" {
		kv := strings.Split(Options.BGPNodeSelector, "=")
		// Check if node should be participating in IP sharing via the given selector
		if val, ok := node.Labels[kv[0]]; !ok || len(kv) != 2 || val != kv[1] {
			// not a selected Node
			return nil
		}
	}
	// check if node has been updated with IPs to share
	if _, foundIpSharingUpdatedLabel := node.Labels[annotations.AnnLinodeNodeIPSharingUpdated]; foundIpSharingUpdatedLabel {
		// IPs are already shared on the Node
		return nil
	}
	// Get the IPs to be shared on the Node and configure sharing.
	// This also annotates the node that IPs have been shared.
	inClusterAddrs, err := l.getExistingSharedIPsInCluster(ctx)
	if err != nil {
		klog.Infof("error getting shared IPs in cluster: %s", err.Error())
		return err
	}
	// if any of the addrs don't exist on the ip-holder (e.g. someone manually deleted it outside the CCM),
	// we need to exclude that from the list
	// TODO: also clean up the CiliumLoadBalancerIPPool for that missing IP if that happens
	ipHolder, err := l.getIPHolder(ctx)
	if err != nil {
		return err
	}
	ipHolderAddrs, err := l.getExistingSharedIPs(ctx, ipHolder)
	if err != nil {
		klog.Infof("error getting shared IPs in cluster: %s", err.Error())
		return err
	}
	addrs := []string{}
	for _, i := range inClusterAddrs {
		if slices.Contains(ipHolderAddrs, i) {
			addrs = append(addrs, i)
		}
	}
	if err = l.shareIPs(ctx, addrs, node); err != nil {
		klog.Infof("error sharing IPs: %s", err.Error())
		return err
	}

	return nil
}

// createSharedIP requests an additional IP that can be shared on Nodes to support
// loadbalancing via Cilium LB IPAM + BGP Control Plane.
func (l *loadbalancers) createSharedIP(ctx context.Context, nodes []*v1.Node) (string, error) {
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
	inClusterAddrs, err := l.getExistingSharedIPsInCluster(ctx)
	if err != nil {
		return "", err
	}
	inClusterAddrs = append(inClusterAddrs, newSharedIP.Address)
	// if any of the addrs don't exist on the ip-holder (e.g. someone manually deleted it outside the CCM),
	// we need to exclude that from the list
	// TODO: also clean up the CiliumLoadBalancerIPPool for that missing IP if that happens
	ipHolderAddrs, err := l.getExistingSharedIPs(ctx, ipHolder)
	if err != nil {
		klog.Infof("error getting shared IPs in cluster: %s", err.Error())
		return "", err
	}
	addrs := []string{}
	for _, i := range inClusterAddrs {
		if slices.Contains(ipHolderAddrs, i) {
			addrs = append(addrs, i)
		}
	}

	// share the IPs with nodes participating in Cilium BGP peering
	if Options.BGPNodeSelector == "" {
		for _, node := range nodes {
			if err = l.shareIPs(ctx, addrs, node); err != nil {
				return "", err
			}
		}
	} else {
		kv := strings.Split(Options.BGPNodeSelector, "=")
		for _, node := range nodes {
			if val, ok := node.Labels[kv[0]]; ok && len(kv) == 2 && val == kv[1] {
				if err = l.shareIPs(ctx, addrs, node); err != nil {
					return "", err
				}
			}
		}
	}

	return newSharedIP.Address, nil
}

// deleteSharedIP cleans up the shared IP for a LoadBalancer Service if it was assigned
// by Cilium LB IPAM, removing it from the ip-holder
func (l *loadbalancers) deleteSharedIP(ctx context.Context, service *v1.Service) error {
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
		Label:    fmt.Sprintf("%s-%s", ipHolderLabelPrefix, l.zone),
		RootPass: uuid.NewString(),
		Image:    "linode/ubuntu22.04",
		Booted:   ptr.To(false),
	})
	if err != nil {
		return nil, err
	}

	return ipHolder, nil
}

func (l *loadbalancers) getIPHolder(ctx context.Context) (*linodego.Instance, error) {
	filter := map[string]string{"label": fmt.Sprintf("%s-%s", ipHolderLabelPrefix, l.zone)}
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
			Name:   fmt.Sprintf("%s-%s-pool", service.Namespace, service.Name),
			Labels: map[string]string{"app.kubernetes.io/managed-by": "linode-ccm"},
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

// NOTE: Cilium CRDs must be installed for this to work
func (l *loadbalancers) ensureCiliumBGPPeeringPolicy(ctx context.Context) error {
	regionID, ok := regionIDMap[l.zone]
	if !ok {
		return fmt.Errorf("unsupported region for BGP: %s", l.zone)
	}
	if err := l.retrieveCiliumClientset(); err != nil {
		return err
	}
	// check if policy already exists
	policy, err := l.ciliumClient.CiliumBGPPeeringPolicies().Get(ctx, ciliumBGPPeeringPolicyName, metav1.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		klog.Infof("Failed to get CiliumBGPPeeringPolicy: %s", err.Error())
		return err
	}
	// if the CiliumBGPPeeringPolicy doesn't exist, it's not nil, just empty
	if policy != nil && policy.Name != "" {
		return nil
	}

	// otherwise create it
	var nodeSelector slimv1.LabelSelector
	// If no BGPNodeSelector is specified, select all nodes by default.
	if Options.BGPNodeSelector == "" {
		nodeSelector = slimv1.LabelSelector{}
	} else {
		kv := strings.Split(Options.BGPNodeSelector, "=")
		if len(kv) != 2 {
			return fmt.Errorf("invalid node selector %s", Options.BGPNodeSelector)
		}

		nodeSelector = slimv1.LabelSelector{MatchLabels: map[string]string{kv[0]: kv[1]}}
	}

	ciliumBGPPeeringPolicy := &v2alpha1.CiliumBGPPeeringPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: ciliumBGPPeeringPolicyName,
		},
		Spec: v2alpha1.CiliumBGPPeeringPolicySpec{
			NodeSelector: &nodeSelector,
			VirtualRouters: []v2alpha1.CiliumBGPVirtualRouter{{
				LocalASN:      65001,
				ExportPodCIDR: ptr.To(true),
				ServiceSelector: &slimv1.LabelSelector{
					// By default, virtual routers will not announce any services.
					// This selector makes it so all services within the cluster are announced.
					// See https://docs.cilium.io/en/stable/network/bgp-control-plane/#service-announcements
					// for more information.
					MatchExpressions: []slimv1.LabelSelectorRequirement{{
						Key:      "somekey",
						Operator: slimv1.LabelSelectorOpNotIn,
						Values:   []string{"never-used-value"},
					}},
				},
			}},
		},
	}
	// As in https://github.com/linode/lelastic, there are 4 peers per DC
	for i := 1; i <= 4; i++ {
		neighbor := v2alpha1.CiliumBGPNeighbor{
			PeerAddress:             fmt.Sprintf("2600:3c0f:%d:34::%d/64", regionID, i),
			PeerASN:                 65000,
			EBGPMultihopTTL:         ptr.To(int32(10)),
			ConnectRetryTimeSeconds: ptr.To(int32(5)),
			HoldTimeSeconds:         ptr.To(int32(9)),
			KeepAliveTimeSeconds:    ptr.To(int32(3)),
			AdvertisedPathAttributes: []v2alpha1.CiliumBGPPathAttributes{
				{
					SelectorType: "CiliumLoadBalancerIPPool",
					Communities: &v2alpha1.BGPCommunities{
						Standard: []v2alpha1.BGPStandardCommunity{"65000:1", "65000:2"},
					},
				},
			},
		}
		ciliumBGPPeeringPolicy.Spec.VirtualRouters[0].Neighbors = append(ciliumBGPPeeringPolicy.Spec.VirtualRouters[0].Neighbors, neighbor)
	}

	klog.Info("Creating CiliumBGPPeeringPolicy")
	_, err = l.ciliumClient.CiliumBGPPeeringPolicies().Create(ctx, ciliumBGPPeeringPolicy, metav1.CreateOptions{})

	return err
}

package linode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2alpha1"
	ciliumclient "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/typed/cilium.io/v2alpha1"
	slimv1 "github.com/cilium/cilium/pkg/k8s/slim/k8s/apis/meta/v1"
	"github.com/google/uuid"
	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	"github.com/linode/linode-cloud-controller-manager/cloud/annotations"
)

const (
	ciliumLBClass              = "io.cilium/bgp-control-plane"
	ipHolderLabelPrefix        = "linode-ccm-ip-holder"
	ciliumBGPPeeringPolicyName = "linode-ccm-bgp-peering"
	defaultBGPPeerPrefix       = "2600:3c0f"
	commonControlPlaneLabel    = "node-role.kubernetes.io/control-plane"
)

// This mapping is unfortunately necessary since there is no way to get the
// numeric ID for a data center from the API.
// These values come from https://www.linode.com/docs/products/compute/compute-instances/guides/failover/#ip-sharing-availability
var (
	regionIDMap = map[string]int{
		"nl-ams":       22, // Amsterdam (Netherlands)
		"us-southeast": 4,  // Atlanta, GA (USA)
		"in-maa":       25, // Chennai (India)
		"us-ord":       18, // Chicago, IL (USA)
		"us-central":   2,  // Dallas, TX (USA)
		"eu-central":   10, // Frankfurt (Germany)
		// "us-west":       3,  // Fremont, CA (USA) Undergoing network upgrades
		"id-cgk":       29, // Jakarta (Indonesia)
		"eu-west":      7,  // London (United Kingdom)
		"gb-lon":       44, // London 2 (United Kingdom)
		"us-lax":       30, // Los Angeles, CA (USA)
		"es-mad":       24, // Madrid (Spain)
		"au-mel":       45, // Melbourne (Australia)
		"us-mia":       28, // Miami, FL (USA)
		"it-mil":       27, // Milan (Italy)
		"ap-west":      14, // Mumbai (India)
		"in-bom-2":     46, // Mumbai 2 (India)
		"us-east":      6,  // Newark, NJ (USA)
		"jp-osa":       26, // Osaka (Japan)
		"fr-par":       19, // Paris (France)
		"br-gru":       21, // São Paulo (Brazil)
		"us-sea":       20, // Seattle, WA (USA)
		"ap-south":     9,  // Singapore
		"sg-sin-2":     48, // Singapore 2
		"se-sto":       23, // Stockholm (Sweden)
		"ap-southeast": 16, // Sydney (Australia)
		"ap-northeast": 11, // Tokyo (Japan)
		"ca-central":   15, // Toronto (Canada)
		"us-iad":       17, // Washington, DC (USA)
	}
	BGPNodeSelectorFlagInputLen int = 2
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
	addrs := make([]string, 0, len(ipHolderAddrs.IPv4.Public))
	for _, addr := range ipHolderAddrs.IPv4.Public {
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
func (l *loadbalancers) handleIPSharing(ctx context.Context, node *v1.Node, ipHolderSuffix string) error {
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
	} else if _, ok := node.Labels[commonControlPlaneLabel]; ok {
		// If there is no node selector specified, default to sharing across worker nodes only
		return nil
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
	ipHolder, err := l.getIPHolder(ctx, ipHolderSuffix)
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
func (l *loadbalancers) createSharedIP(ctx context.Context, nodes []*v1.Node, ipHolderSuffix string) (string, error) {
	ipHolder, err := l.ensureIPHolder(ctx, ipHolderSuffix)
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
	// if any of the addrs don't exist on the ip-holder (e.g. someone manually deleted it outside the CCM),
	// we need to exclude that from the list
	// TODO: also clean up the CiliumLoadBalancerIPPool for that missing IP if that happens
	ipHolderAddrs, err := l.getExistingSharedIPs(ctx, ipHolder)
	if err != nil {
		klog.Infof("error getting shared IPs in cluster: %s", err.Error())
		return "", err
	}
	addrs := []string{newSharedIP.Address}
	for _, i := range inClusterAddrs {
		if slices.Contains(ipHolderAddrs, i) {
			addrs = append(addrs, i)
		}
	}

	// share the IPs with nodes participating in Cilium BGP peering
	if Options.BGPNodeSelector == "" {
		for _, node := range nodes {
			if _, ok := node.Labels[commonControlPlaneLabel]; !ok {
				if err = l.shareIPs(ctx, addrs, node); err != nil {
					return "", err
				}
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

	serviceNn := getServiceNn(service)
	var ipHolderSuffix string
	if Options.IpHolderSuffix != "" {
		ipHolderSuffix = Options.IpHolderSuffix
		klog.V(3).Infof("using parameter-based IP Holder suffix %s for Service %s", ipHolderSuffix, serviceNn)
	}

	ipHolder, err := l.getIPHolder(ctx, ipHolderSuffix)
	if err != nil {
		// return error or nil if not found since no IP holder means there
		// is no IP to reclaim
		return IgnoreLinodeAPIError(err, http.StatusNotFound)
	}
	svcIngress := service.Status.LoadBalancer.Ingress
	if len(svcIngress) > 0 && ipHolder != nil {
		var nodeLinodeID int

		for _, ingress := range svcIngress {
			// delete the shared IP on the Linodes it's shared on
			for _, node := range bgpNodes {
				nodeLinodeID, err = parseProviderID(node.Spec.ProviderID)
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
func (l *loadbalancers) ensureIPHolder(ctx context.Context, suffix string) (*linodego.Instance, error) {
	ipHolder, err := l.getIPHolder(ctx, suffix)
	if err != nil {
		return nil, err
	}
	if ipHolder != nil {
		return ipHolder, nil
	}
	label := generateClusterScopedIPHolderLinodeName(l.zone, suffix)
	ipHolder, err = l.client.CreateInstance(ctx, linodego.InstanceCreateOptions{
		Region:   l.zone,
		Type:     "g6-nanode-1",
		Label:    label,
		RootPass: uuid.NewString(),
		Image:    "linode/ubuntu22.04",
		Booted:   ptr.To(false),
	})
	if err != nil {
		if linodego.ErrHasStatus(err, http.StatusBadRequest) && strings.Contains(err.Error(), "Label must be unique") {
			// TODO (rk): should we handle more status codes on error?
			klog.Errorf("failed to create new IP Holder instance %s since it already exists: %s", label, err.Error())
			return nil, err
		}
		return nil, err
	}
	klog.Infof("created new IP Holder instance %s", label)

	return ipHolder, nil
}

func (l *loadbalancers) getIPHolder(ctx context.Context, suffix string) (*linodego.Instance, error) {
	// even though we have updated the naming convention, leaving this in ensures we have backwards compatibility
	filter := map[string]string{"label": fmt.Sprintf("%s-%s", ipHolderLabelPrefix, l.zone)}
	rawFilter, err := json.Marshal(filter)
	if err != nil {
		panic("this should not have failed")
	}
	var ipHolder *linodego.Instance
	// TODO (rk): should we switch to using GET instead of LIST? we would be able to wrap logic around errors
	linodes, err := l.client.ListInstances(ctx, linodego.NewListOptions(1, string(rawFilter)))
	if err != nil {
		return nil, err
	}
	if len(linodes) == 0 {
		// since a list that returns 0 results has a 200/OK status code (no error)

		// we assume that either
		// a) an ip holder instance does not exist yet
		// or
		// b) another cluster already holds the linode grant to an ip holder using the old naming convention
		filter = map[string]string{"label": generateClusterScopedIPHolderLinodeName(l.zone, suffix)}
		rawFilter, err = json.Marshal(filter)
		if err != nil {
			panic("this should not have failed")
		}
		linodes, err = l.client.ListInstances(ctx, linodego.NewListOptions(1, string(rawFilter)))
		if err != nil {
			return nil, err
		}
	}
	if len(linodes) > 0 {
		ipHolder = &linodes[0]
	}
	return ipHolder, nil
}

// generateClusterScopedIPHolderLinodeName attempts to generate a unique name for the IP Holder
// instance used alongside Cilium LoadBalancers and Shared IPs for Kubernetes Services.
// If the `--ip-holder-suffix` arg is passed when running Linode CCM, `suffix` is set to that value.
func generateClusterScopedIPHolderLinodeName(zone, suffix string) (label string) {
	// since Linode CCM consumers are varied, we require a method of providing a
	// suffix that does not rely on the use of a specific product (ex. LKE) to
	// have a specific piece of metadata (ex. annotation(s), label(s) ) present to key off of.

	if suffix == "" {
		// this avoids a trailing hyphen if suffix is empty (ex. linode-ccm-ip-holder-us-ord-)
		label = fmt.Sprintf("%s-%s", ipHolderLabelPrefix, zone)
	} else {
		label = fmt.Sprintf("%s-%s-%s", ipHolderLabelPrefix, zone, suffix)
	}
	klog.V(5).Infof("generated IP Holder Linode label: %s", label)
	return label
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
	if raw, ok := os.LookupEnv("BGP_CUSTOM_ID_MAP"); ok && raw != "" {
		klog.Info("BGP_CUSTOM_ID_MAP env variable specified, using it instead of the default region map")
		if err := json.Unmarshal([]byte(raw), &regionIDMap); err != nil {
			return err
		}
	}
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
	// If no BGPNodeSelector is specified, select all worker nodes.
	if Options.BGPNodeSelector == "" {
		nodeSelector = slimv1.LabelSelector{
			MatchExpressions: []slimv1.LabelSelectorRequirement{
				{
					Key:      commonControlPlaneLabel,
					Operator: slimv1.LabelSelectorOpDoesNotExist,
				},
			},
		}
	} else {
		kv := strings.Split(Options.BGPNodeSelector, "=")
		if len(kv) != BGPNodeSelectorFlagInputLen {
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
	bgpPeerPrefix := defaultBGPPeerPrefix
	if raw, ok := os.LookupEnv("BGP_PEER_PREFIX"); ok && raw != "" {
		klog.Info("BGP_PEER_PREFIX env variable specified, using it instead of the default bgpPeer prefix")
		bgpPeerPrefix = raw
	}
	// As in https://github.com/linode/lelastic, there are 4 peers per DC
	for i := 1; i <= 4; i++ {
		neighbor := v2alpha1.CiliumBGPNeighbor{
			PeerAddress:             fmt.Sprintf("%s:%d:34::%d/64", bgpPeerPrefix, regionID, i),
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

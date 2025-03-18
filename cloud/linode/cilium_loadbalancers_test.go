package linode

import (
	"encoding/json"
	"fmt"
	"net"
	"testing"

	k8sClient "github.com/cilium/cilium/pkg/k8s/client"
	fakev2alpha1 "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/typed/cilium.io/v2alpha1/fake"
	"github.com/golang/mock/gomock"
	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client/mocks"
)

const (
	clusterName  string = "linodelb"
	nodeSelector string = "cilium-bgp-peering=true"
	dummyIP      string = "45.76.101.26"
)

var (
	zone  = "us-ord"
	nodes = []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-1",
				Labels: map[string]string{"cilium-bgp-peering": "true"},
			},
			Spec: v1.NodeSpec{
				ProviderID: fmt.Sprintf("%s%d", providerIDPrefix, 11111),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-2",
				Labels: map[string]string{"cilium-bgp-peering": "true"},
			},
			Spec: v1.NodeSpec{
				ProviderID: fmt.Sprintf("%s%d", providerIDPrefix, 22222),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-3",
			},
			Spec: v1.NodeSpec{
				ProviderID: fmt.Sprintf("%s%d", providerIDPrefix, 33333),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-control",
				Labels: map[string]string{
					commonControlPlaneLabel: "",
				},
			},
			Spec: v1.NodeSpec{
				ProviderID: fmt.Sprintf("%s%d", providerIDPrefix, 44444),
			},
		},
	}
	additionalNodes = []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-5",
				Labels: map[string]string{"cilium-bgp-peering": "true"},
			},
			Spec: v1.NodeSpec{
				ProviderID: fmt.Sprintf("%s%d", providerIDPrefix, 55555),
			},
		},
	}
	publicIPv4          = net.ParseIP("45.76.101.25")
	oldIpHolderInstance = linodego.Instance{
		ID:     12345,
		Label:  fmt.Sprintf("%s-%s", ipHolderLabelPrefix, zone),
		Type:   "g6-standard-1",
		Region: "us-west",
		IPv4:   []*net.IP{&publicIPv4},
	}
	newIpHolderInstance = linodego.Instance{}
)

func TestCiliumCCMLoadBalancers(t *testing.T) {
	testCases := []struct {
		name string
		f    func(*testing.T, *mocks.MockClient)
	}{
		{
			name: "Create Cilium Load Balancer Without BGP Node Labels specified",
			f:    testNoBGPNodeLabel,
		},
		{
			name: "Create Cilium Load Balancer with unsupported region",
			f:    testUnsupportedRegion,
		},
		{
			name: "Create Cilium Load Balancer With explicit loadBalancerClass and existing IP holder nanode with old IP Holder naming convention",
			f:    testCreateWithExistingIPHolderWithOldIpHolderNamingConvention,
		},
		{
			name: "Create Cilium Load Balancer With explicit loadBalancerClass and existing IP holder nanode with new IP Holder naming convention",
			f:    testCreateWithExistingIPHolderWithNewIpHolderNamingConvention,
		},
		{
			name: "Create Cilium Load Balancer With explicit loadBalancerClass and existing IP holder nanode with new IP Holder naming convention and 63 char long suffix",
			f:    testCreateWithExistingIPHolderWithNewIpHolderNamingConventionUsingLongSuffix,
		},
		{
			name: "Create Cilium Load Balancer With no existing IP holder nanode and short suffix",
			f:    testCreateWithNoExistingIPHolderUsingShortSuffix,
		},
		{
			name: "Create Cilium Load Balancer With no existing IP holder nanode and no suffix",
			f:    testCreateWithNoExistingIPHolderUsingNoSuffix,
		},
		{
			name: "Create Cilium Load Balancer With no existing IP holder nanode and 63 char long suffix",
			f:    testCreateWithNoExistingIPHolderUsingLongSuffix,
		},
		{
			name: "Delete Cilium Load Balancer With Old IP Holder Naming Convention",
			f:    testEnsureCiliumLoadBalancerDeletedWithOldIpHolderNamingConvention,
		},
		{
			name: "Delete Cilium Load Balancer With New IP Holder Naming Convention",
			f:    testEnsureCiliumLoadBalancerDeletedWithNewIpHolderNamingConvention,
		},
		{
			name: "Add node to existing Cilium Load Balancer With Old IP Holder Naming Convention",
			f:    testCiliumUpdateLoadBalancerAddNodeWithOldIpHolderNamingConvention,
		},
		{
			name: "Add node to existing Cilium Load Balancer With New IP Holder Naming Convention",
			f:    testCiliumUpdateLoadBalancerAddNodeWithNewIpHolderNamingConvention,
		},
	}
	//nolint: paralleltest // two tests use t.Setenv, which fails after t.Parallel() call
	for _, tc := range testCases {
		ctrl := gomock.NewController(t)
		mc := mocks.NewMockClient(ctrl)
		t.Run(tc.name, func(t *testing.T) {
			defer ctrl.Finish()
			tc.f(t, mc)
		})
	}
}

func createTestService() *v1.Service {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      randString(),
			Namespace: "test-ns",
			UID:       "foobar123",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
					Protocol: "TCP",
					Port:     int32(80),
					NodePort: int32(30000),
				},
				{
					Name:     randString(),
					Protocol: "TCP",
					Port:     int32(8080),
					NodePort: int32(30001),
				},
			},
		},
	}

	return svc
}

func addService(t *testing.T, kubeClient kubernetes.Interface, svc *v1.Service) {
	t.Helper()

	_, err := kubeClient.CoreV1().Services(svc.Namespace).Create(t.Context(), svc, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to add Service: %v", err)
	}
}

func addNodes(t *testing.T, kubeClient kubernetes.Interface, nodes []*v1.Node) {
	t.Helper()

	for _, node := range nodes {
		_, err := kubeClient.CoreV1().Nodes().Create(t.Context(), node, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("failed to add Node: %v", err)
		}
	}
}

func createNewIpHolderInstance() linodego.Instance {
	return linodego.Instance{
		ID:     123456,
		Label:  generateClusterScopedIPHolderLinodeName(zone, Options.IpHolderSuffix),
		Type:   "g6-standard-1",
		Region: "us-west",
		IPv4:   []*net.IP{&publicIPv4},
	}
}

func testNoBGPNodeLabel(t *testing.T, mc *mocks.MockClient) {
	t.Helper()

	Options.BGPNodeSelector = ""
	Options.IpHolderSuffix = clusterName
	t.Setenv("BGP_PEER_PREFIX", "2600:3cef")
	svc := createTestService()
	newIpHolderInstance = createNewIpHolderInstance()

	kubeClient, _ := k8sClient.NewFakeClientset()
	ciliumClient := &fakev2alpha1.FakeCiliumV2alpha1{Fake: &kubeClient.CiliumFakeClientset.Fake}
	addService(t, kubeClient, svc)
	addNodes(t, kubeClient, nodes)
	lb := &loadbalancers{mc, zone, kubeClient, ciliumClient, ciliumLBType}

	filter := map[string]string{"label": fmt.Sprintf("%s-%s", ipHolderLabelPrefix, zone)}
	rawFilter, err := json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}

	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{}, nil)
	filter = map[string]string{"label": generateClusterScopedIPHolderLinodeName(zone, Options.IpHolderSuffix)}
	rawFilter, err = json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}

	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{}, nil)
	dummySharedIP := dummyIP
	mc.EXPECT().CreateInstance(gomock.Any(), gomock.Any()).Times(1).Return(&newIpHolderInstance, nil)
	mc.EXPECT().GetInstanceIPAddresses(gomock.Any(), newIpHolderInstance.ID).Times(1).Return(&linodego.InstanceIPAddressResponse{
		IPv4: &linodego.InstanceIPv4Response{
			Public: []*linodego.InstanceIP{{Address: publicIPv4.String()}, {Address: dummySharedIP}},
		},
	}, nil)
	mc.EXPECT().AddInstanceIPAddress(gomock.Any(), newIpHolderInstance.ID, true).Times(1).Return(&linodego.InstanceIP{Address: dummySharedIP}, nil)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 11111,
	}).Times(1)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 22222,
	}).Times(1)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 33333,
	}).Times(1)

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), clusterName, svc, nodes)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
	if lbStatus == nil {
		t.Fatal("expected non-nil lbStatus")
	}
}

func testUnsupportedRegion(t *testing.T, mc *mocks.MockClient) {
	t.Helper()

	Options.BGPNodeSelector = nodeSelector
	svc := createTestService()

	kubeClient, _ := k8sClient.NewFakeClientset()
	ciliumClient := &fakev2alpha1.FakeCiliumV2alpha1{Fake: &kubeClient.CiliumFakeClientset.Fake}
	addService(t, kubeClient, svc)
	lb := &loadbalancers{mc, "us-foobar", kubeClient, ciliumClient, ciliumLBType}

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), clusterName, svc, nodes)
	if err == nil {
		t.Fatal("expected not nil error")
	}
	if lbStatus != nil {
		t.Fatalf("expected a nil lbStatus, got %v", lbStatus)
	}

	// Use BGP custom id map
	t.Setenv("BGP_CUSTOM_ID_MAP", "{'us-foobar': 2}")
	lb = &loadbalancers{mc, zone, kubeClient, ciliumClient, ciliumLBType}
	lbStatus, err = lb.EnsureLoadBalancer(t.Context(), clusterName, svc, nodes)
	if err == nil {
		t.Fatal("expected not nil error")
	}
	if lbStatus != nil {
		t.Fatalf("expected a nil lbStatus, got %v", lbStatus)
	}
}

func testCreateWithExistingIPHolderWithOldIpHolderNamingConvention(t *testing.T, mc *mocks.MockClient) {
	t.Helper()

	Options.BGPNodeSelector = nodeSelector
	svc := createTestService()
	newIpHolderInstance = createNewIpHolderInstance()

	kubeClient, _ := k8sClient.NewFakeClientset()
	ciliumClient := &fakev2alpha1.FakeCiliumV2alpha1{Fake: &kubeClient.CiliumFakeClientset.Fake}
	addService(t, kubeClient, svc)
	addNodes(t, kubeClient, nodes)
	lb := &loadbalancers{mc, zone, kubeClient, ciliumClient, ciliumLBType}

	filter := map[string]string{"label": fmt.Sprintf("%s-%s", ipHolderLabelPrefix, zone)}
	rawFilter, err := json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{oldIpHolderInstance}, nil)
	dummySharedIP := dummyIP
	mc.EXPECT().AddInstanceIPAddress(gomock.Any(), oldIpHolderInstance.ID, true).Times(1).Return(&linodego.InstanceIP{Address: dummySharedIP}, nil)
	mc.EXPECT().GetInstanceIPAddresses(gomock.Any(), oldIpHolderInstance.ID).Times(1).Return(&linodego.InstanceIPAddressResponse{
		IPv4: &linodego.InstanceIPv4Response{
			Public: []*linodego.InstanceIP{{Address: publicIPv4.String()}, {Address: dummySharedIP}},
		},
	}, nil)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 11111,
	}).Times(1)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 22222,
	}).Times(1)

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), clusterName, svc, nodes)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
	if lbStatus == nil {
		t.Fatal("expected non-nil lbStatus")
	}
}

func testCreateWithExistingIPHolderWithNewIpHolderNamingConvention(t *testing.T, mc *mocks.MockClient) {
	t.Helper()

	Options.BGPNodeSelector = nodeSelector
	Options.IpHolderSuffix = clusterName
	svc := createTestService()
	newIpHolderInstance = createNewIpHolderInstance()

	kubeClient, _ := k8sClient.NewFakeClientset()
	ciliumClient := &fakev2alpha1.FakeCiliumV2alpha1{Fake: &kubeClient.CiliumFakeClientset.Fake}
	addService(t, kubeClient, svc)
	addNodes(t, kubeClient, nodes)
	lb := &loadbalancers{mc, zone, kubeClient, ciliumClient, ciliumLBType}

	filter := map[string]string{"label": fmt.Sprintf("%s-%s", ipHolderLabelPrefix, zone)}
	rawFilter, err := json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{oldIpHolderInstance}, nil)
	dummySharedIP := dummyIP
	mc.EXPECT().AddInstanceIPAddress(gomock.Any(), oldIpHolderInstance.ID, true).Times(1).Return(&linodego.InstanceIP{Address: dummySharedIP}, nil)
	mc.EXPECT().GetInstanceIPAddresses(gomock.Any(), oldIpHolderInstance.ID).Times(1).Return(&linodego.InstanceIPAddressResponse{
		IPv4: &linodego.InstanceIPv4Response{
			Public: []*linodego.InstanceIP{{Address: publicIPv4.String()}, {Address: dummySharedIP}},
		},
	}, nil)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 11111,
	}).Times(1)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 22222,
	}).Times(1)

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), clusterName, svc, nodes)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
	if lbStatus == nil {
		t.Fatal("expected non-nil lbStatus")
	}
}

func testCreateWithExistingIPHolderWithNewIpHolderNamingConventionUsingLongSuffix(t *testing.T, mc *mocks.MockClient) {
	t.Helper()

	Options.BGPNodeSelector = nodeSelector
	Options.IpHolderSuffix = "OaTJrRuufacHVougjwkpBpmstiqvswvBNEMWXsRYfMBTCkKIUTXpbGIcIbDWSQp"
	svc := createTestService()
	newIpHolderInstance = createNewIpHolderInstance()

	kubeClient, _ := k8sClient.NewFakeClientset()
	ciliumClient := &fakev2alpha1.FakeCiliumV2alpha1{Fake: &kubeClient.CiliumFakeClientset.Fake}
	addService(t, kubeClient, svc)
	addNodes(t, kubeClient, nodes)
	lb := &loadbalancers{mc, zone, kubeClient, ciliumClient, ciliumLBType}

	filter := map[string]string{"label": fmt.Sprintf("%s-%s", ipHolderLabelPrefix, zone)}
	rawFilter, err := json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{oldIpHolderInstance}, nil)
	dummySharedIP := dummyIP
	mc.EXPECT().AddInstanceIPAddress(gomock.Any(), oldIpHolderInstance.ID, true).Times(1).Return(&linodego.InstanceIP{Address: dummySharedIP}, nil)
	mc.EXPECT().GetInstanceIPAddresses(gomock.Any(), oldIpHolderInstance.ID).Times(1).Return(&linodego.InstanceIPAddressResponse{
		IPv4: &linodego.InstanceIPv4Response{
			Public: []*linodego.InstanceIP{{Address: publicIPv4.String()}, {Address: dummySharedIP}},
		},
	}, nil)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 11111,
	}).Times(1)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 22222,
	}).Times(1)

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), clusterName, svc, nodes)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
	if lbStatus == nil {
		t.Fatal("expected non-nil lbStatus")
	}
}

func testCreateWithNoExistingIPHolderUsingNoSuffix(t *testing.T, mc *mocks.MockClient) {
	t.Helper()

	Options.BGPNodeSelector = nodeSelector
	Options.IpHolderSuffix = ""
	svc := createTestService()
	newIpHolderInstance = createNewIpHolderInstance()

	kubeClient, _ := k8sClient.NewFakeClientset()
	ciliumClient := &fakev2alpha1.FakeCiliumV2alpha1{Fake: &kubeClient.CiliumFakeClientset.Fake}
	addService(t, kubeClient, svc)
	addNodes(t, kubeClient, nodes)
	lb := &loadbalancers{mc, zone, kubeClient, ciliumClient, ciliumLBType}

	filter := map[string]string{"label": fmt.Sprintf("%s-%s", ipHolderLabelPrefix, zone)}
	rawFilter, err := json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{}, nil)
	filter = map[string]string{"label": generateClusterScopedIPHolderLinodeName(zone, Options.IpHolderSuffix)}
	rawFilter, err = json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{}, nil)
	dummySharedIP := dummyIP
	mc.EXPECT().CreateInstance(gomock.Any(), gomock.Any()).Times(1).Return(&newIpHolderInstance, nil)
	mc.EXPECT().GetInstanceIPAddresses(gomock.Any(), newIpHolderInstance.ID).Times(1).Return(&linodego.InstanceIPAddressResponse{
		IPv4: &linodego.InstanceIPv4Response{
			Public: []*linodego.InstanceIP{{Address: publicIPv4.String()}, {Address: dummySharedIP}},
		},
	}, nil)
	mc.EXPECT().AddInstanceIPAddress(gomock.Any(), newIpHolderInstance.ID, true).Times(1).Return(&linodego.InstanceIP{Address: dummySharedIP}, nil)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 11111,
	}).Times(1)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 22222,
	}).Times(1)

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), clusterName, svc, nodes)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
	if lbStatus == nil {
		t.Fatal("expected non-nil lbStatus")
	}
}

func testCreateWithNoExistingIPHolderUsingShortSuffix(t *testing.T, mc *mocks.MockClient) {
	t.Helper()

	Options.BGPNodeSelector = nodeSelector
	Options.IpHolderSuffix = clusterName
	svc := createTestService()
	newIpHolderInstance = createNewIpHolderInstance()

	kubeClient, _ := k8sClient.NewFakeClientset()
	ciliumClient := &fakev2alpha1.FakeCiliumV2alpha1{Fake: &kubeClient.CiliumFakeClientset.Fake}
	addService(t, kubeClient, svc)
	addNodes(t, kubeClient, nodes)
	lb := &loadbalancers{mc, zone, kubeClient, ciliumClient, ciliumLBType}

	filter := map[string]string{"label": fmt.Sprintf("%s-%s", ipHolderLabelPrefix, zone)}
	rawFilter, err := json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{}, nil)
	filter = map[string]string{"label": generateClusterScopedIPHolderLinodeName(zone, Options.IpHolderSuffix)}
	rawFilter, err = json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{}, nil)
	dummySharedIP := dummyIP
	mc.EXPECT().CreateInstance(gomock.Any(), gomock.Any()).Times(1).Return(&newIpHolderInstance, nil)
	mc.EXPECT().GetInstanceIPAddresses(gomock.Any(), newIpHolderInstance.ID).Times(1).Return(&linodego.InstanceIPAddressResponse{
		IPv4: &linodego.InstanceIPv4Response{
			Public: []*linodego.InstanceIP{{Address: publicIPv4.String()}, {Address: dummySharedIP}},
		},
	}, nil)
	mc.EXPECT().AddInstanceIPAddress(gomock.Any(), newIpHolderInstance.ID, true).Times(1).Return(&linodego.InstanceIP{Address: dummySharedIP}, nil)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 11111,
	}).Times(1)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 22222,
	}).Times(1)

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), clusterName, svc, nodes)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
	if lbStatus == nil {
		t.Fatal("expected non-nil lbStatus")
	}
}

func testCreateWithNoExistingIPHolderUsingLongSuffix(t *testing.T, mc *mocks.MockClient) {
	t.Helper()

	Options.BGPNodeSelector = nodeSelector
	Options.IpHolderSuffix = "OaTJrRuufacHVougjwkpBpmstiqvswvBNEMWXsRYfMBTCkKIUTXpbGIcIbDWSQp"
	svc := createTestService()
	newIpHolderInstance = createNewIpHolderInstance()

	kubeClient, _ := k8sClient.NewFakeClientset()
	ciliumClient := &fakev2alpha1.FakeCiliumV2alpha1{Fake: &kubeClient.CiliumFakeClientset.Fake}
	addService(t, kubeClient, svc)
	addNodes(t, kubeClient, nodes)
	lb := &loadbalancers{mc, zone, kubeClient, ciliumClient, ciliumLBType}

	filter := map[string]string{"label": fmt.Sprintf("%s-%s", ipHolderLabelPrefix, zone)}
	rawFilter, err := json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{}, nil)
	filter = map[string]string{"label": generateClusterScopedIPHolderLinodeName(zone, Options.IpHolderSuffix)}
	rawFilter, err = json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{}, nil)
	dummySharedIP := dummyIP
	mc.EXPECT().CreateInstance(gomock.Any(), gomock.Any()).Times(1).Return(&newIpHolderInstance, nil)
	mc.EXPECT().GetInstanceIPAddresses(gomock.Any(), newIpHolderInstance.ID).Times(1).Return(&linodego.InstanceIPAddressResponse{
		IPv4: &linodego.InstanceIPv4Response{
			Public: []*linodego.InstanceIP{{Address: publicIPv4.String()}, {Address: dummySharedIP}},
		},
	}, nil)
	mc.EXPECT().AddInstanceIPAddress(gomock.Any(), newIpHolderInstance.ID, true).Times(1).Return(&linodego.InstanceIP{Address: dummySharedIP}, nil)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 11111,
	}).Times(1)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 22222,
	}).Times(1)

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), clusterName, svc, nodes)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
	if lbStatus == nil {
		t.Fatal("expected non-nil lbStatus")
	}
}

func testEnsureCiliumLoadBalancerDeletedWithOldIpHolderNamingConvention(t *testing.T, mc *mocks.MockClient) {
	t.Helper()

	Options.BGPNodeSelector = nodeSelector
	svc := createTestService()

	kubeClient, _ := k8sClient.NewFakeClientset()
	ciliumClient := &fakev2alpha1.FakeCiliumV2alpha1{Fake: &kubeClient.CiliumFakeClientset.Fake}
	addService(t, kubeClient, svc)
	addNodes(t, kubeClient, nodes)
	lb := &loadbalancers{mc, zone, kubeClient, ciliumClient, ciliumLBType}

	dummySharedIP := dummyIP
	svc.Status.LoadBalancer = v1.LoadBalancerStatus{Ingress: []v1.LoadBalancerIngress{{IP: dummySharedIP}}}

	filter := map[string]string{"label": fmt.Sprintf("%s-%s", ipHolderLabelPrefix, zone)}
	rawFilter, err := json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{oldIpHolderInstance}, nil)
	mc.EXPECT().DeleteInstanceIPAddress(gomock.Any(), 11111, dummySharedIP).Times(1).Return(nil)
	mc.EXPECT().DeleteInstanceIPAddress(gomock.Any(), 22222, dummySharedIP).Times(1).Return(nil)
	mc.EXPECT().DeleteInstanceIPAddress(gomock.Any(), oldIpHolderInstance.ID, dummySharedIP).Times(1).Return(nil)

	err = lb.EnsureLoadBalancerDeleted(t.Context(), clusterName, svc)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
}

func testEnsureCiliumLoadBalancerDeletedWithNewIpHolderNamingConvention(t *testing.T, mc *mocks.MockClient) {
	t.Helper()

	Options.BGPNodeSelector = nodeSelector
	Options.IpHolderSuffix = clusterName
	svc := createTestService()
	newIpHolderInstance = createNewIpHolderInstance()

	kubeClient, _ := k8sClient.NewFakeClientset()
	ciliumClient := &fakev2alpha1.FakeCiliumV2alpha1{Fake: &kubeClient.CiliumFakeClientset.Fake}
	addService(t, kubeClient, svc)
	addNodes(t, kubeClient, nodes)
	lb := &loadbalancers{mc, zone, kubeClient, ciliumClient, ciliumLBType}

	dummySharedIP := dummyIP
	svc.Status.LoadBalancer = v1.LoadBalancerStatus{Ingress: []v1.LoadBalancerIngress{{IP: dummySharedIP}}}

	filter := map[string]string{"label": fmt.Sprintf("%s-%s", ipHolderLabelPrefix, zone)}
	rawFilter, err := json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{}, nil)
	filter = map[string]string{"label": generateClusterScopedIPHolderLinodeName(zone, Options.IpHolderSuffix)}
	rawFilter, err = json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{newIpHolderInstance}, nil)
	mc.EXPECT().DeleteInstanceIPAddress(gomock.Any(), 11111, dummySharedIP).Times(1).Return(nil)
	mc.EXPECT().DeleteInstanceIPAddress(gomock.Any(), 22222, dummySharedIP).Times(1).Return(nil)
	mc.EXPECT().DeleteInstanceIPAddress(gomock.Any(), newIpHolderInstance.ID, dummySharedIP).Times(1).Return(nil)

	err = lb.EnsureLoadBalancerDeleted(t.Context(), clusterName, svc)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
}

func testCiliumUpdateLoadBalancerAddNodeWithOldIpHolderNamingConvention(t *testing.T, mc *mocks.MockClient) {
	t.Helper()

	Options.BGPNodeSelector = nodeSelector
	svc := createTestService()

	kubeClient, _ := k8sClient.NewFakeClientset()
	ciliumClient := &fakev2alpha1.FakeCiliumV2alpha1{Fake: &kubeClient.CiliumFakeClientset.Fake}
	addService(t, kubeClient, svc)
	addNodes(t, kubeClient, nodes)
	lb := &loadbalancers{mc, zone, kubeClient, ciliumClient, ciliumLBType}

	filter := map[string]string{"label": fmt.Sprintf("%s-%s", ipHolderLabelPrefix, zone)}
	rawFilter, err := json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{oldIpHolderInstance}, nil)
	dummySharedIP := dummyIP
	mc.EXPECT().AddInstanceIPAddress(gomock.Any(), oldIpHolderInstance.ID, true).Times(1).Return(&linodego.InstanceIP{Address: dummySharedIP}, nil)
	mc.EXPECT().GetInstanceIPAddresses(gomock.Any(), oldIpHolderInstance.ID).Times(1).Return(&linodego.InstanceIPAddressResponse{
		IPv4: &linodego.InstanceIPv4Response{
			Public: []*linodego.InstanceIP{{Address: publicIPv4.String()}, {Address: dummySharedIP}},
		},
	}, nil)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 11111,
	}).Times(1)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 22222,
	}).Times(1)

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), clusterName, svc, nodes)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
	if lbStatus == nil {
		t.Fatal("expected non-nil lbStatus")
	}

	// Now add another node to the cluster and assert that it gets the shared IP
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{oldIpHolderInstance}, nil)
	mc.EXPECT().GetInstanceIPAddresses(gomock.Any(), oldIpHolderInstance.ID).Times(1).Return(&linodego.InstanceIPAddressResponse{
		IPv4: &linodego.InstanceIPv4Response{
			Public: []*linodego.InstanceIP{{Address: publicIPv4.String()}, {Address: dummySharedIP}},
		},
	}, nil)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 55555,
	}).Times(1)
	addNodes(t, kubeClient, additionalNodes)

	err = lb.UpdateLoadBalancer(t.Context(), clusterName, svc, additionalNodes)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
}

func testCiliumUpdateLoadBalancerAddNodeWithNewIpHolderNamingConvention(t *testing.T, mc *mocks.MockClient) {
	t.Helper()

	Options.BGPNodeSelector = nodeSelector
	Options.IpHolderSuffix = clusterName
	svc := createTestService()
	newIpHolderInstance = createNewIpHolderInstance()

	kubeClient, _ := k8sClient.NewFakeClientset()
	ciliumClient := &fakev2alpha1.FakeCiliumV2alpha1{Fake: &kubeClient.CiliumFakeClientset.Fake}
	addService(t, kubeClient, svc)
	addNodes(t, kubeClient, nodes)
	lb := &loadbalancers{mc, zone, kubeClient, ciliumClient, ciliumLBType}

	filter := map[string]string{"label": fmt.Sprintf("%s-%s", ipHolderLabelPrefix, zone)}
	rawFilter, err := json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{}, nil)
	filter = map[string]string{"label": generateClusterScopedIPHolderLinodeName(zone, Options.IpHolderSuffix)}
	rawFilter, err = json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{newIpHolderInstance}, nil)
	dummySharedIP := dummyIP
	mc.EXPECT().AddInstanceIPAddress(gomock.Any(), newIpHolderInstance.ID, true).Times(1).Return(&linodego.InstanceIP{Address: dummySharedIP}, nil)
	mc.EXPECT().GetInstanceIPAddresses(gomock.Any(), newIpHolderInstance.ID).Times(1).Return(&linodego.InstanceIPAddressResponse{
		IPv4: &linodego.InstanceIPv4Response{
			Public: []*linodego.InstanceIP{{Address: publicIPv4.String()}, {Address: dummySharedIP}},
		},
	}, nil)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 11111,
	}).Times(1)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 22222,
	}).Times(1)

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), clusterName, svc, nodes)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
	if lbStatus == nil {
		t.Fatal("expected non-nil lbStatus")
	}

	// Now add another node to the cluster and assert that it gets the shared IP
	filter = map[string]string{"label": fmt.Sprintf("%s-%s", ipHolderLabelPrefix, zone)}
	rawFilter, err = json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{}, nil)
	filter = map[string]string{"label": generateClusterScopedIPHolderLinodeName(zone, Options.IpHolderSuffix)}
	rawFilter, err = json.Marshal(filter)
	if err != nil {
		t.Errorf("json marshal error: %v", err)
	}
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{newIpHolderInstance}, nil)

	mc.EXPECT().GetInstanceIPAddresses(gomock.Any(), newIpHolderInstance.ID).Times(1).Return(&linodego.InstanceIPAddressResponse{
		IPv4: &linodego.InstanceIPv4Response{
			Public: []*linodego.InstanceIP{{Address: publicIPv4.String()}, {Address: dummySharedIP}},
		},
	}, nil)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 55555,
	}).Times(1)
	addNodes(t, kubeClient, additionalNodes)

	err = lb.UpdateLoadBalancer(t.Context(), clusterName, svc, additionalNodes)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
}

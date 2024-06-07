package linode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"testing"

	k8sClient "github.com/cilium/cilium/pkg/k8s/client"
	fakev2alpha1 "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/typed/cilium.io/v2alpha1/fake"
	"github.com/golang/mock/gomock"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client/mocks"
	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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
	}
	publicIPv4       = net.ParseIP("45.76.101.25")
	ipHolderInstance = linodego.Instance{
		ID:     12345,
		Label:  fmt.Sprintf("%s-%s", ipHolderLabelPrefix, zone),
		Type:   "g6-standard-1",
		Region: "us-west",
		IPv4:   []*net.IP{&publicIPv4},
	}
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
			name: "Create Cilium Load Balancer With explicit loadBalancerClass and existing IP holder nanode",
			f:    testCreateWithExistingIPHolder,
		},
		{
			name: "Create Cilium Load Balancer With no existing IP holder nanode",
			f:    testCreateWithNoExistingIPHolder,
		},
		{
			name: "Delete Cilium Load Balancer",
			f:    testEnsureCiliumLoadBalancerDeleted,
		},
	}
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
	_, err := kubeClient.CoreV1().Services(svc.Namespace).Create(context.TODO(), svc, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to add Service: %v", err)
	}
}

func addNodes(t *testing.T, kubeClient kubernetes.Interface, nodes []*v1.Node) {
	for _, node := range nodes {
		_, err := kubeClient.CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("failed to add Node: %v", err)
		}
	}
}

func testNoBGPNodeLabel(t *testing.T, mc *mocks.MockClient) {
	svc := createTestService()

	kubeClient, _ := k8sClient.NewFakeClientset()
	ciliumClient := &fakev2alpha1.FakeCiliumV2alpha1{Fake: &kubeClient.CiliumFakeClientset.Fake}
	addService(t, kubeClient, svc)
	lb := &loadbalancers{mc, zone, kubeClient, ciliumClient, ciliumLBType}

	lbStatus, err := lb.EnsureLoadBalancer(context.TODO(), "linodelb", svc, nodes)
	if !errors.Is(err, errNoBGPSelector) {
		t.Fatalf("expected %v, got %v... %s", errNoBGPSelector, err, Options.BGPNodeSelector)
	}
	if lbStatus != nil {
		t.Fatalf("expected a nil lbStatus, got %v", lbStatus)
	}
}

func testUnsupportedRegion(t *testing.T, mc *mocks.MockClient) {
	Options.BGPNodeSelector = "cilium-bgp-peering=true"
	svc := createTestService()

	kubeClient, _ := k8sClient.NewFakeClientset()
	ciliumClient := &fakev2alpha1.FakeCiliumV2alpha1{Fake: &kubeClient.CiliumFakeClientset.Fake}
	addService(t, kubeClient, svc)
	lb := &loadbalancers{mc, "us-foobar", kubeClient, ciliumClient, ciliumLBType}

	lbStatus, err := lb.EnsureLoadBalancer(context.TODO(), "linodelb", svc, nodes)
	if err == nil {
		t.Fatal("expected nil error")
	}
	if lbStatus != nil {
		t.Fatalf("expected a nil lbStatus, got %v", lbStatus)
	}
}

func testCreateWithExistingIPHolder(t *testing.T, mc *mocks.MockClient) {
	Options.BGPNodeSelector = "cilium-bgp-peering=true"
	svc := createTestService()

	kubeClient, _ := k8sClient.NewFakeClientset()
	ciliumClient := &fakev2alpha1.FakeCiliumV2alpha1{Fake: &kubeClient.CiliumFakeClientset.Fake}
	addService(t, kubeClient, svc)
	addNodes(t, kubeClient, nodes)
	lb := &loadbalancers{mc, zone, kubeClient, ciliumClient, ciliumLBType}

	filter := map[string]string{"label": fmt.Sprintf("%s-%s", ipHolderLabelPrefix, zone)}
	rawFilter, _ := json.Marshal(filter)
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{ipHolderInstance}, nil)
	dummySharedIP := "45.76.101.26"
	mc.EXPECT().AddInstanceIPAddress(gomock.Any(), ipHolderInstance.ID, true).Times(1).Return(&linodego.InstanceIP{Address: dummySharedIP}, nil)
	mc.EXPECT().GetInstanceIPAddresses(gomock.Any(), ipHolderInstance.ID).Times(1).Return(&linodego.InstanceIPAddressResponse{
		IPv4: &linodego.InstanceIPv4Response{
			Shared: []*linodego.InstanceIP{{Address: dummySharedIP}},
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

	lbStatus, err := lb.EnsureLoadBalancer(context.TODO(), "linodelb", svc, nodes)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
	if lbStatus == nil {
		t.Fatal("expected non-nil lbStatus")
	}
}

func testCreateWithNoExistingIPHolder(t *testing.T, mc *mocks.MockClient) {
	Options.BGPNodeSelector = "cilium-bgp-peering=true"
	svc := createTestService()

	kubeClient, _ := k8sClient.NewFakeClientset()
	ciliumClient := &fakev2alpha1.FakeCiliumV2alpha1{Fake: &kubeClient.CiliumFakeClientset.Fake}
	addService(t, kubeClient, svc)
	addNodes(t, kubeClient, nodes)
	lb := &loadbalancers{mc, zone, kubeClient, ciliumClient, ciliumLBType}

	filter := map[string]string{"label": fmt.Sprintf("%s-%s", ipHolderLabelPrefix, zone)}
	rawFilter, _ := json.Marshal(filter)
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{}, nil)
	dummySharedIP := "45.76.101.26"
	mc.EXPECT().CreateInstance(gomock.Any(), gomock.Any()).Times(1).Return(&ipHolderInstance, nil)
	mc.EXPECT().GetInstanceIPAddresses(gomock.Any(), ipHolderInstance.ID).Times(1).Return(&linodego.InstanceIPAddressResponse{
		IPv4: &linodego.InstanceIPv4Response{
			Shared: []*linodego.InstanceIP{{Address: dummySharedIP}},
		},
	}, nil)
	mc.EXPECT().AddInstanceIPAddress(gomock.Any(), ipHolderInstance.ID, true).Times(1).Return(&linodego.InstanceIP{Address: dummySharedIP}, nil)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 11111,
	}).Times(1)
	mc.EXPECT().ShareIPAddresses(gomock.Any(), linodego.IPAddressesShareOptions{
		IPs:      []string{dummySharedIP},
		LinodeID: 22222,
	}).Times(1)

	lbStatus, err := lb.EnsureLoadBalancer(context.TODO(), "linodelb", svc, nodes)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
	if lbStatus == nil {
		t.Fatal("expected non-nil lbStatus")
	}
}

func testEnsureCiliumLoadBalancerDeleted(t *testing.T, mc *mocks.MockClient) {
	Options.BGPNodeSelector = "cilium-bgp-peering=true"
	svc := createTestService()

	kubeClient, _ := k8sClient.NewFakeClientset()
	ciliumClient := &fakev2alpha1.FakeCiliumV2alpha1{Fake: &kubeClient.CiliumFakeClientset.Fake}
	addService(t, kubeClient, svc)
	addNodes(t, kubeClient, nodes)
	lb := &loadbalancers{mc, zone, kubeClient, ciliumClient, ciliumLBType}

	dummySharedIP := "45.76.101.26"
	svc.Status.LoadBalancer = v1.LoadBalancerStatus{Ingress: []v1.LoadBalancerIngress{{IP: dummySharedIP}}}

	filter := map[string]string{"label": fmt.Sprintf("%s-%s", ipHolderLabelPrefix, zone)}
	rawFilter, _ := json.Marshal(filter)
	mc.EXPECT().ListInstances(gomock.Any(), linodego.NewListOptions(1, string(rawFilter))).Times(1).Return([]linodego.Instance{ipHolderInstance}, nil)
	mc.EXPECT().DeleteInstanceIPAddress(gomock.Any(), 11111, dummySharedIP).Times(1).Return(nil)
	mc.EXPECT().DeleteInstanceIPAddress(gomock.Any(), 22222, dummySharedIP).Times(1).Return(nil)
	mc.EXPECT().DeleteInstanceIPAddress(gomock.Any(), ipHolderInstance.ID, dummySharedIP).Times(1).Return(nil)

	err := lb.EnsureLoadBalancerDeleted(context.TODO(), "linodelb", svc)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
}

package linode

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"

	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

const testCert string = `-----BEGIN CERTIFICATE-----
MIIFITCCAwkCAWQwDQYJKoZIhvcNAQELBQAwUjELMAkGA1UEBhMCQVUxEzARBgNV
BAgMClNvbWUtU3RhdGUxITAfBgNVBAoMGEludGVybmV0IFdpZGdpdHMgUHR5IEx0
ZDELMAkGA1UEAwwCY2EwHhcNMTkwNDA5MDkzNjQyWhcNMjMwNDA4MDkzNjQyWjBb
MQswCQYDVQQGEwJBVTETMBEGA1UECAwKU29tZS1TdGF0ZTEhMB8GA1UECgwYSW50
ZXJuZXQgV2lkZ2l0cyBQdHkgTHRkMRQwEgYDVQQDDAtsaW5vZGUudGVzdDCCAiIw
DQYJKoZIhvcNAQEBBQADggIPADCCAgoCggIBANUC0KStr84PLnM1dTYuEtk4HOTc
ufb6pMHyttJv5oYxCAJaN5AI9QXPqJpUFI6GlS1oDpjRe9RQghXso/IihD9eoEP1
zkHcHJyb6TXThofatxX5jLUM9TgmTIrYH+1KyKraBO6iMz2UQkbJq04BZWI9wADq
ffn1Cw6RueDe4QdqXpv/M9d/PetsIQLjjNAFHo87gYIkw838DMyTNikIweg8tRSS
6hivBVLLF0WB7p4ZARic8t+VqEFz0xl9AANE3OYMcsZCYacHxMBnX/OpHgEMxVkZ
GZ/5ikb6HJNnK/OintBlTqmGJK77fwSYXeO/5Zn6HpakfsNf6ZWSXsWRaatRvwL7
RD45RqSUpx0GALhxXTlQWv4F0cEn5MJSZX9uTJbFTuTYqC5NrB/M33hcUWy5N/L8
fz8GOxLRmrAthZ//dW4GBASOHdwMJOPz0Hb7DwNP5tSi74o7k+vCNuAHW8c8KCno
EIOS5Z6VNc252KVWZ0Y7gz7/w1Jk+cepNmpTRWzQAWc1RRYgRvAfKwXCFZpE5y6T
iu9LYtH0eKp55MBdWJ44lBu2iXc/rzcWNo0jDeHkBevS0prBxIgH377WVq/GoPRW
g3uVC6nGczHEGq1j1u6q3JKU97JSVznXIJssZLCQ4NYxtuZtmqcfEUDictq1W2Lh
upOn8Y/XQtI8gdb1AgMBAAEwDQYJKoZIhvcNAQELBQADggIBAB1Se+wlSOsRlII3
zk5VYSwiuvWc3pBYHShbSjdOFo4StZ4MRFyKu+gBssNZ7ZyM5B1oDOjslwm31nWP
j5NnlCeSeTJ2LGIkn1AFsZ4LK/ffHnxRVSUZCTUdW9PLbwDf7oDUxdtfrLdsC39F
RBn22oXTto4SNAqNQJGSkPrVT5a23JSplsPWu8ZwruaslvCtC8MRwpUp+A8EKdau
8BeYgzJWY/QkJom159//crgvt4tDZA0ekByS/SOZ4YtIFckm5XMo7ToQCkoNNu6Y
JYfNBi9ryQMEiS0yUNghhJHxCMQp4cHISrftlPAsyv1yvf69FSoy2+RFa+KIyohK
7m6oCwCYl7I43em10kle3j8rNABEU2RCin2G92PKuweUYyabsOV8sgJpCn+r5tDJ
bIRgmSWyodP4tiu6xn1zfcK2aAQYl8PhoWIY9aSmFPKIPuxTkWu/dyNhZ2R0Ii/3
+2wU9j4bLc4ZrMROYAiQ5++EUaLIQRSVuuvJqGlfdUffJF7c6rjXHLyTKCmo079B
pCLzKBQTXQmeIWJue3/GcA8RLzcGtaTtQTJcAwNZp4V6exA869uDwFzbZA/z9jHJ
mmccdLY3hP1Ozwikm5Pecysk+bdx9rbzHbA6xLz8fp5oJYUbyyaqnWLdTZvubpur
2/6vm/KHkJHqFcF/LtIxgaZFnGYR
-----END CERTIFICATE-----`
const testKey string = `-----BEGIN RSA PRIVATE KEY-----
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
-----END RSA PRIVATE KEY-----`

func TestCCMLoadBalancers(t *testing.T) {
	testCases := []struct {
		name string
		f    func(*testing.T, *linodego.Client, *fakeAPI)
	}{
		{
			name: "Get Load Balancer",
			f:    testGetLoadBalancer,
		},
		{
			name: "Create Load Balancer",
			f:    testCreateNodeBalancer,
		},
		{
			name: "Update Load Balancer - Add Annotation",
			f:    testUpdateLoadBalancerAddAnnotation,
		},
		{
			name: "Update Load Balancer - Add Port Annotation",
			f:    testUpdateLoadBalancerAddPortAnnotation,
		},
		{
			name: "Update Load Balancer - Add TLS Port",
			f:    testUpdateLoadBalancerAddTLSPort,
		},
		{
			name: "Update Load Balancer - Specify NodeBalancerID",
			f:    testUpdateLoadBalancerAddNodeBalancerID,
		},
		{
			name: "Update Load Balancer - Proxy Protocol",
			f:    testUpdateLoadBalancerAddProxyProtocol,
		},
		{
			name: "Build Load Balancer Request",
			f:    testBuildLoadBalancerRequest,
		},
		{
			name: "Ensure Load Balancer Deleted",
			f:    testEnsureLoadBalancerDeleted,
		},
		{
			name: "Ensure Load Balancer Deleted - Preserve Annotation",
			f:    testEnsureLoadBalancerPreserveAnnotation,
		},
		{
			name: "Ensure Existing Load Balancer",
			f:    testEnsureExistingLoadBalancer,
		},
		{
			name: "Ensure New Load Balancer",
			f:    testEnsureNewLoadBalancer,
		},
		{
			name: "Ensure New Load Balancer with NodeBalancerID",
			f:    testEnsureNewLoadBalancerWithNodeBalancerID,
		},
		{
			name: "getNodeBalancerForService - NodeBalancerID does not exist",
			f:    testGetNodeBalancerForServiceIDDoesNotExist,
		},
	}

	for _, tc := range testCases {
		fake := newFake(t)
		ts := httptest.NewServer(fake)

		linodeClient := linodego.NewClient(http.DefaultClient)
		linodeClient.SetBaseURL(ts.URL)

		t.Run(tc.name, func(t *testing.T) {
			defer ts.Close()
			tc.f(t, &linodeClient, fake)
		})
	}
}

func stubService(fake *fake.Clientset, service *v1.Service) {
	fake.CoreV1().Services("").Create(service)
}

func testCreateNodeBalancer(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: randString(10),
			UID:  "foobar123",
			Annotations: map[string]string{
				annLinodeThrottle: "15",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(10),
					Protocol: "TCP",
					Port:     int32(80),
					NodePort: int32(30000),
				},
				{
					Name:     randString(10),
					Protocol: "TCP",
					Port:     int32(8080),
					NodePort: int32(30001),
				},
			},
		},
	}

	lb := &loadbalancers{client, "us-west", nil}
	var nodes []*v1.Node
	nb, err := lb.buildLoadBalancerRequest(context.TODO(), svc, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if nb.Region != lb.zone {
		t.Error("unexpected nodebalancer region")
		t.Logf("expected: %s", lb.zone)
		t.Logf("actual: %s", nb.Region)
	}

	configs, err := client.ListNodeBalancerConfigs(context.TODO(), nb.ID, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(configs) != len(svc.Spec.Ports) {
		t.Error("unexpected nodebalancer config count")
		t.Logf("expected: %v", len(svc.Spec.Ports))
		t.Logf("actual: %v", len(configs))
	}

	if !reflect.DeepEqual(err, nil) {
		t.Error("unexpected error")
		t.Logf("expected: %v", nil)
		t.Logf("actual: %v", err)
	}

	nb, err = client.GetNodeBalancer(context.TODO(), nb.ID)
	if !reflect.DeepEqual(err, nil) {
		t.Error("unexpected error")
		t.Logf("expected: %v", nil)
		t.Logf("actual: %v", err)
	}

	if nb.ClientConnThrottle != 15 {
		t.Error("unexpected ClientConnThrottle")
		t.Logf("expected: %v", 15)
		t.Logf("actual: %v", nb.ClientConnThrottle)
	}

	defer func() { _ = lb.EnsureLoadBalancerDeleted(context.TODO(), "lnodelb", svc) }()
}

func testUpdateLoadBalancerAddAnnotation(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: randString(10),
			UID:  "foobar123",
			Annotations: map[string]string{
				annLinodeThrottle: "15",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(10),
					Protocol: "TCP",
					Port:     int32(80),
					NodePort: int32(30000),
				},
			},
		},
	}

	nodes := []*v1.Node{
		{
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeInternalIP,
						Address: "127.0.0.1",
					},
				},
			},
		},
	}

	lb := &loadbalancers{client, "us-west", nil}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	defer lb.EnsureLoadBalancerDeleted(context.TODO(), "lnodelb", svc)

	lbStatus, err := lb.EnsureLoadBalancer(context.TODO(), "lnodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus

	stubService(fakeClientset, svc)
	svc.ObjectMeta.SetAnnotations(map[string]string{
		annLinodeThrottle: "10",
	})

	err = lb.UpdateLoadBalancer(context.TODO(), "lnodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error while updated annotations: %s", err)
	}

	nb, err := lb.getNodeBalancerByIPv4(context.TODO(), svc, lbStatus.Ingress[0].IP)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if nb.ClientConnThrottle != 10 {
		t.Errorf("unexpected ClientConnThrottle: expected %d, got %d", 10, nb.ClientConnThrottle)
		t.Logf("expected: %v", 10)
		t.Logf("actual: %v", nb.ClientConnThrottle)
	}
}

func testUpdateLoadBalancerAddPortAnnotation(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	targetTestPort := 80
	portConfigAnnotation := fmt.Sprintf("%s%d", annLinodePortConfigPrefix, targetTestPort)
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        randString(10),
			UID:         "foobar123",
			Annotations: map[string]string{},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(10),
					Protocol: "TCP",
					Port:     int32(80),
					NodePort: int32(30000),
				},
			},
		},
	}

	nodes := []*v1.Node{
		&v1.Node{
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeInternalIP,
						Address: "127.0.0.1",
					},
				},
			},
		},
	}

	lb := &loadbalancers{client, "us-west", nil}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	defer lb.EnsureLoadBalancerDeleted(context.TODO(), "lnodelb", svc)

	lbStatus, err := lb.EnsureLoadBalancer(context.TODO(), "lnodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus
	stubService(fakeClientset, svc)

	svc.ObjectMeta.SetAnnotations(map[string]string{
		portConfigAnnotation: `{"protocol": "http"}`,
	})

	err = lb.UpdateLoadBalancer(context.TODO(), "lnodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error while updated annotations: %s", err)
	}

	nb, err := lb.getNodeBalancerByStatus(context.TODO(), svc)
	if err != nil {
		t.Errorf("failed to get NodeBalancer by status: %v", err)
	}

	cfgs, errConfigs := client.ListNodeBalancerConfigs(context.TODO(), nb.ID, nil)
	if errConfigs != nil {
		t.Errorf("error getting NodeBalancer configs: %v", errConfigs)
	}

	expectedPortConfigs := map[int]string{
		80: "http",
	}
	observedPortConfigs := make(map[int]string)

	for _, cfg := range cfgs {
		observedPortConfigs[int(cfg.Port)] = string(cfg.Protocol)
	}

	if !reflect.DeepEqual(expectedPortConfigs, observedPortConfigs) {
		t.Errorf("NodeBalancer port mismatch: expected %v, got %v", expectedPortConfigs, observedPortConfigs)
	}
}

func testUpdateLoadBalancerAddTLSPort(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: randString(10),
			UID:  "foobar123",
			Annotations: map[string]string{
				annLinodeThrottle: "15",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(10),
					Protocol: "TCP",
					Port:     int32(80),
					NodePort: int32(30000),
				},
			},
		},
	}

	nodes := []*v1.Node{
		&v1.Node{
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeInternalIP,
						Address: "127.0.0.1",
					},
				},
			},
		},
	}

	extraPort := v1.ServicePort{
		Name:     randString(10),
		Protocol: "TCP",
		Port:     int32(443),
		NodePort: int32(30001),
	}

	lb := &loadbalancers{client, "us-west", nil}

	defer lb.EnsureLoadBalancerDeleted(context.TODO(), "lnodelb", svc)

	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset
	addTLSSecret(t, lb.kubeClient)

	lbStatus, err := lb.EnsureLoadBalancer(context.TODO(), "lnodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus

	stubService(fakeClientset, svc)
	svc.Spec.Ports = append(svc.Spec.Ports, extraPort)
	svc.ObjectMeta.SetAnnotations(map[string]string{
		annLinodePortConfigPrefix + "443": `{ "protocol": "https", "tls-secret-name": "tls-secret"}`,
	})
	err = lb.UpdateLoadBalancer(context.TODO(), "lnodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error while updated annotations: %s", err)
	}

	nb, err := lb.getNodeBalancerByStatus(context.TODO(), svc)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	cfgs, errConfigs := client.ListNodeBalancerConfigs(context.TODO(), nb.ID, nil)
	if errConfigs != nil {
		t.Errorf("error getting NodeBalancer configs: %v", errConfigs)
	}

	expectedPorts := map[int]struct{}{
		80:  struct{}{},
		443: struct{}{},
	}

	observedPorts := make(map[int]struct{})

	for _, cfg := range cfgs {
		nodes, errNodes := client.ListNodeBalancerNodes(context.TODO(), nb.ID, cfg.ID, nil)
		if errNodes != nil {
			t.Errorf("error getting NodeBalancer nodes: %v", errNodes)
		}

		if len(nodes) == 0 {
			t.Errorf("no nodes found for port %d", cfg.Port)
		}

		observedPorts[int(cfg.Port)] = struct{}{}
	}

	if !reflect.DeepEqual(expectedPorts, observedPorts) {
		t.Errorf("NodeBalancer ports mismatch: expected %v, got %v", expectedPorts, observedPorts)
	}
}

func testUpdateLoadBalancerAddProxyProtocol(t *testing.T, client *linodego.Client, fakeAPI *fakeAPI) {
	nodes := []*v1.Node{
		{
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeInternalIP,
						Address: "127.0.0.1",
					},
				},
			},
		},
	}

	lb := &loadbalancers{client, "us-west", nil}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	for _, tc := range []struct {
		name                string
		proxyProtocolConfig linodego.ConfigProxyProtocol
		invalidErr          bool
	}{
		{
			name:                "with invalid Proxy Protocol",
			proxyProtocolConfig: "bogus",
			invalidErr:          true,
		},
		{
			name:                "with none",
			proxyProtocolConfig: linodego.ProxyProtocolNone,
		},
		{
			name:                "with v1",
			proxyProtocolConfig: linodego.ProxyProtocolV1,
		},
		{
			name:                "with v2",
			proxyProtocolConfig: linodego.ProxyProtocolV2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        randString(10),
					UID:         "foobar123",
					Annotations: map[string]string{},
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:     randString(10),
							Protocol: "tcp",
							Port:     int32(80),
							NodePort: int32(8080),
						},
					},
				},
			}

			defer lb.EnsureLoadBalancerDeleted(context.TODO(), "lnodelb", svc)
			nodeBalancer, err := client.CreateNodeBalancer(context.TODO(), linodego.NodeBalancerCreateOptions{
				Region: lb.zone,
			})

			if err != nil {
				t.Fatalf("failed to create NodeBalancer: %s", err)
			}

			svc.Status.LoadBalancer = *makeLoadBalancerStatus(nodeBalancer)
			svc.ObjectMeta.SetAnnotations(map[string]string{
				annLinodeProxyProtocol: string(tc.proxyProtocolConfig),
			})

			stubService(fakeClientset, svc)
			if err = lb.UpdateLoadBalancer(context.TODO(), "lnodelb", svc, nodes); err != nil {
				expectedErrMessage := fmt.Sprintf("invalid NodeBalancer proxy protocol value '%s'", tc.proxyProtocolConfig)
				if tc.invalidErr && err.Error() == expectedErrMessage {
					return
				}
				t.Fatalf("UpdateLoadBalancer returned an unexpected error while updated annotations: %s", err)
				return
			}
			if tc.invalidErr {
				t.Fatal("expected UpdateLoadBalancer to return an error")
			}

			nodeBalancerConfigs, err := client.ListNodeBalancerConfigs(context.TODO(), nodeBalancer.ID, nil)
			if err != nil {
				t.Fatalf("failed to get NodeBalancer: %s", err)
			}

			for _, config := range nodeBalancerConfigs {
				proxyProtocol := config.ProxyProtocol
				if proxyProtocol != tc.proxyProtocolConfig {
					t.Errorf("expected ProxyProtocol to be %s; got %s", tc.proxyProtocolConfig, proxyProtocol)
				}
			}
		})
	}
}

func testUpdateLoadBalancerAddNodeBalancerID(t *testing.T, client *linodego.Client, fakeAPI *fakeAPI) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        randString(10),
			UID:         "foobar123",
			Annotations: map[string]string{},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(10),
					Protocol: "http",
					Port:     int32(80),
					NodePort: int32(8080),
				},
			},
		},
	}

	nodes := []*v1.Node{
		{
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeInternalIP,
						Address: "127.0.0.1",
					},
				},
			},
		},
	}

	lb := &loadbalancers{client, "us-west", nil}
	defer lb.EnsureLoadBalancerDeleted(context.TODO(), "lnodelb", svc)

	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	nodeBalancer, err := client.CreateNodeBalancer(context.TODO(), linodego.NodeBalancerCreateOptions{
		Region: lb.zone,
	})
	if err != nil {
		t.Fatalf("failed to create NodeBalancer: %s", err)
	}

	svc.Status.LoadBalancer = *makeLoadBalancerStatus(nodeBalancer)

	newNodeBalancer, err := client.CreateNodeBalancer(context.TODO(), linodego.NodeBalancerCreateOptions{
		Region: lb.zone,
	})
	if err != nil {
		t.Fatalf("failed to create new NodeBalancer: %s", err)
	}

	stubService(fakeClientset, svc)
	svc.ObjectMeta.SetAnnotations(map[string]string{
		annLinodeNodeBalancerID: strconv.Itoa(newNodeBalancer.ID),
	})
	err = lb.UpdateLoadBalancer(context.TODO(), "lnodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error while updated annotations: %s", err)
	}

	lbStatus, _, err := lb.GetLoadBalancer(context.TODO(), svc.ClusterName, svc)
	if err != nil {
		t.Errorf("GetLoadBalancer returned an error: %s", err)
	}

	expectedLBStatus := makeLoadBalancerStatus(newNodeBalancer)
	if !reflect.DeepEqual(expectedLBStatus, lbStatus) {
		t.Errorf("LoadBalancer status mismatch: expected %v, got %v", expectedLBStatus, lbStatus)
	}

	if !fakeAPI.didRequestOccur(http.MethodDelete, fmt.Sprintf("/nodebalancers/%d", nodeBalancer.ID), "") {
		t.Errorf("expected old NodeBalancer to have been deleted")
	}
}

func Test_getConnectionThrottle(t *testing.T) {
	testcases := []struct {
		name     string
		service  *v1.Service
		expected int
	}{
		{
			"throttle not specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        randString(10),
					UID:         "abc123",
					Annotations: map[string]string{},
				},
			},
			20,
		},
		{
			"throttle value is a string",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodeThrottle: "foo",
					},
				},
			},
			20,
		},
		{
			"throttle value is less than 0",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodeThrottle: "-123",
					},
				},
			},
			0,
		},
		{
			"throttle value is valid",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodeThrottle: "1",
					},
				},
			},
			1,
		},
		{
			"throttle value is too high",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodeThrottle: "21",
					},
				},
			},
			20,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			connThrottle := getConnectionThrottle(test.service)

			if test.expected != connThrottle {
				t.Fatalf("expected throttle value (%d) does not match actual value (%d)", test.expected, connThrottle)
			}
		})
	}
}

func Test_getPortConfig(t *testing.T) {
	testcases := []struct {
		name               string
		service            *v1.Service
		expectedPortConfig portConfig
		err                error
	}{
		{
			"default no protocol specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
				},
			},
			portConfig{Port: 443, Protocol: "tcp"},

			nil,
		},
		{
			"default tcp protocol specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodeDefaultProtocol: "tcp",
					},
				},
			},
			portConfig{Port: 443, Protocol: "tcp"},
			nil,
		},
		{
			"default capitalized protocol specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodeDefaultProtocol: "HTTP",
					},
				},
			},
			portConfig{Port: 443, Protocol: "http"},
			nil,
		},
		{
			"default invalid protocol",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodeDefaultProtocol: "invalid",
					},
				},
			},
			portConfig{},
			fmt.Errorf("invalid protocol: %q specified", "invalid"),
		},
		{
			"port config falls back to default",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodeDefaultProtocol:          "http",
						annLinodePortConfigPrefix + "443": `{}`,
					},
				},
			},
			portConfig{Port: 443, Protocol: "http"},
			nil,
		},
		{
			"port config capitalized protocol",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodePortConfigPrefix + "443": `{ "protocol": "HTTp" }`,
					},
				},
			},
			portConfig{Port: 443, Protocol: "http"},
			nil,
		},
		{
			"port config invalid protocol",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodePortConfigPrefix + "443": `{ "protocol": "invalid" }`,
					},
				},
			},
			portConfig{},
			fmt.Errorf("invalid protocol: %q specified", "invalid"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			testPort := 443
			portConfig, err := getPortConfig(test.service, testPort)

			if !reflect.DeepEqual(portConfig, test.expectedPortConfig) {
				t.Error("unexpected port config")
				t.Logf("expected: %q", test.expectedPortConfig)
				t.Logf("actual: %q", portConfig)
			}

			if !reflect.DeepEqual(err, test.err) {
				t.Error("unexpected error")
				t.Logf("expected: %q", test.err)
				t.Logf("actual: %q", err)
			}
		})
	}
}

func Test_getHealthCheckType(t *testing.T) {
	testcases := []struct {
		name       string
		service    *v1.Service
		healthType linodego.ConfigCheck
		err        error
	}{
		{
			"no type specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        randString(10),
					UID:         "abc123",
					Annotations: map[string]string{},
				},
			},
			linodego.CheckConnection,
			nil,
		},
		{
			"http specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodeHealthCheckType: "http",
					},
				},
			},
			linodego.CheckHTTP,
			nil,
		},
		{
			"invalid specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodeHealthCheckType: "invalid",
					},
				},
			},
			"",
			fmt.Errorf("invalid health check type: %q specified in annotation: %q", "invalid", annLinodeHealthCheckType),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			hType, err := getHealthCheckType(test.service)
			if !reflect.DeepEqual(hType, test.healthType) {
				t.Error("unexpected health check type")
				t.Logf("expected: %v", test.healthType)
				t.Logf("actual: %v", hType)
			}

			if !reflect.DeepEqual(err, test.err) {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.err)
				t.Logf("actual: %v", err)
			}
		})
	}
}

func Test_getNodeInternalIP(t *testing.T) {
	testcases := []struct {
		name    string
		node    *v1.Node
		address string
	}{
		{
			"node internal ip specified",
			&v1.Node{
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    v1.NodeInternalIP,
							Address: "127.0.0.1",
						},
					},
				},
			},
			"127.0.0.1",
		},
		{
			"node internal ip not specified",
			&v1.Node{
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    v1.NodeExternalIP,
							Address: "127.0.0.1",
						},
					},
				},
			},
			"",
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ip := getNodeInternalIP(test.node)
			if ip != test.address {
				t.Error("unexpected certificate")
				t.Logf("expected: %q", test.address)
				t.Logf("actual: %q", ip)
			}
		})
	}

}

func testBuildLoadBalancerRequest(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			UID:  "foobar123",
			Annotations: map[string]string{
				annLinodeDefaultProtocol: "tcp",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     "test",
					Protocol: "TCP",
					Port:     int32(80),
					NodePort: int32(30000),
				},
			},
		},
	}
	nodes := []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-3",
			},
		},
	}

	lb := &loadbalancers{client, "us-west", nil}
	nb, err := lb.buildLoadBalancerRequest(context.TODO(), svc, nodes)
	if err != nil {
		t.Fatal(err)
	}

	if nb == nil {
		t.Error("unexpected nodeID")
		t.Logf("expected: != \"\"")
		t.Logf("actual: %v", lb)
	}
	if !reflect.DeepEqual(err, err) {
		t.Error("unexpected error")
		t.Logf("expected: %v", nil)
		t.Logf("actual: %v", err)
	}

	configs, err := client.ListNodeBalancerConfigs(context.TODO(), nb.ID, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(configs) != len(svc.Spec.Ports) {
		t.Error("unexpected nodebalancer config count")
		t.Logf("expected: %v", len(svc.Spec.Ports))
		t.Logf("actual: %v", len(configs))
	}

	nbNodes, _ := client.ListNodeBalancerNodes(context.TODO(), nb.ID, configs[0].ID, nil)

	if len(nbNodes) != len(nodes) {
		t.Error("unexpected nodebalancer nodes count")
		t.Logf("expected: %v", len(nodes))
		t.Logf("actual: %v", len(nbNodes))
	}

}

func testEnsureLoadBalancerPreserveAnnotation(t *testing.T, client *linodego.Client, fake *fakeAPI) {
	testServiceSpec := v1.ServiceSpec{
		Ports: []v1.ServicePort{
			{
				Name:     "test",
				Protocol: "TCP",
				Port:     int32(80),
				NodePort: int32(30000),
			},
		},
	}

	lb := &loadbalancers{client, "us-west", nil}
	for _, test := range []struct {
		name        string
		deleted     bool
		annotations map[string]string
	}{
		{
			name:        "load balancer preserved",
			annotations: map[string]string{annLinodeLoadBalancerPreserve: "true"},
			deleted:     false,
		},
		{
			name:        "load balancer not preserved (deleted)",
			annotations: map[string]string{annLinodeLoadBalancerPreserve: "false"},
			deleted:     true,
		},
		{
			name:        "invalid value treated as false (deleted)",
			annotations: map[string]string{annLinodeLoadBalancerPreserve: "bogus"},
			deleted:     true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test",
					UID:         types.UID("foobar" + randString(10)),
					Annotations: test.annotations,
				},
				Spec: testServiceSpec,
			}

			nb, err := lb.createNodeBalancer(context.TODO(), svc, []*linodego.NodeBalancerConfigCreateOptions{})
			if err != nil {
				t.Fatal(err)
			}

			svc.Status.LoadBalancer = *makeLoadBalancerStatus(nb)
			err = lb.EnsureLoadBalancerDeleted(context.TODO(), "lnodelb", svc)

			didDelete := fake.didRequestOccur(http.MethodDelete, fmt.Sprintf("/nodebalancers/%d", nb.ID), "")
			if didDelete && !test.deleted {
				t.Fatal("load balancer was unexpectedly deleted")
			} else if !didDelete && test.deleted {
				t.Fatal("load balancer was unexpectedly preserved")
			}

			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
		})
	}
}

func testEnsureLoadBalancerDeleted(t *testing.T, client *linodego.Client, fake *fakeAPI) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			UID:  "foobar123",
			Annotations: map[string]string{
				annLinodeDefaultProtocol: "tcp",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     "test",
					Protocol: "TCP",
					Port:     int32(80),
					NodePort: int32(30000),
				},
			},
		},
	}
	testcases := []struct {
		name        string
		clusterName string
		service     *v1.Service
		err         error
	}{
		{
			"load balancer delete",
			"linodelb",
			svc,
			nil,
		},
		{
			"load balancer not exists",
			"linodelb",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "notexists",
					UID:  "notexists123",
					Annotations: map[string]string{
						annLinodeDefaultProtocol: "tcp",
					},
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:     "test",
							Protocol: "TCP",
							Port:     int32(80),
							NodePort: int32(30000),
						},
					},
				},
			},
			nil,
		},
	}

	lb := &loadbalancers{client, "us-west", nil}
	configs := []*linodego.NodeBalancerConfigCreateOptions{}
	_, err := lb.createNodeBalancer(context.TODO(), svc, configs)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lb.EnsureLoadBalancerDeleted(context.TODO(), "lnodelb", svc) }()

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := lb.EnsureLoadBalancerDeleted(context.TODO(), test.clusterName, test.service)
			if !reflect.DeepEqual(err, test.err) {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.err)
				t.Logf("actual: %v", err)
			}
		})
	}
}

func testEnsureExistingLoadBalancer(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testensure",
			UID:  "foobar123",
			Annotations: map[string]string{
				annLinodeDefaultProtocol:           "tcp",
				annLinodePortConfigPrefix + "8443": `{ "protocol": "https", "tls-secret-name": "tls-secret"}`,
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     "test",
					Protocol: "TCP",
					Port:     int32(8443),
					NodePort: int32(30000),
				},
				{
					Name:     "test2",
					Protocol: "TCP",
					Port:     int32(80),
					NodePort: int32(30001),
				},
			},
		},
	}

	lb := &loadbalancers{client, "us-west", nil}
	lb.kubeClient = fake.NewSimpleClientset()
	addTLSSecret(t, lb.kubeClient)

	configs := []*linodego.NodeBalancerConfigCreateOptions{}
	nb, err := lb.createNodeBalancer(context.TODO(), svc, configs)
	if err != nil {
		t.Fatal(err)
	}

	svc.Status.LoadBalancer = *makeLoadBalancerStatus(nb)
	defer func() { _ = lb.EnsureLoadBalancerDeleted(context.TODO(), "lnodelb", svc) }()
	getLBStatus, exists, err := lb.GetLoadBalancer(context.TODO(), "linodelb", svc)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("Node balancer not found")
	}

	testcases := []struct {
		name        string
		service     *v1.Service
		nodes       []*v1.Node
		clusterName string
		nbIP        string
		err         error
	}{
		{
			"update load balancer",
			svc,
			[]*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
					},
					Status: v1.NodeStatus{
						Addresses: []v1.NodeAddress{
							{
								Type:    v1.NodeInternalIP,
								Address: "127.0.0.1",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-2",
					},
					Status: v1.NodeStatus{
						Addresses: []v1.NodeAddress{
							{
								Type:    v1.NodeInternalIP,
								Address: "127.0.0.2",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-3",
					},
					Status: v1.NodeStatus{
						Addresses: []v1.NodeAddress{
							{
								Type:    v1.NodeInternalIP,
								Address: "127.0.0.3",
							},
						},
					},
				},
			},
			"linodelb",
			getLBStatus.Ingress[0].IP,
			nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			lbStatus, err := lb.EnsureLoadBalancer(context.TODO(), test.clusterName, test.service, test.nodes)
			if err != nil {
				t.Fatal(err)
			}
			if lbStatus.Ingress[0].IP != test.nbIP {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.nbIP)
				t.Logf("actual: %v", lbStatus.Ingress)
			}
			if !reflect.DeepEqual(err, test.err) {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.err)
				t.Logf("actual: %v", err)
			}
		})
	}
}

func testGetNodeBalancerForServiceIDDoesNotExist(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	lb := &loadbalancers{client, "us-west", nil}
	bogusNodeBalancerID := "123456"

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			UID:  "foobar123",
			Annotations: map[string]string{
				annLinodeNodeBalancerID: bogusNodeBalancerID,
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     "test",
					Protocol: "TCP",
					Port:     int32(8443),
					NodePort: int32(30000),
				},
			},
		},
	}

	_, err := lb.getNodeBalancerForService(context.TODO(), svc)
	if err == nil {
		t.Fatal("expected getNodeBalancerForService to return an error")
	}

	expectedErr := fmt.Sprintf("%s annotation points to a NodeBalancer that does not exist: LoadBalancer (%s) not found for service (%s)",
		annLinodeNodeBalancerID, bogusNodeBalancerID, getServiceNn(svc))

	if err.Error() != expectedErr {
		t.Errorf("expected error to be '%s' but got '%s'", expectedErr, err)
	}
}

func testEnsureNewLoadBalancerWithNodeBalancerID(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	lb := &loadbalancers{client, "us-west", nil}
	nodeBalancer, err := client.CreateNodeBalancer(context.TODO(), linodego.NodeBalancerCreateOptions{
		Region: lb.zone,
	})
	if err != nil {
		t.Fatalf("failed to create NodeBalancer: %s", err)
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testensure",
			UID:  "foobar123",
			Annotations: map[string]string{
				annLinodeNodeBalancerID: strconv.Itoa(nodeBalancer.ID),
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     "test",
					Protocol: "TCP",
					Port:     int32(8443),
					NodePort: int32(30000),
				},
			},
		},
	}

	nodes := []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
			},
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeInternalIP,
						Address: "127.0.0.1",
					},
				},
			},
		},
	}

	defer func() { _ = lb.EnsureLoadBalancerDeleted(context.TODO(), "lnodelb", svc) }()

	if _, err = lb.EnsureLoadBalancer(context.TODO(), "lnodelb", svc, nodes); err != nil {
		t.Fatal(err)
	}
}

func testEnsureNewLoadBalancer(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testensure",
			UID:  "foobar123",
			Annotations: map[string]string{
				annLinodeDefaultProtocol:           "tcp",
				annLinodePortConfigPrefix + "8443": `{ "protocol": "https", "tls-secret-name": "tls-secret"}`,
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     "test",
					Protocol: "TCP",
					Port:     int32(8443),
					NodePort: int32(30000),
				},
				{
					Name:     "test2",
					Protocol: "TCP",
					Port:     int32(80),
					NodePort: int32(30001),
				},
			},
		},
	}

	nodes := []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
			},
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeInternalIP,
						Address: "127.0.0.1",
					},
				},
			},
		},
	}
	lb := &loadbalancers{client, "us-west", nil}
	lb.kubeClient = fake.NewSimpleClientset()
	addTLSSecret(t, lb.kubeClient)

	defer func() { _ = lb.EnsureLoadBalancerDeleted(context.TODO(), "lnodelb", svc) }()

	_, err := lb.EnsureLoadBalancer(context.TODO(), "lnodelb", svc, nodes)
	if err != nil {
		t.Fatal(err)
	}
}

func testGetLoadBalancer(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	lb := &loadbalancers{client, "us-west", nil}
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			UID:  "foobar123",
			Annotations: map[string]string{
				annLinodeDefaultProtocol: "tcp",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     "test",
					Protocol: "TCP",
					Port:     int32(80),
					NodePort: int32(30000),
				},
			},
		},
	}

	configs := []*linodego.NodeBalancerConfigCreateOptions{}
	nb, err := lb.createNodeBalancer(context.TODO(), svc, configs)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lb.EnsureLoadBalancerDeleted(context.TODO(), "lnodelb", svc) }()

	lbStatus := makeLoadBalancerStatus(nb)
	svc.Status.LoadBalancer = *lbStatus

	testcases := []struct {
		name        string
		service     *v1.Service
		clusterName string
		found       bool
		err         error
	}{
		{
			"Load balancer exists",
			svc,
			"linodelb",
			true,
			nil,
		},
		{
			"Load balancer not exists",

			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "notexists",
					UID:  "notexists123",
					Annotations: map[string]string{
						annLinodeDefaultProtocol: "tcp",
					},
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:     "test",
							Protocol: "TCP",
							Port:     int32(80),
							NodePort: int32(30000),
						},
					},
				},
			},
			"linodelb",
			false,
			nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {

			_, found, err := lb.GetLoadBalancer(context.TODO(), test.clusterName, test.service)
			if found != test.found {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.found)
				t.Logf("actual: %v", found)
			}
			if !reflect.DeepEqual(err, test.err) {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.err)
				t.Logf("actual: %v", err)
			}
		})
	}
}

func Test_getPortConfigAnnotation(t *testing.T) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     "test",
					Protocol: "TCP",
					Port:     int32(80),
					NodePort: int32(30000),
				},
			},
		},
	}

	testcases := []struct {
		name     string
		ann      map[string]string
		expected portConfigAnnotation
		err      string
	}{
		{
			name: "Test single port annotation",
			ann:  map[string]string{annLinodePortConfigPrefix + "443": `{ "tls-secret-name": "prod-app-tls", "protocol": "https" }`},
			expected: portConfigAnnotation{
				TLSSecretName: "prod-app-tls",
				Protocol:      "https",
			},
			err: "",
		},
		{
			name: "Test multiple port annotation",
			ann: map[string]string{
				annLinodePortConfigPrefix + "443": `{ "tls-secret-name": "prod-app-tls", "protocol": "https" }`,
				annLinodePortConfigPrefix + "80":  `{ "protocol": "http" }`,
			},
			expected: portConfigAnnotation{
				TLSSecretName: "prod-app-tls",
				Protocol:      "https",
			},
			err: "",
		},
		{
			name: "Test no port annotation",
			ann:  map[string]string{},
			expected: portConfigAnnotation{
				Protocol: "",
			},
			err: "",
		},
		{
			name: "Test invalid json",
			ann: map[string]string{
				annLinodePortConfigPrefix + "443": `{ "tls-secret-name": "prod-app-tls" `,
			},
			expected: portConfigAnnotation{},
			err:      "unexpected end of JSON input",
		},
	}
	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			svc.Annotations = test.ann
			ann, err := getPortConfigAnnotation(svc, 443)
			if !reflect.DeepEqual(ann, test.expected) {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.expected)
				t.Logf("actual: %v", ann)
			}
			if test.err != "" && test.err != err.Error() {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.err)
				t.Logf("actual: %v", err)
			}
		})
	}
}

func Test_getTLSCertInfo(t *testing.T) {
	kubeClient := fake.NewSimpleClientset()
	addTLSSecret(t, kubeClient)

	testcases := []struct {
		name       string
		portConfig portConfig
		namespace  string
		cert       string
		key        string
		err        error
	}{
		{
			name: "Test valid Cert info",
			portConfig: portConfig{
				TLSSecretName: "tls-secret",
				Port:          8080,
			},
			namespace: "test",
			cert:      testCert,
			key:       testKey,
			err:       nil,
		},
		{
			name: "Test unspecified Cert info",
			portConfig: portConfig{
				Port: 8080,
			},
			namespace: "test",
			cert:      "",
			key:       "",
			err:       fmt.Errorf("TLS secret name for port 8080 is not specified"),
		},
		{
			name: "Test blank Cert info",
			portConfig: portConfig{
				TLSSecretName: "",
				Port:          8080,
			},
			namespace: "test",
			cert:      "",
			key:       "",
			err:       fmt.Errorf("TLS secret name for port 8080 is not specified"),
		},
		{
			name: "Test no secret found",
			portConfig: portConfig{
				TLSSecretName: "secret",
				Port:          8080,
			},
			namespace: "test",
			cert:      "",
			key:       "",
			err: errors.NewNotFound(schema.GroupResource{
				Group:    "",
				Resource: "secrets",
			}, "secret"), /*{}(`secrets "secret" not found`)*/
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			cert, key, err := getTLSCertInfo(kubeClient, test.namespace, test.portConfig)
			if cert != test.cert {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.cert)
				t.Logf("actual: %v", cert)
			}
			if key != test.key {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.key)
				t.Logf("actual: %v", key)
			}
			if !reflect.DeepEqual(err, test.err) {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.err)
				t.Logf("actual: %v", err)
			}
		})
	}
}

func addTLSSecret(t *testing.T, kubeClient kubernetes.Interface) {
	_, err := kubeClient.CoreV1().Secrets("test").Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tls-secret",
		},
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte(testCert),
			v1.TLSPrivateKeyKey: []byte(testKey),
		},
		StringData: nil,
		Type:       "kubernetes.io/tls",
	})
	if err != nil {
		t.Error(err)
	}
}

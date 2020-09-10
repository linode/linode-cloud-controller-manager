package linode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"reflect"
	"testing"

	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCCMLoadBalancersDeprecated(t *testing.T) {
	fake := newFake(t)
	ts := httptest.NewServer(fake)
	defer ts.Close()

	linodeClient := linodego.NewClient(http.DefaultClient)
	linodeClient.SetBaseURL(ts.URL)

	testCases := []struct {
		name string
		f    func(*testing.T, *linodego.Client)
	}{
		{
			name: "Get Load Balancer",
			f:    testGetLoadBalancerDeprecated,
		},
		{
			name: "Build Load Balancer Request",
			f:    testBuildLoadBalancerRequestDeprecated,
		},
		{
			name: "Ensure Load Balancer Deleted",
			f:    testEnsureLoadBalancerDeletedDeprecated,
		},
		{
			name: "Ensure Load Balancer",
			f:    testEnsureLoadBalancerDeprecated,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.f(t, &linodeClient)
		})
	}
}

func testBuildLoadBalancerRequestDeprecated(t *testing.T, client *linodego.Client) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			UID:  "foobar123",
			Annotations: map[string]string{
				annLinodeProtocolDeprecated: "tcp",
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

func testEnsureLoadBalancerDeletedDeprecated(t *testing.T, client *linodego.Client) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			UID:  "foobar123",
			Annotations: map[string]string{
				annLinodeProtocolDeprecated: "tcp",
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
						annLinodeProtocolDeprecated: "tcp",
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

func testEnsureLoadBalancerDeprecated(t *testing.T, client *linodego.Client) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testensure",
			UID:  "foobar123",
			Annotations: map[string]string{
				annLinodeProtocolDeprecated: "tcp",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     "test",
					Protocol: "TCP",
					Port:     int32(8000),
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

	configs := []*linodego.NodeBalancerConfigCreateOptions{}
	nb, err := lb.createNodeBalancer(context.TODO(), svc, configs)
	if err != nil {
		t.Fatal(err)
	}

	svc.Status.LoadBalancer = *makeLoadBalancerStatus(nb)
	defer func() { _ = lb.EnsureLoadBalancerDeleted(context.TODO(), "lnodelb", svc) }()
	lbStatus, exists, err := lb.GetLoadBalancer(context.TODO(), "linodelb", svc)
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
			lbStatus.Ingress[0].IP,
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

func testGetLoadBalancerDeprecated(t *testing.T, client *linodego.Client) {
	lb := &loadbalancers{client, "us-west", nil}
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			UID:  "foobar123",
			Annotations: map[string]string{
				annLinodeProtocolDeprecated: "tcp",
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

	lbStatus := makeLoadBalancerStatus(nb)
	svc.Status.LoadBalancer = *lbStatus
	defer func() { _ = lb.EnsureLoadBalancerDeleted(context.TODO(), "lnodelb", svc) }()
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
						annLinodeProtocolDeprecated: "tcp",
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

func Test_getTLSAnnotationsDeprecated(t *testing.T) {
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
		name   string
		ann    map[string]string
		annTLS *tlsAnnotationDeprecated
		err    error
	}{
		{
			name: "Test single TLS annotation",
			ann:  map[string]string{annLinodeLoadBalancerTLSDeprecated: `[ { "tls-secret-name": "prod-app-tls", "port": 443} ]`},
			annTLS: &tlsAnnotationDeprecated{
				TLSSecretName: "prod-app-tls",
				Port:          443,
			},
			err: nil,
		},
		{
			name: "Test multiple TLS annotation",
			ann:  map[string]string{annLinodeLoadBalancerTLSDeprecated: `[ { "tls-secret-name": "prod-app-tls", "port": 443}, {"tls-secret-name": "dev-app-tls", "port": 8443} ]`},
			annTLS: &tlsAnnotationDeprecated{
				TLSSecretName: "prod-app-tls",
				Port:          443,
			},
			err: nil,
		},
		{
			name:   "Test invalid json",
			ann:    map[string]string{annLinodeLoadBalancerTLSDeprecated: `[ { "tls-secret-name": "prod-app-tls", "port": 443}`},
			annTLS: nil,
			err:    json.Unmarshal([]byte(`[ { "tls-secret-name": "prod-app-tls", "port": 443}`), &tlsAnnotationDeprecated{}),
		},
	}
	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			svc.Annotations = test.ann
			ann, err := getTLSAnnotationDeprecated(svc, 443)
			if !reflect.DeepEqual(ann, test.annTLS) {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.annTLS)
				t.Logf("actual: %v", ann)
			}
			if !reflect.DeepEqual(err, test.err) {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.err)
				t.Logf("actual: %v", err)
			}
		})
	}
}

func Test_tryDeprecatedTLSAnnotation(t *testing.T) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	testcases := []struct {
		name                  string
		ann                   map[string]string
		expectedProtocol      string
		expectedTLSSecretName string
	}{
		{
			name:                  "Test TLS annotation for port in JSON",
			ann:                   map[string]string{annLinodeLoadBalancerTLSDeprecated: `[ { "tls-secret-name": "prod-app-tls", "port": 443} ]`},
			expectedProtocol:      "https",
			expectedTLSSecretName: "prod-app-tls",
		},
		{
			name:                  "Test Linode Protocol set as default",
			ann:                   map[string]string{annLinodeProtocolDeprecated: `https`},
			expectedProtocol:      "https",
			expectedTLSSecretName: "",
		},
		{
			name: "Test Linode Protocol when both set",
			ann: map[string]string{
				annLinodeLoadBalancerTLSDeprecated: `[ { "tls-secret-name": "prod-app-tls", "port": 443} ]`,
				annLinodeProtocolDeprecated:        `tcp`,
			},
			expectedProtocol:      "https",
			expectedTLSSecretName: "prod-app-tls",
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			svc.Annotations = test.ann
			portConfigAnnotation, _ := tryDeprecatedTLSAnnotation(svc, 443)
			if portConfigAnnotation.Protocol != test.expectedProtocol {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.expectedProtocol)
				t.Logf("actual: %v", portConfigAnnotation.Protocol)
			}
			if portConfigAnnotation.TLSSecretName != test.expectedTLSSecretName {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.expectedTLSSecretName)
				t.Logf("actual: %v", portConfigAnnotation.TLSSecretName)
			}
		})
	}
}

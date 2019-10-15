package linode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/cloudprovider"
)

func TestCCMLoadBalancers(t *testing.T) {
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
			f:    testGetLoadBalancer,
		},
		{
			name: "Create Load Balancer",
			f:    testCreateNodeBalancer,
		},
		{
			name: "Update Load Balancer",
			f:    testUpdateLoadBalancer,
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
			name: "Ensure Load Balancer",
			f:    testEnsureLoadBalancer,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.f(t, &linodeClient)
		})
	}
}

func testCreateNodeBalancer(t *testing.T, client *linodego.Client) {
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

func testUpdateLoadBalancer(t *testing.T, client *linodego.Client) {
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

	lb := &loadbalancers{client, "us-west", nil}
	_, err := lb.EnsureLoadBalancer(context.TODO(), "lnodelb", svc, []*v1.Node{})
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.ObjectMeta.SetAnnotations(map[string]string{
		annLinodeThrottle: "10",
	})
	err = lb.UpdateLoadBalancer(context.TODO(), "lnodelb", svc, []*v1.Node{})
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error while updated annotations: %s", err)
	}

	lbName := cloudprovider.GetLoadBalancerName(svc)
	nb, err := lb.lbByName(context.TODO(), lb.client, lbName)

	if !reflect.DeepEqual(err, nil) {
		t.Error("unexpected error")
		t.Logf("expected: %v", nil)
		t.Logf("actual: %v", err)
	}

	if nb.ClientConnThrottle != 10 {
		t.Error("unexpected ClientConnThrottle")
		t.Logf("expected: %v", 10)
		t.Logf("actual: %v", nb.ClientConnThrottle)
	}

	defer func() { _ = lb.EnsureLoadBalancerDeleted(context.TODO(), "lnodelb", svc) }()
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

func Test_getProtocol(t *testing.T) {
	testcases := []struct {
		name     string
		service  *v1.Service
		protocol linodego.ConfigProtocol
		err      error
	}{
		{
			"no protocol specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
				},
			},
			linodego.ProtocolTCP,
			nil,
		},
		{
			"tcp protocol specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodeProtocol: "http",
					},
				},
			},
			linodego.ProtocolHTTP,
			nil,
		},
		{
			"invalid protocol",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodeProtocol: "invalid",
					},
				},
			},
			"",
			fmt.Errorf("invalid protocol: %q specified in annotation: %q", "invalid", annLinodeProtocol),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			protocol, err := getProtocol(test.service)
			if protocol != test.protocol {
				t.Error("unexpected protocol")
				t.Logf("expected: %q", test.protocol)
				t.Logf("actual: %q", protocol)
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

func testBuildLoadBalancerRequest(t *testing.T, client *linodego.Client) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			UID:  "foobar123",
			Annotations: map[string]string{
				annLinodeProtocol: "tcp",
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

func testEnsureLoadBalancerDeleted(t *testing.T, client *linodego.Client) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			UID:  "foobar123",
			Annotations: map[string]string{
				annLinodeProtocol: "tcp",
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
						annLinodeProtocol: "tcp",
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

func testEnsureLoadBalancer(t *testing.T, client *linodego.Client) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testensure",
			UID:  "foobar123",
			Annotations: map[string]string{
				annLinodeProtocol: "tcp",
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
	_, err := lb.createNodeBalancer(context.TODO(), svc, configs)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lb.EnsureLoadBalancerDeleted(context.TODO(), "lnodelb", svc) }()
	nb, exists, err := lb.GetLoadBalancer(context.TODO(), "linodelb", svc)
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
			nb.Ingress[0].IP,
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

func testGetLoadBalancer(t *testing.T, client *linodego.Client) {
	lb := &loadbalancers{client, "us-west", nil}
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			UID:  "foobar123",
			Annotations: map[string]string{
				annLinodeProtocol: "tcp",
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
	_, err := lb.createNodeBalancer(context.TODO(), svc, configs)
	if err != nil {
		t.Fatal(err)
	}
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
						annLinodeProtocol: "tcp",
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

func Test_getTLSPorts(t *testing.T) {
	testcases := []struct {
		name    string
		ann     []*tlsAnnotation
		portLen int
	}{
		{
			name: "Test get TLS ports",
			ann: []*tlsAnnotation{
				{
					TLSSecretName: "secret-1",
					Port:          8080,
				},
				{
					TLSSecretName: "secret-2",
					Port:          8081,
				},
			},
			portLen: 2,
		},
	}
	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ports := getTLSPorts(test.ann)
			if len(ports) != test.portLen {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.portLen)
				t.Logf("actual: %v", len(ports))
			}
		})
	}
}

func Test_getTLSAnnotations(t *testing.T) {
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
		annTLS []*tlsAnnotation
		err    error
	}{
		{
			name: "Test single TLS annotation",
			ann:  map[string]string{annLinodeLoadBalancerTLS: `[ { "tls-secret-name": "prod-app-tls", "port": 443} ]`},
			annTLS: []*tlsAnnotation{
				{
					TLSSecretName: "prod-app-tls",
					Port:          443,
				},
			},
			err: nil,
		},
		{
			name: "Test multiple TLS annotation",
			ann:  map[string]string{annLinodeLoadBalancerTLS: `[ { "tls-secret-name": "prod-app-tls", "port": 443}, {"tls-secret-name": "dev-app-tls", "port": 8443} ]`},
			annTLS: []*tlsAnnotation{
				{
					TLSSecretName: "prod-app-tls",
					Port:          443,
				},
				{
					TLSSecretName: "dev-app-tls",
					Port:          8443,
				},
			},
			err: nil,
		},
		{
			name:   "Test without TLS annotation",
			ann:    nil,
			annTLS: nil,
			err:    fmt.Errorf("annotation %v must be specified", annLinodeLoadBalancerTLS),
		},
		{
			name:   "Test invalid json",
			ann:    map[string]string{annLinodeLoadBalancerTLS: `[ { "tls-secret-name": "prod-app-tls", "port": 443}`},
			annTLS: nil,
			err:    json.Unmarshal([]byte(`[ { "tls-secret-name": "prod-app-tls", "port": 443}`), &tlsAnnotation{}),
		},
	}
	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			svc.Annotations = test.ann
			ann, err := getTLSAnnotations(svc)
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

func Test_getTLSCertInfo(t *testing.T) {
	cert := `-----BEGIN CERTIFICATE-----
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
	key := `-----BEGIN RSA PRIVATE KEY-----
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
-----END RSA PRIVATE KEY-----`
	kubeClient := fake.NewSimpleClientset()
	_, err := kubeClient.CoreV1().Secrets("test").Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tls-secret",
		},
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte(cert),
			v1.TLSPrivateKeyKey: []byte(key),
		},
		StringData: nil,
		Type:       "kubernetes.io/tls",
	})
	if err != nil {
		t.Error(err)
	}

	testcases := []struct {
		name      string
		annTLS    []*tlsAnnotation
		namespace string
		port      int
		cert      string
		key       string
		err       error
	}{
		{
			name: "Test valid Cert info",
			annTLS: []*tlsAnnotation{
				{
					TLSSecretName: "tls-secret",
					Port:          8080,
				},
			},
			namespace: "test",
			port:      8080,
			cert:      cert,
			key:       key,
			err:       nil,
		},
		{
			name: "Test invalid Cert info",
			annTLS: []*tlsAnnotation{
				{
					TLSSecretName: "tls-secret",
					Port:          8080,
				},
			},
			namespace: "test",
			port:      8081,
			cert:      "",
			key:       "",
			err:       fmt.Errorf("cert & key for port %v is not specified in annotation %v", 8081, annLinodeLoadBalancerTLS),
		},
		{
			name: "Test no secret found",
			annTLS: []*tlsAnnotation{
				{
					TLSSecretName: "secret",
					Port:          8080,
				},
			},
			namespace: "test",
			port:      8080,
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
			cert, key, err := getTLSCertInfo(kubeClient, test.annTLS, test.namespace, test.port)
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

func Test_isTLSPort(t *testing.T) {
	testcases := []struct {
		name     string
		tlsPorts []int
		port     int
		ok       bool
	}{
		{
			name:     "Test TLS port",
			tlsPorts: []int{8080, 443, 7443},
			port:     443,
			ok:       true,
		},
		{
			name:     "Test not TLS port",
			tlsPorts: []int{8080, 443, 7443},
			port:     80,
			ok:       false,
		},
	}
	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ok := isTLSPort(test.tlsPorts, test.port)
			if ok != test.ok {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.ok)
				t.Logf("actual: %v", ok)
			}
		})
	}
}

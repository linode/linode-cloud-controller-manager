package linode

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/linode/linodego"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/cloudprovider"
)

var _ cloudprovider.LoadBalancer = new(loadbalancers)

func TestCCMLoadBalancers(t *testing.T) {
	fake := newFake(t)
	ts := httptest.NewServer(fake)
	defer ts.Close()

	linodeClient := linodego.NewClient(http.DefaultClient)
	linodeClient.SetBaseURL(ts.URL)

	testCases := struct {
		f []func(t *testing.T, client *linodego.Client)
	}{
		[]func(t *testing.T, client *linodego.Client){
			testGetLoadBalancer,
			testCreateNodeBalancer,
			testBuildLoadBalancerRequest,
			testEnsureLoadBalancerDeleted,
			testEnsureLoadBalancer,
			testGetLoadBalancer,
		},
	}
	for _, tf := range testCases.f {
		t.Run("Running", func(t *testing.T) {
			tf(t, &linodeClient)
		})
	}

	//x:= func(f func(t *testing.T, client *linodego.Client)){}

	//t.Run(test.name, func(t *testing.T) {

}

func testCreateNodeBalancer(t *testing.T, client *linodego.Client) {
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
	lb := &loadbalancers{client, "us-west"}
	id, err := lb.createNodeBalancer(context.TODO(), svc)
	if id == -1 {
		t.Error("unexpected nodeID")
		t.Logf("expected: >%v", 0)
		t.Logf("actual: %v", id)
	}
	if !reflect.DeepEqual(err, nil) {
		t.Error("unexpected error")
		t.Logf("expected: %v", nil)
		t.Logf("actual: %v", err)
	}
	defer func() { _ = lb.EnsureLoadBalancerDeleted(context.TODO(), "lnodelb", svc) }()
}

func Test_getAlgorithm(t *testing.T) {
	testcases := []struct {
		name      string
		service   *v1.Service
		algorithm linodego.ConfigAlgorithm
	}{
		{
			"algorithm should be least_connection",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodeAlgorithm: "least_connections",
					},
				},
			},
			linodego.AlgorithmLeastConn,
		},
		{
			"algorithm should be source",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodeAlgorithm: "source",
					},
				},
			},
			linodego.AlgorithmSource,
		},
		{
			"algorithm should be round_robin",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodeAlgorithm: "roundrobin",
					},
				},
			},
			linodego.AlgorithmRoundRobin,
		},
		{
			"invalid algorithm should default to round_robin",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodeAlgorithm: "invalid",
					},
				},
			},
			linodego.AlgorithmRoundRobin,
		},
		{
			"no algorithm specified should default to round_robin",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
				},
			},
			linodego.AlgorithmRoundRobin,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			algorithm := getAlgorithm(test.service)
			if algorithm != test.algorithm {
				t.Error("unexpected algoritmh")
				t.Logf("expected: %q", test.algorithm)
				t.Logf("actual: %q", algorithm)
			}
		})
	}
}

func Test_getCertificate(t *testing.T) {
	cert := `-----BEGIN CERTIFICATE-----
MIICuDCCAaCgAwIBAgIBADANBgkqhkiG9w0BAQsFADANMQswCQYDVQQDEwJjYTAe
Fw0xNzExMjcwNTQ3NDJaFw0yNzExMjUwNTQ3NDJaMA0xCzAJBgNVBAMTAmNhMIIB
IjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA7GdQywI4pm50c0TyiOoKi4ar
AwSSgHdDSQFNM4k2ssXuem8S1DMRScY663LYn14n1PM6fppCtZWC/vtsDnmEEGUy
/w+hJ8w90uFExMBmkn8D765W59jWtE3x3/7Kd0PGyiXGsdqRxmhainOO6p9Q8/Ln
SwPpsVMRnbSDAnoNqRFK59YIfxoQXML2+e45M+oFbxUoi2xXQCsj1qdxTshtqwT/
7u0nWOOSoq8a3YKv7zk+qZwCNe0PSKXKbnNNJgzdx+UJWBChvrt0Ndm+swTG125B
lMlBrmNJOYWdNGLKuFsWX+OPC7fNj9VwxarOy+H5ykLH0i+7jxCpgYGF+eFDvwID
AQABoyMwITAOBgNVHQ8BAf8EBAMCAqQwDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG
9w0BAQsFAAOCAQEAJwH7LC0d1z7r/ztQ2AekAsqwu+So/EHqVzGRP4jHRJMPtYN1
SFBCIdLYmACuj0MfWLyPy2WkpT4F3h9CrQzswl42mdjd0Q5vutjGsDo6Jta9Jf4Y
ouM2felPMvbAXHIAFIXXa64okcnWoJzp1oAxfCieqZXb2yhPJMcULtNUC5NtYEpG
oNF1FzyoGh5GNpeARDnzU7RACF9PiCxx8hWHV9V09IXXP5TjBDdc4rvll7P93W7V
3WV87/Aeh/W8TueGYBeUOmzn63VbEkpmGT9KJe8t+IrVymuG4rYS08z6g5Ib9FNh
KHB9fdnWTibkrKB/319X4GfMjGNN2/YyER2F8g==
-----END CERTIFICATE-----`
	key := `-----BEGIN RSA PRIVATE KEY-----
MIIEpQIBAAKCAQEA7GdQywI4pm50c0TyiOoKi4arAwSSgHdDSQFNM4k2ssXuem8S
1DMRScY663LYn14n1PM6fppCtZWC/vtsDnmEEGUy/w+hJ8w90uFExMBmkn8D765W
59jWtE3x3/7Kd0PGyiXGsdqRxmhainOO6p9Q8/LnSwPpsVMRnbSDAnoNqRFK59YI
fxoQXML2+e45M+oFbxUoi2xXQCsj1qdxTshtqwT/7u0nWOOSoq8a3YKv7zk+qZwC
Ne0PSKXKbnNNJgzdx+UJWBChvrt0Ndm+swTG125BlMlBrmNJOYWdNGLKuFsWX+OP
C7fNj9VwxarOy+H5ykLH0i+7jxCpgYGF+eFDvwIDAQABAoIBAQDj8sdDyPOI/66H
y261uD7MxOC2+zysZNNbXMbtL5yviw1lvx5/wHImGd+MUmQwX2C3BIVduC8k2nLC
nPpXhrJiAMLIkHCLaHQgmBhwQzlkftbz0L55tmto1lOo8gyWLaNMHlrV+fRgRRUw
tTaUY2RypcCCY9Z9pqSw1XMR+1CauHhicfY9K1rQgF8xtZ6sB+P7y2SwVlp2OjBr
R7E66O4s3LPf6A30ZbnaertZrrO36//sXKMKLeUlginzE3oMZBfr1IMYtd+5JKVX
axyMMNAqUjdpJk/ahE0B52Toebj9XSxTNkiswmNS6Zve9CV5oiRkntsDZXpiDnRb
7lEHXnjhAoGBAPtYQ+Y+sg4utk4BOIK2apjUVLwXuDQiCREzCnhA3CLCSqJMb6Y8
7N1+KzRZYeDNECt5DOJOrUqM2pTIQ+RkZEhaUfJr1ILFGQmD7FhjxrM8nQh5gUKO
9fGEKPPIOshkUoVCNm5HMixa7YnGM1xhvXvHLPSXILwuz082e2ZnI5SRAoGBAPDI
NSWEJ3d81YnIK6aDoPmpDv0FG+TweYqIdEs8eja8TN7Bpbx2vuUS/vkWsjJeyTkS
7V0Bq6bKVwfiFCYjEPNQ8qekifb+tHRLu6DRbj4UbeAcZXr3C5mcUQk07/84gXXj
FUDfT8EI6Eerr6RM75CTN7nesiwGXMjyYSSomTtPAoGBAOs8s+fVO95sN7GQEOy9
f8zjxR55cKxSQnw3chAUXDOn9iQqN8C1etbeU99d3G6CXiTh2X4hNqz0YUsol+o1
T2osJlAmPbHaeFFgiB492+U60Jny5lh95o+RKqbm+qU8x8LysnDJ75p1y6XLu5w1
2hrz0g5lN30IrnwruJih5ToRAoGBAISK8RaRxNf1k+aglca3tqk38tQ9N7my1nT3
4Gx6AhyXUwlcN8ui4jpfVpPvdnBb1RDh5l/IR6EsyPPB8616qB4IdUrrPDcGxncu
KT7BipoJzOINP5+M1oncjo8u4N3xUPJ/6ncndlOgf5zUWX9sCoPfRlG+0P2DExha
tDblyFPpAoGAC29vNqFODcEhmiXfOEW2pvMXztgBVKopYGnKl2qDv9uBtMkn7qUi
V/teWB7SHT+secFM8H2lukmIO6PQhqH3sK7mAGxGXWWBUMVKeU8assuJmqXQsvMs
b8QPmGZdja1VyGqpAMkPmQOu9N5RbhKw1UOU/XGa31p6v96oayL+u8Q=
-----END RSA PRIVATE KEY-----`
	testcases := []struct {
		name    string
		service *v1.Service
		cert    string
		key     string
	}{
		{
			"certificate set",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodeSSLCertificate: base64.StdEncoding.EncodeToString([]byte(cert)),
						annLinodeSSLKey:         base64.StdEncoding.EncodeToString([]byte(key)),
					},
				},
			},
			cert,
			key,
		},
		{
			"certificate not set",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        randString(10),
					UID:         "abc123",
					Annotations: map[string]string{},
				},
			},
			"",
			"",
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			c, k := getSSLCertInfo(test.service)
			if c != test.cert {
				t.Error("unexpected certificate")
				t.Logf("expected: %q", test.cert)
				t.Logf("actual: %q", c)
			}
			if k != test.key {
				t.Error("unexpected key")
				t.Logf("expected: %q", test.key)
				t.Logf("actual: %q", k)
			}
		})
	}
}

func Test_getTLSPorts(t *testing.T) {
	testcases := []struct {
		name     string
		service  *v1.Service
		tlsPorts []int
		err      error
	}{
		{
			"tls port specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(10),
					UID:  "abc123",
					Annotations: map[string]string{
						annLinodeTLSPorts: "443",
					},
				},
			},
			[]int{443},
			nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			tlsPorts, err := getTLSPorts(test.service)
			if !reflect.DeepEqual(tlsPorts, test.tlsPorts) {
				t.Error("unexpected TLS ports")
				t.Logf("expected %v", test.tlsPorts)
				t.Logf("actual: %v", tlsPorts)
			}

			if !reflect.DeepEqual(err, test.err) {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.err)
				t.Logf("actual: %v", err)
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
			fmt.Errorf("invalid protocol: %q specifed in annotation: %q", "invalid", annLinodeProtocol),
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
			fmt.Errorf("invalid health check type: %q specifed in annotation: %q", "invalid", annLinodeHealthCheckType),
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

func Test_getNodeInternalIp(t *testing.T) {
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
							v1.NodeInternalIP,
							"127.0.0.1",
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
							v1.NodeExternalIP,
							"127.0.0.1",
						},
					},
				},
			},
			"",
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ip := getNodeInternalIp(test.node)
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

	lb := &loadbalancers{client, "us-west"}
	id, err := lb.buildLoadBalancerRequest(context.TODO(), svc, nodes)
	if id == "" {
		t.Error("unexpected nodeID")
		t.Logf("expected: != \"\"")
		t.Logf("actual: %v", id)
	}
	if !reflect.DeepEqual(err, err) {
		t.Error("unexpected error")
		t.Logf("expected: %v", nil)
		t.Logf("actual: %v", err)
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

	lb := &loadbalancers{client, "us-west"}
	_, err := lb.createNodeBalancer(context.TODO(), svc)
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

	lb := &loadbalancers{client, "us-west"}

	_, err := lb.createNodeBalancer(context.TODO(), svc)
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
								v1.NodeInternalIP,
								"127.0.0.1",
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
								v1.NodeInternalIP,
								"127.0.0.2",
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
								v1.NodeInternalIP,
								"127.0.0.3",
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
	lb := &loadbalancers{client, "us-west"}
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
	_, err := lb.createNodeBalancer(context.TODO(), svc)
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

package linode

import (
	"context"
	cryptoRand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	ciliumclient "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/typed/cilium.io/v2alpha1"
	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/linode/linode-cloud-controller-manager/cloud/annotations"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/options"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/services"
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
MIIJKAIBAAKCAgEA1QLQpK2vzg8uczV1Ni4S2Tgc5Ny59vqkwfK20m/mhjEIAlo3
kAj1Bc+omlQUjoaVLWgOmNF71FCCFeyj8iKEP16gQ/XOQdwcnJvpNdOGh9q3FfmM
tQz1OCZMitgf7UrIqtoE7qIzPZRCRsmrTgFlYj3AAOp9+fULDpG54N7hB2pem/8z
138962whAuOM0AUejzuBgiTDzfwMzJM2KQjB6Dy1FJLqGK8FUssXRYHunhkBGJzy
35WoQXPTGX0AA0Tc5gxyxkJhpwfEwGdf86keAQzFWRkZn/mKRvock2cr86Ke0GVO
qYYkrvt/BJhd47/lmfoelqR+w1/plZJexZFpq1G/AvtEPjlGpJSnHQYAuHFdOVBa
/gXRwSfkwlJlf25MlsVO5NioLk2sH8zfeFxRbLk38vx/PwY7EtGasC2Fn/91bgYE
BI4d3Awk4/PQdvsPA0/m1KLvijuT68I24AdbxzwoKegQg5LlnpU1zbnYpVZnRjuD
Pv/DUmT5x6k2alNFbNABZzVFFiBG8B8rBcIVmkTnLpOK70ti0fR4qnnkwF1YnjiU
G7aJdz+vNxY2jSMN4eQF69LSmsHEiAffvtZWr8ag9FaDe5ULqcZzMcQarWPW7qrc
kpT3slJXOdcgmyxksJDg1jG25m2apx8RQOJy2rVbYuG6k6fxj9dC0jyB1vUCAwEA
AQKCAgAJEXOcbyB63z6U/QOeaNu4j6D7RUJNd2IoN5L85nKj59Z1cy3GXftAYhTF
bSrq3mPfaPymGNTytvKyyD46gqmqoPalrgM33o0BRcnp1rV1dyQwNU1+L60I1OiR
SJ4jVfmw/FMVbaZMytD/fnpiecC9K+/Omiz+xSXRWvbU0eg2jpq0fWrRk8MpEJNf
Mhy+hllEs73Rsor7a+2HkATQPmUy49K5q393yYuqeKbm+J8V7+6SA6x7RD3De5DT
FvU3LmlRCdqhAhZyK+x+XGhDUUHLvaVxI5Zprw/p8Z/hzpSabKPiL03n/aP2JxLD
OVFV7sdxhKpks2AKJT0mdvK96nDbHFSn6cWvcwI9vprtfp3L+hk1OcYCpnjgphZf
Br6jTxIGOVVgzWGJQv89h17j1zYTY/VX0RZD+wSfewvjzm1lBdUWIZKvi5nhsoqd
4qjIeJnpBOVE0G4rY7hWlzPYk/JAPaXnD1Vj1u37CgodRGGWQjqtcoEPPQNI8HTU
wPPPJBrW9bSCywjupBPOZz+1gmwRKbyQgBGLQPJqn1BB3LsNpPervUa9udoTrelA
+c36EBlo9eAt5h2U11Q9yuLsyoUFWkndRWdHpJKPwt5tVOVQd8nnVZFGHvZhCt7M
XGy1jKL3CWpQavAtuSoX7YChQnQYM7TWTI/RtMdD62m8bbhgCQKCAQEA+YI8UvFm
6AZ4om8c3IwfGBeDpug4d2Dl1Uvmp5Zzaexp6UMKE8OgxFeyw5THjtjco6+IfDbm
lyxvUoDMxIWdBl8IuYpNZw5b8eW2SACTda7Sc8DeAuGg2VQcVYXUFzsUJiKhZLwc
CVfVVDoaMOC5T9M9cr/0dQ/AGk+dkdhx/IDRMSISNfZPwxEQvh43tciqpnme+eIg
CVqa+vfyUU4OC2kNpJj9m2bePkncRKUog+3exv+D4CPECXXF1a5qwFToXv6JiK3q
AlDPoVHz/MtZBw6PYiJau9gOV54bT+xdWSII4MO62bsvDM0GUppIMVpc3CgmDRcm
gnC/BIwcAvIBPwKCAQEA2o1/yEqniln6UfNbl8/AFFisZW9t+gXEHI0C1iYG588U
4NqpJqyFx62QlOgIgyfyE6Fk9M42LsW9CPoP+X9rdmqhnSVhbQgKbqI8ayeBCABu
oTbfh72MuFd0cco1P1Q/2XMGeQMAMMASSjyLe9xWHOGBnE5q1VfRz4yCA37+Zxo1
55eIbCfmYtu5S5GZLzTvFhpodDgC9qOBgWenXkYZor6AhopZU33Yr3a1Anp3VTfF
hMneGl6OVRyOhorphCG4yYS6hAL71ylLyqQRP0SPiSic/ipfdxT/Egs4Sov2f7cI
Lj8Sa5B7+vh4R4zsTAoeErpNZuMUo3y24rX+BzSmywKCAQB+BS6Mwgq01FfnyvEr
38XwuCexjIbAnPtYoQ5txMqkTFkuDMMxOlSf9p9+s02bs6K1NfpcqqoK3tGXPSCv
fcDSr/tLIzR3AcSkx94qPcg830DCYD6B/A3u1tG8zGxUE23Y2RLlOzF58pf4A6So
3UgbrljR9Wv2GC9x2pZ+THE+FJ4UD95czPx6TMtFCyQeN60hijomgfSmZNH0Qnls
YV0snDHc2bz12Z4Und+X+EcfY2xq3DFyav4fvRFgHMkkPX5kRHGYzCZuZvyHwUnX
e6mKq+r1qN5lE/oifOPUmVCIrW0IgTOFt0pLT96KqAwgiUBvngOiBvhXV7TTCiU3
w52nAoIBABie7jFLL7qnTkrjJoNgtRvVrX4z4mjTM3ef7xze5dJBgvGd0IZ50wxe
ojYUOblEy8GoYe4uOO5l+ljDiv8pepq5goFoj6QvzrUN886Cgce7/LqOqvnowayW
tZiIFh2PSS4fBjClxOS5DpZsYa5PcSgJw4cvUlu8a/d8tbzdFp3Y1w/DA2xjxlGG
vUYlHeOyi+iqiu/ky3irjNBeM/2r2gF6gpIljdCZEcsajWO9Fip0gPznnOzNkC1I
bUn85jercNzK5hQvHd3sWgx3FTZSa/UgrSb48Q5CQEXxG6NSRy+2F+bV1iZl/YGV
cj9lQc2DKkYj1MptdIrCZvv9UqPPK6cCggEBAO3uGtkCjbhiy2hZsfIybRBVk+Oz
/ViSe9xRTMO5UQYn7TXGUk5GwMIoBUSwujiLBPwPoAAlh26rZtnOfblLS74siBZu
sagVhoN02tqN5sM/AhUEVieGNb/WQjgeyd2bL8yIs9vyjH4IYZkljizp5+VLbEcR
o/aoxqmE0mN1lyCPOa9UP//LlsREkWVKI3+Wld/xERtzf66hjcH+ilsXDxxpMEXo
+jczfFY/ivf7HxfhyYAMMUT50XaQuN82ZcSdZt8fNwWL86sLtKQ3wugk9qsQG+6/
bSiPJQsGIKtQvyCaZY2szyOoeUGgOId+He7ITlezxKrjdj+1pLMESvAxKeo=
-----END RSA PRIVATE KEY-----`

const (
	drop          string = "DROP"
	defaultSubnet string = "default"
)

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
			name: "Create Load Balancer With node having no addresses",
			f:    testCreateNodeBalancerWithNodeNoAddresses,
		},
		{
			name: "Create Load Balancer Without Firewall",
			f:    testCreateNodeBalancerWithOutFirewall,
		},
		{
			name: "Create Load Balancer With Valid Firewall ID",
			f:    testCreateNodeBalancerWithFirewall,
		},
		{
			name: "Create Load Balancer With Invalid Firewall ID",
			f:    testCreateNodeBalancerWithInvalidFirewall,
		},
		{
			name: "Create Load Balancer With Valid Firewall ACL - AllowList",
			f:    testCreateNodeBalancerWithAllowList,
		},
		{
			name: "Create Load Balancer With Valid Firewall ACL - DenyList",
			f:    testCreateNodeBalancerWithDenyList,
		},
		{
			name: "Create Load Balancer With Invalid Firewall ACL - Both Allow and Deny",
			f:    testCreateNodeBalanceWithBothAllowOrDenyList,
		},
		{
			name: "Create Load Balancer With Invalid Firewall ACL - NO Allow Or Deny",
			f:    testCreateNodeBalanceWithNoAllowOrDenyList,
		},
		{
			name: "Create Load Balancer With VPC Backend",
			f:    testCreateNodeBalancerWithVPCBackend,
		},
		{
			name: "Update Load Balancer With VPC Backend",
			f:    testUpdateNodeBalancerWithVPCBackend,
		},
		{
			name: "Create Load Balancer With VPC Backend - Just specify Subnet flag",
			f:    testCreateNodeBalancerWithVPCOnlySubnetFlag,
		},
		{
			name: "Create Load Balancer With VPC Backend - Just specify Subnet id",
			f:    testCreateNodeBalancerWithVPCOnlySubnetIDFlag,
		},
		{
			name: "Create Load Balancer With VPC Backend - No specific flags or annotations",
			f:    testCreateNodeBalancerWithVPCNoFlagOrAnnotation,
		},
		{
			name: "Create Load Balancer With VPC Backend - annotations for vpc and subnet",
			f:    testCreateNodeBalancerWithVPCAnnotationOnly,
		},
		{
			name: "Create Load Balancer With VPC Backend - Specify subnet ID",
			f:    testCreateNodeBalancerWithVPCAnnotationOverwrite,
		},
		{
			name: "Create Load Balancer With Global Tags set",
			f:    testCreateNodeBalancerWithGlobalTags,
		},
		{
			name: "Create Load Balancer With Reserved IP",
			f:    testCreateNodeBalancerWithReservedIP,
		},
		{
			name: "Update Load Balancer - Add Node",
			f:    testUpdateLoadBalancerAddNode,
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
			name: "Update Load Balancer - Add Tags",
			f:    testUpdateLoadBalancerAddTags,
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
			name: "Update Load Balancer - Add new Firewall ID",
			f:    testUpdateLoadBalancerAddNewFirewall,
		},
		{
			name: "Update Load Balancer - Update Firewall ID",
			f:    testUpdateLoadBalancerUpdateFirewall,
		},
		{
			name: "Update Load Balancer - Delete Firewall ID",
			f:    testUpdateLoadBalancerDeleteFirewallRemoveID,
		},
		{
			name: "Update Load Balancer - Delete Firewall ACL",
			f:    testUpdateLoadBalancerDeleteFirewallRemoveACL,
		},
		{
			name: "Update Load Balancer - Update Firewall ACL",
			f:    testUpdateLoadBalancerUpdateFirewallACL,
		},
		{
			name: "Update Load Balancer - Remove Firewall ID & Add ACL",
			f:    testUpdateLoadBalancerUpdateFirewallRemoveIDaddACL,
		},
		{
			name: "Update Load Balancer - Remove Firewall ACL & Add ID",
			f:    testUpdateLoadBalancerUpdateFirewallRemoveACLaddID,
		},
		{
			name: "Update Load Balancer - Add a new Firewall ACL",
			f:    testUpdateLoadBalancerAddNewFirewallACL,
		},
		{
			name: "Update Load Balancer - Add Reserved IP",
			f:    testUpdateLoadBalancerAddReservedIP,
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
		{
			name: "makeLoadBalancerStatus",
			f:    testMakeLoadBalancerStatus,
		},
		{
			name: "makeLoadBalancerStatusWithIPv6",
			f:    testMakeLoadBalancerStatusWithIPv6,
		},
		{
			name: "makeLoadBalancerStatusEnvVar",
			f:    testMakeLoadBalancerStatusEnvVar,
		},
		{
			name: "Cleanup does not call the API unless Service annotated",
			f:    testCleanupDoesntCall,
		},
		{
			name: "Update Load Balancer - No Nodes",
			f:    testUpdateLoadBalancerNoNodes,
		},
		{
			name: "Update Load Balancer - Node excluded by annotation",
			f:    testUpdateLoadBalancerNodeExcludedByAnnotation,
		},
		{
			name: "Create Load Balancer - Very long Service name",
			f:    testVeryLongServiceName,
		},
		{
			name: "getNodeBalancerByStatus with IPv4 and IPv6 addresses",
			f:    testGetNodeBalancerByStatus,
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
	_, _ = fake.CoreV1().Services("").Create(context.TODO(), service, metav1.CreateOptions{})
}

func stubServiceUpdate(fake *fake.Clientset, service *v1.Service) {
	_, _ = fake.CoreV1().Services("").Update(context.TODO(), service, metav1.UpdateOptions{})
}

func testCreateNodeBalancer(t *testing.T, client *linodego.Client, _ *fakeAPI, annMap map[string]string, expectedTags []string) error {
	t.Helper()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: randString(),
			UID:  "foobar123",
			Annotations: map[string]string{
				annotations.AnnLinodeThrottle:         "15",
				annotations.AnnLinodeLoadBalancerTags: "fake,test,yolo",
			},
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
	for key, value := range annMap {
		svc.Annotations[key] = value
	}
	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	nodes := []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
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
	nb, err := lb.buildLoadBalancerRequest(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		return err
	}

	if nb.Region != lb.zone {
		t.Error("unexpected nodebalancer region")
		t.Logf("expected: %s", lb.zone)
		t.Logf("actual: %s", nb.Region)
	}

	configs, err := client.ListNodeBalancerConfigs(t.Context(), nb.ID, nil)
	if err != nil {
		return err
	}

	if len(configs) != len(svc.Spec.Ports) {
		t.Error("unexpected nodebalancer config count")
		t.Logf("expected: %v", len(svc.Spec.Ports))
		t.Logf("actual: %v", len(configs))
	}

	nb, err = client.GetNodeBalancer(t.Context(), nb.ID)
	if err != nil {
		return err
	}

	if nb.ClientConnThrottle != 15 {
		t.Error("unexpected ClientConnThrottle")
		t.Logf("expected: %v", 15)
		t.Logf("actual: %v", nb.ClientConnThrottle)
	}

	if len(expectedTags) == 0 {
		expectedTags = []string{"linodelb", "fake", "test", "yolo"}
	}
	if !reflect.DeepEqual(nb.Tags, expectedTags) {
		t.Error("unexpected Tags")
		t.Logf("expected: %v", expectedTags)
		t.Logf("actual: %v", nb.Tags)
	}

	_, ok := annMap[annotations.AnnLinodeCloudFirewallACL]
	if ok {
		// a firewall was configured for this
		firewalls, err := client.ListNodeBalancerFirewalls(t.Context(), nb.ID, &linodego.ListOptions{})
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		if len(firewalls) == 0 {
			t.Errorf("Expected 1 firewall, got %d", len(firewalls))
		}
	}

	defer func() { _ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc) }()
	return nil
}

func testCreateNodeBalancerWithReservedIP(t *testing.T, client *linodego.Client, f *fakeAPI) {
	t.Helper()

	annMap := map[string]string{
		annotations.AnnLinodeLoadBalancerIPv4: "156.1.1.101",
	}
	err := testCreateNodeBalancer(t, client, f, annMap, nil)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
}

func testCreateNodeBalancerWithOutFirewall(t *testing.T, client *linodego.Client, f *fakeAPI) {
	t.Helper()

	err := testCreateNodeBalancer(t, client, f, nil, nil)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
}

func testCreateNodeBalancerWithNodeNoAddresses(t *testing.T, client *linodego.Client, f *fakeAPI) {
	t.Helper()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: randString(),
			UID:  "foobar123",
			Annotations: map[string]string{
				annotations.NodeBalancerBackendIPv4Range: "10.100.0.0/30",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
					Protocol: "TCP",
					Port:     int32(80),
					NodePort: int32(30000),
				},
			},
		},
	}

	nodes := []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node-no-addrs"},
		},
	}

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	stubService(fakeClientset, svc)
	_, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err == nil {
		t.Errorf("EnsureLoadBalancer should have returned an error for node with no addresses")
	}
}

func testCreateNodeBalanceWithNoAllowOrDenyList(t *testing.T, client *linodego.Client, f *fakeAPI) {
	t.Helper()

	annotations := map[string]string{
		annotations.AnnLinodeCloudFirewallACL: `{}`,
	}

	err := testCreateNodeBalancer(t, client, f, annotations, nil)
	if err == nil || !stderrors.Is(err, services.ErrInvalidFWConfig) {
		t.Fatalf("expected a %v error, got %v", services.ErrInvalidFWConfig, err)
	}
}

func testCreateNodeBalanceWithBothAllowOrDenyList(t *testing.T, client *linodego.Client, f *fakeAPI) {
	t.Helper()

	annotations := map[string]string{
		annotations.AnnLinodeCloudFirewallACL: `{
			"allowList": {
				"ipv4": ["2.2.2.2/32"],
				"ipv6": ["2001:db8::/128"]
			},
			"denyList": {
				"ipv4": ["2.2.2.2/32"],
				"ipv6": ["2001:db8::/128"]
			}
		}`,
	}

	err := testCreateNodeBalancer(t, client, f, annotations, nil)
	if err == nil || !stderrors.Is(err, services.ErrInvalidFWConfig) {
		t.Fatalf("expected a %v error, got %v", services.ErrInvalidFWConfig, err)
	}
}

func testCreateNodeBalancerWithAllowList(t *testing.T, client *linodego.Client, f *fakeAPI) {
	t.Helper()

	annotations := map[string]string{
		annotations.AnnLinodeCloudFirewallACL: `{
			"allowList": {
				"ipv4": ["2.2.2.2/32"],
				"ipv6": ["2001:db8::/128"]
			}
		}`,
	}

	err := testCreateNodeBalancer(t, client, f, annotations, nil)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
}

func testCreateNodeBalancerWithDenyList(t *testing.T, client *linodego.Client, f *fakeAPI) {
	t.Helper()

	annotations := map[string]string{
		annotations.AnnLinodeCloudFirewallACL: `{
			"denyList": {
				"ipv4": ["2.2.2.2/32"],
				"ipv6": ["2001:db8::/128"]
			}
		}`,
	}

	err := testCreateNodeBalancer(t, client, f, annotations, nil)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
}

func testCreateNodeBalancerWithFirewall(t *testing.T, client *linodego.Client, f *fakeAPI) {
	t.Helper()

	annotations := map[string]string{
		annotations.AnnLinodeCloudFirewallID: "123",
	}
	err := testCreateNodeBalancer(t, client, f, annotations, nil)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
}

func testCreateNodeBalancerWithInvalidFirewall(t *testing.T, client *linodego.Client, f *fakeAPI) {
	t.Helper()

	annotations := map[string]string{
		annotations.AnnLinodeCloudFirewallID: "qwerty",
	}
	expectedError := "strconv.Atoi: parsing \"qwerty\": invalid syntax"
	err := testCreateNodeBalancer(t, client, f, annotations, nil)
	if err.Error() != expectedError {
		t.Fatalf("expected a %s error, got %v", expectedError, err)
	}
}

func testCreateNodeBalancerWithGlobalTags(t *testing.T, client *linodego.Client, f *fakeAPI) {
	t.Helper()

	original := options.Options.NodeBalancerTags
	defer func() {
		options.Options.NodeBalancerTags = original
	}()
	options.Options.NodeBalancerTags = []string{"foobar"}
	expectedTags := []string{"linodelb", "foobar", "fake", "test", "yolo"}
	err := testCreateNodeBalancer(t, client, f, nil, expectedTags)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
}

func testCreateNodeBalancerWithVPCBackend(t *testing.T, client *linodego.Client, f *fakeAPI) {
	t.Helper()

	// provision vpc and test
	ann := map[string]string{
		annotations.NodeBalancerBackendIPv4Range: "10.100.0.0/30",
	}

	vpcNames := options.Options.VPCNames
	subnetNames := options.Options.SubnetNames
	defer func() {
		options.Options.VPCNames = vpcNames
		options.Options.SubnetNames = subnetNames
	}()
	options.Options.VPCNames = []string{"test1"}
	options.Options.SubnetNames = []string{defaultSubnet}
	_, _ = client.CreateVPC(t.Context(), linodego.VPCCreateOptions{
		Label:       "test1",
		Description: "",
		Region:      "us-west",
		Subnets: []linodego.VPCSubnetCreateOptions{
			{
				Label: defaultSubnet,
				IPv4:  "10.0.0.0/8",
			},
		},
	})

	err := testCreateNodeBalancer(t, client, f, ann, nil)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}

	f.ResetRequests()

	// test with IPv4Range outside of defined NodeBalancer subnet
	nodebalancerBackendIPv4Subnet := options.Options.NodeBalancerBackendIPv4Subnet
	defer func() {
		options.Options.NodeBalancerBackendIPv4Subnet = nodebalancerBackendIPv4Subnet
	}()
	options.Options.NodeBalancerBackendIPv4Subnet = "10.99.0.0/24"
	if err := testCreateNodeBalancer(t, client, f, ann, nil); err == nil {
		t.Fatalf("expected nodebalancer creation to fail")
	}
}

func testUpdateNodeBalancerWithVPCBackend(t *testing.T, client *linodego.Client, f *fakeAPI) {
	t.Helper()

	// provision vpc and test
	vpcNames := options.Options.VPCNames
	subnetNames := options.Options.SubnetNames
	defer func() {
		options.Options.VPCNames = vpcNames
		options.Options.SubnetNames = subnetNames
	}()
	options.Options.VPCNames = []string{"test1"}
	options.Options.SubnetNames = []string{defaultSubnet}
	_, _ = client.CreateVPC(t.Context(), linodego.VPCCreateOptions{
		Label:       "test1",
		Description: "",
		Region:      "us-west",
		Subnets: []linodego.VPCSubnetCreateOptions{
			{
				Label: defaultSubnet,
				IPv4:  "10.0.0.0/8",
			},
		},
	})

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: randString(),
			UID:  "foobar123",
			Annotations: map[string]string{
				annotations.NodeBalancerBackendIPv4Range: "10.100.0.0/30",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	stubService(fakeClientset, svc)
	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus

	stubServiceUpdate(fakeClientset, svc)
	svc.SetAnnotations(map[string]string{
		annotations.NodeBalancerBackendIPv4Range: "10.100.1.0/30",
	})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error while updated annotations: %s", err)
	}
}

func testCreateNodeBalancerWithVPCOnlySubnetFlag(t *testing.T, client *linodego.Client, f *fakeAPI) {
	t.Helper()

	// provision vpc and test
	vpcNames := options.Options.VPCNames
	subnetNames := options.Options.SubnetNames
	nbBackendSubnet := options.Options.NodeBalancerBackendIPv4Subnet
	defer func() {
		options.Options.VPCNames = vpcNames
		options.Options.SubnetNames = subnetNames
		options.Options.NodeBalancerBackendIPv4Subnet = nbBackendSubnet
	}()
	options.Options.VPCNames = []string{"test-subflag"}
	options.Options.SubnetNames = []string{defaultSubnet}
	options.Options.NodeBalancerBackendIPv4Subnet = "10.254.0.0/24"
	_, _ = client.CreateVPC(t.Context(), linodego.VPCCreateOptions{
		Label:       "test-subflag",
		Description: "",
		Region:      "us-west",
		Subnets: []linodego.VPCSubnetCreateOptions{
			{
				Label: defaultSubnet,
				IPv4:  "10.0.0.0/8",
			},
		},
	})

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        randString(),
			UID:         "foobar123",
			Annotations: map[string]string{},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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
						Address: "10.0.0.2",
					},
				},
			},
		},
	}

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus

	stubService(fakeClientset, svc)
	svc.SetAnnotations(map[string]string{
		annotations.NodeBalancerBackendIPv4Range: "10.254.0.60/30",
	})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error with updated annotations: %s", err)
	}

	// test with IPv4Range outside of defined NodeBalancer subnet
	svc.SetAnnotations(map[string]string{
		annotations.NodeBalancerBackendIPv4Range: "10.100.0.0/30",
	})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err == nil {
		t.Errorf("UpdateLoadBalancer should have returned an error when ipv4_range is outside of specified subnet: %s", err)
	}
}

func testCreateNodeBalancerWithVPCNoFlagOrAnnotation(t *testing.T, client *linodego.Client, f *fakeAPI) {
	t.Helper()

	// provision vpc and test
	vpcNames := options.Options.VPCNames
	subnetNames := options.Options.SubnetNames
	defer func() {
		options.Options.VPCNames = vpcNames
		options.Options.SubnetNames = subnetNames
	}()
	options.Options.VPCNames = []string{"test-noflags"}
	options.Options.SubnetNames = []string{defaultSubnet}
	_, _ = client.CreateVPC(t.Context(), linodego.VPCCreateOptions{
		Label:       "test-noflags",
		Description: "",
		Region:      "us-west",
		Subnets: []linodego.VPCSubnetCreateOptions{
			{
				Label: defaultSubnet,
				IPv4:  "10.0.0.0/8",
			},
		},
	})

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        randString(),
			UID:         "foobar123",
			Annotations: map[string]string{},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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
						Address: "10.10.10.2",
					},
				},
			},
		},
	}

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus

	stubService(fakeClientset, svc)
	svc.SetAnnotations(map[string]string{
		annotations.NodeBalancerBackendIPv4Range: "10.254.0.60/30",
	})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error with updated annotations: %s", err)
	}
}

func testCreateNodeBalancerWithVPCAnnotationOnly(t *testing.T, client *linodego.Client, f *fakeAPI) {
	t.Helper()

	// provision vpc and test
	vpcNames := options.Options.VPCNames
	subnetNames := options.Options.SubnetNames
	defer func() {
		options.Options.VPCNames = vpcNames
		options.Options.SubnetNames = subnetNames
	}()
	options.Options.VPCNames = []string{"test-onlyannotation"}
	options.Options.SubnetNames = []string{defaultSubnet}
	_, _ = client.CreateVPC(t.Context(), linodego.VPCCreateOptions{
		Label:       "test-onlyannotation",
		Description: "",
		Region:      "us-west",
		Subnets: []linodego.VPCSubnetCreateOptions{
			{
				Label: defaultSubnet,
				IPv4:  "10.0.0.0/8",
			},
			{
				Label: "custom",
				IPv4:  "192.168.0.0/24",
			},
		},
	})

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: randString(),
			UID:  "foobar123",
			Annotations: map[string]string{
				annotations.NodeBalancerBackendSubnetName: "custom",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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
						Address: "10.11.11.2",
					},
				},
			},
		},
	}

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	stubService(fakeClientset, svc)
	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus

	stubServiceUpdate(fakeClientset, svc)
	svc.SetAnnotations(map[string]string{})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error with updated annotations: %s", err)
	}
}

func testCreateNodeBalancerWithVPCOnlySubnetIDFlag(t *testing.T, client *linodego.Client, f *fakeAPI) {
	t.Helper()

	// provision vpc and test
	vpcNames := options.Options.VPCNames
	subnetNames := options.Options.SubnetNames
	nbBackendSubnetID := options.Options.NodeBalancerBackendIPv4SubnetID
	defer func() {
		options.Options.VPCNames = vpcNames
		options.Options.SubnetNames = subnetNames
		options.Options.NodeBalancerBackendIPv4SubnetID = nbBackendSubnetID
	}()
	options.Options.VPCNames = []string{"test1"}
	options.Options.SubnetNames = []string{defaultSubnet}
	options.Options.NodeBalancerBackendIPv4SubnetID = 1111
	_, _ = client.CreateVPC(t.Context(), linodego.VPCCreateOptions{
		Label:       "test-subid-flag",
		Description: "",
		Region:      "us-west",
		Subnets: []linodego.VPCSubnetCreateOptions{
			{
				Label: defaultSubnet,
				IPv4:  "10.0.0.0/8",
			},
		},
	})

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        randString(),
			UID:         "foobar123",
			Annotations: map[string]string{},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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
						Address: "10.0.0.3",
					},
				},
			},
		},
	}

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus

	stubService(fakeClientset, svc)

	// test with annotation specifying subnet id
	svc.SetAnnotations(map[string]string{
		annotations.NodeBalancerBackendSubnetID: "2222",
	})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error while updated annotations: %s", err)
	}
}

func testCreateNodeBalancerWithVPCAnnotationOverwrite(t *testing.T, client *linodego.Client, f *fakeAPI) {
	t.Helper()

	// provision multiple vpcs
	vpcNames := options.Options.VPCNames
	subnetNames := options.Options.SubnetNames
	nodebalancerBackendIPv4Subnet := options.Options.NodeBalancerBackendIPv4Subnet
	defer func() {
		options.Options.VPCNames = vpcNames
		options.Options.SubnetNames = subnetNames
		options.Options.NodeBalancerBackendIPv4Subnet = nodebalancerBackendIPv4Subnet
	}()
	options.Options.VPCNames = []string{"test1"}
	options.Options.SubnetNames = []string{defaultSubnet}
	options.Options.NodeBalancerBackendIPv4Subnet = "10.100.0.0/24"

	_, _ = client.CreateVPC(t.Context(), linodego.VPCCreateOptions{
		Label:       "test1",
		Description: "",
		Region:      "us-west",
		Subnets: []linodego.VPCSubnetCreateOptions{
			{
				Label: defaultSubnet,
				IPv4:  "10.0.0.0/8",
			},
		},
	})

	_, _ = client.CreateVPC(t.Context(), linodego.VPCCreateOptions{
		Label:       "test2",
		Description: "",
		Region:      "us-west",
		Subnets: []linodego.VPCSubnetCreateOptions{
			{
				Label: "subnet1",
				IPv4:  "10.0.0.0/8",
			},
		},
	})
	ann := map[string]string{
		annotations.NodeBalancerBackendIPv4Range:  "10.100.0.0/30",
		annotations.NodeBalancerBackendVPCName:    "test2",
		annotations.NodeBalancerBackendSubnetName: "subnet1",
	}
	err := testCreateNodeBalancer(t, client, f, ann, nil)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
}

func testUpdateLoadBalancerAddNode(t *testing.T, client *linodego.Client, f *fakeAPI) {
	t.Helper()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: randString(),
			UID:  "foobar1234",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
					Protocol: "TCP",
					Port:     int32(80),
					NodePort: int32(30000),
				},
			},
		},
	}

	nodes1 := []*v1.Node{
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

	nodes2 := []*v1.Node{
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
		{
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
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeInternalIP,
						Address: "127.0.0.3",
					},
				},
			},
		},
	}

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes1)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus

	stubService(fakeClientset, svc)

	f.ResetRequests()

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes1)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error while updated LB to have one node: %s", err)
	}

	rx := regexp.MustCompile("/nodebalancers/[0-9]+/configs/[0-9]+/rebuild")
	checkIDs := func() (int, int) {
		var req *fakeRequest
		for request := range f.requests {
			if rx.MatchString(request.Path) {
				req = &request
				break
			}
		}

		if req == nil {
			t.Fatalf("Nodebalancer config rebuild request was not called.")
			return 0, 0 // explicitly return to satisfy staticcheck
		}

		var nbcro linodego.NodeBalancerConfigRebuildOptions

		if err = json.Unmarshal([]byte(req.Body), &nbcro); err != nil {
			t.Fatalf("Unable to unmarshall request body %#v, error: %#v", req.Body, err)
		}

		withIds := 0
		for i := range nbcro.Nodes {
			if nbcro.Nodes[i].ID > 0 {
				withIds++
			}
		}

		return len(nbcro.Nodes), withIds
	}

	nodecount, nodeswithIdcount := checkIDs()
	if nodecount != 1 {
		t.Fatalf("Unexpected node count (%d) in request on updating the nodebalancer with one node.", nodecount)
	}
	if nodeswithIdcount != 1 {
		t.Fatalf("Expected Node ID to be set when updating the nodebalancer with the same node it had previously.")
	}

	f.ResetRequests()
	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes2)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error while updated LB to have three nodes: %s", err)
	}
	nodecount, nodeswithIdcount = checkIDs()
	if nodecount != 3 {
		t.Fatalf("Unexpected node count (%d) in request on updating the nodebalancer with three nodes.", nodecount)
	}
	if nodeswithIdcount != 1 {
		t.Fatalf("Expected ID to be set just on one node which was existing prior updating the node with three nodes, it is set on %d nodes", nodeswithIdcount)
	}

	f.ResetRequests()
	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes2)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error while updated LB to have three nodes second time: %s", err)
	}
	nodecount, nodeswithIdcount = checkIDs()
	if nodecount != 3 {
		t.Fatalf("Unexpected node count (%d) in request on updating the nodebalancer with three nodes second time.", nodecount)
	}
	if nodeswithIdcount != 3 {
		t.Fatalf("Expected ID to be set just on all three nodes when updating the NB with all three nodes which were pre-existing, instead it is set on %d nodes", nodeswithIdcount)
	}

	nb, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer via status: %s", err)
	}

	cfgs, errConfigs := client.ListNodeBalancerConfigs(t.Context(), nb.ID, nil)
	if errConfigs != nil {
		t.Fatalf("error getting NodeBalancer configs: %v", errConfigs)
	}

	expectedPorts := map[int]struct{}{
		80: {},
	}

	observedPorts := make(map[int]struct{})

	for _, cfg := range cfgs {
		nbnodes, errNodes := client.ListNodeBalancerNodes(t.Context(), nb.ID, cfg.ID, nil)
		if errNodes != nil {
			t.Errorf("error getting NodeBalancer nodes: %v", errNodes)
		}

		for _, node := range nbnodes {
			t.Logf("Node %#v", node)
		}

		if len(nbnodes) != len(nodes2) {
			t.Errorf("Expected %d nodes for port %d, got %d (%#v)", len(nodes2), cfg.Port, len(nbnodes), nbnodes)
		}

		observedPorts[cfg.Port] = struct{}{}
	}

	if !reflect.DeepEqual(expectedPorts, observedPorts) {
		t.Errorf("NodeBalancer ports mismatch: expected %#v, got %#v", expectedPorts, observedPorts)
	}
}

func testUpdateLoadBalancerAddAnnotation(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	t.Helper()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: randString(),
			UID:  "foobar123",
			Annotations: map[string]string{
				annotations.AnnLinodeThrottle: "15",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus

	stubService(fakeClientset, svc)
	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeThrottle: "10",
	})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error while updated annotations: %s", err)
	}

	nb, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer via status: %s", err)
	}

	if nb.ClientConnThrottle != 10 {
		t.Errorf("unexpected ClientConnThrottle: expected %d, got %d", 10, nb.ClientConnThrottle)
		t.Logf("expected: %v", 10)
		t.Logf("actual: %v", nb.ClientConnThrottle)
	}
}

func testUpdateLoadBalancerAddPortAnnotation(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	t.Helper()

	targetTestPort := 80
	portConfigAnnotation := fmt.Sprintf("%s%d", annotations.AnnLinodePortConfigPrefix, targetTestPort)
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        randString(),
			UID:         "foobar123",
			Annotations: map[string]string{},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	stubService(fakeClientset, svc)
	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus
	stubServiceUpdate(fakeClientset, svc)

	svc.SetAnnotations(map[string]string{
		portConfigAnnotation: `{"protocol": "http"}`,
	})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Fatalf("UpdateLoadBalancer returned an error while updated annotations: %s", err)
	}

	nb, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer by status: %v", err)
	}

	cfgs, errConfigs := client.ListNodeBalancerConfigs(t.Context(), nb.ID, nil)
	if errConfigs != nil {
		t.Fatalf("error getting NodeBalancer configs: %v", errConfigs)
	}

	expectedPortConfigs := map[int]string{
		80: "http",
	}
	observedPortConfigs := make(map[int]string)

	for _, cfg := range cfgs {
		observedPortConfigs[cfg.Port] = string(cfg.Protocol)
	}

	if !reflect.DeepEqual(expectedPortConfigs, observedPortConfigs) {
		t.Errorf("NodeBalancer port mismatch: expected %v, got %v", expectedPortConfigs, observedPortConfigs)
	}
}

func testVeryLongServiceName(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	t.Helper()

	ipv4DenyList := make([]string, 130)
	ipv6DenyList := make([]string, 130)

	for i := 0; i < len(ipv4DenyList); i++ {
		ipv4DenyList[i] = fmt.Sprintf("192.168.1.%d/32", i)
		ipv6DenyList[i] = fmt.Sprintf("2001:db8::%x/128", i)
	}

	var jsonV4DenyList, jsonV6DenyList []byte
	jsonV4DenyList, err := json.Marshal(ipv4DenyList)
	if err != nil {
		t.Error("Could not marshal ipv4DenyList into json")
	}
	jsonV6DenyList, err = json.Marshal(ipv6DenyList)
	if err != nil {
		t.Error("Could not marshal ipv6DenyList into json")
	}

	denyListJSON := fmt.Sprintf(`{
		"denyList": {
			"ipv4": %s,
			"ipv6": %s
		}
	}`, string(jsonV4DenyList), string(jsonV6DenyList))

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: strings.Repeat(randString(), 6),
			UID:  "foobar123",
			Annotations: map[string]string{
				annotations.AnnLinodeCloudFirewallACL: denyListJSON,
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	stubService(fakeClientset, svc)
	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus
	stubServiceUpdate(fakeClientset, svc)

	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeCloudFirewallACL: `{
			"denyList": {
				"ipv4": ["192.168.1.0/32"],
				"ipv6": ["2001:db8::/128"]
			}
		}`,
	})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Fatalf("UpdateLoadBalancer returned an error with updated annotations: %s", err)
	}
}

func testUpdateLoadBalancerAddTags(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	t.Helper()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        randString(),
			UID:         "foobar123",
			Annotations: map[string]string{},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset
	clusterName := "linodelb"

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), clusterName, svc)
	}()

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), clusterName, svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus
	stubService(fakeClientset, svc)

	testTags := "test,new,tags"
	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeLoadBalancerTags: testTags,
	})

	err = lb.UpdateLoadBalancer(t.Context(), clusterName, svc, nodes)
	if err != nil {
		t.Fatalf("UpdateLoadBalancer returned an error while updated annotations: %s", err)
	}

	nb, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer by status: %v", err)
	}

	expectedTags := append([]string{clusterName}, strings.Split(testTags, ",")...)
	observedTags := nb.Tags

	if !reflect.DeepEqual(expectedTags, observedTags) {
		t.Errorf("NodeBalancer tags mismatch: expected %v, got %v", expectedTags, observedTags)
	}
}

func testUpdateLoadBalancerAddTLSPort(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	t.Helper()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: randString(),
			UID:  "foobar123",
			Annotations: map[string]string{
				annotations.AnnLinodeThrottle: "15",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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

	extraPort := v1.ServicePort{
		Name:     randString(),
		Protocol: "TCP",
		Port:     int32(443),
		NodePort: int32(30001),
	}

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset
	addTLSSecret(t, lb.kubeClient)

	stubService(fakeClientset, svc)
	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus

	stubServiceUpdate(fakeClientset, svc)
	svc.Spec.Ports = append(svc.Spec.Ports, extraPort)
	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodePortConfigPrefix + "443": `{ "protocol": "https", "tls-secret-name": "tls-secret"}`,
	})
	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Fatalf("UpdateLoadBalancer returned an error while updated annotations: %s", err)
	}

	nb, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfgs, errConfigs := client.ListNodeBalancerConfigs(t.Context(), nb.ID, nil)
	if errConfigs != nil {
		t.Fatalf("error getting NodeBalancer configs: %v", errConfigs)
	}

	expectedPorts := map[int]struct{}{
		80:  {},
		443: {},
	}

	observedPorts := make(map[int]struct{})

	for _, cfg := range cfgs {
		nodes, errNodes := client.ListNodeBalancerNodes(t.Context(), nb.ID, cfg.ID, nil)
		if errNodes != nil {
			t.Errorf("error getting NodeBalancer nodes: %v", errNodes)
		}

		if len(nodes) == 0 {
			t.Errorf("no nodes found for port %d", cfg.Port)
		}

		observedPorts[cfg.Port] = struct{}{}
	}

	if !reflect.DeepEqual(expectedPorts, observedPorts) {
		t.Errorf("NodeBalancer ports mismatch: expected %v, got %v", expectedPorts, observedPorts)
	}
}

func testUpdateLoadBalancerAddProxyProtocol(t *testing.T, client *linodego.Client, fakeAPI *fakeAPI) {
	t.Helper()

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

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
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
					Name:        randString(),
					UID:         "foobar123",
					Annotations: map[string]string{},
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:     randString(),
							Protocol: "tcp",
							Port:     int32(80),
							NodePort: int32(8080),
						},
					},
				},
			}

			defer func() {
				_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
			}()
			nodeBalancer, err := client.CreateNodeBalancer(t.Context(), linodego.NodeBalancerCreateOptions{
				Region: lb.zone,
			})
			if err != nil {
				t.Fatalf("failed to create NodeBalancer: %s", err)
			}

			svc.Status.LoadBalancer = *makeLoadBalancerStatus(svc, nodeBalancer)
			svc.SetAnnotations(map[string]string{
				annotations.AnnLinodeDefaultProxyProtocol: string(tc.proxyProtocolConfig),
			})

			stubService(fakeClientset, svc)
			if err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes); err != nil {
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

			nodeBalancerConfigs, err := client.ListNodeBalancerConfigs(t.Context(), nodeBalancer.ID, nil)
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

func testUpdateLoadBalancerAddNewFirewall(t *testing.T, client *linodego.Client, fakeAPI *fakeAPI) {
	t.Helper()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: randString(),
			UID:  "foobar123",
			Annotations: map[string]string{
				annotations.AnnLinodeThrottle: "15",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	stubService(fakeClientset, svc)
	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus
	stubServiceUpdate(fakeClientset, svc)
	fwClient := services.LinodeClient{Client: client}
	fw, err := fwClient.CreateFirewall(t.Context(), linodego.FirewallCreateOptions{
		Label: "test",
		Rules: linodego.FirewallRuleSet{Inbound: []linodego.FirewallRule{{
			Action:      "ACCEPT",
			Label:       "inbound-rule123",
			Description: "inbound rule123",
			Ports:       "4321",
			Protocol:    linodego.TCP,
			Addresses: linodego.NetworkAddresses{
				IPv4: &[]string{"0.0.0.0/0"},
			},
		}}, Outbound: []linodego.FirewallRule{}, InboundPolicy: "ACCEPT", OutboundPolicy: "ACCEPT"},
	})
	if err != nil {
		t.Errorf("CreatingFirewall returned an error: %s", err)
	}
	defer func() {
		_ = fwClient.DeleteFirewall(t.Context(), fw)
	}()

	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeCloudFirewallID: strconv.Itoa(fw.ID),
	})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error while updated annotations: %s", err)
	}

	nb, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer via status: %s", err)
	}

	firewalls, err := lb.client.ListNodeBalancerFirewalls(t.Context(), nb.ID, &linodego.ListOptions{})
	if err != nil {
		t.Fatalf("failed to List Firewalls %s", err)
	}

	if len(firewalls) == 0 {
		t.Fatalf("No attached firewalls found")
	}

	if firewalls[0].ID != fw.ID {
		t.Fatalf("Attached firewallID not matching with created firewall")
	}
}

// This will also test the firewall with >255 IPs
func testUpdateLoadBalancerAddNewFirewallACL(t *testing.T, client *linodego.Client, fakeAPI *fakeAPI) {
	t.Helper()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: randString(),
			UID:  "foobar123",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus
	stubService(fakeClientset, svc)

	nb, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer via status: %s", err)
	}

	firewalls, err := lb.client.ListNodeBalancerFirewalls(t.Context(), nb.ID, &linodego.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list nodeBalancer firewalls %s", err)
	}

	if len(firewalls) != 0 {
		t.Fatalf("Firewalls attached when none specified")
	}

	ipv4s := make([]string, 0, 400)
	ipv6s := make([]string, 0, 300)
	i := 0
	for i < 400 {
		ipv4s = append(ipv4s, fmt.Sprintf("%d.%d.%d.%d", 192, rand.Int31n(255), rand.Int31n(255), rand.Int31n(255)))
		i += 1
	}
	i = 0
	for i < 300 {
		ip := make([]byte, 16)
		if _, err = cryptoRand.Read(ip); err != nil {
			t.Fatalf("unable to read random bytes")
		}
		ipv6s = append(ipv6s, fmt.Sprintf("%s:%s:%s:%s:%s:%s:%s:%s",
			hex.EncodeToString(ip[0:2]),
			hex.EncodeToString(ip[2:4]),
			hex.EncodeToString(ip[4:6]),
			hex.EncodeToString(ip[6:8]),
			hex.EncodeToString(ip[8:10]),
			hex.EncodeToString(ip[10:12]),
			hex.EncodeToString(ip[12:14]),
			hex.EncodeToString(ip[14:16])))
		i += 1
	}
	acl := map[string]map[string][]string{
		"allowList": {
			"ipv4": ipv4s,
			"ipv6": ipv6s,
		},
	}
	aclString, err := json.Marshal(acl)
	if err != nil {
		t.Fatalf("unable to marshal json acl")
	}

	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeCloudFirewallACL: string(aclString),
	})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error: %s", err)
	}

	nbUpdated, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer via status: %s", err)
	}

	firewallsNew, err := lb.client.ListNodeBalancerFirewalls(t.Context(), nbUpdated.ID, &linodego.ListOptions{})
	if err != nil {
		t.Fatalf("failed to List Firewalls %s", err)
	}

	if len(firewallsNew) == 0 {
		t.Fatalf("No firewalls found")
	}

	if firewallsNew[0].Rules.InboundPolicy != drop {
		t.Errorf("expected DROP inbound policy, got %s", firewallsNew[0].Rules.InboundPolicy)
	}

	if len(firewallsNew[0].Rules.Inbound) != 4 {
		t.Errorf("expected 4 rules, got %d", len(firewallsNew[0].Rules.Inbound))
	}
}

func testUpdateLoadBalancerDeleteFirewallRemoveACL(t *testing.T, client *linodego.Client, fakeAPI *fakeAPI) {
	t.Helper()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: randString(),
			UID:  "foobar123",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeCloudFirewallACL: `{
			"allowList": {
				"ipv4": ["2.2.2.2"]
			}
		}`,
	})

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus
	stubService(fakeClientset, svc)

	nb, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer via status: %s", err)
	}

	firewalls, err := lb.client.ListNodeBalancerFirewalls(t.Context(), nb.ID, &linodego.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list nodeBalancer firewalls %s", err)
	}

	if len(firewalls) == 0 {
		t.Fatalf("No firewalls attached")
	}

	if firewalls[0].Rules.InboundPolicy != drop {
		t.Errorf("expected DROP inbound policy, got %s", firewalls[0].Rules.InboundPolicy)
	}

	fwIPs := firewalls[0].Rules.Inbound[0].Addresses.IPv4
	if fwIPs == nil {
		t.Errorf("expected IP, got %v", fwIPs)
	}

	svc.SetAnnotations(map[string]string{})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error: %s", err)
	}

	firewallsNew, err := lb.client.ListNodeBalancerFirewalls(t.Context(), nb.ID, &linodego.ListOptions{})
	if err != nil {
		t.Fatalf("failed to List Firewalls %s", err)
	}

	if len(firewallsNew) != 0 {
		t.Fatalf("firewall's %d still attached", firewallsNew[0].ID)
	}
}

func testUpdateLoadBalancerUpdateFirewallRemoveACLaddID(t *testing.T, client *linodego.Client, fakeAPI *fakeAPI) {
	t.Helper()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: randString(),
			UID:  "foobar123",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeCloudFirewallACL: `{
			"allowList": {
				"ipv4": ["2.2.2.2"]
			}
		}`,
	})

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus
	stubService(fakeClientset, svc)

	nb, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer via status: %s", err)
	}

	firewalls, err := lb.client.ListNodeBalancerFirewalls(t.Context(), nb.ID, &linodego.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list nodeBalancer firewalls %s", err)
	}

	if len(firewalls) == 0 {
		t.Fatalf("No firewalls attached")
	}

	if firewalls[0].Rules.InboundPolicy != drop {
		t.Errorf("expected DROP inbound policy, got %s", firewalls[0].Rules.InboundPolicy)
	}

	fwIPs := firewalls[0].Rules.Inbound[0].Addresses.IPv4
	if fwIPs == nil {
		t.Errorf("expected IP, got %v", fwIPs)
	}

	fwClient := services.LinodeClient{Client: client}
	fw, err := fwClient.CreateFirewall(t.Context(), linodego.FirewallCreateOptions{
		Label: "test",
		Rules: linodego.FirewallRuleSet{Inbound: []linodego.FirewallRule{{
			Action:      "ACCEPT",
			Label:       "inbound-rule123",
			Description: "inbound rule123",
			Ports:       "4321",
			Protocol:    linodego.TCP,
			Addresses: linodego.NetworkAddresses{
				IPv4: &[]string{"0.0.0.0/0"},
			},
		}}, Outbound: []linodego.FirewallRule{}, InboundPolicy: "ACCEPT", OutboundPolicy: "ACCEPT"},
	})
	if err != nil {
		t.Errorf("Error creating firewall %s", err)
	}
	defer func() {
		_ = fwClient.DeleteFirewall(t.Context(), fw)
	}()

	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeCloudFirewallID: strconv.Itoa(fw.ID),
	})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error: %s", err)
	}

	nbUpdated, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer via status: %s", err)
	}

	firewallsNew, err := lb.client.ListNodeBalancerFirewalls(t.Context(), nbUpdated.ID, &linodego.ListOptions{})
	if err != nil {
		t.Fatalf("failed to List Firewalls %s", err)
	}

	if len(firewallsNew) == 0 {
		t.Fatalf("No attached firewalls found")
	}

	if firewallsNew[0].Rules.InboundPolicy != "ACCEPT" {
		t.Errorf("expected ACCEPT inbound policy, got %s", firewallsNew[0].Rules.InboundPolicy)
	}

	fwIPs = firewallsNew[0].Rules.Inbound[0].Addresses.IPv4
	if fwIPs == nil {
		t.Errorf("expected 2.2.2.2, got %v", fwIPs)
	}

	if firewallsNew[0].ID != fw.ID {
		t.Errorf("Firewall ID does not match what we created, something wrong.")
	}
}

func testUpdateLoadBalancerUpdateFirewallRemoveIDaddACL(t *testing.T, client *linodego.Client, fakeAPI *fakeAPI) {
	t.Helper()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: randString(),
			UID:  "foobar123",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	fwClient := services.LinodeClient{Client: client}
	fw, err := fwClient.CreateFirewall(t.Context(), linodego.FirewallCreateOptions{
		Label: "test",
		Rules: linodego.FirewallRuleSet{Inbound: []linodego.FirewallRule{{
			Action:      "ACCEPT",
			Label:       "inbound-rule123",
			Description: "inbound rule123",
			Ports:       "4321",
			Protocol:    linodego.TCP,
			Addresses: linodego.NetworkAddresses{
				IPv4: &[]string{"0.0.0.0/0"},
			},
		}}, Outbound: []linodego.FirewallRule{}, InboundPolicy: "ACCEPT", OutboundPolicy: "ACCEPT"},
	})
	if err != nil {
		t.Errorf("Error creating firewall %s", err)
	}
	defer func() {
		_ = fwClient.DeleteFirewall(t.Context(), fw)
	}()

	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeCloudFirewallID: strconv.Itoa(fw.ID),
	})

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	stubService(fakeClientset, svc)
	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus
	stubServiceUpdate(fakeClientset, svc)

	nb, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer via status: %s", err)
	}

	firewalls, err := lb.client.ListNodeBalancerFirewalls(t.Context(), nb.ID, &linodego.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list nodeBalancer firewalls %s", err)
	}

	if len(firewalls) == 0 {
		t.Fatalf("No firewalls attached")
	}

	if firewalls[0].Rules.InboundPolicy != "ACCEPT" {
		t.Errorf("expected ACCEPT inbound policy, got %s", firewalls[0].Rules.InboundPolicy)
	}

	fwIPs := firewalls[0].Rules.Inbound[0].Addresses.IPv4
	if fwIPs == nil {
		t.Errorf("expected IP, got %v", fwIPs)
	}
	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeCloudFirewallACL: `{
			"allowList": {
				"ipv4": ["2.2.2.2"]
			}
		}`,
	})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error: %s", err)
	}

	nbUpdated, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer via status: %s", err)
	}

	firewallsNew, err := lb.client.ListNodeBalancerFirewalls(t.Context(), nbUpdated.ID, &linodego.ListOptions{})
	if err != nil {
		t.Fatalf("failed to List Firewalls %s", err)
	}

	if len(firewallsNew) == 0 {
		t.Fatalf("No attached firewalls found")
	}

	if firewallsNew[0].Rules.InboundPolicy != drop {
		t.Errorf("expected DROP inbound policy, got %s", firewallsNew[0].Rules.InboundPolicy)
	}

	fwIPs = firewallsNew[0].Rules.Inbound[0].Addresses.IPv4
	if fwIPs == nil {
		t.Errorf("expected 2.2.2.2, got %v", fwIPs)
	}

	if firewallsNew[0].ID != fw.ID {
		t.Errorf("Firewall ID does not match, something wrong.")
	}
}

func testUpdateLoadBalancerUpdateFirewallACL(t *testing.T, client *linodego.Client, fakeAPI *fakeAPI) {
	t.Helper()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: randString(),
			UID:  "foobar123",
			Annotations: map[string]string{
				annotations.AnnLinodeCloudFirewallACL: `{
					"allowList": {
						"ipv4": ["2.2.2.2/32", "3.3.3.3/32"]
					}
				}`,
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus
	stubService(fakeClientset, svc)

	nb, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer via status: %s", err)
	}

	firewalls, err := lb.client.ListNodeBalancerFirewalls(t.Context(), nb.ID, &linodego.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list nodeBalancer firewalls %s", err)
	}

	if len(firewalls) == 0 {
		t.Fatalf("No firewalls attached")
	}

	if firewalls[0].Rules.InboundPolicy != drop {
		t.Errorf("expected DROP inbound policy, got %s", firewalls[0].Rules.InboundPolicy)
	}

	fwIPs := firewalls[0].Rules.Inbound[0].Addresses.IPv4
	if fwIPs == nil {
		t.Errorf("expected ips, got %v", fwIPs)
	}

	// Add ipv6 ips in allowList
	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeCloudFirewallACL: `{
			"allowList": {
				"ipv4": ["2.2.2.2/32", "3.3.3.3/32"],
				"ipv6": ["dead:beef::/128", "dead:bee::/128"]
			}
		}`,
	})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error: %s", err)
	}

	nbUpdated, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer via status: %s", err)
	}

	firewallsNew, err := lb.client.ListNodeBalancerFirewalls(t.Context(), nbUpdated.ID, &linodego.ListOptions{})
	if err != nil {
		t.Fatalf("failed to List Firewalls %s", err)
	}

	if len(firewallsNew) == 0 {
		t.Fatalf("No attached firewalls found")
	}

	fwIPs = firewallsNew[0].Rules.Inbound[0].Addresses.IPv4
	if fwIPs == nil {
		t.Errorf("expected non nil IPv4, got %v", fwIPs)
	}

	if len(*fwIPs) != 2 {
		t.Errorf("expected two IPv4 ips, got %v", fwIPs)
	}

	if firewallsNew[0].Rules.Inbound[0].Addresses.IPv6 == nil {
		t.Errorf("expected non nil IPv6, got %v", firewallsNew[0].Rules.Inbound[0].Addresses.IPv6)
	}

	if len(*firewallsNew[0].Rules.Inbound[0].Addresses.IPv6) != 2 {
		t.Errorf("expected two IPv6 ips, got %v", firewallsNew[0].Rules.Inbound[0].Addresses.IPv6)
	}

	// Update ips in allowList
	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeCloudFirewallACL: `{
			"allowList": {
				"ipv4": ["2.2.2.1/32", "3.3.3.3/32"],
				"ipv6": ["dead::/128", "dead:bee::/128"]
			}
		}`,
	})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error: %s", err)
	}

	nbUpdated, err = lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer via status: %s", err)
	}

	firewallsNew, err = lb.client.ListNodeBalancerFirewalls(t.Context(), nbUpdated.ID, &linodego.ListOptions{})
	if err != nil {
		t.Fatalf("failed to List Firewalls %s", err)
	}

	if len(firewallsNew) == 0 {
		t.Fatalf("No attached firewalls found")
	}

	fwIPs = firewallsNew[0].Rules.Inbound[0].Addresses.IPv4
	if fwIPs == nil {
		t.Errorf("expected non nil IPv4, got %v", fwIPs)
	}

	if len(*fwIPs) != 2 {
		t.Errorf("expected two IPv4 ips, got %v", fwIPs)
	}

	if firewallsNew[0].Rules.Inbound[0].Addresses.IPv6 == nil {
		t.Errorf("expected non nil IPv6, got %v", firewallsNew[0].Rules.Inbound[0].Addresses.IPv6)
	}

	if len(*firewallsNew[0].Rules.Inbound[0].Addresses.IPv6) != 2 {
		t.Errorf("expected two IPv6 ips, got %v", firewallsNew[0].Rules.Inbound[0].Addresses.IPv6)
	}

	// remove one ipv4 and one ipv6 ip from allowList
	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeCloudFirewallACL: `{
			"allowList": {
				"ipv4": ["3.3.3.3/32"],
				"ipv6": ["dead:beef::/128"]
			}
		}`,
	})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error: %s", err)
	}

	nbUpdated, err = lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer via status: %s", err)
	}

	firewallsNew, err = lb.client.ListNodeBalancerFirewalls(t.Context(), nbUpdated.ID, &linodego.ListOptions{})
	if err != nil {
		t.Fatalf("failed to List Firewalls %s", err)
	}

	if len(firewallsNew) == 0 {
		t.Fatalf("No attached firewalls found")
	}

	fwIPs = firewallsNew[0].Rules.Inbound[0].Addresses.IPv4
	if fwIPs == nil {
		t.Errorf("expected non nil IPv4, got %v", fwIPs)
	}

	if len(*fwIPs) != 1 {
		t.Errorf("expected one IPv4, got %v", fwIPs)
	}

	if firewallsNew[0].Rules.Inbound[0].Addresses.IPv6 == nil {
		t.Errorf("expected non nil IPv6, got %v", firewallsNew[0].Rules.Inbound[0].Addresses.IPv6)
	}

	if len(*firewallsNew[0].Rules.Inbound[0].Addresses.IPv6) != 1 {
		t.Errorf("expected one IPv6, got %v", firewallsNew[0].Rules.Inbound[0].Addresses.IPv6)
	}

	// Run update with same ACL
	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error: %s", err)
	}
}

func testUpdateLoadBalancerUpdateFirewall(t *testing.T, client *linodego.Client, fakeAPI *fakeAPI) {
	t.Helper()

	firewallCreateOpts := linodego.FirewallCreateOptions{
		Label: "test",
		Rules: linodego.FirewallRuleSet{Inbound: []linodego.FirewallRule{{
			Action:      "ACCEPT",
			Label:       "inbound-rule123",
			Description: "inbound rule123",
			Ports:       "4321",
			Protocol:    linodego.TCP,
			Addresses: linodego.NetworkAddresses{
				IPv4: &[]string{"0.0.0.0/0"},
			},
		}}, Outbound: []linodego.FirewallRule{}, InboundPolicy: "ACCEPT", OutboundPolicy: "ACCEPT"},
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: randString(),
			UID:  "foobar123",
			Annotations: map[string]string{
				annotations.AnnLinodeThrottle: "15",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	fwClient := services.LinodeClient{Client: client}
	fw, err := fwClient.CreateFirewall(t.Context(), firewallCreateOpts)
	if err != nil {
		t.Errorf("Error creating firewall %s", err)
	}
	defer func() {
		_ = fwClient.DeleteFirewall(t.Context(), fw)
	}()

	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeCloudFirewallID: strconv.Itoa(fw.ID),
	})

	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus
	stubService(fakeClientset, svc)

	nb, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer via status: %s", err)
	}

	firewalls, err := lb.client.ListNodeBalancerFirewalls(t.Context(), nb.ID, &linodego.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list nodeBalancer firewalls %s", err)
	}

	if len(firewalls) == 0 {
		t.Fatalf("No firewalls attached")
	}

	if fw.ID != firewalls[0].ID {
		t.Fatalf("Attached firewallID not matching with created firewall")
	}

	firewallCreateOpts.Label = "test2"
	firewallNew, err := fwClient.CreateFirewall(t.Context(), firewallCreateOpts)
	if err != nil {
		t.Fatalf("Error in creating firewall %s", err)
	}
	defer func() {
		_ = fwClient.DeleteFirewall(t.Context(), firewallNew)
	}()

	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeCloudFirewallID: strconv.Itoa(firewallNew.ID),
	})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error: %s", err)
	}

	nbUpdated, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer via status: %s", err)
	}

	firewallsNew, err := lb.client.ListNodeBalancerFirewalls(t.Context(), nbUpdated.ID, &linodego.ListOptions{})
	if err != nil {
		t.Fatalf("failed to List Firewalls %s", err)
	}

	if len(firewallsNew) == 0 {
		t.Fatalf("No attached firewalls found")
	}

	if firewallsNew[0].ID != firewallNew.ID {
		t.Fatalf("Attached firewallID not matching with created firewall")
	}
}

func testUpdateLoadBalancerDeleteFirewallRemoveID(t *testing.T, client *linodego.Client, fakeAPI *fakeAPI) {
	t.Helper()

	firewallCreateOpts := linodego.FirewallCreateOptions{
		Label: "test",
		Rules: linodego.FirewallRuleSet{Inbound: []linodego.FirewallRule{{
			Action:      "ACCEPT",
			Label:       "inbound-rule123",
			Description: "inbound rule123",
			Ports:       "4321",
			Protocol:    linodego.TCP,
			Addresses: linodego.NetworkAddresses{
				IPv4: &[]string{"0.0.0.0/0"},
			},
		}}, Outbound: []linodego.FirewallRule{}, InboundPolicy: "ACCEPT", OutboundPolicy: "ACCEPT"},
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: randString(),
			UID:  "foobar123",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	fwClient := services.LinodeClient{Client: client}
	fw, err := fwClient.CreateFirewall(t.Context(), firewallCreateOpts)
	if err != nil {
		t.Errorf("Error in creating firewall %s", err)
	}
	defer func() {
		_ = fwClient.DeleteFirewall(t.Context(), fw)
	}()

	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeCloudFirewallID: strconv.Itoa(fw.ID),
	})

	stubService(fakeClientset, svc)
	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("EnsureLoadBalancer returned an error: %s", err)
	}
	svc.Status.LoadBalancer = *lbStatus
	stubServiceUpdate(fakeClientset, svc)

	nb, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("failed to get NodeBalancer via status: %s", err)
	}

	firewalls, err := lb.client.ListNodeBalancerFirewalls(t.Context(), nb.ID, &linodego.ListOptions{})
	if err != nil {
		t.Errorf("Error in listing firewalls %s", err)
	}

	if len(firewalls) == 0 {
		t.Fatalf("No firewalls attached")
	}

	if fw.ID != firewalls[0].ID {
		t.Fatalf("Attached firewallID not matching with created firewall")
	}

	svc.SetAnnotations(map[string]string{})

	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error: %s", err)
	}

	firewallsNew, err := lb.client.ListNodeBalancerFirewalls(t.Context(), nb.ID, &linodego.ListOptions{})
	if err != nil {
		t.Fatalf("failed to List Firewalls %s", err)
	}

	if len(firewallsNew) != 0 {
		t.Fatalf("firewall's %d still attached", firewallsNew[0].ID)
	}
}

func testUpdateLoadBalancerAddReservedIP(t *testing.T, client *linodego.Client, fakeAPI *fakeAPI) {
	t.Helper()
	clusterName := "linodelb"
	region := "us-west"

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        randString(),
			UID:         "foobar123",
			Annotations: map[string]string{},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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

	lb, assertion := newLoadbalancers(client, region).(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), clusterName, svc)
	}()

	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	nodeBalancer, err := client.CreateNodeBalancer(t.Context(), linodego.NodeBalancerCreateOptions{
		Region: lb.zone,
	})
	if err != nil {
		t.Fatalf("failed to create NodeBalancer: %s", err)
	}

	initialIP := string(*nodeBalancer.IPv4)
	svc.Status.LoadBalancer = *makeLoadBalancerStatus(svc, nodeBalancer)

	ipaddr, err := client.ReserveIPAddress(context.TODO(), linodego.ReserveIPOptions{
		Region: lb.zone,
	})
	if err != nil {
		t.Fatalf("failed to reserve IP address: %s", err)
	}

	stubService(fakeClientset, svc)
	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeLoadBalancerIPv4: ipaddr.Address,
	})

	err = lb.UpdateLoadBalancer(t.Context(), "", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error while updated annotations: %s", err)
	}

	status, _, err := lb.GetLoadBalancer(t.Context(), clusterName, svc)
	if status.Ingress[0].IP != initialIP {
		t.Fatalf("IP should not have changed in service status: %s", err)
	}

	event, _ := fakeClientset.CoreV1().Events("").Get(t.Context(),
		eventIPChangeIgnoredWarning,
		metav1.GetOptions{})
	if event == nil {
		t.Fatalf("failed to generate %s event: %s", eventIPChangeIgnoredWarning, err)
	}
}

func testUpdateLoadBalancerAddNodeBalancerID(t *testing.T, client *linodego.Client, fakeAPI *fakeAPI) {
	t.Helper()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        randString(),
			UID:         "foobar123",
			Annotations: map[string]string{},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
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

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	nodeBalancer, err := client.CreateNodeBalancer(t.Context(), linodego.NodeBalancerCreateOptions{
		Region: lb.zone,
	})
	if err != nil {
		t.Fatalf("failed to create NodeBalancer: %s", err)
	}

	svc.Status.LoadBalancer = *makeLoadBalancerStatus(svc, nodeBalancer)

	newNodeBalancer, err := client.CreateNodeBalancer(t.Context(), linodego.NodeBalancerCreateOptions{
		Region: lb.zone,
	})
	if err != nil {
		t.Fatalf("failed to create new NodeBalancer: %s", err)
	}

	stubService(fakeClientset, svc)
	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeNodeBalancerID: strconv.Itoa(newNodeBalancer.ID),
	})
	err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Errorf("UpdateLoadBalancer returned an error while updated annotations: %s", err)
	}

	clusterName := strings.TrimPrefix(svc.Namespace, "kube-system-")
	lbStatus, _, err := lb.GetLoadBalancer(t.Context(), clusterName, svc)
	if err != nil {
		t.Errorf("GetLoadBalancer returned an error: %s", err)
	}

	expectedLBStatus := makeLoadBalancerStatus(svc, newNodeBalancer)
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
					Name:        randString(),
					UID:         "abc123",
					Annotations: map[string]string{},
				},
			},
			0,
		},
		{
			"throttle value is a string",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeThrottle: "foo",
					},
				},
			},
			0,
		},
		{
			"throttle value is less than 0",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeThrottle: "-123",
					},
				},
			},
			0,
		},
		{
			"throttle value is valid",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeThrottle: "1",
					},
				},
			},
			1,
		},
		{
			"throttle value is too high",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeThrottle: "21",
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
		port               v1.ServicePort
		expectedPortConfig portConfig
		err                error
	}{
		{
			"default no proxy protocol specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolTCP,
				Port:     443,
			},
			portConfig{
				Port:          443,
				Protocol:      "tcp",
				ProxyProtocol: linodego.ProxyProtocolNone,
				Algorithm:     linodego.AlgorithmRoundRobin,
			},
			nil,
		},
		{
			"default proxy protocol specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProxyProtocol: string(linodego.ProxyProtocolV2),
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolTCP,
				Port:     443,
			},
			portConfig{
				Port:          443,
				Protocol:      "tcp",
				ProxyProtocol: linodego.ProxyProtocolV2,
				Algorithm:     linodego.AlgorithmRoundRobin,
			},
			nil,
		},
		{
			"port specific proxy protocol specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProxyProtocol:     string(linodego.ProxyProtocolV2),
						annotations.AnnLinodePortConfigPrefix + "443": fmt.Sprintf(`{"proxy-protocol": "%s"}`, linodego.ProxyProtocolV1),
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolTCP,
				Port:     443,
			},
			portConfig{
				Port:          443,
				Protocol:      "tcp",
				ProxyProtocol: linodego.ProxyProtocolV1,
				Algorithm:     linodego.AlgorithmRoundRobin,
			},
			nil,
		},
		{
			"default invalid proxy protocol",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProxyProtocol: "invalid",
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolTCP,
				Port:     443,
			},
			portConfig{
				Port:     443,
				Protocol: "tcp",
			},
			fmt.Errorf("invalid NodeBalancer proxy protocol value '%s'", "invalid"),
		},
		{
			"default no protocol specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolTCP,
				Port:     int32(443),
			},
			portConfig{
				Port:          443,
				Protocol:      "tcp",
				ProxyProtocol: linodego.ProxyProtocolNone,
				Algorithm:     linodego.AlgorithmRoundRobin,
			},
			nil,
		},
		{
			"default tcp protocol specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProtocol: "tcp",
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolTCP,
				Port:     443,
			},
			portConfig{
				Port:          443,
				Protocol:      "tcp",
				ProxyProtocol: linodego.ProxyProtocolNone,
				Algorithm:     linodego.AlgorithmRoundRobin,
			},
			nil,
		},
		{
			"different algorithm specified for tcp protocol",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProtocol:  "tcp",
						annotations.AnnLinodeDefaultAlgorithm: string(linodego.AlgorithmSource),
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolTCP,
				Port:     443,
			},
			portConfig{
				Port:          443,
				Protocol:      "tcp",
				ProxyProtocol: linodego.ProxyProtocolNone,
				Algorithm:     linodego.AlgorithmSource,
			},
			nil,
		},
		{
			"algorithm ring_hash is not allowed for tcp protocol",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProtocol:  "tcp",
						annotations.AnnLinodeDefaultAlgorithm: string(linodego.AlgorithmRingHash),
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolTCP,
				Port:     443,
			},
			portConfig{
				Port:          443,
				Protocol:      "tcp",
				ProxyProtocol: linodego.ProxyProtocolNone,
			},
			fmt.Errorf("invalid algorithm: %q specified for TCP/HTTP/HTTPS protocol", string(linodego.AlgorithmRingHash)),
		},
		{
			"default udp protocol specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProtocol: "udp",
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolUDP,
				Port:     2222,
			},
			portConfig{
				Port:          2222,
				Protocol:      "udp",
				ProxyProtocol: linodego.ProxyProtocolNone,
				Algorithm:     linodego.AlgorithmRoundRobin,
				Stickiness:    linodego.StickinessSession,
				UDPCheckPort:  80,
			},
			nil,
		},
		{
			"default udp protocol with different port specific udp check port specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProtocol:           "udp",
						annotations.AnnLinodePortConfigPrefix + "2222": `{"udp-check-port": "8080"}`,
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolUDP,
				Port:     2222,
			},
			portConfig{
				Port:          2222,
				Protocol:      "udp",
				ProxyProtocol: linodego.ProxyProtocolNone,
				Algorithm:     linodego.AlgorithmRoundRobin,
				Stickiness:    linodego.StickinessSession,
				UDPCheckPort:  8080,
			},
			nil,
		},
		{
			"default udp protocol with different global udp check port specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProtocol: "udp",
						annotations.AnnLinodeUDPCheckPort:    "8080",
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolUDP,
				Port:     2222,
			},
			portConfig{
				Port:          2222,
				Protocol:      "udp",
				ProxyProtocol: linodego.ProxyProtocolNone,
				Algorithm:     linodego.AlgorithmRoundRobin,
				Stickiness:    linodego.StickinessSession,
				UDPCheckPort:  8080,
			},
			nil,
		},
		{
			"invalid proxyprotocol specified for udp protocol",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProtocol:      "udp",
						annotations.AnnLinodeDefaultProxyProtocol: string(linodego.ProxyProtocolV1),
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolUDP,
				Port:     2222,
			},
			portConfig{
				Port:     2222,
				Protocol: "udp",
			},
			fmt.Errorf("proxy protocol [%s] is not supported for UDP", string(linodego.ProxyProtocolV1)),
		},
		{
			"algorithm source is not allowed for udp protocol",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProtocol:  "udp",
						annotations.AnnLinodeDefaultAlgorithm: string(linodego.AlgorithmSource),
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolUDP,
				Port:     2222,
			},
			portConfig{
				Port:          2222,
				Protocol:      "udp",
				ProxyProtocol: linodego.ProxyProtocolNone,
			},
			fmt.Errorf("invalid algorithm: %q specified for UDP protocol", string(linodego.AlgorithmSource)),
		},
		{
			"udp_check_port should be within 1-65535",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProtocol:           "udp",
						annotations.AnnLinodePortConfigPrefix + "2222": `{"udp-check-port": "88888"}`,
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolUDP,
				Port:     2222,
			},
			portConfig{
				Port:          2222,
				Protocol:      "udp",
				ProxyProtocol: linodego.ProxyProtocolNone,
				Algorithm:     linodego.AlgorithmRoundRobin,
			},
			fmt.Errorf("UDPCheckPort must be between 1 and 65535, got %d", 88888),
		},
		{
			"tls secret is not allowed for udp protocol",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProtocol:           "udp",
						annotations.AnnLinodePortConfigPrefix + "2222": `{"tls-secret-name": "test"}`,
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolUDP,
				Port:     2222,
			},
			portConfig{
				Port:          2222,
				Protocol:      "udp",
				ProxyProtocol: linodego.ProxyProtocolNone,
				Algorithm:     linodego.AlgorithmRoundRobin,
			},
			fmt.Errorf("specifying TLS secret name is not supported for UDP"),
		},
		{
			"no error on stickiness for tcp protocol, it gets ignored",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProtocol:          "tcp",
						annotations.AnnLinodePortConfigPrefix + "443": `{"stickiness": "table"}`,
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolUDP,
				Port:     443,
			},
			portConfig{
				Port:          443,
				Protocol:      "tcp",
				ProxyProtocol: linodego.ProxyProtocolNone,
				Algorithm:     linodego.AlgorithmRoundRobin,
			},
			nil,
		},
		{
			"stickiness table is not allowed for udp protocol",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProtocol:   "udp",
						annotations.AnnLinodeDefaultStickiness: string(linodego.StickinessTable),
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolUDP,
				Port:     2222,
			},
			portConfig{
				Port:          2222,
				Protocol:      "udp",
				ProxyProtocol: linodego.ProxyProtocolNone,
				Algorithm:     linodego.AlgorithmRoundRobin,
				UDPCheckPort:  80,
			},
			fmt.Errorf("invalid stickiness: %q specified for UDP protocol", linodego.StickinessTable),
		},
		{
			"stickiness session is not allowed for http protocol",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProtocol:   "http",
						annotations.AnnLinodeDefaultStickiness: string(linodego.StickinessSession),
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolTCP,
				Port:     443,
			},
			portConfig{
				Port:          443,
				Protocol:      "http",
				ProxyProtocol: linodego.ProxyProtocolNone,
				Algorithm:     linodego.AlgorithmRoundRobin,
			},
			fmt.Errorf("invalid stickiness: %q specified for HTTP protocol", linodego.StickinessSession),
		},
		{
			"stickiness session is not allowed for https protocol",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProtocol:   "https",
						annotations.AnnLinodeDefaultStickiness: string(linodego.StickinessSession),
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolTCP,
				Port:     443,
			},
			portConfig{
				Port:          443,
				Protocol:      "https",
				ProxyProtocol: linodego.ProxyProtocolNone,
				Algorithm:     linodego.AlgorithmRoundRobin,
			},
			fmt.Errorf("invalid stickiness: %q specified for HTTPS protocol", linodego.StickinessSession),
		},
		{
			"default capitalized protocol specified",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProtocol: "HTTP",
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolTCP,
				Port:     443,
			},
			portConfig{
				Port:          443,
				Protocol:      "http",
				ProxyProtocol: linodego.ProxyProtocolNone,
				Algorithm:     linodego.AlgorithmRoundRobin,
				Stickiness:    linodego.StickinessTable,
			},
			nil,
		},
		{
			"default invalid protocol",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProtocol: "invalid",
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolTCP,
				Port:     443,
			},
			portConfig{
				Port: 443,
			},
			fmt.Errorf("invalid protocol: %q specified", "invalid"),
		},
		{
			"port config falls back to default",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodeDefaultProtocol:          "http",
						annotations.AnnLinodePortConfigPrefix + "443": `{}`,
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolTCP,
				Port:     443,
			},
			portConfig{
				Port:          443,
				Protocol:      "http",
				ProxyProtocol: linodego.ProxyProtocolNone,
				Algorithm:     linodego.AlgorithmRoundRobin,
				Stickiness:    linodego.StickinessTable,
			},
			nil,
		},
		{
			"port config capitalized protocol",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodePortConfigPrefix + "443": `{ "protocol": "HTTp" }`,
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolTCP,
				Port:     443,
			},
			portConfig{
				Port:          443,
				Protocol:      "http",
				ProxyProtocol: linodego.ProxyProtocolNone,
				Algorithm:     linodego.AlgorithmRoundRobin,
				Stickiness:    linodego.StickinessTable,
			},
			nil,
		},
		{
			"port config invalid protocol",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: randString(),
					UID:  "abc123",
					Annotations: map[string]string{
						annotations.AnnLinodePortConfigPrefix + "443": `{ "protocol": "invalid" }`,
					},
				},
			},
			v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolTCP,
				Port:     443,
			},
			portConfig{Port: 443},
			fmt.Errorf("invalid protocol: %q specified", "invalid"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			portConfigResult, err := getPortConfig(test.service, test.port)

			if !reflect.DeepEqual(portConfigResult, test.expectedPortConfig) {
				t.Error("unexpected port config")
				t.Logf("expected: %q", test.expectedPortConfig)
				t.Logf("actual: %q", portConfigResult)
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
					Name:        randString(),
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
						annotations.AnnLinodeHealthCheckType: "http",
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
						annotations.AnnLinodeHealthCheckType: "invalid",
					},
				},
			},
			"",
			fmt.Errorf("invalid health check type: %q specified in annotation: %q", "invalid", annotations.AnnLinodeHealthCheckType),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			port := v1.ServicePort{
				Name:     "test",
				Protocol: v1.ProtocolTCP,
				Port:     int32(443),
			}
			hType, err := getHealthCheckType(test.service, port)
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

func Test_getNodePrivateIP(t *testing.T) {
	testcases := []struct {
		name     string
		node     *v1.Node
		address  string
		subnetID int
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
			0,
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
			0,
		},
		{
			"node internal ip annotation present",
			&v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						annotations.AnnLinodeNodePrivateIP: "192.168.42.42",
					},
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    v1.NodeInternalIP,
							Address: "10.0.1.1",
						},
					},
				},
			},
			"192.168.42.42",
			0,
		},
		{
			"node internal ip annotation present and subnet id is not zero",
			&v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						annotations.AnnLinodeNodePrivateIP: "192.168.42.42",
					},
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    v1.NodeInternalIP,
							Address: "10.0.1.1",
						},
					},
				},
			},
			"10.0.1.1",
			100,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ip, _ := getNodePrivateIP(test.node, test.subnetID)
			if ip != test.address {
				t.Error("unexpected certificate")
				t.Logf("expected: %q", test.address)
				t.Logf("actual: %q", ip)
			}
		})
	}
}

func testBuildLoadBalancerRequest(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	t.Helper()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			UID:  "foobar123",
			Annotations: map[string]string{
				annotations.AnnLinodeDefaultProtocol: "tcp",
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
	}

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	nb, err := lb.buildLoadBalancerRequest(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(err, err) {
		t.Error("unexpected error")
		t.Logf("expected: %v", nil)
		t.Logf("actual: %v", err)
	}

	configs, err := client.ListNodeBalancerConfigs(t.Context(), nb.ID, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(configs) != len(svc.Spec.Ports) {
		t.Error("unexpected nodebalancer config count")
		t.Logf("expected: %v", len(svc.Spec.Ports))
		t.Logf("actual: %v", len(configs))
	}

	nbNodes, err := client.ListNodeBalancerNodes(t.Context(), nb.ID, configs[0].ID, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(nbNodes) != len(nodes) {
		t.Error("unexpected nodebalancer nodes count")
		t.Logf("expected: %v", len(nodes))
		t.Logf("actual: %v", len(nbNodes))
	}
}

func testEnsureLoadBalancerPreserveAnnotation(t *testing.T, client *linodego.Client, fake *fakeAPI) {
	t.Helper()

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

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	for _, test := range []struct {
		name        string
		deleted     bool
		annotations map[string]string
	}{
		{
			name:        "load balancer preserved",
			annotations: map[string]string{annotations.AnnLinodeLoadBalancerPreserve: "true"},
			deleted:     false,
		},
		{
			name:        "load balancer not preserved (deleted)",
			annotations: map[string]string{annotations.AnnLinodeLoadBalancerPreserve: "false"},
			deleted:     true,
		},
		{
			name:        "invalid value treated as false (deleted)",
			annotations: map[string]string{annotations.AnnLinodeLoadBalancerPreserve: "bogus"},
			deleted:     true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test",
					UID:         types.UID("foobar" + randString()),
					Annotations: test.annotations,
				},
				Spec: testServiceSpec,
			}

			nb, err := lb.createNodeBalancer(t.Context(), "linodelb", svc, []*linodego.NodeBalancerConfigCreateOptions{})
			if err != nil {
				t.Fatal(err)
			}

			svc.Status.LoadBalancer = *makeLoadBalancerStatus(svc, nb)
			err = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)

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
	t.Helper()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			UID:  "foobar123",
			Annotations: map[string]string{
				annotations.AnnLinodeDefaultProtocol: "tcp",
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
						annotations.AnnLinodeDefaultProtocol: "tcp",
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

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	configs := []*linodego.NodeBalancerConfigCreateOptions{}
	_, err := lb.createNodeBalancer(t.Context(), "linodelb", svc, configs)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc) }()

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := lb.EnsureLoadBalancerDeleted(t.Context(), test.clusterName, test.service)
			if !reflect.DeepEqual(err, test.err) {
				t.Error("unexpected error")
				t.Logf("expected: %v", test.err)
				t.Logf("actual: %v", err)
			}
		})
	}
}

func testEnsureExistingLoadBalancer(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	t.Helper()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testensure",
			UID:  "foobar123",
			Annotations: map[string]string{
				annotations.AnnLinodeDefaultProtocol:           "tcp",
				annotations.AnnLinodePortConfigPrefix + "8443": `{ "protocol": "https", "tls-secret-name": "tls-secret"}`,
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

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	lb.kubeClient = fake.NewSimpleClientset()
	addTLSSecret(t, lb.kubeClient)

	configs := []*linodego.NodeBalancerConfigCreateOptions{}
	nb, err := lb.createNodeBalancer(t.Context(), "linodelb", svc, configs)
	if err != nil {
		t.Fatal(err)
	}

	svc.Status.LoadBalancer = *makeLoadBalancerStatus(svc, nb)
	defer func() { _ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc) }()
	getLBStatus, exists, err := lb.GetLoadBalancer(t.Context(), "linodelb", svc)
	if err != nil {
		t.Fatalf("failed to create nodebalancer: %s", err)
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
			lbStatus, err := lb.EnsureLoadBalancer(t.Context(), test.clusterName, test.service, test.nodes)
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

func testMakeLoadBalancerStatus(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	t.Helper()

	ipv4 := "192.168.0.1"
	hostname := "nb-192-168-0-1.newark.nodebalancer.linode.com"
	nb := &linodego.NodeBalancer{
		IPv4:     &ipv4,
		Hostname: &hostname,
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test",
			Annotations: make(map[string]string, 1),
		},
	}

	expectedStatus := &v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{{
			Hostname: hostname,
			IP:       ipv4,
		}},
	}
	status := makeLoadBalancerStatus(svc, nb)
	if !reflect.DeepEqual(status, expectedStatus) {
		t.Errorf("expected status for basic service to be %#v; got %#v", expectedStatus, status)
	}

	svc.Annotations[annotations.AnnLinodeHostnameOnlyIngress] = "true"
	expectedStatus.Ingress[0] = v1.LoadBalancerIngress{Hostname: hostname}
	status = makeLoadBalancerStatus(svc, nb)
	if !reflect.DeepEqual(status, expectedStatus) {
		t.Errorf("expected status for %q annotated service to be %#v; got %#v", annotations.AnnLinodeHostnameOnlyIngress, expectedStatus, status)
	}
}

func testMakeLoadBalancerStatusWithIPv6(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	t.Helper()

	ipv4 := "192.168.0.1"
	ipv6 := "2600:3c00::f03c:91ff:fe24:3a2f"
	hostname := "nb-192-168-0-1.newark.nodebalancer.linode.com"
	nb := &linodego.NodeBalancer{
		IPv4:     &ipv4,
		IPv6:     &ipv6,
		Hostname: &hostname,
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test",
			Annotations: make(map[string]string, 1),
		},
	}

	// Test with EnableIPv6ForLoadBalancers = false (default)
	options.Options.EnableIPv6ForLoadBalancers = false
	expectedStatus := &v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{{
			Hostname: hostname,
			IP:       ipv4,
		}},
	}
	status := makeLoadBalancerStatus(svc, nb)
	if !reflect.DeepEqual(status, expectedStatus) {
		t.Errorf("expected status with EnableIPv6ForLoadBalancers=false to be %#v; got %#v", expectedStatus, status)
	}

	// Test with EnableIPv6ForLoadBalancers = true
	options.Options.EnableIPv6ForLoadBalancers = true
	expectedStatus = &v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{
			{
				Hostname: hostname,
				IP:       ipv4,
			},
			{
				Hostname: hostname,
				IP:       ipv6,
			},
		},
	}
	status = makeLoadBalancerStatus(svc, nb)
	if !reflect.DeepEqual(status, expectedStatus) {
		t.Errorf("expected status with EnableIPv6ForLoadBalancers=true to be %#v; got %#v", expectedStatus, status)
	}

	// Test with per-service annotation
	// Reset the global flag to false and set the annotation
	options.Options.EnableIPv6ForLoadBalancers = false
	svc.Annotations[annotations.AnnLinodeEnableIPv6Ingress] = "true"

	// Expect the same result as when the global flag is enabled
	status = makeLoadBalancerStatus(svc, nb)
	if !reflect.DeepEqual(status, expectedStatus) {
		t.Errorf("expected status with %s=true annotation to be %#v; got %#v",
			annotations.AnnLinodeEnableIPv6Ingress, expectedStatus, status)
	}

	// Reset the flag to its default value
	options.Options.EnableIPv6ForLoadBalancers = false
}

func testMakeLoadBalancerStatusEnvVar(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	t.Helper()

	ipv4 := "192.168.0.1"
	hostname := "nb-192-168-0-1.newark.nodebalancer.linode.com"
	nb := &linodego.NodeBalancer{
		IPv4:     &ipv4,
		Hostname: &hostname,
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test",
			Annotations: make(map[string]string, 1),
		},
	}

	expectedStatus := &v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{{
			Hostname: hostname,
			IP:       ipv4,
		}},
	}
	status := makeLoadBalancerStatus(svc, nb)
	if !reflect.DeepEqual(status, expectedStatus) {
		t.Errorf("expected status for basic service to be %#v; got %#v", expectedStatus, status)
	}

	t.Setenv("LINODE_HOSTNAME_ONLY_INGRESS", "true")
	expectedStatus.Ingress[0] = v1.LoadBalancerIngress{Hostname: hostname}
	status = makeLoadBalancerStatus(svc, nb)
	if !reflect.DeepEqual(status, expectedStatus) {
		t.Errorf("expected status for %q annotated service to be %#v; got %#v", annotations.AnnLinodeHostnameOnlyIngress, expectedStatus, status)
	}

	t.Setenv("LINODE_HOSTNAME_ONLY_INGRESS", "false")
	expectedStatus.Ingress[0] = v1.LoadBalancerIngress{Hostname: hostname}
	status = makeLoadBalancerStatus(svc, nb)
	if reflect.DeepEqual(status, expectedStatus) {
		t.Errorf("expected status for %q annotated service to be %#v; got %#v", annotations.AnnLinodeHostnameOnlyIngress, expectedStatus, status)
	}

	t.Setenv("LINODE_HOSTNAME_ONLY_INGRESS", "banana")
	expectedStatus.Ingress[0] = v1.LoadBalancerIngress{Hostname: hostname}
	status = makeLoadBalancerStatus(svc, nb)
	if reflect.DeepEqual(status, expectedStatus) {
		t.Errorf("expected status for %q annotated service to be %#v; got %#v", annotations.AnnLinodeHostnameOnlyIngress, expectedStatus, status)
	}
	os.Unsetenv("LINODE_HOSTNAME_ONLY_INGRESS")
}

func getLatestNbNodesForService(t *testing.T, client *linodego.Client, svc *v1.Service, lb *loadbalancers) []linodego.NodeBalancerNode {
	t.Helper()
	nb, err := lb.getNodeBalancerByStatus(t.Context(), svc)
	if err != nil {
		t.Fatalf("expected no error got %v", err)
	}
	cfgs, errConfigs := client.ListNodeBalancerConfigs(t.Context(), nb.ID, nil)
	if errConfigs != nil {
		t.Fatalf("expected no error getting configs, got %v", errConfigs)
	}
	slices.SortFunc(cfgs, func(a, b linodego.NodeBalancerConfig) int {
		return a.ID - b.ID
	})

	// Verify nodes were created correctly (only non-excluded nodes)
	nodeBalancerNodes, err := client.ListNodeBalancerNodes(t.Context(), nb.ID, cfgs[0].ID, nil)
	if err != nil {
		t.Fatalf("expected no error got %v", err)
	}
	return nodeBalancerNodes
}

func testCleanupDoesntCall(t *testing.T, client *linodego.Client, fakeAPI *fakeAPI) {
	t.Helper()

	region := "us-west"
	nb1, err := client.CreateNodeBalancer(t.Context(), linodego.NodeBalancerCreateOptions{Region: region})
	if err != nil {
		t.Fatal(err)
	}
	nb2, err := client.CreateNodeBalancer(t.Context(), linodego.NodeBalancerCreateOptions{Region: region})
	if err != nil {
		t.Fatal(err)
	}

	svc := &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test"}}
	svcAnn := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test",
			Annotations: map[string]string{annotations.AnnLinodeNodeBalancerID: strconv.Itoa(nb2.ID)},
		},
	}
	svc.Status.LoadBalancer = *makeLoadBalancerStatus(svc, nb1)
	svcAnn.Status.LoadBalancer = *makeLoadBalancerStatus(svcAnn, nb1)
	lb, assertion := newLoadbalancers(client, region).(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}

	fakeAPI.ResetRequests()
	t.Run("non-annotated service shouldn't call the API during cleanup", func(t *testing.T) {
		if err := lb.cleanupOldNodeBalancer(t.Context(), svc); err != nil {
			t.Fatal(err)
		}
		if len(fakeAPI.requests) != 0 {
			t.Fatalf("unexpected API calls: %v", fakeAPI.requests)
		}
	})

	fakeAPI.ResetRequests()
	t.Run("annotated service calls the API to load said NB", func(t *testing.T) {
		if err := lb.cleanupOldNodeBalancer(t.Context(), svcAnn); err != nil {
			t.Fatal(err)
		}
		expectedRequests := map[fakeRequest]struct{}{
			{Path: "/nodebalancers", Body: "", Method: "GET"}:                            {},
			{Path: fmt.Sprintf("/nodebalancers/%v", nb2.ID), Body: "", Method: "GET"}:    {},
			{Path: fmt.Sprintf("/nodebalancers/%v", nb1.ID), Body: "", Method: "DELETE"}: {},
		}
		if !reflect.DeepEqual(fakeAPI.requests, expectedRequests) {
			t.Fatalf("expected requests %#v, got %#v instead", expectedRequests, fakeAPI.requests)
		}
	})
}

func testUpdateLoadBalancerNodeExcludedByAnnotation(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	t.Helper()
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        randString(),
			UID:         "foobar123",
			Annotations: map[string]string{},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
					Protocol: "http",
					Port:     int32(80),
					NodePort: int32(8080),
				},
			},
		},
	}

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	nodeBalancer, err := client.CreateNodeBalancer(t.Context(), linodego.NodeBalancerCreateOptions{
		Region: lb.zone,
	})
	if err != nil {
		t.Fatalf("failed to create NodeBalancer: %s", err)
	}
	svc.Status.LoadBalancer = *makeLoadBalancerStatus(svc, nodeBalancer)
	stubService(fakeClientset, svc)
	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeNodeBalancerID: strconv.Itoa(nodeBalancer.ID),
	})

	// setup done, test ensure/update
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
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
				Annotations: map[string]string{
					annotations.AnnExcludeNodeFromNb: "true",
				},
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
	}

	// Test initial creation - should only create nodes that aren't excluded
	lbStatus, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Fatalf("expected no error got %v", err)
	}
	svc.Status.LoadBalancer = *lbStatus
	stubServiceUpdate(fakeClientset, svc)
	nodeBalancerNodes := getLatestNbNodesForService(t, client, svc, lb)
	// Should have only 2 nodes (node-1 and node-3), since node-2 is excluded
	if len(nodeBalancerNodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodeBalancerNodes))
	}

	// Verify excluded node is not present
	for _, nbNode := range nodeBalancerNodes {
		if strings.Contains(nbNode.Label, "node-2") {
			t.Errorf("excluded node 'node-2' should not be present in nodeBalancer nodes")
		}
	}

	// Test Update operation
	updatedNodes := []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				Annotations: map[string]string{
					annotations.AnnExcludeNodeFromNb: "true", // Now exclude node-1
				},
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
				Annotations: map[string]string{
					annotations.AnnExcludeNodeFromNb: "true", // Still excluded
				},
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
	}

	// Update the load balancer with updated nodes
	if err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, updatedNodes); err != nil {
		t.Fatalf("unexpected error updating LoadBalancer: %v", err)
	}

	// Verify nodes were updated correctly
	nodeBalancerNodesAfterUpdate := getLatestNbNodesForService(t, client, svc, lb)

	// Should have only 1 node (node-3), since both node-1 and node-2 are excluded
	if len(nodeBalancerNodesAfterUpdate) != 1 {
		t.Errorf("expected 1 node after update, got %d", len(nodeBalancerNodesAfterUpdate))
	}

	// Verify excluded nodes are not present
	for _, nbNode := range nodeBalancerNodesAfterUpdate {
		if strings.Contains(nbNode.Label, "node-1") || strings.Contains(nbNode.Label, "node-2") {
			t.Errorf("excluded nodes should not be present in nodeBalancer nodes after update")
		}
	}

	// Test edge case: all nodes excluded
	allExcludedNodes := []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				Annotations: map[string]string{
					annotations.AnnExcludeNodeFromNb: "true",
				},
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
				Annotations: map[string]string{
					annotations.AnnExcludeNodeFromNb: "true",
				},
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
				Annotations: map[string]string{
					annotations.AnnExcludeNodeFromNb: "true",
				},
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
	}

	if err = lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, allExcludedNodes); err != nil {
		t.Errorf("expected no error when all nodes are excluded, got %v", err)
	}

	// Verify nodes were updated correctly
	nodeBalancerNodesAfterUpdate = getLatestNbNodesForService(t, client, svc, lb)
	// Should have only 0 node (node-3), since all nodes are excluded
	if len(nodeBalancerNodesAfterUpdate) != 0 {
		t.Errorf("expected 0 nodes after update, got %d", len(nodeBalancerNodesAfterUpdate))
	}
}

func testUpdateLoadBalancerNoNodes(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	t.Helper()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        randString(),
			UID:         "foobar123",
			Annotations: map[string]string{},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     randString(),
					Protocol: "http",
					Port:     int32(80),
					NodePort: int32(8080),
				},
			},
		},
	}

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	defer func() {
		_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc)
	}()

	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	nodeBalancer, err := client.CreateNodeBalancer(t.Context(), linodego.NodeBalancerCreateOptions{
		Region: lb.zone,
	})
	if err != nil {
		t.Fatalf("failed to create NodeBalancer: %s", err)
	}
	svc.Status.LoadBalancer = *makeLoadBalancerStatus(svc, nodeBalancer)
	stubService(fakeClientset, svc)
	svc.SetAnnotations(map[string]string{
		annotations.AnnLinodeNodeBalancerID: strconv.Itoa(nodeBalancer.ID),
	})

	// setup done, test ensure/update
	nodes := []*v1.Node{}

	if _, err = lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes); !stderrors.Is(err, errNoNodesAvailable) {
		t.Errorf("EnsureLoadBalancer should return %v, got %v", errNoNodesAvailable, err)
	}

	if err := lb.UpdateLoadBalancer(t.Context(), "linodelb", svc, nodes); !stderrors.Is(err, errNoNodesAvailable) {
		t.Errorf("UpdateLoadBalancer should return %v, got %v", errNoNodesAvailable, err)
	}
}

func testGetNodeBalancerByStatus(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	t.Helper()

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	for _, test := range []struct {
		name    string
		service *v1.Service
	}{
		{
			name: "hostname only",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "hostname-ingress-" + randString(),
					Annotations: map[string]string{annotations.AnnLinodeHostnameOnlyIngress: "true"},
				},
			},
		},
		{
			name: "ipv4",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ipv4-ingress-" + randString(),
				},
			},
		},
		{
			name: "ipv4 and ipv6",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "ipv6-ingress-" + randString(),
					Annotations: map[string]string{annotations.AnnLinodeEnableIPv6Ingress: "true"},
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			expectedNB, err := lb.createNodeBalancer(t.Context(), "linodelb", test.service, []*linodego.NodeBalancerConfigCreateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			test.service.Status.LoadBalancer = *makeLoadBalancerStatus(test.service, expectedNB)

			stubService(fakeClientset, test.service)
			actualNB, err := lb.getNodeBalancerByStatus(t.Context(), test.service)
			if err != nil {
				t.Fatal(err)
			}

			if expectedNB.ID != actualNB.ID {
				t.Error("unexpected nodebalancer ID")
				t.Logf("expected: %v", expectedNB.ID)
				t.Logf("actual: %v", actualNB.ID)
			}

			_ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", test.service)
		})
	}
}

func testGetNodeBalancerForServiceIDDoesNotExist(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	t.Helper()

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	bogusNodeBalancerID := "123456"

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			UID:  "foobar123",
			Annotations: map[string]string{
				annotations.AnnLinodeNodeBalancerID: bogusNodeBalancerID,
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

	_, err := lb.getNodeBalancerForService(t.Context(), svc)
	if err == nil {
		t.Fatal("expected getNodeBalancerForService to return an error")
	}

	nbid, _ := strconv.Atoi(bogusNodeBalancerID)
	expectedErr := lbNotFoundError{
		serviceNn:      getServiceNn(svc),
		nodeBalancerID: nbid,
	}
	if err.Error() != expectedErr.Error() {
		t.Errorf("expected error to be '%s' but got '%s'", expectedErr, err)
	}
}

func testEnsureNewLoadBalancerWithNodeBalancerID(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	t.Helper()

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	nodeBalancer, err := client.CreateNodeBalancer(t.Context(), linodego.NodeBalancerCreateOptions{
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
				annotations.AnnLinodeNodeBalancerID: strconv.Itoa(nodeBalancer.ID),
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

	defer func() { _ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc) }()

	if _, err = lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes); err != nil {
		t.Fatal(err)
	}
}

func testEnsureNewLoadBalancer(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	t.Helper()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testensure",
			UID:  "foobar123",
			Annotations: map[string]string{
				annotations.AnnLinodeDefaultProtocol:           "tcp",
				annotations.AnnLinodePortConfigPrefix + "8443": `{ "protocol": "https", "tls-secret-name": "tls-secret"}`,
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
	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	lb.kubeClient = fake.NewSimpleClientset()
	addTLSSecret(t, lb.kubeClient)

	defer func() { _ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc) }()

	_, err := lb.EnsureLoadBalancer(t.Context(), "linodelb", svc, nodes)
	if err != nil {
		t.Fatal(err)
	}
}

func testGetLoadBalancer(t *testing.T, client *linodego.Client, _ *fakeAPI) {
	t.Helper()

	lb, assertion := newLoadbalancers(client, "us-west").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			UID:  "foobar123",
			Annotations: map[string]string{
				annotations.AnnLinodeDefaultProtocol: "tcp",
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
	svc2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "svc2",
			UID:  "svc234",
			Annotations: map[string]string{
				annotations.AnnLinodeDefaultProtocol: "tcp",
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
	svc3 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "notexist3",
			UID:  "notexists345",
			Annotations: map[string]string{
				annotations.AnnLinodeDefaultProtocol: "tcp",
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
	nb, err := lb.createNodeBalancer(t.Context(), "linodelb", svc, configs)
	if err != nil {
		t.Fatal(err)
	}
	fakeClientset := fake.NewSimpleClientset()
	lb.kubeClient = fakeClientset

	defer func() { _ = lb.EnsureLoadBalancerDeleted(t.Context(), "linodelb", svc) }()

	lbStatus := makeLoadBalancerStatus(svc, nb)
	svc.Status.LoadBalancer = *lbStatus
	stubService(fakeClientset, svc)
	stubService(fakeClientset, svc2)

	testcases := []struct {
		name        string
		service     *v1.Service
		clusterName string
		found       bool
		err         error
	}{
		{
			"Service and Load balancer exists",
			svc,
			"linodelb",
			true,
			nil,
		},
		{
			"Service exists but no Load balancer",
			svc2,
			"linodelb",
			false,
			nil,
		},
		{
			"Service does not exists",
			svc3,
			"linodelb",
			false,
			nil,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			_, found, err := lb.GetLoadBalancer(t.Context(), test.clusterName, test.service)
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
			ann:  map[string]string{annotations.AnnLinodePortConfigPrefix + "443": `{ "tls-secret-name": "prod-app-tls", "protocol": "https" }`},
			expected: portConfigAnnotation{
				TLSSecretName: "prod-app-tls",
				Protocol:      "https",
			},
			err: "",
		},
		{
			name: "Test multiple port annotation",
			ann: map[string]string{
				annotations.AnnLinodePortConfigPrefix + "443": `{ "tls-secret-name": "prod-app-tls", "protocol": "https" }`,
				annotations.AnnLinodePortConfigPrefix + "80":  `{ "protocol": "http" }`,
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
				annotations.AnnLinodePortConfigPrefix + "443": `{ "tls-secret-name": "prod-app-tls" `,
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
			cert: testCert,
			key:  testKey,
			err:  nil,
		},
		{
			name: "Test unspecified Cert info",
			portConfig: portConfig{
				Port: 8080,
			},
			cert: "",
			key:  "",
			err:  fmt.Errorf("TLS secret name for port 8080 is not specified"),
		},
		{
			name: "Test blank Cert info",
			portConfig: portConfig{
				TLSSecretName: "",
				Port:          8080,
			},
			cert: "",
			key:  "",
			err:  fmt.Errorf("TLS secret name for port 8080 is not specified"),
		},
		{
			name: "Test no secret found",
			portConfig: portConfig{
				TLSSecretName: "secret",
				Port:          8080,
			},
			cert: "",
			key:  "",
			err: errors.NewNotFound(schema.GroupResource{
				Group:    "",
				Resource: "secrets",
			}, "secret"), /*{}(`secrets "secret" not found`)*/
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			cert, key, err := getTLSCertInfo(t.Context(), kubeClient, "", test.portConfig)
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
	t.Helper()

	_, err := kubeClient.CoreV1().Secrets("").Create(t.Context(), &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tls-secret",
		},
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte(testCert),
			v1.TLSPrivateKeyKey: []byte(testKey),
		},
		StringData: nil,
		Type:       "kubernetes.io/tls",
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to add TLS secret: %s\n", err)
	}
}

func Test_LoadbalNodeNameCoercion(t *testing.T) {
	type testCase struct {
		nodeName       string
		padding        string
		expectedOutput string
	}
	testCases := []testCase{
		{
			nodeName:       "n",
			padding:        "z",
			expectedOutput: "zzn",
		},
		{
			nodeName:       "n",
			padding:        "node-",
			expectedOutput: "node-n",
		},
		{
			nodeName:       "n",
			padding:        "",
			expectedOutput: "xxn",
		},
		{
			nodeName:       "infra-logging-controlplane-3-atl1-us-prod",
			padding:        "node-",
			expectedOutput: "infra-logging-controlplane-3-atl",
		},
		{
			nodeName:       "node1",
			padding:        "node-",
			expectedOutput: "node1",
		},
	}

	for _, tc := range testCases {
		if out := coerceString(tc.nodeName, 3, 32, tc.padding); out != tc.expectedOutput {
			t.Fatalf("Expected loadbal backend name to be %s (got: %s)", tc.expectedOutput, out)
		}
	}
}

func Test_loadbalancers_GetLinodeNBType(t *testing.T) {
	type fields struct {
		client           client.Client
		zone             string
		kubeClient       kubernetes.Interface
		ciliumClient     ciliumclient.CiliumV2alpha1Interface
		loadBalancerType string
	}
	type args struct {
		service *v1.Service
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		defaultNB linodego.NodeBalancerPlanType
		want      linodego.NodeBalancerPlanType
	}{
		{
			name: "No annotation in service and common as default",
			fields: fields{
				client:           nil,
				zone:             "",
				kubeClient:       nil,
				ciliumClient:     nil,
				loadBalancerType: "nodebalancer",
			},
			args: args{
				service: &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "test",
						Annotations: map[string]string{},
					},
				},
			},
			defaultNB: linodego.NBTypeCommon,
			want:      linodego.NBTypeCommon,
		},
		{
			name: "No annotation in service and premium as default",
			fields: fields{
				client:           nil,
				zone:             "",
				kubeClient:       nil,
				ciliumClient:     nil,
				loadBalancerType: "nodebalancer",
			},
			args: args{
				service: &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "test",
						Annotations: map[string]string{},
					},
				},
			},
			defaultNB: linodego.NBTypePremium,
			want:      linodego.NBTypePremium,
		},
		{
			name: "Nodebalancer type annotation in service",
			fields: fields{
				client:           nil,
				zone:             "",
				kubeClient:       nil,
				ciliumClient:     nil,
				loadBalancerType: "nodebalancer",
			},
			args: args{
				service: &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
						Annotations: map[string]string{
							annotations.AnnLinodeNodeBalancerType: string(linodego.NBTypePremium),
						},
					},
				},
			},
			defaultNB: linodego.NBTypeCommon,
			want:      linodego.NBTypePremium,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &loadbalancers{
				client:           tt.fields.client,
				zone:             tt.fields.zone,
				kubeClient:       tt.fields.kubeClient,
				ciliumClient:     tt.fields.ciliumClient,
				loadBalancerType: tt.fields.loadBalancerType,
			}
			options.Options.DefaultNBType = string(tt.defaultNB)
			if got := l.GetLinodeNBType(tt.args.service); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("loadbalancers.GetLinodeNBType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_validateNodeBalancerBackendIPv4Range(t *testing.T) {
	type args struct {
		backendIPv4Range string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "Valid IPv4 range",
			args:    args{backendIPv4Range: "10.100.0.0/30"},
			wantErr: false,
		},
		{
			name:    "Invalid IPv4 range",
			args:    args{backendIPv4Range: "10.100.0.0"},
			wantErr: true,
		},
	}

	nbBackendSubnet := options.Options.NodeBalancerBackendIPv4Subnet
	defer func() {
		options.Options.NodeBalancerBackendIPv4Subnet = nbBackendSubnet
	}()
	options.Options.NodeBalancerBackendIPv4Subnet = "10.100.0.0/24"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateNodeBalancerBackendIPv4Range(tt.args.backendIPv4Range); (err != nil) != tt.wantErr {
				t.Errorf("validateNodeBalancerBackendIPv4Range() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

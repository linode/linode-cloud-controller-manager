package firewall

import (
	"reflect"
	"testing"

	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
)

// makeOldRuleSet constructs a FirewallRuleSet with the given IPs, ports string, and policy.
func makeOldRuleSet(ipList []string, ports string, policy string) linodego.FirewallRuleSet {
	ips := linodego.NetworkAddresses{IPv4: &ipList}
	rule := linodego.FirewallRule{
		Protocol:  "TCP",
		Ports:     ports,
		Addresses: ips,
	}
	return linodego.FirewallRuleSet{
		InboundPolicy: policy,
		Inbound:       []linodego.FirewallRule{rule},
	}
}

func TestRuleChanged(t *testing.T) {
	tests := []struct {
		name       string
		oldIPs     []string
		oldPorts   string
		policy     string
		newACL     aclConfig
		svcPorts   []int32
		wantChange bool
	}{
		{
			name:       "NoChange",
			oldIPs:     []string{"1.2.3.4/32"},
			oldPorts:   "80,8080",
			policy:     drop,
			newACL:     aclConfig{AllowList: &linodego.NetworkAddresses{IPv4: &[]string{"1.2.3.4/32"}}},
			svcPorts:   []int32{80, 8080},
			wantChange: false,
		},
		{
			name:       "IPChange",
			oldIPs:     []string{"1.2.3.4/32"},
			oldPorts:   "80",
			policy:     drop,
			newACL:     aclConfig{AllowList: &linodego.NetworkAddresses{IPv4: &[]string{"5.6.7.8/32"}}},
			svcPorts:   []int32{80},
			wantChange: true,
		},
		{
			name:       "PortChange",
			oldIPs:     []string{"1.2.3.4/32"},
			oldPorts:   "80",
			policy:     drop,
			newACL:     aclConfig{AllowList: &linodego.NetworkAddresses{IPv4: &[]string{"1.2.3.4/32"}}},
			svcPorts:   []int32{80, 8080},
			wantChange: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			old := makeOldRuleSet(tc.oldIPs, tc.oldPorts, tc.policy)
			svc := &v1.Service{Spec: v1.ServiceSpec{Ports: func() []v1.ServicePort {
				ps := make([]v1.ServicePort, len(tc.svcPorts))
				for i, p := range tc.svcPorts {
					ps[i] = v1.ServicePort{Port: p}
				}
				return ps
			}()}}
			got := ruleChanged(old, tc.newACL, svc)
			if got != tc.wantChange {
				t.Errorf("ruleChanged() = %v, want %v", got, tc.wantChange)
			}
		})
	}
}

func TestParsePorts(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []int32
		wantErr bool
	}{
		{"ValidSingle", "80", []int32{80}, false},
		{"ValidMultiple", "80,443", []int32{80, 443}, false},
		{"ValidRange", "100-102", []int32{100, 101, 102}, false},
		{"Combined", "80,100-102,8080", []int32{80, 100, 101, 102, 8080}, false},
		{"Spaces", " 80 ,  443-445 ", []int32{80, 443, 444, 445}, false},
		{"InvalidRangeFormat", "1-2-3", nil, true},
		{"InvalidRangeFormat2", "100-", nil, true},
		{"NonNumeric", "abc", nil, true},
		{"NonNumeric2", "80,a", nil, true},
		{"NonNumeric3", "a-100", nil, true},
		{"StartGreaterThanEnd", "200-100", nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePorts(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parsePorts(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
			if !tc.wantErr && !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parsePorts(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

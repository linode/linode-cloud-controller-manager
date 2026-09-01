package services

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/linode/linodego/v2"
	v1 "k8s.io/api/core/v1"
)

// makeOldRuleSet constructs a FirewallRuleSet with the given IPs, ports string, and policy.
func makeOldRuleSet(ipList []string, ports string, policy string) linodego.FirewallRules {
	ips := linodego.NetworkAddresses{IPv4: ipList}
	rule := linodego.FirewallRuleInbound{
		Protocol:  "TCP",
		Ports:     ports,
		Addresses: ips,
	}
	return linodego.FirewallRules{
		InboundPolicy: policy,
		Inbound:       []linodego.FirewallRuleInbound{rule},
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
			newACL:     aclConfig{AllowList: &linodego.NetworkAddresses{IPv4: []string{"1.2.3.4/32"}}},
			svcPorts:   []int32{80, 8080},
			wantChange: false,
		},
		{
			name:       "IPChange",
			oldIPs:     []string{"1.2.3.4/32"},
			oldPorts:   "80",
			policy:     drop,
			newACL:     aclConfig{AllowList: &linodego.NetworkAddresses{IPv4: []string{"5.6.7.8/32"}}},
			svcPorts:   []int32{80},
			wantChange: true,
		},
		{
			name:       "PortChange",
			oldIPs:     []string{"1.2.3.4/32"},
			oldPorts:   "80",
			policy:     drop,
			newACL:     aclConfig{AllowList: &linodego.NetworkAddresses{IPv4: []string{"1.2.3.4/32"}}},
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
			got := ruleChanged(&old, tc.newACL, svc)
			if got != tc.wantChange {
				t.Errorf("ruleChanged() = %v, want %v", got, tc.wantChange)
			}
		})
	}
}

// generateCIDRs builds count distinct CIDRs using the given per-index formatter,
// e.g. an IPv4 /32 or an IPv6 /128 formatter.
func generateCIDRs(count int, format func(i int) string) []string {
	ips := make([]string, count)
	for i := range ips {
		ips[i] = format(i)
	}
	return ips
}

// TestProcessACLNoEmptyRuleForMissingFamily is a regression test for a bug where
// processACL created an empty inbound rule for the IP family with no addresses
// when the other family exceeded maxIPsPerFirewall (255), because chunkIPs
// returns a single chunk for an empty slice. It exercises both single-family
// cases (IPv4-only and the symmetric IPv6-only case) with 256 addresses, which
// must be split into exactly two chunks: 255 and 1.
func TestProcessACLNoEmptyRuleForMissingFamily(t *testing.T) {
	const addrCount = 256
	wantChunkSizes := []int{255, 1}

	tests := []struct {
		name string
		ips  linodego.NetworkAddresses
	}{
		{
			name: "IPv4Only",
			ips: linodego.NetworkAddresses{
				IPv4: generateCIDRs(addrCount, func(i int) string { return fmt.Sprintf("10.0.%d.1/32", i) }),
			},
		},
		{
			name: "IPv6Only",
			ips: linodego.NetworkAddresses{
				IPv6: generateCIDRs(addrCount, func(i int) string { return fmt.Sprintf("2001:db8::%x/128", i) }),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fwcreateOpts := &linodego.FirewallCreateOptions{}
			err := processACL(fwcreateOpts, accept, "test", "svc", "80", tt.ips)
			if err != nil {
				t.Fatalf("processACL() error = %v", err)
			}

			if len(fwcreateOpts.Rules.Inbound) != len(wantChunkSizes) {
				t.Fatalf("processACL() created %d inbound rules, want %d", len(fwcreateOpts.Rules.Inbound), len(wantChunkSizes))
			}

			for i, rule := range fwcreateOpts.Rules.Inbound {
				if len(rule.Addresses.IPv4) == 0 && len(rule.Addresses.IPv6) == 0 {
					t.Errorf("processACL() created an inbound rule with no addresses: %+v", rule)
				}

				if gotSize := len(rule.Addresses.IPv4) + len(rule.Addresses.IPv6); gotSize != wantChunkSizes[i] {
					t.Errorf("rule %d has %d addresses, want %d", i, gotSize, wantChunkSizes[i])
				}

				if len(tt.ips.IPv4) > 0 && len(rule.Addresses.IPv6) != 0 {
					t.Errorf("rule %d unexpectedly has IPv6 addresses for an IPv4-only ACL: %+v", i, rule)
				}
				if len(tt.ips.IPv6) > 0 && len(rule.Addresses.IPv4) != 0 {
					t.Errorf("rule %d unexpectedly has IPv4 addresses for an IPv6-only ACL: %+v", i, rule)
				}
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

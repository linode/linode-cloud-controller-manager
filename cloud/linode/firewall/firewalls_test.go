package firewall

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/linode/linodego"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/linode/linode-cloud-controller-manager/cloud/annotations"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client/mocks"
)

func dummyNode(anns map[string]string) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-node",
			Annotations: anns,
		},
	}
}

func dummyInst() *linodego.Instance {
	return &linodego.Instance{ID: 111, Label: "test-node"}
}

func TestFirewall(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)

	t.Run("Create Firewall", func(t *testing.T) {
		fwClient := NewFirewalls(client)

		createOpts := linodego.FirewallCreateOptions{
			Rules: linodego.FirewallRuleSet{
				Inbound: []linodego.FirewallRule{{
					Action:      "ACCEPT",
					Label:       "inbound-rule123",
					Description: "inbound rule123",
					Ports:       "4321",
					Protocol:    linodego.TCP,
					Addresses: linodego.NetworkAddresses{
						IPv4: &[]string{"0.0.0.0/0"},
					},
				}},
				Outbound:       []linodego.FirewallRule{},
				InboundPolicy:  "ACCEPT",
				OutboundPolicy: "ACCEPT",
			},
		}
		expectedFirewall := &linodego.Firewall{}
		client.EXPECT().CreateFirewall(gomock.Any(), createOpts).Times(1).Return(expectedFirewall, nil)
		_, err := fwClient.CreateFirewall(context.TODO(), createOpts)
		if err != nil {
			t.Errorf("CreatingFirewall returned an error: %s", err)
		}
	})

	t.Run("Delete Firewall", func(t *testing.T) {
		fwClient := NewFirewalls(client)

		firewall := &linodego.Firewall{ID: 123}
		client.EXPECT().DeleteFirewall(gomock.Any(), 123).Times(1).Return(nil)
		err := fwClient.DeleteFirewall(context.TODO(), firewall)
		if err != nil {
			t.Errorf("DeleteFirewall returned an error: %s", err)
		}
	})

	t.Run("Create Node without firewall ID or ACL without existing firewalls", func(t *testing.T) {
		fwClient := NewFirewalls(client)
		anns := map[string]string{}
		node := dummyNode(anns)
		inst := dummyInst()

		client.EXPECT().ListInstanceFirewalls(gomock.Any(), inst.ID, gomock.Any()).Times(1).Return([]linodego.Firewall{}, nil)

		err := fwClient.UpdateNodeFirewall(context.TODO(), node, inst)
		assert.NoError(t, err)
	})

	t.Run("Create Node without firewall ID or ACL with existing firewall without deleting old firewall", func(t *testing.T) {
		fwClient := NewFirewalls(client)
		anns := map[string]string{}
		node := dummyNode(anns)
		inst := dummyInst()
		existingFWID := 333
		existingDeviceID := 555

		client.EXPECT().ListInstanceFirewalls(gomock.Any(), inst.ID, gomock.Any()).Times(1).Return([]linodego.Firewall{
			{ID: existingFWID, Label: "existing"},
		}, nil)
		client.EXPECT().ListFirewallDevices(gomock.Any(), existingFWID, gomock.Any()).Times(1).Return([]linodego.FirewallDevice{
			{
				ID:     existingDeviceID,
				Entity: linodego.FirewallDeviceEntity{ID: inst.ID, Type: linodego.FirewallDeviceLinode},
			}, {
				ID:     444,
				Entity: linodego.FirewallDeviceEntity{ID: 1234, Type: linodego.FirewallDeviceLinode},
			},
		}, nil)
		client.EXPECT().DeleteFirewallDevice(gomock.Any(), existingFWID, existingDeviceID).Times(1).Return(nil)
		client.EXPECT().ListFirewallDevices(gomock.Any(), existingFWID, gomock.Any()).Times(1).Return([]linodego.FirewallDevice{{
			ID:     444,
			Entity: linodego.FirewallDeviceEntity{ID: 1234, Type: linodego.FirewallDeviceLinode},
		}}, nil)

		err := fwClient.UpdateNodeFirewall(context.TODO(), node, inst)
		assert.NoError(t, err)
	})

	t.Run("Create Node without firewall ID or ACL without existing firewall", func(t *testing.T) {
		fwClient := NewFirewalls(client)
		anns := map[string]string{}
		node := dummyNode(anns)
		inst := dummyInst()

		client.EXPECT().ListInstanceFirewalls(gomock.Any(), inst.ID, gomock.Any()).Times(1).Return([]linodego.Firewall{}, nil)

		err := fwClient.UpdateNodeFirewall(context.TODO(), node, inst)
		assert.NoError(t, err)
	})

	t.Run("Create Node with firewall ID already attached to existing firewall and cleanup old firewall", func(t *testing.T) {
		fwClient := NewFirewalls(client)
		fwID := 123
		attachedFWID := 444
		existingDeviceID := 777
		anns := map[string]string{
			annotations.AnnLinodeNodeFirewallID: strconv.Itoa(fwID),
		}
		node := dummyNode(anns)
		inst := dummyInst()

		client.EXPECT().ListInstanceFirewalls(gomock.Any(), inst.ID, gomock.Any()).Times(1).Return(
			[]linodego.Firewall{{ID: attachedFWID}},
			nil,
		)
		client.EXPECT().ListFirewallDevices(gomock.Any(), attachedFWID, gomock.Any()).Times(1).Return(
			[]linodego.FirewallDevice{
				{ID: existingDeviceID, Entity: linodego.FirewallDeviceEntity{ID: inst.ID}},
			},
			nil,
		)
		client.EXPECT().CreateFirewallDevice(gomock.Any(), fwID, gomock.Any()).Times(1).Return(
			&linodego.FirewallDevice{
				ID: 5675, Entity: linodego.FirewallDeviceEntity{ID: inst.ID},
			},
			nil,
		)
		client.EXPECT().DeleteFirewallDevice(gomock.Any(), attachedFWID, existingDeviceID).Times(1).Return(nil)
		client.EXPECT().ListFirewallDevices(gomock.Any(), attachedFWID, gomock.Any()).Times(1).Return(
			[]linodego.FirewallDevice{},
			nil,
		)
		client.EXPECT().DeleteFirewall(gomock.Any(), attachedFWID).Times(1).Return(nil)

		err := fwClient.UpdateNodeFirewall(context.TODO(), node, inst)
		assert.NoError(t, err)
	})

	t.Run("Create Node with firewall ACL Allow List", func(t *testing.T) {
		fwClient := NewFirewalls(client)
		fwID := 123
		anns := map[string]string{
			annotations.AnnLinodeNodeFirewallACL: `{
	   			"allowList": {
	   				"ipv4": ["2.2.2.2"]
	   			}
	   		}`,
		}
		node := dummyNode(anns)
		inst := dummyInst()

		client.EXPECT().ListInstanceFirewalls(gomock.Any(), inst.ID, gomock.Any()).Times(1).Return([]linodego.Firewall{}, nil)
		client.EXPECT().CreateFirewall(gomock.Any(), gomock.Any()).Times(1).Return(&linodego.Firewall{
			ID: fwID,
		}, nil)
		client.EXPECT().CreateFirewallDevice(gomock.Any(), fwID, gomock.Any()).Times(1).Return(&linodego.FirewallDevice{
			ID: inst.ID,
		}, nil)

		err := fwClient.UpdateNodeFirewall(context.TODO(), node, inst)
		assert.NoError(t, err)
	})

	t.Run("Create Node with too many firewall rules in ACL Allow List", func(t *testing.T) {
		ipv4addrs := []string{}
		for i := 0; i < 255; i++ {
			for j := 0; j < 254; j++ {
				ipv4addrs = append(ipv4addrs, fmt.Sprintf("\"192.168.%d.%d\"", i, j))
			}
		}
		ipv4 := strings.Join(ipv4addrs[:], ",")
		fwClient := NewFirewalls(client)
		anns := map[string]string{
			annotations.AnnLinodeNodeFirewallACL: fmt.Sprintf(`{
	   			"allowList": {
	   				"ipv4": [%s]
	   			}
	   		}`, ipv4),
		}
		node := dummyNode(anns)
		inst := dummyInst()

		client.EXPECT().ListInstanceFirewalls(gomock.Any(), inst.ID, gomock.Any()).Times(1).Return([]linodego.Firewall{}, nil)

		err := fwClient.UpdateNodeFirewall(context.TODO(), node, inst)
		// failed to create firewall options: too many IPs in this ACL, will exceed rules per firewall limit
		assert.Error(t, err)
	})

	t.Run("Create Node with firewall ACL Deny List", func(t *testing.T) {
		fwClient := NewFirewalls(client)
		fwID := 123
		anns := map[string]string{
			annotations.AnnLinodeNodeFirewallACL: `{
	   			"denyList": {
	   				"ipv4": ["2.2.2.2"]
	   			}
	   		}`,
		}
		node := dummyNode(anns)
		inst := dummyInst()

		client.EXPECT().ListInstanceFirewalls(gomock.Any(), inst.ID, gomock.Any()).Times(1).Return([]linodego.Firewall{}, nil)
		client.EXPECT().CreateFirewall(gomock.Any(), gomock.Any()).Times(1).Return(&linodego.Firewall{
			ID: fwID,
		}, nil)
		client.EXPECT().CreateFirewallDevice(gomock.Any(), fwID, gomock.Any()).Times(1).Return(&linodego.FirewallDevice{
			ID: inst.ID,
		}, nil)

		err := fwClient.UpdateNodeFirewall(context.TODO(), node, inst)
		assert.NoError(t, err)
	})

	t.Run("Create Node with firewall ACL Allow List and Ports", func(t *testing.T) {
		fwClient := NewFirewalls(client)
		fwID := 123
		anns := map[string]string{
			annotations.AnnLinodeNodeFirewallACL: `{
	   			"allowList": {
          			"ipv4": ["192.166.0.0/16", "172.23.41.0/24"],
          			"ipv6": ["2001:DB8::/128"]
	   			},
        		"ports": ["22","6443"]
	   		}`,
		}
		node := dummyNode(anns)
		inst := dummyInst()

		client.EXPECT().ListInstanceFirewalls(gomock.Any(), inst.ID, gomock.Any()).Times(1).Return([]linodego.Firewall{}, nil)
		client.EXPECT().CreateFirewall(gomock.Any(), gomock.Any()).Times(1).Return(&linodego.Firewall{
			ID: fwID,
		}, nil)
		client.EXPECT().CreateFirewallDevice(gomock.Any(), fwID, gomock.Any()).Times(1).Return(&linodego.FirewallDevice{
			ID: inst.ID,
		}, nil)

		err := fwClient.UpdateNodeFirewall(context.TODO(), node, inst)
		assert.NoError(t, err)
	})

	t.Run("Create Node with firewall ACL Allow and Deny List", func(t *testing.T) {
		fwClient := NewFirewalls(client)
		anns := map[string]string{
			annotations.AnnLinodeNodeFirewallACL: `{
	   			"allowList": {
          			"ipv4": ["192.166.0.0/16", "172.23.41.0/24"],
          			"ipv6": ["2001:DB8::/128"]
	   			},
				"denyList": {
	   				"ipv4": ["2.2.2.2"]
	   			},
        		"ports": ["22","6443"]
	   		}`,
		}
		node := dummyNode(anns)
		inst := dummyInst()

		client.EXPECT().ListInstanceFirewalls(gomock.Any(), inst.ID, gomock.Any()).Times(1).Return([]linodego.Firewall{}, nil)

		err := fwClient.UpdateNodeFirewall(context.TODO(), node, inst)
		// failed to create firewall options: specify either an allowList or a denyList for a firewall
		assert.Error(t, err)
	})

	t.Run("Create Node without firewall ACL Empty", func(t *testing.T) {
		fwClient := NewFirewalls(client)
		anns := map[string]string{
			annotations.AnnLinodeNodeFirewallACL: `{}`,
		}
		node := dummyNode(anns)
		inst := dummyInst()

		client.EXPECT().ListInstanceFirewalls(gomock.Any(), inst.ID, gomock.Any()).Times(1).Return([]linodego.Firewall{}, nil)

		err := fwClient.UpdateNodeFirewall(context.TODO(), node, inst)
		// failed to create firewall options: specify either an allowList or a denyList for a firewall
		assert.Error(t, err)
	})

	t.Run("Update existing Node firewall", func(t *testing.T) {
		fwClient := NewFirewalls(client)
		anns := map[string]string{
			annotations.AnnLinodeNodeFirewallACL: `{
	   			"allowList": {
          			"ipv4": ["192.166.0.0/16", "172.23.41.0/24"],
          			"ipv6": ["2001:DB8::/128"]
	   			},
        		"ports": ["22","6443"]
	   		}`,
		}
		node := dummyNode(anns)
		inst := dummyInst()

		existingFWID := 123
		existingRules := linodego.FirewallRuleSet{
			Inbound: []linodego.FirewallRule{{
				Action:      "DROP",
				Label:       "inbound-rule123",
				Description: "inbound rule123",
				Ports:       "4321",
				Protocol:    linodego.TCP,
				Addresses: linodego.NetworkAddresses{
					IPv4: &[]string{"0.0.0.0/0"},
					IPv6: &[]string{"::/0"},
				},
			}},
			Outbound:       []linodego.FirewallRule{},
			InboundPolicy:  "DROP",
			OutboundPolicy: "ACCEPT",
		}
		client.EXPECT().ListInstanceFirewalls(gomock.Any(), inst.ID, gomock.Any()).Times(1).Return([]linodego.Firewall{{
			ID:    existingFWID,
			Rules: existingRules,
		}}, nil)

		client.EXPECT().UpdateFirewallRules(gomock.Any(), existingFWID, gomock.Any()).Times(1)

		err := fwClient.UpdateNodeFirewall(context.TODO(), node, inst)
		assert.NoError(t, err)
	})
}

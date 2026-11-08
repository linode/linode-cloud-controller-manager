package services

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/linode/linodego/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/linode/linode-cloud-controller-manager/cloud/annotations"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client/mocks"
)

// devicesWithIDCollision returns the devices of a firewall that protects both a Linode and a
// NodeBalancer which happen to share the same entity ID.
func devicesWithIDCollision(entityID, linodeDeviceID, nbDeviceID int) []linodego.FirewallDevice {
	return []linodego.FirewallDevice{
		{ID: linodeDeviceID, Entity: linodego.FirewallDeviceEntity{ID: entityID, Type: linodego.FirewallDeviceLinode}},
		{ID: nbDeviceID, Entity: linodego.FirewallDeviceEntity{ID: entityID, Type: linodego.FirewallDeviceNodeBalancer}},
	}
}

func TestGetNodeBalancerDeviceIDIgnoresOtherEntityTypes(t *testing.T) {
	const (
		firewallID     = 1
		nbID           = 4242
		linodeDeviceID = 10
		nbDeviceID     = 11
	)

	t.Run("matches the nodebalancer device when a linode has the same ID", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		client := mocks.NewMockClient(ctrl)
		client.EXPECT().ListFirewallDevices(gomock.Any(), firewallID, gomock.Any()).Return(devicesWithIDCollision(nbID, linodeDeviceID, nbDeviceID), nil)

		l := &LinodeClient{Client: client}
		deviceID, exists, err := l.getNodeBalancerDeviceID(t.Context(), firewallID, nbID)
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, nbDeviceID, deviceID)
	})

	t.Run("does not match a linode or linode interface with the nodebalancer ID", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		client := mocks.NewMockClient(ctrl)
		client.EXPECT().ListFirewallDevices(gomock.Any(), firewallID, gomock.Any()).Return([]linodego.FirewallDevice{
			{ID: linodeDeviceID, Entity: linodego.FirewallDeviceEntity{ID: nbID, Type: linodego.FirewallDeviceLinode}},
			{ID: nbDeviceID, Entity: linodego.FirewallDeviceEntity{ID: nbID, Type: linodego.FirewallDeviceLinodeInterface}},
		}, nil)

		l := &LinodeClient{Client: client}
		_, exists, err := l.getNodeBalancerDeviceID(t.Context(), firewallID, nbID)
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestReplaceNBFirewallKeepsLinodeWithSameID(t *testing.T) {
	const (
		oldFirewallID  = 1
		newFirewallID  = 2
		nbID           = 4242
		linodeDeviceID = 10
		nbDeviceID     = 11
	)

	ctrl := gomock.NewController(t)
	client := mocks.NewMockClient(ctrl)
	client.EXPECT().ListNodeBalancerFirewalls(gomock.Any(), nbID, gomock.Any()).Return([]linodego.Firewall{{ID: oldFirewallID}}, nil)
	client.EXPECT().CreateFirewallDevice(gomock.Any(), newFirewallID, linodego.FirewallDeviceCreateOptions{ID: nbID, Type: "nodebalancer"}).Return(&linodego.FirewallDevice{}, nil)
	client.EXPECT().ListFirewallDevices(gomock.Any(), oldFirewallID, gomock.Any()).Return(devicesWithIDCollision(nbID, linodeDeviceID, nbDeviceID), nil)
	// only the nodebalancer leaves the old firewall, the linode stays protected by it
	client.EXPECT().DeleteFirewallDevice(gomock.Any(), oldFirewallID, nbDeviceID).Return(nil)

	svc := &v1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:        "svc",
		Annotations: map[string]string{annotations.AnnLinodeCloudFirewallID: "2"},
	}}
	l := &LinodeClient{Client: client}
	err := l.UpdateNodeBalancerFirewall(t.Context(), "lb", nil, svc, &linodego.NodeBalancer{ID: nbID})
	require.NoError(t, err)
}

func TestRemoveNBFirewallKeepsLinodeWithSameID(t *testing.T) {
	const (
		firewallID     = 1
		nbID           = 4242
		linodeDeviceID = 10
		nbDeviceID     = 11
	)

	ctrl := gomock.NewController(t)
	client := mocks.NewMockClient(ctrl)
	client.EXPECT().ListNodeBalancerFirewalls(gomock.Any(), nbID, gomock.Any()).Return([]linodego.Firewall{{ID: firewallID}}, nil)
	gomock.InOrder(
		client.EXPECT().ListFirewallDevices(gomock.Any(), firewallID, gomock.Any()).Return(devicesWithIDCollision(nbID, linodeDeviceID, nbDeviceID), nil),
		client.EXPECT().DeleteFirewallDevice(gomock.Any(), firewallID, nbDeviceID).Return(nil),
		client.EXPECT().ListFirewallDevices(gomock.Any(), firewallID, gomock.Any()).Return(devicesWithIDCollision(nbID, linodeDeviceID, nbDeviceID)[:1], nil),
	)
	// the linode is still attached, so the firewall is neither deleted nor touched further

	svc := &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc"}}
	l := &LinodeClient{Client: client}
	err := l.UpdateNodeBalancerFirewall(t.Context(), "lb", nil, svc, &linodego.NodeBalancer{ID: nbID})
	require.NoError(t, err)
}

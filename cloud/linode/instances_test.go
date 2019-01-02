package linode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/linode/linodego"
	"k8s.io/api/core/v1"
)

func TestCCMInstances(t *testing.T) {
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
			name: "Instances Init",
			f:    testNewInstances,
		},
		{
			name: "Node Addresses",
			f:    testNodeAddresses,
		},
		{
			name: "Node Addresses by ProviderID",
			f:    testNodeAddressesByProviderID,
		},
		{
			name: "Intance ID",
			f:    testInstanceID,
		},
		{
			name: "Instance Type",
			f:    testInstanceType,
		},
		{
			name: "Instance Type by ProviderID",
			f:    testInstanceTypeByProviderID,
		},
		{
			name: "Instance Exists By Provider ID",
			f:    testInstanceExistsByProviderID,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.f(t, &linodeClient)
		})
	}
}

func testNewInstances(t *testing.T, client *linodego.Client) {
	linodeInstances := newInstances(client)
	if linodeInstances.(*instances).client != client {
		t.Errorf("instances not initialized")
	}
}

func testNodeAddresses(t *testing.T, client *linodego.Client) {
	testCases := []struct {
		name string
		f    func(*testing.T, *linodego.Client)
	}{
		{
			name: "Node Addresses Found",
			f:    testNodeAddressesFound,
		},
		{
			name: "Node Addresses Not Found",
			f:    testNodeAddressesNotFound,
		},
		// TODO: Add test for the failure mode of multiple Linodes returned for the
		// same label. The API should prevent this but we must handle it gracefully on
		// the Kubernetes side.
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.f(t, client)
		})
	}
}

func testNodeAddressesFound(t *testing.T, client *linodego.Client) {
	expectedAddresses := []v1.NodeAddress{
		{
			Type:    v1.NodeHostName,
			Address: "test-instance",
		},
		{
			Type:    v1.NodeExternalIP,
			Address: "45.79.101.25",
		},
		{
			Type:    v1.NodeInternalIP,
			Address: "192.168.133.65",
		},
	}

	instances := newInstances(client)

	addresses, err := instances.NodeAddresses(context.TODO(), "test-instance")
	if !reflect.DeepEqual(addresses, expectedAddresses) {
		t.Errorf("unexpected node addresses. got: %v want: %v", addresses, expectedAddresses)
	}

	if err != nil {
		t.Errorf("unexpected err, expected nil. got: %v", err)
	}
}

func testNodeAddressesNotFound(t *testing.T, client *linodego.Client) {
	instances := newInstances(client)

	_, err := instances.NodeAddresses(context.TODO(), "non-existant")
	if err == nil {
		t.Errorf("Expected error for nonexistent Linode label")
	}
}

func testNodeAddressesByProviderID(t *testing.T, client *linodego.Client) {
	testCases := []struct {
		name string
		f    func(*testing.T, *linodego.Client)
	}{
		{
			name: "Node Addresses by ProviderID Found",
			f:    testNodeAddressesByProviderIDFound,
		},
		{
			name: "Node Addresses by ProviderID Bad LinodeID",
			f:    testNodeAddressesByProviderIDBadLinodeID,
		},
		{
			name: "Node Addresses by ProviderID Bad ProviderName",
			f:    testNodeAddressesByProviderIDBadProviderName,
		},
		{
			name: "Node Addresses by ProviderID Bad Format",
			f:    testNodeAddressesByProviderIDBadProviderIDFormat,
		},
		{
			name: "Node Addresses by ProviderID Empty ProviderID",
			f:    testNodeAddressesByProviderIDEmptyProviderID,
		},
		// TODO: Add a test for a Linode which returns no IP addresses. This should be
		// prevented by the API but we must handle it gracefully on the Kubernetes
		// side.
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.f(t, client)
		})
	}
}

func testNodeAddressesByProviderIDFound(t *testing.T, client *linodego.Client) {
	expectedAddresses := []v1.NodeAddress{
		{
			Type:    v1.NodeHostName,
			Address: "test-instance",
		},
		{
			Type:    v1.NodeExternalIP,
			Address: "45.79.101.25",
		},
		{
			Type:    v1.NodeInternalIP,
			Address: "192.168.133.65",
		},
	}

	instances := newInstances(client)

	addresses, err := instances.NodeAddressesByProviderID(context.TODO(), "linode://123")
	if !reflect.DeepEqual(addresses, expectedAddresses) {
		t.Errorf("unexpected node addresses. got: %v want: %v", addresses, expectedAddresses)
	}

	if err != nil {
		t.Errorf("unexpected err, expected nil. got: %v", err)
	}
}

func testNodeAddressesByProviderIDBadLinodeID(t *testing.T, client *linodego.Client) {
	instances := newInstances(client)

	_, err := instances.NodeAddressesByProviderID(context.TODO(), "linode://456")
	if err == nil {
		t.Errorf("Expected error for nonexistent LinodeID")
	}
}

func testNodeAddressesByProviderIDBadProviderName(t *testing.T, client *linodego.Client) {
	instances := newInstances(client)

	_, err := instances.NodeAddressesByProviderID(context.TODO(), "thecloud://123")
	if err == nil {
		t.Errorf("Expected error for bad ProviderID")
	}
}

func testNodeAddressesByProviderIDBadProviderIDFormat(t *testing.T, client *linodego.Client) {
	instances := newInstances(client)

	_, err := instances.NodeAddressesByProviderID(context.TODO(), "linode123")
	if err == nil {
		t.Errorf("Expected error for bad ProviderID format")
	}
}

func testNodeAddressesByProviderIDEmptyProviderID(t *testing.T, client *linodego.Client) {
	instances := newInstances(client)

	_, err := instances.NodeAddressesByProviderID(context.TODO(), "")
	if err == nil {
		t.Errorf("Expected error for empty ProviderID")
	}
}

func testInstanceID(t *testing.T, client *linodego.Client) {
	instances := newInstances(client)
	id, err := instances.InstanceID(context.TODO(), "test-instance")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if id != "123" {
		t.Errorf("expected id 123, got %s", id)
	}

	_, err = instances.InstanceID(context.TODO(), "test-linode")
	if err == nil {
		t.Errorf("expected error, got: nil")
	}

}

func testInstanceType(t *testing.T, client *linodego.Client) {
	instances := newInstances(client)

	insType, err := instances.InstanceType(context.TODO(), "test-instance")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if insType != "g6-standard-2" {
		t.Errorf("expected id g6-standard-2, got %s", insType)
	}
}

func testInstanceTypeByProviderID(t *testing.T, client *linodego.Client) {
	instances := newInstances(client)

	insType, err := instances.InstanceTypeByProviderID(context.TODO(), "linode://123")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if insType != "g6-standard-2" {
		t.Errorf("expected id g6-standard-2, got %s", insType)
	}
}

func testInstanceExistsByProviderID(t *testing.T, client *linodego.Client) {
	instances := newInstances(client)

	found, err := instances.InstanceExistsByProviderID(context.TODO(), "linode://123")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if !found {
		t.Errorf("expected found true, got %v", found)
	}

	found, err = instances.InstanceExistsByProviderID(context.TODO(), "linode://12345")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if found {
		t.Errorf("expected found false, got %v", found)
	}

}

package linode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/linode/linodego"
	"k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/cloudprovider"
)

var _ cloudprovider.Instances = new(instances)

func TestCCMInstances(t *testing.T) {
	fake := newFake(t)
	ts := httptest.NewServer(fake)
	defer ts.Close()

	linodeClient := linodego.NewClient(http.DefaultClient)
	linodeClient.SetBaseURL(ts.URL)

	testNodeAddresses(t, &linodeClient)
	testNodeAddressesByProviderID(t, &linodeClient)
	testInstanceID(t, &linodeClient)
	testInstanceType(t, &linodeClient)
	testInstanceTypeByProviderID(t, &linodeClient)
	testInstanceExistsByProviderID(t, &linodeClient)
}

func testNodeAddresses(t *testing.T, client *linodego.Client) {
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

func testNodeAddressesByProviderID(t *testing.T, client *linodego.Client) {
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

func testInstanceID(t *testing.T, client *linodego.Client) {
	instances := newInstances(client)
	id, err := instances.InstanceID(context.TODO(), "test-instance")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if id != "123" {
		t.Errorf("expected id 1234, got %s", id)
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

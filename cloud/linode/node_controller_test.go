package linode

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/linode/linodego"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"

	"github.com/linode/linode-cloud-controller-manager/cloud/annotations"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client/mocks"
)

func TestNodeController_Run(t *testing.T) {
	// Mock dependencies
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)
	kubeClient := fake.NewSimpleClientset()
	informer := informers.NewSharedInformerFactory(kubeClient, 0).Core().V1().Nodes()
	mockQueue := workqueue.NewTypedDelayingQueueWithConfig(workqueue.TypedDelayingQueueConfig[any]{Name: "test"})

	nodeCtrl := newNodeController(kubeClient, client, informer, newInstances(client))
	nodeCtrl.queue = mockQueue
	nodeCtrl.ttl = 1 * time.Second

	// Add test node
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "nodeA",
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: v1.NodeSpec{},
	}
	_, err := kubeClient.CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
	assert.NoError(t, err, "expected no error during node creation")

	// Start the controller
	stopCh := make(chan struct{})
	go nodeCtrl.Run(stopCh)

	client.EXPECT().ListInstances(gomock.Any(), nil).AnyTimes().Return([]linodego.Instance{}, &linodego.Error{Code: http.StatusTooManyRequests, Message: "Too many requests"})
	// Add the node to the informer
	err = nodeCtrl.informer.Informer().GetStore().Add(node)
	assert.NoError(t, err, "expected no error when adding node to informer")

	// Allow some time for the queue to process
	time.Sleep(1 * time.Second)

	// Stop the controller
	close(stopCh)
}

func TestNodeController_processNext(t *testing.T) {
	// Mock dependencies
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)
	kubeClient := fake.NewSimpleClientset()
	queue := workqueue.NewTypedDelayingQueueWithConfig(workqueue.TypedDelayingQueueConfig[any]{Name: "testQueue"})
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test",
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: v1.NodeSpec{},
	}

	_, err := kubeClient.CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
	assert.NoError(t, err, "expected no error during node creation")

	controller := &nodeController{
		kubeclient:         kubeClient,
		instances:          newInstances(client),
		queue:              queue,
		metadataLastUpdate: make(map[string]time.Time),
		ttl:                defaultMetadataTTL,
	}

	t.Run("should return no error on unknown errors", func(t *testing.T) {
		queue.Add(node)
		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{}, errors.New("lookup failed"))
		result := controller.processNext()
		assert.True(t, result, "processNext should return true")
		if queue.Len() != 0 {
			t.Errorf("expected queue to be empty, got %d items", queue.Len())
		}
	})

	t.Run("should return no error if node exists", func(t *testing.T) {
		queue.Add(node)
		publicIP := net.ParseIP("172.234.31.123")
		privateIP := net.ParseIP("192.168.159.135")
		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{
			{ID: 111, Label: "test", IPv4: []*net.IP{&publicIP, &privateIP}, HostUUID: "111"},
		}, nil)
		result := controller.processNext()
		assert.True(t, result, "processNext should return true")
		if queue.Len() != 0 {
			t.Errorf("expected queue to be empty, got %d items", queue.Len())
		}
	})

	t.Run("should return no error if queued object is not of type Node", func(t *testing.T) {
		queue.Add("abc")
		result := controller.processNext()
		assert.True(t, result, "processNext should return true")
		if queue.Len() != 0 {
			t.Errorf("expected queue to be empty, got %d items", queue.Len())
		}
	})

	t.Run("should return no error if node in k8s doesn't exist", func(t *testing.T) {
		queue.Add(node)
		controller.kubeclient = fake.NewSimpleClientset()
		defer func() { controller.kubeclient = kubeClient }()
		result := controller.processNext()
		assert.True(t, result, "processNext should return true")
		if queue.Len() != 0 {
			t.Errorf("expected queue to be empty, got %d items", queue.Len())
		}
	})

	t.Run("should return error and requeue when it gets 429 from linode API", func(t *testing.T) {
		queue = workqueue.NewTypedDelayingQueueWithConfig(workqueue.TypedDelayingQueueConfig[any]{Name: "testQueue1"})
		queue.Add(node)
		controller.queue = queue
		client := mocks.NewMockClient(ctrl)
		controller.instances = newInstances(client)
		retryInterval = 1 * time.Nanosecond
		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{}, &linodego.Error{Code: http.StatusTooManyRequests, Message: "Too many requests"})
		result := controller.processNext()
		time.Sleep(1 * time.Second)
		assert.True(t, result, "processNext should return true")
		if queue.Len() == 0 {
			t.Errorf("expected queue to not be empty, got it empty")
		}
	})

	t.Run("should return error and requeue when it gets error >= 500 from linode API", func(t *testing.T) {
		queue = workqueue.NewTypedDelayingQueueWithConfig(workqueue.TypedDelayingQueueConfig[any]{Name: "testQueue2"})
		queue.Add(node)
		controller.queue = queue
		client := mocks.NewMockClient(ctrl)
		controller.instances = newInstances(client)
		retryInterval = 1 * time.Nanosecond
		client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{}, &linodego.Error{Code: http.StatusInternalServerError, Message: "Too many requests"})
		result := controller.processNext()
		time.Sleep(1 * time.Second)
		assert.True(t, result, "processNext should return true")
		if queue.Len() == 0 {
			t.Errorf("expected queue to not be empty, got it empty")
		}
	})
}

func TestNodeController_handleNode(t *testing.T) {
	// Mock dependencies
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)
	kubeClient := fake.NewSimpleClientset()
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-node",
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: v1.NodeSpec{ProviderID: "linode://123"},
	}
	_, err := kubeClient.CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
	assert.NoError(t, err, "expected no error during node creation")

	instCache := newInstances(client)

	t.Setenv("LINODE_METADATA_TTL", "30")
	nodeCtrl := newNodeController(kubeClient, client, nil, instCache)
	assert.Equal(t, 30*time.Second, nodeCtrl.ttl, "expected ttl to be 30 seconds")

	// Test: Successful metadata update
	publicIP := net.ParseIP("172.234.31.123")
	privateIP := net.ParseIP("192.168.159.135")
	client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{
		{ID: 123, Label: "test-node", IPv4: []*net.IP{&publicIP, &privateIP}, HostUUID: "123"},
	}, nil)
	err = nodeCtrl.handleNode(context.TODO(), node)
	assert.NoError(t, err, "expected no error during handleNode")

	// Check metadataLastUpdate
	lastUpdate := nodeCtrl.LastMetadataUpdate("test-node")
	if time.Since(lastUpdate) > 5*time.Second {
		t.Errorf("metadataLastUpdate was not updated correctly")
	}

	// Annotations set, no update needed as ttl not reached
	node.Labels[annotations.AnnLinodeHostUUID] = "123"
	node.Annotations[annotations.AnnLinodeNodePrivateIP] = privateIP.String()
	err = nodeCtrl.handleNode(context.TODO(), node)
	assert.NoError(t, err, "expected no error during handleNode")

	// Lookup failure for linode instance
	client = mocks.NewMockClient(ctrl)
	nodeCtrl.instances = newInstances(client)
	nodeCtrl.metadataLastUpdate["test-node"] = time.Now().Add(-2 * nodeCtrl.ttl)
	client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{}, errors.New("lookup failed"))
	err = nodeCtrl.handleNode(context.TODO(), node)
	assert.Error(t, err, "expected error during handleNode, got nil")

	// All fields already set
	client = mocks.NewMockClient(ctrl)
	nodeCtrl.instances = newInstances(client)
	nodeCtrl.metadataLastUpdate["test-node"] = time.Now().Add(-2 * nodeCtrl.ttl)
	client.EXPECT().ListInstances(gomock.Any(), nil).Times(1).Return([]linodego.Instance{
		{ID: 123, Label: "test-node", IPv4: []*net.IP{&publicIP, &privateIP}, HostUUID: "123"},
	}, nil)
	err = nodeCtrl.handleNode(context.TODO(), node)
	assert.NoError(t, err, "expected no error during handleNode")
}

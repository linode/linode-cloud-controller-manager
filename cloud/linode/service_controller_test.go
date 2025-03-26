package linode

import (
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client/mocks"
)

func Test_serviceController_Run(t *testing.T) {
	// Mock dependencies
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockClient(ctrl)
	kubeClient := fake.NewSimpleClientset()
	informer := informers.NewSharedInformerFactory(kubeClient, 0).Core().V1().Services()
	mockQueue := workqueue.NewTypedDelayingQueueWithConfig(workqueue.TypedDelayingQueueConfig[any]{Name: "test"})

	loadbalancers, assertion := newLoadbalancers(client, "us-east").(*loadbalancers)
	if !assertion {
		t.Error("type assertion failed")
	}
	svcCtrl := newServiceController(loadbalancers, informer)
	svcCtrl.queue = mockQueue

	svc := createTestService()
	svc.Spec.Type = "LoadBalancer"
	_, err := kubeClient.CoreV1().Services("test-ns").Create(t.Context(), svc, metav1.CreateOptions{})
	require.NoError(t, err, "expected no error during svc creation")

	// Start the controller
	stopCh := make(chan struct{})
	go svcCtrl.Run(stopCh)

	// Add svc to the informer
	err = svcCtrl.informer.Informer().GetStore().Add(svc)
	require.NoError(t, err, "expected no error when adding svc to informer")

	// Allow some time for the queue to process
	time.Sleep(1 * time.Second)

	// Stop the controller
	close(stopCh)
}

func Test_serviceController_processNextDeletion(t *testing.T) {
	type fields struct {
		loadbalancers *loadbalancers
		queue         workqueue.TypedDelayingInterface[any]
		Client        *mocks.MockClient
	}
	tests := []struct {
		name     string
		fields   fields
		Setup    func(*fields)
		want     bool
		queueLen int
	}{
		{
			name: "Invalid service type",
			fields: fields{
				loadbalancers: nil,
			},
			Setup: func(f *fields) {
				f.loadbalancers = &loadbalancers{client: f.Client, zone: "test", loadBalancerType: Options.LoadBalancerType}
				f.queue = workqueue.NewTypedDelayingQueueWithConfig(workqueue.TypedDelayingQueueConfig[any]{Name: "testQueue"})
				f.queue.Add("test")
			},
			want:     true,
			queueLen: 0,
		},
		{
			name: "Valid service type",
			fields: fields{
				loadbalancers: nil,
			},
			Setup: func(f *fields) {
				f.loadbalancers = &loadbalancers{client: f.Client, zone: "test", loadBalancerType: Options.LoadBalancerType}
				f.queue = workqueue.NewTypedDelayingQueueWithConfig(workqueue.TypedDelayingQueueConfig[any]{Name: "testQueue"})
				svc := createTestService()
				f.queue.Add(svc)
			},
			want:     true,
			queueLen: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &serviceController{
				loadbalancers: tt.fields.loadbalancers,
				queue:         tt.fields.queue,
			}
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := mocks.NewMockClient(ctrl)
			tt.fields.Client = client
			tt.Setup(&tt.fields)
			s.loadbalancers = tt.fields.loadbalancers
			s.queue = tt.fields.queue
			s.loadbalancers.client = tt.fields.Client
			if got := s.processNextDeletion(); got != tt.want {
				t.Errorf("serviceController.processNextDeletion() = %v, want %v", got, tt.want)
			}
			assert.Equal(t, tt.queueLen, tt.fields.queue.Len())
		})
	}
}

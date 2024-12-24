package linode

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client/mocks"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/util/workqueue"
)

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

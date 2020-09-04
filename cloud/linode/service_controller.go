package linode

import (
	"context"
	"net/http"
	"time"

	"github.com/appscode/go/wait"
	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	v1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

const retryInterval = time.Minute * 1

type serviceController struct {
	loadbalancers *loadbalancers
	informer      v1informers.ServiceInformer

	queue workqueue.DelayingInterface
}

func newServiceController(loadbalancers *loadbalancers, informer v1informers.ServiceInformer) *serviceController {
	return &serviceController{
		loadbalancers: loadbalancers,
		informer:      informer,
		queue:         workqueue.NewDelayingQueue(),
	}
}

func (s *serviceController) Run(stopCh <-chan struct{}) {
	s.informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj interface{}) {
			service, ok := obj.(*v1.Service)
			if !ok {
				return
			}

			if service.Spec.Type != "LoadBalancer" {
				return
			}

			klog.Infof("ServiceController will handle service (%s) deletion", getServiceNn(service))
			s.queue.Add(service)
		},
	})

	go wait.Until(s.worker, time.Second, stopCh)
	s.informer.Informer().Run(stopCh)
}

// worker runs a worker thread that dequeues deleted services and processes
// deleting their underlying NodeBalancers.
func (s *serviceController) worker() {
	for s.processNextDeletion() {
	}
}

func (s *serviceController) processNextDeletion() bool {
	key, quit := s.queue.Get()
	if quit {
		return false
	}
	defer s.queue.Done(key)

	service, ok := key.(*v1.Service)
	if !ok {
		klog.Errorf("expected dequeued key to be of type *v1.Service but got %T", service)
		return true
	}

	err := s.handleServiceDeleted(service)
	switch deleteErr := err.(type) {
	case nil:
		break

	case *linodego.Error:
		if deleteErr.Code >= http.StatusInternalServerError || deleteErr.Code == http.StatusTooManyRequests {
			klog.Errorf("failed to delete NodeBalancer for service (%s); retrying in 1 minute: %s", getServiceNn(service), err)
			s.queue.AddAfter(service, retryInterval)
		}

	default:
		klog.Errorf("failed to delete NodeBalancer for service (%s); will not retry: %s", getServiceNn(service), err)
	}
	return true
}

func (s *serviceController) handleServiceDeleted(service *v1.Service) error {
	klog.Infof("ServiceController handling service (%s) deletion", getServiceNn(service))
	return s.loadbalancers.EnsureLoadBalancerDeleted(context.Background(), service.ClusterName, service)
}

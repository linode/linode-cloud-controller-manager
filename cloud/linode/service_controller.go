package linode

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/appscode/go/wait"
	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	v1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

var retryInterval = time.Minute * 1

type serviceController struct {
	loadbalancers *loadbalancers
	informer      v1informers.ServiceInformer

	queue workqueue.TypedDelayingInterface[any]
}

func newServiceController(loadbalancers *loadbalancers, informer v1informers.ServiceInformer) *serviceController {
	return &serviceController{
		loadbalancers: loadbalancers,
		informer:      informer,
		queue:         workqueue.NewTypedDelayingQueue[any](),
	}
}

func (s *serviceController) Run(stopCh <-chan struct{}) {
	if _, err := s.informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj interface{}) {
			service, ok := obj.(*v1.Service)
			if !ok {
				return
			}

			if service.Spec.Type != v1.ServiceTypeLoadBalancer {
				return
			}

			klog.Infof("ServiceController will handle service (%s) deletion", getServiceNn(service))
			s.queue.Add(service)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			newSvc, ok := newObj.(*v1.Service)
			if !ok {
				return
			}
			oldSvc, ok := oldObj.(*v1.Service)
			if !ok {
				return
			}

			if newSvc.Spec.Type != v1.ServiceTypeLoadBalancer && oldSvc.Spec.Type == v1.ServiceTypeLoadBalancer {
				klog.Infof("ServiceController will handle service (%s) LoadBalancer deletion", getServiceNn(oldSvc))
				s.queue.Add(oldSvc)
			}
		},
	}); err != nil {
		klog.Errorf("ServiceController didn't successfully register it's Informer %s", err)
	}

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
	var targetError *linodego.Error
	if err != nil {
		if errors.As(err, &targetError) &&
			(targetError.Code >= http.StatusInternalServerError || targetError.Code == http.StatusTooManyRequests) {
			klog.Errorf("failed to delete NodeBalancer for service (%s); retrying in 1 minute: %s", getServiceNn(service), err)
			s.queue.AddAfter(service, retryInterval)
		} else {
			klog.Errorf("failed to delete NodeBalancer for service (%s); will not retry: %s", getServiceNn(service), err)
		}
	}

	return true
}

func (s *serviceController) handleServiceDeleted(service *v1.Service) error {
	klog.Infof("ServiceController handling service (%s) deletion", getServiceNn(service))
	clusterName := strings.TrimPrefix(service.Namespace, "kube-system-")
	return s.loadbalancers.EnsureLoadBalancerDeleted(context.Background(), clusterName, service)
}

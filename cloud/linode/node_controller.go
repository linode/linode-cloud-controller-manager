package linode

import (
	"context"
	"net/http"
	"time"

	"github.com/appscode/go/wait"
	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

type nodeController struct {
	client     Client
	instances  *instances
	kubeclient kubernetes.Interface
	informer   v1informers.NodeInformer

	queue workqueue.DelayingInterface
}

func newNodeController(kubeclient kubernetes.Interface, client Client, informer v1informers.NodeInformer) *nodeController {
	return &nodeController{
		client:     client,
		instances:  newInstances(client),
		kubeclient: kubeclient,
		informer:   informer,
		queue:      workqueue.NewDelayingQueue(),
	}
}

func (s *nodeController) Run(stopCh <-chan struct{}) {
	s.informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node, ok := obj.(*v1.Node)
			if !ok {
				return
			}

			klog.Infof("NodeController will handle newly created node (%s) metadata", node.Name)
			s.queue.Add(node)
		},
		UpdateFunc: func(_, new interface{}) {
			node, ok := new.(*v1.Node)
			if !ok {
				return
			}

			klog.Infof("NodeController will handle updated node (%s) metadata", node.Name)
			s.queue.Add(node)
		},
	})

	go wait.Until(s.worker, time.Second, stopCh)
	s.informer.Informer().Run(stopCh)
}

// worker runs a worker thread that dequeues new or modified nodes and processes
// metadata (host UUID) on each of them.
func (s *nodeController) worker() {
	for s.processNext() {
	}
}

func (s *nodeController) processNext() bool {
	key, quit := s.queue.Get()
	if quit {
		return false
	}
	defer s.queue.Done(key)

	node, ok := key.(*v1.Node)
	if !ok {
		klog.Errorf("expected dequeued key to be of type *v1.Node but got %T", node)
		return true
	}

	err := s.handleNodeAdded(context.TODO(), node)
	switch deleteErr := err.(type) {
	case nil:
		break

	case *linodego.Error:
		if deleteErr.Code >= http.StatusInternalServerError || deleteErr.Code == http.StatusTooManyRequests {
			klog.Errorf("failed to add metadata for node (%s); retrying in 1 minute: %s", node.Name, err)
			s.queue.AddAfter(node, retryInterval)
		}

	default:
		klog.Errorf("failed to add metadata for node (%s); will not retry: %s", node.Name, err)
	}
	return true
}

func (s *nodeController) handleNodeAdded(ctx context.Context, node *v1.Node) error {
	klog.Infof("NodeController handling node (%s) addition", node.Name)

	linode, err := s.instances.lookupLinode(ctx, node)
	if err != nil {
		klog.Infof("instance lookup error: %s", err.Error())
		return err
	}

	uuid, ok := node.Labels[annLinodeHostUUID]
	if ok && uuid == linode.HostUUID {
		return nil
	}

	node.Labels[annLinodeHostUUID] = linode.HostUUID

	_, err = s.kubeclient.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		klog.Infof("node update error: %s", err.Error())
		return err
	}

	return nil
}

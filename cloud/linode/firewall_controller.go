package linode

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/appscode/go/wait"
	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	v1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/linode/linode-cloud-controller-manager/cloud/annotations"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/firewall"
)

type firewallController struct {
	sync.RWMutex

	instances *instances
	firewalls *firewall.Firewalls
	informer  v1informers.NodeInformer

	queue workqueue.DelayingInterface
}

func newFirewallController(client client.Client, informer v1informers.NodeInformer) *firewallController {
	return &firewallController{
		instances: newInstances(client),
		firewalls: firewall.NewFirewalls(client),
		informer:  informer,
		queue:     workqueue.NewDelayingQueue(),
	}
}

func (s *firewallController) Run(stopCh <-chan struct{}) {
	s.informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, cur interface{}) {
			oldNode, okOld := old.(*v1.Node)
			curNode, okCur := cur.(*v1.Node)
			if !okCur || !okOld {
				return
			}
			// check if fw annotations changed
			oldACL := oldNode.GetAnnotations()[annotations.AnnLinodeNodeFirewallACL]
			curACL := curNode.GetAnnotations()[annotations.AnnLinodeNodeFirewallACL]

			oldID := oldNode.GetAnnotations()[annotations.AnnLinodeNodeFirewallID]
			curID := curNode.GetAnnotations()[annotations.AnnLinodeNodeFirewallID]

			if oldACL == curACL && oldID == curID {
				return
			}

			klog.Infof("FirewallController will handle updated node (%s) firewall annotations", curNode.Name)
			s.queue.Add(curNode)
		},
		AddFunc: func(obj interface{}) {
			node, ok := obj.(*v1.Node)
			if !ok {
				return
			}
			// check if fw annotations set
			acl := node.GetAnnotations()[annotations.AnnLinodeNodeFirewallACL]
			id := node.GetAnnotations()[annotations.AnnLinodeNodeFirewallID]

			if acl == "" && id == "" {
				return
			}

			klog.Infof("FirewallController will handle new node (%s) firewall annotations", node.Name)
			s.queue.Add(node)
		},
	})

	go wait.Until(s.worker, time.Second, stopCh)
	s.informer.Informer().Run(stopCh)
}

// worker runs a worker thread that dequeues new or modified nodes and processes
// firewall annotations on each of them.
func (s *firewallController) worker() {
	for s.processNext() {
	}
}

func (s *firewallController) processNext() bool {
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

	err := s.handleNode(context.TODO(), node)
	switch err := err.(type) {
	case nil:
		break

	case *linodego.Error:
		if err.Code >= http.StatusInternalServerError || err.Code == http.StatusTooManyRequests {
			klog.Errorf("failed to handle node (%s); retrying in 1 minute: %v", node.Name, err)
			s.queue.AddAfter(node, retryInterval)
		}

	default:
		klog.Errorf("failed to handle node (%s); will not retry: %v", node.Name, err)
	}
	return true
}

func (s *firewallController) handleNode(ctx context.Context, node *v1.Node) error {
	klog.Infof("FirewallController handling node (%s)", node.Name)

	linode, err := s.instances.lookupLinode(ctx, node)
	if err != nil {
		klog.Infof("instance lookup error: %v", err)
		return err
	}

	// reconcile the firewall if any
	if err = s.firewalls.UpdateNodeFirewall(ctx, node, linode); err != nil {
		klog.Infof("firewall update error: %v", err)
		return err
	}

	return nil
}

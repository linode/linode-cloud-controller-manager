package linode

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/appscode/go/wait"
	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/linode/linode-cloud-controller-manager/cloud/annotations"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client"
)

const (
	informerResyncPeriod = 1 * time.Minute
	defaultMetadataTTL   = 300 * time.Second
)

type nodeController struct {
	sync.RWMutex

	client     client.Client
	instances  *instances
	kubeclient kubernetes.Interface
	informer   v1informers.NodeInformer

	metadataLastUpdate map[string]time.Time
	ttl                time.Duration

	queue workqueue.TypedDelayingInterface[any]
}

func newNodeController(kubeclient kubernetes.Interface, client client.Client, informer v1informers.NodeInformer, instanceCache *instances) *nodeController {
	timeout := defaultMetadataTTL
	if raw, ok := os.LookupEnv("LINODE_METADATA_TTL"); ok {
		if t, _ := strconv.Atoi(raw); t > 0 {
			timeout = time.Duration(t) * time.Second
		}
	}

	return &nodeController{
		client:             client,
		instances:          instanceCache,
		kubeclient:         kubeclient,
		informer:           informer,
		ttl:                timeout,
		metadataLastUpdate: make(map[string]time.Time),
		queue:              workqueue.NewTypedDelayingQueueWithConfig[any](workqueue.TypedDelayingQueueConfig[any]{Name: "ccm_node"}),
	}
}

func (s *nodeController) Run(stopCh <-chan struct{}) {
	if _, err := s.informer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				node, ok := obj.(*v1.Node)
				if !ok {
					return
				}

				klog.Infof("NodeController will handle newly created node (%s) metadata", node.Name)
				s.queue.Add(node)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				node, ok := newObj.(*v1.Node)
				if !ok {
					return
				}

				klog.Infof("NodeController will handle newly updated node (%s) metadata", node.Name)
				s.queue.Add(node)
			},
		},
		informerResyncPeriod,
	); err != nil {
		klog.Errorf("NodeController can't handle newly created node's metadata. %s", err)
	}

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

	err := s.handleNode(context.TODO(), node)
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

func (s *nodeController) LastMetadataUpdate(nodeName string) time.Time {
	s.RLock()
	defer s.RUnlock()
	return s.metadataLastUpdate[nodeName]
}

func (s *nodeController) SetLastMetadataUpdate(nodeName string) {
	s.Lock()
	defer s.Unlock()
	s.metadataLastUpdate[nodeName] = time.Now()
}

func (s *nodeController) handleNode(ctx context.Context, node *v1.Node) error {
	klog.V(3).InfoS("NodeController handling node metadata",
		"node", klog.KObj(node))

	lastUpdate := s.LastMetadataUpdate(node.Name)

	uuid, foundLabel := node.Labels[annotations.AnnLinodeHostUUID]
	configuredPrivateIP, foundAnnotation := node.Annotations[annotations.AnnLinodeNodePrivateIP]

	metaAge := time.Since(lastUpdate)
	if foundLabel && foundAnnotation && metaAge < s.ttl {
		klog.V(3).InfoS("Skipping refresh, ttl not reached",
			"node", klog.KObj(node),
			"ttl", s.ttl,
			"metadata_age", metaAge,
		)
		return nil
	}

	linode, err := s.instances.lookupLinode(ctx, node)
	if err != nil {
		klog.V(1).ErrorS(err, "Instance lookup error")
		return err
	}

	expectedPrivateIP := ""
	// linode API response for linode will contain only one private ip
	// if any private ip is configured. If it changes in future or linode
	// supports other subnets with nodebalancer, this logic needs to be updated.
	// https://www.linode.com/docs/api/linode-instances/#linode-view
	for _, addr := range linode.IPv4 {
		if isPrivate(addr) {
			expectedPrivateIP = addr.String()
			break
		}
	}

	if uuid == linode.HostUUID && node.Spec.ProviderID != "" && configuredPrivateIP == expectedPrivateIP {
		s.SetLastMetadataUpdate(node.Name)
		return nil
	}

	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get a fresh copy of the node so the resource version is up-to-date
		nodeResult, err := s.kubeclient.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		// Try to update the node UUID if it has not been set
		if nodeResult.Labels[annotations.AnnLinodeHostUUID] != linode.HostUUID {
			nodeResult.Labels[annotations.AnnLinodeHostUUID] = linode.HostUUID
		}

		// Try to update the node ProviderID if it has not been set
		if nodeResult.Spec.ProviderID == "" {
			nodeResult.Spec.ProviderID = providerIDPrefix + strconv.Itoa(linode.ID)
		}

		// Try to update the expectedPrivateIP if its not set or doesn't match
		if nodeResult.Annotations[annotations.AnnLinodeNodePrivateIP] != expectedPrivateIP && expectedPrivateIP != "" {
			nodeResult.Annotations[annotations.AnnLinodeNodePrivateIP] = expectedPrivateIP
		}
		_, err = s.kubeclient.CoreV1().Nodes().Update(ctx, nodeResult, metav1.UpdateOptions{})
		return err
	}); err != nil {
		klog.V(1).ErrorS(err, "Node update error")
		return err
	}

	s.SetLastMetadataUpdate(node.Name)

	return nil
}

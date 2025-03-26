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
	informerResyncPeriod   = 1 * time.Minute
	defaultMetadataTTL     = 300 * time.Second
	defaultK8sNodeCacheTTL = 300 * time.Second
	listNodeContextTimeout = 30 * time.Second
)

var registeredK8sNodeCache *k8sNodeCache = newK8sNodeCache()

type nodeRequest struct {
	node      *v1.Node
	timestamp time.Time
}

type nodeController struct {
	sync.RWMutex

	client     client.Client
	instances  *instances
	kubeclient kubernetes.Interface
	informer   v1informers.NodeInformer

	metadataLastUpdate map[string]time.Time
	ttl                time.Duration

	queue         workqueue.TypedDelayingInterface[nodeRequest]
	nodeLastAdded map[string]time.Time
}

// k8sNodeCache stores node related info as registered in k8s
type k8sNodeCache struct {
	sync.RWMutex
	nodes       map[string]*v1.Node
	providerIDs map[string]string
	lastUpdate  time.Time
	ttl         time.Duration
}

// updateCache updates the k8s node cache with the latest nodes from the k8s API server.
func (c *k8sNodeCache) updateCache(kubeclient kubernetes.Interface) {
	c.Lock()
	defer c.Unlock()
	if time.Since(c.lastUpdate) < c.ttl {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), listNodeContextTimeout)
	defer cancel()

	nodeList, err := kubeclient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Errorf("failed to list nodes, cannot create/update k8s node cache: %s", err)
		return
	}

	nodes := make(map[string]*v1.Node, len(nodeList.Items))
	providerIDs := make(map[string]string, len(nodeList.Items))
	for _, node := range nodeList.Items {
		if node.Spec.ProviderID == "" {
			klog.Errorf("Empty providerID [%s] for node %s, skipping it", node.Spec.ProviderID, node.Name)
			continue
		}
		nodes[node.Name] = &node
		providerIDs[node.Spec.ProviderID] = node.Name
	}

	c.nodes = nodes
	c.providerIDs = providerIDs
	c.lastUpdate = time.Now()
}

// addNodeToCache stores the specified node in k8s node cache
func (c *k8sNodeCache) addNodeToCache(node *v1.Node) {
	c.Lock()
	defer c.Unlock()
	if node.Spec.ProviderID == "" {
		klog.Errorf("Empty providerID [%s] for node %s, skipping it", node.Spec.ProviderID, node.Name)
		return
	}
	c.nodes[node.Name] = node
	c.providerIDs[node.Spec.ProviderID] = node.Name
}

// getNodeLabel returns the k8s node label for the given provider ID or instance label.
// If the provider ID or label is not found in the cache, it returns an empty string and false.
func (c *k8sNodeCache) getNodeLabel(providerID string, instanceLabel string) (string, bool) {
	c.RLock()
	defer c.RUnlock()

	// check if instance label matches with the registered k8s node
	if _, exists := c.nodes[instanceLabel]; exists {
		return instanceLabel, true
	}

	// check if provider id matches with the registered k8s node
	if label, exists := c.providerIDs[providerID]; exists {
		return label, true
	}

	return "", false
}

// getProviderID returns linode specific providerID for given k8s node name
func (c *k8sNodeCache) getProviderID(nodeName string) (string, bool) {
	c.RLock()
	defer c.RUnlock()

	if node, exists := c.nodes[nodeName]; exists {
		return node.Spec.ProviderID, true
	}

	return "", false
}

// newK8sNodeCache returns new k8s node cache instance
func newK8sNodeCache() *k8sNodeCache {
	timeout := defaultK8sNodeCacheTTL
	if raw, ok := os.LookupEnv("K8S_NODECACHE_TTL"); ok {
		if t, err := strconv.Atoi(raw); t > 0 && err == nil {
			timeout = time.Duration(t) * time.Second
		}
	}

	return &k8sNodeCache{
		nodes:       make(map[string]*v1.Node, 0),
		providerIDs: make(map[string]string, 0),
		ttl:         timeout,
	}
}

func newNodeController(kubeclient kubernetes.Interface, client client.Client, informer v1informers.NodeInformer, instanceCache *instances) *nodeController {
	timeout := defaultMetadataTTL
	if raw, ok := os.LookupEnv("LINODE_METADATA_TTL"); ok {
		if t, err := strconv.Atoi(raw); t > 0 && err == nil {
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
		queue:              workqueue.NewTypedDelayingQueueWithConfig(workqueue.TypedDelayingQueueConfig[nodeRequest]{Name: "ccm_node"}),
		nodeLastAdded:      make(map[string]time.Time),
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
				s.addNodeToQueue(node)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				node, ok := newObj.(*v1.Node)
				if !ok {
					return
				}

				klog.Infof("NodeController will handle newly updated node (%s) metadata", node.Name)
				s.addNodeToQueue(node)
			},
		},
		informerResyncPeriod,
	); err != nil {
		klog.Errorf("NodeController can't handle newly created node's metadata. %s", err)
	}

	go wait.Until(s.worker, time.Second, stopCh)
	s.informer.Informer().Run(stopCh)
}

// addNodeToQueue adds a node to the queue for processing.
func (s *nodeController) addNodeToQueue(node *v1.Node) {
	s.Lock()
	defer s.Unlock()
	currTime := time.Now()
	s.nodeLastAdded[node.Name] = currTime
	s.queue.Add(nodeRequest{node: node, timestamp: currTime})
}

// worker runs a worker thread that dequeues new or modified nodes and processes
// metadata (host UUID) on each of them.
func (s *nodeController) worker() {
	for s.processNext() {
	}
}

func (s *nodeController) processNext() bool {
	request, quit := s.queue.Get()
	if quit {
		return false
	}
	defer s.queue.Done(request)

	s.RLock()
	latestTimestamp, exists := s.nodeLastAdded[request.node.Name]
	s.RUnlock()
	if !exists || request.timestamp.Before(latestTimestamp) {
		klog.V(3).InfoS("Skipping node metadata update as its not the most recent object", "node", klog.KObj(request.node))
		return true
	}
	err := s.handleNode(context.TODO(), request.node)
	//nolint: errorlint //switching to errors.Is()/errors.As() causes errors with Code field
	switch deleteErr := err.(type) {
	case nil:
		break

	case *linodego.Error:
		if deleteErr.Code >= http.StatusInternalServerError || deleteErr.Code == http.StatusTooManyRequests {
			klog.Errorf("failed to add metadata for node (%s); retrying in 1 minute: %s", request.node.Name, err)
			s.queue.AddAfter(request, retryInterval)
		}

	default:
		klog.Errorf("failed to add metadata for node (%s); will not retry: %s", request.node.Name, err)
	}

	registeredK8sNodeCache.updateCache(s.kubeclient)
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
	// if any private ip is configured.
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

	var updatedNode *v1.Node
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
		updatedNode, err = s.kubeclient.CoreV1().Nodes().Update(ctx, nodeResult, metav1.UpdateOptions{})
		return err
	}); err != nil {
		klog.V(1).ErrorS(err, "Node update error")
		return err
	}

	if updatedNode != nil {
		registeredK8sNodeCache.addNodeToCache(updatedNode)
	}
	s.SetLastMetadataUpdate(node.Name)

	return nil
}

package annotator

import (
	"context"
	"fmt"
	"strconv"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	policy "github.com/gocrane/crane-scheduler/pkg/plugins/apis/policy"

	nodestats "github.com/gocrane/crane-scheduler/pkg/controller/nodestats"
	utils "github.com/gocrane/crane-scheduler/pkg/utils"
)

const (
	HotValueKey    = "node_hot_value"
	DefaultBackOff = 10 * time.Second
	MaxBackOff     = 360 * time.Second
)

type nodeController struct {
	*Controller
	queue workqueue.RateLimitingInterface
}

func newNodeController(c *Controller) *nodeController {
	nodeRateLimiter := workqueue.NewItemExponentialFailureRateLimiter(DefaultBackOff,
		MaxBackOff)

	return &nodeController{
		Controller: c,
		queue:      workqueue.NewNamedRateLimitingQueue(nodeRateLimiter, "node_event_queue"),
	}
}

func (n *nodeController) Run() {
	defer n.queue.ShutDown()
	klog.Infof("Start to reconcile node events")

	for n.processNextWorkItem() {
	}
}

func (n *nodeController) processNextWorkItem() bool {
	key, quit := n.queue.Get()
	if quit {
		return false
	}
	defer n.queue.Done(key)

	forget, err := n.syncNode(key.(string))
	if err != nil {
		klog.Warningf("failed to sync this node [%q]: %v", key.(string), err)
	}
	if forget {
		n.queue.Forget(key)
		return true
	}

	n.queue.AddRateLimited(key)
	return true
}

func (n *nodeController) syncNode(key string) (bool, error) {
	startTime := time.Now()
	defer func() {
		klog.Infof("Finished syncing node event %q (%v)", key, time.Since(startTime))
	}()

	nodeName, metricName, err := splitMetaKeyWithMetricName(key)
	if err != nil {
		return true, fmt.Errorf("invalid resource key: %s", key)
	}

	node, err := n.nodeLister.Get(nodeName)
	if err != nil {
		return true, fmt.Errorf("can not find node[%s]: %v", node, err)
	}

	err = n.annotateNodeLoad(node, metricName)
	if err != nil {
		return false, fmt.Errorf("can not annotate node[%s]: %v", node.Name, err)
	}

	err = annotateNodeHotValue(n.kubeClient, n.bindingRecords, node, n.policy)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (n *nodeController) annotateNodeLoad(node *v1.Node, key string) error {
	if n.promClient != nil {
		value, err := n.promClient.QueryByNodeIP(key, getNodeInternalIP(node))
		if err == nil && len(value) > 0 {
			return patchNodeAnnotation(n.kubeClient, node, key, value)
		}
		value, err = n.promClient.QueryByNodeName(key, getNodeName(node))
		if err == nil && len(value) > 0 {
			return patchNodeAnnotation(n.kubeClient, node, key, value)
		}
		return fmt.Errorf("failed to get data %s{%s=%s}: %v", key, node.Name, value, err)
	} else {
		data, err := nodestats.GetNodeStats(n.httpClientPool, getNodeInternalIP(node), n.cfg.StatisticsPort, n.cfg.StatisticsPath)
		var value float64
		if err == nil {
			value, ok := data[key]
			if ok {
				return patchNodeAnnotation(n.kubeClient, node, key, strconv.FormatFloat(value, 'f', 1, 64))
			}
		}
		return fmt.Errorf("failed to get data %s{%s=%s}: %v", key, node.Name, strconv.FormatFloat(value, 'f', 1, 64), err)
	}
}

func annotateNodeHotValue(kubeClient clientset.Interface, br *BindingRecords, node *v1.Node, policy policy.DynamicSchedulerPolicy) error {
	var value int

	for _, p := range policy.Spec.HotValue {
		value += br.GetLastNodeBindingCount(node.Name, p.TimeRange.Duration) / p.Count
	}

	return patchNodeAnnotation(kubeClient, node, HotValueKey, strconv.Itoa(value))
}

func patchNodeAnnotation(kubeClient clientset.Interface, node *v1.Node, key, value string) error {
	annotation := node.GetAnnotations()
	if annotation == nil {
		annotation = map[string]string{}
	}

	operator := "add"
	_, exist := annotation[key]
	if exist {
		operator = "replace"
	}

	patchAnnotationTemplate :=
		`[{
		"op": "%s",
		"path": "/metadata/annotations/%s",
		"value": "%s"
	}]`

	patchData := fmt.Sprintf(patchAnnotationTemplate, operator, key, value+","+utils.GetLocalTime())

	_, err := kubeClient.CoreV1().Nodes().Patch(context.TODO(), node.Name, types.JSONPatchType, []byte(patchData), metav1.PatchOptions{})
	return err
}

func (n *nodeController) CreateMetricSyncTicker(stopCh <-chan struct{}) {

	for _, p := range n.policy.Spec.SyncPeriod {
		enqueueFunc := func(policy policy.SyncPolicy) {
			nodes, err := n.nodeLister.List(labels.Everything())
			if err != nil {
				panic(fmt.Errorf("failed to list nodes: %v", err))
			}

			for _, node := range nodes {
				n.queue.Add(handlingMetaKeyWithMetricName(node.Name, policy.Name))
			}
		}

		enqueueFunc(p)

		go func(policy policy.SyncPolicy) {
			ticker := time.NewTicker(policy.Period.Duration)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					enqueueFunc(policy)
				case <-stopCh:
					return
				}
			}
		}(p)
	}
}

func getNodeInternalIP(node *v1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == v1.NodeInternalIP {
			return addr.Address
		}
	}

	return node.Name
}

func getNodeName(node *v1.Node) string {
	return node.Name
}

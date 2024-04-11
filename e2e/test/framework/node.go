package framework

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	masterLabel = "node-role.kubernetes.io/master"
)

func (i *Invocation) GetWorkerNodeList() ([]string, error) {
	workers := make([]string, 0)
	nodes, err := i.kubeClient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, node := range nodes.Items {
		if _, found := node.ObjectMeta.Labels[masterLabel]; !found {
			workers = append(workers, node.Name)
		}
	}
	return workers, nil
}

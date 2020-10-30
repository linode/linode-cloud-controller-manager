package framework

import (
	"context"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *Framework) Namespace() string {
	return f.namespace
}

func (f *Framework) CreateNamespace() error {
	obj := &core.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: f.namespace,
		},
	}
	_, err := f.kubeClient.CoreV1().Namespaces().Create(context.TODO(), obj, metav1.CreateOptions{})
	return err
}

func (f *Framework) DeleteNamespace() error {
	return f.kubeClient.CoreV1().Namespaces().Delete(context.TODO(), f.namespace, deleteInForeground())
}

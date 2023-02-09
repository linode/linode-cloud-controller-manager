package framework

import (
	"context"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (i *lbInvocation) GetPodObject(podName, image string, ports []core.ContainerPort, labels map[string]string) *core.Pod {
	return &core.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: i.Namespace(),
			Labels:    labels,
		},
		Spec: core.PodSpec{
			Containers: []core.Container{
				{
					Name:  "server",
					Image: image,
					Env: []core.EnvVar{
						{
							Name: "POD_NAME",
							ValueFrom: &core.EnvVarSource{
								FieldRef: &core.ObjectFieldSelector{
									FieldPath: "metadata.name",
								},
							},
						},
					},
					Ports: ports,
				},
			},
		},
	}
}

func (i *lbInvocation) SetNodeSelector(pod *core.Pod, nodeName string) *core.Pod {
	pod.Spec.NodeSelector = map[string]string{
		"kubernetes.io/hostname": nodeName,
	}
	return pod
}

func (i *lbInvocation) CreatePod(pod *core.Pod) (*core.Pod, error) {
	return i.kubeClient.CoreV1().Pods(i.Namespace()).Create(context.TODO(), pod, metav1.CreateOptions{})
}

func (i *lbInvocation) DeletePod(name string) error {
	return i.kubeClient.CoreV1().Pods(i.Namespace()).Delete(context.TODO(), name, deleteInForeground())
}

func (i *lbInvocation) GetPod(name, ns string) (*core.Pod, error) {
	return i.kubeClient.CoreV1().Pods(ns).Get(context.TODO(), name, metav1.GetOptions{})
}

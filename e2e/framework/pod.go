package framework

import (
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/util/wait"
)

func (i *lbInvocation) GetPodObject(podName, nodeName string, labels map[string]string) *core.Pod {
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
					Image: testServerImage,
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
					Ports: []core.ContainerPort{
						{
							Name:          "http-1",
							ContainerPort: 8080,
						},
					},
				},
			},
			NodeSelector: map[string]string{
				"kubernetes.io/hostname": nodeName,
			},
		},
	}
}

func (i *lbInvocation) CreatePod(pod *core.Pod) error {
	pod, err := i.kubeClient.CoreV1().Pods(i.Namespace()).Create(pod)
	if err != nil {
		return err
	}
	return i.WaitForReady(pod.ObjectMeta)

}

func (i *lbInvocation) DeletePod(name string) error {
	return i.kubeClient.CoreV1().Pods(i.Namespace()).Delete(name, deleteInForeground())
}

func (i *lbInvocation) GetPod(name, ns string) (*core.Pod, error) {
	return i.kubeClient.CoreV1().Pods(ns).Get(name, metav1.GetOptions{})
}

func (i *lbInvocation) WaitForReady(meta metav1.ObjectMeta) error {
	return wait.PollImmediate(RetryInterval, RetryTimout, func() (bool, error) {
		pod, err := i.kubeClient.CoreV1().Pods(i.Namespace()).Get(meta.Name, metav1.GetOptions{})
		if pod == nil || err != nil {
			return false, nil
		}
		if pod.Status.Phase == core.PodRunning {
			return true, nil
		}
		return false, nil
	})
}

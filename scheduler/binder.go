package scheduler

import (
    "context"

    v1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
)

func BindPod(clientset *kubernetes.Clientset, pod *v1.Pod, nodeName string) error {
    binding := &v1.Binding{
        ObjectMeta: metav1.ObjectMeta{
            Name: pod.Name,
            Namespace: pod.Namespace,
        },
        Target: v1.ObjectReference{
            Kind: "Node",
            Name: nodeName,
        },
    }
    return clientset.CoreV1().Pods(pod.Namespace).Bind(context.TODO(), binding, metav1.CreateOptions{})
}

package scheduler

import (
    "context"
    "fmt"
    "time"

    v1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/clientcmd"
    "k8s.io/client-go/util/homedir"
    "path/filepath"
)

func Start() {
	fmt.Print("Starting the scheduler...\n")
    kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
    config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
    if err != nil {
        panic(err.Error())
    }

    clientset, err := kubernetes.NewForConfig(config)
    if err != nil {
        panic(err.Error())
    }

    for {
        pods, _ := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
            FieldSelector: "spec.schedulerName=simple-k8,spec.nodeName=",
        })

        for _, pod := range pods.Items {
            fmt.Printf("Found pod to schedule: %s/%s\n", pod.Namespace, pod.Name)
            SchedulePod(clientset, &pod)
        }

        time.Sleep(2 * time.Second)
    }

	
}

func SchedulePod(clientset *kubernetes.Clientset, pod *v1.Pod) {
    Enqueue(pod)
    selected := Dequeue(pod.Namespace)
    if selected == nil {
        return
    }

    node, err := SelectBestNode(clientset)
    if err != nil {
        fmt.Printf("No suitable node: %v\n", err)
        return
    }

    err = BindPod(clientset, selected, node)
    if err != nil {
        fmt.Printf("Binding failed: %v\n", err)
    } else {
        fmt.Printf("Bound pod %s to node %s\n", selected.Name, node)
    }
}


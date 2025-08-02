package scheduler

import (
	"context"
	"fmt"
	"time"

	"path/filepath"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
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

func SchedulePod(clientset kubernetes.Interface, pod *v1.Pod) {
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

// SchedulePodWithCapacity enforces queue capacity when scheduling
func SchedulePodWithCapacity(clientset kubernetes.Interface, pod *v1.Pod) {
	queuePath := pod.Annotations["scheduler.kubernetes.io/queue"]
	if queuePath == "" {
		queuePath = fmt.Sprintf("root.%s", pod.Namespace)
	}
	queue := GetQueue(queuePath)
	if queue == nil {
		return
	}

	clusterTotal, err := GetClusterTotalResources(clientset)
	if err != nil {
		fmt.Printf("Error getting cluster resources: %v\n", err)
		return
	}
	podReq := getPodResourceRequests(pod)
	futureUsage := addResourceLists(queue.ResourceUsage, podReq)
	if !isWithinCapacity(futureUsage, clusterTotal, queue) {
		fmt.Printf("Queue %s exceeds capacity, cannot schedule pod %s\n", queuePath, pod.Name)
		return
	}

	// If within capacity, proceed to select node and bind
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
		// Update queue resource usage
		queue.ResourceUsage = addResourceLists(queue.ResourceUsage, podReq)
	}
}

package scheduler

import (
	"context"
	"fmt"
	"time"

	"path/filepath"

	"sample-k8-scheduler/scheduler/update_status"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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

	// Start watching Queue CRD
	go WatchQueueCRD(config)

	for {
		pods, _ := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
			FieldSelector: "spec.schedulerName=kubescheduler,spec.nodeName=",
		})

		for _, pod := range pods.Items {
			fmt.Printf("Found pod to schedule: %s/%s\n", pod.Namespace, pod.Name)
			SchedulePodWithCapacity(clientset, config, &pod)
		}

		time.Sleep(2 * time.Second)
	}

}

// WatchQueueCRD watches for changes to the Queue CRD and updates scheduler state
func WatchQueueCRD(config *rest.Config) {
	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		fmt.Printf("Error creating dynamic client: %v\n", err)
		return
	}

	queueGVR := schema.GroupVersionResource{
		Group:    "kubescheduler.example.com", // replace with your CRD group
		Version:  "v1",                        // replace with your CRD version
		Resource: "queues",                    // replace with your CRD resource name
	}

	watcher, err := dynClient.Resource(queueGVR).Namespace("").Watch(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Error watching Queue CRD: %v\n", err)
		return
	}

	for event := range watcher.ResultChan() {
		switch event.Type {
		case watch.Added, watch.Modified:
			fmt.Printf("Queue CRD event: %v\n", event.Type)
			UpdateQueueState(event.Object) // Now calls the function from queues.go
		case watch.Deleted:
			fmt.Printf("Queue CRD deleted event\n")
			DeleteQueueState(event.Object) // Now calls the function from queues.go
		}
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
func SchedulePodWithCapacity(clientset kubernetes.Interface, config *rest.Config, pod *v1.Pod) {
	Enqueue(pod)
	queuePath := pod.Annotations["scheduler.kubernetes.io/queue"]
	if queuePath == "" {
		queuePath = fmt.Sprintf("root.%s", pod.Namespace)
	}
	queue := GetQueue(queuePath)
	if queue == nil {
		return
	}
	if queue.ResourceUsage == nil {
		queue.ResourceUsage = v1.ResourceList{}
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
	// Debug log
	fmt.Printf("Going ahead with scheduling pod %s in queue %s\n", pod.Name, queuePath)

	// If within capacity, proceed to select node and bind
	selected := Dequeue(queuePath)
	// Debug for selected pod
	fmt.Printf("Selected pod for scheduling: %v\n", selected)
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

		// Calculate usage percent for CPU and memory
		cpuPercent := 0
		memPercent := 0
		totalCPU := clusterTotal[v1.ResourceCPU]
		usedCPU := queue.ResourceUsage[v1.ResourceCPU]
		totalMem := clusterTotal[v1.ResourceMemory]
		usedMem := queue.ResourceUsage[v1.ResourceMemory]
		if !totalCPU.IsZero() {
			cpuPercent = int(float64(usedCPU.MilliValue()) / float64(totalCPU.MilliValue()) * 100)
		}
		if !totalMem.IsZero() {
			memPercent = int(float64(usedMem.Value()) / float64(totalMem.Value()) * 100)
		}

		// Update Queue CRD status with both CPU and memory usage
		err = update_status.UpdateQueueStatus(config, queue.Name, cpuPercent, memPercent)
		if err != nil {
			fmt.Printf("Failed to update queue status: %v\n", err)
		}
	}
}

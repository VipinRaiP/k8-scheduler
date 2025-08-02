package scheduler

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type QueueConfig struct {
	Capacity    int    // Percentage of total cluster resources
	MaxCapacity int    // Maximum capacity the queue can grow to
	Policy      string // Scheduling policy (e.g., "fifo", "fair")
}

type Queue struct {
	Name     string
	Parent   *Queue
	Children map[string]*Queue
	Config   QueueConfig
	Pods     []*v1.Pod
	Path     string // Full path of queue (e.g., "root.development.team-a")
	// Track current resource usage for the queue
	ResourceUsage v1.ResourceList
}

var rootQueue = &Queue{
	Name:     "root",
	Path:     "root",
	Children: make(map[string]*Queue),
	Config: QueueConfig{
		Capacity:    100,
		MaxCapacity: 100,
		Policy:      "fifo",
	},
}

// CreateQueue creates a new queue at the specified path
func CreateQueue(path string, config QueueConfig) error {
	if path == "" || path == "root" {
		return fmt.Errorf("invalid queue path")
	}

	parts := strings.Split(path, ".")
	current := rootQueue

	// Navigate through the hierarchy
	for i := 1; i < len(parts); i++ {
		name := parts[i]
		child, exists := current.Children[name]
		if !exists {
			child = &Queue{
				Name:     name,
				Parent:   current,
				Path:     strings.Join(parts[:i+1], "."),
				Children: make(map[string]*Queue),
				Config:   config,
			}
			current.Children[name] = child
		}
		current = child
	}
	return nil
}

// GetQueue returns a queue by its path
func GetQueue(path string) *Queue {
	if path == "" || path == "root" {
		return rootQueue
	}

	parts := strings.Split(path, ".")
	current := rootQueue

	for i := 1; i < len(parts); i++ {
		child, exists := current.Children[parts[i]]
		if !exists {
			return nil
		}
		current = child
	}
	return current
}

func Enqueue(pod *v1.Pod) {
	// Get queue path from pod annotation, default to namespace if not specified
	queuePath := pod.Annotations["scheduler.kubernetes.io/queue"]
	if queuePath == "" {
		queuePath = fmt.Sprintf("root.%s", pod.Namespace)
	}

	queue := GetQueue(queuePath)
	if queue == nil {
		// Create default queue for namespace if it doesn't exist
		err := CreateQueue(queuePath, QueueConfig{
			Capacity:    0, // No specific capacity limit
			MaxCapacity: 100,
			Policy:      "fifo",
		})
		if err != nil {
			return
		}
		queue = GetQueue(queuePath)
	}

	queue.Pods = append(queue.Pods, pod)
}

func Dequeue(namespace string) *v1.Pod {
	queuePath := fmt.Sprintf("root.%s", namespace)
	queue := GetQueue(queuePath)
	if queue == nil || len(queue.Pods) == 0 {
		return nil
	}

	// Apply queue policy (currently only FIFO)
	pod := queue.Pods[0]
	queue.Pods = queue.Pods[1:]
	return pod
}

// Helper to sum resource requests for a pod
func getPodResourceRequests(pod *v1.Pod) v1.ResourceList {
	total := v1.ResourceList{}
	for _, c := range pod.Spec.Containers {
		for name, quantity := range c.Resources.Requests {
			if val, ok := total[name]; ok {
				val.Add(quantity)
				total[name] = val
			} else {
				total[name] = quantity.DeepCopy()
			}
		}
	}
	return total
}

// Helper to sum two resource lists
func addResourceLists(a, b v1.ResourceList) v1.ResourceList {
	result := a.DeepCopy()
	for name, quantity := range b {
		if val, ok := result[name]; ok {
			val.Add(quantity)
			result[name] = val
		} else {
			result[name] = quantity.DeepCopy()
		}
	}
	return result
}

// Helper to compare resource usage with capacity
func isWithinCapacity(usage, total v1.ResourceList, percent int) bool {
	for name, totalQty := range total {
		capQty := totalQty.DeepCopy()
		capVal := int64(float64(capQty.Value()) * float64(percent) / 100.0)
		usageQty, ok := usage[name]
		if !ok {
			continue
		}
		if usageQty.Value() > capVal {
			return false
		}
	}
	return true
}

// Calculate total cluster resources (sum of all node allocatable)
func GetClusterTotalResources(clientset kubernetes.Interface) (v1.ResourceList, error) {
	nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	total := v1.ResourceList{}
	for _, node := range nodes.Items {
		for name, quantity := range node.Status.Allocatable {
			if val, ok := total[name]; ok {
				val.Add(quantity)
				total[name] = val
			} else {
				total[name] = quantity.DeepCopy()
			}
		}
	}
	return total, nil
}

// Modified SchedulePod to enforce queue capacity
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
	if !isWithinCapacity(futureUsage, clusterTotal, queue.Config.Capacity) {
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

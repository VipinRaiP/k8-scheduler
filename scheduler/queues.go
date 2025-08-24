package scheduler

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

var (
	rootQueue = &Queue{
		Name:     "root",
		Path:     "root",
		Children: make(map[string]*Queue),
		Config: QueueConfig{
			Capacity:    100,
			MaxCapacity: 100,
			Policy:      "fifo",
		},
	}
	queues = map[string]*Queue{
		"root": rootQueue,
	}
)

// CreateQueue creates a new queue at the specified path
func CreateQueue(name string, path string, config QueueConfig) error {
	if path == "" || path == "root" {
		return fmt.Errorf("invalid queue path")
	}

	parts := strings.Split(path, ".")
	current := rootQueue

	// Navigate through the hierarchy
	for i := 1; i < len(parts); i++ {
		if name == "" {
			name = parts[i]
		}
		child, exists := current.Children[parts[i]]
		if !exists {
			child = &Queue{
				Name:     name,
				Parent:   current,
				Path:     strings.Join(parts[:i+1], "."),
				Children: make(map[string]*Queue),
				Config:   config,
				ResourceUsage: v1.ResourceList{},
			}
			current.Children[parts[i]] = child
			queues[child.Path] = child // Add to global map,
		}
		current = child
	}
	queues[path] = current // Ensure latest queue is in map
	return nil
}

// GetQueue returns a queue by its path
func GetQueue(path string) *Queue {
	if path == "" || path == "root" {
		return rootQueue
	}
	if q, ok := queues[path]; ok {
		return q
	}
	// fallback to hierarchy search
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
		err := CreateQueue("", queuePath, QueueConfig{
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

func Dequeue(queuePath string) *v1.Pod {
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

// Helper to compute effective capacity percentage for a queue (relative to root)
func getEffectiveCapacityPercent(q *Queue) int {
	percent := q.Config.Capacity
	parent := q.Parent
	for parent != nil {
		percent = percent * parent.Config.Capacity / 100
		parent = parent.Parent
	}
	return percent
}

// Helper to compare resource usage with effective capacity
func isWithinCapacity(usage, total v1.ResourceList, queue *Queue) bool {
	effectivePercent := getEffectiveCapacityPercent(queue)
	for name, totalQty := range total {
		capQty := totalQty.DeepCopy()
		capVal := int64(float64(capQty.MilliValue()) * float64(effectivePercent) / 100.0)
		usageQty, ok := usage[name]
		if !ok {
			continue
		}
		// print usageQty and capVal for debugging
		fmt.Printf("Checking %s: usage=%d, capacity=%d\n", name, usageQty.MilliValue(), capVal)
		if usageQty.MilliValue() > capVal {
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

// UpdateQueueState updates or creates the queue state from CRD object
func UpdateQueueState(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		fmt.Printf("Failed to cast object to Unstructured\n")
		return
	}
	name := u.GetName()
	path, _, _ := unstructured.NestedString(u.Object, "spec", "path")
	capacity, _, _ := unstructured.NestedInt64(u.Object, "spec", "capacity")
	maxCapacity, _, _ := unstructured.NestedInt64(u.Object, "spec", "maxCapacity")
	policy, _, _ := unstructured.NestedString(u.Object, "spec", "policy")

	if path == "" {
		path = fmt.Sprintf("root.%s", name)
	}
	config := QueueConfig{
		Capacity:    int(capacity),
		MaxCapacity: int(maxCapacity),
		Policy:      policy,
	}

	q := GetQueue(path)
	if q != nil {
		// Update config only, keep pods and resource usage
		q.Config = config
		fmt.Printf("Queue config updated: %s\n", path)
	} else {
		// Create new queue
		err := CreateQueue(name, path, config)
		if err != nil {
			fmt.Printf("Error creating queue: %v\n", err)
			return
		}
		fmt.Printf("Queue created: %s\n", path)
	}
	queues[path] = GetQueue(path)
}

// DeleteQueueState removes the queue state for a deleted CRD object
func DeleteQueueState(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		fmt.Printf("Failed to cast object to Unstructured\n")
		return
	}
	path, _, _ := unstructured.NestedString(u.Object, "spec", "path")
	if path == "" {
		path = fmt.Sprintf("root.%s", u.GetName())
	}
	delete(queues, path)
	fmt.Printf("Queue deleted: %s\n", path)
}

package scheduler

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEnqueueDequeue(t *testing.T) {
	pod1 := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "ns1"}}
	pod2 := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod2", Namespace: "ns1"}}
	Enqueue(pod1)
	Enqueue(pod2)

	selected1 := Dequeue("ns1")
	if selected1 == nil || selected1.Name != "pod1" {
		t.Errorf("Expected pod1 to be dequeued first, got %v", selected1)
	}
	selected2 := Dequeue("ns1")
	if selected2 == nil || selected2.Name != "pod2" {
		t.Errorf("Expected pod2 to be dequeued second, got %v", selected2)
	}
	selected3 := Dequeue("ns1")
	if selected3 != nil {
		t.Errorf("Expected nil when queue is empty, got %v", selected3)
	}
}

func TestSelectBestNode_NoNodes(t *testing.T) {
	clientset := fake.NewSimpleClientset() // No nodes added
	_, err := SelectBestNode(clientset)
	if err == nil {
		t.Error("Expected error when no nodes are available")
	}
}

func TestSelectBestNode_ReadyNode(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&v1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node1"},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{{
					Type:   v1.NodeReady,
					Status: v1.ConditionTrue,
				}},
			},
		},
	)
	node, err := SelectBestNode(clientset)
	if err != nil {
		t.Errorf("Expected to find a ready node, got error: %v", err)
	}
	if node != "node1" {
		t.Errorf("Expected node1, got %s", node)
	}
}

func TestCustomQueue(t *testing.T) {
	// Reset rootQueue for test isolation
	rootQueue.Children = make(map[string]*Queue)

	// Create a custom queue hierarchy
	err := CreateQueue("root.teamA.subteam1", QueueConfig{Capacity: 30, MaxCapacity: 50, Policy: "fifo"})
	if err != nil {
		t.Fatalf("Failed to create custom queue: %v", err)
	}

	// Pod with custom queue annotation
	podCustom := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-custom",
			Namespace: "ns-custom",
			Annotations: map[string]string{
				"scheduler.kubernetes.io/queue": "root.teamA.subteam1",
			},
		},
	}
	// Pod without annotation (should go to root.ns-default)
	podDefault := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-default",
			Namespace: "ns-default",
		},
	}

	Enqueue(podCustom)
	Enqueue(podDefault)

	// Check podCustom is in the correct custom queue
	qCustom := GetQueue("root.teamA.subteam1")
	if qCustom == nil || len(qCustom.Pods) != 1 || qCustom.Pods[0].Name != "pod-custom" {
		t.Errorf("pod-custom not found in custom queue")
	}

	// Check podDefault is in the default namespace queue
	qDefault := GetQueue("root.ns-default")
	if qDefault == nil || len(qDefault.Pods) != 1 || qDefault.Pods[0].Name != "pod-default" {
		t.Errorf("pod-default not found in default namespace queue")
	}

	// Dequeue from custom queue
	deqCustom := qCustom.Pods[0]
	if deqCustom.Name != "pod-custom" {
		t.Errorf("Expected pod-custom, got %s", deqCustom.Name)
	}

	// Dequeue from default queue
	deqDefault := Dequeue("ns-default")
	if deqDefault == nil || deqDefault.Name != "pod-default" {
		t.Errorf("Expected pod-default, got %v", deqDefault)
	}
}

func TestHierarchicalQueueCapacity(t *testing.T) {
	// Reset rootQueue for test isolation
	rootQueue.Children = make(map[string]*Queue)

	// Create a hierarchy: root (100%) -> teamA (50%) -> subteam1 (20%)
	CreateQueue("root.teamA", QueueConfig{Capacity: 50, MaxCapacity: 100, Policy: "fifo"})
	CreateQueue("root.teamA.subteam1", QueueConfig{Capacity: 20, MaxCapacity: 100, Policy: "fifo"})

	// Simulate a cluster with 1000m CPU and 2Gi memory
	clusterResources := v1.ResourceList{
		v1.ResourceCPU:    resourceMustParse("1000m"),
		v1.ResourceMemory: resourceMustParse("2Gi"),
	}

	// Effective capacity for subteam1: 100% * 50% * 20% = 10% of cluster
	q := GetQueue("root.teamA.subteam1")
	if q == nil {
		t.Fatal("subteam1 queue not found")
	}
	percent := getEffectiveCapacityPercent(q)
	if percent != 10 {
		t.Errorf("Expected effective percent 10, got %d", percent)
	}

	// Pod requesting 200m CPU, 256Mi memory (should NOT fit, 200m > 100m allowed)
	pod := &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU:    resourceMustParse("200m"),
						v1.ResourceMemory: resourceMustParse("256Mi"),
					},
				},
			}},
		},
	}
	usage := getPodResourceRequests(pod)
	q.ResourceUsage = v1.ResourceList{} // reset
	if isWithinCapacity(usage, clusterResources, q) {
		t.Error("Pod should NOT fit within subteam1's effective capacity (CPU request too high)")
	}

	// Pod requesting 200m CPU, 512Mi memory (should NOT fit if over 10%)
	bigPod := &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU:    resourceMustParse("200m"),
						v1.ResourceMemory: resourceMustParse("512Mi"),
					},
				},
			}},
		},
	}
	// 10% of 2Gi = ~204Mi, so 512Mi > 204Mi
	usageBig := getPodResourceRequests(bigPod)
	if isWithinCapacity(usageBig, clusterResources, q) {
		t.Error("Big pod should NOT fit within subteam1's effective capacity")
	}

	// Pod requesting 50m CPU, 128Mi memory (should fit)
	fitPod := &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU:    resourceMustParse("50m"),
						v1.ResourceMemory: resourceMustParse("128Mi"),
					},
				},
			}},
		},
	}
	usageFit := getPodResourceRequests(fitPod)
	if !isWithinCapacity(usageFit, clusterResources, q) {
		t.Error("Pod with 50m CPU, 128Mi memory should fit within subteam1's effective capacity")
	}
}

// Helper for test: parse resource quantity and panic on error
func resourceMustParse(s string) resource.Quantity {
	q, err := resource.ParseQuantity(s)
	if err != nil {
		panic(err)
	}
	return q
}

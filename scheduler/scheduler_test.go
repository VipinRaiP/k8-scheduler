package scheduler

import (
	"testing"

	v1 "k8s.io/api/core/v1"
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

func TestCustomQueueHierarchy(t *testing.T) {
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

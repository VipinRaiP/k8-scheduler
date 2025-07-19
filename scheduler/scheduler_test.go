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

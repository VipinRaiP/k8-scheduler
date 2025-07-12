package scheduler

import (
    "context"
    "fmt"
    "k8s.io/client-go/kubernetes"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func SelectBestNode(clientset *kubernetes.Clientset) (string, error) {
    nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
    if err != nil || len(nodes.Items) == 0 {
        return "", fmt.Errorf("no nodes available")
    }

    // Pick first ready node (simplified)
    for _, node := range nodes.Items {
		fmt.Print("Checking node: ", node.Name, "\n")
        for _, cond := range node.Status.Conditions {
            if cond.Type == "Ready" && cond.Status == "True" {
                return node.Name, nil
            }
        }
    }

    return "", fmt.Errorf("no ready nodes")
}

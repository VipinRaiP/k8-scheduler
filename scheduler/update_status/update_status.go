package update_status

import (
    "context"
    "fmt"
    "k8s.io/apimachinery/pkg/types"
    "k8s.io/client-go/dynamic"
    "k8s.io/client-go/rest"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpdateQueueStatus updates the status.cpuUsage and status.memoryUsage fields of the Queue CRD
func UpdateQueueStatus(config *rest.Config, queueName string, cpuPercent, memPercent int) error {
    dynClient, err := dynamic.NewForConfig(config)
    if err != nil {
        return err
    }

    queueGVR := schema.GroupVersionResource{
        Group:    "kubescheduler.example.com",
        Version:  "v1",
        Resource: "queues",
    }

    patch := []byte(fmt.Sprintf(`{"status":{"cpuUsage":%d, "memoryUsage":%d}}`, cpuPercent, memPercent))
    _, err = dynClient.Resource(queueGVR).Namespace("").Patch(
        context.TODO(),
        queueName,
        types.MergePatchType,
        patch,
        v1.PatchOptions{},
        "status",
    )
    return err
}

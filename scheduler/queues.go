package scheduler

import (
    v1 "k8s.io/api/core/v1"
)

type Queue struct {
    Pods []*v1.Pod
}

var queues = map[string]*Queue{}

func Enqueue(pod *v1.Pod) {
    q, exists := queues[pod.Namespace]
    if !exists {
        q = &Queue{}
        queues[pod.Namespace] = q
    }
    q.Pods = append(q.Pods, pod)
}

func Dequeue(namespace string) *v1.Pod {
    q, exists := queues[namespace]
    if !exists || len(q.Pods) == 0 {
        return nil
    }
    pod := q.Pods[0]
    q.Pods = q.Pods[1:]
    return pod
}

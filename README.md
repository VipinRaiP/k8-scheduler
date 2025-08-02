# k8-scheduler

A custom Kubernetes scheduler written in Go, designed to demonstrate advanced scheduling concepts such as hierarchical queues, resource-based scheduling, and extensible scheduling policies.

## Features

- **Custom Hierarchical Queues**: Define queues in a hierarchy (e.g., `root.teamA.subteam1`) with configurable capacity and scheduling policy.
- **Queue Resource Capacity Enforcement**: Each queue can be assigned a capacity (as a percentage of its parent or the cluster), and pods are only scheduled if the queue's total resource usage stays within this limit.
- **Namespace and Annotation-based Queue Assignment**: Pods are assigned to queues based on a special annotation or by default to a namespace queue.
- **FIFO Scheduling Policy**: Queues currently use FIFO (first-in, first-out) scheduling, but the design allows for future extension to other policies.
- **Kubernetes API Integration**: Uses the Kubernetes Go client to watch for unscheduled pods and available nodes, and to bind pods to nodes.
- **Tested with Unit Tests**: Includes tests for queue logic, hierarchical capacity enforcement, and scheduling behavior.

## How It Works

1. **Queue Definition**: Queues are defined hierarchically, each with its own capacity and policy. For example, `root.teamA.subteam1` can be set to 20% of `teamA`, which is 50% of `root` (the cluster), so its effective capacity is 10% of the cluster.
2. **Pod Assignment**: Pods can specify their target queue via the annotation `scheduler.kubernetes.io/queue`. If not specified, they are assigned to a queue based on their namespace.
3. **Resource-based Scheduling**: Before a pod is scheduled, the scheduler checks if adding it would exceed the queue's effective resource capacity (CPU, memory, etc.).
4. **Scheduling Loop**: The scheduler continuously watches for unscheduled pods and attempts to schedule them according to the above rules.

## Example Queue Annotation

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: mypod
  annotations:
    scheduler.kubernetes.io/queue: "root.teamA.subteam1"
```

## Getting Started

1. Build and run the scheduler (see `main.go` for entry point).
2. Configure your kubeconfig for cluster access.
3. Deploy pods with the appropriate annotation or namespace.
4. Observe scheduling decisions and queue enforcement in the logs.

## TODO
- Populate current capacity for each queue in real time.
- Add more advanced scheduling policies (e.g., fair, priority-based).
- Improve metrics and monitoring.
- Add more comprehensive integration tests.

---

This project is for educational and experimental purposes.


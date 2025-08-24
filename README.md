# k8-scheduler

A custom Kubernetes scheduler written in Go, designed to demonstrate advanced scheduling concepts such as hierarchical queues, resource-based scheduling, and extensible scheduling policies.

## Features

- **Custom Hierarchical Queues**: Define queues in a hierarchy (e.g., `root.teamA.subteam1`) with configurable capacity and scheduling policy. Queues can be created and managed using Kubernetes Custom Resource Definitions (CRDs).
- **Queue Resource Capacity Enforcement**: Each queue can be assigned a capacity (as a percentage of its parent or the cluster), and pods are only scheduled if the queue's total resource usage stays within this limit. The scheduler updates the CRD status with current CPU and memory usage for each queue, enabling real-time monitoring via kubectl.
- **Namespace and Annotation-based Queue Assignment**: Pods are assigned to queues based on a special annotation or by default to a namespace queue.
- **FIFO Scheduling Policy**: Queues currently use FIFO (first-in, first-out) scheduling, but the design allows for future extension to other policies.
- **Kubernetes API Integration**: Uses the Kubernetes Go client to watch for unscheduled pods and available nodes, and to bind pods to nodes.
- **Tested with Unit Tests**: Includes tests for queue logic, hierarchical capacity enforcement, and scheduling behavior.

## How It Works

1. **Queue Definition**: Queues are defined hierarchically, each with its own capacity and policy. For example, `root.teamA.subteam1` can be set to 20% of `teamA`, which is 50% of `root` (the cluster), so its effective capacity is 10% of the cluster.
2. **Pod Assignment**: Pods can specify their target queue via the annotation `scheduler.kubernetes.io/queue`. If not specified, they are assigned to a queue based on their namespace.
3. **Resource-based Scheduling**: Before a pod is scheduled, the scheduler checks if adding it would exceed the queue's effective resource capacity (CPU, memory, etc.).
4. **Scheduling Loop**: The scheduler continuously watches for unscheduled pods and attempts to schedule them according to the above rules.



## Queue CRD Creation Example

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: queues.kubescheduler.example.com
spec:
  group: kubescheduler.example.com
  names:
    kind: Queue
    plural: queues
    singular: queue
  scope: Cluster
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                path:
                  type: string
                capacity:
                  type: integer
                maxCapacity:
                  type: integer
                policy:
                  type: string
            status:
              type: object
              properties:
                cpuUsage:
                  type: integer
                memoryUsage:
                  type: integer
      subresources:
        status: {}
```

## Example Queue CRD

```yaml
apiVersion: kubescheduler.example.com/v1
kind: Queue
metadata:
  name: engineering-queue
spec:
  path: root.engineering
  capacity: 50         # Percentage of cluster resources
  maxCapacity: 80      # Maximum capacity allowed
  policy: fifo         # Scheduling policy ("fifo", "fair", etc.)
```

## Example Queue CRD Status (populated by scheduler)

```yaml
status:
  cpuUsage: 25
  memoryUsage: 40
```

## Example Pod Annotation

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
- Real-time capacity tracking: Each queue's current CPU and memory usage is updated in its CRD status, visible via kubectl.
- Add more advanced scheduling policies (e.g., fair, priority-based, weighted round-robin, deadline-aware, resource guarantees).
- Support for pod priorities and preemption.
- Dynamic queue reconfiguration and autoscaling.
- Multi-cluster and cross-namespace scheduling.
- Integration with Kubernetes events and custom metrics.
- Web UI or CLI for queue management and visualization.
- Pluggable scheduling policies and admission controllers.
- Improve metrics, monitoring, and alerting (Prometheus/Grafana integration).
- Add more comprehensive integration and end-to-end tests.
- Documentation and usage examples for CRD management and troubleshooting.

---

This project is for educational and experimental purposes.
Vibe coded using Github copilot.



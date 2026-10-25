# Task autoscaling

Choosing a good task limit for Stroom's stream processing is hard. Too many tasks and a node can become
unresponsive or crash; too few and CPU sits idle. The right number varies through the day, and a static limit
risks over-committing nodes during spikes such as backlog processing or several large searches.

A `StroomTaskAutoscaler` adjusts a Stroom job's task limit on each processing node to keep the node's CPU usage
inside a target range.

## Requirements

- [metrics-server](https://github.com/kubernetes-sigs/metrics-server), which provides Pod CPU usage.
- A CPU limit (`resources.limits.cpu`) on each node set to be scaled. CPU usage is measured as a percentage of it,
  so node sets without one aren't scaled.
- Database access from the operator Pod. Task limits are read and written directly in the Stroom database.

## Create an autoscaler

```yaml
apiVersion: stroom.gchq.github.io/v1
kind: StroomTaskAutoscaler
metadata:
  name: dev
  namespace: stroom
spec:
  stroomClusterRef:
    name: dev                    # namespace defaults to the autoscaler's
  taskName: Data Processor
  adjustmentIntervalMins: 1
  metricsSlidingWindowMins: 5
  minCpuPercent: 50
  maxCpuPercent: 90
  minTaskLimit: 1
  maxTaskLimit: 8
  stepAmount: 1
```

```shell
kubectl apply -f autoscaler.yaml
```

See also [`samples/operator-database/autoscaler.yaml`](https://github.com/gradata-systems/stroom-k8s-operator/blob/master/samples/operator-database/autoscaler.yaml).
Each autoscaler keeps its own per-node state, so a `StroomCluster` can have several, for example to scale
different jobs. Don't point two autoscalers at the same job on the same cluster, as each adjusts the task limit
without regard to the other.

| Field | Default | Description |
| --- | --- | --- |
| `stroomClusterRef` | | The cluster to scale. |
| `taskName` | | The Stroom job whose task limit is adjusted, usually `Data Processor`. Must match the name in Stroom's _Jobs_ screen. |
| `adjustmentIntervalMins` | 1 | How often each node's limit may change. |
| `metricsSlidingWindowMins` | 1 | The window over which CPU usage is averaged. |
| `minCpuPercent` | 50 | Below this, the limit is raised (subject to the rule below). |
| `maxCpuPercent` | 90 | Above this, the limit is lowered. |
| `minTaskLimit`, `maxTaskLimit` | 1, 20 | Bounds for the limit. |
| `stepAmount` | 1 | How much the limit changes at each adjustment. |

## How it works

The operator samples the CPU usage of the `stroom-node` container of every Pod in every node set that isn't
`Frontend`, about every 10 seconds, and keeps the samples in memory for up to an hour. Each time
`adjustmentIntervalMins` has passed for a node, it averages the samples from the last `metricsSlidingWindowMins`
and compares the result, as a percentage of the CPU limit, with the target range:

- **Above `maxCpuPercent`:** the node's limit for `taskName` is lowered by `stepAmount`, but not below
  `minTaskLimit`.
- **Below `minCpuPercent`:** the limit is raised by `stepAmount`, up to `maxTaskLimit`, but **only if the node is
  already running as many tasks as its limit allows**. An idle node is left alone, so quiet periods don't push
  every node's limit up to the maximum.
- **Inside the range:** nothing changes.

Each change is logged by the operator, for example
`Updating task limit for node 'stroom-dev-node-data-0' from 4 to 5 due to CPU usage being at 42 percent`.

## Choosing values

- **`adjustmentIntervalMins`:** often enough to react to load spikes, but not so often that each change has no
  time to take effect.
- **`metricsSlidingWindowMins`:** long enough that a momentary spike doesn't cut the limit, short enough that a
  sustained spike does.
- **`minCpuPercent` and `maxCpuPercent`:** a fairly narrow range keeps the limit close to optimal.
- **`minTaskLimit` and `maxTaskLimit`:** safety bounds. The number of CPUs in the limit is a reasonable starting
  point for `maxTaskLimit`. Be realistic: if a node can never reach `maxTaskLimit` without overloading, the
  autoscaler may keep raising the limit until it does.

## Notes

- Samples are held in the operator's memory, so they are lost if the operator restarts, and scaling resumes once
  enough new samples have been collected. In very large clusters, give the operator more memory; see
  [Helm chart values](installation/helm.md#chart-values).
- The autoscaler changes the same setting as the _Jobs_ screen in Stroom, so manual changes to the job's task
  limit on a scaled node are overridden.

## Disable autoscaling

Delete the autoscaler. Task limits stay at their last values.

```shell
kubectl -n stroom delete stroomtaskautoscaler dev
```

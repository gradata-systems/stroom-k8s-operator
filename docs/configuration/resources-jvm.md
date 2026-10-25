# Resources and JVM memory

## Pod resources

Each node set must set `resources`, the standard container
[requests and limits](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/) for the
Stroom container:

```yaml
    - name: data
      resources:
        requests:
          cpu: "2"
          memory: 8Gi
        limits:
          cpu: "4"
          memory: 8Gi
      memoryOptions:
        initialPercentage: 50
        maxPercentage: 70
```

- Set a **memory limit**. The JVM sizes its heap from the container's memory limit; without one it uses the
  memory of the whole Kubernetes node.
- Setting the memory request equal to the limit avoids the node being evicted under memory pressure.
- Set a **CPU limit** if you use the [`StroomTaskAutoscaler`](../autoscaling.md): it measures CPU usage as a
  percentage of the limit, and its decisions are meaningless for node sets without one.

## JVM heap

`memoryOptions` sets the Java heap as a percentage of the container's memory limit. Both values are required and
must be between 1 and 100:

| Field | JVM option |
| --- | --- |
| `initialPercentage` | `-XX:InitialRAMPercentage` |
| `maxPercentage` | `-XX:MaxRAMPercentage` |

Stroom uses a lot of memory outside the Java heap, notably for LMDB (reference data and other stores), direct
buffers, metaspace and threads, so leave headroom. A `maxPercentage` of around 50 to 75 is a sensible range; lower
it if containers are OOMKilled.

## Extra JVM options

`spec.extraJvmOpts` adds options to every node, after the memory options:

```yaml
spec:
  extraJvmOpts:
    - -XX:+HeapDumpOnOutOfMemoryError
    - -XX:HeapDumpPath=/stroom/logs
```

## Overriding `JAVA_OPTS`

The operator builds the `JAVA_OPTS` environment variable from `memoryOptions` and `extraJvmOpts`. If you set
`JAVA_OPTS` in `spec.extraEnv`, your value is used **instead**, and `memoryOptions` and `extraJvmOpts` are ignored
for every node set.

To check the options a node is using:

```shell
kubectl -n stroom exec stroom-dev-node-data-0 -c stroom-node -- printenv JAVA_OPTS
```

## The operator's own resources

To change the resources of the operator Pod, see [Helm chart values](../installation/helm.md#chart-values).

# Node sets and roles

A node set is a group of identical Stroom nodes with a common role. The operator creates one StatefulSet for each
entry in `spec.nodeSets`.

```yaml
spec:
  nodeSets:
    - name: data
      count: 3
      role: Processing
      # ...
    - name: ui
      count: 1
      role: Frontend
      # ...
```

## Names

Each node's Pod name, which is also its Stroom node name, is `stroom-<cluster name>-node-<node set name>-<ordinal>`.
For example, the first node of node set `data` in cluster `dev` is `stroom-dev-node-data-0`. Keep node set names
short. Changing a node set's name creates a new set of nodes; see [Removing a node set](../operations/scaling.md#removing-a-node-set).

## Roles

| `role` | Receives event data | Serves the UI and API | Managed jobs (default) | Autoscaled |
| --- | --- | --- | --- | --- |
| `Processing` | Yes | No | All jobs | Yes |
| `Frontend` | No | Yes | `Reindex Content` only | No |
| Omitted | Yes | Yes | All jobs | Yes |

The routing is done by the Ingresses the operator creates. See [Ingress and TLS](ingress-tls.md#routing).

Recommended layouts:

- **Small or lightly loaded clusters:** a single node set with no `role`.
- **Larger clusters:** one `Processing` node set and one `Frontend` node set. Dedicated frontend nodes keep the
  user interface responsive however busy the processing nodes are.

Don't create more than one node set with Ingress enabled for the same role. The operator names Ingresses after the
cluster, not the node set, so a second node set with the same role overwrites the first one's Ingress. A node set
with no role creates both Ingresses, so it collides with any `Processing` or `Frontend` node set.

If you need extra processing capacity that doesn't receive data, add a node set with `role: Processing` and
`ingressEnabled: false`, and list it **after** the node set that receives data: `/stroom/noauth/datafeed` is
routed to the first node set that isn't `Frontend`, whether or not its Ingress is enabled.

## Managed jobs

When a node starts, the operator's startup script enables the node and a list of Stroom jobs on it. Before the node
stops, the `preStop` script disables them again so the node can drain. `managedJobs` sets that list:

```yaml
    - name: data
      role: Processing
      managedJobs:
        - Data Processor
```

If you omit `managedJobs` (or leave it empty):

- `Processing` nodes, and nodes with no role, enable and disable **all** jobs on the node.
- `Frontend` nodes enable and disable only `Reindex Content`.

When a node starts or stops, jobs outside the list are left as they are, so you can manage them in Stroom's _Jobs_
screen. Deleting the whole `StroomCluster` is different: it disables every job (see
[Stopping a Stroom cluster](../operations/stopping-and-removal.md#stopping-a-stroom-cluster)). On a `Frontend`
node, for example, the operator doesn't disable `Data Processor`: it simply never enables it. If you enable it by
hand, it stays enabled.

Job names must match the names in Stroom's _Jobs_ screen exactly.

## Count

`count` is the number of nodes (StatefulSet replicas), at least 1. See [Scaling](../operations/scaling.md) for
what happens when you change it.

## Pod management policy

`spec.podManagementPolicy` applies to every node set:

- **`OrderedReady`** (the Kubernetes default): nodes start one at a time, each waiting for the previous one to be
  `Ready`. Slow for large node sets, but only one node at a time competes for Stroom's database bootstrap lock.
- **`Parallel`**: all nodes in a node set start together. Faster, but while one node creates or migrates the
  database schema the others wait for its lock. The operator lengthens the startup probe to allow for this; see
  [Startup probe](health-probes.md#startup-probe).

The policy can't be changed once the StatefulSets exist. Rolling updates of an existing node set happen one node at
a time under either policy.

## Scheduling

Each node set has the usual Pod scheduling and security settings: `nodeSelector`, `tolerations`, `affinity`,
`podAnnotations`, `podSecurityContext` (Pod level) and `securityContext` (the Stroom container).

To spread a node set's Pods across Kubernetes nodes, use the labels the operator sets on every Pod:

```yaml
    - name: data
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              podAffinityTerm:
                topologyKey: kubernetes.io/hostname
                labelSelector:
                  matchLabels:
                    stroom/cluster: dev
                    stroom/nodeSet: data
```

By default the Stroom image's entrypoint starts as root, makes its directories owned by the `stroom` user (ID
`1000`), then runs Stroom as that user. To run the container as that user from the start, set the Pod security
context. The entrypoint then skips changing ownership, so add `fsGroup: 1000` to make the node's volumes writable,
on storage that honours `fsGroup`:

```yaml
      podSecurityContext:
        runAsNonRoot: true
        runAsUser: 1000
        runAsGroup: 1000
        fsGroup: 1000
```

An existing node set can be changed to run as the `stroom` user: Stroom's own files already belong to it, and the
node lifecycle scripts give their logs (`/stroom/logs/k8s`) to it as well.

## Other node set settings

| Field | See |
| --- | --- |
| `localDataVolumeClaim`, `extraVolumeMounts` | [Storage](storage.md) |
| `resources`, `memoryOptions` | [Resources and JVM memory](resources-jvm.md) |
| `ingressEnabled`, `ingressAnnotations`, `ingressLabels` | [Ingress and TLS](ingress-tls.md) |
| `startupProbeTimings`, `readinessProbeTimings`, `livenessProbeTimings` | [Health probes](health-probes.md) |

# Scaling

## Changing the number of nodes

Change the node set's `count` and apply the manifest:

```yaml
    - name: data
      count: 5
```

```shell
kubectl apply -f stroom-cluster.yaml
kubectl -n stroom get pods -l stroom/cluster=dev,stroom/nodeSet=data -w
```

- **Scaling up** adds nodes with the next ordinals (`stroom-dev-node-data-3`, `-4`, ...). Each gets a new
  PersistentVolumeClaim, or reuses the one left by an earlier node with the same ordinal.
- **Scaling down** removes the highest ordinals first. Each removed node is drained before it stops, within the
  [termination grace period](node-lifecycle.md#termination-grace-period).

Removed nodes stay in Stroom's list of nodes, disabled. With `volumeClaimDeletePolicy` set, the removed nodes'
claims are deleted; see [Deleting PersistentVolumeClaims](../configuration/storage.md#deleting-persistentvolumeclaims).

!!! note "Parallel node sets roll when scaled"
    With `podManagementPolicy: Parallel`, the startup probe allowance depends on `count`, so changing `count` also
    rolls the node set's existing nodes. See [Health probes](../configuration/health-probes.md#parallel-pod-management).

Don't scale with `kubectl scale statefulset`: the operator resets the replica count to the `StroomCluster`'s
`count` the next time it reconciles.

## Adding a node set

Add an entry to `spec.nodeSets` and apply. The operator creates its StatefulSet and Services. Bear in mind the
[Ingress rules for roles](../configuration/node-sets.md#roles).

## Removing a node set

Removing an entry from `spec.nodeSets` (or renaming it) does **not** remove its nodes: the operator stops managing
the StatefulSet but doesn't delete it. To remove the old node set's nodes, delete its StatefulSet and Services
after removing it from the manifest:

```shell
kubectl -n stroom delete statefulset stroom-dev-node-old
kubectl -n stroom delete service stroom-dev-node-old stroom-dev-node-old-http
```

Each node is drained as it stops. Its PersistentVolumeClaims are kept; delete them when you no longer need them:

```shell
kubectl -n stroom delete pvc -l stroom/cluster=dev,stroom/nodeSet=old
```

## Scaling vertically

Changing a node set's `resources` or `memoryOptions` rolls the node set with the new values. See
[Resources and JVM memory](../configuration/resources-jvm.md).

## Scaling processing

To scale the number of tasks each processing node runs, rather than the number of nodes, see
[Task autoscaling](../autoscaling.md).

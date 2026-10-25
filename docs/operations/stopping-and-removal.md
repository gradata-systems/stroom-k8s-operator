# Stopping and removal

## Stopping a Stroom cluster

To stop a cluster, delete its `StroomCluster`:

```shell
kubectl -n stroom delete stroomcluster dev
```

Before the cluster's objects are removed, the operator drains it:

1. It disables every job on each of the cluster's nodes, directly in the Stroom database. Nodes of other clusters
   sharing the database aren't affected.
2. It checks the database once a minute for processing tasks still running on the cluster's nodes, logging which
   nodes are holding up the deletion. For example:
   `StroomCluster deletion waiting on task completion for 1 nodes: stroom-dev-node-data-0 (5)`.
3. When no tasks remain, it allows the deletion to proceed. Kubernetes deletes the StatefulSets and their Pods (each
   of which also runs its [`preStop` drain](node-lifecycle.md#stopping)), and the operator deletes the cluster's
   Ingresses.

If no Stroom node has ever started against the database, so it has no Stroom tables, there's nothing to drain and
the deletion proceeds straight away. If the operator can't connect to the database, it keeps retrying, and the
deletion waits until it can.

`kubectl delete` waits while this happens, which can take a while. You can press ++ctrl+c++; the deletion
continues in the background. To see progress:

```shell
kubectl -n stroom-operator-system logs deployment/stroom-operator-controller-manager -f | grep -i 'deletion\|drain'
```

What's kept:

- The nodes' PersistentVolumeClaims, unless `volumeClaimDeletePolicy` is `DeleteOnScaledownAndClusterDeletion`.
- The database, and with it all of Stroom's content and metadata.
- Any `StroomTaskAutoscaler`, which does nothing while the cluster is absent. Delete it too if you're not going to
  recreate the cluster.

To start the cluster again, apply the same `StroomCluster` manifest. Each node reattaches to its previous claim.
Step 1 disabled **every** job, but on start each node re-enables only its
[managed jobs](../configuration/node-sets.md#managed-jobs), so re-enable any jobs you manage by hand in Stroom.

## Deleting a database server

Delete the `DatabaseServer` after the `StroomCluster`. If you delete it first, its deletion waits until the
cluster has gone, so the order doesn't matter, but the database is never removed from under a running cluster.

```shell
kubectl -n stroom delete databaseserver dev
```

The data volume claim (`data-stroom-dev-db-0`) and the credentials Secret (`stroom-dev-db`) are kept. Delete them
yourself only when you're sure you no longer need the data.

Delete any `DatabaseBackup` as well; its CronJob is deleted with it.

## Removing the operator

Remove the Stroom resources first, then the operator, then the CRDs.

1. Delete every `StroomCluster` (and wait for the deletions to finish), `DatabaseServer`, `DatabaseBackup` and
   `StroomTaskAutoscaler`:

    ```shell
    kubectl get stroomclusters,databaseservers,databasebackups,stroomtaskautoscalers --all-namespaces
    ```

2. Uninstall the operator:

    ```shell
    helm uninstall stroom-operator --namespace stroom-operator-system
    ```

3. Uninstall the CRDs:

    ```shell
    helm uninstall stroom-operator-crds --namespace stroom-operator-system
    ```


!!! danger "Order matters"
    - Deleting the CRDs deletes every resource of those kinds, in every namespace.
    - The operator's finalizers keep `StroomCluster` and `DatabaseServer` resources from disappearing until the
      operator has drained them. If the operator is no longer running, their deletion, and so the CRDs' deletion,
      hangs until it's reinstalled.

Uninstalling just the operator (step 2) while clusters exist is safe: Stroom keeps running. But nothing reconciles
the clusters while it's gone, task autoscaling stops, and the clusters can't be deleted until it returns.

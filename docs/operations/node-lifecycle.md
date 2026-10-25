# Node lifecycle, restarts and draining

The operator adds two scripts to every Stroom node, so that Kubernetes starts and stops nodes in a way that suits
Stroom. Both authenticate to the Stroom API with a token obtained using `spec.openId` (see
[Identity provider](../configuration/identity-provider.md)), and both log to the node's volume.

## Starting

When a node's container starts, Stroom boots. If the database schema needs creating or migrating, one node takes
Stroom's bootstrap lock and does it while the others wait.

Meanwhile, the startup probe runs `/stroom/scripts/node-start.sh` every `periodSeconds`. Once Stroom's API responds,
the script:

1. enables the node in Stroom, and
2. enables the node's [managed jobs](../configuration/node-sets.md#managed-jobs).

The probe then succeeds and the readiness and liveness probes take over. If it hasn't succeeded within the startup
window, Kubernetes restarts the container. See [Health probes](../configuration/health-probes.md#startup-probe).

Log: `/stroom/logs/k8s/node-start.log`.

## Stopping

Whenever a node's Pod is deleted, whether by a rolling update, a scale-down, `kubectl delete pod`, a Kubernetes node
drain or deletion of the cluster, Kubernetes runs the `preStop` hook, `/stroom/scripts/node-pre-stop.sh`, before
stopping the container. The script:

1. disables the node's managed jobs, so it takes no new tasks,
2. disables the node in Stroom, so the rest of the cluster stops contacting it,
3. checks the node's active task count every 5 seconds, until it reaches zero.

Then Stroom is sent `SIGTERM` and shuts down.

If a task count request fails, the script logs an error and tries again; it no longer treats a failure as "no
tasks". If the first two steps fail, for example because the IdP is unreachable, the hook exits immediately and the
node is stopped **without draining**.

Log: `/stroom/logs/k8s/pre-stop.log`.

### Termination grace period

`spec.nodeTerminationPeriodSecs` (default `60`) is the Pod's termination grace period. The drain and Stroom's own
shutdown must both finish within it; when it runs out, Kubernetes kills the container, and any tasks still running
fail.

Set it longer than your longest-running processing task. Setting it too long has a cost too: a hung node takes
that long to be killed. A change takes effect for Pods created after it (it is part of the Pod template, so
applying it rolls the nodes).

## Restarting a node

To restart a hung or failed node, delete its Pod. The StatefulSet recreates it with the same name and volume:

```shell
kubectl -n stroom delete pod stroom-dev-node-data-0
```

The node is drained first, as above. The command waits until the Pod has gone; press ++ctrl+c++ to stop waiting
(the deletion continues).

### Force deletion

If you can't wait for the grace period, and either the node has no active tasks or you accept that they will fail,
force the deletion, as described in the
[Kubernetes documentation](https://kubernetes.io/docs/tasks/run-application/force-delete-stateful-set-pod/):

```shell
kubectl -n stroom delete pod stroom-dev-node-data-0 --grace-period=0 --force
```

!!! danger
    Only force-delete a Pod when you are sure its container has stopped, or its Kubernetes node is gone for good.
    Otherwise two copies of the same Stroom node can run at once.

## Rolling restarts

To restart every node in a node set, one at a time and each drained first, change something in its Pod template.
The simplest way is a Pod annotation in the `StroomCluster`:

```yaml
    - name: data
      podAnnotations:
        restartedAt: "2026-10-25T10:00:00Z"
```

```shell
kubectl apply -f stroom-cluster.yaml
kubectl -n stroom rollout status statefulset/stroom-dev-node-data
```

Avoid `kubectl rollout restart statefulset/...`. It works by adding an annotation to the StatefulSet's Pod
template, which the operator removes again when it next reconciles. The rollout is then reverted part-way
through, and some nodes restart twice.

For a few nodes, deleting their Pods one at a time, waiting for each replacement to be `Ready`, also works.

## Draining Kubernetes nodes

`kubectl drain` evicts Pods, and each evicted Stroom node is drained by its `preStop` hook. The operator doesn't
create PodDisruptionBudgets. To stop a Kubernetes node drain from taking down several Stroom nodes of a node set at
once, create one per node set:

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: stroom-dev-node-data
  namespace: stroom
spec:
  maxUnavailable: 1
  selector:
    matchLabels:
      stroom/cluster: dev
      stroom/nodeSet: data
```

## Recreating a StatefulSet

Some StatefulSet fields can't be changed in place, notably the volume claim template (`localDataVolumeClaim`) and
`podManagementPolicy`. To apply a change to them, delete the StatefulSet without deleting its Pods, and let the
operator create it again:

```shell
kubectl apply -f stroom-cluster.yaml   # with the change
kubectl -n stroom delete statefulset stroom-dev-node-data --cascade=orphan
kubectl -n stroom get statefulset stroom-dev-node-data -w
```

The new StatefulSet adopts the running Pods, and replaces them in a rolling update if their template has changed.
Existing PersistentVolumeClaims are kept and aren't changed: a new volume claim template applies only to claims
created from now on.

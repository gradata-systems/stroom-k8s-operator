# Configuring a StroomCluster

A `StroomCluster` describes the Stroom nodes, how they are grouped and exposed, and how they connect to the
database and identity provider. The [`samples`](https://github.com/gradata-systems/stroom-k8s-operator/tree/master/samples)
directory has complete example deployments:

| Sample | Shows |
| --- | --- |
| [`operator-database`](https://github.com/gradata-systems/stroom-k8s-operator/tree/master/samples/operator-database) | A MySQL server managed by the operator, with backups, processing and frontend node sets, task autoscaling, log shipping and NetworkPolicies |
| [`external-database`](https://github.com/gradata-systems/stroom-k8s-operator/tree/master/samples/external-database) | A MySQL server the operator doesn't manage |
| [`external-idp`](https://github.com/gradata-systems/stroom-k8s-operator/tree/master/samples/external-idp) | A custom Stroom configuration that authenticates with Keycloak, included by both deployments |

Each is applied with `kubectl apply -k`. The [quick start](../getting-started/quick-start.md) builds a minimal
cluster step by step.

## Top-level settings

| Field | Required | Purpose | See |
| --- | --- | --- | --- |
| `image` | Yes | Stroom image `repository` and `tag`, for example `gchq/stroom` and `v7.13.2`. | [Upgrading](../operations/upgrading.md#upgrading-stroom) |
| `imagePullPolicy` | No | Pull policy for the Stroom image (and the keystore init container). | |
| `databaseServerRef` | Yes | An operator-managed `DatabaseServer`, or the address of an external database. | [Database](../database/database-server.md) |
| `appDatabaseName`, `statsDatabaseName` | Defaulted | Database names. Default `stroom` and `stats`. | [External database](../database/external-database.md) |
| `openId` | Yes | OpenID client credentials used by the operator's node lifecycle scripts. | [Identity provider](identity-provider.md) |
| `ingress` | Yes | Public host name, Ingress class and TLS Secret. | [Ingress and TLS](ingress-tls.md) |
| `https` | No | Serve HTTPS from the Stroom nodes themselves. | [Ingress and TLS](ingress-tls.md#https-inside-the-cluster) |
| `configMapRef` | No | Replace the bundled Stroom `config.yml`. | [Stroom configuration](stroom-config.md) |
| `nodeSets` | Yes | One or more groups of Stroom nodes. | [Node sets](node-sets.md) |
| `podManagementPolicy` | No | `OrderedReady` (the Kubernetes default) or `Parallel`. | [Node sets](node-sets.md#pod-management-policy) |
| `nodeTerminationPeriodSecs` | Defaulted | Time allowed for a node to drain and stop. Default `60`. | [Node lifecycle](../operations/node-lifecycle.md#termination-grace-period) |
| `volumeClaimDeletePolicy` | No | When to delete the nodes' PersistentVolumeClaims. | [Storage](storage.md#deleting-persistentvolumeclaims) |
| `extraJvmOpts` | No | Extra JVM options for every node. | [Resources and JVM memory](resources-jvm.md) |
| `extraEnv` | No | Extra environment variables for every node. | [Stroom configuration](stroom-config.md#extra-environment-variables) |
| `extraVolumes`, `extraVolumeMounts` | No | Extra volumes mounted into every node. | [Storage](storage.md#shared-storage-for-stroom-volumes) |
| `logSender` | No | Ship Stroom's audit and application logs into Stroom. | [Log shipping](log-shipping.md) |

Each node set requires `name`, `count`, `localDataVolumeClaim`, `resources` and `memoryOptions`. The
[CRD reference](../reference/api.md) lists every field. You can also ask the cluster:

```shell
kubectl explain stroomcluster.spec --recursive | less
kubectl explain stroomcluster.spec.nodeSets.startupProbeTimings
```

## Applying changes

Edit your manifest and apply it again:

```shell
kubectl apply -f stroom-cluster.yaml
```

The operator reconciles the StatefulSets, Services and Ingresses to match. Most changes alter the Pod template
and cause a [rolling restart](../operations/node-lifecycle.md#rolling-restarts) of the affected node sets, one node
at a time, with each node drained first.

Some changes can't be applied in place:

| Change | What happens | What to do |
| --- | --- | --- |
| `localDataVolumeClaim` or `podManagementPolicy` | Kubernetes rejects the StatefulSet update, because these fields are immutable. The operator logs the error and keeps retrying, and changes to later node sets aren't applied either. | Revert the change, or see [Recreating a StatefulSet](../operations/node-lifecycle.md#recreating-a-statefulset). |
| Removing or renaming a node set | The operator doesn't delete the old node set's StatefulSet or Services; the old nodes keep running. | Delete the old StatefulSet yourself. See [Scaling](../operations/scaling.md#removing-a-node-set). |
| Disabling a node set's Ingress | Existing Ingresses aren't deleted until the cluster is deleted. | Delete or edit the Ingress yourself if required. |
| The contents of a referenced ConfigMap or Secret | Running Pods don't see the change. | Restart the nodes. See [Stroom configuration](stroom-config.md#changing-the-configuration). |

!!! tip "Keep your manifests in version control"
    The `StroomCluster` manifest is the definition of your cluster. Keep it, and your `DatabaseServer`,
    `DatabaseBackup` and `StroomTaskAutoscaler` manifests, in Git.

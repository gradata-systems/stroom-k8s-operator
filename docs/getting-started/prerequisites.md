# Prerequisites

## Kubernetes cluster

- A currently supported Kubernetes release. The operator manages StatefulSets, Services, Ingresses
  (`networking.k8s.io/v1`) and CronJobs (`batch/v1`), and the autoscaler reads Pod metrics from
  `metrics.k8s.io/v1beta1`.
- `kubectl`, and cluster-admin rights for the installation. The installation creates CRDs, a ClusterRole and a
  ClusterRoleBinding.
- [Helm](https://helm.sh/) 3.8 or later, if you install with Helm. The charts are published as OCI artifacts.
- An **Ingress controller**. The operator adds [ingress-nginx](https://kubernetes.github.io/ingress-nginx/)
  annotations by default. Other controllers work if you supply equivalent annotations, and ingress-nginx is now
  retired; see [Ingress and TLS](../configuration/ingress-tls.md#other-ingress-controllers).
- [metrics-server](https://github.com/kubernetes-sigs/metrics-server), if you want to use the
  [`StroomTaskAutoscaler`](../autoscaling.md). Some Kubernetes distributions include it.

## Storage

- **Local storage for each Stroom node.** Each node gets its own `ReadWriteOnce` PersistentVolumeClaim, from a
  StorageClass or from PersistentVolumes you create in advance. It holds logs, LMDB reference data, search
  results and other node-local state, so use fast, low-latency storage. See [Storage](../configuration/storage.md).
- **Shared storage for Stroom's data and index volumes**, mounted into every node (for example NFS). You mount
  this yourself through `spec.extraVolumes` and `spec.extraVolumeMounts`.
- **Database storage**, if you use an operator-managed `DatabaseServer`. Use fast, low-latency storage.

## Database

Stroom 7 requires MySQL 8. Either:

- let the operator run a single-instance MySQL server (a [`DatabaseServer`](../database/database-server.md)),
  which suits evaluation and smaller deployments, or
- provide an [external MySQL 8 server](../database/external-database.md), for example a managed or replicated
  service, which is the better choice for production.

## Identity provider

Stroom must use an **external** OpenID Connect identity provider (IdP) such as Keycloak, Microsoft Entra ID or
Amazon Cognito. You need:

- A confidential client for Stroom with a client ID and secret.
- The client credentials grant enabled for that client. The operator's scripts use it to get tokens.
- The Stroom redirect URI registered with the IdP.
- Network access from the Stroom Pods to the IdP.

See [Identity provider](../configuration/identity-provider.md) for the details and the reason this is required.

## TLS

- A certificate for the public host name, used by the Ingress.
- If you enable [HTTPS inside the cluster](../configuration/ingress-tls.md#https-inside-the-cluster), a Secret
  containing `tls.crt`, `tls.key` and `ca.crt` for the Stroom nodes. A Secret issued by
  [cert-manager](https://cert-manager.io/) has this layout.

## Container images

The following images must be available to your cluster, either from their public registries or mirrored into a
private registry (see [Air-gapped environments](../installation/air-gapped.md)):

| Image | Used for |
| --- | --- |
| `ghcr.io/gradata-systems/stroom-k8s-operator` | The operator |
| `gchq/stroom` (7.13 release, for example `v7.13.2`) | Stroom nodes |
| A MySQL 8 server image | `DatabaseServer` and `DatabaseBackup` (the backup job needs `bash`, `mysqldump` and `gzip`) |
| `gchq/stroom-log-sender` | Only if [log shipping](../configuration/log-shipping.md) is enabled |

## Network access

| From | To | Why |
| --- | --- | --- |
| Operator Pod | Kubernetes API | Normal operation |
| Operator Pod | MySQL, port 3306 | Draining the cluster on deletion; task autoscaling |
| Stroom Pods | MySQL, port 3306 | Stroom itself |
| Stroom Pods | Identity provider | User sign-in and tokens for the node lifecycle scripts |
| Stroom Pods | Each other, ports 8080, 8443 and 8081 | Node-to-node communication |

Bear these in mind if you use NetworkPolicies.

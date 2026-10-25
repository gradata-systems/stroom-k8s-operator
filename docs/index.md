# Stroom K8s Operator

The Stroom K8s Operator deploys and runs [Stroom](https://github.com/gchq/stroom) clusters on
[Kubernetes](https://kubernetes.io/). You describe the cluster you want in a few custom resources, and the
operator creates and maintains the StatefulSets, Services, Ingresses, configuration and database that Stroom
needs. It also handles the parts of running Stroom that plain Kubernetes doesn't know about, such as draining a
node's processing tasks before it stops.

This documentation is for people who deploy and run Stroom on Kubernetes. If you want to change the operator
itself, see the [README](https://github.com/gradata-systems/stroom-k8s-operator#readme) in the repository.

## What the operator does

**Deployment**

- Defines a whole Stroom cluster (nodes, roles, storage, ingress, TLS, identity provider and database) in one
  `StroomCluster` resource.
- Splits nodes into dedicated `Processing` and `Frontend` node sets, so the user interface stays responsive
  whatever the processing load, and routes event data only to the nodes that process it.
- Optionally runs a single-instance MySQL server for the cluster (`DatabaseServer`) and generates its
  credentials.
- Installs from Helm charts published as OCI artifacts, or from a single manifest.

**Operations**

- Drains Stroom processing tasks from a node before it shuts down, whether for an upgrade, a rescheduled Pod or
  deletion of the whole cluster.
- Enables a node and its jobs when it starts, and disables them when it stops.
- Rolls out Stroom version upgrades one node at a time.
- Schedules database backups (`DatabaseBackup`).
- Ships Stroom's audit and application logs back into Stroom.
- Tunes each processing node's task limit to keep CPU usage within a target range (`StroomTaskAutoscaler`).

## Before you start

!!! warning "An external identity provider is required"
    The operator authenticates to the Stroom API with an OpenID Connect client credentials token, which Stroom
    only issues when it is configured to use an **external** identity provider (for example Keycloak, Microsoft
    Entra ID or Amazon Cognito). Stroom's internal identity provider is not supported. See
    [Identity provider](configuration/identity-provider.md).

| Component | Supported |
| --- | --- |
| Stroom | 7.13 (the baseline for this documentation) |
| Database | MySQL 8, either operator-managed or external |
| Kubernetes | A currently supported release, with an Ingress controller |
| Helm | 3.8 or later (for OCI chart support) |

## Where to go next

- New to the operator? Read the [architecture overview](getting-started/architecture.md), check the
  [prerequisites](getting-started/prerequisites.md), then follow the [quick start](getting-started/quick-start.md).
- Installing for real? See [Installation](installation/index.md) and
  [Configuring a StroomCluster](configuration/index.md).
- Already running? See [Operations](operations/upgrading.md) and
  [Troubleshooting](operations/troubleshooting.md).
- Looking for a field? See the [CRD reference](reference/api.md).

!!! note "Where this documentation came from"
    Parts of these pages were adapted from the Kubernetes section of the
    [Stroom documentation](https://gchq.github.io/stroom-docs/docs/install-guide/kubernetes/)
    ([gchq/stroom-docs](https://github.com/gchq/stroom-docs)), which is licensed under the Apache License 2.0.
    The content has since been reorganised and updated. See [Attribution](about/attribution.md).

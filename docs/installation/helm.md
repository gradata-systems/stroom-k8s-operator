# Install with Helm (OCI)

The operator is published as two Helm charts in the GitHub Container Registry:

| Chart | Contents |
| --- | --- |
| `oci://ghcr.io/gradata-systems/helm-charts/stroom-operator-crds` | The four CRDs. |
| `oci://ghcr.io/gradata-systems/helm-charts/stroom-operator` | The controller Deployment, RBAC and ServiceAccount. |

The CRDs are a separate chart so that uninstalling or reinstalling the controller never deletes them. Deleting a
CRD deletes every resource of that kind in the cluster, including every `StroomCluster`.

## Install

Install the CRDs first, then the operator:

```shell
helm install stroom-operator-crds \
  oci://ghcr.io/gradata-systems/helm-charts/stroom-operator-crds \
  --namespace stroom-operator-system --create-namespace

helm install stroom-operator \
  oci://ghcr.io/gradata-systems/helm-charts/stroom-operator \
  --namespace stroom-operator-system
```

Without `--version`, Helm installs the latest chart. To install a specific version, which is advisable in
production, add `--version <chart version>` to both commands and use the same version for each. To find the
latest version and the operator version it deploys (`appVersion`):

```shell
helm show chart oci://ghcr.io/gradata-systems/helm-charts/stroom-operator
```

## Check the installation

```shell
kubectl -n stroom-operator-system rollout status deployment/stroom-operator-controller-manager
kubectl get crds | grep stroom.gchq.github.io
```

You should see four CRDs: `stroomclusters`, `databaseservers`, `databasebackups` and `stroomtaskautoscalers`.

To follow the operator's log:

```shell
kubectl -n stroom-operator-system logs deployment/stroom-operator-controller-manager -f
```

## Chart values

The `stroom-operator` chart accepts these values:

| Value | Default | Description |
| --- | --- | --- |
| `registry` | `""` | Private registry to pull the operator image from, in place of the registry in `image.repository`. The image becomes `<registry>/gradata-systems/stroom-k8s-operator:<tag>`. |
| `image.repository` | `ghcr.io/gradata-systems/stroom-k8s-operator` | Operator image repository. |
| `image.tag` | Chart `appVersion` | Operator image tag. |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy. |
| `resources` | Requests `cpu: 500m`, `memory: 512Mi`; limit `memory: 1Gi` | Controller resources. |
| `securityContext` | Non-root (UID/GID 65532), `RuntimeDefault` seccomp | Pod security context. |
| `nodeSelector`, `tolerations`, `affinity` | Empty | Controller scheduling. |

The `stroom-operator-crds` chart has no values.

For example, to give the controller more memory:

```shell
helm upgrade stroom-operator \
  oci://ghcr.io/gradata-systems/helm-charts/stroom-operator \
  --namespace stroom-operator-system \
  --reuse-values --set resources.limits.memory=2Gi
```

!!! tip "Controller memory"
    The `StroomTaskAutoscaler` keeps up to 60 minutes of CPU samples in memory for every processing node it
    manages. In large deployments, raise the controller's memory limit if it is OOMKilled.

## Upgrade and removal

See [Upgrading](../operations/upgrading.md#upgrading-the-operator) and
[Stopping and removal](../operations/stopping-and-removal.md#removing-the-operator).

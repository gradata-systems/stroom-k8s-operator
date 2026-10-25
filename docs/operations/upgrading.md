# Upgrading

## Upgrading the operator

### With Helm

Upgrade the CRDs first, so that any new fields and defaults are in place before the new controller starts, then
the operator. Use the same chart version for both:

```shell
version=<chart version>

helm upgrade stroom-operator-crds \
  oci://ghcr.io/gradata-systems/helm-charts/stroom-operator-crds \
  --namespace stroom-operator-system --version "$version"

helm upgrade stroom-operator \
  oci://ghcr.io/gradata-systems/helm-charts/stroom-operator \
  --namespace stroom-operator-system --version "$version" --reuse-values

kubectl -n stroom-operator-system rollout status deployment/stroom-operator-controller-manager
```

The CRDs are ordinary templates in the `stroom-operator-crds` chart, so `helm upgrade` updates them.

### From the all-in-one manifest

From 1.2.0 the operator is only published as Helm charts, not as the all-in-one manifest (`deploy/all-in-one.yaml`).
An operator installed from the manifest moves to Helm without stopping Stroom: label and annotate its resources as
belonging to the two Helm releases, so that Helm adopts them rather than refusing to install over them, then install
the charts. The charts' resources have the same names as the manifest's.

!!! danger "Don't delete the manifest's resources"
    Deleting the manifest (`kubectl delete -f all-in-one.yaml`) deletes the CRDs, and with them every
    `StroomCluster`, `DatabaseServer`, `DatabaseBackup` and `StroomTaskAutoscaler`.

```shell
ns=stroom-operator-system

# The CRDs belong to the stroom-operator-crds release
for crd in stroomclusters databaseservers databasebackups stroomtaskautoscalers; do
  kubectl annotate --overwrite "crd/$crd.stroom.gchq.github.io" \
    meta.helm.sh/release-name=stroom-operator-crds meta.helm.sh/release-namespace="$ns"
  kubectl label --overwrite "crd/$crd.stroom.gchq.github.io" app.kubernetes.io/managed-by=Helm
done

# The controller and its RBAC belong to the stroom-operator release
for resource in $(kubectl get clusterroles,clusterrolebindings -o name | grep '/stroom-operator-'); do
  kubectl annotate --overwrite "$resource" meta.helm.sh/release-name=stroom-operator meta.helm.sh/release-namespace="$ns"
  kubectl label --overwrite "$resource" app.kubernetes.io/managed-by=Helm
done
for resource in serviceaccount/stroom-operator-controller-manager role/stroom-operator-leader-election-role \
    rolebinding/stroom-operator-leader-election-rolebinding deployment/stroom-operator-controller-manager; do
  kubectl -n "$ns" annotate --overwrite "$resource" meta.helm.sh/release-name=stroom-operator meta.helm.sh/release-namespace="$ns"
  kubectl -n "$ns" label --overwrite "$resource" app.kubernetes.io/managed-by=Helm
done

helm install stroom-operator-crds oci://ghcr.io/gradata-systems/helm-charts/stroom-operator-crds \
  --namespace "$ns" --version <chart version>
helm install stroom-operator oci://ghcr.io/gradata-systems/helm-charts/stroom-operator \
  --namespace "$ns" --version <chart version>
kubectl -n "$ns" rollout status deployment/stroom-operator-controller-manager
```

The new operator then reconciles each cluster, as for any upgrade (below). From then on, upgrade with Helm. The
e2e tests run these commands against an operator installed from the 1.1.5 manifest, and check that the CRDs and
`StroomCluster`s are kept.

!!! note "The 1.1.5 manifest installs operator 1.1.3"
    The manifest published with 1.1.5 wasn't regenerated, so it installs the 1.1.3 operator and CRDs. Check which
    you're running with `kubectl -n stroom-operator-system get deployment stroom-operator-controller-manager
    -o jsonpath='{.spec.template.spec.containers[0].image}'`. The upgrade notes for 1.2.0 apply either way.

    Operator 1.1.3 can't update a `StroomCluster` that sets `https.enabled: false` or `logSender.mtlsEnabled: false`.
    It drops `false` values from its updates, which its CRD then rejects. Its log repeats
    `spec.https.enabled: Required value`, and the cluster's nodes are never deployed or changed. Moving to Helm fixes
    this.

### What happens to running Stroom clusters

Replacing the operator Pod doesn't stop Stroom. However, once the new operator starts, it reconciles every
`StroomCluster`, and if the new version generates different Pods (new environment variables, volume mounts or
probe settings, for example), **each node set is rolled**, one node at a time, with each node drained first. Read
the release notes and plan operator upgrades for a quiet period.

To check which operator version is running:

```shell
kubectl -n stroom-operator-system get deployment stroom-operator-controller-manager \
  -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
```

If the new operator Pod fails to start, the old one keeps running until it does.

## Upgrading Stroom

1. Read the Stroom release notes for the target version, particularly for database migrations and configuration
   changes. If you use your own configuration, update it first.
2. [Back up](../database/backups.md) the database.
3. If the upgrade includes a long database migration, raise the
   [startup probe's `failureThreshold`](../configuration/health-probes.md#allowing-for-long-upgrades) in the same
   change.
4. Change `spec.image.tag` and apply the manifest:

    ```shell
    kubectl apply -f stroom-cluster.yaml
    kubectl -n stroom rollout status statefulset/stroom-dev-node-data
    kubectl -n stroom rollout status statefulset/stroom-dev-node-ui
    ```

The operator updates each node set's StatefulSet, and Kubernetes replaces the nodes one at a time, starting from
the highest ordinal. Each node is drained before it stops, and the next node isn't replaced until the new one is
`Ready`. The first node on the new version migrates the database.

!!! warning "Mixed versions during a rolling upgrade"
    While the rollout is in progress, nodes on the old and new versions run side by side against the migrated
    database. That's normally fine for patch releases. For upgrades between minor versions, check the Stroom
    release notes; if they advise against mixed versions, stop the cluster first (see
    [Stopping and removal](stopping-and-removal.md#stopping-a-stroom-cluster)), change the image, and start it
    again.

## Upgrade notes

### Probe changes

Operator 1.2.0 changes the Stroom node probes. See [Health probes](../configuration/health-probes.md)
for the full description. Applying the upgrade rolls every node set, because the probes are part of the Pod
template.

**Liveness and readiness filters.** The liveness probe now runs only the `deadlocks` health check, and the
readiness probe runs every check except `stroom.security.common.impl.ExternalIdpConfigurationProvider`. Previously
both ran every check, so an outage of the IdP or database could fail liveness on every node and restart the whole
cluster. Existing `StroomCluster`s pick up the new defaults automatically, because the `allow` and `deny` fields are
new. If you relied on the old behaviour, set `livenessProbeTimings.allow: []`.

**`periodSeconds`.** Previously the readiness and liveness probes took their period from `initialDelaySeconds`,
and `periodSeconds` had no effect. It's now honoured. With the default values (both 5) nothing changes; if you set
either, check that the resulting timings are what you want.

**Startup probe default.** The default startup `failureThreshold` rises from 10 to 24 (about two minutes instead
of 50 seconds), and the new `lockWaitAllowanceSeconds` (default 30) extends it for `Parallel` node sets.

!!! warning "Existing clusters keep `failureThreshold: 10`"
    The Kubernetes API server stores defaulted values, so a `StroomCluster` created with an earlier version has
    `startupProbeTimings.failureThreshold: 10` saved in it, even if your manifest never mentioned it. It keeps that
    value after the upgrade. Check:

    ```shell
    kubectl -n stroom get stroomcluster dev \
      -o jsonpath='{range .spec.nodeSets[*]}{.name}: {.startupProbeTimings.failureThreshold}{"\n"}{end}'
    ```

    To adopt the new default, set it explicitly in your manifest (`failureThreshold: 24`) and apply, or patch
    each node set by index:

    ```shell
    kubectl -n stroom patch stroomcluster dev --type=json \
      -p '[{"op": "replace", "path": "/spec/nodeSets/0/startupProbeTimings/failureThreshold", "value": 24}]'
    ```

**Scaling `Parallel` node sets.** Under `podManagementPolicy: Parallel`, changing a node set's `count` now changes
its startup probe, so scaling also rolls the node set's existing nodes.

**Draining.** The `preStop` drain no longer treats a failed task list request as "no tasks". It keeps retrying
until the node has no active tasks or the termination grace period runs out. See
[Node lifecycle](node-lifecycle.md#stopping).

### Operator image registry

The operator image moves from Docker Hub (`gradata/stroom-k8s-operator`) to GitHub Container Registry
(`ghcr.io/gradata-systems/stroom-k8s-operator`). Releases up to 1.1.5 remain on Docker Hub. The Helm chart has a
new `image.repository` value, and the `registry` value now replaces the registry in it, so a mirror is expected at
`<registry>/gradata-systems/stroom-k8s-operator` rather than `<registry>/gradata/stroom-k8s-operator`. If you mirror
images, see [Air-gapped environments](../installation/air-gapped.md).

### Other changes in 1.2.0

See the [changelog](https://github.com/gradata-systems/stroom-k8s-operator/blob/master/CHANGELOG.md) for every
change. Those that affect an upgrade:

- **Database servers restart once.** The operator now applies changes to a `DatabaseServer`, so its Pod is
  updated to match after the upgrade. MySQL restarts first, and each node set is rolled once it's ready, so the
  nodes can drain. Stroom loses its database connection while MySQL restarts. See
  [Changing a DatabaseServer](../database/database-server.md#changing-a-databaseserver).
- **Security defaults.** Pods now disallow privilege escalation, use the container runtime's default seccomp
  profile and don't mount a service account token, unless set otherwise. This is part of why every node set rolls.
  See [Security](../configuration/security.md).
- **Stricter validation.** Database names (`databaseNames`, `appDatabaseName`, `statsDatabaseName`) must be
  letters, digits and underscores. `DatabaseServer.spec.additionalConfig` lines must each be a single option, and
  can't set options the operator sets or that give file access or bypass authentication. Node `role` and
  `volumeClaimDeletePolicy` must be one of their documented values. A resource with an invalid value keeps
  running, but can't be changed until the value is fixed.
- **Ingress TLS.** Each Ingress's TLS section now follows `ingress.secretName`, rather than `spec.https`. See
  [TLS at the Ingress](../configuration/ingress-tls.md#tls-at-the-ingress).
- **Operator permissions.** The operator's ClusterRole now only allows getting and creating Secrets.
- **Log sender.** A `StroomCluster` without a `logSender` is now deployed. Earlier versions never deployed one, so
  if you added a disabled log sender to work around that, it can be removed.

# Install with the all-in-one manifest

[`deploy/all-in-one.yaml`](https://github.com/gradata-systems/stroom-k8s-operator/blob/master/deploy/all-in-one.yaml)
contains everything the operator needs in one file: the `stroom-operator-system` namespace, the CRDs, RBAC and the
controller Deployment.

## Install

Download the manifest, check it, then apply it with server-side apply:

```shell
curl -sSfLO https://raw.githubusercontent.com/gradata-systems/stroom-k8s-operator/master/deploy/all-in-one.yaml
grep 'image:' all-in-one.yaml | grep stroom-k8s-operator
kubectl apply --server-side -f all-in-one.yaml
```

Check that the controller `image:` tag is the operator version you intend to run. The manifest on the `master`
branch is not always in step with the latest Helm chart.

!!! tip "Why `--server-side`?"
    The `StroomCluster` CRD embeds large parts of the Kubernetes Pod schema. Server-side apply avoids storing a
    copy of each large object in a `last-applied-configuration` annotation. Plain `kubectl apply` also works.

Then check that the controller is running:

```shell
kubectl -n stroom-operator-system rollout status deployment/stroom-operator-controller-manager
```

## Use a private registry

Edit every `image:` in the manifest so that it points at your registry before you apply it. For example,
`gradata/stroom-k8s-operator:1.1.5` becomes `registry.example.com/gradata/stroom-k8s-operator:1.1.5`.

## Change the controller's resources

Edit the `resources` of the `manager` container in the `stroom-operator-controller-manager` Deployment, then apply
the manifest again.

## Upgrade

Apply the manifest for the new version in the same way. Kubernetes replaces the controller Pod. Don't delete the
old manifest first: that would delete the CRDs and every Stroom resource with them. See also
[Upgrading](../operations/upgrading.md).

## Remove

!!! danger "Deleting the manifest deletes your Stroom clusters"
    `kubectl delete -f all-in-one.yaml` removes the CRDs, and Kubernetes then deletes every `StroomCluster`,
    `DatabaseServer`, `DatabaseBackup` and `StroomTaskAutoscaler` in the cluster. Follow
    [Stopping and removal](../operations/stopping-and-removal.md) instead.

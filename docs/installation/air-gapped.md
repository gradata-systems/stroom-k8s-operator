# Air-gapped environments

To install without access to public registries, download the charts and images on a connected machine, move them
into the isolated environment and push the images to a private registry.

## 1. Download the Helm charts

```shell
version=<chart version>   # see: helm show chart oci://ghcr.io/gradata-systems/helm-charts/stroom-operator
helm pull oci://ghcr.io/gradata-systems/helm-charts/stroom-operator-crds --version "$version"
helm pull oci://ghcr.io/gradata-systems/helm-charts/stroom-operator --version "$version"
```

This produces `stroom-operator-crds-<version>.tgz` and `stroom-operator-<version>.tgz`.

## 2. Download the container images

You need the operator image and the images your custom resources reference. To see the operator image a chart
deploys:

```shell
helm template ./stroom-operator-<version>.tgz | grep 'image:'
```

```shell
images="
ghcr.io/gradata-systems/stroom-k8s-operator:<operator version>
gchq/stroom:v7.13.2
mysql:<MySQL 8 version>
gchq/stroom-log-sender:v2.2.1
"
for image in $images; do docker pull "$image"; done
docker image save --output stroom-images.tar $images
```

Leave out the MySQL image if you use an external database, and the log sender if you don't ship logs.

## 3. Transfer and push

Copy the chart archives and `stroom-images.tar` into the isolated environment, then load the images and push them
to your registry, keeping the same repository paths without their original registry (so
`ghcr.io/gradata-systems/stroom-k8s-operator` becomes `registry.example.com/gradata-systems/stroom-k8s-operator`):

```shell
registry=registry.example.com
docker image load --input stroom-images.tar
for image in $images; do
  # Remove the image's registry, if it has one, such as ghcr.io
  path=$image
  first=${image%%/*}
  if [[ $image == */* && ( $first == *.* || $first == *:* ) ]]; then path=${image#*/}; fi
  docker tag "$image" "$registry/$path"
  docker push "$registry/$path"
done
```

## 4. Install the operator

```shell
helm install stroom-operator-crds ./stroom-operator-crds-<version>.tgz \
  --namespace stroom-operator-system --create-namespace

helm install stroom-operator ./stroom-operator-<version>.tgz \
  --namespace stroom-operator-system \
  --set registry=registry.example.com
```

The `registry` value replaces the registry in the operator image, giving
`registry.example.com/gradata-systems/stroom-k8s-operator:<tag>`.

## 5. Point your resources at the registry

The operator doesn't rewrite the images in your custom resources, so set the full image repository in each one:

```yaml
spec:
  image:
    repository: registry.example.com/gchq/stroom
    tag: v7.13.2
```

Do the same for `DatabaseServer.spec.image`, `DatabaseBackup.spec.image` and `StroomCluster.spec.logSender.image`.

## Registry credentials

The charts and custom resources have no `imagePullSecrets` setting. If your registry requires credentials, either
configure them on the Kubernetes nodes, or add an image pull secret to the ServiceAccounts the Pods use:

| Pods | ServiceAccount |
| --- | --- |
| Operator | `stroom-operator-controller-manager` in `stroom-operator-system` |
| Stroom nodes | `stroom-<cluster name>`, created by the operator in the cluster's namespace |
| `DatabaseServer` and `DatabaseBackup` | `default` in their namespace |

For example:

```shell
kubectl -n stroom patch serviceaccount stroom-dev \
  -p '{"imagePullSecrets": [{"name": "registry-credentials"}]}'
```

The operator creates the `stroom-<cluster name>` ServiceAccount when it first reconciles the `StroomCluster` and
doesn't change it afterwards, so the patch persists. A ServiceAccount's pull secrets are added to Pods only when
they are created, so delete any Pods already in `ImagePullBackOff`; the StatefulSet recreates them.

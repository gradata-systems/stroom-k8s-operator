The Stroom K8s Operator is a Kubernetes operator written in Go and developed using the [Operator SDK](https://sdk.operatorframework.io).

Its purpose is to simplify the deployment and operational management of a Stroom cluster in a Kubernetes environment.

This project is not related to [stroom-kubernetes](https://github.com/p-kimberley/stroom-kubernetes), which is a Helm chart for deploying a Stroom stack, including optional components like Kafka.
The purpose of this Operator is to focus on the Stroom deployment and automation. 

# Features
## Deployment
1. Custom Resource Definitions (CRDs) for defining the desired state of a Stroom cluster, nodes and database
2. Ability to designate dedicated `Processing` and `Frontend` nodes and route event traffic appropriately
3. Automatic secrets management (e.g. secure database credential generation and storage)
4. Simple deployment via Helm charts
   
## Operations
1. Scheduled database backups
2. Stroom node audit log shipping
3. Stroom node lifecycle management
    1. Prevent node shutdown while Stroom processing tasks are still active
    2. Automatic task draining during shutdown
    3. Rolling Stroom version upgrades
4. Automatically scale the maximum tasks for each Stroom node by continually assessing average CPU usage.
   The following parameters are configurable:
    1. Adjustment time interval (how often adjustments should be made)
    2. Metric sliding window (calculate the average based on the specified number of minutes)
    3. Minimum CPU % to keep the node above
    4. Maximum CPU % to keep the node below
    5. Minimum number of tasks allowed for the node
    6. Maximum number of tasks allowed for the node

# Building
If you are just looking to install the Operator and don't wish to make any changes, you can skip this section.

This project was built with the Operator SDK, which bundles Kubernetes resource manifests (such as CRDs) and custom code into a deployable format.
1. Install [Operator SDK](https://sdk.operatorframework.io/docs/installation/) and [additional prerequisites](https://sdk.operatorframework.io/docs/building-operators/golang/installation/)
2. Clone this repository
3. `make build-offline-bundle` (optional)`PRIVATE_REGISTRY=my-registry.example.com`

# Installation

## Prerequisites
1. Kubernetes cluster running version >= v1.20
2. Helm >= v3.8.0
2. [metrics-server](https://github.com/kubernetes-sigs/metrics-server) (pre-installed with some K8s distributions)

## (Optional) Air-Gap Preparation
1. Pull Helm charts for offline use. The following commands each produce a `.tar.gz` file containing the latest version of the latest Stroom operator Helm charts:
   ```shell
   helm pull oci://ghcr.io/gradata-systems/helm-charts/stroom-operator-crds
   helm pull oci://ghcr.io/gradata-systems/helm-charts/stroom-operator
   ```
2. Pull all images in the [offline image list](https://raw.githubusercontent.com/gradata-systems/stroom-k8s-operator/master/deploy/images.txt) locally. The following script block saves all images in `images.txt` to a file `images.tar.gz` in the current directory:
   ```shell
   images=$(curl -s 'https://raw.githubusercontent.com/gradata-systems/stroom-k8s-operator/master/deploy/images.txt') && \
   printf %s "$images" | \
   while IFS= read -r line; do \
   docker pull $line; \
   docker image save --output=images.tar.gz $(echo $images); \
   done
   ```
3. Transport all downloaded archives to the airgapped environment.
4. Push all container images to a private registry.

## Install the Stroom K8s operator
The operator requires two Helm charts to be installed, in order to function. The chart `stroom-operator-crds` deploys Custom Resource Definitions (CRDs), which define the structure of the custom Stroom cluster resources. The `stroom-operator` chart deploys the actual operator.

### 1. stroom-operator-crds
```shell
helm install -n stroom-operator-system --create-namespace stroom-operator-crds \
oci://ghcr.io/gradata-systems/helm-charts/stroom-operator-crds
```

### 2. stroom-operator
```shell
helm install -n stroom-operator-system stroom-operator \
oci://ghcr.io/gradata-systems/helm-charts/stroom-operator
```
The operator will be deployed to namespace `stroom-operator-system`. You can monitor its progress by watching the `Pod` named `stroom-operator-controller-manager`. Once it reaches `Ready` state, you can deploy a Stroom cluster.

#### Air-Gap Deployment
```shell
helm install -n stroom-operator-system stroom-operator \
oci://ghcr.io/gradata-systems/helm-charts/stroom-operator \
--set registry=<private registry URL>
```

## Explore sample configuration
An example Stroom cluster configuration is at [./samples](./samples), which has the following features:
1. Dedicated UI node for handling user web front-end traffic. The Stroom K8s Operator disables data processing for such nodes.
2. Three dedicated data processing nodes. Only these nodes receive and process event traffic.
3. Persistent storage for all nodes.
4. Automatic task scaling for processing nodes, which aims to achieve optimal CPU utilisation during periods of high load.

## Deploy a Stroom cluster using the sample configuration
1. Create a `PersistentVolume` for each Stroom node 
2. Create `DatabaseServer` resource (example: [database-server.yaml](./samples/database-server.yaml))
3. Create `StroomCluster` resource (example: [stroom-cluster.yaml](./samples/stroom-cluster.yaml))
4. (Optional) Create `StroomTaskAutoscaler` resource (example: [autoscaler.yaml](./samples/autoscaler.yaml))
5. Deploy each resource
   ```shell
   kubectl apply -f database-server.yaml
   kubectl apply -f stroom-cluster.yaml
   kubectl apply -f autoscaler.yaml
   ```

# Upgrading the Operator
```shell
helm upgrade -n stroom-operator-system stroom-operator \
oci://ghcr.io/gradata-systems/helm-charts/stroom-operator
helm upgrade -n stroom-operator-system stroom-operator-crds \
oci://ghcr.io/gradata-systems/helm-charts/stroom-operator-crds
```
This upgrades the controller in-place, without affecting any deployed Stroom clusters.

# Upgrading a Stroom cluster
To upgrade a Stroom cluster to use a newer, tagged container image:
1. Edit the `StroomCluster` resource manifest (e.g. `stroom-cluster.yaml`), replacing the property `spec.image.tag` with the new value.
2. `kubectl apply -f stroom-cluster.yaml`
3. Watch the status of the `StroomCluster` pods, as the Stroom K8s Operator executes a rolling upgrade of each of them.
The Operator will drain each Stroom node of any processing tasks, before restarting it.

# Removing the Stroom K8s Operator
## stroom-operator
The operator can be safely removed without impacting any operational Stroom clusters. Bear in mind however, that features such as task autoscaling, will not work without the operator running.
```shell
helm uninstall -n stroom-operator-system stroom-operator
```
## stroom-operator-crds
This should only be performed once all Stroom clusters (`StroomCluster` resources) are deleted. This ensures that any Stroom processing tasks have had a chance to complete.

**WARNING:** Removing CRDs will in turn delete ALL Stroom clusters! If Stroom cluster persistent storage was configured correctly, deleting the CRDs will not result in data loss, as the `PersistentVolumeClaims` will remain bound.

```shell
helm uninstall -n stroom-operator-system stroom-operator-crds
```

# Deleting a Stroom cluster
```shell
kubectl delete -f stroom-cluster.yaml
kubectl delete -f database-server.yaml
```
The order of deletion does not matter, as the `DatabaseServer` resource deletion will only be finalised when the parent `StroomCluster` is removed.
If `kubectl` waits for a period of time after issuing the above commands, this is normal, as the `StroomCluster` may be draining tasks.

After deleting a cluster, depending on the `StroomCluster` property `spec.volumeClaimDeletePolicy`, one of the following will happen:
1. (Not defined) - This is the safest option and the `PersistentVolumeClaim` created for each Stroom node remains. This means the `StroomCluster` may be re-deployed and each `Pod` will assume the same PVC it was allocated previously.
2. `DeleteOnScaledownOnly` - PVCs are deleted only when the number of nodes in a `NodeSet` is reduced.
3. `DeleteOnScaledownAndClusterDeletion` - PVCs are deleted if the `StroomCluster` is deleted. Be careful with this setting, as it requires intervention afterward to unbind `PersistentVolume`s that were previously claimed.

A Stroom cluster may be re-deployed by re-applying the `StroomCluster` resource.

# Restarting a hung or failed Stroom node
If a Stroom node becomes non-responsive, it may be necessary to restart its `Pod`. The example below deletes the first (as identified by the index #0) Stroom data node in `StroomCluster` named `dev`:
```shell
kubectl delete pod -n <namespace> stroom-dev-node-data-0
```
As with deleting a `StroomCluster` resource, the Stroom K8s Operator will ensure the `Pod` is drained of all currently processing tasks, before allowing it to be shut down.

# Logging
You can follow the `stroom-operator-controller-manager` Pod log to observe controller output and in particular, what actions it is performing with regard to Stroom cluster state.

# General tips
1. Use a version control system like Git, to manage cluster configurations.
2. Backup the database secrets generated by the Stroom K8s Operator. These are stored in a `Secret` resource in the same namespace as the `StroomCluster`, named in the convention: `stroom-<cluster name>-db`.
   The credentials for users `root` and `stroomuser` are contained within and deletion of this `Secret` will cause the Stroom cluster to stop functioning!
3. Ensure `StroomCluster` property `spec.nodeTerminationGracePeriodSecs` is set to a sufficiently large value.
   If your Stroom nodes typically have long-running tasks, ensure the value of this property is larger than the longest task.
   This will give Stroom nodes enough time to finish processing tasks before fulfilling a shutdown request. If the time interval is too short, any tasks still processing will fail.
   Conversely, setting this interval to too long a value, will cause non-responsive Stroom nodes to linger for extended periods of time, before being killed.
4. Experiment with different `StroomTaskAutoscaler` parameters. A tighter CPU percentage min/max range is probably preferable, as this will make the Operator work harder to keep CPU usage in range.
Bear in mind that the CPU percentages are based on a rolling average, so be careful to set a realistic upper task limit, to ensure momentary heavy load doesn't overwhelm the node.
5. In particularly large deployments (i.e. involving many Stroom nodes), it may be necessary to increase the resources allocated to `stroom-operator-controller-manager` `Pod`. This can be done by editing the `all-in-one.yaml` prior to deployment.
The need for more resources is due to the Operator maintaining a finite collection of `StroomCluster` `Pod` metrics in-memory.
6. `DatabaseServer` backups are performed as a single transaction. As this can cause issues with concurrent schema changes, Stroom upgrades (which sometimes modify the DB schema) should not be performed while a database backup is in progress.
7. If a Stroom `Pod` hangs and you do not want to wait for it to be deleted (and are comfortable accepting the risk of the loss of processing tasks), you can force its deletion by:
   1. Deleting the `Pod` (e.g. using `kubectl`)
   2. Terminating the Stroom Java process within the running container (named `stroom-node`)
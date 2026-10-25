The Stroom K8s Operator is a Kubernetes operator written in Go and developed using the [Operator SDK](https://sdk.operatorframework.io).

Its purpose is to simplify the deployment and operational management of a Stroom cluster in a Kubernetes environment.

**Documentation: <https://gradata-systems.github.io/stroom-k8s-operator/>**

The documentation covers installation (Helm, all-in-one manifest and air-gapped), configuring a `StroomCluster`, databases and backups, task autoscaling, day-2 operations, troubleshooting and a full CRD reference.

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
4. Automatically scale the maximum tasks for each Stroom node by continually assessing average CPU usage

# Quick install
The operator requires Stroom to use an external OpenID Connect identity provider. See the [prerequisites](https://gradata-systems.github.io/stroom-k8s-operator/getting-started/prerequisites/) before you start.

```shell
helm install -n stroom-operator-system --create-namespace stroom-operator-crds \
oci://ghcr.io/gradata-systems/helm-charts/stroom-operator-crds

helm install -n stroom-operator-system stroom-operator \
oci://ghcr.io/gradata-systems/helm-charts/stroom-operator
```

Then follow the [quick start](https://gradata-systems.github.io/stroom-k8s-operator/getting-started/quick-start/) to deploy a Stroom cluster. Example resources are in [./samples](./samples).

# Building
If you are just looking to install the Operator and don't wish to make any changes, you can skip this section.

This project was built with the Operator SDK, which bundles Kubernetes resource manifests (such as CRDs) and custom code into a deployable format.
1. Install [Operator SDK](https://sdk.operatorframework.io/docs/installation/) and [additional prerequisites](https://sdk.operatorframework.io/docs/building-operators/golang/installation/)
2. Clone this repository
3. `make build-offline-bundle` (optional)`PRIVATE_REGISTRY=my-registry.example.com`

# Documentation
The documentation source is in [./docs](./docs) and is published with MkDocs Material. If you change the API types in `api/v1`, regenerate the CRD reference with `hack/gen-crd-docs.sh`. See [Contributing to the docs](https://gradata-systems.github.io/stroom-k8s-operator/contributing/).

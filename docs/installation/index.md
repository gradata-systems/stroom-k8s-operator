# Installation

The operator consists of the CRDs and a controller Deployment (`stroom-operator-controller-manager`) in the
namespace `stroom-operator-system`. It's installed with two Helm charts, one for the CRDs and one for the
controller, so the CRDs are versioned separately and are never removed with the controller:

- [Install with Helm](helm.md) from the GitHub Container Registry.
- [Air-gapped environments](air-gapped.md): install from downloaded charts and a private registry, where the
  cluster can't reach public registries.

Install only one copy of the operator per Kubernetes cluster. It watches all namespaces.

Releases up to 1.1.5 were also published as an all-in-one manifest. To move an operator installed that way to the
Helm charts, see [Upgrading](../operations/upgrading.md#from-the-all-in-one-manifest).

Once the controller Pod is `Ready`, continue with the [quick start](../getting-started/quick-start.md) or
[Configuring a StroomCluster](../configuration/index.md).

# Installation

The operator consists of the CRDs and a controller Deployment (`stroom-operator-controller-manager`) in the
namespace `stroom-operator-system`. Install it in one of these ways:

| Method | Use when |
| --- | --- |
| [Helm (OCI)](helm.md) | Recommended. Upgrades and removal are straightforward, and the CRDs are versioned separately from the controller. |
| [All-in-one manifest](manifest.md) | You can't use Helm, or you want to inspect or patch a single YAML file. |
| [Air-gapped environments](air-gapped.md) | The cluster can't reach public registries. Works with either method. |

Install only one copy of the operator per Kubernetes cluster. It watches all namespaces.

Once the controller Pod is `Ready`, continue with the [quick start](../getting-started/quick-start.md) or
[Configuring a StroomCluster](../configuration/index.md).

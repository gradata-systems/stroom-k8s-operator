# Samples

A Stroom cluster with an external identity provider and an operator-managed MySQL server, in the `stroom`
namespace. They're based on the configuration the e2e tests run (`test/e2e/testdata`), and are checked against the
CRDs by the unit tests.

| File | Contents |
|---|---|
| `secrets.yaml` | OpenID client secrets and the key store password. Replace the values. |
| `stroom-config.yaml` | Stroom's configuration, with the identity provider's address to fill in |
| `database-server.yaml` | A MySQL server managed by the operator |
| `external-database.yaml` | How to use a MySQL server the operator doesn't manage instead |
| `stroom-cluster.yaml` | The Stroom cluster, with processing and frontend node sets |
| `database-backup.yaml` | Daily database backups |
| `autoscaler.yaml` | Task autoscaling for the processing nodes |
| `network-policies.yaml` | NetworkPolicies limiting connections to the nodes and database |

Apply them in that order, after creating the namespace and the `stroom-tls` Secret:

```sh
kubectl create namespace stroom
kubectl apply -f samples/secrets.yaml -f samples/stroom-config.yaml -f samples/database-server.yaml
kubectl apply -f samples/stroom-cluster.yaml -f samples/database-backup.yaml -f samples/autoscaler.yaml
```

See the [documentation](https://gradata-systems.github.io/stroom-k8s-operator/) for the identity provider setup and
each setting.

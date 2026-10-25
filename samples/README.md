# Samples

Example deployments of a Stroom cluster in the `stroom` namespace. They're based on the configuration the e2e tests
run (`test/e2e/testdata`), and the unit tests build them and check them against the CRDs.

| Directory | What it shows |
|---|---|
| [`operator-database`](operator-database) | A MySQL server deployed and managed by the operator (`DatabaseServer`), with daily backups, processing and frontend node sets, task autoscaling, log shipping and NetworkPolicies |
| [`external-database`](external-database) | A MySQL server the operator doesn't manage, such as a cloud database service, with a single node set |
| [`external-idp`](external-idp) | A custom Stroom configuration that authenticates with Keycloak, and the OpenID client Secrets. Both deployments include it as a kustomize component. |

Stroom must use an external identity provider with the operator, as the node lifecycle scripts can't get a token
from Stroom's internal one. So both deployments include `external-idp`. To use a different identity provider,
change its `config.yml`.

## Before you start

You need:

- The operator, installed as in the [installation guide](https://gradata-systems.github.io/stroom-k8s-operator/installation/).
- The `stroom` namespace: `kubectl create namespace stroom`.
- `stroom-tls`, a `kubernetes.io/tls` Secret in that namespace with `tls.crt`, `tls.key` and `ca.crt`, for the
  nodes' HTTPS connector and the Ingress. cert-manager can issue one.
- An Ingress controller. The samples use the `nginx` class.
- A Keycloak realm, set up as below.
- For `external-database`, a MySQL 8 server with the databases and user described in its `database-secret.yaml`.

## Keycloak

Keycloak isn't included. The configuration expects a realm named `stroom` at `https://keycloak.example.com`, with two
confidential clients:

| Client | Used by | Settings |
|---|---|---|
| `stroom` | Stroom, for user sign-in | Client authentication on; standard flow on; valid redirect URI `https://stroom.example.com/*` |
| `stroom-operator` | The operator's node lifecycle scripts, to enable and drain nodes | Client authentication on; service accounts on; standard flow off |

Access tokens from both clients must have `stroom` in their audience (`aud`) claim, which Stroom checks. In Keycloak,
add an _Audience_ mapper to each client (or to a client scope they share), with _Included Client Audience_ set to
`stroom` and _Add to access token_ on.

The `stroom-operator` client's service account needs a Stroom user with the **Manage Nodes**, **Manage Jobs** and
**Manage Tasks** permissions, or the nodes can't start. The user is identified by the service account's user ID in
Keycloak (_Service account roles_ tab, then the service account user's _ID_). Create it before the first node
starts, with Stroom's `manage_users` command, run against the Stroom database, for example from a one-off Pod with
the Stroom image and the cluster's database settings:

```sh
./start.sh manage_users \
  --createUser <service account user ID> \
  --grantPermission <service account user ID> "Manage Nodes" \
  --grantPermission <service account user ID> "Manage Jobs" \
  --grantPermission <service account user ID> "Manage Tasks"
```

See the [identity provider documentation](https://gradata-systems.github.io/stroom-k8s-operator/configuration/identity-provider/)
for the details.

## Deploying

1. Edit the samples for your environment:
    - `external-idp/config.yml`: the `openIdConfigurationEndpoint`, for your Keycloak host and realm.
    - `external-idp/openid-secrets.yaml`: each Keycloak client's secret.
    - `*/secrets.yaml`: the key store password.
    - `*/stroom-cluster.yaml`: the Ingress host name (`stroom.example.com`), node counts and resources.
    - `external-database/stroom-cluster.yaml` and `database-secret.yaml`: the MySQL server's address and the user's
      password.
2. Apply one deployment:

    ```sh
    kubectl apply -k samples/operator-database
    ```

    or

    ```sh
    kubectl apply -k samples/external-database
    ```

3. Wait for the nodes to become ready:

    ```sh
    kubectl -n stroom get pods -w
    ```

To see what a deployment creates without applying it, run `kubectl kustomize samples/operator-database`.

In a real deployment, create the Secrets another way, for example with a secret manager, and leave them out of the
kustomizations, rather than keeping their values in files.

See the [documentation](https://gradata-systems.github.io/stroom-k8s-operator/) for each setting.

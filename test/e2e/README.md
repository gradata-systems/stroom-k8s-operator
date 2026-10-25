# End-to-end tests

These tests deploy the operator to a [kind](https://kind.sigs.k8s.io/) cluster and use it to run MySQL and Stroom, with
either Keycloak or Stroom's internal identity provider.

The suites are in two groups, selected by Ginkgo label:

- **core**: run on merge to master.
- **extended**: run nightly.

The operator is installed with its Helm charts, except where a suite installs the previous release another way.

## Running

Requirements: Docker, kind, kubectl, Helm and Go. The suites run one after another, and need roughly 6 GB of memory
for Docker with two Stroom nodes, or 4 GB with one. The operator upgrade suite needs the previous release's Git tag,
so clone with tags.

```sh
make test-e2e                                  # create a kind cluster, run the core suites, delete the cluster
make test-e2e-extended                         # the same, for the extended suites
E2E_KEEP_CLUSTER=1 make test-e2e               # keep the cluster afterwards, to inspect or rerun
E2E_STROOM_NODES=1 make test-e2e               # run Stroom clusters with one node, to reduce memory use
go test ./test/e2e/ -v -ginkgo.v -ginkgo.focus "external database"   # rerun one suite on an existing cluster
```

| Variable | Default | Purpose |
|---|---|---|
| `KIND_CLUSTER` | `stroom-operator-e2e` | kind cluster to use |
| `E2E_LABELS` | `core` | Ginkgo label filter selecting the suites, for `make test-e2e` |
| `E2E_FOCUS` | | Regular expression: only run the specs whose names match it, for `make test-e2e` |
| `E2E_DIAGNOSTICS_DIR` | | Directory to save each failed suite's namespace to (resources, events, pod descriptions and logs) before it's deleted |
| `E2E_OPERATOR_TAG` | `e2e` | Tag of the operator image built and tested, as `ghcr.io/gradata-systems/stroom-k8s-operator:<tag>` |
| `E2E_SKIP_BUILD` | | Set to `1` to test an existing operator image |
| `E2E_PREVIOUS_RELEASE` | `1.1.5` | Operator release the upgrade suite upgrades from |
| `E2E_STROOM_TAG` | `v7.13.2` | Stroom image tag |
| `E2E_PREVIOUS_STROOM_TAG` | `v7.13.1` | Stroom image tag the Stroom upgrade starts from |
| `E2E_STROOM_NODES` | `2` | Nodes in each Stroom cluster with an external identity provider |
| `E2E_KEYCLOAK_IMAGE` | `quay.io/keycloak/keycloak:26.0.7` | Keycloak image |
| `E2E_KEEP_NAMESPACES` | | Set to `1` to keep each suite's namespace afterwards |

## Suites

| Group | Suite | Namespace | What it covers |
|---|---|---|---|
| core | Operator upgrade | `stroom-e2e-upgrade` | Running Stroom with the previous operator release, then upgrading the operator with Helm |
| core | Operator installed from the all-in-one manifest, moved to Helm | `stroom-e2e-manifest` | Installing the previous release from its all-in-one manifest, then moving it to the Helm charts as the upgrade docs describe (`adopt-manifest-install.sh`), keeping the CRDs and a StroomCluster |
| core | DatabaseServer | `stroom-e2e-db` | MySQL with the `mysql/mysql-server` and official `mysql` images; root password handling; DatabaseBackup |
| core | External identity provider, operator-managed database | `stroom-e2e-external` | Keycloak, a Stroom user for the operator with only the Manage Nodes, Manage Jobs and Manage Tasks permissions, node start, health check filters, node drain, scaling the node set up and down, an identity provider outage, and deleting the cluster while a node has a processing task |
| core | External identity provider, external database | `stroom-e2e-external-db` | As above, except scaling and the outage, with a MySQL server not managed by the operator (`external-mysql.yaml`), connected to by address |
| core | Bootstrap lock and Stroom upgrade | `stroom-e2e-lock` | Two or more nodes waiting on the Stroom bootstrap lock, which the test holds, within the startup probe's lock wait allowance, then a rolling upgrade of Stroom |
| core | Internal identity provider | `stroom-e2e-internal` | Stroom's bundled configuration with its internal identity provider, without and with HTTPS, then changed to run as the stroom user |
| extended | Node roles, HTTPS, Ingress, log sender, autoscaling | `stroom-e2e-features` | Processing and frontend node sets and their managed jobs, the key store init container, ingress-nginx, the log sender sidecar, and StroomTaskAutoscaler with metrics-server |
| extended | Volume claim delete policies | `stroom-e2e-volumes` | Each `volumeClaimDeletePolicy`, on scale-down and cluster deletion |
| extended | Operator restarts | `stroom-e2e-restart` | Restarting the operator while idle, during a scale-up, and during cluster deletion |
| extended | Sample deployment with an operator-managed database | `stroom-e2e-samples` | `samples/operator-database`, applied with kustomize: Stroom starting within the default startup probe, the Ingress, a backup, and the NetworkPolicies refusing other connections |
| extended | Sample deployment with an external database | `stroom-e2e-samples-external` | `samples/external-database`, applied with kustomize, against a MySQL server not managed by the operator |

The volume claim and operator restart suites run the pause image in place of Stroom, as they only need the operator
to manage pods and volumes. Processing tasks are simulated by adding rows to Stroom's database, so draining can be
tested without a data processing workload.

Fixtures are in `testdata`. `${VAR}` references in them are filled in by the tests; a reference with no value is an
error. `external-idp-stroom-config.yaml` and `external-idp-stroom-config-https.yaml` are the operator's bundled Stroom
configuration with Keycloak as the identity provider, the first without the HTTPS connector. They are applied
with only the namespace substituted, as they contain Stroom's own `${VAR}` references.

The sample suites deploy `samples/` through the overlays in `testdata/samples`, which size them for a kind cluster.
The tests replace the samples' placeholders (`keycloak.example.com`, `mysql.example.com` and the `replace-with-...`
secrets) with the test Keycloak and MySQL server, and run Keycloak in the cluster's namespace, so the overlay for
`operator-database` adds NetworkPolicies allowing connections to it. kind enforces NetworkPolicies from v0.24.

## Known issues pinned by the tests

Some specs assert the current behaviour where it is a known bug or limitation, marked `BASELINE` in the code. A fix
changes the spec to assert the fixed behaviour, in the same change.

| Spec | Current behaviour | Reference |
|---|---|---|
| Internal identity provider | Stroom starts, but node startup can't obtain a token, so the node never becomes ready | Stroom 7.13 rejects client credentials token requests with its internal identity provider, with 400 Bad Request |
| Operator upgrade | The previous release never deploys a StroomCluster without a log sender, so the suite sets a disabled one | Fixed in 1.2.0 |

The tests found, and pinned, these bugs in 1.1.5, which 1.2.0 fixes: a StroomCluster without a log sender was never
deployed; the official `mysql` image's root password was the password file's path; backups never ran; the
DatabaseServer was ready before MySQL; liveness and readiness ran every health check, so an identity provider outage
restarted every node; cluster deletion disabled jobs on other clusters' nodes; scaling didn't extend the startup
probe; nodes waiting on the bootstrap lock were restarted; the bundled configuration needed HTTPS; Stroom couldn't
read the key store; the lifecycle scripts' logs weren't writable after changing to the stroom user; and the datafeed
Ingress sent plain HTTP to the nodes' HTTPS port.

# End-to-end tests

These tests deploy the operator to a [kind](https://kind.sigs.k8s.io/) cluster and use it to run MySQL and Stroom, with
either Keycloak or Stroom's internal identity provider.

The suites are in two groups, selected by Ginkgo label:

- **core**: run on merge to master. The operator is installed with its Helm charts.
- **extended**: run nightly. The operator is installed with the kustomize manifests that `deploy/all-in-one.yaml` is
  built from.

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
| `E2E_OPERATOR_TAG` | `e2e` | Tag of the operator image built and tested, as `gradata/stroom-k8s-operator:<tag>` |
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
| core | DatabaseServer | `stroom-e2e-db` | MySQL with the `mysql/mysql-server` and official `mysql` images; root password handling; DatabaseBackup |
| core | External identity provider, operator-managed database | `stroom-e2e-external` | Keycloak, a Stroom user for the operator with only the Manage Nodes, Manage Jobs and Manage Tasks permissions, node start, health check filters, node drain, scaling the node set up and down, an identity provider outage, and deleting the cluster while a node has a processing task |
| core | External identity provider, external database | `stroom-e2e-external-db` | As above, except scaling and the outage, with a MySQL server not managed by the operator (`external-mysql.yaml`), connected to by address |
| core | Bootstrap lock and Stroom upgrade | `stroom-e2e-lock` | Nodes waiting on the Stroom bootstrap lock, which the test holds, then a rolling upgrade of Stroom |
| core | Internal identity provider | `stroom-e2e-internal` | Stroom's bundled configuration with its internal identity provider, without and with HTTPS, and as the stroom user |
| extended | Node roles, HTTPS, Ingress, log sender, autoscaling | `stroom-e2e-features` | Processing and frontend node sets and their managed jobs, the key store init container, ingress-nginx, the log sender sidecar, and StroomTaskAutoscaler with metrics-server |
| extended | Volume claim delete policies | `stroom-e2e-volumes` | Each `volumeClaimDeletePolicy`, on scale-down and cluster deletion |
| extended | Operator restarts | `stroom-e2e-restart` | Restarting the operator while idle, during a scale-up, and during cluster deletion |

The volume claim and operator restart suites run the pause image in place of Stroom, as they only need the operator
to manage pods and volumes. Processing tasks are simulated by adding rows to Stroom's database, so draining can be
tested without a data processing workload.

Fixtures are in `testdata`. `${VAR}` references in them are filled in by the tests; a reference with no value is an
error. `external-idp-stroom-config.yaml` and `external-idp-stroom-config-https.yaml` are the operator's bundled Stroom
configuration with Keycloak as the identity provider, the first without the HTTPS connector. They are applied
with only the namespace substituted, as they contain Stroom's own `${VAR}` references.

## Known issues pinned by the tests

Some specs assert the current behaviour where it is a known bug or limitation, marked `BASELINE` in the code. A fix
changes the spec to assert the fixed behaviour, in the same change.

| Spec | Current behaviour | Reference |
|---|---|---|
| DatabaseServer: StroomCluster without a log sender | Never deployed, as the operator can't add its finalizer. Other StroomCluster fixtures set a disabled log sender to work around it. | Typed update sends empty log sender settings |
| DatabaseServer: root password with an image that doesn't read it from a file | Root password is `/etc/mysql/password/root` | Security review L1 |
| DatabaseServer: backup CronJob | Not created, as the controller looks up `batch/v1beta1` CronJobs | Docs review |
| DatabaseServer: readiness (unit test) | The pod is ready before MySQL accepts connections, as the probe always succeeds. The tests wait for MySQL itself. | Docs review |
| External identity provider: probe paths | Liveness and readiness run all health checks | #62 |
| External identity provider: outage | Every node is restarted | #62 |
| External identity provider: cluster deletion | Jobs are disabled on every node whose name starts with the cluster's resource name, including other clusters' nodes sharing the database | `disableTaskProcessing` matches node names with `LIKE` |
| External identity provider: scaling up | Existing nodes are not restarted, as the startup probe doesn't depend on the node count | Changes with the startup lock allowance |
| Operator upgrade | Nodes aren't restarted, as the previous release renders the same pod template | Changes with any pod template change |
| Bootstrap lock | Nodes waiting on the lock longer than the startup probe allows are restarted | Startup lock allowance |
| Internal identity provider: bundled configuration without HTTPS | Stroom exits as it starts, as the bundled configuration always has an HTTPS connector, whose key store only exists when HTTPS is enabled. The suite then enables HTTPS. | Found by the e2e tests |
| Internal identity provider: HTTPS | Stroom exits as it starts, as the key store init container runs as root and creates the key store readable only by root, while Stroom runs as the stroom user. The suite, and the extended suite, then run the pods as the stroom user, which works around it. | Found by the e2e tests |
| Internal identity provider: changing to the stroom user | The node lifecycle scripts' logs, created by root, can't be written by the stroom user, so the suite recreates the cluster with a new volume | Found by the e2e tests |
| Internal identity provider | Stroom starts, but node startup can't obtain a token, so the node never becomes ready | Stroom 7.13 rejects client credentials token requests with its internal identity provider |
| Extended: datafeed Ingress with HTTPS | Requests fail, as the Ingress sends plain HTTP to the nodes' HTTPS port | Docs review |

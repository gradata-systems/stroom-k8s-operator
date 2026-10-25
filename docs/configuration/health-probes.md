# Health probes

Each Stroom node has three Kubernetes probes, configured per node set:

| Probe | Field | What it does |
| --- | --- | --- |
| Startup | `startupProbeTimings` | Runs the node startup script, which enables the node and its managed jobs through the Stroom API. Succeeds once Stroom is up and the script has run. Until then, the other probes don't run. |
| Readiness | `readinessProbeTimings` | `GET /stroomAdmin/filteredhealthcheck` on the admin port (8081). While it fails, the node is removed from its Services, so it receives no traffic. |
| Liveness | `livenessProbeTimings` | The same endpoint, with its own filter. If it fails `failureThreshold` times in a row, Kubernetes restarts the container. |

!!! info "Version note"
    The health check filters (`allow` and `deny`), the separate startup probe settings (with
    `lockWaitAllowanceSeconds`) and the fix that makes `periodSeconds` take effect on the readiness and liveness
    probes were introduced in operator version 1.2.0. For existing clusters, see the
    [upgrade notes](../operations/upgrading.md#probe-changes).

## Defaults

| Setting | Startup | Readiness | Liveness |
| --- | --- | --- | --- |
| `initialDelaySeconds` | 5 | 5 | 5 |
| `periodSeconds` | 5 | 5 | 5 |
| `timeoutSeconds` | 5 | 5 | 5 |
| `successThreshold` | 1 | 1 | 1 |
| `failureThreshold` | 24 | 10 | 10 |
| `lockWaitAllowanceSeconds` | 30 | | |
| `allow` | | (none: all checks) | `[deadlocks]` |
| `deny` | | `[stroom.security.common.impl.ExternalIdpConfigurationProvider]` | (none) |

With these defaults:

- A node has about two minutes (5 s + 24 × 5 s) to start, more with `Parallel` pod management (see below).
- A node is taken out of service after 50 seconds of failed readiness checks.
- A node is restarted only after 50 seconds of failed liveness checks, and the liveness probe only checks for
  deadlocked JVM threads.

## Filtering health checks

Stroom registers a set of named health checks. The `filteredhealthcheck` endpoint runs a subset of them, selected
by the `allow` and `deny` lists:

- `allow`: run only these checks. If empty or omitted, run all checks.
- `deny`: don't run these checks.

The defaults are chosen so that an outage of a dependency doesn't take nodes down:

- **Liveness** checks only `deadlocks`. If it ran every check, an outage of the IdP or database would make every
  node fail its liveness probe, and Kubernetes would restart the whole cluster, which doesn't help and makes
  recovery slower.
- **Readiness** runs every check except the external IdP check, so nodes keep receiving data while the IdP is
  unavailable.

Stroom 7.13 registers these checks:

| Name | Checks |
| --- | --- |
| `deadlocks` | Deadlocked JVM threads |
| `stroom.config.global.impl.AppConfigMonitor` | Monitoring of the configuration file |
| `stroom.security.common.impl.ExternalIdpConfigurationProvider` | That the external IdP's configuration can be fetched |

To list the checks your Stroom version registers, with their current state:

```shell
kubectl -n stroom exec stroom-dev-node-data-0 -c stroom-node -- \
  curl -s 'http://localhost:8081/stroomAdmin/healthcheck?pretty=true'
```

!!! danger "Use only registered check names"
    If `allow` or `deny` contains a name Stroom doesn't recognise, the health check endpoint returns an error and
    the probe **always fails**. On the liveness probe, that restarts the node over and over. Check names against
    the list above, or the output of the command, before you change them, and check again after upgrading Stroom.

Names are case-sensitive and can't contain commas. Leading and trailing spaces are removed.

### Examples

To also ignore the configuration file monitor for readiness, list it with the IdP check. Setting `deny` replaces
the default list, so include the default entry too:

```yaml
    - name: data
      livenessProbeTimings:
        allow:            # the default, shown for clarity
          - deadlocks
      readinessProbeTimings:
        deny:
          - stroom.security.common.impl.ExternalIdpConfigurationProvider
          - stroom.config.global.impl.AppConfigMonitor
```

To run every health check on the liveness probe (the behaviour before the filters were introduced), set `allow`
to an empty list. Omitting `allow` gives you the default instead:

```yaml
      livenessProbeTimings:
        allow: []
```

To test a filter by hand before you apply it:

```shell
kubectl -n stroom exec stroom-dev-node-data-0 -c stroom-node -- \
  curl -s -w '\nHTTP %{http_code}\n' \
  'http://localhost:8081/stroomAdmin/filteredhealthcheck?allow=deadlocks&pretty=true'
```

A `200` response means healthy. Anything else fails the probe.

## Startup probe

The startup probe runs `/stroom/scripts/node-start.sh` every `periodSeconds`. It fails until Stroom's API
responds and the script has enabled the node and its jobs. Its log is `/stroom/logs/k8s/node-start.log` in the
node, and any API errors are in `/stroom/logs/k8s/api.log`.

Kubernetes restarts the container if the probe hasn't succeeded within about
`initialDelaySeconds + failureThreshold × periodSeconds`. The time a node needs depends mainly on whether it has to
wait for a database migration: when Stroom starts, one node takes Stroom's database bootstrap lock and creates or
migrates the schema, and the other nodes wait for it.

### Parallel pod management

With `spec.podManagementPolicy: Parallel`, all the nodes in a node set start at once, and all but one may wait for
the bootstrap lock. The operator therefore adds `lockWaitAllowanceSeconds` for each additional node in the node
set:

```
failureThreshold (effective) = failureThreshold + ceil(lockWaitAllowanceSeconds × (count − 1) / periodSeconds)
```

For example, a node set with `count: 4` and the defaults gets 24 + ceil(30 × 3 / 5) = 42, about 3.5 minutes. Set
`lockWaitAllowanceSeconds: 0` to turn this off. With `OrderedReady` (the default), nodes start one at a time and no
allowance is added.

!!! note "Scaling restarts a Parallel node set"
    Because the effective threshold depends on `count`, changing the `count` of a node set under `Parallel` pod
    management changes its Pod template, so the existing nodes are also rolled.

### Allowing for long upgrades

A Stroom upgrade with a long database migration can take longer than the startup window, and the migrating node
is then restarted part-way through. Before such an upgrade, raise `failureThreshold`, for example to `120` (10
minutes), together with the image change:

```yaml
    - name: data
      startupProbeTimings:
        failureThreshold: 120
```

## Seeing the probes Kubernetes uses

```shell
kubectl -n stroom get pod stroom-dev-node-data-0 \
  -o jsonpath='{range .spec.containers[?(@.name=="stroom-node")]}{.startupProbe}{"\n"}{.readinessProbe}{"\n"}{.livenessProbe}{"\n"}{end}'
```

Probe failures appear as events on the Pod: `kubectl -n stroom describe pod stroom-dev-node-data-0`.

Changing any probe setting changes the Pod template, so the node set is rolled.

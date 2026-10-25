# Changelog

## 1.2.0

### Upgrade notes

Read these before upgrading. See [Upgrading](https://gradata-systems.github.io/stroom-k8s-operator/operations/upgrading/)
for details.

- **The operator image moves to `ghcr.io/gradata-systems/stroom-k8s-operator`.** Releases up to 1.1.5 remain on
  Docker Hub. The Helm chart's `registry` value now replaces the registry in the new `image.repository` value, so
  mirrors are expected at `<registry>/gradata-systems/stroom-k8s-operator`.
- **Every node set rolls once**, as the Stroom node probes and security settings change.
- **Database servers restart once**, as their Pod is now updated to match the `DatabaseServer`.
- **Liveness and readiness probes filter Stroom's health checks** (#62). Liveness runs only the `deadlocks` check, and
  readiness runs every check except the external identity provider's, so an identity provider outage no longer
  restarts every node, and nodes keep receiving data. Set `allow: []` or `deny: []` to run every check.
- **Node sets that omit probe timings now get the CRD defaults**, rather than Kubernetes' probe defaults.
- **Existing `StroomCluster`s keep a startup `failureThreshold` of 10**, which the API server stored when they were
  created. Remove it, or set it to 24, to use the new default of about two minutes.
- **Scaling a node set rolls it**, when its pods start in parallel (`podManagementPolicy: Parallel`), as the startup
  probe allows more time the more nodes there are.
- **Stricter validation.** Database names, `DatabaseServer.spec.additionalConfig`, node `role` and
  `volumeClaimDeletePolicy` are now validated. A resource with an invalid value can't be changed until it's fixed.
- **Ingress TLS follows `ingress.secretName`**, rather than `spec.https`. Without a Secret, the Ingresses have no TLS
  section, where they previously had an empty one.
- **The operator can no longer list, watch, change or delete Secrets**, only get and create them.

### Fixed

- Liveness and readiness probes ignored `periodSeconds`, taking the period from `initialDelaySeconds`.
- Nodes waiting on the Stroom bootstrap lock, held while another node upgrades the database, were restarted. The
  startup probe now allows `lockWaitAllowanceSeconds` (default 30) for each additional node starting in parallel.
- A `StroomCluster` without a log sender was never deployed, as the operator couldn't add its finalizer.
- Database backups didn't run on Kubernetes 1.25 and later, as the CronJob was looked up with `batch/v1beta1`.
  Changes to a `DatabaseBackup` are now applied.
- Changes to a `DatabaseServer` were never applied, and its `podAnnotations` were ignored.
- A `DatabaseServer`'s readiness probe always succeeded, before MySQL accepted connections.
- A `DatabaseServer` whose name differed from its `StroomCluster`'s couldn't be used, as the claim recorded the wrong
  name.
- Deleting a `StroomCluster` disabled jobs on other clusters' nodes whose names started the same way, and waited for
  tasks on every node in the database.
- A node stopped without draining if its task count couldn't be retrieved.
- The datafeed Ingress failed with `spec.https`, as it had no HTTPS backend protocol, and an omitted `className` was
  sent as an empty class.
- Task autoscaling divided by zero without a CPU limit, and autoscalers for the same cluster interfered with each
  other.

### Security

- Database names were interpolated into SQL run as root, and into the backup job's shell command.
- With images other than `mysql/mysql-server`, the MySQL root password was the path `/etc/mysql/password/root`.
- The OpenID client secret was visible in the node container's process list.
- `DatabaseServer.spec.additionalConfig` could set any server option, including those giving file access or
  bypassing authentication.
- The operator could read and change every Secret in the cluster.
- The operator connected to MySQL without TLS.
- Pods now disallow privilege escalation, use the runtime's default seccomp profile, and don't mount a service
  account token, unless set otherwise.
- Built with Go 1.26.10, and updated golang.org/x/net and golang.org/x/text, fixing vulnerabilities reported by
  govulncheck. Base images are pinned by digest.

### Added

- Documentation, published at https://gradata-systems.github.io/stroom-k8s-operator/, with a CRD reference.
- Samples for an external identity provider, an operator-managed or external database, and NetworkPolicies.
- Unit, envtest and end-to-end tests, run against Stroom 7.13 with Keycloak, and releases published from GitHub
  releases.

### Known issues

- With Stroom 7.13's internal identity provider, the node lifecycle scripts can't obtain a token, so nodes never
  become ready. Use an external identity provider.

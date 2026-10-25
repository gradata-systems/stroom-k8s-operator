# Troubleshooting

The examples use the namespace `stroom`, the cluster `dev` and the node `stroom-dev-node-data-0`.

## Where to look

### The operator

The operator logs what it creates and updates, and any errors reconciling your resources:

```shell
kubectl -n stroom-operator-system logs deployment/stroom-operator-controller-manager -f
```

### Resources and events

```shell
kubectl -n stroom get stroomclusters,databaseservers,databasebackups,stroomtaskautoscalers
kubectl -n stroom get statefulsets,pods,pvc,ingresses -l stroom/cluster=dev
kubectl -n stroom describe pod stroom-dev-node-data-0      # probe failures, FailedPreStopHook, scheduling
kubectl -n stroom get events --sort-by=.lastTimestamp
```

### Stroom's console log

```shell
kubectl -n stroom logs stroom-dev-node-data-0 -c stroom-node -f
kubectl -n stroom logs stroom-dev-node-data-0 -c stroom-node --previous   # the container before its last restart
```

### Log files in the node

These are on the node's PersistentVolumeClaim, so they survive restarts:

| File | Contents |
| --- | --- |
| `/stroom/logs/k8s/node-start.log` | The startup script: node and jobs enabled |
| `/stroom/logs/k8s/pre-stop.log` | The `preStop` drain: jobs and node disabled, remaining task counts |
| `/stroom/logs/k8s/api.log` | Failed Stroom API and token requests made by either script |
| `/stroom/logs/app/app.log` | Stroom application log |
| `/stroom/logs/access/access.log` | HTTP access log |
| `/stroom/logs/user/user.log` | User (audit) events |
| `/stroom/logs/migration/migration.log` | Database migration SQL |

The `app`, `access` and `user` logs are rolled every minute (and removed once shipped, if
[log shipping](../configuration/log-shipping.md) is on). For example:

```shell
kubectl -n stroom exec stroom-dev-node-data-0 -c stroom-node -- tail -n 50 /stroom/logs/k8s/api.log
kubectl -n stroom exec stroom-dev-node-data-0 -c stroom-node -- ls -l /stroom/logs/app
```

### Health checks

```shell
# Every health check, with details
kubectl -n stroom exec stroom-dev-node-data-0 -c stroom-node -- \
  curl -s 'http://localhost:8081/stroomAdmin/healthcheck?pretty=true'

# Exactly what the liveness probe sees
kubectl -n stroom exec stroom-dev-node-data-0 -c stroom-node -- \
  curl -s -w '\nHTTP %{http_code}\n' 'http://localhost:8081/stroomAdmin/filteredhealthcheck?allow=deadlocks'
```

## Common problems

### No StatefulSets are created

Check the operator log. Common causes:

- `DatabaseServer '...' could not be found`: `spec.databaseServerRef.serverRef` names a `DatabaseServer` that
  doesn't exist in that namespace. The operator keeps retrying, with increasing back-off.
- `DatabaseServer ... already claimed by StroomCluster ...`: each `DatabaseServer` serves only one cluster.
- A Kubernetes validation error when updating a StatefulSet: you changed an immutable field. See
  [Applying changes](../configuration/index.md#applying-changes).

### Nodes restart during startup

The startup probe ran out of time. Look at `kubectl describe pod` for `Startup probe failed` events, then:

- `/stroom/logs/k8s/api.log`: token or API errors (see below).
- The console log: is Stroom still migrating the database, or waiting for the bootstrap lock? If so, raise
  `startupProbeTimings.failureThreshold`; see [Health probes](../configuration/health-probes.md#startup-probe).
- The console log: did Stroom fail to start, for example because of a configuration error or no database
  connection?

### Authentication failures in api.log

`Token request ... failed` or responses of `401` or `403` mean the scripts can't authenticate:

- Is Stroom configured with `identityProviderType: EXTERNAL_IDP`? The internal IdP isn't supported. See
  [Identity provider](../configuration/identity-provider.md).
- Is the IdP reachable from the Pod?
- Are the client ID and secret in `spec.openId` correct, and does the client allow the client credentials grant?
- Does the token's audience match what Stroom expects?
- Does the identity behind the token have permission in Stroom to manage nodes and jobs?

To test the token request from inside a node (this prints only the HTTP status, not the token):

```shell
kubectl -n stroom exec stroom-dev-node-data-0 -c stroom-node -- bash -c '
  curl -s -o /dev/null -w "HTTP %{http_code}\n" -X POST \
    -H "Content-Type: application/json" -H "Accept: text/plain" \
    -d "{\"clientId\": \"$STROOM_OPERATOR_OPENID_CLIENT_ID\", \"clientSecret\": \"$STROOM_OPERATOR_OPENID_CLIENT_SECRET\"}" \
    http://localhost:8080/api/authproxy/v1/fetchClientCredsToken'
```

### Nodes are Running but not Ready

The readiness probe is failing. Run the filtered health check the probe uses (from `kubectl describe pod`, under
_Readiness_) and see which check is unhealthy. If you changed `allow` or `deny`, check that every name is
registered: an unknown name always fails the probe.

### Nodes restart repeatedly after starting

The liveness probe is failing. If `livenessProbeTimings.allow` or `deny` contains a name Stroom doesn't register,
the probe can never pass; fix the name. Otherwise check the health check output for deadlocked threads, and the
previous container's log (`--previous`).

### MySQL can't change the data directory owner

The `DatabaseServer` Pod restarts repeatedly, and its log ends with:

```text
[ERROR] [MY-010174] [Server] Can't change data directory owner to mysql
[ERROR] [MY-013236] [Server] The designated data directory /var/lib/mysql/ is unusable.
```

This happens when the Kubernetes node's host has MySQL installed and AppArmor enabled, as on GitHub's Ubuntu
runners or a developer's machine running kind. AppArmor attaches the host's profile for `/usr/sbin/mysqld` to
mysqld in containers too, and the profile doesn't let the `mysql/mysql-server` image's mysqld, which runs as root,
take ownership of a new data directory. Unload the profile on the host:

```shell
sudo apparmor_parser -R /etc/apparmor.d/usr.sbin.mysqld
```

To keep it unloaded after a reboot, also link it into `/etc/apparmor.d/disable/`.

### A Pod is stuck terminating

It's probably draining. Check `/stroom/logs/k8s/pre-stop.log` for the remaining task count. The Pod is killed when
`nodeTerminationPeriodSecs` runs out. To stop it sooner, see [Force deletion](node-lifecycle.md#force-deletion).

### Deleting a StroomCluster or DatabaseServer hangs

- `StroomCluster`: the operator is waiting for processing tasks to finish; the operator log names the nodes and
  task counts. The check runs once a minute.
- `DatabaseServer`: it waits for the `StroomCluster` using it to be deleted.
- Either: is the operator running? Its finalizers block deletion until it handles them.

### The autoscaler does nothing

- Is metrics-server installed? `kubectl top pod -n stroom` should show usage.
- Does the node set have `resources.limits.cpu`?
- Look for `PodMetrics not found` and `CPU usage for Stroom node` in the operator log.
- Limits are only raised on nodes already running as many tasks as their limit allows.

### Containers are OOMKilled

Lower `memoryOptions.maxPercentage` or raise the memory limit. See
[Resources and JVM memory](../configuration/resources-jvm.md).

### The Ingress returns 404 or 502

- Does `ingress.className` match an IngressClass in the cluster? Always set it: the operator sends the value even
  when it's empty, and the API server may reject an Ingress with an empty class name (look for Ingress errors in the
  operator log).
- With `spec.https`, the Ingress must connect to the nodes over HTTPS; see
  [Default annotations](../configuration/ingress-tls.md#default-annotations).
- Are the nodes `Ready`? Un-ready nodes are removed from the Service.

# Stroom configuration

Each node reads its configuration from `/stroom/config/config.yml`.

## The bundled configuration

By default, the operator mounts its own configuration, which it stores in the ConfigMap
`stroom-<cluster>-static-content`. There are two variants, which differ only in the HTTPS connector:

| Key | Used when | Source |
| --- | --- | --- |
| `stroomcluster-config.yaml` | `spec.https` isn't set | [`stroomcluster-config.yaml`](https://github.com/gradata-systems/stroom-k8s-operator/blob/master/internal/controller/static_content/stroomcluster-config.yaml) |
| `stroomcluster-config-https.yaml` | `spec.https` is set | [`stroomcluster-config-https.yaml`](https://github.com/gradata-systems/stroom-k8s-operator/blob/master/internal/controller/static_content/stroomcluster-config-https.yaml) |

To see the version your operator uses:

```shell
kubectl -n stroom get configmap stroom-dev-static-content \
  -o jsonpath='{.data.stroomcluster-config-https\.yaml}'
```

It takes most values from environment variables that the operator sets (see below). It's a good starting point,
but you will need your own configuration to use an [external identity provider](identity-provider.md), which the
operator requires.

## Supplying your own configuration

Put your `config.yml` in a ConfigMap in the cluster's namespace and reference it:

```shell
kubectl -n stroom create configmap stroom-config --from-file=config.yml
```

```yaml
spec:
  configMapRef:
    name: stroom-config    # ConfigMap name
    itemName: config.yml   # key holding the configuration
```

Start from the bundled configuration and keep the parts the operator relies on:

- The application connector on port `${STROOM_APP_HTTP_PORT}` (8080). The lifecycle scripts call the API at
  `http://localhost:8080/api`.
- The admin connector on port `${STROOM_ADMIN_PORT}` (8081) and `adminContextPath: /stroomAdmin`. The liveness and
  readiness probes use `/stroomAdmin/filteredhealthcheck`.
- `appConfig.node.name: ${STROOM_NODE}`. The node name must be the Pod name, which the scripts, cluster deletion
  and the autoscaler use to find the node.
- `appConfig.nodeUri.hostname: ${POD_HOSTNAME}.${POD_SUBDOMAIN}`, so nodes can reach each other.
- The database connections, which use the `STROOM_JDBC_*` and `STROOM_STATISTICS_JDBC_*` variables.
- The log files under `logs/app`, `logs/access` and `logs/user`, rolled every minute, if you use
  [log shipping](log-shipping.md).
- The HTTPS connector, with the keystore and truststore paths (`/stroom/pki/tls/...`), only if you set
  `spec.https`. Without `spec.https` the keystore doesn't exist, and Stroom exits as it starts.

See the [Stroom configuration documentation](https://gchq.github.io/stroom-docs/docs/install-guide/configuration/stroom-and-proxy/configuring-stroom/)
for everything else.

### Changing the configuration

The ConfigMap is mounted as a single file (`subPath`), so running Pods never see changes to it. After updating it,
restart the nodes; see [Rolling restarts](../operations/node-lifecycle.md#rolling-restarts). Changing
`configMapRef` itself rolls the nodes automatically.

## Environment variables set by the operator

You can refer to these in your configuration with `${NAME}` or `${NAME:-default}`:

| Variable | Value |
| --- | --- |
| `API_GATEWAY_HOST` | `spec.ingress.hostName` |
| `ADMIN_CONTEXT_PATH` | `/stroomAdmin` |
| `APPLICATION_CONTEXT_PATH` | `/` |
| `STROOM_APP_HTTP_PORT`, `STROOM_APP_HTTPS_PORT`, `STROOM_ADMIN_PORT` | `8080`, `8443`, `8081` |
| `STROOM_NODE`, `POD_HOSTNAME`, `DOCKER_HOST_HOSTNAME` | The Pod name, for example `stroom-dev-node-data-0` |
| `POD_SUBDOMAIN` | `<headless service>.<namespace>.svc.cluster.local` |
| `DOCKER_HOST_IP` | The Pod IP |
| `STROOM_NODE_ROLE` | The node set's `role` (empty if none) |
| `STROOM_NODE_MANAGED_JOBS` | Comma-separated managed job names (empty means all jobs) |
| `JAVA_OPTS` | JVM options; see [Resources and JVM memory](resources-jvm.md) |
| `STROOM_JDBC_DRIVER_URL`, `STROOM_JDBC_DRIVER_CLASS_NAME`, `STROOM_JDBC_DRIVER_USERNAME`, `STROOM_JDBC_DRIVER_PASSWORD` | Connection to the `appDatabaseName` database |
| `STROOM_STATISTICS_JDBC_DRIVER_URL`, `..._CLASS_NAME`, `..._USERNAME`, `..._PASSWORD` | Connection to the `statsDatabaseName` database |
| `STROOM_TEMP` | `/stroom/tmp` |
| `STROOM_OPERATOR_OPENID_CLIENT_ID`, `STROOM_OPERATOR_OPENID_CLIENT_SECRET` | From `spec.openId` |
| `STROOM_KEYSTORE_PASSWORD` | From `spec.https.tlsKeystorePasswordSecret` (only when `spec.https` is set) |

The JDBC URLs have the form `jdbc:mysql://<host>:<port>/<database>?serverTimezone=UTC&useUnicode=yes&characterEncoding=UTF-8`.

## Extra environment variables

`spec.extraEnv` adds environment variables to every node. Use it to pass values your configuration refers to:

```yaml
spec:
  extraEnv:
    - name: STROOM_LOGGING_LEVEL
      value: INFO
    - name: KAFKA_PASSWORD
      valueFrom:
        secretKeyRef:
          name: kafka-credentials
          key: password
```

Don't redefine the variables the operator sets, except `JAVA_OPTS`, which the operator handles specially (see
[Resources and JVM memory](resources-jvm.md#overriding-java_opts)).

## Extra files

To give Stroom extra files, such as certificates or a truststore for a Kafka connection, mount them from a Secret
or ConfigMap with `spec.extraVolumes` and `spec.extraVolumeMounts`. See
[Storage](storage.md#shared-storage-for-stroom-volumes) for the syntax.

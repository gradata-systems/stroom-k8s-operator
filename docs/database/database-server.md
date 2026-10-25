# Operator-managed DatabaseServer

A `DatabaseServer` is a single-instance MySQL 8 server that the operator runs for one `StroomCluster`. The
operator generates its passwords and creates the Stroom databases. It suits evaluation and smaller deployments.
For production, consider an [external database](external-database.md) with replication and managed backups:
a `DatabaseServer` has one replica and no failover.

## Create a DatabaseServer

```yaml
apiVersion: stroom.gchq.github.io/v1
kind: DatabaseServer
metadata:
  name: dev
  namespace: stroom
spec:
  image:
    repository: mysql
    tag: "8.4"
  databaseNames:
    - stroom
    - stats
  additionalConfig:
    - max_connections=1000
  resources:
    requests:
      cpu: "1"
      memory: 4Gi
    limits:
      memory: 4Gi
  volumeClaim:
    storageClassName: fast-ssd
    accessModes:
      - ReadWriteOnce
    resources:
      requests:
        storage: 100Gi
```

See also [`samples/operator-database/database-server.yaml`](https://github.com/gradata-systems/stroom-k8s-operator/blob/master/samples/operator-database/database-server.yaml).

| Field | Description |
| --- | --- |
| `image` | A MySQL 8 server image. The operator relies on the image's standard entrypoint (`MYSQL_USER`, `MYSQL_PASSWORD` and `/docker-entrypoint-initdb.d`). |
| `databaseNames` | Databases to create on first start. Include the `StroomCluster`'s `appDatabaseName` and `statsDatabaseName` (by default `stroom` and `stats`). |
| `additionalConfig` | Lines appended to the `[mysqld]` section of `/etc/my.cnf`, each a single option such as `max_connections=500`. Options the operator sets (`datadir`, `port`, `user`), and options that give access to the server's files or bypass authentication (such as `secure_file_priv`, `local_infile`, `init_file` and `skip_grant_tables`), are rejected. |
| `resources` | Container requests and limits. |
| `volumeClaim` | PersistentVolumeClaim spec for the data directory. Use fast, low-latency storage. |
| `readinessProbeTimings`, `livenessProbeTimings` | Probe timings. If omitted, Kubernetes' probe defaults apply. |
| `podSecurityContext`, `securityContext`, `nodeSelector`, `tolerations`, `affinity` | Pod settings. |
| `podAnnotations` | Annotations for the database Pod. |

### Connection limit

MySQL allows 151 connections by default. Each Stroom node opens connection pools to both databases, and the
operator also connects, so a cluster of more than a few nodes can exceed the default. Set `max_connections` in
`additionalConfig`, with headroom.

## Deploy and check

```shell
kubectl apply -f database-server.yaml
kubectl -n stroom get databaseserver dev
kubectl -n stroom logs stroom-dev-db-0 -f
```

The `State` column shows `Deployed` once the operator has created the database objects. The Pod is
`stroom-<name>-db-0` and the server is reachable in the cluster at
`stroom-<name>-db-headless.<namespace>.svc.cluster.local:3306`.

## Connect a StroomCluster

```yaml
spec:
  databaseServerRef:
    serverRef:
      name: dev
      # namespace: stroom   # defaults to the StroomCluster's namespace
```

A `DatabaseServer` can serve only one `StroomCluster`. The first cluster to reference it claims it. A second
cluster referencing it fails to reconcile, with the error `DatabaseServer ... already claimed by StroomCluster ...`
in the operator log.

While a `StroomCluster` uses it, the `DatabaseServer` can't be deleted: the deletion waits until the cluster has
gone.

!!! note "Operator 1.1.5 and earlier"
    Earlier versions recorded the claim using the `DatabaseServer`'s own name rather than the cluster's, so a
    `StroomCluster` whose `DatabaseServer` had a different name failed with `already claimed`. The operator now
    replaces a claim by a `StroomCluster` that doesn't exist, which repairs these claims when you upgrade.

## Credentials

The operator generates a Secret, `stroom-<name>-db`, with passwords under the keys `root` and `stroomuser`. Stroom
and the operator connect as `stroomuser`, which is granted all privileges on each of `databaseNames`.

!!! warning "Back up this Secret"
    The Secret isn't deleted with the `DatabaseServer`, but if it's lost the data can't be reached with the
    generated credentials. Keep a copy somewhere secure:

    ```shell
    kubectl -n stroom get secret stroom-dev-db -o yaml > stroom-dev-db-secret.yaml
    ```

## Changing a DatabaseServer

Changes to a `DatabaseServer`'s image, resources, probe timings, Pod settings and `additionalConfig` are applied
to the running server. Changing anything in its Pod restarts MySQL, so Stroom loses its database connection
briefly; plan changes for a quiet period. A change to `additionalConfig` alone updates the configuration, which
MySQL reads when it next starts.

While the server's Pod is being replaced, and until it's ready again, the operator doesn't create or change the
nodes of the `StroomCluster` using it, as a node stopping then couldn't drain. If you change both at once, or an
operator upgrade changes both, MySQL restarts first, then the nodes are rolled. The operator logs
`Waiting for the DatabaseServer before creating or updating the nodes` while it waits. A new `StroomCluster`'s
nodes are likewise created once its `DatabaseServer` is ready.

Some changes need more:

- **`volumeClaim`.** Kubernetes doesn't allow a StatefulSet's volume claim template to change, so the operator
  keeps the original. To change it, delete the `DatabaseServer` (its PersistentVolumeClaim
  `data-stroom-<name>-db-0` and Secret are kept), resize or replace the volume claim yourself, and apply the
  `DatabaseServer` again.
- **`databaseNames`.** The databases are created when MySQL first initialises its data directory. Adding a name
  later doesn't create the database; create it, and grant `stroomuser` access to it, yourself.
- **The passwords.** The generated Secret is never changed by the operator.

Take a [backup](backups.md) before changing the image. Upgrading MySQL to a new major or minor version also needs
the usual MySQL upgrade checks.

!!! note "Operator 1.1.5 and earlier"
    Earlier versions created these resources once and didn't update them afterwards. After upgrading the operator,
    the database server's Pod is updated to match the `DatabaseServer`, which restarts MySQL once.

## Deleting a DatabaseServer

```shell
kubectl -n stroom delete databaseserver dev
```

If a `StroomCluster` still uses it, the command waits (press ++ctrl+c++ to stop waiting; the deletion continues in
the background) until that cluster is deleted. The data volume claim and the Secret are kept. Delete them yourself
if you no longer need the data.

# External database

Instead of a `DatabaseServer`, you can connect Stroom to an existing MySQL 8 server, such as a managed cloud
database or a replicated MySQL cluster.

## 1. Create the databases and user

On the MySQL server, create the two Stroom databases and a user with full rights on them:

```sql
CREATE DATABASE stroom;
CREATE DATABASE stats;
CREATE USER 'stroomuser'@'%' IDENTIFIED BY '<password>';
GRANT ALL PRIVILEGES ON stroom.* TO 'stroomuser'@'%';
GRANT ALL PRIVILEGES ON stats.* TO 'stroomuser'@'%';
```

The database names must match the `StroomCluster`'s `appDatabaseName` and `statsDatabaseName` (by default
`stroom` and `stats`). Stroom creates the schema itself when it first starts. Restrict the user's host (`'%'`) to
your cluster's address range if you can. Set `max_connections` high enough for your node count; see
[Connection limit](database-server.md#connection-limit).

## 2. Store the password in a Secret

Create a Secret in the `StroomCluster`'s namespace. The **key must be the database user name** (`stroomuser` by
default) and the value its password:

```shell
kubectl -n stroom create secret generic stroom-db-credentials \
  --from-literal=stroomuser='<password>'
```

## 3. Reference the server

```yaml
spec:
  databaseServerRef:
    userName: stroomuser          # default; must match the Secret key
    serverAddress:
      host: mysql.example.com
      port: 3306                  # default
      secretName: stroom-db-credentials
```

Don't set `serverRef` as well: if `serverRef` is set, the operator uses the `DatabaseServer` and ignores
`serverAddress`.

## Things to know

- **Network access.** The operator Pod, as well as the Stroom Pods, connects to the database: it drains the cluster
  through the database when the `StroomCluster` is deleted, and the autoscaler reads and sets task limits there.
- **TLS.** The operator and Stroom both use TLS when the server supports it, which MySQL 8 does by default, so
  passwords aren't sent in clear text. Neither verifies the server's certificate, as MySQL servers generate their
  own by default. If you need certificate verification for Stroom's connections, configure it in your own
  [configuration](../configuration/stroom-config.md).
- **Password changes.** The password is read from the Secret when each Pod starts. After changing it, update the
  Secret and restart the nodes; see [Rolling restarts](../operations/node-lifecycle.md#rolling-restarts).
- **Backups.** A [`DatabaseBackup`](backups.md) can back up an external server too, but your database service's own
  backups are usually a better choice.

The [`external-database`](https://github.com/gradata-systems/stroom-k8s-operator/tree/master/samples/external-database)
sample is a complete deployment using an external server.

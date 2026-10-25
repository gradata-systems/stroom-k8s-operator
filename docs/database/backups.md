# Database backups

A `DatabaseBackup` runs `mysqldump` on a schedule, as a Kubernetes CronJob, and writes compressed dumps to a volume
you provide.

!!! note "Operator 1.1.5 and earlier"
    Earlier operator versions looked up the CronJob through the `batch/v1beta1` API, which Kubernetes 1.25 removed,
    so on current clusters no CronJob was created and no backups ran. If you created a `DatabaseBackup` with one of
    those versions, check that `kubectl -n <namespace> get cronjob stroom-<name>-db-backup` exists after upgrading.

## Create a DatabaseBackup

```yaml
apiVersion: stroom.gchq.github.io/v1
kind: DatabaseBackup
metadata:
  name: dev
  namespace: stroom
spec:
  image:
    repository: mysql
    tag: "8.4"
  schedule: "0 2 * * *"           # every day at 02:00
  databaseServerRef:
    serverRef:
      name: dev
  databaseNames:
    - stroom
    - stats
  volume:
    nfs:
      server: backup.example.com
      path: /exports/stroom-backups
```

See also [`samples/operator-database/database-backup.yaml`](https://github.com/gradata-systems/stroom-k8s-operator/blob/master/samples/operator-database/database-backup.yaml).

| Field | Description |
| --- | --- |
| `image` | An image with `bash`, `mysqldump` and `gzip`. Use the official `mysql:8.4` image, which can back up MySQL 8.0 servers too. The `mysql/mysql-server` images have no `gzip`, and the `mysqldump` in 8.0.32 needs the `RELOAD` privilege, which the database user doesn't have. |
| `schedule` | A [cron schedule](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/#schedule-syntax). |
| `databaseServerRef` | A `DatabaseServer` (`serverRef`) or an external server (`serverAddress` and `userName`), as for a [`StroomCluster`](external-database.md#3-reference-the-server). If both are set, `serverRef` is used. |
| `databaseNames` | Databases to back up. If omitted, all databases are backed up (`--all-databases`). |
| `volume` | Any Kubernetes [volume source](https://kubernetes.io/docs/concepts/storage/volumes/), such as `nfs` or `persistentVolumeClaim`, where the dumps are written. |

```shell
kubectl apply -f database-backup.yaml
kubectl -n stroom get cronjob stroom-dev-db-backup
```

To run a backup now, outside the schedule:

```shell
kubectl -n stroom create job --from=cronjob/stroom-dev-db-backup stroom-dev-db-backup-manual
kubectl -n stroom logs -f job/stroom-dev-db-backup-manual
```

## What a backup contains

Each run connects as the database user (by default `stroomuser`, with the password from the database Secret) and
runs:

```shell
mysqldump --single-transaction --no-tablespaces --databases stroom stats | gzip > "<file>"
```

The file is written to `<volume>/<YYYY-MM>/<backup name>_<YYYY-MM-DD_HH-MM-SS>.sql.gz` and made read-only, and the
log ends with `Backup successful`. If `mysqldump` fails, the partial file is removed, the log ends with
`Backup failed`, and the Job fails, so failed backups show up as failed Jobs. Runs never overlap
(`concurrencyPolicy: Forbid`), and finished Job Pods are removed after five minutes, so check the outcome soon after
a run, or rely on your monitoring.

Things to bear in mind:

- **No retention.** Old backups are never deleted. Prune the backup volume yourself.
- **Consistency.** `--single-transaction` gives a consistent snapshot without locking tables, but conflicts with
  schema changes. Don't upgrade Stroom, which may migrate the schema, while a backup is running.
- **Changes.** Changes to a `DatabaseBackup`, such as its schedule, are applied to its CronJob. A backup already
  running isn't affected.
- **The Secret.** The dumps don't include the database Secret. Back it up separately; see
  [Credentials](database-server.md#credentials).

## Restoring a backup

A restore replaces the contents of the Stroom databases. Test the procedure before you need it.

1. Stop Stroom: [delete the `StroomCluster`](../operations/stopping-and-removal.md#stopping-a-stroom-cluster) and wait
   for it to go.
2. Load the dump into the database server. For an operator-managed `DatabaseServer`:

    ```shell
    gunzip -c dev_2026-10-25_02-00-00.sql.gz | \
      kubectl -n stroom exec -i stroom-dev-db-0 -- sh -c 'mysql -u stroomuser -p"$MYSQL_PASSWORD"'
    ```

3. Apply the `StroomCluster` again.

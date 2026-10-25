# Storage

A Stroom node needs two kinds of storage:

- **Local storage**, private to the node: logs, LMDB reference data, search results, temporary files and so on.
  The operator gives each node its own PersistentVolumeClaim for this.
- **Shared storage** for Stroom's data volumes, which hold the stream data, mounted into every node. You add this
  with extra volumes.

## Local storage

`localDataVolumeClaim` is a standard
[PersistentVolumeClaim spec](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims).
The StatefulSet creates one claim per node, named `data-stroom-<cluster>-node-<node set>-<ordinal>`:

```yaml
    - name: data
      localDataVolumeClaim:
        storageClassName: fast-ssd
        accessModes:
          - ReadWriteOnce
        resources:
          requests:
            storage: 50Gi
```

Use fast, low-latency storage: LMDB performance depends on it. If you pre-provision PersistentVolumes instead of
using a StorageClass, create one per node and use `selector` or `volumeName` to bind them.

The claim is mounted at these paths in the Stroom container:

| Path in container | Sub-directory of the volume | Contents |
| --- | --- | --- |
| `/stroom/logs` | `logs` | Stroom application, access, user (audit) and migration logs, and the operator's lifecycle script logs in `/stroom/logs/k8s` |
| `/stroom/output` | `output` | Output written by pipelines to the local file system |
| `/stroom/tmp` | `tmp` | Stroom's temporary directory (`STROOM_TEMP`) |
| `/stroom/lmdb` | `lmdb` | LMDB stores |
| `/stroom/lmdb_library` | `lmdb_library` | The LMDB native library |
| `/stroom/planb` | `planb` | Plan B state stores |
| `/stroom/proxy_repo` | `proxy-repo` | Proxy repository |
| `/stroom/reference_data` | `reference-data` | Reference data (LMDB) |
| `/stroom/reference_staging_data` | `reference_staging_data` | Reference data staging |
| `/stroom/search_results` | `search-results` | Search result stores |

When sizing the claim, allow for logs (the bundled configuration keeps up to a week of minute-rolled application
logs), reference data and search results.

!!! warning "Paths that are not persisted"
    The Stroom 7.13 image declares other directories as volumes, including `/stroom/volumes`,
    `/stroom/content_index`, `/stroom/git_repo` and `/stroom/data_feed_identities`. The operator doesn't mount
    anything there, and Kubernetes doesn't create volumes for image `VOLUME` declarations, so anything Stroom
    writes there is lost when the Pod is recreated. If your Stroom configuration uses any of these paths, mount
    persistent storage there with `extraVolumes` and `extraVolumeMounts`.

### Resizing local storage

Kubernetes doesn't allow a StatefulSet's volume claim template to change, so changing `localDataVolumeClaim` on an
existing node set fails (see [Applying changes](index.md#applying-changes)). To grow existing volumes, expand each
claim directly, if your StorageClass allows volume expansion:

```shell
kubectl -n stroom patch pvc data-stroom-dev-node-data-0 \
  -p '{"spec": {"resources": {"requests": {"storage": "100Gi"}}}}'
```

New nodes added later still get the size in the template.

## Shared storage for Stroom volumes

Stroom stores stream data in data volumes, which you configure in Stroom (_Volumes_ in the user interface). In a
cluster, every node must be able to reach them, so put them on shared storage mounted into every node.

`spec.extraVolumes` defines volumes for every node Pod. `spec.extraVolumeMounts` mounts them into the Stroom
container of every node set, and `nodeSets[].extraVolumeMounts` adds mounts for one node set only:

```yaml
spec:
  extraVolumes:
    - name: shared
      nfs:
        server: nfs.example.com
        path: /exports/stroom
  extraVolumeMounts:
    - name: shared
      mountPath: /stroom/shared
```

Any volume type works, for example a `persistentVolumeClaim` for a `ReadWriteMany` claim that you create. Then, in
Stroom, create the data volumes under the mount path, for example `/stroom/shared/data`.

You can use the same mechanism to provide other files to Stroom, such as certificates for a Kafka connection,
from a Secret or ConfigMap.

## Deleting PersistentVolumeClaims

By default, PersistentVolumeClaims are never deleted. When a node is recreated, or the cluster is deleted and
created again, each node gets back the claim (and data) it had before. `spec.volumeClaimDeletePolicy` changes this:

| `volumeClaimDeletePolicy` | Scale-down | Cluster deletion |
| --- | --- | --- |
| Not set (default) | Claims kept | Claims kept |
| `DeleteOnScaledownOnly` | Claims of the removed nodes deleted | Claims kept |
| `DeleteOnScaledownAndClusterDeletion` | Claims of the removed nodes deleted | All the cluster's claims deleted |

Whether the PersistentVolume, and its data, survives a deleted claim depends on the volume's reclaim policy. With
`Retain`, the volume moves to `Released` and must be cleaned up and made `Available` again by hand before it can be
reused.

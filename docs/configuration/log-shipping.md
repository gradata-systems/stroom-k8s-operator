# Log shipping

Stroom writes its own access, application and user (audit) logs to files. The operator can add a
[stroom-log-sender](https://github.com/gchq/stroom-log-sender) sidecar to every node that posts those logs back
into Stroom, so you can search and audit them there.

## Enabling the log sender

```yaml
spec:
  logSender:
    enabled: true
    image:
      repository: gchq/stroom-log-sender
      tag: v2.2.1
```

`image` is required when `logSender` is present. `enabled` defaults to `false`.

| Field | Default | Description |
| --- | --- | --- |
| `destinationUrl` | `https://<ingress host>/stroom/datafeeddirect` | Where to post the logs. The default sends them to the cluster's own processing nodes. |
| `environmentName` | The cluster name in upper case, for example `DEV` | The `Environment` header sent with the data. |
| `systemName` | `Stroom` | The `System` header sent with the data. |
| `resources` | Limits `cpu: 500m`, `memory: 256Mi` | Sidecar resources. |
| `securityContext` | None | Sidecar security context. Privilege escalation is disallowed unless set otherwise; see [Security](security.md). |
| `imagePullPolicy` | | Pull policy for the sidecar image. |
| `tls.secretName` | None | Client certificate authentication; see below. |

## Feeds

Every minute, the sidecar posts each completed (rolled) log file to one of these feeds, and deletes files once they
have been sent:

| Log directory | Feed |
| --- | --- |
| `/stroom/logs/access` | `STROOM-ACCESS-EVENTS` |
| `/stroom/logs/app` | `STROOM-APP-EVENTS` |
| `/stroom/logs/user` | `STROOM-USER-EVENTS` |

Create these feeds in the destination Stroom, with pipelines to process them, before you enable the log sender.

The sidecar relies on the log file layout in the bundled configuration: files rolled every minute and named like
`app-2026-10-25T10:15.log.gz`. Keep that layout if you [supply your own configuration](stroom-config.md).

## TLS

Without `tls`, the sidecar posts over HTTPS **without verifying the server's certificate** (`--no-secure`).

To verify the server and authenticate with a client certificate, create a Secret containing `ca.crt`, `tls.crt`
and `tls.key`, and reference it:

```yaml
spec:
  logSender:
    tls:
      secretName: stroom-log-sender-tls
```

If the destination Stroom requires authentication for data receipt, configure it to accept the client certificate.

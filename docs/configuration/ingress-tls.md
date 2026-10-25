# Ingress and TLS

## Ingress settings

```yaml
spec:
  ingress:
    hostName: stroom.example.com   # public DNS name (required)
    className: nginx               # IngressClass to use; if omitted, the cluster's default IngressClass is used
    secretName: stroom-tls         # TLS Secret for the Ingress; if omitted, the Ingresses have no TLS section
    pathTypeOverride: false        # true: use pathType ImplementationSpecific for every path
```

`hostName` is also passed to Stroom as its public host name (`API_GATEWAY_HOST`), with port 443. If your public
port differs, set `API_GATEWAY_PORT` in [`extraEnv`](stroom-config.md#extra-environment-variables).

## Routing

The operator creates up to two Ingresses for the cluster, from the node sets whose `ingressEnabled` is `true` (the
default):

| Ingress | Created for | Path | Routed to |
| --- | --- | --- | --- |
| `stroom-<cluster>` | A node set with role `Frontend` or no role | `/stroom/noauth/datafeed` (exact) | The first node set that isn't `Frontend` |
| | | `/` (prefix) | That node set |
| `stroom-<cluster>-datafeed` | A node set with role `Processing` or no role | `/stroom/datafeeddirect` (exact), rewritten to `/stroom/noauth/datafeed` | That node set |

So with `Processing` and `Frontend` node sets, event data posted to `https://<host>/stroom/datafeeddirect` or
`https://<host>/stroom/noauth/datafeed` goes to the processing nodes, and everything else goes to the frontend
nodes.

!!! note "Other datafeed paths"
    Only the two paths above are routed to processing nodes. Data posted to any other receipt path, for example
    `/stroom/datafeed`, matches the `/` rule and is received by a frontend node. Point data senders at
    `/stroom/datafeeddirect`.

### Default annotations

The operator adds these annotations, intended for [ingress-nginx](https://kubernetes.github.io/ingress-nginx/):

| Ingress | Annotation | Value |
| --- | --- | --- |
| `stroom-<cluster>` | `nginx.ingress.kubernetes.io/backend-protocol` | `HTTPS` if `spec.https` is set, otherwise `HTTP` |
| | `nginx.ingress.kubernetes.io/affinity` | `cookie` (sticky sessions for the UI) |
| | `nginx.ingress.kubernetes.io/affinity-mode` | `persistent` |
| | `nginx.ingress.kubernetes.io/proxy-body-size` | `0` (no request size limit) |
| `stroom-<cluster>-datafeed` | `nginx.ingress.kubernetes.io/backend-protocol` | `HTTPS` if `spec.https` is set, otherwise `HTTP` |
| | `nginx.ingress.kubernetes.io/rewrite-target` | `/stroom/noauth/datafeed` |
| | `haproxy.router.openshift.io/rewrite-target` | `/stroom/noauth/datafeed` |
| | `nginx.ingress.kubernetes.io/proxy-body-size` | `0` |

Add or override annotations and labels per node set. They apply to the Ingresses created for that node set:

```yaml
    - name: ui
      role: Frontend
      ingressAnnotations:
        nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
      ingressLabels:
        team: data-platform
```

### Other Ingress controllers

!!! warning "ingress-nginx is retired"
    The Kubernetes project retired ingress-nginx in March 2026, and no longer releases fixes for it, including
    security fixes. Existing installations keep working, and the operator's tests still run against its last
    release, but plan to move to another controller. The operator keeps adding the ingress-nginx annotations, which
    other controllers ignore.

With a controller other than ingress-nginx, use `ingressAnnotations` to configure the equivalent behaviour:
rewriting `/stroom/datafeeddirect` to `/stroom/noauth/datafeed`, the backend protocol, unlimited request bodies and
sticky sessions for the user interface. If your controller needs `pathType: ImplementationSpecific` (for example to
interpret paths as regular expressions), set `ingress.pathTypeOverride: true`.

## TLS at the Ingress

When `ingress.secretName` is set, each Ingress has a TLS section for `ingress.hostName` using that Secret, a
standard `kubernetes.io/tls` Secret with `tls.crt` and `tls.key`. Otherwise the Ingresses have no TLS section, and
whether clients can use HTTPS depends on your Ingress controller. TLS at the Ingress is independent of
[HTTPS inside the cluster](#https-inside-the-cluster), which encrypts traffic from the Ingress controller to the
nodes.

!!! note "Operator 1.1.5 and earlier"
    Earlier versions only used `ingress.secretName` when `spec.https` was set, and otherwise gave the Ingresses an
    empty TLS entry, so ingress-nginx served its default certificate and redirected HTTP to HTTPS. They also sent
    an empty `ingressClassName` when `className` was omitted, and the datafeed Ingress had no `backend-protocol`
    annotation, so it failed when `spec.https` was set.

## HTTPS inside the cluster

`spec.https` makes the Stroom nodes serve HTTPS on port 8443, and the Ingress connects to them over HTTPS:

```yaml
spec:
  https:
    tlsSecretName: stroom-node-tls
    tlsKeystorePasswordSecret:
      secretName: stroom-keystore
      key: password
```

| Field | Contents |
| --- | --- |
| `tlsSecretName` | A Secret containing `tls.crt` (the server certificate), `tls.key` (its private key) and `ca.crt` (the CA certificate). A Secret issued by cert-manager has this layout. |
| `tlsKeystorePasswordSecret` | The Secret name and key holding the password for the generated keystore and truststore. |

An init container (`generate-keystore`) converts the certificate into PKCS#12 files, `keystore.p12` (the
certificate and key) and `truststore.p12` (the CA), in a Pod-local volume mounted at `/stroom/pki/tls`, readable
only by the user Stroom runs as (`stroom`, unless the Pod security context sets another). With
`spec.https` set, the operator mounts the variant of its bundled Stroom configuration that has an HTTPS connector
on port 8443, reading them from there. Your own configuration needs the same connector (see
[Stroom configuration](stroom-config.md)).

The keystore is built when the Pod starts. After the certificate is renewed, restart the nodes so they pick it up.
See [Rolling restarts](../operations/node-lifecycle.md#rolling-restarts).

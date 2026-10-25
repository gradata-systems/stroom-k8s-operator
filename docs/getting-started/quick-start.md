# Quick start

This walk-through deploys a small Stroom 7.13 cluster with an operator-managed database, two processing nodes and
one frontend node. It's meant for evaluation. Read [Configuring a StroomCluster](../configuration/index.md) before
you build a production cluster.

The examples use the namespace `stroom`, a cluster named `dev` and the host name `stroom.example.com`. Change
them to suit.

## Before you begin

You need:

- The operator [installed](../installation/index.md), with its Pod `Ready`.
- A DNS name for Stroom that resolves to your Ingress controller.
- A TLS certificate and key for that name, and the CA certificate that issued it.
- An OpenID Connect client for Stroom at an external identity provider (IdP). See the next step.
- A StorageClass that can provision `ReadWriteOnce` volumes, or PersistentVolumes created in advance.

## 1. Create the namespace

```shell
kubectl create namespace stroom
```

## 2. Register Stroom with your identity provider

Create a confidential OpenID Connect client for Stroom. For Keycloak, follow the
[Stroom KeyCloak guide](https://gchq.github.io/stroom-docs/docs/install-guide/setup/open-id/external-idp/keycloak/),
and in addition:

- Turn on **Client authentication** and **Service accounts roles**, so the client can use the client credentials
  grant. The operator's lifecycle scripts depend on it.
- Set the valid redirect URI to `https://stroom.example.com/api/auth/flow/v1/signin-oidc`.
- Add an audience mapper so that access tokens carry the client ID in their `aud` claim.

Note the client ID (here `stroom`), the client secret and the IdP's discovery URL. For a Keycloak realm named
`stroom`, the discovery URL is `https://keycloak.example.com/realms/stroom/.well-known/openid-configuration`.

## 3. Create the Secrets

```shell
# The OpenID client secret
kubectl -n stroom create secret generic stroom-openid \
  --from-literal=client-secret='<client secret>'

# TLS certificate, key and CA, used by the Ingress and by the Stroom nodes
kubectl -n stroom create secret generic stroom-tls --type=kubernetes.io/tls \
  --from-file=tls.crt=stroom.crt --from-file=tls.key=stroom.key --from-file=ca.crt=ca.crt

# Password for the PKCS#12 keystore the operator generates from stroom-tls
kubectl -n stroom create secret generic stroom-keystore \
  --from-literal=password="$(openssl rand -base64 24)"
```

## 4. Create the Stroom configuration

The configuration bundled with the operator uses Stroom's internal IdP, so supply your own based on it. Download
the bundled file with the HTTPS connector:

```shell
curl -sSfLo config.yml \
  https://raw.githubusercontent.com/gradata-systems/stroom-k8s-operator/master/internal/controller/static_content/stroomcluster-config-https.yaml
```

Edit the `appConfig.security.authentication.openId` section so that it reads:

```yaml
      openId:
        identityProviderType: EXTERNAL_IDP
        openIdConfigurationEndpoint: "https://keycloak.example.com/realms/stroom/.well-known/openid-configuration"
        clientId: ${STROOM_OPERATOR_OPENID_CLIENT_ID}
        clientSecret: ${STROOM_OPERATOR_OPENID_CLIENT_SECRET}
```

The operator sets `STROOM_OPERATOR_OPENID_CLIENT_ID` and `STROOM_OPERATOR_OPENID_CLIENT_SECRET` in each node from
`spec.openId`, so the secret doesn't appear in the ConfigMap. Then create the ConfigMap:

```shell
kubectl -n stroom create configmap stroom-config --from-file=config.yml
```

See [Stroom configuration](../configuration/stroom-config.md) and
[Identity provider](../configuration/identity-provider.md) for more.

## 5. Create the database server

```yaml title="database-server.yaml"
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
    - max_connections=500
  resources:
    requests:
      cpu: 500m
      memory: 2Gi
    limits:
      memory: 2Gi
  volumeClaim:
    accessModes:
      - ReadWriteOnce
    resources:
      requests:
        storage: 20Gi
```

```shell
kubectl apply -f database-server.yaml
kubectl -n stroom get databaseserver dev
kubectl -n stroom get pods -l app.kubernetes.io/instance=dev,app.kubernetes.io/component=database-server -w
```

The `State` column shows `Deployed` once the operator has created the database objects. Wait for the Pod
`stroom-dev-db-0` to be `Running`, and check its log (`kubectl -n stroom logs stroom-dev-db-0`) to confirm MySQL
has finished initialising. A fuller example is in
[`samples/operator-database/database-server.yaml`](https://github.com/gradata-systems/stroom-k8s-operator/blob/master/samples/operator-database/database-server.yaml).

## 6. Create the Stroom cluster

```yaml title="stroom-cluster.yaml"
apiVersion: stroom.gchq.github.io/v1
kind: StroomCluster
metadata:
  name: dev
  namespace: stroom
spec:
  image:
    repository: gchq/stroom
    tag: v7.13.2
  databaseServerRef:
    serverRef:
      name: dev
  configMapRef:
    name: stroom-config
    itemName: config.yml
  openId:
    clientId: stroom
    clientSecret:
      secretName: stroom-openid
      key: client-secret
  https:
    tlsSecretName: stroom-tls
    tlsKeystorePasswordSecret:
      secretName: stroom-keystore
      key: password
  ingress:
    hostName: stroom.example.com
    secretName: stroom-tls
    className: nginx
  nodeTerminationPeriodSecs: 600
  nodeSets:
    - name: data
      count: 2
      role: Processing
      localDataVolumeClaim:
        accessModes:
          - ReadWriteOnce
        resources:
          requests:
            storage: 20Gi
      resources:
        requests:
          cpu: "1"
          memory: 4Gi
        limits:
          cpu: "2"
          memory: 4Gi
      memoryOptions:
        initialPercentage: 50
        maxPercentage: 70
    - name: ui
      count: 1
      role: Frontend
      localDataVolumeClaim:
        accessModes:
          - ReadWriteOnce
        resources:
          requests:
            storage: 10Gi
      resources:
        requests:
          cpu: 500m
          memory: 4Gi
        limits:
          cpu: "2"
          memory: 4Gi
      memoryOptions:
        initialPercentage: 50
        maxPercentage: 70
```

```shell
kubectl apply -f stroom-cluster.yaml
kubectl -n stroom get pods -l stroom/cluster=dev -w
```

The first node to start creates the database schema, which can take several minutes. Other nodes wait for it.
Each Pod becomes `Ready` once Stroom has started and the startup script has enabled the node. If Pods restart or
stay un-ready, see [Troubleshooting](../operations/troubleshooting.md).

!!! warning "Stream data needs shared storage"
    This minimal example doesn't mount shared storage for Stroom's data and index volumes. Before you send real
    data, mount a shared volume into every node and point Stroom's volumes at it. See
    [Storage](../configuration/storage.md#shared-storage-for-stroom-volumes).

## 7. Create the first administrator

With an external IdP, Stroom has no built-in `admin` account. Sign in once through the IdP, then grant your user
the `Administrator` permission with Stroom's `manage_users` command, run inside a node container:

```shell
kubectl -n stroom exec -it stroom-dev-node-ui-0 -c stroom-node -- \
  java -jar /stroom/stroom-app-all.jar manage_users \
    --createUser '<your subject ID>' \
    --createGroup Administrators \
    --addToGroup '<your subject ID>' Administrators \
    --grantPermission Administrators Administrator \
    /stroom/config/config.yml
```

`<your subject ID>` is the value of the claim Stroom uses to identify users (`sub` by default) for your account at
the IdP. Permissions are cached, so restart the node if they don't take effect:
`kubectl -n stroom delete pod stroom-dev-node-ui-0`. See
[Creating the First Administrator](https://gchq.github.io/stroom-docs/docs/install-guide/setup/create-first-admin/)
in the Stroom documentation for details.

!!! note
    Stroom's documentation recommends running `manage_users` while Stroom isn't running. Running it in a live
    node's container, as above, works for this one-off task, but it starts a second JVM inside the container's
    memory limit.

## 8. Sign in

Browse to `https://stroom.example.com/`. You'll be redirected to the IdP to sign in.

## Next steps

- [Configure shared storage](../configuration/storage.md) for stream data.
- [Back up the database](../database/backups.md) and the generated `stroom-dev-db` Secret.
- [Enable task autoscaling](../autoscaling.md) for the processing nodes.
- [Ship Stroom's own logs](../configuration/log-shipping.md) into Stroom.

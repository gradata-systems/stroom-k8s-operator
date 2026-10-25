# Identity provider (OpenID)

Stroom authenticates users with OpenID Connect. With the operator, Stroom must use an **external** identity
provider (IdP), such as Keycloak, Microsoft Entra ID or Amazon Cognito.

## Why an external IdP is required

The operator's node lifecycle scripts call the Stroom API on the node they run in: to enable the node and its jobs
when it starts, and to disable them and wait for its tasks to finish before it stops. To authenticate, they ask
Stroom for a token with the client credentials in `spec.openId`, using Stroom's
`/api/authproxy/v1/fetchClientCredsToken` endpoint, which forwards a client credentials grant to the IdP.

In Stroom 7.13 that endpoint only works when Stroom is configured with `identityProviderType: EXTERNAL_IDP`. With
Stroom's internal IdP (`INTERNAL_IDP`) the request is rejected, the startup script can't enable the node, the
startup probe never succeeds, and Kubernetes restarts the node in a loop.

!!! warning "Node start and drain depend on the IdP"
    Because the lifecycle scripts need a token:

    - A node that starts or restarts while the IdP is unreachable can't pass its startup probe, and is restarted
      when the startup window runs out.
    - A node that stops while the IdP is unreachable fails its `preStop` hook straight away, so it is **not
      drained**: tasks still running on it fail.

    Nodes that are already running are not affected. By default the liveness probe checks only for JVM deadlocks
    and the readiness probe ignores the IdP health check, so an IdP outage doesn't restart nodes or stop them
    receiving data. See [Health probes](health-probes.md).

## The `spec.openId` setting

```yaml
spec:
  openId:
    clientId: stroom
    clientSecret:
      secretName: stroom-openid
      key: client-secret
```

The operator passes these to every node as the environment variables `STROOM_OPERATOR_OPENID_CLIENT_ID` and
`STROOM_OPERATOR_OPENID_CLIENT_SECRET`. The lifecycle scripts use them to request tokens, and the bundled Stroom
configuration also uses them as Stroom's own OpenID client ID and secret. Using the same client for both is the
simplest arrangement.

The client needs:

- **Client authentication** (a confidential client with a secret).
- **The client credentials grant**. In Keycloak, turn on _Service accounts roles_ for the client.
- **The authorisation code flow**, with the redirect URI `https://<ingress host>/api/auth/flow/v1/signin-oidc`,
  for interactive sign-in.
- **An audience** that Stroom accepts. Stroom checks the `aud` claim of access tokens against `clientId` (or
  `allowedAudiences`). Some IdPs, Keycloak included, need a mapper to put the client ID in the access token's
  `aud` claim.

The identity the token represents must be allowed to manage nodes and jobs and to list tasks in Stroom. If the
scripts get `401` or `403` responses, see
[Troubleshooting](../operations/troubleshooting.md#authentication-failures-in-apilog).

## Configuring Stroom for the IdP

The bundled configuration sets `identityProviderType` from the `IDENTITY_PROVIDER_TYPE` environment variable,
defaulting to `INTERNAL_IDP`, and doesn't set an IdP discovery endpoint. To use an external IdP, provide your own
configuration through [`spec.configMapRef`](stroom-config.md) with at least:

```yaml
appConfig:
  security:
    authentication:
      openId:
        identityProviderType: EXTERNAL_IDP
        openIdConfigurationEndpoint: "https://keycloak.example.com/realms/stroom/.well-known/openid-configuration"
        clientId: ${STROOM_OPERATOR_OPENID_CLIENT_ID}
        clientSecret: ${STROOM_OPERATOR_OPENID_CLIENT_SECRET}
```

Stroom refuses to start with `EXTERNAL_IDP` and no `openIdConfigurationEndpoint`. For the other settings (logout
endpoint, scopes, audience and claim settings) and provider-specific instructions, see the Stroom documentation:

- [Stroom configuration for an external IdP](https://gchq.github.io/stroom-docs/docs/install-guide/setup/open-id/external-idp/stroom-configuration/)
- [KeyCloak](https://gchq.github.io/stroom-docs/docs/install-guide/setup/open-id/external-idp/keycloak/)
- [Creating the First Administrator](https://gchq.github.io/stroom-docs/docs/install-guide/setup/create-first-admin/),
  which you need to do because Stroom has no built-in administrator when using an external IdP. See also the
  [quick start](../getting-started/quick-start.md#7-create-the-first-administrator).

The [`external-idp`](https://github.com/gradata-systems/stroom-k8s-operator/tree/master/samples/external-idp) sample
is a complete configuration for Keycloak, with the realm's clients described in the
[samples README](https://github.com/gradata-systems/stroom-k8s-operator/tree/master/samples#keycloak).

## How tokens are handled

The scripts cache the token in `/stroom/auth/token`, a Pod-local volume, and request a new one shortly before it
expires. Failed token requests and API calls are logged, with the HTTP status and response, to
`/stroom/logs/k8s/api.log` in the node.

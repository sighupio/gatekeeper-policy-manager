# Authentication

GPM is unauthenticated by default. There are two ways to protect it:

- `GPM_AUTH_ENABLED=OIDC`. GPM runs the login flow itself against an OpenID Connect provider.
- `GPM_AUTH_ENABLED=JWT`. An authenticating proxy in front of GPM identifies the user. See
  [Behind an authenticating proxy](#behind-an-authenticating-proxy).

| Env Var Name                      | Description                                                                                                                                              | Default                |
| --------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------- |
| `GPM_AUTH_ENABLED`                | Set to `OIDC` or to `JWT` to protect GPM. Any other value leaves it open.                                                            | `Anonymous`            |
| `GPM_SECRET_KEY`                  | Key used to sign and encrypt the session cookie. **Required when authentication is on**: GPM refuses to start if it is still the 1.x default, which is published in this repository, so anyone can forge a session. | `g8k1p3rp0l1c7m4n4g3r` (the 1.x default) |
| `GPM_PREFERRED_URL_SCHEME`        | Set to `https` when GPM is served over TLS, so the session cookie is marked `Secure`. A `GPM_OIDC_REDIRECT_DOMAIN` that starts with `https://` also marks it `Secure`. | `http`                 |
| `GPM_SESSION_MAX_AGE`             | How long a session lasts, in seconds.                                                                                                                    | `28800` (8 hours)      |
| `GPM_OIDC_REDIRECT_DOMAIN`        | The public address of GPM, for example `https://gpm.example.com`. The provider sends users back to `<domain>/oidc-auth`. Required.                       |                        |
| `GPM_OIDC_CLIENT_ID`              | Client ID registered with the provider. Required.                                                                                                        |                        |
| `GPM_OIDC_CLIENT_SECRET`          | Client secret, if the client is confidential.                                                                                                            |                        |
| `GPM_OIDC_SCOPES`                 | Extra scopes for the login request, separated by spaces or commas. `openid`, `profile` and `email` are always requested. Add the scope that carries group membership when your provider keeps it behind one. |                        |
| `GPM_OIDC_ISSUER`                 | Issuer URL. GPM reads the rest of the provider's configuration from it, unless the endpoints below are set.                                              |                        |
| `GPM_OIDC_AUTHORIZATION_ENDPOINT` | Authorization endpoint. Setting any endpoint below turns discovery off, so set them all together.                                                        |                        |
| `GPM_OIDC_TOKEN_ENDPOINT`         | Token endpoint. See the note above.                                                                                                                      |                        |
| `GPM_OIDC_JWKS_URI`               | JWKS URI. See the note above.                                                                                                                            |                        |
| `GPM_OIDC_END_SESSION_ENDPOINT`   | End session endpoint. Discovered automatically when the provider advertises one. If GPM has one, a logout from GPM also ends your session at the provider.   |                        |
| `GPM_OIDC_INTROSPECTION_ENDPOINT` | Accepted for compatibility with GPM 1.x. Not used.                                                                                                       |                        |
| `GPM_OIDC_USERINFO_ENDPOINT`      | Accepted for compatibility with GPM 1.x. Not used.                                                                                                       |                        |

> [!IMPORTANT]
> Register `<GPM_OIDC_REDIRECT_DOMAIN>/oidc-auth` as a valid redirect URI with your provider, and
> `<GPM_OIDC_REDIRECT_DOMAIN>/logout` as a valid post logout redirect URI.
>
> Set `GPM_SECRET_KEY` to a long random string. GPM will not start with the old default.
>
> Set `GPM_PREFERRED_URL_SCHEME=https` whenever GPM is reachable over HTTPS.

GPM uses PKCE, so the authorization code cannot be used by anyone who intercepts it.

The session is a cookie. GPM signs the cookie and also encrypts it, so the contents are not
readable. GPM derives the two keys for this from `GPM_SECRET_KEY` with HKDF. Every replica derives
the same keys from the same secret, and so does the same replica after a restart.

> [!NOTE]
> GPM does not keep sessions on the server. A logout clears the cookie in your browser. When the
> provider supports it, GPM also ends the session at the provider. But GPM cannot cancel a copy of
> the cookie that someone took to a different machine. Such a copy stays valid until
> `GPM_SESSION_MAX_AGE` expires it. If this risk is a problem for you, use a short value.
>
> On a subpath deployment the cookie is scoped to that subpath. A different application on
> the same host does not receive it.

Once authentication is on, everything requires a session except these paths, which have to stay
reachable for a user who is not logged in yet:

| Path | Why it is open |
| --- | --- |
| `/health` | the liveness and readiness probes run without credentials |
| `/login`, `/oidc-auth`, `/logout` | the login flow itself |
| `/metrics` | Prometheus scrapes it. It holds request counters only, no policy data |
| `/static/*`, `/favicon.ico` | the assets the login and logout pages need |

Everything else — every page, including the list of clusters — needs a valid session.

When the session expires, GPM sends the user to `/login` to sign in again. The login route accepts
`?next=` with a same-site path that says where the user lands after signing in.

## Behind an authenticating proxy

Set `GPM_AUTH_ENABLED=JWT` when a proxy such as Pomerium already identifies your users. The proxy
signs an assertion for each request, and GPM verifies that signature against the proxy's keys.

| Env Var Name | Description | Default |
| --- | --- | --- |
| `GPM_JWT_JWK_SET_URL` | The proxy's JWKS. For Pomerium it is `https://<pomerium-host>/.well-known/pomerium/jwks.json`. Required. | |
| `GPM_JWT_AUDIENCE` | The `aud` claim GPM accepts. Set it to the host GPM is served on, for example `gpm.example.com`. Required, and see the warning below. | |
| `GPM_JWT_HEADER_NAME` | The header that carries the assertion. Set it to `Authorization` for oauth2-proxy with `--set-authorization-header`. A `Bearer ` prefix is removed. | `X-Pomerium-Jwt-Assertion` |
| `GPM_JWT_ISSUER` | The `iss` claim GPM accepts. Not checked when this is empty. Set it when the keys belong to an identity provider. See the note below. | |
| `GPM_JWT_LOGOUT_URL` | Where the Log out button points, for example `https://gpm.example.com/.pomerium/sign_out`. GPM shows no Log out button when this is empty. | |

GPM verifies the signature, the audience, the issuer when you set one, and the expiry time. A
request with no assertion, or with one that does not pass, gets a 401 and no data. GPM accepts
RS256 and ES256. Pomerium signs with ES256.

This mode has no login page and no session cookie, so GPM does not read `GPM_SECRET_KEY`. GPM reads
the assertion again on every request, which is also why a change to a user's groups takes effect
immediately.

> [!WARNING]
> `GPM_JWT_AUDIENCE` is required, and GPM does not start without it.
>
> A proxy signs every route it serves with the same key, and it writes the route into the `aud`
> claim. Without this check, GPM accepts an assertion that the proxy made for a different route. A
> user who is allowed on that other route can then read GPM.
>
> For Pomerium the value is the bare host, with no scheme and no trailing slash.

> [!NOTE]
> Set `GPM_JWT_ISSUER` when `GPM_JWT_JWK_SET_URL` points at an identity provider and not at the
> proxy. With Pomerium, one key set serves the routes of one proxy, and `GPM_JWT_AUDIENCE` already
> names this route, so the issuer adds little. With a shared identity provider, one key set and one
> client ID cover many tenants, and the issuer is the claim that separates them.
>
> GPM writes a warning at start when `GPM_JWT_ISSUER` is empty.

> [!NOTE]
> GPM fetches the keys from `GPM_JWT_JWK_SET_URL` over HTTPS. A plaintext `http://` URL is refused:
> these keys decide who every user is. If your proxy uses a certificate from
> a private CA, mount that CA into the GPM Pod. GPM has no setting to skip this check.

Authentication is not authorization. The proxy decides who reaches GPM. To also limit what each
person sees, turn on [RBAC-aligned views](rbac-aligned-views.md). That feature works the same way in
both modes.

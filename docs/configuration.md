# Configuration

GPM is a stateless application. You can configure it with environment variables. The possible configurations are:

| Env Var Name         | Description                                                                                                                                                                                                                       | Default              |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------- |
| `GPM_LISTEN_ADDRESS` | Server listen address                                                                                                                                                                                                             | `:8080`              |
| `GPM_LOG_LEVEL`      | Log level (`DEBUG`, `INFO`, `WARN`, `ERROR`)                                                                                                                                                                                     | `INFO`               |
| `GPM_EVENTS_SOURCE`  | Comma-separated event source components to show. Gatekeeper tags admission events with `gatekeeper-webhook` and audit events with `gatekeeper-audit`.                                                                              | `gatekeeper-webhook,gatekeeper-audit` |
| `GPM_SKIP_TLS_VERIFY` | Skip TLS certificate verification while connecting to the Kubernetes API Server. Needed on clusters whose CA certificate is missing the AKI/SKI extensions, as happens on EKS. **USE WITH CAUTION.**                            | `false`              |
| `GPM_EVENTS_NAMESPACE` | Read events from this namespace only. Empty means every namespace, which needs a cluster-wide read on `events`. See [Events and RBAC](#events-and-rbac). | `` (every namespace) |
| `GPM_BASE_PATH` | The subpath for GPM, for example `/gpm`. The image sets this value from the `PUBLIC_URL` build argument. See [Running behind a reverse proxy on a subpath](#running-behind-a-reverse-proxy-on-a-subpath). | `` (the domain root) |
| `KUBECONFIG`         | Path to a [kubeconfig](https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/) file, if provided while running inside a cluster this configuration file will be used instead of the cluster's API. | `$HOME/.kube/config` |

For authentication, see [Authentication](authentication.md). For per-user views, see [RBAC-aligned views](rbac-aligned-views.md). For more than one cluster, see [Multi-cluster support](multi-cluster.md).

## Running behind a reverse proxy on a subpath

GPM assumes by default that it is served from the domain root. If you put it behind a reverse proxy
on a subpath, for example `example.com/gpm`, set the `GPM_BASE_PATH` environment variable to that
subpath. GPM prepends it to the paths it hands the browser: the asset URLs, the login URL and the
OIDC redirects.

You can also set the subpath at build time with the `PUBLIC_URL` build argument, which sets
`GPM_BASE_PATH` in the image:

```bash
docker build --build-arg PUBLIC_URL=/gpm -t gatekeeper-policy-manager:subpath .
```

If you set neither, GPM runs at the domain root, as before.

> [!IMPORTANT]
> Configure your reverse proxy to **strip the subpath** before forwarding to GPM. GPM matches its
> routes at the root, so it expects to receive `/constraints` and `/static/...`, not
> `/gpm/constraints`. With nginx, a `proxy_pass` ending in a slash does this for you.

The image published on quay.io is built for the root path. If you need a subpath deployment, build
your own image with the argument above and push it to your own registry, or reference the
Dockerfile from your CI pipeline with the same `--build-arg`.

If you enable OIDC, keep `GPM_OIDC_REDIRECT_DOMAIN` as the scheme and host only, for example
`https://example.com`. GPM adds the subpath. Register `https://example.com/gpm/oidc-auth` with your
provider as the redirect URI.

## Events and RBAC

The events view reads the `events` resource of the Kubernetes core API. By default GPM reads events
from every namespace. This needs a `ClusterRole` with read access to `events` in the whole cluster,
which is more access than the other views need.

To make the access smaller, set `GPM_EVENTS_NAMESPACE` to the namespace that OPA Gatekeeper runs
in, usually `gatekeeper-system`. Then GPM reads events from that namespace only. You can move the
read on `events` out of the `ClusterRole` and into a `Role` in that namespace.

The Helm chart does both steps for you. Set `config.eventsNamespace` and the chart removes `events`
from the `ClusterRole`. The chart then creates a `Role` and a `RoleBinding` in the namespace that
you named.

> [!NOTE]
> `GPM_EVENTS_NAMESPACE` has priority over the `?namespace=` parameter of the events endpoint. A
> request cannot read a namespace that the deployment is not configured for.

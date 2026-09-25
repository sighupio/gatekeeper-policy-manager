# RBAC-aligned views

By default every person who reaches GPM sees the whole cluster. Set `GPM_RBAC_FILTERING=true` and
GPM shows each person only what their own Kubernetes account can read.

GPM asks the Kubernetes API server, with a `SubjectAccessReview`, whether the logged-in person can
list the data behind a view. A view whose answer is no does not appear in the navigation, and a
request for it gets a 403. A request for the dashboard sends the reader to the Resources view
instead, because a browser lands there by default. The Resources view is always available, and it lists only the objects
that the person can read. Someone with access to one namespace sees that namespace.

GPM does not act as the user. It reads the cluster with its own ServiceAccount and asks about the
user separately, so it needs `create subjectaccessreviews` and not the `impersonate` permission. The
Helm chart adds this rule when you set `config.rbacFiltering.enabled`.

The feature narrows what a person sees through GPM. It does not narrow what GPM reads. The
ServiceAccount keeps its cluster-wide read on the Gatekeeper objects, and a person who can run code
in the pod reads all of them.

This feature needs three things, and GPM refuses to start without them:

- **Authentication.** Without an identity there is nobody to ask about. `GPM_AUTH_ENABLED` must be
  `OIDC` or `JWT`. Both modes work the same way here: GPM reads the same claims, applies the same
  prefixes, and sends the same reviews.
- **One cluster.** One identity cannot be authorized against several clusters, so GPM refuses to
  start when the kubeconfig names more than one context.
- **A named username claim.** `GPM_RBAC_USERNAME_CLAIM` must name the claim the API server reads.
  Without it the reviews carry whichever claim the token happens to hold, and two people can be
  authorized as one identity.

The name that GPM sends must be the name that the API server knows. Many clusters add a prefix with
`--oidc-username-prefix`, and some read the username from a different claim:

| Variable | Purpose |
| --- | --- |
| `GPM_RBAC_USERNAME_CLAIM` | **Required.** The claim that holds the username the API server knows. Use the claim your API server reads in `--oidc-username-claim`. |
| `GPM_RBAC_USERNAME_PREFIX` | The prefix that `--oidc-username-prefix` adds, for example `oidc:`. |
| `GPM_RBAC_GROUPS_CLAIM` | The claim that lists the groups. The default is `groups`. |
| `GPM_RBAC_GROUPS_PREFIX` | The prefix that `--oidc-groups-prefix` adds. |

> [!NOTE]
> A wrong name denies everything, and the page is empty. To find the name that GPM used, put the
> pointer on the "scoped to your access" label next to the page title. Compare that name with the
> subject of your `RoleBinding`. GPM writes the same name to its log.

> [!IMPORTANT]
> Your API server must authenticate the same users. GPM asks it about a person by name, and an API
> server that does not know that name denies it, exactly as it denies a person with no access. The
> page then says that the person can read nothing, which hides the real cause.
>
> Before you turn this on, confirm that your API server runs with `--oidc-issuer-url`,
> `--oidc-username-claim` and the matching prefixes. GPM cannot check this for you.

In `JWT` mode GPM reads these claims from the proxy's assertion, on every request, instead of from
an ID token at login. A change to a person's groups therefore takes effect at once. The claims must
still be the ones the API server reads, and the proxy must put them in the assertion. For Pomerium,
`email` and `groups` are both present.

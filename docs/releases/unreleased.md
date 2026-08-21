# Gatekeeper Policy Manager release vNEXT

Welcome to the release of Gatekeeper Policy Manager `vNEXT`, maintained with ❤️ by the team [SIGHUP by ReeVo](https://sighup.io/).

This version adds a Resources view, which shows the audit from the side of the objects that break policies. The other views work the same as before.

## New features 🌟

- **A new Resources view answers "what of mine is broken?".** The Constraint Templates and Constraints views describe the policies. This view describes the objects: one card per namespace, one row per object, and one line for each policy that the object breaks. The data is the same audit data, so the view needs no new permissions.
- **Each row counts the violations by enforcement action.** A row has three columns: deny, dry-run and warn. The rows come in order of the blocking violations first, so the objects that stop a deployment are at the top of the card. The namespace cards use the same order.
- **The namespace list shows where the trouble is.** Each namespace in the sidebar carries a bar with the mix of enforcement actions and the total count. The page hides this sidebar when the cluster has violations in one namespace only.
- **A filter narrows the page to one resource, kind or policy.** The filter hides the rows that do not match, and it hides a namespace card when all of its rows are hidden.
- **You can share a link to a single resource.** Each row has a copy button, like the violation rows in the Constraints view. The link opens the page, expands that row and marks it. The link is readable, for example `#ns-apps-prod--Deployment--checkout-api`, so a reader can see the object before a click.
- **GPM can show each person only what their Kubernetes account can read.** Set `GPM_RBAC_FILTERING=true`, or `config.rbacFiltering.enabled` in the Helm chart. GPM then asks the API server what the logged-in person is allowed to list, hides the views that answer no, and lists in the Resources view only the objects that the person can read. GPM reads the cluster with its own ServiceAccount and never acts as the user: it needs `create subjectaccessreviews`, not `impersonate`. The feature is off by default. It needs OIDC authentication and a single cluster, and GPM refuses to start with it on and either one missing. Read "RBAC-aligned views" in the README before you turn it on: the username GPM sends must match the one your API server uses, which often carries a prefix.

## Other changes

- **The navigation shows `Templates` for the Constraint Templates view.** The page title is still "Constraint Templates". The short label gives the new `Resources` entry the space that it needs.
- **You can ask the identity provider for more scopes.** Set `GPM_OIDC_SCOPES`, or `config.oidc.scopes` in the Helm chart, to a list separated by spaces or commas. GPM always asks for `openid`, `profile` and `email`, and adds your list to them. Some providers send group membership only when the client asks for a scope by name. The RBAC-aligned views need that claim.
- **The pages that need no session no longer show the navigation.** The signed-out page, the "not found" page and the error pages are open to a visitor with no session. Their menu offered links that only send the visitor to the login page. The signed-out page also had a "Log out" button, which had nothing left to do.
- **The image carries a current set of root certificates.** GPM now uses the newest distroless base, with 150 root certificates in place of 129. The previous base was missing the newer roots, for example ISRG Root X2. GPM needs a current set of roots when it connects to an OIDC provider.
- **An SVG image in a description no longer renders.** The `description` annotation accepts markdown from this project's previous release. An SVG can carry a script, so GPM now drops an image with a `data:image/svg+xml` source. A PNG or JPEG data image still renders, and a normal image link still works.

## Upgrade procedure

This release needs no action. Update the image tag, then apply the manifests or upgrade the Helm release as usual.


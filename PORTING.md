# Porting `main` → `feature/go-backend`

Tracking document for bringing the Go-backend branch back in sync with `main` (the Python
implementation).

- **Divergence point:** `1d46f15` — 17 Mar 2023
- **Commits on `main` since:** 1245 (≈600 dependabot)
- **Commits on `feature/go-backend` since:** 117
- **Last backport wave:** July 2025. Only CI/e2e touch-ups since (Jan 2026).
- **Baseline compared:** `main` @ `8ff4a56` — the **v1.1.2** release (4 Aug 2026)

Re-diffed after v1.1.2 shipped. Everything in that release is either a Python dependency bump or
version bookkeeping; **nothing new to port**. The chart moved only its `version`, `appVersion` and
image tag, with no template changes, so section 2 is unaffected. `44f55eb`
(`useradd --create-home`, so gunicorn can write `~/.gunicorn`) is Python-specific and does not apply
to the distroless image. `71d71be` is the `sleep 5` discussed under the port-forward item in
section 3.

To re-run the comparison:

```bash
git fetch origin main && git diff HEAD:<path> origin/main:<path>
```

> Note the layout difference: the frontend lives in `web-client/` on this branch and in
> `app/web-client/` on `main`.

## Status legend

- [ ] pending &nbsp;·&nbsp; 🔴 blocking / broken &nbsp;·&nbsp; 🟠 important &nbsp;·&nbsp; 🟡 nice to
  have &nbsp;·&nbsp; 🟢 cosmetic

> Items tagged `[1.x-background]` concern the `release-1.x` (Python 1.x) line only. Leave them until
> a 1.x security-patch release actually happens; exclude them from 2.0 / `main` planning. Filter with
> `grep -L` / `grep -v '1.x-background'`.

---

## 0. Root cause of the dependency drift

Dependabot reads `.github/dependabot.yml` **only from the default branch**, which is `main`. The
copy on `feature/go-backend` has therefore never been read — it is inert. On top of that it points
npm at `directory: "/app/web-client"`, while the frontend moved to `/web-client` in `82d8371`
(Apr 2023). Net effect: the frontend on this branch has **never** received a Dependabot update, and
every JS bump so far landed as a manual backport.

The fix has to happen on `main`:

- [x] 🔴 **On `main`** — done.
- [x] 🟡 **On this branch** — correct the npm path to `/web-client` anyway.
- [x] 🟠 ~~**At the 2.0 switchover, rewrite `.github/dependabot.yml` deliberately.**~~ — no rewrite
      needed. `feature/go-backend` already carries the correct config: `gomod` at `/` and `npm` at
      `/web-client`, with no `pip`/`/app` entries and no `target-branch` indirection. The `-s ours`
      merge keeps this tree, so `main` inherits it. (Frontend e2e Dependabot coverage is a separate
      open item.)
- [x] 🟠 **Dependabot covers Go modules too — confirmed.** GitHub's **Insights → Dependency graph → Dependabot** tab lists all four manifests: `app/constraints.txt` and `app….
- [x] 🟡 ~~**`tests/e2e/package.json` is covered by Dependabot on neither branch.**~~ — added an
      `npm` entry at `/tests/e2e` in `.github/dependabot.yml`, with a comment that the Playwright
      version must match the two `mcr.microsoft.com/playwright` image tags in `.drone.yml`, so a bump
      PR needs a manual follow-up to update those tags in the same change.

- [x] 🟠 **Frontend dependency sync** — done against `main` @ `ca5b7a8` (after the Dependabot JS PRs were merged there).

> One deliberate nit: `@types/react-dom` went from this branch's `^17.0.20` back to `main`'s
> `^17.0.2`. Both resolve to the same installed 17.x, and matching `main` exactly keeps future syncs
> a trivial copy. Flip it back if the tighter floor was intentional.

> The three `react-hooks/exhaustive-deps` warnings in `yarn build` (`Error`, `Events`, `Mutations`)
> are **pre-existing, not from this bump** — `react-scripts` stays pinned at 5.0.1, so the bundled
> eslint ruleset is unchanged, and two of the three files do not exist on `main` at all.

The actual `package.json` gap is small once that is fixed:

| Package | this branch | `main` |
| --- | --- | --- |
| `@testing-library/jest-dom` | ^6.6.3 | ^6.9.1 |
| `@types/node` | ^24.0.3 | ^25.9.3 |
| `react-json-tree` | ^0.19.0 | ^0.20.0 |
| `react-router-dom` | ^6.28.2 | ^6.30.3 |
| `sass` | ^1.83.4 | ^1.99.0 |
| `typescript` | ^5.6.3 | ^5.9.3 |
| `web-vitals` | ^4.2.4 | ^5.2.0 |

---

## 1. Functional features

- [x] 🟠 **`GPM_SKIP_TLS_VERIFY`** — PR [#1454](https://github.com/sighupio/gatekeeper-policy-manager/pull/1454), released in v1.1.2.

- [x] 🟠 **`PUBLIC_URL` / serving from a subpath** — PR [#1463](https://github.com/sighupio/gatekeeper-policy-manager/pull/1463), 3 Aug 2026.

### Pre-existing parity gap (not a port, but it gates part of the backlog)

- [x] 🔴 **OIDC authentication — implemented.** The largest gap in the port, now closed.

> **The logout redirect loop, and why the tests missed it.** Real Keycloak found a bug the fake
> provider could not. `/logout` both starts provider logout *and* is where the provider sends the
> browser back, via `post_logout_redirect_uri`. It redirected unconditionally, so the return hop
> started the flow again — `ERR_TOO_MANY_REDIRECTS`.
>
> `/logout` is now idempotent: it only hands over to the provider when there was a session to end,
> so the return hop finds none and renders the logout page.
>
> The original test **encoded the bug**: it asserted `post_logout_redirect_uri` pointed at
> `/logout` and passed, because it only ever checked the first hop. It is now
> `TestLogoutRoundTripDoesNotLoop`, which logs in, logs out, and follows the redirect back the way
> a browser does. The replacement was checked by reintroducing the bug and confirming it fails —
> a regression test nobody has seen fail is not yet a regression test. Any future single-hop
> redirect assertion deserves the same suspicion.

- [x] 🟠 **Two ideas adopted from the earlier `feature/go-backend-oidc` WIP branch.** That branch reached for the same libraries independently, which was a useful check on the approach, and had two things this implementation lacked: - **A `/login` route.** The 401 answer needs somewhere to send people; "reload the page" was a poor substitute. It takes an optional `?next=` (same-site paths only) and short-circuits to that target when a session already exists. - **Unwrapping `oauth2.RetrieveError`.** `oauth2` flattens a provider's rejection to "cannot fetch token: 401 Unauthorized", while the body underneath names the actual problem — unknown client, bad secret, mismatched redirect URI.

- [x] 🟠 **A "Log in" button on the error page.** Checked in a browser rather than argued about: the frontend already handles a 401 well — it parses the `ErrorAnswer` body and renders a proper error page — but it does not act on it, so an expired session dead-ended….

- [x] 🟡 ~~**Nothing ever populated `ErrorPageState.entity`.**~~ — fixed. All five pages now pass
      `entity: pathname` (the react-router path of the page that failed) in the `/error` navigation
      state. The Error page's Go-back button returns to that page via `appPath(entity || "/")`, and
      the login button appends `?next=<appPath(entity)>` (the backend `/login` already honors it).
      so do both at once.

- [x] 🔴 **Pre-commit security review, and the six defects it found.** A fresh reviewer went over the uncommitted OIDC work before it landed. It was worth it: two of the findings were full authentication bypasses, and one would have turned red in CI on the first push. Every fix has a regression test that was **confirmed to fail against the old code** before being trusted. - **Path traversal, reachable with no session** — `serveIndex` joined the raw `RequestURI`, query string included and unnormalised, into a filesystem path.

### Second review, of the fixes themselves

The fixes above were new, unreviewed code written straight after finding out the first pass had
holes, so a second reviewer went over just that diff. **No critical findings** — the two bypasses
are genuinely closed. It found four things worth fixing and confirmed the rest by mutation testing.

- [x] 🔴 **The regression test for the session-expiry fix was worthless.** It mutated the store's own codecs before decoding, so it measured its own mutation rather than the code, and passed with `store.MaxAge(maxAge)` deleted. The header of `security_test.go` claimed every test in it had been verified against the broken code; that was untrue for this one. **Third time in this port** that a test which could not fail slipped through — the pattern is asserting on something the test itself set up.
- [x] 🟠 **`GPM_SESSION_MAX_AGE=0`, empty or unparseable disabled expiry entirely.** `viper.GetInt` yields `0` for all of them, and `securecookie.MaxAge(0)`….
- [x] 🟠 **PKCE and the manual-mode issuer requirement were shipping untested.** Mutation testing showed both could be deleted with the suite still green. The stub provider now verifies PKCE the way a real one does — S256 of the verifier must equal the recorded challenge — and there is a case for complete manual endpoints with no issuer.
- [x] 🟠 **`clientSecret` was the one chart value the quoting pass missed.** A secret containing YAML metacharacters broke templating outright, and a numeri….
- [x] 🟡 **`/metrics` host label was unbounded.** Keeping metrics on the main port outside auth is the chosen design, which makes `RequestCounterHostLabelMappingFunc` — the raw `Host` header — an attacker-controlled label in a process that never forgets series.
- [x] 🟢 `startLogin` discarded the session error and would nil-deref if the store were ever absent; `middleware.Recover()` was missing entirely, so any pan….

### OIDC follow-ups, deliberately not done now

Findings from the review that were consciously left, so they are decisions rather than oversights.

- [x] 🟠 ~~**Session cookies are signed but not encrypted**~~ — done, the readability half.

**Security review after the subpath and session work.** One MEDIUM finding, fixed: the session
cookie used `Path: "/"`, which was right while GPM owned its origin but wrong once subpath serving
worked — a subpath is normally chosen because something else answers on that host, so the browser
handed GPM's session to the neighbouring application on every request. Now scoped with
`cookiePath()`. Everything else checked came back clean: the `?namespace=` parameter cannot
traverse the Kubernetes API path (`Request.Namespace` runs `IsValidPathSegmentName`), the redirect
chain rejects every protocol-relative form, and the HKDF derivation is sound.

**Second review, of those fixes.** Found a HIGH the first review and I both missed: any session
cookie the store could not decode produced a 500 from `startLogin`, with no `Set-Cookie` on the
response and no recovery through `/login` or `/logout`. The 500 was pre-existing, but this change
*guaranteed* it fired — the key material and the cookie format both changed, so every cookie from
every earlier build became undecodable. Every logged-in user would have hit a 500 on every page for
up to 8 hours at upgrade. Reproduced, then fixed: a cookie that does not decode now means "not
logged in", and logout clears it instead of skipping it.

Also fixed from that review: the legacy `Path=/` cookie is now explicitly expired on a subpath
deployment (GPM cannot overwrite it — deletion has to match the path — so without this it shadowed
the scoped cookie and kept leaking to neighbouring apps until it expired); `basePath` normalizes
with `path.Clean`, so a `//` typo no longer silently restores origin-wide cookie scope and no longer
makes `browserPath("/login")` return the off-site `//login`; and three tests that could not fail
were rewritten.

Two of my own new tests were wrong and my own mutation pass caught them: the root-deployment test
logged in first, so `startLogin` never ran and it could not see the deletion firing where it must
not; and the callback test asserted `!strings.Contains(landed, "evil.com")`, which fails on
`/gpm/gpm//evil.com` — a same-origin path. Both now assert the real property.

**Third pass: the upgrade path exercised for real**, not in Go tests — an old binary and a new one,
a standalone fake OIDC provider, the stripping proxy, curl cookie jars and a real browser, at the
root and on a subpath, with the same secret and a rotated one. Result: clean. Every stale-cookie
request is a 302 into a fresh login or a 401 carrying `login_url`, always with a `Set-Cookie` that
replaces the bad state. The browser check also confirmed the leak actually closes: a neighbouring
app on the same origin received `gpm-session` before the fix and no cookie after.

Two low findings from it, both fixed: the API 401 branch did not expire the legacy root cookie
(so it kept reaching neighbouring apps until the user navigated), and the callback dead-ended on
raw JSON when the session would not decode — the same race as the `startLogin` bug, and it now
restarts the login.

The review also reported that a downgrade to, or a rolling update from, a pre-fix Go build strands
users on a 500. **Dropped, not fixed and not documented:** the Go backend was never released, so no
such build is in anyone's hands, and 1.x is Flask with a cookie named `session`, which does not
collide with `gpm-session`. The release-note caveat that had been written for it was removed again.
Breaking 1.x sessions is acceptable to the maintainer in any case.

**What the fixes actually rest on**, now that the migration framing is gone. Rotating
`GPM_SECRET_KEY` invalidates every cookie at once, so without the `startLogin` fix that one
operator action strands every user for a full `GPM_SESSION_MAX_AGE`, with no server-side repair.
That is permanent behaviour, not a one-off. Likewise the root-cookie deletion: a root-scoped cookie
appears whenever a deployment *moves* to a subpath — an operator setting `GPM_BASE_PATH` on a
running instance, or a `PUBLIC_URL` image replacing one without it. The comments and test names now
say this rather than talking about upgrades.

**Fourth pass: a conciseness review plus a second adversarial pass on the result.** The conciseness
pass (run by the maintainer from a terminal) suggested twelve reductions; eight were taken, and it
cost about 130 lines of comment and test duplication. Two were rejected as factually wrong — it
claimed every unauthenticated path reaches `startLogin` (the API-401 branch does not, which is why
that call site exists) and that dropping layer 2 of the redirect-guard test was safe (it is the
only mutation-sensitive check on `startLogin`'s own sanitization).

Accepting one of its suggestions introduced a bug that the adversarial pass then caught: collapsing
the cookie-peeling in `TestSessionCookieDoesNotRevealItsContents` to a single `strings.Split` made
the test **fail at random in about 12% of runs**, because securecookie's third field is a raw
HMAC-SHA-256 and one byte in eight is `|`. Reproduced at 26 failures in 200 runs, fixed with
`SplitN(..., 3)`, verified at 300/300 green and still catching a removed block key.

The adversarial pass also found the scoping fix **never fired for the users who most needed it**.
All three `clearLegacyRootCookie` call sites are on unauthenticated paths, but a root-scoped cookie
from before a base path was configured still decodes — the keys do not depend on the base path — so
those users are authenticated, take `return next(c)`, and keep leaking to every neighbouring app for
a full `GPM_SESSION_MAX_AGE`. `migrateSessionOffTheRoot` now re-saves the session onto the base path
and expires the root cookie, in that order, so the migration cannot log anyone out.

Two more from it, both fixed: two new assertions used a bare `len(cookies) != 0`, which the legacy
deletion cookie satisfies on a subpath, so they could not fail there; and the callback's login
restart had no loop breaker, which I had introduced — before it, the 401 terminated the bounce
between GPM and the provider. A one-hop marker cookie bounds it.

Also fixed: `GPM_BASE_PATH` containing a backslash produced an off-site redirect, since browsers
read `\` as a path delimiter. `safeRedirectTarget` already rejects a `/\` prefix; the base path
was the one place the rule was not applied. And `basepath.go` had shadowed the newly imported
`path` package with its own parameter names.

One claim in the first review was wrong and is corrected in the test comments: reversing the two calls
in `login()` would **not** be a live open redirect, because `startLogin` sanitizes again and
`browserPath` re-prefixes the base path. Found by mutating the production code — the mutation
passed, which is what exposed it.

- [ ] 🟠 **Logout cannot invalidate a stolen cookie.** Inherent to a stateless cookie store, and
      left as a deliberate tradeoff for the rc rather than an oversight. A server-side store is the
      only real fix, and it needs state: in-memory breaks the chart's `replicaCount > 1` and drops
      every session on restart, so a usable version means Redis or equivalent, a chart dependency,
      new configuration and a single-replica fallback. Documented instead — the README says a
      logout cannot cancel a copy taken elsewhere and points at `GPM_SESSION_MAX_AGE`, and the
      release notes say the same. Revisit with the package split, or when someone asks.
- [ ] 🟡 **The pre-auth leg reuses the long-lived session cookie.** State, nonce, PKCE verifier and
      destination live in it before login, so someone who can set a cookie on the GPM origin can
      complete a login as themselves in a victim's browser. **Deliberately deferred past the rc**,
      like the logout-revocation item. The fix (a short-lived dedicated pre-auth cookie) reworks the
      callback and the stale-cookie retry-breaker, which key off the session cookie -- a refactor of
      the most security-critical, already-reviewed path for a low-impact, read-only-UI gain, and
      still only defence-in-depth (a cookie-injecting attacker could set the short-lived cookie too;
      the real anti-injection fix is `__Host-` cookies, HTTPS-only). Revisit with the server-side
      session / per-test `*viper.Viper` work.
- [x] 🟢 ~~Assets the logout page references are not public~~ — done. `logo192.png`, `logo512.png`,
      `asset-manifest.json` and `robots.txt` added to the allowlist; covered by `TestIsPublicPath`.
- [x] 🟠 ~~**Stored XSS in the HTML violations report** (found by the post-hardening security
      review).~~ — fixed. `/constraints?report=html` rendered through `text/template`, which does
      not HTML-escape, and interpolated cluster-controlled data (constraint and resource names,
      namespaces, violation messages) straight into the page. A tenant who can name a resource could
      plant markup that runs in the operator's session when they open the report. Switched the
      renderer to `html/template` (contextual auto-escaping; the template has no raw-HTML
      interpolations, so a clean swap) via a shared `newRenderer()`. `report_test.go` reproduces the
      payload and asserts it is escaped, and that benign data still renders. **A port regression:**
      1.x rendered the report with Jinja2, which auto-escapes `.html` templates, so it was safe.
- [x] 🟢 ~~`/logout` GET has no CSRF token~~ — accepted, not fixed. A forced logout is an annoyance,
      not a breach: it ends a session, exposes nothing. GET is the RP-initiated-logout norm, and the
      frontend logs out with `window.location.replace("/logout")`, which POST-only would break. Not
      worth a CSRF token here.
- [x] 🟠 ~~**OIDC is broken on the documented subpath deployment.**~~ — done, and the entry was wrong about the scope.
- [x] 🟡 ~~**`isPublicPath` runs on a decoded, non-normalised path.**~~ — done.

- [x] 🟡 ~~**Symlinks escape the static root.**~~ — done.
- [x] 🟡 **The callback returns JSON on failure, but `/oidc-auth` is a browser navigation.** A user  ** — done: the callback restarts the login instead, bounded to one hop.** arriving with stale state sees raw JSON rather than the er….
- [x] 🟢 ~~No minimum length check on `GPM_SECRET_KEY`~~ — done. `secretKeyError` refuses the 1.x
      default and anything under 16 chars when auth is on. Tested, mutation-checked.
- [x] 🟢 ~~No security headers or server timeouts~~ — done. `middleware.Secure()` (nosniff,
      SAMEORIGIN, XSS) and read/read-header/write/idle timeouts on `e.Server`. No CSP (breaks the
      built React app) and no HSTS (the operator's TLS decision).

### Test-quality debt from the same review

- [x] 🟠 ~~**Tests mutate global viper state and restore the wrong thing.**~~ — done.
- [x] 🟡 ~~`fakeProvider` fields written and read across goroutines with no sync~~ — reassessed:
      there is no reachable race. Each test builds its own `fakeProvider`, and every write precedes
      the `ServeHTTP` that hands it to a handler, so the fields are never shared across goroutines
      without ordering, even under `t.Parallel`. The original note overstated it. A mutex was added
      and then removed as speculative (ponytail's second review). The real `t.Parallel` blocker is
      the global viper `useTestSettings` mutates, which stays with the package split.
- [x] 🟡 ~~Uncovered behaviour worth tests~~ — done. Added: `/api/v1/contexts` returns 401 with a
      login URL, `/metrics` stays reachable without a session, the session cookie's HttpOnly/Secure/
      SameSite flags (Secure toggling with scheme), and a manual-endpoint login end to end (with
      discovery broken to prove that path, not a fallback, drives it). The empty-issuer case was
      already covered by `TestNewAuthenticatorRequiresItsSettings`.
- [x] 🟢 ~~`TestCallbackRejectsWrongNonce`/`...WrongAudience` assert only the status~~ — done. Both
      now assert the answer `Description` mentions the nonce / the audience. Mutation-checked.

- [x] 🔴 **Open redirect fixed in code written for this port.** The post-login destination was validated with `strings.HasPrefix(d, "/")`, which accepts `//evil.com` — browsers resolve that as protocol-relative and leave the site.
- [x] PR [#1399](https://github.com/sighupio/gatekeeper-policy-manager/pull/1399): the `manifests.json` → `manifest.json` typo in the auth-exempt paths, which caused CORS errors. Not portable as a diff, since the Go exempt list was written fresh — but the bug it fixed is accounted for: `isPublicPath` exempts `manifest.json` (spelled correctly), `favicon.ico`, `touch-icon.p….

---

## 2. Helm chart — `0.6.0` here vs `0.16.0` on `main`

Keep `appVersion` and `image.tag` at `v2.0.0-alpha1`; do **not** take `main`'s `v1.1.1`.

- [x] 🔴 ~~`templates/hpa.yaml` uses `autoscaling/v2beta1`~~ — **resolved by removing the HPA entirely**, rather than porting `main`'s `autoscaling/v2` rewrite.
- [x] 🟠 **Configurable probes** — PR [#1274](https://github.com/sighupio/gatekeeper-policy-manager/pull/1274).
- [x] 🟠 **`config.secretRef`** — PRs [#1288](https://github.com/sighupio/gatekeeper-policy-manager/pull/1288) / [#1156](https://github.com/sighupio/gatekeeper-p….
- [x] 🟡 **OIDC optional params** — PR [#976](https://github.com/sighupio/gatekeeper-policy-manager/pull/976).
- [x] 🟢 `templates/NOTES.txt` docs URL — done, but **not** by copying `main`: its version uses `docs.sighup.com`, which is wrong.
- [x] 🔴 **Multi-cluster kubeconfig path fixed.** Both the chart's `deployment.yaml` and `manifests/multi-cluster.yaml` — the Kustomize route, which had the same stale path — mounted the kubeconfig at `/home/gpm/.kube/config`, the Python image's hom….

- [x] 🟡 ~~**`config.preferredURLScheme` is still `required`**~~ — fixed. `required` never fired (the
      chart ships `http` as a default, so the value is never empty) and it checked the wrong thing.
      Replaced it with a `fail` guard that rejects anything other than `http`/`https`, the constraint
      that actually matters since the value drives the `Secure` flag on the session cookie.
- [x] 🟢 Regenerate `chart/README.md`, and add `[bumpversion:file:chart/README.md]` to  ** — done: regenerated with frigate; the bumpversion entry stays out, frigate owns the heading.** `.bumpversion.cfg`.

---

## 3. CI and e2e

- [x] 🔴 **kustomize v5 syntax** — `tests/tests.sh` called `--load_restrictor none` (kustomize v3); `main` moved to `--load-restrictor LoadRestrictionsNone` in `d….
- [x] 🔴 **`e2e-testing` image** — the `tests` and `gpm-port-forward` steps were on `1.1.0_0.7.0_3.1.1_1.9.4_1.24.1_3.8.7_4.21.1` (kustomize 3.1.1, 2023-era).
- [x] 🔴 **kind and cluster version** — was kind `v0.17.0` / cluster `v1.24.7` (2022-era).
- [x] 🟠 `registry.sighup.io/fury/kindest/node:v1.36.1` — was missing from the mirror on the first run; added to `container-image-sync/modules/extra/images.yml` (`E2E Kind`) and synced.
- [x] 🟡 **kubectl skew in the `e2e-testing` image** — *resolved by the mise migration (`4ed4c46`), which pins `kubectl 1.36.1` directly and retires the image.
- [x] 🟡 dependabot npm path on this branch — see section 0.
- [x] 🔴 **QA pipeline re-enabled, with pluto.** It had been commented out wholesale (`# name: qa`) by `dd3874f`, `bf918a2` and `fba0805`, taking superlint, "render manifests" and the pluto deprecated-API check with it. The pluto step here was *newer* than `main`'s (k8s v1.33.0 vs v1.31.0), so it had been ported and then switched off — the `GoPM.md` note was right about the port and stale about the state.
- [x] 🟡 `.github/`: `ISSUE_TEMPLATE/bug_report.md`, `ISSUE_TEMPLATE/feature_request.md`,  ** — done: copied across before the switch.** `pull_request_template.md` are `main`-only.
- [x] 🟠 **UI e2e: wait for the port-forward, and stop hardcoding port 8080.** Done. Two related problems, fixed together. *The race.* `gpm-port-forward` is a `detach: true` step, so Drone starts `ui-tests` immediately — Playwright can connect before the tunnel is up.

- [x] 🟡 ~~**Port the same fix back to `main`** to retire its `sleep 5` (`71d71be`).~~ — out of 2.0
      scope. The §14 `ours` merge overwrites `main`'s tree with the go-backend tree, whose e2e already
      has the fix, so any edit to `main`'s Python e2e is discarded by the switch. The `sleep 5`
      survives only on `release-v1.x`, where retiring it is a ~3 s CI speedup on a maintenance-only
      line. Optional, post-switch, `release-v1.x` only — not a 2.0 task.

> ⚠️ Do **not** copy `main`'s kubectl download URL verbatim. `d3e8c85` ("fix kubectl download url")
> changed it to `https://dl.k8s.io/$CLUSTER_VERSION/bin/linux/arm/kubectl` on an amd64 runner —
> that looks like a bug on `main`, not a fix. Take the `dl.k8s.io` host, keep `amd64`.

### Migrate Drone to `mise` (separate effort, not a port)

Rather than keep chasing `quay.io/sighup/e2e-testing` image tags, the CI should pin its toolchain
with **`mise`**, the way `distribution`, `furyctl` and the Kubernetes modules already do. A
`mise.toml` at the repo root pins kubectl, kustomize, helm, yq, kind, bats (and here also Go and
Node/yarn) at explicit versions, and the Drone steps become `mise run <task>` against a plain base
image. Benefits: the same versions locally and in CI, upgrades become a one-line edit reviewable in
a PR instead of decoding a seven-field image tag, and no dependency on a shared image that has to
carry every tool the whole org needs.

The `tests` step in `.drone.yml` already carries a `# TODO: change to mise` marker.

- [x] 🟠 **Seed `mise.toml`** — created at the repo root pinning `go = "1.26.5"` (matching the `toolchain` line in `go.mod`) and `golangci-lint = "2.12.2"` (cu….
- [x] 🟠 **Migration done** — `4ed4c46`, green on build **#3458**.

#### The org pattern

Taken from `furyctl/.drone.yml` and `module-ingress/.drone.yml` — do not invent a new shape:

```yaml
volumes:
  - name: mise-cache
    host:
      path: /root/mise_data_dir

steps:
  - name: <step>
    image: quay.io/sighup/mise:v2026.6.14
    pull: always
    environment:
      MISE_DATA_DIR: /mise-data
      # Avoids GitHub's 60 req/h unauthenticated limit during tool resolution.
      MISE_GITHUB_TOKEN:
        from_secret: GITHUB_TOKEN
    volumes:
      - name: mise-cache
        path: /mise-data
    commands:
      - eval "$(mise activate bash --shims)"
      - mise run <task>
```

Details worth copying:

- `quay.io/sighup/mise` is an org-published image — no `curl | sh` bootstrap needed.
- The host-mounted tool cache (`/root/mise_data_dir`) stops every step reinstalling the toolchain.
- `furyctl` uses a dedicated `install-tools` step that runs `mise install` once, with the later steps
  only doing `eval "$(mise activate bash --shims)"`. Cleanest of the variants.
- `distribution` wraps the install in `flock` to serialise parallel pipelines sharing the host cache:
  `(flock -w 1800 9 || exit 1; mise install) 9>/mise-data/.mise-install.lock`. Worth adopting — this
  repo has six pipelines.
- `module-ingress` runs `kind create cluster` **from the mise image with the docker socket mounted**,
  so the `docker:dind` step for cluster creation goes away entirely.
- Skip `MISE_OVERRIDE_CONFIG_FILENAMES: "mise.ci.toml"`. The modules set it but `module-monitoring`
  has no such file, so it is vestigial there; `furyctl` does not use it. Keep one `mise.toml`.

#### What migrates

| Step | Today | After |
| --- | --- | --- |
| `license/check` | `golang:1.26` + `go install addlicense` | mise, `mise run license-check` |
| `qa/render` | `e2e-testing` image | mise (kustomize, helm) |
| `qa/check-deprecated-apis` | fairwinds pluto image | mise (`ubi:FairwindsOps/pluto`), or keep — it is a clean single-purpose image |
| `e2e/kind` + `kind-destroy` | `docker:dind` + `wget` kind/kubectl | mise + docker socket, tools pinned |
| `e2e/tests` | `e2e-testing` image | mise (kustomize, bats, kubectl) |
| `e2e/gpm-port-forward` | `e2e-testing` image | mise (kubectl) |

**Stays as-is:** the `docker buildx` build/push steps (`docker:dind` is correct), both Playwright
steps (the official image bundles browsers and system dependencies — reimplementing that under mise
is a bad trade), and the `github-release` / `chart-releaser` plugins.

This removes both remaining `e2e-testing` tag references and every hand-rolled `wget` install — the
exact class of problem that produced the kustomize v3 blocker and `main`'s `linux/arm/kubectl` bug.

#### `mise.toml` additions needed

Currently pins `go` and `golangci-lint` only. Add: `kustomize 5.6.0`, `helm`, `kubectl 1.36.1`,
`kind 0.32.0`, `bats`, `addlicense v1.1.1`, and `node`/`yarn` if the frontend build moves in too.
Tasks: `license-check`, `license-add`, `render`, `e2e`.

Keep `kubectl` and `kind` in step with `CLUSTER_VERSION` / `KIND_VERSION`, and `kustomize` at 5.x —
that pin is what keeps `--load-restrictor LoadRestrictionsNone` working.

#### Prerequisites — both confirmed working on #3458

- [x] `github_token` secret on this repo in Drone (lowercase, matching this repo's existing `quay_username` / `quay_password` convention rather than furyctl's uppercase).
- [x] `/root/mise_data_dir` available as a host path on the workers.

#### Gotchas hit during the migration

- **Duplicate `volumes:` key.** The e2e pipeline already declared `volumes:` at the *bottom* of the
  document; adding a second block at the top meant YAML silently kept the last one, leaving
  `mise-cache` mounted but undeclared. Validate mounts against declarations, not just that the YAML
  parses.
- **`drone lint` is not usable on this file** and it is *not* this change's fault. The committed
  config that produced green build #3457 fails it identically, a normalised YAML round-trip of the
  e2e pipeline still fails, and `distribution` and `gatekeeper-policy-manager-py` fail too (while
  `furyctl` and `module-ingress` pass). All CLI versions behave the same and 1.9.0 is the latest. A
  `ci-lint` task was written and then removed rather than ship one that reports red on a file Drone
  demonstrably accepts. Worth investigating separately — it affects several repos.
- **Two local-only breakages from `node_modules`**, invisible to CI's fresh clone but hitting any
  developer who has run `yarn install`: `license-check` walked into the gitignored `static-content/`
  build output, and `lint` picked up a Go file vendored inside an npm package
  (`web-client/node_modules/flatted/golang`). Fixed with `-ignore` entries and a new
  `.rules/.golangci.yml` — which reverses the earlier "defaults are fine, no config needed" call,
  now that there is a concrete reason for one. `gpm.yaml` / `rendered_chart.yaml` were also
  gitignored, since `mise run render` produces them.

Reference implementations: `furyctl` (Go repo, closest analogue), `module-ingress` (kind/e2e under
mise), `distribution` (flock-serialised install), `gangplank` (task layout). Note
`gatekeeper-policy-manager-py` also has an untracked `mise.toml` covering the Python side.

### Unit tests

There were **no Go tests at all**, and the only frontend test was CRA's `renders learn react link`
boilerplate — which could not pass: the app has no such text, and Jest could not even transform
`@elastic/eui`'s ESM to get that far. It had clearly never run. Replaced rather than repaired.

- [x] 🟠 **Go** — `main_test.go` covers `kubeAPIErrorAnswer` (the TLS detection: each certificate error type, plus the real `url.Error` → `tls.Ce….
- [x] 🟠 **Frontend** — `web-client/src/utils.test.tsx` covers `autoLink` (no-link text, single and multiple URLs, `target="_blank"` + `rel="noreferrer….
- [x] 🟠 **Tasks** — `mise run test:unit:go`, `mise run test:unit:js`, and `test:unit` for both.

Two things the tests found immediately:

- **A real bug in `autoLink`**: it rendered an array of `<a>` elements with no `key` prop, so React
  logged a warning on every violation message containing a link. Fixed.
- A nil-pointer panic in my own first fixture — `x509.HostnameError.Error()` dereferences its
  `Certificate` field. Worth knowing if these tests get extended.

- [x] 🟠 **Wired into CI** — `unit-tests-go` and `unit-tests-js` in the `qa` pipeline, kept as two steps so a failure names the suite.

> ⚠️ **The `App.test.tsx` deletion is load-bearing.** The fresh-checkout run showed it: with that
> file still present, `unit-tests-js` fails on the EUI transform error and takes the `qa` pipeline —
> and therefore `build`, `e2e` and `release` — down with it. It must be staged in the same commit as
> the new tests.

Still open:
- [ ] 🟡 **Jest cannot transform `@elastic/eui`**, so no component or page can be tested. A CRA Jest
      `transformIgnorePatterns` override could fix it, but it is whack-a-mole — the first failure is
      `chroma-js` shipping ESM, and EUI v99 pulls several such deps. **Deferred on purpose:** both
      frontend-toolchain directions make it moot — Vite+Vitest handles ESM natively, and dropping
      React removes the SPA entirely — so a throwaway CRA-Jest workaround is not worth it. Until one
      of those lands, frontend tests stay limited to EUI-free modules.
- [x] 🟡 ~~YAML and shell scripts are still unlinted in CI.~~ — **shell now linted.** `shellcheck`
      (aqua binary, pinned in `mise.toml`) runs on the shell scripts via a `lint-shell` task, wired
      into the `lint` step as a dependency. The one bats file (`tests/tests.sh`) carries a
      file-local `SC2154,SC2329` disable for the bats-runner false positives. **YAML dedicated
      linting was skipped on purpose:** yamllint is Python-only (`pipx:` backend), which reintroduces
      the uv/mise fragility the frigate note warns about, and the YAML that matters is already
      validated — manifests and chart by `kustomize build` + `helm template` (feeding the pluto
      check), `.drone.yml` by Drone, `.github/dependabot.yml` by GitHub. Not worth the CI fragility
      for style-only gain. (If YAML style-linting is ever wanted, install yamllint via `apt` in a
      step rather than through mise.)
- [x] 🟠 **`govulncheck` added to the `qa` pipeline**, pinned at `1.6.0` in `mise.toml` with a `vulncheck` task. It reports on *reachable* call paths rather than the whole module graph, so it stays quiet where Dependabot would not: the current scan finds **0 reachable vulnerabilities** and one unreachable, unfixable module-level advisory (GO-2026-5932, `x/crypto/openpgp` unmaintained), which correctly does not fail the build. Nothing was scanning for known CVEs before — the five golangci-lint linters do not.
- [x] 🟢 **Snyk triggers removed.** Two `refs/heads/snyk-**` refs were left over in the e2e include and release exclude lists; Snyk is not in use.
- [x] 🟠 **The release pipeline's branch exclusion was misspelled** `featuer/go-backend`, so it never matched and the pipeline has been running on this branch all along. The `unstable`, `go` and commit-SHA tags it would have suppressed turned out to be wanted for testing, so the exclusion was removed rather than corrected, with a comment recording why — otherwise the next reader sees an unguarded `refs/heads/**` and re-adds it.

### Frontend formatting

- [x] 🟠 **Prettier adopted**, pinned at `3.9.6` in `mise.toml`, with `.prettierrc.yaml` (Prettier's own defaults written out so editors and CI cannot disag….

> **This diverges from `main` rather than converging with it.** `main` is not Prettier-clean either
> — 14 of its files would also be reformatted — so adopting Prettier here moves the two further
> apart on formatting. That is an acceptable trade because `main`'s frontend source barely moves:
> **6 source commits in the last year against 79 to `package.json`/`yarn.lock`**. The dependency
> sync, which is the part that actually happens, stays a verbatim copy because `yarn.lock` and
> `package.json` are outside Prettier's scope. When `main` does change a source file, port the
> change and re-run `mise run format` — those ports are manual reviews anyway.

- [ ] 🟡 `[1.x-background]` **Consider adopting Prettier on `release-1.x` too.** It would align the Go `main` and Python
      `release-1.x` frontends. Lower value after the switchover: the two lines now diverge for good
      (2.0 is the future, 1.x is maintenance), so source-level syncing is no longer a goal.
- [ ] 🟢 **Stylelint was evaluated and rejected.** It cannot resolve a shareable config
      (`stylelint-config-standard-scss`) when installed through mise's npm backend, since each
      package lands in its own isolated store. Adding it to `web-client/package.json` instead would
      break the byte-for-byte match with `main` that keeps the dependency sync a plain copy. Its only
      concrete gain here was rewriting one media query to range syntax, and pointing
      `stylelint-config-standard-scss` at stylesheets that deliberately override EUI would likely
      produce a lot of unrelated noise. Revisit if the frontend ever grows its own tooling.

### Go linting

- [x] 🟠 **Wired into CI** — a `lint-go` step in the `qa` pipeline runs `mise run lint`.

`golangci-lint run` covers `go vet` — `govet` is one of the five linters enabled by default and runs
vet's own passes, so **do not add a separate `go vet` step**. (`shadow` and `fieldalignment` stay off
by default, but plain `go vet` does not run those either, so it is parity rather than a gap.)

`Dockerfile` still runs `go vet -v` in the backend build stage. Now redundant with the lint task, but
kept as a cheap build-time guard that fires even when nobody ran lint.

First run surfaced 3 `errcheck` issues, all unchecked `viper.BindEnv` returns in `main()`; fixed with
explicit `_ =` plus a comment noting the error is unreachable (`BindEnv` only fails when called with
no key). `govet`, `staticcheck`, `ineffassign` and `unused` were clean on first run. The tree now
lints at **0 issues**, so the task is ready to gate CI once the Drone steps move to `mise run`.

---

## 4. Docs

- [x] 🟠 `README.md`: `GPM_SKIP_TLS_VERIFY` row added to the configuration table.
- [x] 🟠 `README.md`: "Running behind a reverse proxy on a subpath" added, placed under `## Configuration` to match where `main` moved it in `316e297`, and extended with the prefix-stripping requirement `main` leaves implicit. Everything else in this branch's README is *ahead* of `main` (better tables, mutations and events documented, updated screenshots) — it was not overwritten wholesale.
- [x] 🟠 **Release notes started** — `docs/releases/v2.0.0.md`, kept as a living draft and updated as work lands rather than written at release time.
- [x] 🟡 **Release-notes filename settled: `v2.0.0.md`, no rename.** One file covers the release candidate and the final release, which is what `v1.0.0.md`….
- [x] 🟡 `docs/releases/`: this branch stops at `v1.0.3`; `main` has `v1.0.4` → `v1.1.2` (14 files),  ** — done: copied across, v0.1 to v2.0.0 continuous.** with **v1.1.2 now released**.
- [ ] 🟠 **Keep `v2.0.0.md` current** as work lands. Held to so far: `GPM_SKIP_TLS_VERIFY`,
      `PUBLIC_URL` and the chart probe / `secretRef` options all have entries. Still to come —
      authentication, which would remove the biggest breaking change if it ships before the release.
- [ ] 🟢 `MAINTENANCE.md`: small delta (8 insertions / 4 deletions) — worth eyeballing.

---

## 5. Toolchain and dependency maintenance (not a port)

Not backported from `main` — `main` is Python, so it has no equivalent. Recorded here because it
lands in the same batch of work.

- [x] 🟠 **Go 1.24 → 1.26.** `go.mod` (`go 1.26.0` / `toolchain go1.26.5`), the `Dockerfile` backend stage (`golang:1.24` → `golang:1.26`) and the Drone lic….
- [x] 🟠 **`go get -u ./...` + `go mod tidy`.** Direct deps: `k8s.io/client-go` and `k8s.io/apimachinery` v0.33.2 → **v0.36.3**, `echo-contrib` v0.17.4 → **v0.50.1**, `echo/v4` v4.13.4 → v4.15.4, `viper` v1.20.1 → v1.21.0, `golang.org/x/exp` refreshed. No source changes were needed. Structural churn in the indirect set — `sigs.k8s.io/structured-merge-diff/v6` now sits beside v4, and `go.yaml.in/yaml` v2+v3 appeared — is the upstream Kubernetes ya….

> **client-go skew.** Bumping client-go to v0.36 is what forced the kind cluster bump in section 3.
> client-go supports roughly ±1 minor against the API server; against the old `v1.24.7` cluster it
> was ~12 minors out, and even `main`'s `v1.33.0` would have left a 3-minor gap. Keep
> `CLUSTER_VERSION` tracking the client-go minor when either one moves.

- [x] 🔴 **Verified against a real cluster.** Build #3457 is green end to end against kind `v1.36.1`: the `tests` step's API assertions (`/api/v1/configs`,….
- [ ] 🟡 `[1.x-background]` `addlicense` is pinned at `@v1.1.1` in the Drone license step on `main` (Go 2.0);
      `release-1.x` uses `@latest`. The pin is arguably better practice — left as is, noted so it is
      a decision and not an oversight.

---

## 6. Already done — no action

- Frontend source is effectively identical to `main` apart from formatting. `Constraints`,
  `ConstraintTemplates`, `Configurations` and `utils.tsx` are byte-for-byte equal. rego v1,
  ConstraintTemplates `v1`, autolinking, the #1327 UX improvements, violations-table pagination and
  the context dropdown were all backported in July 2025.
- `manifests/rbac.yaml` — this branch is **ahead** (mutations + events permissions). Porting `main`'s
  version would *remove* permissions.
- `tests/kustomization.yaml` — this branch is on module-policy `v1.15.0` vs `main`'s `v1.14.0`.
- `chart/templates/service.yaml` — the annotations support from `main` is already present.

## 7. Explicitly out of scope

- Python 3.13, `uv`, `requirements.txt`, gunicorn — not applicable to the Go backend.
- `manifests/ingress.yaml` forecastle icon URL: `main` points at `master`, this branch at `main`.
  `main` has the regression.
- `renovate.json` and `.rules/.htmlhintrc` — `main`-only, marginal value alongside dependabot.

---

## 8. Cross-check against `GoPM.md`

Verified against the trees rather than taken at face value.

**Confirmed already done on this branch** — no action:

| Note item | Evidence |
| --- | --- |
| Playwright bump (#1376) | both branches at `1.55.1` in `tests/e2e/package.json` and the Drone image |
| Drop unused EUI CSS (#1216) | no `@elastic/eui/dist` or `eui_theme` import in either tree |
| web-vitals update | `reportWebVitals.ts` byte-identical to `main`; now on `web-vitals` 5.3.0 both sides |
| rego v1 | backported July 2025 |
| Context name with forward slash (#981) | `encodeURIComponent` present in `components/Header/Component.tsx:122` |
| Tooltip on context dropdown | `ae3c148` |
| "MODE WARN" → "WARN MODE" | renders `{mode} MODE` in `pages/Constraints/Component.tsx:121` |
| Multi-arch build | `57672a5` and following |
| Logging migration | `slog` since `f7a6a6e` |
| Mutations, events, switch context, RBAC | all shipped; `manifests/rbac.yaml` is ahead of `main` |

**Full frontend re-verification.** The earlier pass only sampled ~10 files; every shared file under
`src/` has now been diffed. The remaining differences are cosmetic only — stylelint autofixes on
`main` (`> span:first-of-type` spacing, `(width >= 1365px)` media syntax, trailing newlines) and the
`Footer` version string, which is correctly `v2.0.0-alpha1` here. No functional frontend gap exists.
Note `main` appears to run stylelint (the py `mise.toml` pins it); adopting it here would close the
cosmetic drift permanently.

**Docs domain, resolved.** `docs.sighup.io` is the SIGHUP Distribution documentation site and
`sighup.io` is the SIGHUP by ReeVo company site; **`.com` is wrong in both places it appears**. An
earlier note in this document claimed the opposite for the footer — that was incorrect. Each branch
had one of each:

| | footer | chart `NOTES.txt` |
| --- | --- | --- |
| this branch | ~~`docs.sighup.com`~~ → fixed to `.io` | `docs.sighup.io` ✅ |
| `main` | `docs.sighup.io` ✅ | `docs.sighup.com` ❌ |

- [x] 🟢 Footer link corrected to `https://docs.sighup.io/`.
- [x] 🟢 `chart/templates/NOTES.txt` now reads `https://docs.sighup.io | https://sighup.io` — the docs-plus-company pairing `main` introduced, with the domain fixed.
- [ ] 🟡 `[1.x-background]` **`release-1.x` still has `docs.sighup.com` in `chart/templates/NOTES.txt`** — worth fixing
      there too, since 1.x is still a supported line.

**Corrected:** `port check della chart con pluto` and `Deprecated APIs Check in CI` are marked done
in the notes, and the port *was* done — but the pipeline is commented out. See the 🔴 item in
section 3.

**Go-backend backlog — not porting work, recorded so it is not lost:**

- [x] 🟠 ~~**Events namespace configurability.**~~ — done.
- [x] 🟡 ~~Use `unstructured.NestedString` (and friends) instead of type assertions when reading
      constraint fields.~~ — already done. The panic-prone assertions this named lived in the old
      constraints comparator; the `sortConstraints` refactor replaced them with `NestedInt64`/
      `NestedString`, and `getKubernetesEvents` reads `source.component` the same way. No unguarded
      type assertion on cluster data remains (the `auth.go` ones are comma-ok session/token reads).
- [x] 🟡 ~~HTTP proxy support~~ — works by default, no code needed, runtime-verified. Nothing
      overrides the transport, so both paths honor `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY`. Verified
      with a logging forward proxy: OIDC discovery (`http.DefaultClient`) routed through it over
      `HTTP_PROXY`, and a Kubernetes API call (client-go) produced a `CONNECT` over `HTTPS_PROXY`. A
      control run with no proxy env went direct, so the variable is what routes it.
- [ ] 🟡 Events: dynamic update via watch, plus a flag to toggle watch vs static list.
- [x] 🟡 ~~**Events view: schema assumption is untested.**~~ — done.

- [x] 🟡 ~~Constraints report: confirm both the API server hostname and the selected context are
      shown.~~ — done. The report only showed the API host. `getConstraints` now passes the selected
      context (route `:context`, or the kubeconfig current-context when the route names none) and the
      footer shows `(context <name>)`, omitted in-cluster where there is no context.
- [ ] 🟢 Review resource requests; re-shoot and shrink screenshots.

**Architecture review (the "Claudio/Alessio" list):**

- [x] 🟠 ~~Split into packages~~ — done as files, not packages.
- [x] 🟠 ~~Package-level globals (`config`, `startingConfig`, `clientset`, `discoveryClient`)~~ — done.
- [x] 🟡 ~~Two `context.TODO()` calls remain; thread a real context through.~~ — done. Both List
      calls (`getCustomResources`, `getKubernetesEvents`) take the handler's `c.Request().Context()`,
      so a disconnected client or the server WriteTimeout cancels the in-flight Kubernetes call.
      A test with a pre-cancelled context guards it (fails against `context.TODO()`).

---

## 9. Simplified English pass over user-visible text (do last)

- [x] 🟡 ~~**Review log messages, error strings, comments and docs against ASD-STE100.**~~ — done.
      Rewrote every `ErrorAnswer` field in `auth.go` and `handlers.go` to STE: active voice, simple
      past over present perfect ("has expired"→"expired"), no phrasal verbs ("carry on"→"continue"),
      "make sure that" standardized for the check/verify concept, "Please" dropped, `Kubeconfig`→
      `kubeconfig`. Fixed real typos: `ocurred` (×5) and `Kubconfig`. Fixed the two factual errors
      this item flagged — the release note now lists the real set of public paths, and the README
      shows the actual `GPM_SECRET_KEY` default (the 1.x string, not *(none)*) — plus `main.go`'s
      refusal message ("could forge"→"can forge"). **`slog` messages left as-is on purpose:** they
      use the `-ing` progress convention (see [[furyctl-log-ing-convention]], the same style here).
      Code comments reviewed and left — their "would" usages are legitimate explanatory prose. The
      chart README is frigate-generated, so not hand-edited. Verified: `go build` + tests green.

      Worth covering: `slog` messages, every `ErrorAnswer` in `main.go` and `auth.go`, the README
      and chart README, `docs/releases/v2.0.0.md`, and the code comments. Aim for short sentences,
      one meaning per word, active voice, and the condition before the instruction.

      Also wrong today: `docs/releases/v2.0.0.md` says only the health check and the auth endpoint
      stay open, but `/metrics`, `/login`, `/logout`, `/oidc-auth`, `/static/*`, `/favicon.ico`,
      `/manifest.json` and `/touch-icon.png` are public too. The README table is correct; the
      release note is not. The README also lists the `GPM_SECRET_KEY` default as *(none)* when it is
      still `g8k1p3rp0l1c7m4n4g3r` whenever authentication is off.

      Known rough edges to start from: `"An error ocurred while ..."` is misspelled in five places
      and reaches the user; `"Check that the Kubconfig file is correct"` in the events handler;
      `"Kubernetes cilent initialization failed"` at startup; and the mixture of "Kubeconfig",
      "kubeconfig" and "Kubconfig" across the same file.

      Do it in one pass at the end rather than piecemeal, so the whole surface ends up consistent.

---

## 10. Evaluate `aube` for the frontend dependencies (do last)

- [ ] 🟡 **Try `aube` in place of yarn v1 for `web-client`.** From the mise authors, so it fits the
      toolchain the rest of this repo already uses, and **it is quietly in play here already**:
      mise's `npm:` backend installs through it, which is how `prettier`, `stylelint` and the rest
      landed — there is an `~/.cache/aube` with a `virtual-store` and a `trust-policy-v1` on this
      machine right now. So the install path is in use and working, just not for `web-client`.

      The pull is the same one behind section 11: **20 direct dependencies expand to 1006 installed
      packages**, and yarn v1 is unmaintained. A resolver with a virtual store and a trust policy is
      a real improvement over that.

      Sequence it **after** the Create React App decision, not before. CRA pins its own toolchain
      expectations and a Vite migration rewrites `package.json` anyway; changing the package manager
      first would mean doing the work twice. If CRA goes, re-evaluate as part of that move.

      Things that would have to keep working:

  - `yarn install --frozen-lockfile` in `mise run test:unit:js` and the `unit-tests-js` CI step.
  - The `Dockerfile` frontend stage, which copies `package.json` and `yarn.lock` for layer caching
    before the rest of the source.
  - `tests/e2e`, which has its own `package.json` and pins Playwright to match two `.drone.yml`
    image tags.
  - **Dependabot.** It understands `yarn.lock`; whether it understands aube's lockfile is the
    deciding question, because losing automated updates on the frontend would undo section 0.
    Check this first — if the answer is no, that probably settles it.

---

## 11. Get off Create React App (do last)

- [ ] 🟡 **Evaluate replacing `react-scripts` with Vite.** CRA is where most of the dependency noise
      comes from: 20 direct dependencies pull in **1006 installed packages**, and `react-scripts` is
      pinned at `5.0.1` — still the latest, because that is the last release there will be. CRA was
      retired as a recommended toolchain, so the tree underneath it only accumulates advisories.
      Nearly every transitive Dependabot PR on this repo — `body-parser`, `websocket-driver`,
      `fast-uri` — is CRA's webpack and dev-server tree, not anything GPM uses directly.

      **Do this last**: it is a build-system swap, and it wants a stable branch and a quiet moment
      rather than to be tangled up with the port.

      Two open items would fall out of it for free:

  - **The EUI Jest transform** (section 3). Vite uses vitest, which handles ESM natively, so
    `@elastic/eui` stops needing a `transformIgnorePatterns` override and component tests become
    possible for the first time.
  - Much of the Dependabot churn simply disappears with the tree that produced it.

      Things to check before committing to it, all of which the port has already touched:

  - **`PUBLIC_URL`** (section 1) becomes Vite's `base`, and `process.env.PUBLIC_URL` in
    `index.tsx` and `AppContextProvider.tsx` becomes `import.meta.env.BASE_URL`. The subpath
    behaviour and the `--build-arg` in the `Dockerfile` have to survive the move.
  - **`REACT_APP_*`** variables become `VITE_*`, which affects `web-client/.env`.
  - The `yarn build` step in the `Dockerfile` and the `test:unit:js` mise task both assume CRA.
  - Playwright baselines may shift if the build output differs at all; budget a snapshot refresh.
  - `@elastic/eui` v99 with React 17 is the real constraint — confirm it builds under Vite before
    committing, since that pairing is what makes this repo unusual.

- [ ] 🟡 **Bigger alternative: drop React (and the SPA) entirely for a server-rendered UI.** GPM is a
      read-only viewer, and the backend already renders HTML with `html/template` (the violations
      report). A Go-templated UI plus a light touch of HTMX or Alpine.js — for the context switcher
      and expand/collapse, about the extent of the interactivity — would collapse the stack to one
      language, delete the whole CRA/EUI/npm surface (the source of nearly all Dependabot churn) and
      the separate frontend build. The cost is real: it drops Elastic UI and the Fury theme, so the
      look has to be rebuilt in CSS. That is a redesign, not a like-for-like port, and it needs a
      design decision first (the release notes promise "the web interface is the one you already
      know"). This **supersedes the Vite item and the EUI Jest-transform item** if chosen — there is
      no bundler and no Jest-ESM problem without an SPA. A deliberate post-2.0 initiative; do not
      tangle it with the rc. Evaluate this against the Vite path before committing to either.

      **Feasibility evaluation (done).** No user-facing feature is blocked: the handlers already hold
      the data as `unstructured`/`map[string]interface{}` (what `html/template` consumes), every
      route already takes `:context`, and the violations report already server-renders. The
      interactivity is almost all native HTML — `EuiAccordion`→`<details>`, `EuiCodeBlock`→`<pre>`,
      the context switcher→`<select>` (already a full reload via `navigate(0)`), Events rows→
      `<details>`, scroll-spy/hash→CSS `:target` + the existing vanilla IntersectionObserver. The
      **one genuinely stateful widget** is the per-constraint violations table (`EuiInMemoryTable`:
      search + multi-column sort + pagination, and several per page) — a small Alpine component over
      a JSON island, or trim scope. `JSONTree` (match/params/spec, 4 views) → server-rendered YAML/
      `<pre>`. **The dominant cost is rebuilding the EUI/Fury look in hand-written CSS**, not
      functionality. Upsides: dropping EUI lets `main.go` add a strict CSP (it ships none today
      because EUI needs inline styles), and the error path gets simpler (real HTTP errors, not
      router state). `/api/v2/contexts/` is dead/unused. Effort: 1 L CSS/theme rebuild + 1 L
      Constraints view + ~5 M views + 4 S views; the L/M items shrink by trimming the table's
      pagination and the JSON-tree collapsibility.

      **In progress on `feat/drop-react`.** All five content views are server-rendered (Configurations,
      Mutations, Constraint Templates, Events, Constraints — the violations table reached full parity:
      search + sort + pagination via Alpine, verified interactively). Shell, logo, theme toggle, nav
      overflow-fade done. Left: the small static views (Home/Error/Logout/NotFound), syntax
      highlighting (planned — server-side chroma), and the cutover (remove `web-client`, the SPA
      fallback and the Dockerfile yarn stage, set a strict CSP). **Before merge, this MUST get a
      security review and a ponytail review** (see the "Before merge" section in
      `docs/dev/drop-react-plan.md`).

---

## 12. Chart e2e testing (low priority, do last)

Goal: install-test the Helm chart against real clusters on chart changes. The reference is a GitHub
Actions workflow using `helm/chart-testing-action` and `ct install --charts chart/ --target-branch
<default>` across a `kindest/node` matrix.

**Use Drone, not GitHub Actions** — Drone is this org's CI and the repo has no workflows directory.
Also worth knowing: `main` added a chart-test GitHub Action (#1026) and later removed it again
("Remove GitHub chart test"). Find out why before reintroducing the same thing in another form.

Sketch, assuming the `mise` migration has already landed:

- Pin `chart-testing` (`ct`) and `helm` in `mise.toml`; add a `chart-test` task wrapping
  `ct lint --charts chart/` and `ct install --charts chart/`.
- `ct` needs a target branch for change detection — use Drone's `DRONE_TARGET_BRANCH`, falling back
  to `main`, in place of the Action's `github.event.repository.default_branch`.
- Reuse the kind cluster the e2e pipeline already creates rather than standing up another; that is
  most of the runtime.
- The Action's k8s matrix becomes either sequential steps or one pipeline per version. Given the
  client-go v0.36 skew constraint from section 5, test the current target (`v1.36.1`) plus the
  oldest version actually supported — not the Action's ancient 1.23/1.29 pair.
- The Action triggers on `paths: chart/**`. Confirm the Drone version in use supports path triggers;
  if not, either always run it or gate it behind a cheap "did chart/ change" guard.

- [x] 🟢 ~~Implement the above~~ — done, but not as sketched.

---

## 13. Release checklist — deliberately deferred

The chart version was **intentionally not bumped** alongside the HPA removal. Version bumps happen
when the release is planned, per `MAINTENANCE.md`. Everything that has accumulated an unreleased
debt is collected here so it is not rediscovered at release time.

- [x] 🔴 ~~**Bump `chart/Chart.yaml` `version`.**~~ — `0.6.0` → **`0.18.0`**.
- [x] 🔴 **Pre-release scheme decided: release candidates, not alphas.** This resolves the bumpversion blocker at no cost — `parse` is already `(?P<major>\d+)\.(?P<minor>\d+)\.(?P<patch>\d+)(\-rc(?P<rc>\d+))?` and `serialize` already emits `{major}.{m….
- [x] 🟠 ~~**One-time rename to `2.0.0-rc.0`.**~~ — done across `.bumpversion.cfg`, `README.md`, `kustomization.yaml`, `Footer/Component.tsx`, `chart/Chart.yaml` and `chart/values….
- [x] 🟠 **`parse`/`serialize` updated for the dotted `rc.N` form.** SemVer's own examples use `1.0.0-rc.1`, and the dot is not cosmetic: dot-separated nume….

- [x] 🔴 **Fixed: an RC tag would have published a Helm chart release.** `release-helm-chart` triggers on `refs/tags/**` and only excluded the chart-releaser's own `gatekeeper-policy-manager-*` tags, so `v2.0.0-rc.0` would have shipped a chart release. `main` guards against this and this branch had lost the guard. Added `refs/tags/v**-rc**` to the exclude list — broader than `main`'s `v**-rc.**`, so it catches both spellings.
- [x] 🟠 ~~**Regenerate `chart/README.md` with frigate.**~~ — done, and it had drifted far further than the four `autoscaling.*` rows: the committed copy documented `image.tag: "v1.0.7"` an….
- [x] 🟡 ~~**Who owns the `chart/README.md` heading.**~~ — frigate does.
- [x] 🟠 ~~**Chart release notes call out the HPA removal.**~~ — already covered in `docs/releases/v2.0.0.md`, both in Breaking changes and in the upgrade procedure.
- [x] 🟡 App release notes: this branch's `docs/releases/` stops at `v1.0.3` — see section 4 for the  **— done: copied across, v0.1 to v2.0.0 continuous.** open question of whether 2.x restarts the notes or….
- [ ] 🟡 **Evaluate publishing the Helm chart to GitHub's registry (OCI) instead of GitHub Pages.**
      Today `release-helm-chart` uses `chart-releaser` (`cr`) to push an index to the `gh-pages`
      branch — a classic Helm HTTP repo. Helm supports OCI registries natively, so the chart could
      instead be `helm push`ed to `ghcr.io/sighupio/...` alongside the container image. Upsides: no
      `gh-pages` branch to maintain, one registry for image and chart, signable/attestable with the
      same tooling. Check first: consumer impact (users would `helm install oci://ghcr.io/...`
      instead of `helm repo add`, so it is a documented breaking change to the install flow),
      whether the SIGHUP release conventions expect the gh-pages repo elsewhere, and auth/visibility
      of the GHCR package.

Tag conventions worth re-reading in `MAINTENANCE.md` before tagging:

- `gatekeeper-policy-manager-<version>` is reserved for helm/chart-releaser — never create these by
  hand; the Drone triggers explicitly exclude them.
- `helm-chart-<version>` releases the chart on its own, but the chart pipeline depends on a
  successful GPM release, so that dependency has to be relaxed for a chart-only release.

---

## 14. Branch switchover

A real merge is out: 1245 commits apart with almost no shared files. Renaming `main` to
`release-1.x` and this branch to `main` also loses more than it looks — every clone's `main` goes
stale, open PRs retarget themselves, and the default branch changes under branch protection.

**Chosen: an `ours` merge.** `-s ours` records `main` as a second parent and keeps this tree
wholesale, so there is nothing to resolve. `main` then fast-forwards onto it, keeps its name, keeps
its protection rules, and keeps 1.x reachable as an ancestor.

```bash
# release-1.x already fast-forwarded to main's latest 1.x (done — see the checklist below):
#   git push origin origin/main:release-1.x
git checkout feature/go-backend
git merge -s ours origin/main -m "2.0 replaces the Python backend; 1.x continues on release-1.x"
git checkout main && git merge --ff-only feature/go-backend && git push origin main
```

Direction matters: run it **from** the Go branch merging `main` **in**. Reversed, it keeps the
Python tree and throws away 2.0.

- [x] 🟠 ~~Copy the files only `main` had~~ — 15 release notes (`v1.0.4`–`v1.1.2`, so `docs/releases` is continuous again), the issue and pull-request templates (GitHub read….
- [x] 🟠 ~~Preserve the latest 1.x before the switch.~~ — done. `release-1.x` existed but sat at
      `v1.0.4`, 1206 commits behind `main` with none of its own. Fast-forwarded it to `main`'s tip
      with `git push origin origin/main:release-1.x`, so 1.x maintenance continues from the real
      latest release, not 1.0.4. **The branch is `release-1.x`, not `release-v1.x`.**
- [x] 🟠 ~~Remove `refs/heads/feature/go-backend` from the `.drone.yml` triggers~~ — done, committed
      just before the merge. `refs/heads/main` covers the e2e pipeline now.

**✅ Switchover complete.** The `-s ours` merge landed and `main` fast-forwarded onto it: `main` is
now the Go 2.0 tree (green CI), and `release-1.x` holds the latest Python 1.x for security patches.
`main` kept its name and branch protection. Development continues on `main`; `feature/go-backend` is
defunct (identical to `main`) and can be deleted whenever convenient.
- [x] 🟡 ~~Renovate vs Dependabot~~ — Dependabot only.
- [x] 🟡 ~~After the switch, `git describe` on `main` finds a 1.x tag until `v2.0.0-rc.0` exists, so
      tag first.~~ — done. **`v2.0.0-rc.0` is published**: tagged on the green HEAD, full pipeline
      green (`license → qa → build → e2e → release`), GitHub prerelease live with the corrected
      release notes, and the multi-arch image at `quay.io/sighup/gatekeeper-policy-manager:v2.0.0-rc.0`.
      The switchover can now fast-forward and `git describe` will find the 2.0 tag. The rc is baking:
      gather feedback, then cut the final `v2.0.0` with the §14 `ours` merge.

---

## Suggested order

1. Dependabot `target-branch` wiring **on `main`** (section 0) — note this branch also had npm
   pointed at `/app/web-client`, a path the Go rewrite deleted, so JS updates were silently dead
   here. Fixed to `/web-client`.
1. ~~superseded~~ — the pending JS PRs there are already
   merged and synced across; only the wiring itself is left
2. HPA removed from the chart (section 2) — ✅ done
3. kustomize v5 flag + `e2e-testing` image bump (section 3) — ✅ done, ship together
4. QA pipeline re-enabled with pluto (section 3) — ✅ done
5. Green CI run — ✅ **build #3457**, all five pipelines. Validates Go 1.26, client-go v0.36 against
   a real 1.36 cluster, kustomize v5, pluto, the chart render after the HPA removal, and the synced
   JS deps. The only hiccup was the missing kind mirror tag, now synced
6. `mise` migration for Drone (section 3) — ✅ **`4ed4c46`, green on build #3458**. Also closes the
   kubectl-skew item, since `kubectl 1.36.1` is now pinned directly
7. `GPM_SKIP_TLS_VERIFY` (section 1)
8. `PUBLIC_URL` (section 1)
9. remaining chart work (section 2) — leave the version at `0.6.0` throughout; one bump at release
10. ~~frontend dependency sync~~ — ✅ done (section 0)
11. docs (section 4)
12. chart e2e testing (section 12) — last, low priority
13. get off Create React App (section 11) — last, and best done on a quiet branch
    (evaluate `aube`, section 10, as part of that same move)
14. Simplified English pass over user-visible text (section 9) — one sweep at the end
15. release (section 13) — the version bumps, frigate regeneration and release notes all happen
    here, not before

Sections 8 (Go-backend backlog) and the architecture review are separate tracks, not part of getting
this branch back in sync.

OIDC is done, so the only 🔴 left are in the release checklist. What remains of the sync itself is
small: the Dependabot path on this branch, the events-namespace RBAC question, and docs.

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

---

## 0. Root cause of the dependency drift

Dependabot reads `.github/dependabot.yml` **only from the default branch**, which is `main`. The
copy on `feature/go-backend` has therefore never been read — it is inert. On top of that it points
npm at `directory: "/app/web-client"`, while the frontend moved to `/web-client` in `82d8371`
(Apr 2023). Net effect: the frontend on this branch has **never** received a Dependabot update, and
every JS bump so far landed as a manual backport.

The fix has to happen on `main`:

- [x] 🔴 **On `main`** — done. `.github/dependabot.yml` now carries two extra entries with
      `target-branch: "feature/go-backend"`: `gomod` at `/` and `npm` at `/web-client`, alongside
      the existing `pip` at `/app` and `npm` at `/app/web-client` for `main` itself. Every manifest
      path was verified to exist on the branch it targets — a wrong path is precisely the bug that
      caused this. From here the branch is maintained automatically and the manual backports stop.
- [x] 🟡 **On this branch** — correct the npm path to `/web-client` anyway. It does nothing today,  **— done: npm path corrected to /web-client.**
      but it goes live the day this branch becomes the default for v2.
- [ ] 🟠 **At the 2.0 switchover, rewrite `.github/dependabot.yml` deliberately.** Once
      `feature/go-backend` becomes the default branch, the `pip` and `/app/web-client` entries are
      dead and the `target-branch` indirection is no longer needed. Whichever copy of the file wins
      the merge will otherwise be wrong in one direction or the other.
- [x] 🟠 **Dependabot covers Go modules too — confirmed.** GitHub's **Insights → Dependency graph →
      Dependabot** tab lists all four manifests: `app/constraints.txt` and `app/web-client/package.json`
      for `main`, and `web-client/package.json` plus **`go.mod`** for `feature/go-backend`. So the
      `target-branch` wiring works for both ecosystems.

      No `go_modules` PR has appeared only because there is nothing to open: `go list -m -u` reports
      no direct dependency with an update available, since `go get -u ./...` was run during this
      port. Nothing to do before merging into `main`, and at the 2.0 switchover the `target-branch`
      indirection goes away entirely — see the rewrite item above.
- [ ] 🟡 **`tests/e2e/package.json` is covered by Dependabot on neither branch.** Playwright is
      pinned there and in two `.drone.yml` image tags that have to match it, so it only ever moves by
      hand. An `npm` entry at `/tests/e2e` would catch it, though that version-match constraint means
      such PRs always need a manual follow-up.

- [x] 🟠 **Frontend dependency sync** — done against `main` @ `ca5b7a8` (after the Dependabot JS PRs
      were merged there). `web-client/package.json` and `web-client/yarn.lock` were replaced verbatim
      with `main`'s `app/web-client/` copies and are now **byte-identical** to them, so the next sync
      is a straight copy again. Landed: `@types/node` 24 → 26.1.1, `sass` 1.83 → 1.101,
      `web-vitals` 4.2 → 5.3, `react-router-dom` 6.28 → 6.30.4, `typescript` 5.6 → 5.9.3,
      `react-json-tree` 0.19 → 0.20, `@testing-library/jest-dom` 6.6 → 6.9.1, plus transitive
      security bumps (`body-parser`, `websocket-driver`). The `yarn.lock` diff is ~980 lines, far
      more than `main`'s own 125, because this branch's lockfile had drifted since July 2025.
      Verified with `yarn install --frozen-lockfile` (lock matches manifest) and `yarn build`
      (succeeds). No source changes were needed — `main` touched no `src/` file in those commits.

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

- [x] 🟠 **`GPM_SKIP_TLS_VERIFY`** — PR
      [#1454](https://github.com/sighupio/gatekeeper-policy-manager/pull/1454), released in v1.1.2.
      Ported. A `viper` key sets `Insecure` on the `rest.Config` in `kubeClient`, which covers both
      the in-cluster and kubeconfig paths from one place (Python only handled in-cluster), and logs
      a warning when active. The CA has to be cleared alongside it — client-go rejects a config that
      is insecure *and* carries a CA.

      Instead of Python's per-endpoint `SSLError` branch, a `kubeAPIErrorAnswer` helper inspects the
      error with `errors.As` (`tls.CertificateVerificationError`, `x509.UnknownAuthorityError`,
      `HostnameError`, `CertificateInvalidError`) and swaps in the TLS message plus the remediation
      hint. All five Kubernetes-call error sites route through it, so the hint appears everywhere
      rather than on the three endpoints Python covers. The mutations loop is untouched: it
      deliberately swallows errors and continues.

      Verified end to end against a self-signed HTTPS server standing in for the API: with the flag
      unset the response is the tailored TLS error and the hint; with `GPM_SKIP_TLS_VERIFY=true` the
      warning is logged and the call succeeds.

- [x] 🟠 **`PUBLIC_URL` / serving from a subpath** — PR
      [#1463](https://github.com/sighupio/gatekeeper-policy-manager/pull/1463), 3 Aug 2026. Ported
      as three pieces:
  - `web-client/src/index.tsx`: `<BrowserRouter basename={process.env.PUBLIC_URL}>`
  - `web-client/src/AppContextProvider.tsx`: production `apiUrl` becomes `` `${process.env.PUBLIC_URL}/` ``
  - `Dockerfile`: `ARG PUBLIC_URL=""` / `ENV PUBLIC_URL=$PUBLIC_URL` in the frontend build stage

      **The backend needed no change.** The open question was whether the Go static-file handler
      (`e.GET("/*")`) and the `/api/v1` routes cope with a subpath. They do, because the subpath
      never reaches them: the reverse proxy strips it and GPM keeps serving from the root exactly as
      before. Verified by building with `PUBLIC_URL=/gpm`, deploying that build, and requesting
      every path the browser asks for once the prefix is removed — `/`, the hashed JS and CSS,
      `/favicon.ico`, `/manifest.json`, the SPA deep links `/constraints` and `/mutations`,
      `/api/v1/auth` and `/health`. All returned 200, and the deep links return the SPA shell with
      its assets pointing back at `/gpm/`.

      That strip requirement is the one thing a user can get wrong, so the README calls it out in an
      `[!IMPORTANT]` block rather than leaving it implicit as `main` does.

      A default build was checked too: with `PUBLIC_URL` unset, assets stay at `/static/...` and
      `apiUrl` stays `/`, so existing deployments are unaffected.

### Pre-existing parity gap (not a port, but it gates part of the backlog)

- [x] 🔴 **OIDC authentication — implemented.** The largest gap in the port, now closed. The
      frontend needed no changes at all: it already reads `/api/v1/auth`, shows the logout control
      when authentication is on, and renders a logout page.

      Implementation lives in `auth.go` rather than growing `main.go` further:
      `coreos/go-oidc/v3` and `golang.org/x/oauth2` for the flow, `echo-contrib/session` (gorilla)
      for a cookie session keyed by `GPM_SECRET_KEY`. Every environment variable keeps its 1.x name
      and the callback stays at `/oidc-auth`, so an existing client registration and values file
      keep working. Discovery from the issuer by default; setting any endpoint by hand turns
      discovery off and GPM then insists on the full set rather than half-configuring itself.

      Two deliberate departures from the Python behaviour, both agreed up front:

  - **Every data endpoint requires a session.** Python left `/api/v1/contexts` open, leaking
    kubeconfig context names to anyone who could reach GPM. `/api/v1/auth` and `/health` are the
    only public API routes, because the frontend has to ask whether there is anything to log into
    before it can show a login.
  - **`/api/*` answers `401` with the usual JSON error body instead of redirecting.** A cross-origin
    redirect to the identity provider surfaces in `fetch()` as an opaque network error, so an
    expired session used to look like a generic failure. Only real navigation is redirected.

      Verified with an integration test (`auth_flow_test.go`) that stands up a real OIDC provider —
      RSA keypair, discovery document, JWKS, token endpoint, properly signed ID tokens — and drives
      the whole round trip: unauthenticated request redirects to the provider, the callback is
      accepted, the originally requested URL including its query string is restored, and the
      resulting session opens the protected page. Three rejection paths are covered too: mismatched
      `state` (CSRF), mismatched `nonce` (replay), and an ID token minted for another client
      (audience). Plus logout redirecting to the discovered end-session endpoint.

      Follow-ons closed with it: the "Authentication is not available" breaking change is gone from
      the release notes, `TestGetAuthReportsAuthDisabled` was rewritten as
      `TestGetAuthReflectsConfiguration` (it now drives both states rather than asserting a
      hardcoded `false`), and the PR #976 chart conditionals are done — see section 2.

      **Verified against a real Keycloak** (realm `test`, modern layout with no `/auth` prefix):
      login, every page, and logout all work.

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

- [x] 🟠 **Two ideas adopted from the earlier `feature/go-backend-oidc` WIP branch.** That branch
      reached for the same libraries independently, which was a useful check on the approach, and
      had two things this implementation lacked:
  - **A `/login` route.** The 401 answer needs somewhere to send people; "reload the page" was a
    poor substitute. It takes an optional `?next=` (same-site paths only) and short-circuits to
    that target when a session already exists.
  - **Unwrapping `oauth2.RetrieveError`.** `oauth2` flattens a provider's rejection to
    "cannot fetch token: 401 Unauthorized", while the body underneath names the actual problem —
    unknown client, bad secret, mismatched redirect URI. `describeTokenError` surfaces it, handling
    both the RFC 6749 JSON shape and free-form bodies.

      Not adopted, with reasons: route groups (`e.Group`) for protection, because they fail open —
      a new route is public until someone remembers to protect it, whereas the `isPublicPath`
      allowlist fails closed and is pinned by `TestIsPublicPath`; state and nonce in separate
      cookies rather than the signed session; and the `oidc_issuer_url` / `oidc_redirect_url`
      naming, since keeping the 1.x names is what lets an existing values file and client
      registration work untouched.

- [x] 🟠 **A "Log in" button on the error page.** Checked in a browser rather than argued about: the
      frontend already handles a 401 well — it parses the `ErrorAnswer` body and renders a proper
      error page — but it does not act on it, so an expired session dead-ended with the login path
      shown as plain text. Verified by loading the SPA with no session through `/logout` (a public
      path that serves the shell), then doing a client-side route change to a protected page.

      Rather than sniff the message text, the backend now sets an optional `login_url` on the
      `ErrorAnswer`, present only when signing in would actually fix the error. The error page shows
      a **Log in** button when it is there and stays unchanged for every other failure. Confirmed in
      the browser against a live instance.

      Chosen over auto-redirecting on 401, which would have meant editing the identical
      fetch-and-catch block in five page components — that duplication is worth factoring out first.

- [ ] 🟡 **Nothing ever populates `ErrorPageState.entity`.** All five pages navigate to `/error` with
      only `{ error }`, so the existing "Go back" button always points at the home page rather than
      the page that failed, and the login button can send no useful `?next=`. Pre-existing, not
      introduced by the login button. Fixing it means passing the current path from each page — the
      same five call sites whose duplicated fetch-and-catch blocks are worth factoring out anyway,
      so do both at once.

- [x] 🔴 **Pre-commit security review, and the six defects it found.** A fresh reviewer went over the
      uncommitted OIDC work before it landed. It was worth it: two of the findings were full
      authentication bypasses, and one would have turned red in CI on the first push. Every fix has
      a regression test that was **confirmed to fail against the old code** before being trusted.

  - **Path traversal, reachable with no session** — `serveIndex` joined the raw `RequestURI`, query
    string included and unnormalised, into a filesystem path. `/logout` is public and falls through
    to it, so `GET /logout?x=/../../../secret` served arbitrary files past the authentication this
    branch exists to add — in a pod, the service-account token and any mounted kubeconfig.
    Reproduced against a running instance, then fixed to use `URL.Path`, `path.Clean` and a check
    that the result is still under the static root. `logout` now serves the shell directly and never
    derives a path from the request. The underlying bug is pre-existing and also reachable at `/`
    when authentication is off, so it affects every 2.x deployment, not only authenticated ones.
  - **The 1.x default `GPM_SECRET_KEY` signs session cookies** — the value is published in this
    repository, so anyone can forge a session offline. Confirmed by minting one. GPM now exits at
    startup rather than enabling authentication with it, and the chart fails at template time if
    OIDC is on with neither `config.secretKey` nor `config.secretRef`.
  - **`GPM_SESSION_MAX_AGE` never expired anything** — replacing `store.Options` wholesale leaves
    the securecookie codecs, which are the *server-side* check, at their 30-day default. Only the
    browser-facing attribute changed, so a stolen cookie stayed valid for a month regardless of the
    setting. Fixed with `store.MaxAge(n)`, which sets both.
  - **Open redirect via control characters** — browsers strip tab, CR and LF before resolving a URL,
    so `/\t/evil.com` becomes `//evil.com`. The prefix check could not see it. `safeRedirectTarget`
    now rejects those characters and parses the target instead of pattern-matching it.
  - **The test suite would have failed in CI** — `TestLogoutRoundTripDoesNotLoop` asserted `200`,
    which needs `static-content/`: a gitignored frontend build artifact that the `unit-tests-go`
    step does not produce. It passed locally only because of a stale build. Now asserts the property
    under test — no redirect — and the whole suite was verified with `static-content/` moved aside.
  - **Manual endpoints did not require `GPM_OIDC_ISSUER`** — go-oidc compares the `iss` claim
    literally, so an empty issuer rejected every token. The chart made it the default for that path
    by emitting the variable unconditionally. Both fixed.

      Taken on top: **PKCE** (`S256`, verified on the wire), a 30-second timeout on discovery so a
      slow provider cannot hang the pod before `/health` binds, `Secure` on the session cookie when
      the redirect domain is `https://` regardless of the scheme hint, and `/metrics` added to the
      allowlist — enabling authentication had silently broken Prometheus scraping.

      **`describeTokenError` was removed**, which reverses part of what was adopted from the WIP
      branch. Two reasons: `oauth2.RetrieveError.Error()` already contains the provider's response
      body, so it duplicated rather than added; and returning that body to an anonymous caller is an
      oracle, since `/login` is public and a bad code can be replayed. The detail is logged instead,
      where the operator needs it. Its test could not fail either — plain `err.Error()` satisfied
      every assertion.

### Second review, of the fixes themselves

The fixes above were new, unreviewed code written straight after finding out the first pass had
holes, so a second reviewer went over just that diff. **No critical findings** — the two bypasses
are genuinely closed. It found four things worth fixing and confirmed the rest by mutation testing.

- [x] 🔴 **The regression test for the session-expiry fix was worthless.** It mutated the store's own
      codecs before decoding, so it measured its own mutation rather than the code, and passed with
      `store.MaxAge(maxAge)` deleted. The header of `security_test.go` claimed every test in it had
      been verified against the broken code; that was untrue for this one. **Third time in this
      port** that a test which could not fail slipped through — the pattern is asserting on
      something the test itself set up. Rewritten and confirmed to fail without the fix.
- [x] 🟠 **`GPM_SESSION_MAX_AGE=0`, empty or unparseable disabled expiry entirely.** `viper.GetInt`
      yields `0` for all of them, and `securecookie.MaxAge(0)` means "never check the timestamp", so
      a typo reopened the exact hole the fix had just closed. Now falls back to the default with a
      warning, with a test across `0`, `""`, `8h` and `not-a-number`.
- [x] 🟠 **PKCE and the manual-mode issuer requirement were shipping untested.** Mutation testing
      showed both could be deleted with the suite still green. The stub provider now verifies PKCE
      the way a real one does — S256 of the verifier must equal the recorded challenge — and there
      is a case for complete manual endpoints with no issuer.
- [x] 🟠 **`clientSecret` was the one chart value the quoting pass missed.** A secret containing
      YAML metacharacters broke templating outright, and a numeric one rendered as an int, which the
      API server rejects for `stringData`.
- [x] 🟡 **`/metrics` host label was unbounded.** Keeping metrics on the main port outside auth is
      the chosen design, which makes `RequestCounterHostLabelMappingFunc` — the raw `Host` header —
      an attacker-controlled label in a process that never forgets series. Measured at 50 hosts →
      650 series and `/metrics` growing from 40 KB to 279 KB. Now blanked. The `url` label was
      already safe: echo-contrib uses the route pattern, not the path.
- [x] 🟢 `startLogin` discarded the session error and would nil-deref if the store were ever absent;
      `middleware.Recover()` was missing entirely, so any panic dropped the connection with no
      response at all. Both fixed.

### OIDC follow-ups, deliberately not done now

Findings from the review that were consciously left, so they are decisions rather than oversights.

- [x] 🟠 ~~**Session cookies are signed but not encrypted**~~ — done, the readability half.
      `sessionKeys` derives a 64-byte hash key and a 32-byte AES-256 block key from
      `GPM_SECRET_KEY` with HKDF (`crypto/hkdf`, standard library since Go 1.24, so no new
      dependency) and passes both to `sessions.NewCookieStore`. No salt, deliberately: the keys have
      to come out the same on every replica and after every restart.

      Five mutations checked. The first pass of the main test was worthless — it peeled only the
      outer base64, and securecookie's format is `base64(date|base64(payload)|mac)`, so it never
      reached the payload and passed with the block key removed. Fixed to split on `|` and decode
      each part; it now fails with the plaintext user name, nonce and PKCE verifier quoted back.

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
      destination all live in it before anyone has logged in, so someone who can set a cookie on the
      GPM origin can complete a login as themselves in a victim's browser. Low impact on a read-only
      UI. A short-lived dedicated cookie for the pre-auth leg is the conventional fix.
- [ ] 🟢 **Assets the logout page references are not public** — `/manifest.json` is allowlisted but
      `/logo192.png`, `/logo512.png`, `/asset-manifest.json` and `/robots.txt` are not, so they
      redirect to the provider and produce console noise on the one page deliberately made
      reachable without a session.
- [ ] 🟢 **`/logout` is a GET with no CSRF token**, so `<img src=".../logout">` force-logs-out a
      viewer. Annoyance only, and GET is the RP-initiated-logout norm; noted for completeness.
- [x] 🟠 ~~**OIDC is broken on the documented subpath deployment.**~~ — done, and the entry was
      wrong about the scope. Subpath serving was broken for **all** navigation, not only with OIDC:
      nine in-app links were raw root-relative `href`s and the header logout used
      `window.location.replace("/logout")`. `BrowserRouter basename` does not touch either, so with
      `PUBLIC_URL=/gpm` the home page rendered and every link left the app. Confirmed by building
      the frontend with `PUBLIC_URL=/gpm` behind a prefix-stripping proxy: all seven header and
      body links resolved to `http://host/constraints...`, and the proxy logged the 404.

      Frontend: `appPath()` in `utils.tsx` prefixes `process.env.PUBLIC_URL`; applied to all nine
      links and the logout redirect.

      Backend: `basepath.go` adds `GPM_BASE_PATH` with `browserPath` / `backendPath`. The proxy
      strips the prefix before GPM sees a request, so GPM matches on prefix-less paths and has to
      put the prefix back on the six things it sends the browser — `redirect_uri`, the 401
      `login_url`, the post-login destination, the already-signed-in redirect,
      `post_logout_redirect_uri` — while stripping it from an incoming `?next=` so it is not
      applied twice. `browserPath` is the identity at the root, which is why nothing changes for
      the normal deployment.

      The Dockerfile sets `GPM_BASE_PATH` from the existing `PUBLIC_URL` build argument in the
      runtime stage, so the two halves cannot disagree and there is no second knob to set.

      Verified end to end: navigation works under `/gpm`; against the real Keycloak the
      `redirect_uri` is `http://127.0.0.1:8090/gpm/oidc-auth` and the provider accepts it rather
      than rejecting the redirect URI. Root deployment re-checked unchanged. Eleven new tests, all
      mutation-checked; the full post-login round trip is covered against the fake provider in
      `auth_flow_test.go`.

      `GPM_OIDC_REDIRECT_DOMAIN` stays scheme-and-host; GPM appends the subpath. Documented in the
      README, and the release notes' known-limitation entry is removed.
- [x] 🟡 ~~**`isPublicPath` runs on a decoded, non-normalised path.**~~ — done. The allowlist now
      checks the raw path **and** its cleaned form, and both must pass. Cleaning alone was the
      trap: it closes `/static/../api/v1/constraints` and opens `/api/v1/../../static/x`.

      A middleware-level test guards the wiring and looks redundant next to the unit table. It is
      not, and a conciseness review proposed deleting it: cleaning inside the middleware instead of
      inside `isPublicPath` — the instinctive fix, and one of the two this item suggested — makes
      `/api/v1/../../static/x` public, and only the middleware test catches that. Its comment now
      says so.

- [x] 🟡 ~~**Symlinks escape the static root.**~~ — done. `serveIndex` now opens the directory with
      `os.OpenRoot` and reads through `Root.Open`, which refuses `..` and escaping symlinks at the
      syscall level. That also deleted the lexical prefix check and the `os.Stat`/`c.File` pair that
      followed links. `http.ServeContent` serves the opened handle, keeping content type and range.

      The symlink test was vacuous on the first try — it pointed the symlink one level too shallow,
      so `os.Stat` failed and nothing leaked for the wrong reason. Fixed, then confirmed both ways:
      the old `c.File` code leaks `SERVICE-ACCOUNT-TOKEN`, the new code does not.

      Follow-up: that first pass only closed `/*`. `/static/*` was a **second** route,
      `e.Static(...)` over `http.Dir`, which follows symlinks just the same, so the hole was still
      open there. `serveIndex` already serves the whole root, so the `e.Static` route was deleted
      and everything now goes through the one `os.Root` path. Not the framework swap the review
      proposed: echo's `Static` over the default `http.Dir` would have reintroduced the escape.
- [x] 🟡 **The callback returns JSON on failure, but `/oidc-auth` is a browser navigation.** A user  **— done: the callback restarts the login instead, bounded to one hop.**
      arriving with stale state sees raw JSON rather than the error page. Redirect to the SPA with
      the error instead.
- [ ] 🟢 **No minimum length or entropy check on `GPM_SECRET_KEY`** — only the exact 1.x string is
      refused, so `GPM_SECRET_KEY=x` starts happily.
- [ ] 🟢 **No security headers or server timeouts** — no `middleware.Secure()` (X-Frame-Options,
      CSP, X-Content-Type-Options) and `e.Start` uses echo's default server with no read or write
      timeouts.

### Test-quality debt from the same review

- [x] 🟠 ~~**Tests mutate global viper state and restore the wrong thing.**~~ — done. `main()`'s
      configuration block is now `bindSettings()`, and `useTestSettings(t)` in `main_test.go`
      resets viper and re-applies it at the start of every test that touches a setting. Tests keep
      using `viper.Set`; nothing hand-restores keys any more.

      Confirmed against the old code with a probe test running last: it saw `session_max_age=""`,
      `secret_key=""`, `preferred_url_scheme=""` — none of them values GPM could ever hold.

      The helper also blanks `GPM_*` in the environment for the duration of the test, because
      `bindSettings` binds them and this repository's own gitignored `mise.local.toml` exports six.
      Without that, `TestNewAuthenticatorRequiresItsSettings/no_client_id` picks up the
      developer's `GPM_OIDC_CLIENT_ID`, stops testing the missing-client-id path, and makes a real
      network call to the issuer. Both behaviours have tests, checked against a broken helper.

      Passes under `-shuffle=on` and with hostile `GPM_*` set. A per-test `*viper.Viper` was the
      other option; it needs a `*viper.Viper` threaded through every read in `auth.go`, so it is
      better done with the package split.
- [ ] 🟡 **`fakeProvider.nonceOverride` and `audOverride` are written from the test goroutine and
      read from the server goroutine** with no synchronisation. `-race` does not flag it today
      because the connection hand-off establishes happens-before, but it breaks the moment anyone
      adds `t.Parallel()`.
- [ ] 🟡 **Uncovered behaviour worth tests**: `/api/v1/contexts` returning 401 — the one item in the
      release notes' breaking-changes list with nothing behind it — cookie flags (`HttpOnly`,
      `Secure`, `SameSite`), `/metrics` reachability, and the manual-endpoint configuration path end
      to end, which is what would have caught the empty-issuer bug.
- [ ] 🟢 `TestCallbackRejectsWrongNonce` and `...WrongAudience` assert only the status code, so they
      cannot tell "rejected for the right reason" from "the fake provider broke". Assert on the
      `Description` too.

- [x] 🔴 **Open redirect fixed in code written for this port.** The post-login destination was
      validated with `strings.HasPrefix(d, "/")`, which accepts `//evil.com` — browsers resolve
      that as protocol-relative and leave the site. Reachable by requesting such a path directly,
      and it would have become far easier to hit through `/login?next=`. `safeRedirectTarget` now
      rejects `//` and `/\` prefixes as well, and is applied in all three places a destination is
      used. Both tests covering it were confirmed to fail against the old check before being
      relied on.
- [x] PR [#1399](https://github.com/sighupio/gatekeeper-policy-manager/pull/1399): the
      `manifests.json` → `manifest.json` typo in the auth-exempt paths, which caused CORS errors.
      Not portable as a diff, since the Go exempt list was written fresh — but the bug it fixed is
      accounted for: `isPublicPath` exempts `manifest.json` (spelled correctly), `favicon.ico`,
      `touch-icon.png` and everything under `/static/`, and `TestIsPublicPath` asserts each one,
      along with near-misses like `/healthz` and `/api/v1/authorized` that must *not* be public.

---

## 2. Helm chart — `0.6.0` here vs `0.16.0` on `main`

Keep `appVersion` and `image.tag` at `v2.0.0-alpha1`; do **not** take `main`'s `v1.1.1`.

- [x] 🔴 ~~`templates/hpa.yaml` uses `autoscaling/v2beta1`~~ — **resolved by removing the HPA
      entirely**, rather than porting `main`'s `autoscaling/v2` rewrite. The template was broken on
      any cluster ≥ 1.26 and there is no known user of it. Removed: `templates/hpa.yaml`, the
      `autoscaling` block in `values.yaml`, the `{{- if not .Values.autoscaling.enabled }}` guard
      around `replicas` in `deployment.yaml`, and the four `autoscaling.*` rows in `chart/README.md`.
      Nothing else in the chart referenced it. Technically a breaking change for the chart — the
      version bump and release note for it are **deliberately deferred to release time, see
      section 13**. Consequently the HPA `behavior` / `metrics` / `targetMemoryUtilizationPercentage`
      / `maxReplicas` items from `main` are dropped too.
- [x] 🟠 **Configurable probes** — PR
      [#1274](https://github.com/sighupio/gatekeeper-policy-manager/pull/1274). Ported: full
      `livenessProbe` / `readinessProbe` blocks in `values.yaml`, templated behind an `enabled`
      toggle in `deployment.yaml`. Defaults reproduce the previous hardcoded behaviour, so nothing
      changes for existing users.
- [x] 🟠 **`config.secretRef`** — PRs
      [#1288](https://github.com/sighupio/gatekeeper-policy-manager/pull/1288) /
      [#1156](https://github.com/sighupio/gatekeeper-policy-manager/pull/1156). Ported: `secret.yaml`
      guarded by `{{- if .Values.config.secretKey }}`, and the env var is
      `if secretKey / else if secretRef / else nothing`.

      **This also fixed a live bug.** `secret.yaml` used `required` on `config.secretKey`, so
      `helm template chart` failed outright — the chart could not be installed without inventing a
      secret that the Go backend then ignores, since it never reads `GPM_SECRET_KEY`. A bare
      `helm install` now works and emits no Secret and no `GPM_SECRET_KEY`.

      Verified across six renders: bare install, `secretKey` set (Secret created, env points at it),
      `secretRef` set (no Secret created, env points at the named one), probe defaults, probes
      individually disabled and retuned, both probes off. `helm lint` clean.

      Note `config.preferredURLScheme` is still `required` and also unread by the backend. It has a
      default so it does not block installs — folded into the OIDC work rather than fixed here.
- [x] 🟡 **OIDC optional params** — PR
      [#976](https://github.com/sighupio/gatekeeper-policy-manager/pull/976). Done alongside the
      OIDC work. Each endpoint is now emitted only when set, and the stray
      `{{- if and .Values.config.oidc.enabled}}` is gone. This matters more here than it did on
      `main`: setting any endpoint disables discovery, so empty strings would be
      indistinguishable from a deliberate manual configuration. Verified by rendering three ways —
      OIDC off emits no auth variables at all, discovery-only emits just issuer/client/redirect,
      and manual endpoints emit exactly the ones provided.
- [x] 🟢 `templates/NOTES.txt` docs URL — done, but **not** by copying `main`: its version uses
      `docs.sighup.com`, which is wrong. See the docs-domain note in section 8.
- [x] 🔴 **Multi-cluster kubeconfig path fixed.** Both the chart's `deployment.yaml` and
      `manifests/multi-cluster.yaml` — the Kustomize route, which had the same stale path — mounted
      the kubeconfig at `/home/gpm/.kube/config`, the Python image's home directory. The Go image is
      distroless running as `nonroot`, so the file landed where GPM never looks and multi-cluster
      silently did nothing. Both now use `/home/nonroot/.kube/config`, matching what the README and
      release notes already promised.

      Worth recording, because it nearly caused a wrong "fix": the distroless image sets no `HOME`
      in its `Config.Env`, which looks like it would break `/home/nonroot` detection too. It does
      not — the container runtime derives `HOME` from the user's `/etc/passwd` entry at start.
      Confirmed against the published image, which logs
      `"trying to load kubeconfigs","paths":["/home/nonroot/.kube/config"]`. Testing the binary
      locally with `HOME` unset is *not* representative: client-go then falls back to a relative
      `.kube/config`.

- [ ] 🟡 **`config.preferredURLScheme` is still `required`** in the deployment template even though
      it has a default, and the value now genuinely matters — it drives the `Secure` flag on the
      session cookie. Worth revisiting alongside the multi-cluster fix.
- [x] 🟢 Regenerate `chart/README.md`, and add `[bumpversion:file:chart/README.md]` to  **— done: regenerated with frigate; the bumpversion entry stays out, frigate owns the heading.**
      `.bumpversion.cfg`.

---

## 3. CI and e2e

- [x] 🔴 **kustomize v5 syntax** — `tests/tests.sh` called `--load_restrictor none` (kustomize v3);
      `main` moved to `--load-restrictor LoadRestrictionsNone` in `d1f9e12`. **Unblocked**: the
      `e2e-testing` image now carries kustomize 5.6.0, see below.
- [x] 🔴 **`e2e-testing` image** — the `tests` and `gpm-port-forward` steps were on
      `1.1.0_0.7.0_3.1.1_1.9.4_1.24.1_3.8.7_4.21.1` (kustomize 3.1.1, 2023-era). Both moved to
      `2.24.17_1.1.0_3.12.0_1.32.2_5.6.0_1.9.0_4.33.3` — the tag `main` runs, carrying kustomize
      5.6.0. This is what makes the `--load-restrictor LoadRestrictionsNone` change safe. The
      `mise` migration is still the better end state; this unpins the blocker in the meantime.
- [x] 🔴 **kind and cluster version** — was kind `v0.17.0` / cluster `v1.24.7` (2022-era). Bumped to
      kind `v0.32.0` / cluster `v1.36.1` on both the create and destroy steps, and the deprecated
      `storage.googleapis.com/kubernetes-release` kubectl host swapped for `dl.k8s.io/release`
      (keeping `amd64` — see the warning below). Went past `main`'s `v1.33.0` deliberately: see the
      client-go skew note in section 5. `v1.36.1` rather than the `v1.36.3` stable because kind node
      images are built per kind release and `v1.36.1` is the tag kind `v0.32.0` ships.
- [x] 🟠 `registry.sighup.io/fury/kindest/node:v1.36.1` — was missing from the mirror on the first
      run; added to `container-image-sync/modules/extra/images.yml` (`E2E Kind`) and synced. Closed.
- [x] 🟡 **kubectl skew in the `e2e-testing` image** — *resolved by the mise migration (`4ed4c46`),
      which pins `kubectl 1.36.1` directly and retires the image. Kept here for the record.*
      Decoding the tag via
      `generic-container-images/e2e-testing/spec.yaml` (ARGs joined with `_` in alphabetical order:
      `AWSCLI, BATS, HELM, KUBECTL, KUSTOMIZE, TOFU, YQ`) shows
      `2.24.17_1.1.0_3.12.0_1.32.2_5.6.0_1.9.0_4.33.3` carries **kubectl 1.32.2** — four minors
      behind the `v1.36.1` cluster, outside the supported ±1. It confirms kustomize 5.6.0, so the
      `--load-restrictor` change is safe, but the `tests` and `gpm-port-forward` steps run their
      `kubectl apply` / `wait` / `get` / `port-forward` against a 1.36 API server with a 1.32 client.
      **In practice it works** — build #3457 passed — but it stays unsupported and is a latent source
      of odd failures as the cluster version moves further ahead.

      No existing tag fixes this — `spec.yaml`'s `KUBECTL` list stops at `1.33.4`. Resolved for free
      by the mise migration below, which pins `kubectl 1.36.1` directly and retires the image; the
      alternative is adding kubectl 1.36.x to `generic-container-images/e2e-testing/spec.yaml` and
      building a new tag. No longer urgent either way.

      This is the second time the opaque `e2e-testing` tag has hidden a version problem (kustomize
      3.1.1 was the first) — a standing argument for the migration.
- [x] 🟡 dependabot npm path on this branch — see section 0. The `main`-side wiring, which is the  **— done: npm path corrected to /web-client.**
      part that actually does anything, is done.
- [x] 🔴 **QA pipeline re-enabled, with pluto.** It had been commented out wholesale (`# name: qa`)
      by `dd3874f`, `bf918a2` and `fba0805`, taking superlint, "render manifests" and the pluto
      deprecated-API check with it. The pluto step here was *newer* than `main`'s (k8s v1.33.0 vs
      v1.31.0), so it had been ported and then switched off — the `GoPM.md` note was right about the
      port and stale about the state. What changed:
  - Pipeline uncommented and wired back in: `license → qa → build → e2e → release`. The `build`
    pipeline's `depends_on: [qa]` was commented out too and is now restored, so QA gates the build
    the way it does on `main`.
  - **Fixed a latent bug**: the render step wrote `gpm.yml` while pluto read `gpm.yaml`. The step
    could never have passed as written. Now both use `gpm.yaml`.
  - Render step image moved to the kustomize 5.6.0 tag (same as the e2e steps).
  - pluto target raised v1.33.0 → **v1.36.0**, matching `CLUSTER_VERSION`. Keep the two in sync.
  - Superlint stays disabled, with a comment explaining why: Go is covered by `mise run lint`, and
    the front-end/YAML linting is unfinished business for the mise migration.

      Verified locally: `kustomize build .` and `helm template --set config.secretKey=e2e chart`
      both render, and every resulting `apiVersion` (`apps/v1`, `rbac.authorization.k8s.io/v1`,
      `v1`) is current — so pluto passes. That is only true because the HPA was removed;
      `autoscaling/v2beta1` is exactly what this check exists to catch.
- [x] 🟡 `.github/`: `ISSUE_TEMPLATE/bug_report.md`, `ISSUE_TEMPLATE/feature_request.md`,  **— done: copied across before the switch.**
      `pull_request_template.md` are `main`-only.
- [x] 🟠 **UI e2e: wait for the port-forward, and stop hardcoding port 8080.** Done. Two related
      problems, fixed together.

      *The race.* `gpm-port-forward` is a `detach: true` step, so Drone starts `ui-tests`
      immediately — Playwright can connect before the tunnel is up. On `main` this was papered over
      with a `sleep 5` (`71d71be`); that is a workaround, not a fix, and it is both flaky under load
      and wasted time when the tunnel is ready sooner. This branch has no sleep at all, so the race
      is live here.

      **Do not reach for `depends_on` — it was already tried and reverted on `main`** (`c56eda9`,
      reverted by `9077f34`). A detached step never reports completion, so depending on it does not
      express "wait until it is listening". A readiness poll is the only thing that actually works.

      *The hardcoded port.* `tests/e2e/playwright.config.js` pins
      `baseURL: "http://localhost:8080"`, which collides with anything already on 8080 and forces
      developers to edit a tracked file to test against a different address.

      Final shape:

  - Both steps derive the same local port from `DRONE_BUILD_NUMBER`, which Drone injects
    everywhere: `PORT=$((20000 + DRONE_BUILD_NUMBER % 20000))`. No hardcoded 8080, no collisions
    between concurrent builds, and nothing has to be communicated from one step to the other.
  - `gpm-port-forward` leaves kubectl's output on stdout so it lands in that step's log in the
    Drone UI.
  - `ui-tests` polls `http://127.0.0.1:<port>/health` until it answers, bounded at 90 attempts,
    reporting progress every 15 and pointing at the port-forward step's log on failure. This
    replaces the sleep with a real readiness check.
  - Export the resolved address and have Playwright read it:
    `baseURL: process.env.GPM_BASE_URL ?? "http://localhost:8080"`.

      Both steps already run with `network_mode: host`, which is what lets the second container
      reach the first one's `127.0.0.1:<port>` — worth keeping in mind if that ever changes.

      **Took two attempts.** The first version had kubectl pick the port (`:80`) and parsed it back
      out of `port-forward.log`. That failed (build #3499): the loop ran its full 60 s without ever
      getting a usable port, even though the log contained
      `Forwarding from 127.0.0.1:42649` by the end. The redirect also swallowed kubectl's output, so
      the port-forward step's own log was empty and there was no way to tell when the tunnel came up.

      The second version (build #3500, green) drops the parsing entirely — both steps derive the
      port from `DRONE_BUILD_NUMBER`, which is injected into every step.

      A third pass then removed the log file and the `tee` along with it. Once neither step needs to
      learn anything from the other, kubectl can just write to stdout and Drone shows it in the
      port-forward step's own log. That also removes the last cross-step file dependency, which is
      what broke the first attempt.

      That green run settles the diagnosis: **GPM answered after 2 attempts**, so the tunnel is up in
      about 2 s. Timing was never the problem, which means the log-parsing path was: reading the
      port back out of that file did not work from the other step, whatever the precise mechanism.
      Deriving the port removes the question. `ui-tests` went from 34 s (fixed port, no wait) to 7 s.

      The wait script was extracted from `.drone.yml` and exercised against a stand-in server before
      each push: nothing listening (fails cleanly, diagnostics fire, no `set -e` abort), and a server
      appearing late (detected and passed through as `GPM_BASE_URL`).

      Two Drone/shell traps worth remembering for anything else written in `.drone.yml`:

  - **Braced variables in commands are interpolated by Drone**, not the shell, and anything that is
    not a Drone variable is blanked out. `${i}` silently became empty; `$i` is correct. `${DRONE_*}`
    in a *YAML value* is the opposite case — there it is the right mechanism, which is why
    `LOAD_IMAGE` and `CLUSTER_NAME` use it.
  - **`set -e` is on**, so a bare `[ cond ] && cmd` aborts the step whenever the condition is false.
    The periodic-diagnostics check is a full `if` block for that reason.

      Side benefit: with `baseURL` coming from the environment, pointing the tests at another host
      is now `GPM_BASE_URL=http://192.168.2.1:8080 yarn test` instead of editing a tracked file.

- [ ] 🟡 **Port the same fix back to `main`** to retire its `sleep 5` (`71d71be`). The mechanism
      carries over unchanged; only the step names differ. Worth doing: the tunnel is ready in ~2 s,
      so the fixed sleep is both slower than needed and still a guess.

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

- [x] 🟠 **Seed `mise.toml`** — created at the repo root pinning `go = "1.26.5"` (matching the
      `toolchain` line in `go.mod`) and `golangci-lint = "2.12.2"` (current release, same pin as
      `furyctl`), with `lint` and `lint-fix` tasks. No `.golangci.yml`: the five default linters are
      a sane start. If it needs tightening later the SIGHUP convention is `.rules/.golangci.yml`, as
      in `furyctl`. No license header — `addlicense` has no comment style for `.toml` and skips it.
- [x] 🟠 **Migration done** — `4ed4c46`, green on build **#3458**. Six steps moved to
      `quay.io/sighup/mise:v2026.6.14`: `license/check`, `qa/render`, and all four e2e steps
      (`kind`, `tests`, `gpm-port-forward`, `kind-destroy`). Both `e2e-testing` references and every
      hand-rolled `wget` install are gone. Unchanged as planned: buildx, both Playwright steps,
      pluto, and the release plugins.

      Tools were pinned at the versions the retired image shipped (`kustomize 5.6.0`, `helm 3.12.0`,
      `bats 1.1.0`) so the migration was a refactor, not a behaviour change. The one deliberate
      exception is `kubectl 1.36.1`, which closes the skew documented above.

      **The unproven part worked:** `kind load docker-image` from the mise image. No sibling repo
      did this, so the docker CLI's presence in that image was inferred rather than demonstrated —
      it is now demonstrated.

      Wall-clock improved noticeably, mostly from dropping image pulls: `qa` 1:05 → 0:06, `e2e`
      3:18 → 2:30, `kind` 0:34 → 0:18, `ui-tests` 0:34 → 0:07.

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

- [x] `github_token` secret on this repo in Drone (lowercase, matching this repo's existing
      `quay_username` / `quay_password` convention rather than furyctl's uppercase).
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

- [x] 🟠 **Go** — `main_test.go` covers `kubeAPIErrorAnswer` (the TLS detection: each certificate
      error type, plus the real `url.Error` → `tls.CertificateVerificationError` →
      `x509.UnknownAuthorityError` nesting client-go actually produces, and non-TLS errors that must
      pass through untouched) and the two client-free handlers, `getHealth` and `getAuth`. The
      `getAuth` test asserts `auth_enabled: false` **on purpose** — it should fail and be rewritten
      when OIDC lands.
- [x] 🟠 **Frontend** — `web-client/src/utils.test.tsx` covers `autoLink` (no-link text, single and
      multiple URLs, `target="_blank"` + `rel="noreferrer"`) and `scrollToElement` (highlight added
      then removed on the timer, smooth vs instant, ids containing dots and colons, missing element
      is a no-op). Chosen because `utils.tsx` is the only module with real logic that does not
      import EUI, so the tests run without touching the broken transform.
- [x] 🟠 **Tasks** — `mise run test:unit:go`, `mise run test:unit:js`, and `test:unit` for both. The
      Go task filters `node_modules` out of the package list, since `./...` otherwise picks up the
      Go sources vendored inside the `flatted` npm package.

Two things the tests found immediately:

- **A real bug in `autoLink`**: it rendered an array of `<a>` elements with no `key` prop, so React
  logged a warning on every violation message containing a link. Fixed.
- A nil-pointer panic in my own first fixture — `x509.HostnameError.Error()` dereferences its
  `Certificate` field. Worth knowing if these tests get extended.

- [x] 🟠 **Wired into CI** — `unit-tests-go` and `unit-tests-js` in the `qa` pipeline, kept as two
      steps so a failure names the suite. Both depend only on `clone`, so they run alongside
      `render`. This needed `node = "24"` (the LTS line, matching the Dockerfile's `node:lts-alpine`)
      and `yarn = "1.22.22"` (v1, matching the committed lockfile) in `mise.toml`, plus a
      `yarn install --frozen-lockfile` inside `test:unit:js` so the task works from a clean
      checkout — CI has no `node_modules`.

      Verified by cloning the repo to a scratch directory and running the task there: install from
      scratch plus tests takes about 25 s.

> ⚠️ **The `App.test.tsx` deletion is load-bearing.** The fresh-checkout run showed it: with that
> file still present, `unit-tests-js` fails on the EUI transform error and takes the `qa` pipeline —
> and therefore `build`, `e2e` and `release` — down with it. It must be staged in the same commit as
> the new tests.

Still open:
- [ ] 🟡 **Jest cannot transform `@elastic/eui`**, so no component or page can be tested. Fixing it
      needs a `transformIgnorePatterns` override in the CRA Jest config. Until then, frontend tests
      are limited to EUI-free modules.
- [ ] 🟡 YAML and shell scripts are still unlinted in CI. Go and frontend formatting are covered —
      that residue is what restoring or replacing the superlint step would buy.
- [x] 🟠 **`govulncheck` added to the `qa` pipeline**, pinned at `1.6.0` in `mise.toml` with a
      `vulncheck` task. It reports on *reachable* call paths rather than the whole module graph, so
      it stays quiet where Dependabot would not: the current scan finds **0 reachable
      vulnerabilities** and one unreachable, unfixable module-level advisory (GO-2026-5932,
      `x/crypto/openpgp` unmaintained), which correctly does not fail the build. Nothing was
      scanning for known CVEs before — the five golangci-lint linters do not.
- [x] 🟢 **Snyk triggers removed.** Two `refs/heads/snyk-**` refs were left over in the e2e include
      and release exclude lists; Snyk is not in use.
- [x] 🟠 **The release pipeline's branch exclusion was misspelled** `featuer/go-backend`, so it never
      matched and the pipeline has been running on this branch all along. The `unstable`, `go` and
      commit-SHA tags it would have suppressed turned out to be wanted for testing, so the exclusion
      was removed rather than corrected, with a comment recording why — otherwise the next reader
      sees an unguarded `refs/heads/**` and re-adds it. `latest` and the version tag still only move
      on a tag.

### Frontend formatting

- [x] 🟠 **Prettier adopted**, pinned at `3.9.6` in `mise.toml`, with `.prettierrc.yaml` (Prettier's
      own defaults written out so editors and CI cannot disagree) and `.prettierignore`. Tasks:
      `mise run format` and `mise run format:check`, the latter wired into the `qa` pipeline as
      `format-check`. Reformatted 26 files, +338/−212, covering `web-client/src` and `tests/e2e`.
      Verified afterwards: `yarn build` compiles with the same pre-existing warnings, the unit tests
      pass, and the licence headers survived.

> **This diverges from `main` rather than converging with it.** `main` is not Prettier-clean either
> — 14 of its files would also be reformatted — so adopting Prettier here moves the two further
> apart on formatting. That is an acceptable trade because `main`'s frontend source barely moves:
> **6 source commits in the last year against 79 to `package.json`/`yarn.lock`**. The dependency
> sync, which is the part that actually happens, stays a verbatim copy because `yarn.lock` and
> `package.json` are outside Prettier's scope. When `main` does change a source file, port the
> change and re-run `mise run format` — those ports are manual reviews anyway.

- [ ] 🟡 **Consider adopting Prettier on `main` too.** It would make the two branches converge again
      and is a prerequisite if source-level syncing ever becomes frequent.
- [ ] 🟢 **Stylelint was evaluated and rejected.** It cannot resolve a shareable config
      (`stylelint-config-standard-scss`) when installed through mise's npm backend, since each
      package lands in its own isolated store. Adding it to `web-client/package.json` instead would
      break the byte-for-byte match with `main` that keeps the dependency sync a plain copy. Its only
      concrete gain here was rewriting one media query to range syntax, and pointing
      `stylelint-config-standard-scss` at stylesheets that deliberately override EUI would likely
      produce a lot of unrelated noise. Revisit if the frontend ever grows its own tooling.

### Go linting

- [x] 🟠 **Wired into CI** — a `lint-go` step in the `qa` pipeline runs `mise run lint`. It replaces
      the Go half of the disabled superlint, which is now noted as such in the `.drone.yml` comment.
      Verified to pass both with and without `node_modules` present, since CI's checkout has none and
      the `.rules/.golangci.yml` exclusions only matter locally.

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
- [x] 🟠 `README.md`: "Running behind a reverse proxy on a subpath" added, placed under
      `## Configuration` to match where `main` moved it in `316e297`, and extended with the
      prefix-stripping requirement `main` leaves implicit. Everything else in this branch's README
      is *ahead* of `main` (better tables, mutations and events documented, updated screenshots) —
      it was not overwritten wholesale.
- [x] 🟠 **Release notes started** — `docs/releases/v2.0.0.md`, kept as a living draft and updated as
      work lands rather than written at release time. **User-facing changes only, in plain English —
      it is not a changelog.** CI churn, refactors, toolchain and dependency bumps stay out; the
      dependency work gets one summary line.

      **Format follows `distribution` and `furyctl`, not GPM's older notes**: an
      `# <Product> release v<version>` title, a "maintained with ❤️ by the team
      [SIGHUP by ReeVo]" line, a short paragraph summarising the release, then
      `## New features 🌟`, `## Bug fixes 🐞`, `## Breaking changes 💔` and `## Upgrade procedure`.
      Sections with nothing in them are dropped rather than left empty. Entries in those repos are
      prefixed with `[[#NNN](pr-url)]` — add those once the work has PR numbers. Covers so far: the Go rewrite, mutations and events
      views, the API server address in the violations report, `GPM_LISTEN_ADDRESS`, the smaller
      image; and under breaking changes: no authentication, the removed `GPM_SECRET_KEY` /
      `GPM_PREFERRED_URL_SCHEME`, the kubeconfig path change, the new log-level names and the HPA
      removal.
- [x] 🟡 **Release-notes filename settled: `v2.0.0.md`, no rename.** One file covers the release
      candidate and the final release, which is what `v1.0.0.md` (titled `# v1.0.0-rc0`) did. The
      draft banner now says 2.0.0 ships first as `v2.0.0-rc0`, and the intro tells readers to try
      the candidate on a non-production cluster and warns that it has no authentication.
- [x] 🟡 `docs/releases/`: this branch stops at `v1.0.3`; `main` has `v1.0.4` → `v1.1.2` (14 files),  **— done: copied across, v0.1 to v2.0.0 continuous.**
      with **v1.1.2 now released**. Decide whether the 2.x line inherits that history or starts
      clean at 2.0.0. Inheriting keeps the record of what 1.x users are upgrading from, which
      matters more than usual given how much 2.0 changes.
- [ ] 🟠 **Keep `v2.0.0.md` current** as work lands. Held to so far: `GPM_SKIP_TLS_VERIFY`,
      `PUBLIC_URL` and the chart probe / `secretRef` options all have entries. Still to come —
      authentication, which would remove the biggest breaking change if it ships before the release.
- [ ] 🟢 `MAINTENANCE.md`: small delta (8 insertions / 4 deletions) — worth eyeballing.

---

## 5. Toolchain and dependency maintenance (not a port)

Not backported from `main` — `main` is Python, so it has no equivalent. Recorded here because it
lands in the same batch of work.

- [x] 🟠 **Go 1.24 → 1.26.** `go.mod` (`go 1.26.0` / `toolchain go1.26.5`), the `Dockerfile` backend
      stage (`golang:1.24` → `golang:1.26`) and the Drone license step (`golang:1.20` → `golang:1.26`).
      `go build ./...` and `go vet ./...` pass; `go.sum` was unaffected.
- [x] 🟠 **`go get -u ./...` + `go mod tidy`.** Direct deps: `k8s.io/client-go` and
      `k8s.io/apimachinery` v0.33.2 → **v0.36.3**, `echo-contrib` v0.17.4 → **v0.50.1**, `echo/v4`
      v4.13.4 → v4.15.4, `viper` v1.20.1 → v1.21.0, `golang.org/x/exp` refreshed. No source changes
      were needed. Structural churn in the indirect set — `sigs.k8s.io/structured-merge-diff/v6`
      now sits beside v4, and `go.yaml.in/yaml` v2+v3 appeared — is the upstream Kubernetes yaml-fork
      migration, not something introduced here.

> **client-go skew.** Bumping client-go to v0.36 is what forced the kind cluster bump in section 3.
> client-go supports roughly ±1 minor against the API server; against the old `v1.24.7` cluster it
> was ~12 minors out, and even `main`'s `v1.33.0` would have left a 3-minor gap. Keep
> `CLUSTER_VERSION` tracking the client-go minor when either one moves.

- [x] 🔴 **Verified against a real cluster.** Build #3457 is green end to end against kind
      `v1.36.1`: the `tests` step's API assertions (`/api/v1/configs`, `constrainttemplates`,
      `constraints`, `mutations`, `events`) and the Playwright UI tests both pass. That exercises
      client-go v0.36 against a 1.36 API server through every handler, which is the check that was
      outstanding.

      Two predictions that did **not** come true, worth recording so they are not re-raised:
      Playwright baselines did **not** need regenerating, and `tests.sh`'s hard-coded
      `[[ "$output" -eq 2 ]]` violation count still holds on the newer Gatekeeper.
- [ ] 🟡 `addlicense` is pinned at `@v1.1.1` in the Drone license step; `main` uses `@latest`. The
      pin is arguably better practice — left as is, noted so it is a decision and not an oversight.

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
- [x] 🟢 `chart/templates/NOTES.txt` now reads `https://docs.sighup.io | https://sighup.io` — the
      docs-plus-company pairing `main` introduced, with the domain fixed. This closes the
      `NOTES.txt` item in section 2; `main`'s version was not portable as written.
- [ ] 🟡 **`main` still has `docs.sighup.com` in `chart/templates/NOTES.txt`** — worth fixing there
      too, since 1.x is the released line.

**Corrected:** `port check della chart con pluto` and `Deprecated APIs Check in CI` are marked done
in the notes, and the port *was* done — but the pipeline is commented out. See the 🔴 item in
section 3.

**Go-backend backlog — not porting work, recorded so it is not lost:**

- [x] 🟠 ~~**Events namespace configurability.**~~ — done. `GPM_EVENTS_NAMESPACE` selects the
      namespace; empty keeps the cluster-wide list. It takes priority over `?namespace=`, because
      the deployment's RBAC is cut to the configured namespace and honoring a wider request would
      only produce a 403. Four tests drive the handler against a stand-in API server and assert the
      path it requests — a namespaced list and a cluster-wide one are indistinguishable from the
      response, so the request path is the only real evidence. All mutation-checked.

      **Chart bug found while doing this:** the chart's `ClusterRole` was missing
      `mutations.gatekeeper.sh` **and** `events` entirely, so a Helm-installed GPM could not serve
      either view — both of them advertised as new in the release notes. Added, along with
      `config.eventsSource`, which the chart also never set.

      With `config.eventsNamespace` set, the chart drops `events` from the `ClusterRole` and emits
      a `Role` plus `RoleBinding` in that namespace instead. Both renders verified with
      `helm template`.

      README gained an "Events and RBAC" section, written in Simplified English. Release notes
      gained the namespace entry and the chart fix.
- [ ] 🟡 Use `unstructured.NestedString` (and friends) instead of type assertions when reading
      constraint fields.
- [ ] 🟡 HTTP proxy support.
- [ ] 🟡 Events: dynamic update via watch, plus a flag to toggle watch vs static list.
- [x] 🟡 ~~**Events view: schema assumption is untested.**~~ — done. The e2e suite now turns on
      `--emit-admission-events` (kustomize patch on the gatekeeper controller-manager), triggers a
      denied Pod, and asserts a `gatekeeper-webhook` event reaches `/api/v1/events`. Validated on a
      fresh kind cluster with module-policy v1.17.0: all pass with the flag; with the flag off GPM's
      events endpoint returns `null` and the assertion fails. So the core-v1 `source.component`
      path is now exercised end to end, and a future move to `events.k8s.io/v1` would fail the
      suite instead of silently blanking the view.

      Learned along the way: GPM's `getEvents` returns `null` (a nil-slice pointer) when empty,
      unlike `getMutations` which guards with `[]`. The old `grep 'null'` in the events container
      was therefore asserting the *empty* state, not stale. Two test-authoring traps hit and fixed:
      the trigger raced webhook readiness (now `loop_it` until denied, delete-first so retries stay
      clean), and my mutation checks kept matching a `kustomize edit`-reserialized layout. Also
      bumped module-policy v1.15.0 -> v1.17.0.

- [ ] 🟡 Constraints report: confirm both the API server hostname (`3716295`) **and** the selected
      context are shown.
- [ ] 🟢 Review resource requests; re-shoot and shrink screenshots.

**Architecture review (the "Claudio/Alessio" list):**

- [x] 🟠 ~~Split into packages~~ — done as files, not packages. `main.go` went from 675 lines to
      283 (server type, settings, `main`), with `handlers.go` and `static.go` alongside
      `auth.go`, `clients.go` and `basepath.go`. Verified as a pure move: every non-import line
      appears exactly once across the three files.

      **Packages skipped.** The reason recorded here — handlers reading package-level globals, so
      only the two client-free ones were testable — went away with the client registry. There are
      no globals and every handler has tests. For a single binary with no importers, packages buy
      exported identifiers and a large diff. Revisit if a second binary or an external importer
      appears; the per-package `*viper.Viper` goes with it.

      A conciseness review of the split then caught two documentation errors about
      `GPM_LOG_LEVEL`, both now fixed and pinned by tests: `OFF` was in the README from May 2023
      with no code behind it (and Python never took it either), and the release notes claimed the
      Python level names were all invalid when `DEBUG`, `INFO`, `WARN` and `ERROR` still work —
      only `WARNING`, `CRITICAL`, `FATAL` and `NOTSET` stopped.

      Left for its own change: `static.go` duplicates `middleware.StaticWithConfig{HTML5: true}`,
      but those lines are the path-containment fix for the arbitrary-file-read. Needs Echo's
      root-jail verified and the traversal tests moved to the router first.
- [x] 🟠 ~~Package-level globals (`config`, `startingConfig`, `clientset`, `discoveryClient`)~~ —
      done. There are no package-level variables left. `clients.go` holds a `clientRegistry` that
      builds one set of clients per kubeconfig context and caches it; the handlers are methods on a
      `server` that owns the registry and resolve their cluster with `s.clientsFor(c)`.

      This was a correctness fix, not only tidying. `switchKubernetesContext` reassigned the shared
      globals, so in multi-cluster mode a request for one cluster changed which cluster another
      user's in-flight request was reading, and whichever context was switched to last became the
      one that context-less requests used. Python 1.x never had this: it built a client per
      request. Caching matters too — each set owns an `http.Transport` that nothing closes.

      `clients_test.go` covers it against a two-cluster kubeconfig: per-context resolution, no
      bleed between contexts, caching, unknown-context rejection, `-race` under concurrent
      resolution, and the `:context` route wiring. All six were checked against a deliberately
      broken `clients.go`.

      Not user-facing (the bug never shipped), so no release-note entry.
- [ ] 🟡 Two `context.TODO()` calls remain; thread a real context through.

---

## 9. Simplified English pass over user-visible text (do last)

- [ ] 🟡 **Review log messages, error strings, comments and docs against ASD-STE100.** Much of this
      text was written at speed across the port and has never been read as a whole. It matters more
      than usual here: the `ErrorAnswer` `error`/`action`/`description` fields are rendered verbatim
      on the error page, so they are product copy, not developer notes.

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

- [x] 🟢 ~~Implement the above~~ — done, but not as sketched. Three bats tests appended to
      `tests/tests.sh`: `helm install` into its own namespace, `kubectl auth can-i` for every
      resource the views read, `helm uninstall`. No pipeline change and no new tool, because
      `mise run e2e` already runs that file and `helm` is already pinned.

      **`ct` skipped.** It brings a python dependency, `chart/ci/*-values.yaml` files, and
      target-branch change detection that is pointless when the pipeline runs anyway. Its lint is
      covered by `helm lint`, and the version-increment check would fail every non-release commit.

      **Why `main` removed its chart action:** the maintainer removed it because it was GitHub
      Actions and this org runs Drone. Nothing was wrong with chart-testing itself. Question closed.

      `can-i` answers "no" for a resource whose CRD is absent, whatever the RBAC says. These tests
      run after the Deploy test that installs Gatekeeper, so it holds — a reorder fails loudly with
      kubectl's "server doesn't have a resource type" warning in the output, not silently.

      Not done: the k8s version matrix and `paths: chart/**` triggers. Worth adding when the chart
      claims support for a version the e2e cluster does not run.

---

## 13. Release checklist — deliberately deferred

The chart version was **intentionally not bumped** alongside the HPA removal. Version bumps happen
when the release is planned, per `MAINTENANCE.md`. Everything that has accumulated an unreleased
debt is collected here so it is not rediscovered at release time.

- [x] 🔴 ~~**Bump `chart/Chart.yaml` `version`.**~~ — `0.6.0` → **`0.18.0`**.

      **Corrected.** The first attempt used `0.7.0`, reasoning only from this branch's stale
      `0.6.0`. The published chart is at **`0.17.0`** — `main` moved eleven minors ahead while this
      branch sat at `0.6.0` — and `gatekeeper-policy-manager-0.7.0` already exists as a release
      tag. `0.7.0` was therefore both a downgrade and a collision: Helm resolves the highest semver,
      so nobody would ever have received it. Found by testing a dynamic badge against the real tag
      list, not by reading the chart.

      The lesson for `appVersion` vs `version`: §2 says to keep `appVersion` on this branch's value
      and not take `main`'s, which is right, and it does not apply to `version`. The chart version
      is a single line shared with `main` and has to continue from what is published.
- [x] 🔴 **Pre-release scheme decided: release candidates, not alphas.** This resolves the
      bumpversion blocker at no cost — `parse` is already
      `(?P<major>\d+)\.(?P<minor>\d+)\.(?P<patch>\d+)(\-rc(?P<rc>\d+))?` and `serialize` already
      emits `{major}.{minor}.{patch}-rc{rc}`, so the rc scheme works as-is. It was only ever `alpha`
      that the config could not express. The Drone triggers agree: `refs/tags/v**-rc**` selects the
      prerelease GitHub release. `docs/releases/v1.0.0.md`, titled `# v1.0.0-rc0`, is the precedent.
- [x] 🟠 ~~**One-time rename to `2.0.0-rc.0`.**~~ — done across `.bumpversion.cfg`, `README.md`,
      `kustomization.yaml`, `Footer/Component.tsx`, `chart/Chart.yaml` and `chart/values.yaml`.
      Verified with a real `bump2version --dry-run`: `rc.0` → `rc.1` parses, and finalising to
      `2.0.0` works.

      **Two files the item did not list.** `main.go` hardcoded the version in its startup log and
      was not a bumpversion target, so every bump would have left it lying — now added. And the
      shields.io badge escapes a literal dash as `--`, so `GPM-v2.0.0--alpha1-blue` never matched
      bumpversion's plain search; it was fixed by hand here and **will rot again on the next bump**.
      **Done:** both badges now read the real version. GPM uses
      `github/v/tag?filter=v*&sort=semver`, which the chart-releaser tags cannot win; the chart uses
      a `dynamic/yaml` badge reading `chart/Chart.yaml` from the default branch. Both were checked
      against the live endpoint, and the chart one is what exposed the `0.17.0` mismatch.
- [x] 🟠 **`parse`/`serialize` updated for the dotted `rc.N` form.** SemVer's own examples use
      `1.0.0-rc.1`, and the dot is not cosmetic: dot-separated numeric identifiers compare
      numerically, so `rc.9 < rc.10`. Undotted, `rc10` sorts *before* `rc9` as a string, which
      breaks ordering past nine candidates. `parse` is now
      `(?P<major>\d+)\.(?P<minor>\d+)\.(?P<patch>\d+)(\-rc\.(?P<rc>\d+))?` with a matching
      `serialize`. This supersedes `v1.0.0-rc0`, which used the undotted form.

      Verified with a real `bump2version --dry-run` against the new config, not just by reading the
      regex: `2.0.0-rc.0` → `rc.1` (part bump), `rc.1` → `2.0.0` (finalise), `2.0.0` → `2.0.1` and
      → `2.1.0`, and a plain non-rc version still parses. The finalise step needs the explicit
      `--new-version` form, which is what `MAINTENANCE.md` already prescribes.

- [x] 🔴 **Fixed: an RC tag would have published a Helm chart release.** `release-helm-chart`
      triggers on `refs/tags/**` and only excluded the chart-releaser's own
      `gatekeeper-policy-manager-*` tags, so `v2.0.0-rc.0` would have shipped a chart release.
      `main` guards against this and this branch had lost the guard. Added
      `refs/tags/v**-rc**` to the exclude list — broader than `main`'s `v**-rc.**`, so it catches
      both spellings.

      The other two are already correct: `publish-prerelease` includes `v**-rc**`, and
      `publish-stable` excludes it while including `v**`, so an RC tag produces a GitHub prerelease
      and not a stable one.
- [x] 🟠 ~~**Regenerate `chart/README.md` with frigate.**~~ — done, and it had drifted far further
      than the four `autoscaling.*` rows: the committed copy documented `image.tag: "v1.0.7"` and
      none of the OIDC, probe, `secretRef`, `eventsSource`, `eventsNamespace` or `multiCluster`
      values. 155 changed lines. frigate is now pinned in `mise.toml` with a `chart-readme` task,
      so it no longer needs an ad-hoc `uvx`.
- [x] 🟡 ~~**Who owns the `chart/README.md` heading.**~~ — frigate does. The bumpversion entry was
      deliberately not added: frigate rewrites the whole file from `Chart.yaml` at release, so a
      second writer could only disagree with it.
- [x] 🟠 ~~**Chart release notes call out the HPA removal.**~~ — already covered in
      `docs/releases/v2.0.0.md`, both in Breaking changes and in the upgrade procedure.
- [x] 🟡 App release notes: this branch's `docs/releases/` stops at `v1.0.3` — see section 4 for the  **— done: copied across, v0.1 to v2.0.0 continuous.**
      open question of whether 2.x restarts the notes or inherits `main`'s history.

Tag conventions worth re-reading in `MAINTENANCE.md` before tagging:

- `gatekeeper-policy-manager-<version>` is reserved for helm/chart-releaser — never create these by
  hand; the Drone triggers explicitly exclude them.
- `helm-chart-<version>` releases the chart on its own, but the chart pipeline depends on a
  successful GPM release, so that dependency has to be relaxed for a chart-only release.

---

## 14. Branch switchover

A real merge is out: 1245 commits apart with almost no shared files. Renaming `main` to
`release-v1.x` and this branch to `main` also loses more than it looks — every clone's `main` goes
stale, open PRs retarget themselves, and the default branch changes under branch protection.

**Chosen: an `ours` merge.** `-s ours` records `main` as a second parent and keeps this tree
wholesale, so there is nothing to resolve. `main` then fast-forwards onto it, keeps its name, keeps
its protection rules, and keeps 1.x reachable as an ancestor.

```bash
git branch release-v1.x origin/main && git push -u origin release-v1.x
git checkout feature/go-backend
git merge -s ours origin/main -m "2.0 replaces the Python backend; 1.x continues on release-v1.x"
git checkout main && git merge --ff-only feature/go-backend && git push origin main
```

Direction matters: run it **from** the Go branch merging `main` **in**. Reversed, it keeps the
Python tree and throws away 2.0.

- [x] 🟠 ~~Copy the files only `main` had~~ — 15 release notes (`v1.0.4`–`v1.1.2`, so
      `docs/releases` is continuous again), the issue and pull-request templates (GitHub reads
      those from the default branch), and `renovate.json`. The numbered screenshots `01`–`08` were
      left behind: only `main`'s README uses them, and that README does not survive.
- [ ] 🟠 Remove `refs/heads/feature/go-backend` from the `.drone.yml` triggers **in the switch
      commit**. Doing it earlier stops CI on this branch; `refs/heads/main` covers it afterwards.
- [x] 🟡 ~~Renovate vs Dependabot~~ — Dependabot only. `renovate.json` deleted as its own decision,
      after the copy, so the branch switch was not what removed it.
- [ ] 🟡 After the switch, `git describe` on `main` finds a 1.x tag until `v2.0.0-rc.0` exists, so
      tag first.

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

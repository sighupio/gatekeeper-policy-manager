<!--
Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
Use of this source code is governed by a BSD-style
license that can be found in the LICENSE file.
-->

# Drop React: migration plan

This plan describes how to replace the React single-page application (SPA) with a server-rendered
UI. The backend renders HTML with the Go standard library (`html/template`) and adds a light touch
of interactivity with Alpine.js. The plan builds on the feasibility evaluation in `PORTING.md`
(find the item "drop React (and the SPA) entirely").

The goal is one language (Go), no Node build, and no Elastic UI (EUI). This removes almost all of
the Dependabot churn and the separate frontend build stage.

## Status

The cutover is done. Every view is server-rendered at its real path, the React SPA is gone, and the
JSON API is removed. What has landed:

- all six content views (Constraint Templates, Constraints, Mutations, Events, Configurations) plus
  Home and the error/404/signed-out pages, server-rendered;
- a fresh, minimal house style (not a clone of EUI or Fury), light and dark;
- Alpine.js vendored as a local static asset (no CDN), for the context switcher, the theme toggle
  and the searchable/sortable/paginated violations table;
- server-side syntax highlighting with `chroma`;
- the views at their real paths (`/`, `/constraints`, …, each with an optional `/:context`), assets
  at `/static`, and a global error handler that renders the SSR 404 and error pages;
- the `/api/v1` JSON API removed; the violations report served from `/constraints?report=html`;
- `web-client/` and the Docker frontend build deleted; the report template embedded.

### Decisions (resolved)

- **Default landing route:** `/` serves Home.
- **Static asset prefix:** `/static`.
- **`/api/v1/*`:** removed (v2.0.0 is the major release to drop it in).
- **Constraints table:** full parity (search, all-column sort, pagination).

### Remaining before merge

1. **Content Security Policy — implemented, needs live verification.** The inline scripts are
   externalized (`theme.js`, `violations-table.js`) and the CSP is set in `main.go`. It keeps
   `script-src`/`style-src` same-origin with two documented relaxations: `script-src 'unsafe-eval'`
   (Alpine evaluates `x-` expressions with `Function`; removing it needs Alpine's CSP build) and
   `style-src 'unsafe-inline'` (the self-contained printable report). Still to do: load every view
   against a live cluster with the console open and confirm there are no CSP violations, since the
   header is not testable from a static file. If anything breaks, flip to `CSPReportOnly` while it
   is sorted out.
2. **Update the README screenshots.** They still show the old React UI.
3. **Update the Playwright e2e baselines** in `tests/e2e` for the server-rendered UI (routes and
   selectors changed).
4. **Security and ponytail reviews** (see below), scoped to the whole `feat/drop-react` diff.

## POC files

| File | Purpose |
| --- | --- |
| `ssr.go` | The SSR renderer, the layout view model, the Configurations handler, and the route registration. |
| `templates/ssr/layout.html.gotpl` | The base layout: `<head>`, top nav, context switcher, theme toggle, footer, and a `content` block. |
| `templates/ssr/configurations.html.gotpl` | The Configurations view. It fills the `content` block. |
| `static/ssr/app.css` | The house style. Neutral palette, one accent, light and dark. |
| `static/ssr/alpine.min.js` | Alpine.js v3.14.9, vendored (official cdnjs build). |

The templates and the static assets are embedded in the binary with `go:embed`. This SSR path does
not depend on the working directory. (The existing report template is still read from disk by
`static.go`. See "Retirement" below.)

## Target structure

Keep the flat package layout that GPM already uses. Do not add sub-packages for the POC scope.

```
templates/ssr/
  layout.html.gotpl          base layout (one "content" block)
  configurations.html.gotpl  one file per view; each defines "content"
  constraints.html.gotpl
  ...
static/ssr/
  app.css
  alpine.min.js
ssr.go                       renderer + handlers + route registration
```

### Base layout and template sets

`html/template` cannot hold all the pages in one flat template set, because every page defines the
`content` block and the definitions collide. The POC solves this the standard way: it clones the
layout once per page, then it parses that page's file into the clone (see `newSSRRenderer` in
`ssr.go`). To add a view, add one line to the `ssrPages` map and one handler.

Each page receives a `.Layout` value with the shared data (nav with the active item, the context
switcher options, the footer). A handler builds it with `ssrLayoutData`.

### Routing

Today `static.go` serves the SPA. `serveIndex` returns any real file, and it falls back to
`index.html` for every unknown path, so the React router can do client-side routing. This is the
`e.GET("/*", serveIndex)` catch-all in `main.go`.

The SSR routes are specific paths (`/ssr/...`). Echo matches a specific path before the `/*`
catch-all, so the SSR routes and the SPA fallback do not conflict. This is why the POC is safe to
add without a change to the existing routes.

At the end of the migration (big-bang on this branch), the routing changes as follows:

1. Register a real server route for each view, at its final path (for example `/constraints` and
   `/constraints/:context`), not under `/ssr`.
2. Serve the static assets (CSS and Alpine) from an embedded filesystem under a fixed prefix (for
   example `/static/*`).
3. Replace the `/*` catch-all with routes for the real views, a redirect from `/` to the default
   view, and a `404` handler. There is no more SPA fallback to `index.html`.

Keep the base-path helpers in `basepath.go`. They already put the reverse-proxy prefix back on the
links. The POC uses `browserPath` for every link and asset URL.

## View-by-view order

Do the small, static views first to prove the pattern. Do the one stateful view last.

1. **Configurations** — done in the POC. Sidebar list, `<details>` accordion, spec as YAML in a
   `<pre>`. Small.
2. **Mutations** — same shape as Configurations (list of objects, YAML spec). Small. Good second
   view, because it reuses the Configurations template almost unchanged.
3. **Configs/Home** — Home is static text and links. Small.
4. **Constraint Templates** — a list, each with its constraints and the rego source in a `<pre>`.
   Medium.
5. **Events** — a table of events, each row a `<details>`. Medium. Note that the events path has no
   end-to-end test and is alpha (see `handlers.go`).
6. **Constraints (violations table)** — the big one. Do it last. See below.

### The Constraints violations table is the big later piece

Every other view is native HTML. The Constraints view has the one genuinely stateful widget: a
per-constraint violations table. Today (EUI `EuiInMemoryTable`) it has, per table:

- incremental search;
- sort on every column;
- pagination.

There are several of these tables on one page (one per constraint). This must reach **full parity**.

The plan: render each table as a small **Alpine.js component over a JSON data island**. The handler
puts the violations for a constraint in a `<script type="application/json">` block. An Alpine
component reads that block once, then it does the search, the sort, and the pagination on the
client. The rows are already in the page, so there is no extra request.

Keep the components independent, one per constraint, so a large cluster does not build one huge
table. Consider a shared Alpine component definition (registered once) that each table element
instantiates with its own data island.

Do this view only after the pattern is proven on the simple views. It is the largest single piece
of work in the migration.

## Interactivity with Alpine.js

Alpine covers the small amount of interactivity GPM needs. The POC already wires it:

- **Context switcher** — a native `<select>`. On change, an Alpine handler sets
  `window.location.href` to the selected option's URL. This is a full page reload, which matches the
  behavior of the React app today (`navigate(0)`).
- **Accordions** — native `<details>`/`<summary>`. No JavaScript.
- **Theme toggle** — an Alpine component toggles `data-theme` on `<html>` and saves the choice in
  `localStorage`.

Alpine loads as a local `<script defer>`. There is no CDN and no build step.

## Look and feel

The house style is a fresh, minimal, custom design. It is not a clone of EUI or Fury. It uses:

- CSS custom properties for the palette, so a theme change is one place;
- one accent color, neutral surfaces, and system fonts;
- light by default, dark with `prefers-color-scheme`, and a `data-theme` attribute that overrides
  the media query for the manual toggle;
- responsive layout (the sidebar collapses, the brand text hides on small screens).

## Syntax highlighting (planned, not yet done)

The React app highlighted rego (`EuiCodeBlock language="rego"`) and JSON. The SSR code blocks are
currently plain `<pre class="code">`, so **rego, YAML and JSON are not highlighted yet**. This is a
cross-cutting enhancement — it touches every code block (Constraint Templates rego/libs, and the
YAML in every view), so do it once, uniformly, rather than per view.

Recommended approach, in order of fit with the server-rendered, future-strict-CSP design:

1. **Server-side (Go, `chroma`)** — highlight at template time into HTML, via a `highlight`
   template func wrapping `<pre class="code">`. No client JS, CSP-friendly, deterministic. YAML and
   JSON have chroma lexers; **confirm whether chroma ships a rego lexer** — if not, rego falls back
   to plain text (acceptable) or needs a small custom lexer. Theme via a CSS class map that reuses
   the existing palette tokens, with a dark variant.
2. **Client-side `Prism.js`** (vendored, like Alpine) — YAML and JSON are built in; rego needs a
   community grammar. Adds a client script and a theme stylesheet, and would need a CSP nonce later.

Lean toward (1) unless rego highlighting specifically must be pixel-faithful to the old EUI theme.
Either way it is one self-contained pass over the shared code-block rendering.

## Retirement (at the end of the big-bang change)

When every view is server-rendered, remove the React surface in one change:

1. Delete `web-client/`.
2. In `Dockerfile`, remove the `frontend` build stage (the `node`/`yarn` stage) and the
   `COPY --from=frontend /web-client/build/ ./static-content/` line. The image no longer needs Node.
3. In `static.go`, remove `serveIndex` and `serveSPAShell`, and in `main.go` remove the `/*`
   catch-all. Keep only the real view routes and the embedded static assets.
   - **Wire the global error handler here.** The SSR error and 404 pages already exist
     (`templates/ssr/{error,notfound}.html.gotpl`, rendered by `renderSSRError` / `renderSSRNotFound`
     in `ssr.go`; reachable now at the demo routes `/ssr/error` and `/ssr/notfound`). Install an
     `echo.HTTPErrorHandler` that, for an HTML request, renders `notfound` on 404 and `error` on 5xx,
     and returns JSON for `/api/*`. It is deferred until this step because today the `/*` SPA
     fallback intercepts unmatched routes, so a global handler would have to handle the SPA and the
     API together.
   - **Point the `/logout` local fallback at SSR.** `authenticator.logout` (in `auth.go`) currently
     ends the local-logout path with `serveSPAShell`, which renders the React logout confirmation.
     When `serveSPAShell` is removed, redirect to `/ssr/home` instead (or render a small SSR
     logout-confirmation page). The provider end-session path already redirects sensibly and needs
     no change.
4. Move the existing violations **report** template into the embedded set (the SSR renderer), then
   remove the `COPY templates ./templates` line and the disk-based `ParseGlob` in `static.go`. Until
   this step, keep the `templates` copy in the Dockerfile, because the report still loads from disk.
5. Remove the `static-content/` directory (the built SPA and its assets).
6. Add a strict Content Security Policy in `main.go`. EUI needed inline styles, so GPM ships no CSP
   today. The SSR UI has one small inline `<script>` (the pre-paint theme read in the layout). Move
   it to an external file or give it a nonce, then set the CSP.
7. Remove the now-dead `APP_ENV=development` CORS block and the `/api/v2/contexts/` route. The
   evaluation notes that `/api/v2/contexts/` is unused. Confirm before removal.
8. Decide the future of `/api/v1/*`. The SSR views read the Kubernetes data directly, so the JSON
   API is not needed by the UI. Keep it only if an external consumer needs it, and document that.

## Before merge: reviews (required)

Once every view is ported, the CSP is set, and the React surface is retired, do NOT merge to
`main` without both of these:

1. **Security review.** New attack surface to scrutinize: the `<script type="application/json">`
   data islands (the `toJSON` HTML-escaping is what keeps a hostile violation message from breaking
   out — verify it holds for every islanded field), the Alpine expressions (no untrusted data in
   `x-` attributes), the removal of the SPA fallback and any path-handling change, and the strict
   CSP once added. Run it scoped to the whole `feat/drop-react` diff.
2. **Ponytail review.** The SSR path added templates, CSS, an Alpine table, and Go models. Check
   for duplication across the view templates, dead CSS, over-built handlers, and anything the
   stdlib or a native element already covers.

## Open follow-ups (not blocking)

- **Alpine.js update path.** The vendored file pins v3.14.9. Decide how it is bumped: a `mise` task
  that re-downloads a pinned version, or a checked-in file updated by hand.
- **Vestigial allowlist entries.** `isAllowlistedPath` still lists the old Create React App asset
  paths (`/manifest.json`, `/logo192.png`, `/asset-manifest.json`, …). They are harmless but dead;
  trim them to `/static/*` and `/favicon.ico` when convenient.

// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
)

// Guards the new SSR views: newSSRRenderer parses every registered page (it panics on a parse
// error), and executing each new template against a realistic model catches field typos that
// build and vet cannot see because template execution happens at request time.
func TestSSRNewViewsRenderWithoutError(t *testing.T) {
	r := newSSRRenderer()

	ct := ssrConstraintTemplate{
		Name: "k8srequiredlabels", Kind: "K8sRequiredLabels", Created: "2026-01-01T00:00:00Z",
		Description: "Requires labels", Target: "admission.k8s.gatekeeper.sh",
		Rego: "package x", Libs: []string{"lib1"},
		Schema:        map[string]any{"labels": map[string]any{"type": "array"}},
		Constraints:   []string{"must-have-owner"},
		StatusCreated: true,
		Raw:           map[string]any{"kind": "ConstraintTemplate"},
	}
	ctData := map[string]any{"Layout": minimalLayout(), "Templates": []ssrConstraintTemplate{ct}}

	var buf bytes.Buffer
	if err := r.pages["constrainttemplates"].ExecuteTemplate(&buf, "layout", ctData); err != nil {
		t.Fatalf("constrainttemplates render failed: %v", err)
	}
	// No "created" badge: it was green on every template in any working cluster. The card says
	// something about its pods instead, and this fixture has none reporting.
	for _, want := range []string{
		"K8sRequiredLabels", "must-have-owner", "Parameters schema", "no pod has reported on it yet",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("constrainttemplates output missing %q", want)
		}
	}
	// The Rego is syntax-highlighted server-side, so "package x" is split across <span> tokens.
	// Strip the tags and assert the source is contiguous again, keeping the check meaningful.
	if stripped := stripHTMLTags(buf.String()); !strings.Contains(stripped, "package x") {
		t.Errorf("constrainttemplates output missing highlighted Rego %q", "package x")
	}

	cs := ssrConstraint{
		Name: "must-have-owner", Kind: "K8sRequiredLabels", Created: "2026-01-01T00:00:00Z",
		HasSpec: true, EnforcementAction: "deny", EnforcementMode: "deny",
		Match:           map[string]any{"kinds": []any{map[string]any{"kinds": []any{"Pod"}}}},
		Parameters:      map[string]any{"labels": []any{"owner"}},
		ViolationsKnown: true, TotalViolations: 3, ReturnedCount: 2, AuditLimited: true,
		Violations: []ssrConstraintViolation{
			{EnforcementAction: "deny", Kind: "Pod", Namespace: "default", Name: "nginx", Message: "missing owner"},
			{EnforcementAction: "deny", Kind: "Pod", Namespace: "kube-system", Name: "coredns", Message: "missing owner"},
		},
		AuditTimestamp: "2026-01-02T00:00:00Z",
		Pods:           []ssrConstraintPod{{ID: "audit-0", ObservedGeneration: "1", Enforced: true}},
		Raw:            map[string]any{"kind": "K8sRequiredLabels"},
	}
	csData := map[string]any{
		"Layout": minimalLayout(), "Constraints": []ssrConstraint{cs},
		"ReportURL": "/constraints/kind?report=html",
	}

	buf.Reset()
	if err := r.pages["constraints"].ExecuteTemplate(&buf, "layout", csData); err != nil {
		t.Fatalf("constraints render failed: %v", err)
	}
	for _, want := range []string{
		"must-have-owner", "K8sRequiredLabels", "deny mode",
		// The card id, its sidebar link and its data island are all Kind-prefixed; see constraintAnchor.
		"violationsTable('viol-K8sRequiredLabels--must-have-owner')",
		`id="K8sRequiredLabels--must-have-owner"`,
		"missing owner", "audit limit", "Download violations report", "Filter violations",
		// The filter + pager are gated to long lists, and the count label replaced "showing X of Y".
		`x-show="showControls"`, "countLabel",
		// Sidebar scroll-spy is wired via the shared layout script on every page.
		"sidebar-spy.js",
		// Per-violation shareable links (issue #1324): each row has a stable id and a copy control.
		"copyShareLink($data, row._id)", `class="vlink"`, `x-bind:id="row._id"`,
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("constraints output missing %q", want)
		}
	}

	ev := ssrEvent{
		Name: "e1", Reason: "FailedAdmission", Message: "denied", Count: "3",
		Action: "deny", ConstraintKind: "K8sRequiredLabels", ConstraintName: "must-have-owner",
		FirstTimestamp: "2026-01-01 00:00:00 UTC", LastTimestamp: "2026-01-02 00:00:00 UTC",
		ObjKind: "Pod", ObjName: "nginx", ObjNamespace: "default",
		SourceComponent: "gatekeeper-webhook",
	}
	evData := map[string]any{"Layout": minimalLayout(), "Events": []ssrEvent{ev}}

	buf.Reset()
	if err := r.pages["events"].ExecuteTemplate(&buf, "layout", evData); err != nil {
		t.Fatalf("events render failed: %v", err)
	}
	for _, want := range []string{"FailedAdmission", "K8sRequiredLabels", "must-have-owner", "denied", "alpha"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("events output missing %q", want)
		}
	}

	// Home reuses .Layout.Nav for its quick-nav cards, so give the layout one nav entry to exercise
	// that path. The dashboard itself is covered in dashboard_test.go.
	homeLayout := minimalLayout()
	homeLayout.Nav = []navLink{{Name: "Constraints", Href: "/constraints"}}
	homeData := map[string]any{"Layout": homeLayout, "Dashboard": dashboardData{TotalClusters: 1}}
	buf.Reset()
	if err := r.pages["home"].ExecuteTemplate(&buf, "layout", homeData); err != nil {
		t.Fatalf("home render failed: %v", err)
	}
	for _, want := range []string{"Overview", "Constraints", "/constraints"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("home output missing %q", want)
		}
	}

	errData := map[string]any{"Layout": minimalLayout(), "Err": ssrErrorView{
		Message: "Something went wrong", Action: "Try again", Description: "boom",
		LoginURL: "/login", BackURL: "/home",
	}}
	buf.Reset()
	if err := r.pages["error"].ExecuteTemplate(&buf, "layout", errData); err != nil {
		t.Fatalf("error render failed: %v", err)
	}
	for _, want := range []string{"Error", "Something went wrong", "Try again", "boom", "Log in", "Go back"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("error output missing %q", want)
		}
	}

	buf.Reset()
	if err := r.pages["notfound"].ExecuteTemplate(&buf, "layout", map[string]any{"Layout": minimalLayout()}); err != nil {
		t.Fatalf("notfound render failed: %v", err)
	}
	for _, want := range []string{"404", "Page not found", "Go to home"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("notfound output missing %q", want)
		}
	}
}

func minimalLayout() ssrLayout {
	return ssrLayout{Title: "t", Version: appVersion, AssetBase: "/static"}
}

// The pages served without a session -- the signed-out page and the error/404 pages -- must not
// render the context switcher, or an anonymous visitor could read the operator's context names off
// a multi-context kubeconfig. The Home dashboard is fleet-wide, so it drops the switcher too (a
// per-context choice would be a no-op there). ssrLayoutData proves the registry does have contexts,
// so the suppression is deliberate rather than an empty kubeconfig.
func TestPublicPagesHideTheContextSwitcher(t *testing.T) {
	useTestKubeconfig(t, twoClusterKubeconfig)
	registry, err := newClientRegistry()
	if err != nil {
		t.Fatalf("building the registry failed: %v", err)
	}
	s := &server{k8s: registry, ssr: newSSRRenderer()}

	layoutCtx := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	if !s.ssrLayoutData(layoutCtx, "constraints", "/constraints", "Constraints").HasContexts {
		t.Fatal("expected the two-context kubeconfig to produce a context switcher")
	}

	render := func(fn func(echo.Context) error) string {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		if err := fn(echo.New().NewContext(req, rec)); err != nil {
			t.Fatalf("render failed: %v", err)
		}
		return rec.Body.String()
	}

	for name, fn := range map[string]func(echo.Context) error{
		"home":       s.getHome,
		"signed-out": s.renderLoggedOut,
		"not-found":  s.renderNotFound,
	} {
		if out := render(fn); strings.Contains(out, "ctx-select") {
			t.Errorf("the %s page rendered the context switcher; it must not expose context names", name)
		}
	}
}

// The same session-less pages must not draw the main navigation. The navbar is built from the table
// of every view, so a visitor who has just logged out was offered a menu of links that bounce back
// to the IdP -- and with RBAC-aligned views on, a menu that says nothing about what that person may
// read. The signed-out page drops the logout button with it: the session it would end is gone.
func TestPublicPagesHideTheNavigation(t *testing.T) {
	useTestKubeconfig(t, twoClusterKubeconfig)
	t.Cleanup(viper.Reset)
	viper.Set("auth_enabled", "OIDC")

	registry, err := newClientRegistry()
	if err != nil {
		t.Fatalf("building the registry failed: %v", err)
	}
	s := &server{k8s: registry, ssr: newSSRRenderer()}

	// A view page does render the navbar, so the assertions below are about suppression rather than
	// about a template that never draws one.
	layoutCtx := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	if len(s.ssrLayoutData(layoutCtx, "constraints", "/constraints", "Constraints").Nav) < 2 {
		t.Fatal("expected a view page to have a navbar to suppress")
	}

	render := func(fn func(echo.Context) error) string {
		rec := httptest.NewRecorder()
		if err := fn(echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)); err != nil {
			t.Fatalf("render failed: %v", err)
		}
		return rec.Body.String()
	}

	for name, fn := range map[string]func(echo.Context) error{
		"signed-out": s.renderLoggedOut,
		"not-found":  s.renderNotFound,
	} {
		if out := render(fn); strings.Contains(out, `class="nav-link`) {
			t.Errorf("the %s page rendered the navbar; a page reachable without a session must not offer one", name)
		}
	}

	if out := render(s.renderLoggedOut); strings.Contains(out, "Log out") {
		t.Error("the signed-out page offered a logout button")
	}
}

// stripHTMLTags removes tags and unescapes entities so an assertion can match source text that
// syntax highlighting split across <span> tokens.
var htmlTagRE = regexp.MustCompile(`<[^>]*>`)

func stripHTMLTags(s string) string {
	return html.UnescapeString(htmlTagRE.ReplaceAllString(s, ""))
}

func TestExtractRegoFallsBackToCode(t *testing.T) {
	if got := extractRego(map[string]any{"rego": "inline"}); got != "inline" {
		t.Errorf("inline rego = %q, want %q", got, "inline")
	}
	target := map[string]any{"code": []any{
		map[string]any{"engine": "K8sNativeValidation", "source": map[string]any{}},
		map[string]any{"engine": "Rego", "source": map[string]any{"rego": "fromcode"}},
	}}
	if got := extractRego(target); got != "fromcode" {
		t.Errorf("code rego = %q, want %q", got, "fromcode")
	}
}

func TestFormatTimestamp(t *testing.T) {
	if got := formatTimestamp("2026-01-02T03:04:05Z"); got != "2026-01-02 03:04:05 UTC" {
		t.Errorf("formatTimestamp = %q", got)
	}
	if got := formatTimestamp("not-a-time"); got != "not-a-time" {
		t.Errorf("unparseable timestamp should pass through, got %q", got)
	}
	if got := formatTimestamp(""); got != "" {
		t.Errorf("empty timestamp should stay empty, got %q", got)
	}
}

// Issue #631: a view whose Kubernetes call failed must show the error underneath the generic
// sentence, or nobody can tell a 403 from a missing CRD without the pod logs. Checked on every view
// that renders the shared "viewerror" block, and the detail must be escaped like any other
// API-supplied string.
func TestViewsRenderTheErrorDetail(t *testing.T) {
	r := newSSRRenderer()
	var buf bytes.Buffer

	for _, page := range []string{"configurations", "mutations", "constrainttemplates", "constraints", "events"} {
		data := map[string]any{"Layout": minimalLayout()}
		setViewError(data, "GPM could not reach the cluster.", errors.New(`forbidden: User "gpm" cannot list <resource>`))

		buf.Reset()
		if err := r.pages[page].ExecuteTemplate(&buf, "layout", data); err != nil {
			t.Fatalf("%s render failed: %v", page, err)
		}
		out := buf.String()
		for _, want := range []string{"GPM could not reach the cluster.", "Details", `cannot list &lt;resource&gt;`} {
			if !strings.Contains(out, want) {
				t.Errorf("%s output missing %q", page, want)
			}
		}
	}

	// No error, no banner: the block must not leave an empty alert on a healthy page.
	buf.Reset()
	if err := r.pages["constraints"].ExecuteTemplate(&buf, "layout", map[string]any{"Layout": minimalLayout()}); err != nil {
		t.Fatalf("constraints render failed: %v", err)
	}
	if strings.Contains(buf.String(), "alert-error") {
		t.Error("constraints rendered the error banner with no error set")
	}
}

// The error client-go actually surfaces for an untrusted certificate: an x509 error wrapped by
// crypto/tls, wrapped again by net/url. If errors.As stops unwrapping anywhere along that chain the
// hint silently disappears, so the nesting matters more than the leaf type.
func realWorldTLSError() error {
	return &url.Error{
		Op:  "Get",
		URL: "https://10.0.0.1:443/apis/config.gatekeeper.sh/v1alpha1/configs",
		Err: &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}},
	}
}

// A certificate failure must name GPM_SKIP_TLS_VERIFY instead of the caller's generic sentence, and
// nothing else may be mistaken for one. The JSON API did this until 9c9e27f removed
// kubeAPIErrorAnswer with it; these cases are that helper's.
func TestSetViewErrorTLSHint(t *testing.T) {
	const generic = "GPM could not get the configuration objects from the Kubernetes API."

	for _, tt := range []struct {
		name     string
		err      error
		wantHint bool
	}{
		{"unknown authority", x509.UnknownAuthorityError{}, true},
		// Both of these format themselves from the certificate, so it must not be nil.
		{"hostname mismatch", x509.HostnameError{Certificate: &x509.Certificate{}, Host: "10.0.0.1"}, true},
		{"invalid certificate", x509.CertificateInvalidError{Cert: &x509.Certificate{}, Reason: x509.Expired}, true},
		{"verification failure", &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}, true},
		{"wrapped as client-go returns it", realWorldTLSError(), true},
		{"wrapped in fmt.Errorf", fmt.Errorf("listing configs: %w", x509.UnknownAuthorityError{}), true},
		{"connection refused", errors.New("dial tcp 10.0.0.1:443: connect: connection refused"), false},
		{"not found", errors.New("the server could not find the requested resource"), false},
		// Mentions certificates but is not a certificate error: must not be misclassified.
		{"mentions certificates only in text", errors.New("failed to read certificate file"), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := map[string]any{}
			setViewError(data, generic, tt.err)

			msg, _ := data["Error"].(string)
			if got := strings.Contains(msg, "GPM_SKIP_TLS_VERIFY"); got != tt.wantHint {
				t.Errorf("hint = %v, want %v (message %q)", got, tt.wantHint, msg)
			}
			if !tt.wantHint && msg != generic {
				t.Errorf("message = %q, want the caller's %q", msg, generic)
			}
			if data["ErrorDetail"] != tt.err.Error() {
				t.Errorf("detail = %q, want the original error %q", data["ErrorDetail"], tt.err.Error())
			}
		})
	}
}

// A Mutation carrying a description used to show less than a Constraint Template carrying one: the
// views render the same Gatekeeper annotation, but only the Template view read it. Rendering raw API
// objects is what made this fiddly -- an object with no annotations at all must not abort the page.
func TestMutationsRenderTheDescription(t *testing.T) {
	r := newSSRRenderer()
	mutation := func(name string, annotations map[string]any) map[string]any {
		meta := map[string]any{"name": name}
		if annotations != nil {
			meta["annotations"] = annotations
		}
		return map[string]any{"kind": "Assign", "metadata": meta, "spec": map[string]any{"location": "spec"}}
	}

	var buf bytes.Buffer
	data := map[string]any{"Layout": minimalLayout(), "Mutations": []map[string]any{
		mutation("described", map[string]any{"description": "Adds a default runtimeClassName. See https://docs.example.invalid/mutations."}),
		mutation("bare", nil),
		mutation("other-annotations-only", map[string]any{"owner": "platform"}),
	}}
	if err := r.pages["mutations"].ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatalf("mutations render failed: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"Adds a default runtimeClassName.",
		// linkified the same way the Constraint Template description is
		`<a href="https://docs.example.invalid/mutations" target="_blank" rel="noopener noreferrer">`,
		"bare",
		"other-annotations-only",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("mutations output missing %q", want)
		}
	}
}

// The same accessor guards every raw-object view, so check the shapes it has to survive directly.
func TestAnnotationSurvivesObjectsWithoutAnnotations(t *testing.T) {
	for _, tt := range []struct {
		name string
		obj  any
		want string
	}{
		{"has it", map[string]any{"metadata": map[string]any{"annotations": map[string]any{"description": "hi"}}}, "hi"},
		{"annotations without that key", map[string]any{"metadata": map[string]any{"annotations": map[string]any{"owner": "x"}}}, ""},
		{"no annotations", map[string]any{"metadata": map[string]any{"name": "n"}}, ""},
		{"no metadata", map[string]any{"kind": "Assign"}, ""},
		{"not an object", "nonsense", ""},
		{"nil", nil, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := annotation(tt.obj, "description"); got != tt.want {
				t.Errorf("annotation = %q, want %q", got, tt.want)
			}
		})
	}
}

// Two Constraints of different Kinds may carry the same name. Anchoring a card on the name alone
// gave them the same id, so one card was unreachable, its sidebar entry could never be marked, and a
// shared violation link (issue #1324) landed on the wrong card.
func TestConstraintCardsAreUniquePerKindAndName(t *testing.T) {
	same := func(kind string) ssrConstraint {
		return ssrConstraint{
			Name: "must-have-owner", Kind: kind, EnforcementMode: "deny",
			ViolationsKnown: true, TotalViolations: 1, ReturnedCount: 1,
			Violations: []ssrConstraintViolation{
				{EnforcementAction: "deny", Kind: "Pod", Namespace: "default", Name: "nginx", Message: "missing owner"},
			},
		}
	}
	data := map[string]any{
		"Layout":      minimalLayout(),
		"Constraints": []ssrConstraint{same("K8sRequiredLabels"), same("K8sRequiredAnnotations")},
		"ReportURL":   "/constraints?report=html",
	}

	var buf bytes.Buffer
	if err := newSSRRenderer().pages["constraints"].ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatalf("constraints render failed: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		`id="K8sRequiredLabels--must-have-owner"`,
		`id="K8sRequiredAnnotations--must-have-owner"`,
		`href="#K8sRequiredLabels--must-have-owner"`,
		`href="#K8sRequiredAnnotations--must-have-owner"`,
		// the per-card data islands must not collide either, or one table reads the other's rows
		`id="viol-K8sRequiredLabels--must-have-owner"`,
		`id="viol-K8sRequiredAnnotations--must-have-owner"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("constraints output missing %q", want)
		}
	}
	// And nothing may still anchor on the bare name.
	for _, unwanted := range []string{`id="must-have-owner"`, `href="#must-have-owner"`, `id="viol-must-have-owner"`} {
		if strings.Contains(out, unwanted) {
			t.Errorf("constraints output still uses the ambiguous %q", unwanted)
		}
	}
}

// The Events view cross-links to a Constraint card, so it has to build the same fragment.
func TestEventsLinkToTheKindPrefixedConstraintCard(t *testing.T) {
	ev := ssrEvent{
		Name: "e1", Reason: "FailedAdmission", Message: "denied",
		ConstraintKind: "K8sRequiredLabels", ConstraintName: "must-have-owner",
	}
	var buf bytes.Buffer
	data := map[string]any{"Layout": minimalLayout(), "Events": []ssrEvent{ev}}
	if err := newSSRRenderer().pages["events"].ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatalf("events render failed: %v", err)
	}
	if !strings.Contains(buf.String(), "/constraints#K8sRequiredLabels--must-have-owner") {
		t.Errorf("events output does not link to the Kind-prefixed card:\n%s", buf.String())
	}
}

// An Event that never recorded the Kind still links somewhere sensible rather than to "--name".
func TestConstraintAnchorWithoutAKind(t *testing.T) {
	if got := constraintAnchor("", "must-have-owner"); got != "must-have-owner" {
		t.Errorf("constraintAnchor = %q, want the bare name", got)
	}
}

// Gatekeeper records, per pod, whether each enforcement point is actually enforcing. GPM read none
// of it, so a Constraint whose ValidatingAdmissionPolicy engine is missing still read as fully
// enforced on the card. Taken from a live cluster, where every Constraint reports
// vap.k8s.io -> error "K8sNativeValidation engine is missing".
func TestEnforcementIssuesFoldPodsTogether(t *testing.T) {
	pod := func(id string, points ...map[string]any) map[string]any {
		p := map[string]any{"id": id, "enforced": true}
		if points != nil {
			ps := make([]any, 0, len(points))
			for _, x := range points {
				ps = append(ps, x)
			}
			p["enforcementPointsStatus"] = ps
		}
		return p
	}
	point := func(name, state, msg string) map[string]any {
		return map[string]any{"enforcementPoint": name, "state": state, "message": msg}
	}

	for _, tt := range []struct {
		name string
		pods []any
		want []ssrEnforcementIssue
	}{
		{"nothing to report", []any{pod("audit-0", point("webhook.k8s.io", "active", ""))}, nil},
		{"no status at all", []any{pod("audit-0")}, nil},
		{
			"one point, reported by two pods, folded into one line",
			[]any{
				pod("audit-0", point("vap.k8s.io", "error", "K8sNativeValidation engine is missing")),
				pod("audit-1", point("vap.k8s.io", "error", "K8sNativeValidation engine is missing")),
			},
			[]ssrEnforcementIssue{{Label: "vap.k8s.io reports an error", Message: "K8sNativeValidation engine is missing", Pods: 2}},
		},
		{
			"a state that is not an error still speaks up, in its own words",
			[]any{pod("audit-0", point("vap.k8s.io", "pending", "waiting for the engine"))},
			[]ssrEnforcementIssue{{Label: "vap.k8s.io reports pending", Message: "waiting for the engine", Pods: 1}},
		},
		{
			// The explicit ordering slice is gone: order now rides on append. Lock it, or a later
			// refactor can start answering points in map order, which is random per run.
			"two failing points keep the order Gatekeeper reported them in",
			[]any{pod("audit-0",
				point("vap.k8s.io", "error", "engine missing"),
				point("webhook.k8s.io", "pending", "not ready"))},
			[]ssrEnforcementIssue{
				{Label: "vap.k8s.io reports an error", Message: "engine missing", Pods: 1},
				{Label: "webhook.k8s.io reports pending", Message: "not ready", Pods: 1},
			},
		},
		{
			"active points are dropped, failing ones kept",
			[]any{pod("audit-0",
				point("webhook.k8s.io", "active", ""),
				point("vap.k8s.io", "error", "missing"))},
			[]ssrEnforcementIssue{{Label: "vap.k8s.io reports an error", Message: "missing", Pods: 1}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := enforcementIssues(map[string]any{"status": map[string]any{"byPod": tt.pods}})
			if len(got) != len(tt.want) {
				t.Fatalf("got %d issues, want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("issue %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// The card has to show it, and only when there is something to show.
func TestConstraintCardShowsAFailingEnforcementPoint(t *testing.T) {
	base := ssrConstraint{Name: "readiness-probe", Kind: "K8sReadinessProbe", EnforcementMode: "deny", Created: "2026-01-14"}
	failing := base
	failing.EnforcementIssues = []ssrEnforcementIssue{
		{Label: "vap.k8s.io reports an error", Message: "K8sNativeValidation engine is missing", Pods: 1},
	}

	render := func(c ssrConstraint) string {
		var buf bytes.Buffer
		data := map[string]any{"Layout": minimalLayout(), "Constraints": []ssrConstraint{c}, "ReportURL": "/r"}
		if err := newSSRRenderer().pages["constraints"].ExecuteTemplate(&buf, "layout", data); err != nil {
			t.Fatalf("render: %v", err)
		}
		return buf.String()
	}

	out := render(failing)
	for _, want := range []string{
		"vap.k8s.io reports an error",
		"K8sNativeValidation engine is missing",
		"reported by 1 pod",
		`class="foot-error"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("card missing %q", want)
		}
	}

	// and a healthy Constraint says nothing at all
	if clean := render(base); strings.Contains(clean, "foot-error") {
		t.Error("a Constraint with no failing enforcement point still rendered the error line")
	}
}

// The three views share one reader over status.byPod, so the line each card shows is decided here.
// Quiet when everything is in sync, and specific when it is not.
func TestPodSummaryLine(t *testing.T) {
	pod := func(gen int64, ops []string, extra map[string]any) map[string]any {
		p := map[string]any{"id": "gatekeeper-audit-0", "observedGeneration": gen}
		if ops != nil {
			anyOps := make([]any, 0, len(ops))
			for _, o := range ops {
				anyOps = append(anyOps, o)
			}
			p["operations"] = anyOps
		}
		for k, v := range extra {
			p[k] = v
		}
		return p
	}
	obj := func(generation int64, status map[string]any, pods ...map[string]any) map[string]any {
		if pods != nil {
			byPod := make([]any, 0, len(pods))
			for _, p := range pods {
				byPod = append(byPod, p)
			}
			status["byPod"] = byPod
		}
		return map[string]any{
			"metadata": map[string]any{"generation": generation},
			"status":   status,
		}
	}

	for _, tt := range []struct {
		name     string
		obj      map[string]any
		expected int
		line     string
		state    string
	}{
		{
			"every pod at the current generation says so once",
			obj(1, map[string]any{}, pod(1, []string{"audit"}, nil), pod(1, nil, nil)),
			2, "in sync on 2 pods", "",
		},
		{
			"a pod on an older generation is named against the view's count",
			obj(2, map[string]any{}, pod(2, nil, nil), pod(1, nil, nil)),
			2, "in sync on 1 of 2 pods", "warn",
		},
		{
			"fewer pods reporting than the rest of the page has",
			obj(1, map[string]any{}, pod(1, nil, nil)),
			4, "in sync on 1 of 4 pods", "warn",
		},
		{
			"not enforcing outranks being out of sync",
			obj(1, map[string]any{}, pod(1, nil, map[string]any{"enforced": false})),
			1, "not enforced on 1 pod", "warn",
		},
		{
			"a compile error outranks everything else",
			obj(1, map[string]any{}, pod(1, nil, map[string]any{
				"errors": []any{map[string]any{"message": "rego does not compile"}},
			})),
			1, "1 pod reports an error", "error",
		},
		{
			"a template Gatekeeper never compiled",
			obj(1, map[string]any{"created": false}, pod(1, nil, nil)),
			1, "not compiled into a CRD", "error",
		},
		{
			// Only templates carry status.created, so its absence is not a failure for the others.
			"nothing reporting yet",
			obj(1, map[string]any{}),
			2, "no pod has reported on it yet", "warn",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := podSummary(tt.obj, tt.expected)
			if got.Line != tt.line {
				t.Errorf("line = %q, want %q", got.Line, tt.line)
			}
			if got.State != tt.state {
				t.Errorf("state = %q, want %q", got.State, tt.state)
			}
		})
	}
}

// The denominator comes from the page, because GPM may not list pods.
func TestMaxPodCountIsTheBusiestObjectOnThePage(t *testing.T) {
	with := func(n int) map[string]any {
		pods := make([]any, n)
		for i := range pods {
			pods[i] = map[string]any{"id": "p"}
		}
		return map[string]any{"status": map[string]any{"byPod": pods}}
	}
	if got := maxPodCount([]map[string]any{with(1), with(4), with(3)}); got != 4 {
		t.Errorf("maxPodCount = %d, want 4", got)
	}
	if got := maxPodCount([]map[string]any{{"status": map[string]any{}}}); got != 0 {
		t.Errorf("maxPodCount with nothing reporting = %d, want 0", got)
	}
}

// The Constraint Templates card links out to each Constraint built from the template, and those
// cards are anchored on Kind and name together. This link was missed when the anchors changed, so it
// pointed at a fragment nothing answered to and the Constraint was not highlighted on arrival. The
// earlier "no bare-name anchors" check only looked at the Constraints view, which is why it slipped.
func TestTemplateCardLinksToTheKindPrefixedConstraintCard(t *testing.T) {
	ct := ssrConstraintTemplate{
		Name: "k8srequiredlabels", Kind: "K8sRequiredLabels",
		Constraints: []string{"must-have-owner", "must-have-team"},
	}
	var buf bytes.Buffer
	data := map[string]any{"Layout": minimalLayout(), "Templates": []ssrConstraintTemplate{ct}}
	if err := newSSRRenderer().pages["constrainttemplates"].ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"/constraints#K8sRequiredLabels--must-have-owner",
		"/constraints#K8sRequiredLabels--must-have-team",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("template card missing the link %q", want)
		}
	}
	for _, unwanted := range []string{`"/constraints#must-have-owner"`, `"/constraints#must-have-team"`} {
		if strings.Contains(out, unwanted) {
			t.Errorf("template card still links to the bare name %s, which no card answers to", unwanted)
		}
	}
}

// Descriptions come from an annotation on a cluster object, and are now rendered as markdown rather
// than printed as text. That widens what the page will build from cluster content, so the escaping
// and the link handling are asserted here rather than assumed from goldmark's defaults.
func TestMarkdownRendersDescriptionsSafely(t *testing.T) {
	for _, tt := range []struct {
		name   string
		in     string
		want   []string
		unwant []string
	}{
		{
			"code spans and lists, which is what the community templates use",
			"Disallow `HorizontalPodAutoscalers` with:\n\n1. no `scaleTargetRef`\n2. a bad spread",
			[]string{"<code>HorizontalPodAutoscalers</code>", "<ol>", "<li>no <code>scaleTargetRef</code></li>"},
			nil,
		},
		{
			"a bare URL still becomes a link, as it did before markdown",
			"See https://docs.example.invalid/policy for more.",
			[]string{`href="https://docs.example.invalid/policy"`, `target="_blank"`, `rel="noopener noreferrer"`},
			nil,
		},
		{
			// The helper this replaced matched https? -- plain http keeps working.
			"plain http is a link too, and so is a bare www host",
			"see http://example.invalid/docs and www.example.invalid/more",
			[]string{`href="http://example.invalid/docs"`, `href="http://www.example.invalid/more"`},
			nil,
		},
		{
			// goldmark's own autolinker wants a dotted host with a letter suffix and drops all three
			// of these. They are exactly the URLs a description written inside a cluster contains.
			"a host without a dot, and a bare IP, are still links",
			"try http://localhost:8082/constraints#viol-x and http://wiki/page and http://10.0.0.5:9090/x",
			[]string{
				`href="http://localhost:8082/constraints#viol-x"`,
				`href="http://wiki/page"`,
				`href="http://10.0.0.5:9090/x"`,
			},
			nil,
		},
		{
			"a URL ending a sentence does not swallow the full stop",
			"read https://example.invalid/docs. Then stop.",
			[]string{`href="https://example.invalid/docs"`},
			[]string{`href="https://example.invalid/docs."`},
		},
		{
			"a data: link is emptied like javascript:",
			"[x](data:text/html;base64,PHN2Zz4=)",
			[]string{`href=""`},
			[]string{"data:text/html"},
		},
		{
			// goldmark 1.8.4 empties SVG data: URLs, which can carry script, while leaving
			// harmless raster ones alone. Pinned so a downgrade cannot quietly bring it back.
			"an SVG data: image is emptied, a PNG one is not",
			"![a](data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=) ![b](data:image/png;base64,iVBORw0KGgo=)",
			[]string{`src=""`, "data:image/png;base64,iVBORw0KGgo="},
			[]string{"data:image/svg+xml"},
		},
		{
			"raw HTML in the description never reaches the page",
			`Fine <script>alert(1)</script> and <img src=x onerror=alert(1)>`,
			nil,
			[]string{"<script>", "<img src=x", "onerror="},
		},
		{
			"a link with a dangerous scheme is not a working link",
			"[click me](javascript:alert(1))",
			[]string{"click me"},
			[]string{"javascript:alert"},
		},
		{
			"plain prose is simply a paragraph, which is the fallback",
			"Requires containers to have a Liveness Probe defined.",
			[]string{"<p>Requires containers to have a Liveness Probe defined."},
			[]string{"<code>", "<ul>"},
		},
		{
			"markdown special characters in ordinary text do not break the page",
			"Rejecting 'foo' * 2 < 3 & \"bar\"",
			[]string{"&lt; 3 &amp;"},
			[]string{"<script"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := string(markdown(tt.in))
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\n got: %s", want, got)
				}
			}
			for _, unwanted := range tt.unwant {
				if strings.Contains(got, unwanted) {
					t.Errorf("output still contains %q\n got: %s", unwanted, got)
				}
			}
		})
	}
}

// And the card actually uses it.
func TestTemplateCardRendersItsDescriptionAsMarkdown(t *testing.T) {
	ct := ssrConstraintTemplate{
		Name: "k8shpa", Kind: "K8sHorizontalPodAutoscaler",
		Description: "Disallow `HorizontalPodAutoscalers` without a `scaleTargetRef`.",
	}
	var buf bytes.Buffer
	data := map[string]any{"Layout": minimalLayout(), "Templates": []ssrConstraintTemplate{ct}}
	if err := newSSRRenderer().pages["constrainttemplates"].ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(buf.String(), "<code>HorizontalPodAutoscalers</code>") {
		t.Errorf("the template card printed the markdown instead of rendering it:\n%s", buf.String())
	}
}

// --- Resources view -------------------------------------------------------------------------

// renderSSR renders one page the way the server does, and returns the HTML.
func renderSSR(t *testing.T, page string, data map[string]any) string {
	t.Helper()
	if _, ok := data["Layout"]; !ok {
		data["Layout"] = minimalLayout()
	}
	var buf bytes.Buffer
	if err := newSSRRenderer().pages[page].ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatalf("render %s: %v", page, err)
	}
	return buf.String()
}

// viol builds one violation as ssrConstraintModel would have parsed it.
func viol(group, kind, ns, name, action string) ssrConstraintViolation {
	return ssrConstraintViolation{
		Group: group, Version: "v1", Kind: kind, Namespace: ns, Name: name,
		EnforcementAction: action, Message: kind + "/" + name + " breaks " + action,
	}
}

func TestResourceModelPivotsViolationsOntoObjects(t *testing.T) {
	constraints := []ssrConstraint{
		{Name: "liveness-probe", Kind: "K8sLivenessProbe", Violations: []ssrConstraintViolation{
			viol("apps", "Deployment", "apps-prod", "checkout-api", "deny"),
			viol("apps", "Deployment", "apps-prod", "legacy-monolith", "deny"),
			viol("", "Pod", "team-payments", "ledger-0", "warn"),
		}},
		{Name: "pod-must-have-owner", Kind: "K8sRequiredLabels", Violations: []ssrConstraintViolation{
			// the same object as above, a second policy: it must land on one row, not two
			viol("apps", "Deployment", "apps-prod", "checkout-api", "dryrun"),
			viol("", "Namespace", "", "apps-legacy", "dryrun"),
		}},
	}

	got := resourceModel(constraints)

	if len(got) != 3 {
		t.Fatalf("expected three namespaces (apps-prod, team-payments, cluster-scoped), got %d", len(got))
	}

	// apps-prod has the most blocking violations, so it leads; the cluster-scoped bucket is last
	// however bad it is, because it is a different kind of thing.
	if order := []string{got[0].Title(), got[1].Title(), got[2].Title()}; order[0] != "apps-prod" ||
		order[2] != "cluster-scoped" {
		t.Errorf("namespaces out of order: %v", order)
	}

	prod := got[0]
	if prod.Deny != 2 || prod.DryRun != 1 || prod.Warn != 0 || prod.Total() != 3 {
		t.Errorf("apps-prod counts wrong: deny=%d dryrun=%d warn=%d total=%d",
			prod.Deny, prod.DryRun, prod.Warn, prod.Total())
	}
	if len(prod.Resources) != 2 {
		t.Fatalf("checkout-api broke two policies but should be one row; got %d rows", len(prod.Resources))
	}
	// Within a namespace the worst object comes first: checkout-api has a deny and a dryrun, so it
	// outranks legacy-monolith's single deny on total.
	if first := prod.Resources[0]; first.Name != "checkout-api" || first.Deny != 1 || first.DryRun != 1 ||
		first.Total() != 2 || len(first.Violations) != 2 {
		t.Errorf("first row wrong: %+v", first)
	}
	if got[2].Anchor != "cluster-scoped" || got[2].Name != "" {
		t.Errorf("cluster-scoped bucket wrong: name=%q anchor=%q", got[2].Name, got[2].Anchor)
	}
	if prod.Anchor != "ns-apps-prod" {
		t.Errorf("namespace anchor should be prefixed, got %q", prod.Anchor)
	}
}

func TestResourceModelKeepsSameNamedKindsFromDifferentGroupsApart(t *testing.T) {
	// The reason ssrConstraintViolation captures the group at all: two CRDs can share a Kind.
	constraints := []ssrConstraint{{Name: "c", Kind: "K", Violations: []ssrConstraintViolation{
		viol("acme.example.com", "Widget", "shop", "thing", "deny"),
		viol("other.example.com", "Widget", "shop", "thing", "warn"),
	}}}

	rows := resourceModel(constraints)[0].Resources
	if len(rows) != 2 {
		t.Fatalf("same Kind and name from different groups must stay apart; got %d row(s)", len(rows))
	}
	seen := map[string]bool{rows[0].Group: true, rows[1].Group: true}
	if !seen["acme.example.com"] || !seen["other.example.com"] {
		t.Errorf("groups lost: %q and %q", rows[0].Group, rows[1].Group)
	}
}

func TestResourceNamespaceSummaryAndSegments(t *testing.T) {
	ns := ssrResourceNamespace{Deny: 2, DryRun: 0, Warn: 3}
	if got, want := ns.Summary(), "2 deny · 3 warn"; got != want {
		t.Errorf("summary = %q, want %q (absent modes are left out)", got, want)
	}
	if got := ns.Segments(); len(got) != 3 || got[0].Mode != "deny" || got[1].Mode != "dryrun" || got[2].Mode != "warn" {
		t.Errorf("segments must stay in a fixed order so the colours mean one thing: %+v", got)
	}
	if empty := (ssrResourceNamespace{}).Summary(); empty != "no violations" {
		t.Errorf("empty summary = %q", empty)
	}
}

func TestResourcesViewRendersRowsAndCounts(t *testing.T) {
	page := renderSSR(t, "resources", map[string]any{
		"Layout":  minimalLayout(),
		"Audited": true,
		"Namespaces": resourceModel([]ssrConstraint{
			{Name: "liveness-probe", Kind: "K8sLivenessProbe", Violations: []ssrConstraintViolation{
				viol("apps", "Deployment", "apps-prod", "checkout-api", "deny"),
			}},
		}),
	})

	for _, want := range []string{
		`id="ns-apps-prod"`, // the card anchor the sidebar links to
		`data-search="checkout-api Deployment apps liveness-probe K8sLivenessProbe"`, // the filter reads this
		`class="n n-deny">1<`, // the count column
		`href="/constraints#K8sLivenessProbe--liveness-probe"`, // back to the policy
		"resources-filter.js",
		// the share link: a readable row id, and the copy button the violations table established
		`id="ns-apps-prod--Deployment--checkout-api"`,
		`copyShareLink($data, &#39;ns-apps-prod--Deployment--checkout-api&#39;)`,
		"share-link.js",
		`class="vlink"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}
	// One namespace needs no navigator.
	if strings.Contains(page, "layout-sidebar") {
		t.Error("a single namespace should render without the sidebar")
	}
}

func TestResourcesViewDistinguishesEmptyFromUnaudited(t *testing.T) {
	unaudited := renderSSR(t, "resources", map[string]any{"Audited": false})
	if !strings.Contains(unaudited, "Not audited yet") {
		t.Error("before the first audit the page must not claim there are no violations")
	}
	clean := renderSSR(t, "resources", map[string]any{"Audited": true})
	if !strings.Contains(clean, "No object breaks a policy") {
		t.Error("an audited cluster with no violations should say so")
	}
}

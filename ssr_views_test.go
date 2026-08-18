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
	for _, want := range []string{"K8sRequiredLabels", "must-have-owner", "Parameters schema", "created"} {
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
		"must-have-owner", "K8sRequiredLabels", "deny mode", "violationsTable('viol-must-have-owner')",
		"missing owner", "audit limit", "Download violations report", "Filter violations",
		// The filter + pager are gated to long lists, and the count label replaced "showing X of Y".
		`x-show="showControls"`, "countLabel",
		// Sidebar scroll-spy is wired via the shared layout script on every page.
		"sidebar-spy.js",
		// Per-violation shareable links (issue #1324): each row has a stable id and a copy control.
		"copyLink(row)", `class="vlink"`, `x-bind:id="row._id"`,
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

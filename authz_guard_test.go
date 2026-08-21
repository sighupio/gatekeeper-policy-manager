// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
	authorizationv1 "k8s.io/api/authorization/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	authorizationv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/client-go/tools/clientcmd/api"
)

// registryWithContexts builds a registry whose kubeconfig names n contexts, which is all
// rbacFilteringEnabled looks at.
func registryWithContexts(n int) *clientRegistry {
	cfg := api.NewConfig()
	for i := range n {
		cfg.Contexts[fmt.Sprintf("cluster-%d", i)] = api.NewContext()
	}
	return &clientRegistry{kubeconfig: cfg}
}

// The guard is the access control; the trimmed navbar is only its visible half (plan decision 3).
// These tests drive real requests through the middleware chain, so a route that slips past the
// allow-list fails here rather than in production.

// guardedServer runs the real rbacMiddleware. Only the API server is faked: allow decides which
// (group, resource) pairs this person may list, so a test can hand out any subset of the views.
func guardedServer(t *testing.T, allow func(group, resource string) bool) *echo.Echo {
	t.Helper()
	t.Cleanup(viper.Reset)
	viper.Set("secret_key", "a-test-key-that-is-not-the-default")
	viper.Set("rbac_filtering", true)
	viper.Set("auth_enabled", "OIDC")

	client := sarClient(func(r *authorizationv1.SubjectAccessReview) (bool, error) {
		ra := r.Spec.ResourceAttributes
		return allow(ra.Group, ra.Resource), nil
	})
	registry := registryWithContexts(1)
	registry.clients = map[string]*kubeClients{
		defaultKubeContext: {authz: client.AuthorizationV1()},
		"cluster-0":        {authz: client.AuthorizationV1()},
	}
	registry.kubeconfig.CurrentContext = "cluster-0"

	s := &server{k8s: registry, ssr: newSSRRenderer()}

	e := echo.New()
	e.Use(session.Middleware(newSessionStore()))
	// Stand in for the auth middleware: every request carries a logged-in identity.
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			sess, _ := session.Get(sessionName, c)
			sess.Values[sessionKeyUser] = "dev"
			sess.Values[sessionKeyRBACUser] = "dev"
			sess.Values[sessionKeyRBACGroups] = []string{"team-a"}
			return next(c)
		}
	})
	e.Use(s.rbacMiddleware())

	ok := func(c echo.Context) error { return c.String(http.StatusOK, "view") }
	for _, p := range allSSRPaths {
		e.GET(p, ok)
	}
	e.GET("/health", ok)
	e.GET("/static/*", ok)
	return e
}

func nothing(string, string) bool    { return false }
func everything(string, string) bool { return true }

// Every path the UI serves. Kept as a list so a new view added without a guard decision shows up
// here as an untested route rather than as an open door.
var allSSRPaths = []string{
	"/", "/home", "/home/:context",
	"/configurations", "/configurations/:context",
	"/mutations", "/mutations/:context",
	"/constrainttemplates", "/constrainttemplates/:context",
	"/constraints", "/constraints/:context",
	"/resources", "/resources/:context",
	"/events", "/events/:context",
}

func get(e *echo.Echo, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// Every route the app serves must resolve to a view the guard knows: a path it does not recognise
// is served unchecked. Derived from the router rather than a hand-written list, because a list is
// what let /home through -- the route existed, the harness registered it, and nothing asserted it.
func TestEveryRegisteredRouteResolvesToAView(t *testing.T) {
	e := echo.New()
	registerViews(e, &server{k8s: &clientRegistry{}, ssr: newSSRRenderer()})
	// The routes main serves that are not views. Registered through the same function main calls,
	// so a route added beside /health cannot slip past this test.
	registerSystemRoutes(e, &authenticator{})
	for _, r := range e.Routes() {
		if isPublicPath(r.Path) {
			continue
		}
		if _, known := viewForPath(r.Path); !known {
			t.Errorf("%s %s resolves to no view, so the guard serves it unchecked", r.Method, r.Path)
		}
	}
}

// The dashboard answers on three paths. All of them are the front door: a reader who may not have
// it is sent where they can be, and none of them is served unchecked.
func TestEveryNameOfTheDashboardIsGuarded(t *testing.T) {
	e := guardedServer(t, nothing)
	for _, p := range []string{"/", "/home", "/home/cluster-0"} {
		rec := get(e, p)
		if rec.Code != http.StatusFound || !strings.HasSuffix(rec.Header().Get("Location"), "/resources") {
			t.Errorf("%s: want a redirect to /resources, got %d -> %q", p, rec.Code, rec.Header().Get("Location"))
		}
	}
}

// A path that is not a view needs no answers: the 404 page draws no navbar. Asking anyway would
// let any path a scanner invents cost a round of SubjectAccessReviews.
func TestAnUnknownPathAsksTheApiServerNothing(t *testing.T) {
	// A server of its own for each half: on a shared one the first request warms the cache and the
	// second sends nothing whatever the code does, which makes the assertion meaningless.
	var unknown int
	if rec := get(guardedServer(t, func(string, string) bool { unknown++; return true }), "/wp-login.php"); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown path should 404, got %d", rec.Code)
	}
	if unknown != 0 {
		t.Errorf("an unknown path sent %d reviews", unknown)
	}

	// The harness does ask when the path is a view, so the zero above is the guard's doing.
	var known int
	if rec := get(guardedServer(t, func(string, string) bool { known++; return true }), "/resources"); rec.Code != http.StatusOK {
		t.Fatalf("a real view should render, got %d", rec.Code)
	}
	if known == 0 {
		t.Fatal("a real view sent no reviews, so this test proves nothing")
	}
}

func TestAUserWhoCanListNothingReachesOnlyResources(t *testing.T) {
	e := guardedServer(t, nothing)

	for _, path := range []string{
		"/configurations", "/configurations/kind", "/mutations", "/mutations/kind",
		"/constrainttemplates", "/constrainttemplates/kind", "/constraints", "/constraints/kind",
		"/events", "/events/kind",
	} {
		if rec := get(e, path); rec.Code != http.StatusForbidden {
			t.Errorf("%s: must be refused, got %d", path, rec.Code)
		}
	}
	for _, path := range []string{"/resources", "/resources/kind", "/health", "/static/app.css"} {
		if rec := get(e, path); rec.Code != http.StatusOK {
			t.Errorf("%s: must stay reachable, got %d", path, rec.Code)
		}
	}
	// Home is the front door: send them where they can be, rather than refusing it.
	rec := get(e, "/")
	if rec.Code != http.StatusFound || !strings.HasSuffix(rec.Header().Get("Location"), "/resources") {
		t.Errorf("/ should redirect to /resources, got %d -> %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestAUserWhoCanListEverythingReachesEveryView(t *testing.T) {
	e := guardedServer(t, everything)
	for _, path := range []string{
		"/", "/home", "/configurations", "/mutations", "/constrainttemplates",
		"/constraints", "/resources", "/events",
	} {
		if rec := get(e, path); rec.Code != http.StatusOK {
			t.Errorf("%s: must be reachable, got %d", path, rec.Code)
		}
	}
}

// The case the old admin/restricted split could not express: partial access.
func TestPartialAccessOpensExactlyTheViewsItCovers(t *testing.T) {
	e := guardedServer(t, func(group, _ string) bool { return group == "constraints.gatekeeper.sh" })

	for path, want := range map[string]int{
		"/constraints":         http.StatusOK,        // granted
		"/":                    http.StatusOK,        // Home reads the same data
		"/resources":           http.StatusOK,        // never gated
		"/constrainttemplates": http.StatusForbidden, // a different group
		"/mutations":           http.StatusForbidden,
		"/events":              http.StatusForbidden,
		"/configurations":      http.StatusForbidden,
	} {
		if rec := get(e, path); rec.Code != want {
			t.Errorf("%s: got %d, want %d", path, rec.Code, want)
		}
	}
}

// Paths resolve to the view that owns them. Longest match wins, or /constraints would claim
// /constrainttemplates and hand out the wrong permission.
func TestViewForPathIsNotFooledByLookalikes(t *testing.T) {
	for path, wantKey := range map[string]string{
		"/":                    "home",
		"/constraints":         "constraints",
		"/constraints/kind":    "constraints",
		"/constrainttemplates": "constrainttemplates",
		"/resources":           "resources",
		"/resources/kind-gpm":  "resources",
		"/events":              "events",
		"/resourcesx":          "",
		"/notresources":        "",
		"/api/v1/resources":    "",
	} {
		v, ok := viewForPath(path)
		switch {
		case wantKey == "" && ok:
			t.Errorf("%q resolved to the %q view, but belongs to none", path, v.Key)
		case wantKey != "" && (!ok || v.Key != wantKey):
			t.Errorf("%q resolved to %q, want %q", path, v.Key, wantKey)
		}
	}
}

// Every gated view must name the data it shows, or it is open by omission.
func TestEveryViewDeclaresItsPermission(t *testing.T) {
	for _, v := range ssrViews {
		if !v.gated() && v.Key != "resources" {
			t.Errorf("view %q is ungated; only the Resources view may be, because it scopes every row", v.Key)
		}
	}
}

// The runtime switch is the flag plus authentication. The multi-cluster case is refused at startup
// instead (see TestUnsupportedRBACModesStopStartup): a runtime check here would have to fail *open*
// -- disable filtering and serve everything -- which is the wrong direction for a switch whose job
// is to restrict.
func TestFilteringStaysOffUnlessAskedForAndUsable(t *testing.T) {
	for _, tt := range []struct {
		name   string
		filter bool
		auth   string
		want   bool
	}{
		{"off by default", false, "OIDC", false},
		{"on, with OIDC", true, "OIDC", true},
		{"on, but anonymous: nobody to authorize", true, "Anonymous", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(viper.Reset)
			viper.Set("rbac_filtering", tt.filter)
			viper.Set("auth_enabled", tt.auth)

			s := &server{k8s: registryWithContexts(1)}
			if got := s.rbacFilteringEnabled(); got != tt.want {
				t.Errorf("rbacFilteringEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- filtering -----------------------------------------------------------------------------

// scopedServer answers reviews with allow, and maps Kinds through a discovery-backed mapper fed by
// the fake clientset, so the filter runs the same path it runs in production.
func scopedServer(t *testing.T, allow func(namespace, resource string) bool) (*server, *kubeClients) {
	t.Helper()
	client := sarClient(func(r *authorizationv1.SubjectAccessReview) (bool, error) {
		ra := r.Spec.ResourceAttributes
		return allow(ra.Namespace, ra.Resource), nil
	})
	client.Resources = []*metav1.APIResourceList{{
		GroupVersion: "apps/v1",
		APIResources: []metav1.APIResource{{Name: "deployments", Kind: "Deployment", Namespaced: true}},
	}, {
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{{Name: "pods", Kind: "Pod", Namespaced: true}},
	}}
	clients := &kubeClients{authz: client.AuthorizationV1(), discovery: client.Discovery()}
	return &server{k8s: registryWithContexts(1), ssr: newSSRRenderer()}, clients
}

func twoNamespaces() []ssrResourceNamespace {
	return resourceModel([]ssrConstraint{{Name: "c", Kind: "K", Violations: []ssrConstraintViolation{
		viol("apps", "Deployment", "mine", "checkout-api", "deny"),
		viol("apps", "Deployment", "theirs", "ledger", "deny"),
		viol("", "Pod", "theirs", "ledger-0", "warn"),
	}}})
}

// concurrencyProbe answers SubjectAccessReviews directly, recording how many are in flight at once.
// Not the fake clientset: it serializes every call behind its own lock, so no overlap is observable
// through it and a test built on it would pass with the bound removed.
type concurrencyProbe struct {
	authorizationv1client.SubjectAccessReviewInterface
	mu       sync.Mutex
	inFlight int
	peak     int
}

func (p *concurrencyProbe) Create(_ context.Context, sar *authorizationv1.SubjectAccessReview, _ metav1.CreateOptions) (*authorizationv1.SubjectAccessReview, error) {
	p.mu.Lock()
	p.inFlight++
	if p.inFlight > p.peak {
		p.peak = p.inFlight
	}
	p.mu.Unlock()
	// Long enough that the workers overlap, short enough not to slow the suite.
	time.Sleep(2 * time.Millisecond)
	p.mu.Lock()
	p.inFlight--
	p.mu.Unlock()

	sar.Status.Allowed = true
	return sar, nil
}

type probeAuthz struct {
	authorizationv1client.AuthorizationV1Interface
	probe *concurrencyProbe
}

func (p probeAuthz) SubjectAccessReviews() authorizationv1client.SubjectAccessReviewInterface {
	return p.probe
}

// The worker pool is what keeps a big page from opening a connection per row. Nothing else would
// notice if the bound went away: the answers would still be right.
func TestResolveAccessKeepsTheWorkerBound(t *testing.T) {
	kinds := fake.NewSimpleClientset()
	kinds.Resources = []*metav1.APIResourceList{{
		GroupVersion: "apps/v1",
		APIResources: []metav1.APIResource{{Name: "deployments", Kind: "Deployment", Namespaced: true}},
	}}
	probe := &concurrencyProbe{}
	checker := newAccessChecker(&kubeClients{
		authz:     probeAuthz{probe: probe},
		discovery: kinds.Discovery(),
	})

	// Fifty distinct questions: one namespace each, so none of them share an answer.
	var namespaces []ssrResourceNamespace
	for i := range 50 {
		namespaces = append(namespaces, ssrResourceNamespace{
			Name:      fmt.Sprintf("ns-%d", i),
			Resources: []ssrResource{{Group: "apps", Kind: "Deployment", Name: "checkout"}},
		})
	}

	resolveAccess(context.Background(), checker, rbacIdentity{Username: "dev"}, namespaces)

	probe.mu.Lock()
	defer probe.mu.Unlock()
	if probe.peak > 8 {
		t.Errorf("%d reviews were in flight at once, want at most 8", probe.peak)
	}
	if probe.peak < 2 {
		t.Errorf("the reviews never overlapped (peak %d), so this test proves nothing", probe.peak)
	}
}

// The reader closed the tab. Whatever was still queued stays unanswered, and an unanswered question
// hides its row rather than showing it.
func TestAGoneReaderLeavesTheRestUnanswered(t *testing.T) {
	s, clients := scopedServer(t, func(string, string) bool { return true })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	answers := resolveAccess(ctx, s.checkerFor(clients), rbacIdentity{Username: "dev"}, twoNamespaces())
	if len(answers) == 0 {
		t.Fatal("every question should still be in the map, unanswered")
	}
	for q, a := range answers {
		if a.determined || a.allowed {
			t.Errorf("%+v was answered after the reader left: %+v", q, a)
		}
	}
}

// Rows left unasked because the reader closed the tab are not a failure, and must not be logged as
// one: an operator reading that line would go looking for an API server problem that never happened.
func TestAGoneReaderIsNotLoggedAsAFailedReview(t *testing.T) {
	s, clients := scopedServer(t, func(string, string) bool { return true })
	c := requestWithIdentity(t)

	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(restore) })

	ctx, cancel := context.WithCancel(c.Request().Context())
	cancel()
	c.SetRequest(c.Request().WithContext(ctx))

	kept, unverified := s.scopeToReader(c, s.checkerFor(clients), twoNamespaces())
	if len(kept) != 0 || unverified == 0 {
		t.Fatalf("a cancelled render should hide every row and count them, got %d kept, %d unverified", len(kept), unverified)
	}
	if strings.Contains(logged.String(), "left out of the Resources view") {
		t.Errorf("the reader leaving was logged as a failed review: %s", logged.String())
	}
}

// A checker that is refused outright already reports itself, once per window, with the remediation.
// Repeating a weaker version of it on every render buries that line instead of supporting it.
func TestARefusedCheckerDoesNotAlsoWarnPerRender(t *testing.T) {
	s, clients := scopedServer(t, func(string, string) bool { return true })
	refused := s.checkerFor(clients)
	refused.reportReviewFailure(apierrors.NewForbidden(schema.GroupResource{Resource: "subjectaccessreviews"}, "", errors.New("no")))

	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(restore) })

	kept, unverified := s.scopeToReader(requestWithIdentity(t), refused, twoNamespaces())
	if len(kept) != 0 || unverified == 0 {
		t.Fatalf("a refused checker should hide every row and count them, got %d kept, %d unverified", len(kept), unverified)
	}
	if strings.Contains(logged.String(), "left out of the Resources view") {
		t.Errorf("the refusal was reported twice, the second time without the remediation: %s", logged.String())
	}
}

// A panic below the worker must cost one row, not the process: echo's recover covers the request
// goroutine, and these are not it.
func TestAPanicWhileCheckingCostsOneRow(t *testing.T) {
	client := sarClient(func(*authorizationv1.SubjectAccessReview) (bool, error) {
		panic("the client blew up")
	})
	client.Resources = []*metav1.APIResourceList{{
		GroupVersion: "apps/v1",
		APIResources: []metav1.APIResource{{Name: "deployments", Kind: "Deployment", Namespaced: true}},
	}, {
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{{Name: "pods", Kind: "Pod", Namespaced: true}},
	}}
	checker := newAccessChecker(&kubeClients{authz: client.AuthorizationV1(), discovery: client.Discovery()})

	answers := resolveAccess(context.Background(), checker, rbacIdentity{Username: "dev"}, twoNamespaces())
	for q, a := range answers {
		if a.determined || a.allowed {
			t.Errorf("%+v survived a panic as an answer: %+v", q, a)
		}
	}
}

func TestScopingKeepsOnlyWhatTheReaderMayRead(t *testing.T) {
	s, clients := scopedServer(t, func(namespace, _ string) bool { return namespace == "mine" })
	c := requestWithIdentity(t)

	kept, unverified := s.scopeToReader(c, s.checkerFor(clients), twoNamespaces())

	if unverified != 0 {
		t.Errorf("every review answered, so nothing is unverified: got %d", unverified)
	}
	if len(kept) != 1 || kept[0].Name != "mine" {
		t.Fatalf("a namespace the reader cannot read must disappear entirely, got %+v", kept)
	}
	if len(kept[0].Resources) != 1 || kept[0].Resources[0].Name != "checkout-api" {
		t.Errorf("wrong rows kept: %+v", kept[0].Resources)
	}
	if kept[0].Deny != 1 || kept[0].Total() != 1 {
		t.Errorf("the counts must be recomputed from what survives, got deny=%d total=%d",
			kept[0].Deny, kept[0].Total())
	}
}

func TestScopingCountsWhatItCouldNotCheck(t *testing.T) {
	// A Kind discovery does not know: the mapping fails, so the row is hidden and counted.
	s, clients := scopedServer(t, func(string, string) bool { return true })
	unknown := resourceModel([]ssrConstraint{{Name: "c", Kind: "K", Violations: []ssrConstraintViolation{
		viol("acme.example.com", "Widget", "mine", "thing", "deny"),
	}}})

	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(restore) })

	kept, unverified := s.scopeToReader(requestWithIdentity(t), s.checkerFor(clients), unknown)
	if len(kept) != 0 {
		t.Errorf("an unmappable Kind must not be shown, got %+v", kept)
	}
	if unverified != 1 {
		t.Errorf("the reader must be told a row is missing, got unverified=%d", unverified)
	}
	// The page says only that rows are missing, so the operator's copy of the number is the log.
	if !strings.Contains(logged.String(), `"resources":1`) {
		t.Errorf("the count the page no longer prints must reach the log, got %s", logged.String())
	}
}

// The warning that covers a partial failure must not count out loud: how many rows are missing
// measures what this reader may not see, the same fact the empty page withholds.
func TestThePartialFailureWarningDoesNotCount(t *testing.T) {
	page := renderSSR(t, "resources", map[string]any{
		"Audited": true, "Namespaces": twoNamespaces(), "Unverified": 3,
		"Layout": ssrLayout{Title: "t", Version: appVersion, AssetBase: "/static", Scoped: true},
	})
	if strings.Contains(page, "3 resource") {
		t.Error("the page printed how many rows it could not check")
	}
	if !strings.Contains(page, "Some resources are not listed") {
		t.Error("the reader must still be told the page is incomplete")
	}
}

func TestScopingWithoutAnIdentityShowsNothing(t *testing.T) {
	s, clients := scopedServer(t, func(string, string) bool { return true })
	// A context with no session at all: the reviews cannot name a subject.
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/resources", nil), httptest.NewRecorder())

	rows := twoNamespaces()
	total := 0
	for _, ns := range rows {
		total += len(ns.Resources)
	}

	kept, unverified := s.scopeToReader(c, s.checkerFor(clients), rows)
	if len(kept) != 0 {
		t.Errorf("no identity means no rows, got %+v", kept)
	}
	// Unverified, not denied: GPM never asked about these, so the page must not report a refusal.
	if unverified != total {
		t.Errorf("every row should count as unchecked, got %d of %d", unverified, total)
	}
}

// An empty page has three reasons, and they are not interchangeable: GPM cannot ask (no grant), GPM
// did not ask (no usable session), or GPM asked and the answer was no. Only the last one is about
// the reader's permissions, and only it may say so.
func TestAnEmptyPageNamesTheRightReason(t *testing.T) {
	layout := ssrLayout{Title: "t", Version: appVersion, AssetBase: "/static", Scoped: true}
	empty := []ssrResourceNamespace{}

	cannotAsk := renderSSR(t, "resources", map[string]any{
		"Audited": true, "Namespaces": empty, "Misconfigured": true, "Unverified": 3, "Layout": layout,
	})
	if !strings.Contains(cannotAsk, "create subjectaccessreviews") {
		t.Error("a missing grant must name the grant")
	}

	didNotAsk := renderSSR(t, "resources", map[string]any{
		"Audited": true, "Namespaces": empty, "Unverified": 3, "Layout": layout,
	})
	if !strings.Contains(didNotAsk, "could not confirm your access") {
		t.Error("an unverifiable session must say the access was not confirmed")
	}
	if strings.Contains(didNotAsk, "Kubernetes account can read") {
		t.Error("this page reports a denial GPM never asked for")
	}

	denied := renderSSR(t, "resources", map[string]any{
		"Audited": true, "Namespaces": empty, "Unverified": 0, "Layout": layout,
	})
	if !strings.Contains(denied, "Kubernetes account can read") {
		t.Error("an answered no must still say the objects are not theirs to read")
	}
}

// requestWithIdentity returns a context that has been through the session middleware and carries an
// RBAC identity, the way the auth middleware leaves it. Built by serving a real request: session.Get
// reads the store the middleware attaches, and a hand-made context has none.
func requestWithIdentity(t *testing.T) echo.Context {
	t.Helper()
	t.Cleanup(viper.Reset)
	viper.Set("secret_key", "a-test-key-that-is-not-the-default")

	e := echo.New()
	e.Use(session.Middleware(newSessionStore()))
	var captured echo.Context
	e.GET("/resources", func(c echo.Context) error {
		sess, err := session.Get(sessionName, c)
		if err != nil {
			t.Fatalf("session: %v", err)
		}
		sess.Values[sessionKeyRBACUser] = "dev"
		sess.Values[sessionKeyRBACGroups] = []string{"team-a"}
		captured = c
		return c.NoContent(http.StatusOK)
	})
	e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/resources", nil))
	if captured == nil {
		t.Fatal("the handler never ran")
	}
	return captured
}

// GPM refuses to start in a mode where the feature cannot be enforced: serving every view to
// everyone while the operator believes access is scoped is worse than not starting.
func TestUnsupportedRBACModesStopStartup(t *testing.T) {
	for _, tt := range []struct {
		name     string
		filter   bool
		auth     string
		contexts int
		wantErr  string
	}{
		{"off: nothing to check", false, "Anonymous", 5, ""},
		{"on, OIDC, one cluster: supported", true, "OIDC", 1, ""},
		{"on without authentication", true, "Anonymous", 1, "needs authentication"},
		{"on with more than one cluster", true, "OIDC", 3, "does not support more than one cluster"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(viper.Reset)
			viper.Set("rbac_filtering", tt.filter)
			viper.Set("auth_enabled", tt.auth)

			err := (&server{k8s: registryWithContexts(tt.contexts)}).checkRBACConfig()
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("this mode is supported, got %v", err)
			case tt.wantErr != "" && err == nil:
				t.Error("this mode cannot enforce the feature and must stop startup")
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Errorf("the error must say what is wrong; got %q, want it to mention %q", err, tt.wantErr)
			}
			// The message names counts, never the contexts themselves.
			if err != nil && strings.Contains(err.Error(), "cluster-0") {
				t.Errorf("context names must not reach an error message: %q", err)
			}
		})
	}
}

// A typo in a boolean setting must not be read as "off". Both of GPM's booleans switch off a
// protection -- one scopes what a user sees, the other verifies the API server's certificate -- so
// a misspelling that silently disables them is the worst possible reading.
func TestATypoInABooleanSettingStopsStartup(t *testing.T) {
	for _, tt := range []struct {
		name, key, value, want string
	}{
		{"the reported typo", "rbac_filtering", "treu", "GPM_RBAC_FILTERING"},
		{"a word that is not a boolean", "rbac_filtering", "yes please", "GPM_RBAC_FILTERING"},
		{"the same for TLS verification", "skip_tls_verify", "ture", "GPM_SKIP_TLS_VERIFY"},
		{"accepted spellings still work", "rbac_filtering", "TRUE", ""},
		{"and so do the numeric ones", "skip_tls_verify", "1", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(viper.Reset)
			viper.Set("auth_enabled", "OIDC")
			viper.Set(tt.key, tt.value)

			err := (&server{k8s: registryWithContexts(1)}).checkConfig()
			if tt.want == "" {
				if err != nil {
					t.Errorf("%q is a valid boolean, got %v", tt.value, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("%s=%q was accepted as a boolean and silently read as false", tt.want, tt.value)
			}
			if !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), tt.value) {
				t.Errorf("the error must name the variable and the value it got: %q", err)
			}
		})
	}
}

// An empty scoped page must say nothing about the rest of the cluster. "Nothing is breaking a
// policy" is a fact about every namespace, and so is its negation: telling a reader who may see
// none of the violations that some exist hands them a fact their account cannot read.
func TestAnEmptyScopedPageRevealsNothingAboutTheCluster(t *testing.T) {
	scoped := renderSSR(t, "resources", map[string]any{
		"Audited": true, "Namespaces": []ssrResourceNamespace{},
		"Layout": ssrLayout{Title: "t", Version: appVersion, AssetBase: "/static", Scoped: true},
	})
	for _, disclosure := range []string{"no violations in this cluster", "violations in this cluster"} {
		if strings.Contains(scoped, disclosure) {
			t.Errorf("the scoped empty page states %q, which is a fact about namespaces this reader cannot read", disclosure)
		}
	}
	if !strings.Contains(scoped, "Nothing here for you") {
		t.Error("the page must still tell the reader why it is empty for them")
	}

	// Unscoped, the reader sees the whole cluster, so the whole-cluster sentence is theirs to read.
	clean := renderSSR(t, "resources", map[string]any{
		"Audited": true, "Namespaces": []ssrResourceNamespace{},
	})
	if !strings.Contains(clean, "no violations in this cluster") {
		t.Error("a genuinely clean cluster should still say so")
	}
}

// The refusal has to explain itself: a bare "Error" page tells a reader nothing about why the view
// is missing or what to do about it.
func TestTheRefusalPageExplainsItself(t *testing.T) {
	page := renderSSR(t, "error", map[string]any{
		"Err": ssrErrorView{
			Heading:     "Not allowed",
			Message:     "You do not have access to this view.",
			Action:      "GPM shows you the Resources view, with the objects your Kubernetes account can read.",
			Description: "If you think this is a mistake, contact a cluster administrator.",
			BackURL:     "/resources",
		},
	})
	for _, want := range []string{
		"You do not have access to this view.",
		"objects your Kubernetes account can read",
		"contact a cluster administrator",
		`href="/resources"`,
		"Not allowed",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the refusal page is missing %q", want)
		}
	}
	// A refusal must not describe what it is hiding.
	for _, leak := range []string{"cluster-wide policy", "Constraint", "Mutation", "Events"} {
		if strings.Contains(page, leak) {
			t.Errorf("the refusal page describes the views it hid: found %q", leak)
		}
	}

	// A boundary is not a fault: no alarm mark, no red box, nothing that reads as "report this".
	for _, unwanted := range []string{"&#9888;", "alert-error"} {
		if strings.Contains(page, unwanted) {
			t.Errorf("the refusal page still looks like a malfunction: found %q", unwanted)
		}
	}

	// A real error keeps both.
	fault := renderSSR(t, "error", map[string]any{
		"Err": ssrErrorView{Message: "Something broke.", Description: "details", BackURL: "/"},
	})
	if !strings.Contains(fault, "&#9888;") || !strings.Contains(fault, "alert-error") {
		t.Error("an actual error must keep the warning mark and the red box")
	}
}

// GPM refuses to start in a mode where the feature cannot be enforced: serving every view to
// everyone while the operator believes access is scoped is worse than not starting.

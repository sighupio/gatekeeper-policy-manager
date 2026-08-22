// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Drives the whole OIDC login against a stand-in identity provider: discovery, the redirect out,
// the code exchange, ID token verification, and the session that results. The unit tests cover the
// pieces; this covers them wired together, which is where the security-relevant mistakes live.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
)

type fakeProvider struct {
	server *httptest.Server
	key    *rsa.PrivateKey
	// Each test builds its own fakeProvider, and every write precedes the ServeHTTP that hands it
	// to a handler, so the fields are never shared across goroutines without ordering -- no mutex
	// needed, even under t.Parallel. (The blocker to parallel tests is the global viper that
	// useTestSettings mutates, not this struct.)
	// Overrides for the next ID token, so tests can forge a bad one.
	nonceOverride string
	audOverride   string
	// When set, /.well-known returns 500 so a test can prove manual-endpoint mode never calls it.
	breakDiscovery bool
	// Recorded from the authorize request so /token can check the PKCE verifier against it.
	challenge       string
	challengeMethod string
	verifierSeen    string
	pkceOK          bool
}

func newFakeProvider(t *testing.T) *fakeProvider {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating the provider key failed: %v", err)
	}
	p := &fakeProvider{key: key}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		if p.breakDiscovery {
			http.Error(w, "discovery disabled for this test", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{
			"issuer":                 p.issuer(),
			"authorization_endpoint": p.issuer() + "/authorize",
			"token_endpoint":         p.issuer() + "/token",
			"jwks_uri":               p.issuer() + "/jwks",
			"end_session_endpoint":   p.issuer() + "/logout",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key: key.Public(), KeyID: "test", Algorithm: "RS256", Use: "sig",
		}}})
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		p.challenge = r.URL.Query().Get("code_challenge")
		p.challengeMethod = r.URL.Query().Get("code_challenge_method")
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		// Check PKCE exactly as a real provider would: S256(verifier) must equal the challenge
		// recorded at the authorize step.
		p.verifierSeen = r.Form.Get("code_verifier")
		sum := sha256.Sum256([]byte(p.verifierSeen))
		p.pkceOK = p.challenge != "" && base64.RawURLEncoding.EncodeToString(sum[:]) == p.challenge
		// The code carries the nonce so the stub does not have to keep state.
		nonce := r.Form.Get("code")
		if p.nonceOverride != "" {
			nonce = p.nonceOverride
		}
		aud := "gpm"
		if p.audOverride != "" {
			aud = p.audOverride
		}
		writeJSON(w, map[string]any{
			"access_token": "access-token",
			"token_type":   "Bearer",
			"id_token":     p.idToken(t, aud, nonce),
		})
	})

	p.server = httptest.NewServer(mux)
	t.Cleanup(p.server.Close)
	return p
}

func (p *fakeProvider) issuer() string { return p.server.URL }

func (p *fakeProvider) idToken(t *testing.T, aud, nonce string) string {
	t.Helper()

	claims := map[string]any{
		"iss":                p.issuer(),
		"aud":                aud,
		"sub":                "user-1",
		"exp":                time.Now().Add(time.Hour).Unix(),
		"iat":                time.Now().Unix(),
		"nonce":              nonce,
		"preferred_username": "ramiro",
		"email":              "ramiro@example.com",
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("encoding the claims failed: %v", err)
	}

	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: p.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "test"),
	)
	if err != nil {
		t.Fatalf("building the signer failed: %v", err)
	}
	sig, err := signer.Sign(payload)
	if err != nil {
		t.Fatalf("signing the ID token failed: %v", err)
	}
	out, err := sig.CompactSerialize()
	if err != nil {
		t.Fatalf("serialising the ID token failed: %v", err)
	}
	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// Configures GPM against the stand-in provider and returns an echo instance with the session and
// auth middleware installed, exactly as main() wires them.
func newAuthTestServer(t *testing.T, p *fakeProvider) (*echo.Echo, *authenticator) {
	t.Helper()
	return newAuthTestServerOnSubpath(t, p, "")
}

// The same, for a GPM served from a subpath. The routes stay prefix-less, because the reverse
// proxy strips the prefix before GPM sees the request -- that asymmetry is the whole reason the
// base path handling exists.
func newAuthTestServerOnSubpath(t *testing.T, p *fakeProvider, base string, extra ...map[string]any) (*echo.Echo, *authenticator) {
	t.Helper()

	useTestSettings(t)
	settings := map[string]any{
		"base_path":            base,
		"auth_enabled":         "OIDC",
		"secret_key":           "test-secret-key",
		"preferred_url_scheme": "http",
		"session_max_age":      3600,
		"oidc_issuer":          p.issuer(),
		"oidc_client_id":       "gpm",
		"oidc_client_secret":   "gpm-secret",
		"oidc_redirect_domain": "http://gpm.example.com",
	}
	for _, m := range extra {
		for k, v := range m {
			settings[k] = v
		}
	}
	for k, v := range settings {
		viper.Set(k, v)
	}

	auth, err := newAuthenticator(context.Background())
	if err != nil {
		t.Fatalf("configuring the authenticator failed: %v", err)
	}
	// In production this renders the SSR "signed out" page; the tests only exercise the redirect
	// behaviour of logout, so a stub stands in for the template layer.
	auth.renderLoggedOut = func(c echo.Context) error { return c.String(http.StatusOK, "signed out") }
	// The real one renders the SSR error page; this keeps the assertions readable while still
	// proving the callback answers with a page and not with JSON.
	auth.renderError = func(c echo.Context, status int, e ssrErrorView) error {
		return c.HTML(status, "<h1>Error</h1><p>"+e.Message+"</p><p>"+e.Description+"</p>"+
			`<a href="`+e.LoginURL+`">Log in</a>`)
	}

	e := echo.New()
	e.Use(session.Middleware(newSessionStore()))
	e.Use(auth.middleware())
	e.GET(callbackPath, auth.callback)
	e.GET("/login", auth.login)
	e.GET("/logout", auth.logout)
	e.GET("/constraints", func(c echo.Context) error { return c.String(http.StatusOK, "protected") })
	return e, auth
}

// Discovery has to find the end_session_endpoint too, since RP-initiated logout depends on it.
func TestDiscoveryReadsTheProvider(t *testing.T) {
	p := newFakeProvider(t)
	_, auth := newAuthTestServer(t, p)

	if auth.oauth2.Endpoint.AuthURL != p.issuer()+"/authorize" {
		t.Errorf("authorization endpoint = %q", auth.oauth2.Endpoint.AuthURL)
	}
	if auth.endSessionEndpoint != p.issuer()+"/logout" {
		t.Errorf("end session endpoint = %q, want it discovered", auth.endSessionEndpoint)
	}
	if auth.oauth2.RedirectURL != "http://gpm.example.com/oidc-auth" {
		t.Errorf("redirect URL = %q", auth.oauth2.RedirectURL)
	}
}

// The full round trip: an unauthenticated page request is redirected to the provider, the callback
// is accepted, and the original destination is restored.
func TestLoginFlow(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServer(t, p)

	// 1. Ask for a protected page without a session.
	req := httptest.NewRequest(http.MethodGet, "/constraints?foo=bar", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want a redirect to the provider", rec.Code)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parsing the redirect failed: %v", err)
	}
	if !strings.HasPrefix(loc.String(), p.issuer()+"/authorize") {
		t.Fatalf("redirected to %q, want the provider's authorize endpoint", loc)
	}
	state := loc.Query().Get("state")
	nonce := loc.Query().Get("nonce")
	if state == "" || nonce == "" {
		t.Fatalf("state=%q nonce=%q, both must be sent", state, nonce)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie was set before the redirect")
	}

	// 2. Come back on the callback the way the provider would. The stub mints an ID token whose
	//    nonce is whatever we pass as the code.
	req = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("%s?state=%s&code=%s", callbackPath, url.QueryEscape(state), url.QueryEscape(nonce)), nil)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("callback status = %d (%s), want a redirect", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/constraints?foo=bar" {
		t.Errorf("redirected to %q, want the originally requested page including its query", got)
	}

	// 3. The session from the callback should now open the protected page.
	sessionCookies := rec.Result().Cookies()
	req = httptest.NewRequest(http.MethodGet, "/constraints", nil)
	for _, ck := range sessionCookies {
		req.AddCookie(ck)
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "protected" {
		t.Errorf("after login: status = %d body = %q, want the protected page", rec.Code, rec.Body.String())
	}
}

// A callback whose state does not match the session must be refused, or the login is open to CSRF.
func TestCallbackRejectsWrongState(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServer(t, p)

	req := httptest.NewRequest(http.MethodGet, "/constraints", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	cookies := rec.Result().Cookies()

	req = httptest.NewRequest(http.MethodGet, callbackPath+"?state=not-the-stored-state&code=abc", nil)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d for a mismatched state", rec.Code, http.StatusUnauthorized)
	}
}

// A replayed ID token from another login attempt has the wrong nonce and must be refused.
func TestCallbackRejectsWrongNonce(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServer(t, p)
	p.nonceOverride = "some-other-login-attempt"

	req := httptest.NewRequest(http.MethodGet, "/constraints", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	loc, _ := url.Parse(rec.Header().Get("Location"))
	cookies := rec.Result().Cookies()

	req = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("%s?state=%s&code=whatever", callbackPath, url.QueryEscape(loc.Query().Get("state"))), nil)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d for a mismatched nonce", rec.Code, http.StatusUnauthorized)
	}
	// Assert why it was rejected, so a broken stub cannot pass as a working nonce check.
	assertAnsweredWithAPage(t, rec)
	if !strings.Contains(rec.Body.String(), "nonce") {
		t.Errorf("rejection page = %q, want it to mention the nonce", rec.Body.String())
	}
}

// An ID token minted for a different client must not be accepted.
func TestCallbackRejectsWrongAudience(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServer(t, p)
	p.audOverride = "some-other-client"

	req := httptest.NewRequest(http.MethodGet, "/constraints", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	loc, _ := url.Parse(rec.Header().Get("Location"))
	cookies := rec.Result().Cookies()

	req = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("%s?state=%s&code=%s", callbackPath,
			url.QueryEscape(loc.Query().Get("state")), url.QueryEscape(loc.Query().Get("nonce"))), nil)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d for a token minted for another client", rec.Code, http.StatusUnauthorized)
	}
	assertAnsweredWithAPage(t, rec)
	if !strings.Contains(rec.Body.String(), "audience") {
		t.Errorf("rejection page = %q, want it to mention the audience", rec.Body.String())
	}
}

// Logs in for real, then logs out, then follows the hop the provider makes back to GPM.
//
// The provider's post_logout_redirect_uri points at /logout, so this endpoint receives its own
// redirect target. Answering with another redirect to the provider loops until the browser gives
// up with ERR_TOO_MANY_REDIRECTS — which is exactly what happened against a real Keycloak, and
// what an earlier version of this test missed by only ever checking the first hop.
func TestLogoutRoundTripDoesNotLoop(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServer(t, p)

	sessionCookies := loginForTest(t, e, p)

	// First hop: a real session, so GPM hands over to the provider.
	req := httptest.NewRequest(http.MethodGet, "/logout", nil)
	for _, ck := range sessionCookies {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("first /logout status = %d, want a redirect to the provider", rec.Code)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parsing the redirect failed: %v", err)
	}
	if !strings.HasPrefix(loc.String(), p.issuer()+"/logout") {
		t.Fatalf("redirected to %q, want the end session endpoint", loc)
	}
	postLogout := loc.Query().Get("post_logout_redirect_uri")
	if postLogout != "http://gpm.example.com/logout" {
		t.Fatalf("post_logout_redirect_uri = %q", postLogout)
	}

	// Second hop: the provider sends the browser back to post_logout_redirect_uri, carrying
	// whatever cookies survived the first response.
	req = httptest.NewRequest(http.MethodGet, "/logout", nil)
	for _, ck := range rec.Result().Cookies() {
		req.AddCookie(ck)
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// The property under test: the return hop lands on a page, it does not redirect again. The
	// local logout path renders an embedded SSR page, so nothing external has to be served.
	if rec.Code == http.StatusFound || rec.Header().Get("Location") != "" {
		t.Fatalf("the return hop redirected again to %q — this is the redirect loop",
			rec.Header().Get("Location"))
	}
}

// /login gives the frontend somewhere to send the user after a 401, and honors ?next=.
func TestLoginRouteStartsTheFlow(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServer(t, p)

	req := httptest.NewRequest(http.MethodGet, "/login?next=/mutations", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want a redirect to the provider", rec.Code)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parsing the redirect failed: %v", err)
	}
	if !strings.HasPrefix(loc.String(), p.issuer()+"/authorize") {
		t.Fatalf("redirected to %q, want the provider", loc)
	}

	// Finish the login and confirm ?next= is where we land.
	req = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("%s?state=%s&code=%s", callbackPath,
			url.QueryEscape(loc.Query().Get("state")), url.QueryEscape(loc.Query().Get("nonce"))), nil)
	for _, ck := range rec.Result().Cookies() {
		req.AddCookie(ck)
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if got := rec.Header().Get("Location"); got != "/mutations" {
		t.Errorf("landed on %q, want the ?next= target", got)
	}
}

// ?next= is attacker-controllable, so an off-site value must not be honored.
func TestLoginRouteRefusesOffsiteNext(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServer(t, p)

	req := httptest.NewRequest(http.MethodGet, "/login?next=//evil.com", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	loc, _ := url.Parse(rec.Header().Get("Location"))

	req = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("%s?state=%s&code=%s", callbackPath,
			url.QueryEscape(loc.Query().Get("state")), url.QueryEscape(loc.Query().Get("nonce"))), nil)
	for _, ck := range rec.Result().Cookies() {
		req.AddCookie(ck)
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if got := rec.Header().Get("Location"); got != "/" {
		t.Errorf("landed on %q, want it forced back to %q", got, "/")
	}
}

// Hitting /login while already signed in should not restart the whole dance.
func TestLoginRouteWhenAlreadySignedIn(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServer(t, p)

	sessionCookies := loginForTest(t, e, p)

	req := httptest.NewRequest(http.MethodGet, "/login?next=/events", nil)
	for _, ck := range sessionCookies {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if got := rec.Header().Get("Location"); got != "/events" {
		t.Errorf("redirected to %q, want to go straight to the requested page", got)
	}
}

// Logging out without ever having logged in must also render the page rather than bouncing.
func TestLogoutWithoutASessionDoesNotRedirect(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServer(t, p)

	req := httptest.NewRequest(http.MethodGet, "/logout", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code == http.StatusFound || rec.Header().Get("Location") != "" {
		t.Errorf("redirected to %q with no session to end", rec.Header().Get("Location"))
	}
}

// Runs the whole login and returns the cookies that carry the resulting session.
func loginForTest(t *testing.T, e *echo.Echo, p *fakeProvider) []*http.Cookie {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/constraints", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parsing the login redirect failed: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("%s?state=%s&code=%s", callbackPath,
			url.QueryEscape(loc.Query().Get("state")), url.QueryEscape(loc.Query().Get("nonce"))), nil)
	for _, ck := range rec.Result().Cookies() {
		req.AddCookie(ck)
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("login did not complete: status %d (%s)", rec.Code, rec.Body.String())
	}
	return rec.Result().Cookies()
}

// PKCE protects the authorization code when the client is public, which GPM allows because
// GPM_OIDC_CLIENT_SECRET is optional. Assert it end to end: the challenge goes out with the
// authorize request, and the verifier that comes back on the token request hashes to it.
func TestLoginUsesPKCE(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServer(t, p)

	req := httptest.NewRequest(http.MethodGet, "/constraints", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parsing the redirect failed: %v", err)
	}
	challenge := loc.Query().Get("code_challenge")
	if challenge == "" {
		t.Fatal("no code_challenge on the authorize request; PKCE is not being used")
	}
	if got := loc.Query().Get("code_challenge_method"); got != "S256" {
		t.Errorf("code_challenge_method = %q, want S256 (plain offers no protection)", got)
	}
	// The stub records what the authorize step saw so /token can verify against it.
	p.challenge = challenge
	p.challengeMethod = loc.Query().Get("code_challenge_method")

	req = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("%s?state=%s&code=%s", callbackPath,
			url.QueryEscape(loc.Query().Get("state")), url.QueryEscape(loc.Query().Get("nonce"))), nil)
	for _, ck := range rec.Result().Cookies() {
		req.AddCookie(ck)
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	verifierSeen, pkceOK := p.verifierSeen, p.pkceOK
	if verifierSeen == "" {
		t.Fatal("no code_verifier sent on the token request; the exchange is not completing PKCE")
	}
	if !pkceOK {
		t.Errorf("the code_verifier does not hash to the challenge; verifier was %q", verifierSeen)
	}
}

// Everything below is the subpath deployment: GPM behind a proxy that serves it at /gpm and
// removes that prefix before forwarding. GPM therefore sees prefix-less paths and has to put the
// prefix back on every path it sends to the browser, or the browser leaves the proxy's location
// and gets a 404 from whatever else is on the domain.

// The full round trip, as TestLoginFlow does at the root: the user must come back to the page they
// asked for, under the subpath.
func TestLoginFlowOnASubpathReturnsInsideTheApp(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServerOnSubpath(t, p, "/gpm")

	// The proxy has already removed /gpm, so this is what GPM receives for /gpm/constraints.
	req := httptest.NewRequest(http.MethodGet, "/constraints?foo=bar", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parsing the redirect to the provider failed: %v", err)
	}
	if got, want := loc.Query().Get("redirect_uri"), "http://gpm.example.com/gpm/oidc-auth"; got != want {
		t.Errorf("redirect_uri = %q, want %q — the provider would send the browser outside the app", got, want)
	}

	req = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("%s?state=%s&code=%s", callbackPath,
			url.QueryEscape(loc.Query().Get("state")), url.QueryEscape(loc.Query().Get("nonce"))), nil)
	for _, ck := range rec.Result().Cookies() {
		req.AddCookie(ck)
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("callback status = %d, want a redirect (%s)", rec.Code, rec.Body.String())
	}
	if got, want := rec.Header().Get("Location"), "/gpm/constraints?foo=bar"; got != want {
		t.Errorf("landed on %q after logging in, want %q", got, want)
	}
}

// The frontend renders this as the "Log in" button's href, so it is followed by the browser.
func TestUnauthorizedAPIAnswerPointsInsideTheApp(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServerOnSubpath(t, p, "/gpm")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/constraints", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	var answer ErrorAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decoding the answer failed: %v (%s)", err, rec.Body.String())
	}
	if got, want := answer.LoginURL, "/gpm/login"; got != want {
		t.Errorf("login_url = %q, want %q", got, want)
	}
}

func TestLogoutReturnsInsideTheApp(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServerOnSubpath(t, p, "/gpm")

	req := httptest.NewRequest(http.MethodGet, "/logout", nil)
	for _, ck := range loginForTest(t, e, p) {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parsing the redirect failed: %v", err)
	}
	if got, want := loc.Query().Get("post_logout_redirect_uri"), "http://gpm.example.com/gpm/logout"; got != want {
		t.Errorf("post_logout_redirect_uri = %q, want %q", got, want)
	}
}

// ?next= is built by the frontend, which knows only browser paths. Prefixing it a second time
// would send the user to /gpm/gpm/constraints.
func TestLoginRouteDoesNotPrefixTheSubpathTwice(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServerOnSubpath(t, p, "/gpm")

	req := httptest.NewRequest(http.MethodGet, "/login?next=%2Fgpm%2Fconstraints", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parsing the redirect to the provider failed: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("%s?state=%s&code=%s", callbackPath,
			url.QueryEscape(loc.Query().Get("state")), url.QueryEscape(loc.Query().Get("nonce"))), nil)
	for _, ck := range rec.Result().Cookies() {
		req.AddCookie(ck)
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if got, want := rec.Header().Get("Location"), "/gpm/constraints"; got != want {
		t.Errorf("landed on %q, want %q", got, want)
	}
}

// The same property as TestBackendPathCannotSmuggleAnOffsiteRedirect, but end to end: a hostile
// ?next= must not survive the whole login round trip on a subpath deployment. Three separate
// things stop it -- safeRedirectTarget in login(), safeRedirectTarget again in startLogin(), and
// browserPath prefixing the base path on the way out -- so this holds even if one of them moves.
func TestLoginRouteRefusesOffsiteNextOnASubpath(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServerOnSubpath(t, p, "/gpm")

	for _, next := range []string{"/gpm//evil.com", `/gpm/\evil.com`, "//evil.com", "https://evil.com"} {
		req := httptest.NewRequest(http.MethodGet, "/login?next="+url.QueryEscape(next), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		loc, err := url.Parse(rec.Header().Get("Location"))
		if err != nil {
			t.Fatalf("parsing the redirect to the provider failed: %v", err)
		}

		req = httptest.NewRequest(http.MethodGet,
			fmt.Sprintf("%s?state=%s&code=%s", callbackPath,
				url.QueryEscape(loc.Query().Get("state")), url.QueryEscape(loc.Query().Get("nonce"))), nil)
		for _, ck := range rec.Result().Cookies() {
			req.AddCookie(ck)
		}
		rec = httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		// Anything that a browser resolves off-site: an absolute URL, or a path starting with two
		// slashes or with a backslash after the first slash.
		landed := rec.Header().Get("Location")
		target, err := url.Parse(landed)
		if err != nil {
			t.Fatalf("parsing the post-login redirect failed: %v", err)
		}
		if target.IsAbs() || target.Host != "" ||
			strings.HasPrefix(landed, "//") || strings.HasPrefix(landed, `/\`) {
			t.Errorf("?next=%q landed the user on %q, which leaves the site", next, landed)
		}
		if want := "/gpm/"; landed != want {
			t.Errorf("?next=%q landed on %q, want the app root %q", next, landed, want)
		}
	}
}

// A cookie the store cannot decode -- after GPM_SECRET_KEY is rotated, or when it is truncated or
// tampered with -- must send the user to log in again, not fail the request.
//
// Getting this wrong makes a secret rotation strand every logged-in user: a 500 on every page, and
// no response carrying a Set-Cookie that could replace the bad value, so nothing the user does
// clears it and nothing the operator does server-side helps either.
func TestAStaleSessionCookieSendsTheUserToLogInAgain(t *testing.T) {
	for _, base := range []string{"", "/gpm"} {
		t.Run("base="+base, func(t *testing.T) {
			p := newFakeProvider(t)
			e, _ := newAuthTestServerOnSubpath(t, p, base)

			for _, value := range []string{
				"a-cookie-signed-with-the-previous-secret",
				"MTc1NDQwMDAwMHxEdi1CQkFFQ180SUFBUkFCRUFBQV8|bm90LWEtdmFsaWQtbWFj",
				"",
			} {
				req := httptest.NewRequest(http.MethodGet, "/constraints", nil)
				req.AddCookie(&http.Cookie{Name: sessionName, Value: value})
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)

				if rec.Code != http.StatusFound {
					t.Errorf("a stale cookie gave status %d, want a redirect to the provider (%s)",
						rec.Code, strings.TrimSpace(rec.Body.String()))
					continue
				}
				// A usable replacement, not just any Set-Cookie: on a subpath the legacy-root
				// deletion also appears here, and it carries no session at all.
				if !hasUsableSession(rec) {
					t.Errorf("the redirect set no usable session cookie, so the stale one survives: %v",
						rec.Result().Cookies())
				}
			}
		})
	}
}

// Logging out has to clear a cookie that does not decode. That is the one a stuck user most needs
// rid of, and it used to be the one case logout skipped.
func TestLogoutClearsACookieThatDoesNotDecode(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServerOnSubpath(t, p, "/gpm")

	req := httptest.NewRequest(http.MethodGet, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionName, Value: "a-cookie-signed-with-the-previous-secret"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var cleared bool
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == sessionName && ck.Path == "/gpm" && ck.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Errorf("logout did not expire the session cookie: %v", rec.Result().Cookies())
	}
}

// On a subpath deployment a cookie left at Path=/ by an earlier build shadows the scoped one and
// keeps reaching every other application on the host. GPM cannot overwrite it -- deletion has to
// match the path -- so it has to send an explicit expiry for it.
func TestTheLegacyRootCookieIsDeletedOnASubpath(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServerOnSubpath(t, p, "/gpm")

	for _, path := range []string{"/constraints", "/logout"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: sessionName, Value: "a-cookie-signed-with-the-previous-secret"})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		var deleted bool
		for _, ck := range rec.Result().Cookies() {
			if ck.Name == sessionName && ck.Path == "/" && ck.MaxAge < 0 {
				deleted = true
			}
		}
		if !deleted {
			t.Errorf("%s did not expire the root-scoped cookie: %v", path, rec.Result().Cookies())
		}
	}
}

// At the root, Path=/ is the live session cookie. Expiring it there would cancel the very cookie
// startLogin just wrote -- the one carrying the state, the nonce and the PKCE verifier -- so the
// login could never complete. The request has to be one that reaches startLogin, which means no
// session: with a session the middleware passes straight through and proves nothing.
func TestTheRootDeploymentNeverDeletesItsOwnCookie(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServerOnSubpath(t, p, "")

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/constraints", nil))

	var live int
	for _, ck := range rec.Result().Cookies() {
		if ck.Name != sessionName {
			continue
		}
		if ck.MaxAge < 0 || ck.Value == "" {
			t.Errorf("a root deployment expired its own session cookie: %+v", ck)
			continue
		}
		live++
	}
	if live == 0 {
		t.Error("startLogin set no usable session cookie at the root")
	}

	// And the flow still completes end to end.
	if cookies := loginForTest(t, e, p); len(cookies) == 0 {
		t.Error("the login round trip produced no session")
	}
}

// Each of the layers that keep a hostile ?next= on-site has to hold on its own. The end to end
// test cannot see any single one of them fail, because the other two still catch it.
func TestEachRedirectGuardHoldsIndependently(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServerOnSubpath(t, p, "/gpm")

	store, ok := newSessionStore().(*sessions.CookieStore)
	if !ok {
		t.Fatal("expected a CookieStore")
	}
	storedDestination := func(t *testing.T, rec *httptest.ResponseRecorder) interface{} {
		t.Helper()
		for _, ck := range rec.Result().Cookies() {
			if ck.Name != sessionName || ck.MaxAge < 0 {
				continue
			}
			values := map[interface{}]interface{}{}
			if err := securecookie.DecodeMulti(sessionName, ck.Value, &values, store.Codecs...); err != nil {
				t.Fatalf("decoding the session cookie failed: %v", err)
			}
			return values[sessionKeyDestination]
		}
		t.Fatal("no session cookie was set")
		return nil
	}

	// Layer 2: startLogin sanitizes whatever the middleware hands it. The middleware passes the
	// raw request URI, so a request for a hostile path is the real way in.
	// The proxy strips the prefix, so what reaches GPM is already prefix-less. A "/gpm/..." path
	// belongs to the ?next= layer below, not to this one.
	for _, target := range []string{"//evil.com", `/\evil.com`} {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))

		if got := storedDestination(t, rec); got != "/" {
			t.Errorf("a request for %q stored destination %v, want %q", target, got, "/")
		}
	}

	// Layer 3: even a destination planted straight into the session must not escape the callback.
	// Run at the root as well, because that is where this guard is load-bearing: with a base path,
	// browserPath prefixes the result and makes it same-origin whatever the destination was.
	for _, base := range []string{"", "/gpm"} {
		t.Run("callback base="+base, func(t *testing.T) {
			p := newFakeProvider(t)
			e, _ := newAuthTestServerOnSubpath(t, p, base)

			for _, target := range []string{"//evil.com", `/\evil.com`, "https://evil.com", "/gpm//evil.com"} {
				planted := startLoginWithPlantedDestination(t, e, target)

				req := httptest.NewRequest(http.MethodGet, planted.callbackURL, nil)
				for _, ck := range planted.jar {
					req.AddCookie(ck)
				}
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)

				// The property is same-origin, not the absence of a string: "/gpm/gpm//evil.com"
				// contains "evil.com" but stays inside the app.
				landed := rec.Header().Get("Location")
				u, err := url.Parse(landed)
				if err != nil {
					t.Fatalf("parsing the post-login redirect %q failed: %v", landed, err)
				}
				if u.IsAbs() || u.Host != "" ||
					strings.HasPrefix(landed, "//") || strings.HasPrefix(landed, `/\`) {
					t.Errorf("a planted destination %q landed the user on %q, which leaves the site",
						target, landed)
				}
			}
		})
	}
}

type plantedLogin struct {
	jar         []*http.Cookie
	callbackURL string
}

// Runs the first leg of a login, then rewrites the destination stored in the session cookie. This
// is what an attacker who could plant session state would have, and it is the only way to reach
// the callback's own check on the destination -- every other route sanitizes before storing.
func startLoginWithPlantedDestination(t *testing.T, e *echo.Echo, destination string) plantedLogin {
	t.Helper()

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))

	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parsing the redirect to the provider failed: %v", err)
	}

	// A separate store instance, but the keys come from the same secret, so the codecs match.
	store, ok := newSessionStore().(*sessions.CookieStore)
	if !ok {
		t.Fatal("expected a CookieStore")
	}

	var values map[interface{}]interface{}
	for _, ck := range rec.Result().Cookies() {
		// startLogin also expires the legacy root-scoped cookie, which carries no value.
		if ck.Name != sessionName || ck.MaxAge < 0 || ck.Value == "" {
			continue
		}
		values = map[interface{}]interface{}{}
		if err := securecookie.DecodeMulti(sessionName, ck.Value, &values, store.Codecs...); err != nil {
			t.Fatalf("decoding the login cookie failed: %v", err)
		}
	}
	if values == nil {
		t.Fatal("the login leg set no session cookie")
	}
	values[sessionKeyDestination] = destination

	encoded, err := securecookie.EncodeMulti(sessionName, values, store.Codecs...)
	if err != nil {
		t.Fatalf("re-encoding the session failed: %v", err)
	}

	return plantedLogin{
		jar: []*http.Cookie{{Name: sessionName, Value: encoded}},
		callbackURL: fmt.Sprintf("%s?state=%s&code=%s", callbackPath,
			url.QueryEscape(loc.Query().Get("state")), url.QueryEscape(loc.Query().Get("nonce"))),
	}
}

// A login whose keys changed between its start and its callback arrives here with a session that
// will not decode. The callback is a top-level navigation, so raw JSON in the address bar is the
// wrong answer: start a clean login instead.
func TestTheCallbackRestartsTheLoginWhenTheSessionIsUnreadable(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServerOnSubpath(t, p, "/gpm")

	req := httptest.NewRequest(http.MethodGet, callbackPath+"?state=whatever&code=whatever", nil)
	req.AddCookie(&http.Cookie{Name: sessionName, Value: "a-cookie-signed-with-the-previous-secret"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want a redirect into a fresh login (%s)",
			rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, p.issuer()+"/authorize") {
		t.Errorf("redirected to %q, want the provider's authorize endpoint", loc)
	}
	if !hasUsableSession(rec) {
		t.Errorf("the restart set no usable session cookie, so the login cannot complete: %v",
			rec.Result().Cookies())
	}
}

// The frontend's first request is usually an API call, and that branch answers 401 without going
// through startLogin. It still has to expire a root-scoped cookie, or that cookie keeps reaching
// every other application on the host until the user navigates.
func TestTheAPIAnswerAlsoClearsTheLegacyRootCookie(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServerOnSubpath(t, p, "/gpm")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/constraints", nil)
	req.AddCookie(&http.Cookie{Name: sessionName, Value: "a-cookie-signed-with-the-previous-secret"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	var deleted bool
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == sessionName && ck.Path == "/" && ck.MaxAge < 0 {
			deleted = true
		}
	}
	if !deleted {
		t.Errorf("the 401 did not expire the root-scoped cookie: %v", rec.Result().Cookies())
	}
}

// True when the response carries a session cookie the browser will keep and GPM will read back:
// scoped to the base path, not an expiry, and not empty. A bare count is not enough, because the
// legacy-root deletion is also a Set-Cookie named gpm-session.
func hasUsableSession(rec *httptest.ResponseRecorder) bool {
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == sessionName && ck.Path == cookiePath() && ck.MaxAge >= 0 && ck.Value != "" {
			return true
		}
	}
	return false
}

// A session made before the deployment was given a base path still decodes, so the request is
// authenticated and no login path runs. GPM has to move it onto the base path anyway, or its
// cookie keeps reaching every other application on the host until it expires.
func TestAValidRootSessionIsMovedOntoTheBasePath(t *testing.T) {
	p := newFakeProvider(t)

	// Log in with no base path, which produces a Path=/ cookie.
	rootServer, _ := newAuthTestServerOnSubpath(t, p, "")
	rootCookies := loginForTest(t, rootServer, p)

	// The same GPM, now served from a subpath. Same secret, so the cookie still decodes.
	e, _ := newAuthTestServerOnSubpath(t, p, "/gpm")

	req := httptest.NewRequest(http.MethodGet, "/constraints", nil)
	for _, ck := range rootCookies {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// Still logged in: this must not cost the user their session.
	if rec.Code == http.StatusFound {
		t.Fatalf("the request was bounced to the provider, so the migration logged the user out")
	}
	if !hasUsableSession(rec) {
		t.Errorf("no /gpm-scoped session was written: %v", rec.Result().Cookies())
	}
	var rootExpired bool
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == sessionName && ck.Path == "/" && ck.MaxAge < 0 {
			rootExpired = true
		}
	}
	if !rootExpired {
		t.Errorf("the root-scoped cookie was not expired, so it keeps leaking: %v",
			rec.Result().Cookies())
	}
}

// At the root there is nothing to move, and rewriting the cookie on every authenticated request
// would be pure noise.
func TestAValidRootSessionIsLeftAloneAtTheRoot(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServerOnSubpath(t, p, "")

	req := httptest.NewRequest(http.MethodGet, "/constraints", nil)
	for _, ck := range loginForTest(t, e, p) {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if cookies := rec.Result().Cookies(); len(cookies) != 0 {
		t.Errorf("an authenticated request at the root rewrote cookies: %v", cookies)
	}
}

// A cookie GPM cannot overwrite -- one on a more specific path, or one carrying a Domain attribute
// -- makes every callback restart land back on an unreadable session. Unbounded, that bounces the
// browser between GPM and the provider forever and asks for a new authorization each time.
func TestTheCallbackRestartsTheLoginAtMostOnce(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServerOnSubpath(t, p, "/gpm")

	stale := &http.Cookie{Name: sessionName, Value: "a-cookie-signed-with-the-previous-secret"}

	// First hop: restart, and take the marker the response sets.
	req := httptest.NewRequest(http.MethodGet, callbackPath+"?state=x&code=y", nil)
	req.AddCookie(stale)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("first callback status = %d, want a restart", rec.Code)
	}
	var marker *http.Cookie
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == retryCookieName && ck.MaxAge > 0 {
			marker = ck
		}
	}
	if marker == nil {
		t.Fatalf("the restart set no retry marker: %v", rec.Result().Cookies())
	}

	// Second hop: the same unreadable cookie comes back, and so does the marker.
	req = httptest.NewRequest(http.MethodGet, callbackPath+"?state=x&code=y", nil)
	req.AddCookie(stale)
	req.AddCookie(marker)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code == http.StatusFound {
		t.Fatalf("the callback restarted a second time, to %q — this is the loop",
			rec.Header().Get("Location"))
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	// A page again, and it has to offer the way back in rather than dead-ending.
	assertAnsweredWithAPage(t, rec)
	if want := `href="` + browserPath("/login") + `"`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("rejection page = %q, want a login link %q", rec.Body.String(), want)
	}
	// The marker has to go, or the next genuine login cannot use its one restart.
	var cleared bool
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == retryCookieName && ck.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Errorf("the retry marker was not cleared: %v", rec.Result().Cookies())
	}
}

// The manually-configured endpoint path (no discovery) has to complete a real login, not just pass
// the settings validation. This is the configuration that once shipped the empty-issuer bug.
func TestManualEndpointConfigurationCompletesALogin(t *testing.T) {
	p := newFakeProvider(t)
	// Break discovery: if newAuthenticator still succeeds and the login completes, the manual
	// endpoints -- not a discovery fallback -- are what drove it.
	p.breakDiscovery = true
	e, _ := newAuthTestServerOnSubpath(t, p, "", map[string]any{
		"oidc_authorization_endpoint": p.issuer() + "/authorize",
		"oidc_token_endpoint":         p.issuer() + "/token",
		"oidc_jwks_uri":               p.issuer() + "/jwks",
	})

	cookies := loginForTest(t, e, p)

	req := httptest.NewRequest(http.MethodGet, "/constraints", nil)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "protected" {
		t.Fatalf("after a manual-endpoint login, /constraints = %d %q, want 200 \"protected\"",
			rec.Code, rec.Body.String())
	}
}

// The identity provider sends the browser to the callback by top-level navigation, so every failure
// there has to answer with a page. It used to answer with JSON, which showed up raw in the address
// bar (issue #389). The /api/* answer is deliberately still JSON and is covered separately.
//
// This matches the stub above, not the real error page, so what it proves is that the callback went
// through renderError rather than c.JSON -- which is exactly the thing that regressed. The real page
// is covered by the SSR view tests.
func assertAnsweredWithAPage(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	body := strings.TrimSpace(rec.Body.String())
	if strings.HasPrefix(body, "{") {
		t.Errorf("the callback answered with JSON, not a page: %s", body)
	}
	if !strings.Contains(body, "<h1>Error</h1>") {
		t.Errorf("the callback did not render the error page: %s", body)
	}
}

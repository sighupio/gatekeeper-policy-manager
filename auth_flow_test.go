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
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
)

type fakeProvider struct {
	server *httptest.Server
	key    *rsa.PrivateKey
	// Overrides for the next ID token, so tests can forge a bad one.
	nonceOverride string
	audOverride   string
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

	useTestSettings(t)
	settings := map[string]any{
		"auth_enabled":         "OIDC",
		"secret_key":           "test-secret-key",
		"preferred_url_scheme": "http",
		"session_max_age":      3600,
		"oidc_issuer":          p.issuer(),
		"oidc_client_id":       "gpm",
		"oidc_client_secret":   "gpm-secret",
		"oidc_redirect_domain": "http://gpm.example.com",
	}
	for k, v := range settings {
		viper.Set(k, v)
	}

	auth, err := newAuthenticator(context.Background())
	if err != nil {
		t.Fatalf("configuring the authenticator failed: %v", err)
	}

	e := echo.New()
	e.Use(session.Middleware(newSessionStore()))
	e.Use(auth.middleware())
	e.GET(callbackPath, auth.callback)
	e.GET("/login", auth.login)
	e.GET("/logout", auth.logout)
	e.GET("/constraints", func(c echo.Context) error { return c.String(http.StatusOK, "protected") })
	e.GET("/api/v1/auth", getAuth)
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

	// Assert only the property under test. Whether the page renders depends on static-content/
	// being present, which is a gitignored frontend build artifact that CI does not produce for
	// the Go test step — asserting 200 here made the suite pass locally and fail in CI.
	if rec.Code == http.StatusFound || rec.Header().Get("Location") != "" {
		t.Fatalf("the return hop redirected again to %q — this is the redirect loop",
			rec.Header().Get("Location"))
	}
}

// /login gives the frontend somewhere to send the user after a 401, and honours ?next=.
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

// ?next= is attacker-controllable, so an off-site value must not be honoured.
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

// /api/v1/auth has to answer without a session, because the frontend calls it to find out whether
// there is anything to log into.
func TestAuthEndpointStaysReachableWithoutASession(t *testing.T) {
	p := newFakeProvider(t)
	e, _ := newAuthTestServer(t, p)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"auth_enabled":true`) {
		t.Errorf("body = %q, want auth_enabled true", rec.Body.String())
	}
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

	if p.verifierSeen == "" {
		t.Fatal("no code_verifier sent on the token request; the exchange is not completing PKCE")
	}
	if !p.pkceOK {
		t.Errorf("the code_verifier does not hash to the challenge: verifier=%q challenge=%q",
			p.verifierSeen, p.challenge)
	}
}

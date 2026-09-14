// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
)

// A stand-in for the authenticating proxy: it publishes a JWKS and mints assertions, including the
// bad ones a forger would present.
type fakeProxy struct {
	server *httptest.Server
	rsaKey *rsa.PrivateKey
	ecKey  *ecdsa.PrivateKey
}

func newFakeProxy(t *testing.T) *fakeProxy {
	t.Helper()

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating the RSA key failed: %v", err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating the EC key failed: %v", err)
	}
	p := &fakeProxy{rsaKey: rsaKey, ecKey: ecKey}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/pomerium/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
			{Key: rsaKey.Public(), KeyID: "rsa", Algorithm: "RS256", Use: "sig"},
			{Key: ecKey.Public(), KeyID: "ec", Algorithm: "ES256", Use: "sig"},
		}})
	})
	p.server = httptest.NewTLSServer(mux)
	t.Cleanup(p.server.Close)
	return p
}

func (p *fakeProxy) jwksURL() string {
	return p.server.URL + "/.well-known/pomerium/jwks.json"
}

// assertion mints one, shaped like Pomerium's: ES256, the bare route host in aud and iss, and a
// five-minute life. Callers override any claim through extra.
func (p *fakeProxy) assertion(t *testing.T, extra map[string]any) string {
	t.Helper()

	claims := map[string]any{
		"iss":    "gpm.example.com",
		"aud":    "gpm.example.com",
		"sub":    "pomerium-user-id",
		"iat":    time.Now().Unix(),
		"exp":    time.Now().Add(5 * time.Minute).Unix(),
		"email":  "ramiro@example.com",
		"groups": []string{"platform"},
	}
	alg := jose.ES256
	var key any = p.ecKey
	for k, v := range extra {
		switch k {
		case "__alg":
			alg, key = jose.RS256, p.rsaKey
		case "__wrongkey":
			// A different key of the right shape: the signature verifies against nothing published.
			other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				t.Fatalf("generating the forger's key failed: %v", err)
			}
			key = other
		default:
			if v == nil {
				delete(claims, k)
				continue
			}
			claims[k] = v
		}
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("encoding the claims failed: %v", err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: alg, Key: key},
		(&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatalf("building the signer failed: %v", err)
	}
	sig, err := signer.Sign(payload)
	if err != nil {
		t.Fatalf("signing the assertion failed: %v", err)
	}
	out, err := sig.CompactSerialize()
	if err != nil {
		t.Fatalf("serialising the assertion failed: %v", err)
	}
	return out
}

// Configures GPM in JWT mode and returns an echo instance wired as main() wires it.
func newJWTTestServer(t *testing.T, p *fakeProxy, extra ...map[string]any) (*echo.Echo, *jwtAuthenticator) {
	t.Helper()

	useTestSettings(t)
	settings := map[string]any{
		"auth_enabled":    "JWT",
		"jwt_jwk_set_url": p.jwksURL(),
		"jwt_audience":    "gpm.example.com",
		"jwt_header_name": defaultJWTHeaderName,
		"rbac_filtering":  false,
		"session_max_age": 3600,
	}
	for _, m := range extra {
		for k, v := range m {
			settings[k] = v
		}
	}
	for k, v := range settings {
		viper.Set(k, v)
	}

	auth, err := newJWTAuthenticator(oidc.ClientContext(context.Background(), p.server.Client()))
	if err != nil {
		t.Fatalf("configuring the JWT authenticator failed: %v", err)
	}
	auth.renderError = func(c echo.Context, status int, e ssrErrorView) error {
		return c.HTML(status, "<h1>"+e.Heading+"</h1><p>"+e.Message+"</p><p>"+e.Action+
			"</p><p>"+e.Description+"</p>")
	}

	e := echo.New()
	e.Use(auth.middleware())
	e.GET("/constraints", func(c echo.Context) error { return c.String(http.StatusOK, "protected") })
	e.GET("/health", getHealth)
	return e, auth
}

func getWithAssertion(t *testing.T, e *echo.Echo, path, header, value string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if value != "" {
		req.Header.Set(header, value)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// The happy path, and the one that matters most: an assertion Pomerium would really mint, signed
// with ES256, is accepted. go-oidc defaults to RS256 alone, so this fails without the algorithm list.
func TestAValidPomeriumAssertionIsAccepted(t *testing.T) {
	p := newFakeProxy(t)
	e, _ := newJWTTestServer(t, p)

	rec := getWithAssertion(t, e, "/constraints", defaultJWTHeaderName, p.assertion(t, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "protected" {
		t.Errorf("body = %q, want the protected page", rec.Body.String())
	}
}

// An RSA-signed assertion has to work too: not every proxy signs with ES256.
func TestAnRS256AssertionIsAccepted(t *testing.T) {
	p := newFakeProxy(t)
	e, _ := newJWTTestServer(t, p)

	rec := getWithAssertion(t, e, "/constraints", defaultJWTHeaderName,
		p.assertion(t, map[string]any{"__alg": "RS256"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", rec.Code, rec.Body.String())
	}
}

// Every one of these is a request GPM must refuse. A pass here is a bypass of the proxy.
func TestAssertionsThatMustBeRefused(t *testing.T) {
	for name, extra := range map[string]map[string]any{
		// The whole reason the mode verifies a signature instead of trusting a header.
		"signed by somebody else": {"__wrongkey": true},
		// The audience check is what stops an assertion minted for another route on the same proxy.
		"minted for another route": {"aud": "grafana.example.com"},
		"expired":                  {"exp": time.Now().Add(-time.Minute).Unix()},
		"no expiry at all":         {"exp": nil},
	} {
		t.Run(name, func(t *testing.T) {
			p := newFakeProxy(t)
			e, _ := newJWTTestServer(t, p)

			rec := getWithAssertion(t, e, "/constraints", defaultJWTHeaderName, p.assertion(t, extra))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401. body: %s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "protected") {
				t.Error("the protected page was served")
			}
		})
	}
}

// A request that did not come through the proxy carries no header at all. This is the case an
// attacker reaching the Service directly produces, so the refusal must not be conditional.
func TestARequestWithNoAssertionIsRefused(t *testing.T) {
	p := newFakeProxy(t)
	e, _ := newJWTTestServer(t, p)

	rec := getWithAssertion(t, e, "/constraints", defaultJWTHeaderName, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	// The advice has to send them through the proxy, not tell them to log in: GPM has no login.
	if !strings.Contains(rec.Body.String(), "proxy") {
		t.Errorf("the refusal does not mention the proxy: %s", rec.Body.String())
	}
}

// The probes and the scrape reach GPM directly, not through the proxy. Turning the mode on must not
// stop them, exactly as OIDC mode does not.
func TestHealthStaysOpenInJWTMode(t *testing.T) {
	p := newFakeProxy(t)
	e, _ := newJWTTestServer(t, p)

	rec := getWithAssertion(t, e, "/health", defaultJWTHeaderName, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: the liveness probe carries no assertion", rec.Code)
	}
}

// oauth2-proxy sends the provider's ID token in Authorization, with a Bearer prefix.
func TestTheHeaderIsConfigurableAndBearerIsStripped(t *testing.T) {
	p := newFakeProxy(t)
	e, _ := newJWTTestServer(t, p, map[string]any{"jwt_header_name": "Authorization"})

	rec := getWithAssertion(t, e, "/constraints", "Authorization", "Bearer "+p.assertion(t, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", rec.Code, rec.Body.String())
	}
	// The header GPM is no longer reading must not be a way in.
	rec = getWithAssertion(t, e, "/constraints", defaultJWTHeaderName, p.assertion(t, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401: only the configured header counts", rec.Code)
	}
}

// The issuer is optional, because Pomerium's format depends on jwt_issuer_format. When the operator
// does pin it, a mismatch has to be refused, or pinning it means nothing.
func TestTheIssuerIsCheckedOnlyWhenItIsSet(t *testing.T) {
	p := newFakeProxy(t)

	e, _ := newJWTTestServer(t, p, map[string]any{"jwt_issuer": "https://gpm.example.com/"})
	rec := getWithAssertion(t, e, "/constraints", defaultJWTHeaderName, p.assertion(t, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401: the assertion's iss is the bare host, not the URI form", rec.Code)
	}

	e, _ = newJWTTestServer(t, p, map[string]any{"jwt_issuer": "gpm.example.com"})
	rec = getWithAssertion(t, e, "/constraints", defaultJWTHeaderName, p.assertion(t, nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200: the pinned issuer matches", rec.Code)
	}
}

// A configuration that cannot be enforced must stop the process, not start a GPM that looks
// protected. The audience is the one that is easy to leave out and impossible to notice.
func TestJWTConfigurationIsRefusedWhenItCannotBeEnforced(t *testing.T) {
	for name, tt := range map[string]struct {
		settings map[string]any
		want     string
	}{
		"no JWKS URL":    {map[string]any{"jwt_jwk_set_url": ""}, "GPM_JWT_JWK_SET_URL"},
		"JWKS not a URL": {map[string]any{"jwt_jwk_set_url": "not a url"}, "GPM_JWT_JWK_SET_URL"},
		"JWKS not https": {map[string]any{"jwt_jwk_set_url": "file:///etc/keys.json"}, "GPM_JWT_JWK_SET_URL"},
		// Plaintext would let anyone on the path serve their own key set and then choose who every
		// user is. An in-cluster Service URL is the plausible way to reach this, so it is refused
		// as well: a compromised node is on that path too.
		"JWKS over plain http": {map[string]any{
			"jwt_jwk_set_url": "http://pomerium.pomerium.svc/.well-known/pomerium/jwks.json",
		}, "GPM_JWT_JWK_SET_URL"},
		"no audience": {map[string]any{"jwt_audience": ""}, "GPM_JWT_AUDIENCE"},
	} {
		t.Run(name, func(t *testing.T) {
			useTestSettings(t)
			viper.Set("auth_enabled", "JWT")
			viper.Set("jwt_jwk_set_url", "https://pomerium.example.com/.well-known/pomerium/jwks.json")
			viper.Set("jwt_audience", "gpm.example.com")
			for k, v := range tt.settings {
				viper.Set(k, v)
			}

			_, err := newJWTAuthenticator(context.Background())
			if err == nil {
				t.Fatal("GPM accepted a configuration it cannot enforce")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("the error does not name %s: %v", tt.want, err)
			}
		})
	}
}

// The RBAC identity comes from the assertion, per request, and carries the cluster's prefixes. An
// identity that does not reach the reviews correctly denies everybody, silently.
func TestTheRBACIdentityIsBuiltFromTheAssertion(t *testing.T) {
	p := newFakeProxy(t)
	e, auth := newJWTTestServer(t, p, map[string]any{
		"rbac_filtering":       true,
		"rbac_username_claim":  "email",
		"rbac_username_prefix": "oidc:",
		"rbac_groups_claim":    "groups",
		"rbac_groups_prefix":   "oidc:",
	})

	var got rbacIdentity
	e.GET("/resources", func(c echo.Context) error {
		var err error
		got, err = identityFor(c)
		if err != nil {
			t.Errorf("identityFor: %v", err)
		}
		return c.NoContent(http.StatusOK)
	})

	rec := getWithAssertion(t, e, "/resources", defaultJWTHeaderName, p.assertion(t, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got.Username != "oidc:ramiro@example.com" {
		t.Errorf("username = %q, want the email claim behind the cluster's prefix", got.Username)
	}
	if len(got.Groups) != 1 || got.Groups[0] != "oidc:platform" {
		t.Errorf("groups = %v, want the groups claim behind the cluster's prefix", got.Groups)
	}
	_ = auth
}

// A pinned claim the assertion does not carry must deny, not fall back to another claim. Falling
// back would authorize a name the operator never pinned.
func TestAMissingPinnedClaimDeniesInJWTMode(t *testing.T) {
	p := newFakeProxy(t)
	e, _ := newJWTTestServer(t, p, map[string]any{
		"rbac_filtering":      true,
		"rbac_username_claim": "upn",
	})

	var got rbacIdentity
	e.GET("/resources", func(c echo.Context) error {
		got, _ = identityFor(c)
		return c.NoContent(http.StatusOK)
	})

	if rec := getWithAssertion(t, e, "/resources", defaultJWTHeaderName, p.assertion(t, nil)); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: the assertion itself is valid", rec.Code)
	}
	if got.valid() {
		t.Errorf("identity = %v, want an invalid one that authorizes nobody", got)
	}
}

// The claims are decoded on every request here, so one misconfigured deployment would fill the log.
func TestRepeatedClaimWarningsAreRationed(t *testing.T) {
	p := newFakeProxy(t)
	_, auth := newJWTTestServer(t, p)

	now := time.Now()
	auth.now = func() time.Time { return now }

	count := 0
	for range 5 {
		if auth.shouldWarn("same message") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("warned %d times in one window, want 1", count)
	}

	now = now.Add(jwtWarnInterval + time.Second)
	if !auth.shouldWarn("same message") {
		t.Error("the warning never returns, so a fault that is still there stops being reported")
	}
}

// authEnabled is what the RBAC feature and the Log out control read, so it has to be true in both
// modes. Only the login flow itself is OIDC-specific.
func TestAuthModes(t *testing.T) {
	for value, want := range map[string]struct{ mode string }{
		"OIDC":      {authModeOIDC},
		"oidc":      {authModeOIDC},
		"JWT":       {authModeJWT},
		"jwt":       {authModeJWT},
		"Jwt":       {authModeJWT},
		"Anonymous": {""},
		"":          {""},
		"true":      {""},
	} {
		t.Run("value="+value, func(t *testing.T) {
			useTestSettings(t)
			viper.Set("auth_enabled", value)

			if got := authMode(); got != want.mode {
				t.Errorf("authMode() = %q, want %q", got, want.mode)
			}
			if got := authEnabled(); got != (want.mode != "") {
				t.Errorf("authEnabled() = %v for mode %q", got, want.mode)
			}
			if got := jwtAuthEnabled(); got != (want.mode == authModeJWT) {
				t.Errorf("jwtAuthEnabled() = %v for mode %q", got, want.mode)
			}
		})
	}
}

// The Log out control must not appear when it would do nothing. GPM holds no session in JWT mode,
// so without the proxy's sign-out page there is nothing for the button to reach.
func TestTheLogoutControlFollowsTheMode(t *testing.T) {
	for name, tt := range map[string]struct {
		mode   string
		logout string
		want   string
	}{
		"OIDC clears its own session":   {"OIDC", "", "/logout"},
		"JWT with a sign-out page":      {"JWT", "https://gpm.example.com/.pomerium/sign_out", "https://gpm.example.com/.pomerium/sign_out"},
		"JWT without one shows nothing": {"JWT", "", ""},
		"unauthenticated":               {"Anonymous", "", ""},
	} {
		t.Run(name, func(t *testing.T) {
			useTestSettings(t)
			viper.Set("auth_enabled", tt.mode)
			viper.Set("jwt_logout_url", tt.logout)

			if got := logoutTarget(); got != tt.want {
				t.Errorf("logoutTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
)

func TestAuthEnabled(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"OIDC", true},
		// Case-insensitive, because operators write this by hand in a values file.
		{"oidc", true},
		{"Oidc", true},
		{"Anonymous", false},
		{"", false},
		{"true", false},
	}

	for _, tt := range tests {
		t.Run("value="+tt.value, func(t *testing.T) {
			useTestSettings(t)
			viper.Set("auth_enabled", tt.value)

			if got := authEnabled(); got != tt.want {
				t.Errorf("authEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Everything not listed here needs a session, so a path wrongly classified as public is a hole and
// one wrongly classified as private locks the user out before they can log in.
func TestIsPublicPath(t *testing.T) {
	public := []string{
		"/health", "/health/",
		"/oidc-auth",
		"/logout", "/login",
		"/metrics",
		"/static/js/main.abc123.js", "/static/css/main.abc123.css",
	}
	for _, p := range public {
		if !isPublicPath(p) {
			t.Errorf("isPublicPath(%q) = false, want true", p)
		}
	}

	private := []string{
		"/",
		"/constraints", "/mutations", "/events",
		"/api/v1/contexts", "/api/v1/contexts/",
		"/api/v1/configs", "/api/v1/constraints", "/api/v1/constrainttemplates",
		"/api/v1/mutations", "/api/v1/events",
		"/api/v2/contexts/",
		// Must not be reachable just because it starts with a public prefix.
		"/static", "/healthz", "/api/v1/authorized",
		// The old Create React App asset paths are no longer served, so no longer public.
		"/favicon.ico", "/manifest.json", "/robots.txt",
		// Neither the raw nor the cleaned form may sneak past the allowlist.
		"/static/../api/v1/constraints",
		"/static/../../api/v1/constraints",
		"/static/./../api/v1/contexts",
		"/metrics/../api/v1/events",
		"/api/v1/../../static/x",
		"/constraints/../static/js/main.js",
	}
	for _, p := range private {
		if isPublicPath(p) {
			t.Errorf("isPublicPath(%q) = true, want false", p)
		}
	}
}

func TestIsAPIPath(t *testing.T) {
	for _, p := range []string{"/api/v1/configs", "/api/v2/contexts/", "/api/"} {
		if !isAPIPath(p) {
			t.Errorf("isAPIPath(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"/", "/constraints", "/static/js/main.js", "/logout"} {
		if isAPIPath(p) {
			t.Errorf("isAPIPath(%q) = true, want false", p)
		}
	}
}

// An expired session must not send fetch() to the identity provider: a cross-origin redirect shows
// up as an opaque network error rather than something the UI can report.
func TestMiddlewareAnswers401ForAPIRequests(t *testing.T) {
	a := &authenticator{}
	handlerRan := false
	h := a.middleware()(func(echo.Context) error {
		handlerRan = true
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/constraints", nil)
	rec := httptest.NewRecorder()
	if err := h(echo.New().NewContext(req, rec)); err != nil {
		t.Fatalf("middleware returned an error: %v", err)
	}

	if handlerRan {
		t.Error("the wrapped handler ran even though there was no session")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rec.Body.String(), "session") {
		t.Errorf("body should explain the missing session, got %q", rec.Body.String())
	}
}

func TestMiddlewareLetsPublicPathsThrough(t *testing.T) {
	a := &authenticator{}
	handlerRan := false
	h := a.middleware()(func(echo.Context) error {
		handlerRan = true
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	if err := h(echo.New().NewContext(req, rec)); err != nil {
		t.Fatalf("middleware returned an error: %v", err)
	}

	if !handlerRan {
		t.Error("the wrapped handler did not run for a public path")
	}
}

func TestNewAuthenticatorRequiresItsSettings(t *testing.T) {
	tests := []struct {
		name    string
		set     map[string]string
		wantErr string
	}{
		{
			name:    "no redirect domain",
			set:     map[string]string{"oidc_client_id": "gpm", "oidc_issuer": "https://example.com"},
			wantErr: "GPM_OIDC_REDIRECT_DOMAIN",
		},
		{
			name:    "no client id",
			set:     map[string]string{"oidc_redirect_domain": "https://gpm.example.com", "oidc_issuer": "https://example.com"},
			wantErr: "GPM_OIDC_CLIENT_ID",
		},
		{
			name:    "no issuer and no manual endpoints",
			set:     map[string]string{"oidc_redirect_domain": "https://gpm.example.com", "oidc_client_id": "gpm"},
			wantErr: "GPM_OIDC_ISSUER",
		},
		{
			// go-oidc compares the iss claim literally, so an empty issuer rejects every token.
			// Discovery is off in this mode, so nothing fills it in.
			name: "manual endpoints complete but no issuer",
			set: map[string]string{
				"oidc_redirect_domain":        "https://gpm.example.com",
				"oidc_client_id":              "gpm",
				"oidc_authorization_endpoint": "https://example.com/authorize",
				"oidc_token_endpoint":         "https://example.com/token",
				"oidc_jwks_uri":               "https://example.com/jwks",
			},
			wantErr: "GPM_OIDC_ISSUER",
		},
		{
			// Setting any endpoint by hand turns discovery off, so a partial set cannot work.
			name: "manual endpoints given only partially",
			set: map[string]string{
				"oidc_redirect_domain":        "https://gpm.example.com",
				"oidc_client_id":              "gpm",
				"oidc_authorization_endpoint": "https://example.com/authorize",
			},
			wantErr: "GPM_OIDC_TOKEN_ENDPOINT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useTestSettings(t)
			for k, v := range tt.set {
				viper.Set(k, v)
			}

			_, err := newAuthenticator(context.Background())
			if err == nil {
				t.Fatalf("expected an error mentioning %s, got none", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to mention %s", err, tt.wantErr)
			}
		})
	}
}

// The destination comes from a URL the caller controls, so anything that a browser would treat as
// off-site has to be rejected. "//evil.com" is the classic one: it reads as a relative path but
// browsers resolve it as protocol-relative, which turns the post-login redirect into an open
// redirect.
func TestSafeRedirectTarget(t *testing.T) {
	kept := []string{
		"/",
		"/constraints",
		"/constraints?context=prod#anchor",
		"/api/v1/configs",
	}
	for _, target := range kept {
		if got := safeRedirectTarget(target); got != target {
			t.Errorf("safeRedirectTarget(%q) = %q, want it kept", target, got)
		}
	}

	rejected := []string{
		"//evil.com",
		"//evil.com/path",
		`/\evil.com`,
		// Browsers strip these before resolving, so "/\t/evil.com" becomes "//evil.com".
		"/\t/evil.com",
		"/\n/evil.com",
		"/\r/evil.com",
		"/\t\t//evil.com",
		"https://evil.com",
		"http://evil.com",
		"evil.com",
		"",
	}
	for _, target := range rejected {
		if got := safeRedirectTarget(target); got != "/" {
			t.Errorf("safeRedirectTarget(%q) = %q, want it rejected down to %q", target, got, "/")
		}
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "ramiro", "fallback"); got != "ramiro" {
		t.Errorf("firstNonEmpty = %q, want %q", got, "ramiro")
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("firstNonEmpty of all-empty = %q, want empty", got)
	}
}

// Do not fold this into TestIsPublicPath's table. It looks redundant and is not: this is the only
// test that pins which string the middleware passes in. Clean the path here instead of inside
// isPublicPath -- the obvious fix, and one of the two the original report suggested -- and
// "/api/v1/../../static/x" turns public. The table cannot see that; this does.
func TestMiddlewareStopsATraversalDressedAsAPublicPath(t *testing.T) {
	for _, target := range []string{
		"/static/../api/v1/constraints",
		"/metrics/../api/v1/events",
		"/api/v1/../../static/x",
	} {
		a := &authenticator{}
		handlerRan := false
		h := a.middleware()(func(echo.Context) error {
			handlerRan = true
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		// No session store is installed, so startLogin cannot run; the error it returns is itself
		// proof that the request was not treated as public.
		_ = h(echo.New().NewContext(req, rec))

		if handlerRan {
			t.Errorf("%q reached the handler without a session", target)
		}
	}
}

// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestBasePathNormalization(t *testing.T) {
	tests := map[string]string{
		"":          "",
		"/":         "",
		"/gpm":      "/gpm",
		"/gpm/":     "/gpm",
		"gpm":       "/gpm",
		"gpm/":      "/gpm",
		" /gpm ":    "/gpm",
		"/gpm/sub":  "/gpm/sub",
		"/gpm/sub/": "/gpm/sub",
		// Typos with extra slashes. Before these were normalized, "//" came out as "/", which made
		// browserPath("/login") return "//login" -- a protocol-relative URL the browser resolves
		// off-site -- and put the session cookie back at origin-wide scope.
		"//":         "",
		"///":        "",
		"/gpm//":     "/gpm",
		"/gpm//sub":  "/gpm/sub",
		"/gpm/./sub": "/gpm/sub",
		// path.Clean resolves ".." but a rooted path cannot escape above "/", so these land on a
		// sibling or on the root, never on a parent.
		"/gpm/..":      "",
		"..":           "",
		"/gpm/../../x": "/x",
		// Browsers read "\\" as a path delimiter, so "\\evil.com" would make browserPath("/login")
		// resolve off site. Refused rather than passed through.
		`\evil.com`:     "",
		`/gpm\evil.com`: "",
	}

	for configured, want := range tests {
		t.Run("GPM_BASE_PATH="+configured, func(t *testing.T) {
			useTestSettings(t)
			viper.Set("base_path", configured)

			if got := basePath(); got != want {
				t.Errorf("basePath() = %q, want %q", got, want)
			}
		})
	}
}

// The root deployment is the one everybody has. Nothing about it must change.
func TestBrowserPathIsTheIdentityAtTheRoot(t *testing.T) {
	useTestSettings(t)

	for _, path := range []string{"/", "/login", "/logout", "/constraints/alpha", "/oidc-auth"} {
		if got := browserPath(path); got != path {
			t.Errorf("browserPath(%q) = %q, want it unchanged", path, got)
		}
		if got := backendPath(path); got != path {
			t.Errorf("backendPath(%q) = %q, want it unchanged", path, got)
		}
	}
}

func TestBrowserPathAddsTheSubpath(t *testing.T) {
	useTestSettings(t)
	viper.Set("base_path", "/gpm")

	tests := map[string]string{
		"/":                  "/gpm/",
		"":                   "/gpm/",
		"/login":             "/gpm/login",
		"/oidc-auth":         "/gpm/oidc-auth",
		"/constraints/alpha": "/gpm/constraints/alpha",
	}

	for path, want := range tests {
		if got := browserPath(path); got != want {
			t.Errorf("browserPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestBackendPathRemovesTheSubpath(t *testing.T) {
	useTestSettings(t)
	viper.Set("base_path", "/gpm")

	tests := map[string]string{
		"/gpm":                   "/",
		"/gpm/":                  "/",
		"/gpm/constraints":       "/constraints",
		"/gpm/constraints/alpha": "/constraints/alpha",
		// Not under the base path. Left alone rather than mangled, for safeRedirectTarget to judge.
		"/constraints": "/constraints",
		"":             "",
		// A path that merely starts with the same letters is a different path.
		"/gpmx/constraints": "/gpmx/constraints",
	}

	for path, want := range tests {
		if got := backendPath(path); got != want {
			t.Errorf("backendPath(%q) = %q, want %q", path, got, want)
		}
	}
}

// Whatever the browser sends must survive the round trip, or a user is sent somewhere other than
// where they were going.
func TestBrowserAndBackendPathRoundTrip(t *testing.T) {
	for _, base := range []string{"", "/gpm", "/a/b"} {
		t.Run("base="+base, func(t *testing.T) {
			useTestSettings(t)
			viper.Set("base_path", base)

			for _, path := range []string{"/", "/login", "/constraints/alpha"} {
				if got := backendPath(browserPath(path)); got != path {
					t.Errorf("round trip of %q gave %q", path, got)
				}
			}
		})
	}
}

// A base path that normalizes away must leave GPM behaving exactly like a root deployment, rather
// than half way between the two.
func TestASlashOnlyBasePathIsTheRootDeployment(t *testing.T) {
	for _, configured := range []string{"/", "//", "///", "  /  "} {
		t.Run("GPM_BASE_PATH="+configured, func(t *testing.T) {
			useTestSettings(t)
			viper.Set("base_path", configured)

			if got := basePath(); got != "" {
				t.Errorf("basePath() = %q, want empty", got)
			}
			if got := cookiePath(); got != "/" {
				t.Errorf("cookiePath() = %q, want /", got)
			}
			// The one that used to bite: "//login" is protocol-relative and leaves the site.
			if got := browserPath("/login"); got != "/login" {
				t.Errorf("browserPath(\"/login\") = %q, want /login", got)
			}
		})
	}
}

// Both directions of the one property, with the RFC 6265 section 5.1.4 prefix rule written once:
// everything GPM serves is inside the cookie's scope, and nothing else on the host is.
func TestCookiePathCoversTheAppAndNothingElse(t *testing.T) {
	useTestSettings(t)
	viper.Set("base_path", "/gpm")

	scope := cookiePath()
	covers := func(requestPath string) bool {
		return requestPath == scope || strings.HasPrefix(requestPath, strings.TrimSuffix(scope, "/")+"/")
	}

	for _, mine := range []string{
		browserPath("/"), browserPath("/login"), browserPath("/logout"), browserPath(callbackPath),
		browserPath("/api/v1/constraints"), "/gpm",
	} {
		if !covers(mine) {
			t.Errorf("cookie Path %q does not cover %q, so the session is dropped there", scope, mine)
		}
	}

	for _, theirs := range []string{"/", "/other", "/gpmadmin", "/api/v1/constraints", "/admin/gpm"} {
		if covers(theirs) {
			t.Errorf("cookie Path %q reaches %q, which belongs to another application", scope, theirs)
		}
	}
}

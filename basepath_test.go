// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
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

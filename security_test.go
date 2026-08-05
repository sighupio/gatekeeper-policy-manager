// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Regression tests for the security defects a pre-commit review found in the OIDC work. Each one
// was confirmed to fail against the code as it stood before the fix.
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
)

// Puts a static-content tree in a temp directory, with a secret alongside it that must stay
// unreachable, and points the process at it for the duration of the test.
func withStaticContent(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "app", "static-content", "static"), 0o755); err != nil {
		t.Fatalf("building the fixture failed: %v", err)
	}
	write := func(p, body string) {
		if err := os.WriteFile(filepath.Join(root, p), []byte(body), 0o600); err != nil {
			t.Fatalf("writing %s failed: %v", p, err)
		}
	}
	write("secret.txt", "SERVICE-ACCOUNT-TOKEN")
	write(filepath.Join("app", "static-content", "index.html"), "<html>spa</html>")
	write(filepath.Join("app", "static-content", "static", "main.js"), "console.log(1)")

	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(filepath.Join(root, "app")); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	return root
}

// serveIndex used to join the raw RequestURI — query string included, unnormalised — into a
// filesystem path, so "?x=/../../.." walked straight out of the static root. Because /logout falls
// through to the SPA when there is no session, that was reachable with no authentication at all.
func TestServeIndexRefusesToEscapeTheStaticRoot(t *testing.T) {
	withStaticContent(t)

	escapes := []string{
		"/logout?x=/../../../secret.txt",
		"/../../secret.txt",
		"/x/../../secret.txt",
		"/static/../../../secret.txt",
		"/%2e%2e/%2e%2e/secret.txt",
		"/..%2f..%2fsecret.txt",
	}

	for _, target := range escapes {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rec := httptest.NewRecorder()
			if err := serveIndex(echo.New().NewContext(req, rec)); err != nil {
				t.Fatalf("serveIndex returned an error: %v", err)
			}
			if strings.Contains(rec.Body.String(), "SERVICE-ACCOUNT-TOKEN") {
				t.Fatalf("leaked a file from outside the static root: %q", rec.Body.String())
			}
		})
	}
}

// The traversal fix must not stop it serving the files it exists to serve.
func TestServeIndexStillServesTheApp(t *testing.T) {
	withStaticContent(t)

	cases := map[string]string{
		"/":                  "<html>spa</html>",
		"/constraints":       "<html>spa</html>", // client-side route, falls back to the shell
		"/static/main.js":    "console.log(1)",
		"/index.html":        "<html>spa</html>",
		"/constraints?a=b#c": "<html>spa</html>",
	}

	for target, want := range cases {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rec := httptest.NewRecorder()
			if err := serveIndex(echo.New().NewContext(req, rec)); err != nil {
				t.Fatalf("serveIndex returned an error: %v", err)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != want {
				t.Errorf("served %q, want %q", got, want)
			}
		})
	}
}

// Replacing store.Options wholesale left the securecookie codecs on their 30-day default, so
// GPM_SESSION_MAX_AGE only shortened the cookie attribute the browser sees. The server-side check
// is the one that matters, since an attacker replaying a stolen cookie ignores the attribute.
func TestSessionStoreEnforcesMaxAgeServerSide(t *testing.T) {
	viper.Set("secret_key", "a-test-key-that-is-not-the-default")
	viper.Set("session_max_age", 1)
	t.Cleanup(func() {
		viper.Set("secret_key", "")
		viper.Set("session_max_age", 0)
	})

	store, ok := newSessionStore().(*sessions.CookieStore)
	if !ok {
		t.Fatal("expected a CookieStore")
	}

	encoded, err := securecookie.EncodeMulti(sessionName,
		map[interface{}]interface{}{sessionKeyUser: "ramiro"}, store.Codecs...)
	if err != nil {
		t.Fatalf("encoding failed: %v", err)
	}

	// Decode the store exactly as newSessionStore built it. Touching store.Codecs here — an
	// earlier version of this test called sc.MaxAge(1) at this point — measures the test's own
	// mutation instead of the code, and passes even with store.MaxAge(maxAge) deleted.
	time.Sleep(2 * time.Second)

	out := map[interface{}]interface{}{}
	if err := securecookie.DecodeMulti(sessionName, encoded, &out, store.Codecs...); err == nil {
		t.Error("a cookie older than GPM_SESSION_MAX_AGE still decoded; " +
			"the securecookie codecs are not carrying the configured max age, so expiry is not enforced server side")
	}
}

// A max age of zero tells securecookie not to check the timestamp at all, and viper.GetInt turns
// an empty or unparseable GPM_SESSION_MAX_AGE into zero. A typo in the environment must not
// silently give every session unlimited life.
func TestSessionStoreRejectsAUselessMaxAge(t *testing.T) {
	for _, raw := range []any{0, "", "8h", "not-a-number"} {
		t.Run(fmt.Sprintf("%v", raw), func(t *testing.T) {
			viper.Set("secret_key", "a-test-key-that-is-not-the-default")
			viper.Set("session_max_age", raw)
			t.Cleanup(func() {
				viper.Set("secret_key", "")
				viper.Set("session_max_age", defaultSessionMaxAge)
			})

			store := newSessionStore().(*sessions.CookieStore)
			if store.Options.MaxAge != defaultSessionMaxAge {
				t.Errorf("MaxAge = %d for GPM_SESSION_MAX_AGE=%v, want the %d default rather than an unlimited session",
					store.Options.MaxAge, raw, defaultSessionMaxAge)
			}
		})
	}
}

// The 1.x default secret key is published in this repository, so a session signed with it can be
// forged by anyone. Enabling authentication with it must not be possible; main() exits, and this
// pins the constant that check depends on.
func TestDefaultSecretKeyIsRecognised(t *testing.T) {
	if insecureDefaultSecretKey != "g8k1p3rp0l1c7m4n4g3r" {
		t.Fatalf("the guard checks %q, which no longer matches the 1.x default", insecureDefaultSecretKey)
	}

	// Demonstrates why: a session forged offline with the published key is otherwise accepted.
	viper.Set("secret_key", insecureDefaultSecretKey)
	viper.Set("session_max_age", 3600)
	t.Cleanup(func() {
		viper.Set("secret_key", "")
		viper.Set("session_max_age", 0)
	})

	store := newSessionStore().(*sessions.CookieStore)
	forged, err := securecookie.EncodeMulti(sessionName,
		map[interface{}]interface{}{sessionKeyUser: "attacker"}, store.Codecs...)
	if err != nil {
		t.Fatalf("encoding failed: %v", err)
	}

	e := echo.New()
	e.Use(session.Middleware(store))
	e.Use((&authenticator{}).middleware())
	e.GET("/constraints", func(c echo.Context) error { return c.String(http.StatusOK, "protected") })

	req := httptest.NewRequest(http.MethodGet, "/constraints", nil)
	req.AddCookie(&http.Cookie{Name: sessionName, Value: forged})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Skip("forging did not work, so the premise of the startup guard needs rechecking")
	}
	t.Log("confirmed: the published default key lets anyone mint a valid session, which is why main() refuses to start with it")
}

// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Regression tests for the security defects a pre-commit review found in the OIDC work. Each one
// was confirmed to fail against the code as it stood before the fix.
package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
)

// Replacing store.Options wholesale left the securecookie codecs on their 30-day default, so
// GPM_SESSION_MAX_AGE only shortened the cookie attribute the browser sees. The server-side check
// is the one that matters, since an attacker replaying a stolen cookie ignores the attribute.
func TestSessionStoreEnforcesMaxAgeServerSide(t *testing.T) {
	useTestSettings(t)
	viper.Set("secret_key", "a-test-key-that-is-not-the-default")
	viper.Set("session_max_age", 1)

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
			useTestSettings(t)
			viper.Set("secret_key", "a-test-key-that-is-not-the-default")
			viper.Set("session_max_age", raw)

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
	useTestSettings(t)
	viper.Set("secret_key", insecureDefaultSecretKey)
	viper.Set("session_max_age", 3600)

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

// The cookie is signed and encrypted. Signing alone leaves the value as base64-encoded gob, so
// anyone holding the cookie -- a proxy log, a shared machine, a browser extension -- can read the
// username out of it, and during the login leg the state, the nonce and the PKCE verifier too.
func TestSessionCookieDoesNotRevealItsContents(t *testing.T) {
	useTestSettings(t)
	viper.Set("secret_key", "a-test-key-that-is-not-the-default")

	store, ok := newSessionStore().(*sessions.CookieStore)
	if !ok {
		t.Fatal("expected a CookieStore")
	}

	encoded, err := securecookie.EncodeMulti(sessionName, map[interface{}]interface{}{
		sessionKeyUser:     "ramiro",
		sessionKeyNonce:    "the-nonce-value",
		sessionKeyVerifier: "the-pkce-verifier",
	}, store.Codecs...)
	if err != nil {
		t.Fatalf("encoding failed: %v", err)
	}

	// securecookie's wire format is base64.URLEncoding of "date|base64(payload)|mac", so the
	// payload needs two decodes with a split between. Peeling only the outer layer finds nothing
	// readable even when the cookie is plain text, which is how an earlier version of this test
	// passed with the encryption switched off.
	outer, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decoding the cookie failed: %v", err)
	}
	// SplitN, not Split: the third field is a raw HMAC-SHA-256, and one byte in eight is "|" with
	// probability 1-(255/256)^32, about 12%. Splitting on every pipe made this test fail at random
	// in roughly one run in nine, and skip its assertions entirely when it did.
	parts := strings.SplitN(string(outer), "|", 3)
	if len(parts) != 3 {
		t.Fatalf("got %d fields in the cookie, want date|payload|mac", len(parts))
	}
	payload, err := base64.URLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decoding the cookie payload failed: %v", err)
	}
	haystacks := []string{encoded, string(outer), string(payload)}

	for _, secret := range []string{"ramiro", "the-nonce-value", "the-pkce-verifier"} {
		for _, haystack := range haystacks {
			if strings.Contains(haystack, secret) {
				t.Errorf("the cookie reveals %q in plain text; it is signed but not encrypted", secret)
			}
		}
	}

	// It still has to round-trip, or the encryption would be hiding the value from GPM as well.
	out := map[interface{}]interface{}{}
	if err := securecookie.DecodeMulti(sessionName, encoded, &out, store.Codecs...); err != nil {
		t.Fatalf("the store cannot read back its own cookie: %v", err)
	}
	if out[sessionKeyUser] != "ramiro" {
		t.Errorf("decoded user = %v, want ramiro", out[sessionKeyUser])
	}
}

// Both keys come from one secret, so they must not come out equal, and each must be the length its
// algorithm needs. A 32-byte block key selects AES-256.
func TestSessionKeysAreDistinctAndCorrectlySized(t *testing.T) {
	hashKey, blockKey := sessionKeys("a-test-key-that-is-not-the-default")

	if len(hashKey) != 64 {
		t.Errorf("hash key is %d bytes, want 64", len(hashKey))
	}
	if len(blockKey) != 32 {
		t.Errorf("block key is %d bytes, want 32 for AES-256", len(blockKey))
	}
	if bytes.Equal(hashKey, blockKey) {
		t.Error("the hash key and the block key are the same value")
	}
	if bytes.Contains(hashKey, blockKey) {
		t.Error("the block key is a slice of the hash key, so the two are not independent")
	}
}

// Every replica derives the keys separately, and a pod restart derives them again. If the result
// moved, a session would stop decoding as soon as a request reached a different pod.
func TestSessionKeysAreStableForTheSameSecret(t *testing.T) {
	firstHash, firstBlock := sessionKeys("a-test-key-that-is-not-the-default")
	secondHash, secondBlock := sessionKeys("a-test-key-that-is-not-the-default")

	if !bytes.Equal(firstHash, secondHash) || !bytes.Equal(firstBlock, secondBlock) {
		t.Error("the same secret produced different keys, so sessions would break across replicas")
	}

	otherHash, otherBlock := sessionKeys("a-different-secret")
	if bytes.Equal(firstHash, otherHash) || bytes.Equal(firstBlock, otherBlock) {
		t.Error("a different secret produced the same keys")
	}
}

// A subpath deployment shares its origin with whatever else the proxy serves there, so the cookie
// has to be scoped to GPM. With Path=/ the browser attaches the session to every request to the
// host, which hands the neighbouring application a session it can replay against GPM.
func TestSessionCookieIsScopedToTheSubpath(t *testing.T) {
	for base, want := range map[string]string{
		"":     "/",
		"/gpm": "/gpm",
		"/a/b": "/a/b",
	} {
		t.Run("GPM_BASE_PATH="+base, func(t *testing.T) {
			useTestSettings(t)
			viper.Set("secret_key", "a-test-key-that-is-not-the-default")
			viper.Set("base_path", base)

			store := newSessionStore().(*sessions.CookieStore)
			if got := store.Options.Path; got != want {
				t.Errorf("cookie Path = %q, want %q", got, want)
			}
		})
	}
}

// backendPath can turn a path under the base path into one that starts with "//", which the
// browser resolves as an off-site protocol-relative URL: "/gpm//evil.com" comes out as
// "//evil.com". safeRedirectTarget catches that, and it runs afterwards in login().
//
// Reversing the two calls is not enough to make this exploitable on its own -- startLogin
// sanitises the destination again, and browserPath puts the base path back on the front, so the
// result stays same-origin either way. This pins the first of those three layers.
func TestBackendPathCannotSmuggleAnOffsiteRedirect(t *testing.T) {
	useTestSettings(t)
	viper.Set("base_path", "/gpm")

	hostile := []string{
		"/gpm//evil.com", // becomes "//evil.com": protocol-relative
		`/gpm/\evil.com`, // becomes "/\evil.com": treated as "//" by browsers
		"/gpm//evil.com/path",
		"//evil.com", // not under the base path, so passed through untouched
		"https://evil.com",
		"/gpm/\t//evil.com", // browsers strip the tab before resolving
	}

	for _, target := range hostile {
		// The order the handler uses.
		if got := safeRedirectTarget(backendPath(target)); got != "/" {
			t.Errorf("safeRedirectTarget(backendPath(%q)) = %q, want %q — this redirect leaves the site",
				target, got, "/")
		}
	}

	// And a legitimate target still survives the same path.
	if got, want := safeRedirectTarget(backendPath("/gpm/constraints")), "/constraints"; got != want {
		t.Errorf("safeRedirectTarget(backendPath(%q)) = %q, want %q", "/gpm/constraints", got, want)
	}
}

// The session cookie's protective attributes. HttpOnly keeps it away from script; SameSite=Lax
// survives the top-level redirect back from the provider while blocking cross-site sends; Secure
// follows the scheme so TLS deployments never leak it over plain HTTP.
func TestSessionCookieFlags(t *testing.T) {
	base := func() *sessions.Options {
		return newSessionStore().(*sessions.CookieStore).Options
	}

	// HttpOnly and SameSite do not vary, so assert them once.
	useTestSettings(t)
	viper.Set("secret_key", "a-test-key-that-is-not-the-default")
	o := base()
	if !o.HttpOnly {
		t.Error("session cookie is not HttpOnly")
	}
	if o.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", o.SameSite)
	}

	// Secure follows the scheme: off over plain http, on once TLS is in play either way it is signalled.
	for _, tc := range []struct {
		name       string
		set        map[string]any
		wantSecure bool
	}{
		{"default http", nil, false},
		{"preferred scheme https", map[string]any{"preferred_url_scheme": "https"}, true},
		{"redirect domain https", map[string]any{"oidc_redirect_domain": "https://gpm.example.com"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useTestSettings(t)
			viper.Set("secret_key", "a-test-key-that-is-not-the-default")
			for k, v := range tc.set {
				viper.Set(k, v)
			}
			if got := base().Secure; got != tc.wantSecure {
				t.Errorf("Secure = %v, want %v", got, tc.wantSecure)
			}
		})
	}
}

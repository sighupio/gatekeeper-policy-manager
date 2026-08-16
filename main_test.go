// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
	"golang.org/x/exp/slog"
)

// Gives the test the configuration a freshly started GPM has, and takes back anything it changes.
// Any test that calls viper.Set, or that runs code reading a setting, has to call this first.
//
// viper is process-global and viper.Set outranks both the defaults and the environment, so a test
// that sets a key changes the code under test for every test after it. Resetting the keys it
// touched to "" is not enough: "" is not the default, so it leaves the process in a state GPM
// would never be in. session_max_age is the one that bites, because zero means "never expire".
// Tests only passed in the order the files happened to run in.
func useTestSettings(t *testing.T) {
	t.Helper()

	// bindSettings binds GPM_*, so without this the developer's own environment reaches the code
	// under test. viper reads an empty variable as unset.
	for _, entry := range os.Environ() {
		if k, _, found := strings.Cut(entry, "="); found && strings.HasPrefix(k, "GPM_") {
			t.Setenv(k, "")
		}
	}

	// Resetting on the way in is what makes the order stop mattering. Doing it again on the way out
	// is for whatever runs next without calling this.
	reset := func() {
		viper.Reset()
		bindSettings()
	}
	reset()
	t.Cleanup(reset)
}

// Everything else in the suite trusts useTestSettings to hand it a realistic starting point, so
// what "realistic" means is worth pinning: the values main() would produce.
func TestUseTestSettingsGivesTheStartupDefaults(t *testing.T) {
	useTestSettings(t)

	for key, want := range map[string]any{
		"auth_enabled":         "Anonymous",
		"log_level":            "INFO",
		"listen_address":       ":8080",
		"events_source":        "gatekeeper-webhook",
		"secret_key":           insecureDefaultSecretKey,
		"preferred_url_scheme": "http",
		"session_max_age":      defaultSessionMaxAge,
	} {
		if got := viper.Get(key); got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
}

// bindSettings binds GPM_*, and a developer running GPM locally has those set — mise.local.toml in
// this repository sets six of them. Without the guard the suite reads them: the missing-client-id
// case below stops being a missing-client-id case and goes out to the network instead.
func TestUseTestSettingsIgnoresTheDevelopersEnvironment(t *testing.T) {
	t.Setenv("GPM_AUTH_ENABLED", "OIDC")
	t.Setenv("GPM_SESSION_MAX_AGE", "1")
	t.Setenv("GPM_OIDC_CLIENT_ID", "someone-elses-client")

	useTestSettings(t)

	if got := viper.GetString("auth_enabled"); got != "Anonymous" {
		t.Errorf("auth_enabled = %q, want the default Anonymous rather than the environment", got)
	}
	if got := viper.GetInt("session_max_age"); got != defaultSessionMaxAge {
		t.Errorf("session_max_age = %d, want the %d default rather than the environment", got, defaultSessionMaxAge)
	}
	if got := viper.GetString("oidc_client_id"); got != "" {
		t.Errorf("oidc_client_id = %q, want it unset rather than taken from the environment", got)
	}
}

// Calls a handler that needs no Kubernetes client and returns the decoded JSON body.
func callHandler(t *testing.T, handler echo.HandlerFunc, path string) (int, map[string]any) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)

	if err := handler(c); err != nil {
		t.Fatalf("handler returned an error: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding the response body failed: %v (body: %s)", err, rec.Body.String())
	}
	return rec.Code, body
}

func TestGetHealth(t *testing.T) {
	code, body := callHandler(t, getHealth, "/health")

	if code != http.StatusOK {
		t.Errorf("status = %d, want %d", code, http.StatusOK)
	}
	if body["status"] != "ok" {
		t.Errorf(`status field = %v, want "ok"`, body["status"])
	}
}

func TestLogLevel(t *testing.T) {
	tests := map[string]slog.Level{
		"DEBUG": slog.LevelDebug,
		"debug": slog.LevelDebug,
		"INFO":  slog.LevelInfo,
		"WARN":  slog.LevelWarn,
		"ERROR": slog.LevelError,
		// Python's logging took these; slog does not. The release notes say so, so pin it here.
		"WARNING":  slog.LevelInfo,
		"CRITICAL": slog.LevelInfo,
		"FATAL":    slog.LevelInfo,
		"NOTSET":   slog.LevelInfo,
		// Documented in the README from 2023 to 2026 and never implemented. Now gone from both.
		"OFF":      slog.LevelInfo,
		"nonsense": slog.LevelInfo,
		"":         slog.LevelInfo,
	}

	for configured, want := range tests {
		t.Run("GPM_LOG_LEVEL="+configured, func(t *testing.T) {
			level := new(slog.LevelVar)
			setLogLevel(level, configured)

			if got := level.Level(); got != want {
				t.Errorf("level = %v, want %v", got, want)
			}
		})
	}
}

// When authentication is on the session cookie is only as strong as GPM_SECRET_KEY. The published
// 1.x default and anything trivially short must be refused; a real key must pass.
func TestSecretKeyError(t *testing.T) {
	cases := map[string]bool{ // key -> should be rejected
		insecureDefaultSecretKey: true,
		"x":                      true,
		"short":                  true,
		strings.Repeat("a", minSecretKeyLength-1): true,
		strings.Repeat("a", minSecretKeyLength):   false,
		"a-real-long-random-secret-key-value":     false,
	}
	for key, wantRejected := range cases {
		if rejected := secretKeyError(key) != ""; rejected != wantRejected {
			t.Errorf("secretKeyError(%q): rejected=%v, want %v", key, rejected, wantRejected)
		}
	}
}

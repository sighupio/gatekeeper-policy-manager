// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
)

const (
	defaultMessage = "An error ocurred while getting config objects from Kubernetes API."
	defaultAction  = "Check that the Kubeconfig file is correct and that the Kubernetes API is accessible."
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

// The error client-go actually surfaces for an untrusted certificate: an x509 error wrapped by
// crypto/tls, wrapped again by net/url. If errors.As stops unwrapping anywhere along that chain
// the hint silently disappears, so the nesting matters more than the leaf type.
func realWorldTLSError() error {
	return &url.Error{
		Op:  "Get",
		URL: "https://10.0.0.1:443/apis/config.gatekeeper.sh/v1alpha1/configs",
		Err: &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}},
	}
}

func TestKubeAPIErrorAnswerAddsTLSHint(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"unknown authority", x509.UnknownAuthorityError{}},
		// Both of these format themselves from the certificate, so it must not be nil.
		{"hostname mismatch", x509.HostnameError{Certificate: &x509.Certificate{}, Host: "10.0.0.1"}},
		{"invalid certificate", x509.CertificateInvalidError{Cert: &x509.Certificate{}, Reason: x509.Expired}},
		{"verification failure", &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}},
		{"wrapped as client-go returns it", realWorldTLSError()},
		{"wrapped in fmt.Errorf", fmt.Errorf("listing configs: %w", x509.UnknownAuthorityError{})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := kubeAPIErrorAnswer(defaultMessage, defaultAction, tt.err)

			if got.ErrorMessage == defaultMessage {
				t.Errorf("expected the TLS message to replace the default, got the default back")
			}
			if !strings.Contains(got.Action, "GPM_SKIP_TLS_VERIFY") {
				t.Errorf("expected the action to mention GPM_SKIP_TLS_VERIFY, got %q", got.Action)
			}
			if got.Description != tt.err.Error() {
				t.Errorf("description = %q, want the original error %q", got.Description, tt.err.Error())
			}
		})
	}
}

func TestKubeAPIErrorAnswerLeavesOtherErrorsAlone(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"connection refused", errors.New("dial tcp 10.0.0.1:443: connect: connection refused")},
		{"not found", errors.New(`the server could not find the requested resource`)},
		// Mentions certificates but is not a certificate error: must not be misclassified.
		{"mentions certificates only in text", errors.New("failed to read certificate file")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := kubeAPIErrorAnswer(defaultMessage, defaultAction, tt.err)

			if got.ErrorMessage != defaultMessage {
				t.Errorf("message = %q, want the caller's message %q", got.ErrorMessage, defaultMessage)
			}
			if got.Action != defaultAction {
				t.Errorf("action = %q, want the caller's action %q", got.Action, defaultAction)
			}
			if got.Description != tt.err.Error() {
				t.Errorf("description = %q, want %q", got.Description, tt.err.Error())
			}
		})
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

// The frontend decides whether to show the logout control from this endpoint, so it has to follow
// GPM_AUTH_ENABLED rather than being hardcoded.
func TestGetAuthReflectsConfiguration(t *testing.T) {
	tests := []struct {
		authEnabled string
		want        bool
	}{
		{"OIDC", true},
		{"oidc", true},
		{"Anonymous", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run("GPM_AUTH_ENABLED="+tt.authEnabled, func(t *testing.T) {
			useTestSettings(t)
			viper.Set("auth_enabled", tt.authEnabled)

			code, body := callHandler(t, getAuth, "/api/v1/auth")

			if code != http.StatusOK {
				t.Errorf("status = %d, want %d", code, http.StatusOK)
			}
			if body["auth_enabled"] != tt.want {
				t.Errorf("auth_enabled = %v, want %v", body["auth_enabled"], tt.want)
			}
		})
	}
}

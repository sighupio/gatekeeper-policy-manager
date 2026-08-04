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
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

const (
	defaultMessage = "An error ocurred while getting config objects from Kubernetes API."
	defaultAction  = "Check that the Kubeconfig file is correct and that the Kubernetes API is accessible."
)

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

// The frontend decides whether to show the login and logout controls from this endpoint. The Go
// backend has no authentication yet, so it must keep reporting false. When OIDC lands this test
// should fail and be rewritten — that is the point of it.
func TestGetAuthReportsAuthDisabled(t *testing.T) {
	code, body := callHandler(t, getAuth, "/api/v1/auth")

	if code != http.StatusOK {
		t.Errorf("status = %d, want %d", code, http.StatusOK)
	}
	if body["auth_enabled"] != false {
		t.Errorf("auth_enabled = %v, want false", body["auth_enabled"])
	}
}

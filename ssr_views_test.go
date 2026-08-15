// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"strings"
	"testing"
)

// Guards the new SSR views: newSSRRenderer parses every registered page (it panics on a parse
// error), and executing each new template against a realistic model catches field typos that
// build and vet cannot see because template execution happens at request time.
func TestSSRNewViewsRenderWithoutError(t *testing.T) {
	r := newSSRRenderer()

	ct := ssrConstraintTemplate{
		Name: "k8srequiredlabels", Kind: "K8sRequiredLabels", Created: "2026-01-01T00:00:00Z",
		Description: "Requires labels", Target: "admission.k8s.gatekeeper.sh",
		Rego:        "package x", Libs: []string{"lib1"},
		Schema:      map[string]any{"labels": map[string]any{"type": "array"}},
		Constraints: []string{"must-have-owner"},
		Raw:         map[string]any{"kind": "ConstraintTemplate"},
	}
	ctData := map[string]any{"Layout": minimalLayout(), "Templates": []ssrConstraintTemplate{ct}}

	var buf bytes.Buffer
	if err := r.pages["constrainttemplates"].ExecuteTemplate(&buf, "layout", ctData); err != nil {
		t.Fatalf("constrainttemplates render failed: %v", err)
	}
	for _, want := range []string{"K8sRequiredLabels", "package x", "must-have-owner", "Parameters schema"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("constrainttemplates output missing %q", want)
		}
	}

	ev := ssrEvent{
		Name: "e1", Reason: "FailedAdmission", Message: "denied", Count: "3",
		Action: "deny", ConstraintKind: "K8sRequiredLabels", ConstraintName: "must-have-owner",
		FirstTimestamp: "2026-01-01 00:00:00 UTC", LastTimestamp: "2026-01-02 00:00:00 UTC",
		ObjKind: "Pod", ObjName: "nginx", ObjNamespace: "default",
		SourceComponent: "gatekeeper-webhook",
	}
	evData := map[string]any{"Layout": minimalLayout(), "Events": []ssrEvent{ev}}

	buf.Reset()
	if err := r.pages["events"].ExecuteTemplate(&buf, "layout", evData); err != nil {
		t.Fatalf("events render failed: %v", err)
	}
	for _, want := range []string{"FailedAdmission", "K8sRequiredLabels", "must-have-owner", "denied", "alpha"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("events output missing %q", want)
		}
	}
}

func minimalLayout() ssrLayout {
	return ssrLayout{Title: "t", Version: appVersion, AssetBase: "/ssr/static"}
}

func TestExtractRegoFallsBackToCode(t *testing.T) {
	if got := extractRego(map[string]any{"rego": "inline"}); got != "inline" {
		t.Errorf("inline rego = %q, want %q", got, "inline")
	}
	target := map[string]any{"code": []any{
		map[string]any{"engine": "K8sNativeValidation", "source": map[string]any{}},
		map[string]any{"engine": "Rego", "source": map[string]any{"rego": "fromcode"}},
	}}
	if got := extractRego(target); got != "fromcode" {
		t.Errorf("code rego = %q, want %q", got, "fromcode")
	}
}

func TestFormatTimestamp(t *testing.T) {
	if got := formatTimestamp("2026-01-02T03:04:05Z"); got != "2026-01-02 03:04:05 UTC" {
		t.Errorf("formatTimestamp = %q", got)
	}
	if got := formatTimestamp("not-a-time"); got != "not-a-time" {
		t.Errorf("unparseable timestamp should pass through, got %q", got)
	}
	if got := formatTimestamp(""); got != "" {
		t.Errorf("empty timestamp should stay empty, got %q", got)
	}
}

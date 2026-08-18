// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"strings"
	"testing"
)

// The HTML violations report interpolates cluster-controlled data -- constraint and resource names,
// namespaces, violation messages. A tenant who can name a resource must not be able to plant markup
// that runs in the operator's session when they open the report. The renderer must HTML-escape it.
func TestConstraintsReportEscapesClusterData(t *testing.T) {
	const payload = `<script>alert(1)</script>`
	data := map[string]interface{}{
		"apiServerHost": "https://api.example:6443",
		"timestamp":     "now",
		"constraints": []map[string]interface{}{{
			"metadata": map[string]interface{}{"name": payload},
			"status": map[string]interface{}{
				"totalViolations": 1,
				"violations": []map[string]interface{}{{
					"enforcementAction": "deny",
					"kind":              "Pod",
					"namespace":         payload,
					"name":              payload,
					"message":           payload,
				}},
			},
		}},
	}

	var buf bytes.Buffer
	if err := newRenderer().Render(&buf, "report", data, nil); err != nil {
		t.Fatalf("rendering the report failed: %v", err)
	}
	out := buf.String()

	if strings.Contains(out, payload) {
		t.Error("the report rendered a raw <script> from cluster data — stored XSS")
	}
	if !strings.Contains(out, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Errorf("the payload was not HTML-escaped in the report")
	}
	// Benign data must still render verbatim: escaping must not mangle a normal report.
	if !strings.Contains(out, "https://api.example:6443") {
		t.Errorf("the report did not render the (benign) API server host as-is")
	}
}

// The report names the selected context so it is clear which cluster it describes on a
// multi-context kubeconfig, and omits it in-cluster where there is no context.
func TestConstraintsReportShowsContext(t *testing.T) {
	render := func(context string) string {
		data := map[string]interface{}{
			"apiServerHost": "https://api.example:6443",
			"timestamp":     "now",
			"context":       context,
		}
		var buf bytes.Buffer
		if err := newRenderer().Render(&buf, "report", data, nil); err != nil {
			t.Fatalf("rendering the report failed: %v", err)
		}
		return buf.String()
	}

	if out := render("prod-eu"); !strings.Contains(out, "(context prod-eu)") {
		t.Errorf("the report did not name the selected context")
	}
	if out := render(""); strings.Contains(out, "(context ") {
		t.Errorf("the report showed an empty context")
	}
}

// A Constraint applied since the last audit has no status.totalViolations. The report compared that
// missing value with gt, which aborts the render, so a single un-audited Constraint answered the
// "Download violations report" button with a 500 page. Verified against the live e2e cluster before
// the fix: applying one new Constraint took the report from HTTP 200 to HTTP 500 immediately.
func TestConstraintsReportHandlesUnauditedConstraints(t *testing.T) {
	constraint := func(name string, status map[string]any) map[string]any {
		c := map[string]any{"metadata": map[string]any{"name": name}}
		if status != nil {
			c["status"] = status
		}
		return c
	}

	for _, tt := range []struct {
		name       string
		constraint map[string]any
		want       string
	}{
		{"no status at all", constraint("fresh", nil), "unknown"},
		{"status without totalViolations", constraint("pending", map[string]any{"auditTimestamp": "now"}), "unknown"},
		{"audited, no violations", constraint("clean", map[string]any{"totalViolations": int64(0)}), "there are no violations"},
		{
			"a count with no list behind it",
			constraint("counted", map[string]any{"totalViolations": int64(3)}),
			"showing 0 of 3",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			data := map[string]any{
				"constraints":   []map[string]any{tt.constraint},
				"apiServerHost": "https://example.invalid",
				"timestamp":     "now",
			}
			if err := newRenderer().Render(&buf, "report", data, nil); err != nil {
				t.Fatalf("rendering the report failed: %v", err)
			}
			if !strings.Contains(buf.String(), tt.want) {
				t.Errorf("report missing %q\n%s", tt.want, buf.String())
			}
		})
	}
}

// One un-audited Constraint must not cost the report the Constraints that were audited.
func TestConstraintsReportKeepsAuditedRowsBesideUnauditedOnes(t *testing.T) {
	data := map[string]any{
		"constraints": []map[string]any{
			{"metadata": map[string]any{"name": "fresh"}},
			{
				"metadata": map[string]any{"name": "audited"},
				"status": map[string]any{
					"totalViolations": int64(1),
					"violations": []any{map[string]any{
						"enforcementAction": "deny", "kind": "Pod",
						"namespace": "default", "name": "nginx", "message": "no probe",
					}},
				},
			},
		},
		"apiServerHost": "https://example.invalid",
		"timestamp":     "now",
	}
	var buf bytes.Buffer
	if err := newRenderer().Render(&buf, "report", data, nil); err != nil {
		t.Fatalf("rendering the report failed: %v", err)
	}
	for _, want := range []string{"fresh", "unknown", "audited", "nginx", "no probe"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("report missing %q", want)
		}
	}
}

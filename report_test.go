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

// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"strings"
	"testing"
)

// The context switcher <select> navigates via x-on:change, which only fires when the element is an
// Alpine root (x-data). Without it Alpine ignores the directive and switching context silently does
// nothing. Guard the wiring at the template level, independent of any handler.
func TestContextSwitcherIsAlpineWired(t *testing.T) {
	r := newSSRRenderer()

	layout := minimalLayout()
	layout.HasContexts = true
	layout.Contexts = []ctxOption{{Name: "alpha", URL: "/constraints/alpha", Selected: true}}

	var buf bytes.Buffer
	if err := r.pages["notfound"].ExecuteTemplate(&buf, "layout", map[string]any{"Layout": layout}); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, `class="ctx-select"`) {
		t.Fatal("the context switcher did not render with contexts present")
	}
	// Read the tag itself rather than a fixed attribute order, so adding an attribute to the select
	// cannot make this fail while the wiring is intact.
	tag := out[strings.Index(out, "<select"):]
	tag = tag[:strings.Index(tag, ">")+1]
	if !strings.Contains(tag, "x-data") {
		t.Errorf("the context switcher <select> is missing x-data; x-on:change will not fire and switching does nothing: %s", tag)
	}
	// A long context name is clipped in the topbar, so the whole one has to be recoverable on hover.
	if !strings.Contains(tag, `title="alpha"`) {
		t.Errorf("the context switcher does not carry the selected context as its title: %s", tag)
	}
}

// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// The constraints list is ordered by violation count descending, then by name. Nothing else covered
// this, and the comparator was rewritten to drop per-comparison logging.
func TestConstraintOrder(t *testing.T) {
	obj := func(name string, violations int64) map[string]interface{} {
		return map[string]interface{}{
			"metadata": map[string]interface{}{"name": name},
			"status":   map[string]interface{}{"totalViolations": violations},
		}
	}

	response := []map[string]interface{}{
		obj("bravo", 1),
		{"metadata": map[string]interface{}{"name": "malformed"}}, // no status at all
		obj("alpha", 1),
		obj("charlie", 9),
	}

	sortConstraints(response)

	want := []string{"charlie", "alpha", "bravo", "malformed"}
	for i, w := range want {
		got, _, _ := unstructured.NestedString(response[i], "metadata", "name")
		if got != w {
			t.Errorf("position %d is %q, want %q", i, got, w)
		}
	}
}

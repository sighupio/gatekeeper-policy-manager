// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestAggregateDashboard(t *testing.T) {
	results := []clusterConstraints{
		{
			context: "alpha", selected: true, reachable: true,
			constraints: []ssrConstraint{
				{Kind: "K8sLivenessProbe", Name: "liveness-probe", ViolationsKnown: true, TotalViolations: 2},
				{Kind: "K8sRequiredLabels", Name: "must-have-owner", ViolationsKnown: true, TotalViolations: 0},
				// Not audited yet: known=false must not count toward totals.
				{Kind: "K8sReadinessProbe", Name: "readiness-probe", ViolationsKnown: false, TotalViolations: 99},
			},
		},
		{
			context: "beta", reachable: true,
			constraints: []ssrConstraint{
				{Kind: "K8sLivenessProbe", Name: "liveness-probe", ViolationsKnown: true, TotalViolations: 3},
				{Kind: "K8sReadinessProbe", Name: "readiness-probe", ViolationsKnown: true, TotalViolations: 1},
			},
		},
		{context: "gamma", err: errors.New("dial tcp: connection refused")},
	}

	d := aggregateDashboard(results)

	if d.TotalClusters != 3 {
		t.Errorf("TotalClusters = %d, want 3", d.TotalClusters)
	}
	if d.ReachableClusters != 2 {
		t.Errorf("ReachableClusters = %d, want 2", d.ReachableClusters)
	}
	if d.TotalConstraints != 5 {
		t.Errorf("TotalConstraints = %d, want 5", d.TotalConstraints)
	}
	if d.TotalViolations != 6 { // 2 + 3 (liveness) + 1 (readiness beta); the unaudited 99 is excluded
		t.Errorf("TotalViolations = %d, want 6", d.TotalViolations)
	}

	// Most-violated first: liveness-probe (2+3=5) aggregated across alpha and beta, then
	// readiness-probe (1) from beta only.
	if len(d.Violating) != 2 {
		t.Fatalf("Violating = %d rows, want 2", len(d.Violating))
	}
	top := d.Violating[0]
	if top.Name != "liveness-probe" || top.Violations != 5 {
		t.Errorf("top violating = %s/%d, want liveness-probe/5", top.Name, top.Violations)
	}
	if len(top.Clusters) != 2 || top.ClusterCount != 2 {
		t.Errorf("liveness-probe clusters = %d / count %d, want 2 (alpha, beta)", len(top.Clusters), top.ClusterCount)
	}
	// Kind-prefixed, so two Constraints of different Kinds sharing a name land on their own card.
	// Spelled out rather than built with constraintAnchor: an expectation computed by the function
	// under test holds whatever that function returns.
	if top.Clusters[0].URL != "/constraints/alpha#K8sLivenessProbe--liveness-probe" {
		t.Errorf("deep link = %q, want the Kind-prefixed anchor", top.Clusters[0].URL)
	}

	// Per-cluster status/state feed the sortable table + the status dot.
	byName := map[string]dashboardCluster{}
	for _, cc := range d.Clusters {
		byName[cc.Name] = cc
	}
	if got := byName["alpha"]; got.Status != "Violations" || got.State != "bad" {
		t.Errorf("alpha status = %q/%q, want Violations/bad", got.Status, got.State)
	}
	if got := byName["gamma"]; got.Reachable || got.Status != "Unreachable" || got.State != "warn" {
		t.Errorf("gamma = %+v, want unreachable Unreachable/warn", got)
	}

	// Clusters donut: 2 violating + 1 unreachable; the 0 compliant slice is omitted, and the segments
	// carry ring geometry.
	cd := d.ClustersDonut
	if cd.Center != "3" {
		t.Errorf("ClustersDonut center = %q, want 3", cd.Center)
	}
	if len(cd.Segments) != 2 {
		t.Fatalf("ClustersDonut segments = %d, want 2 (violating, unreachable)", len(cd.Segments))
	}
	if cd.Segments[0].Label != "With violations" || cd.Segments[0].Count != 2 || cd.Segments[0].DashArray == "" {
		t.Errorf("ClustersDonut[0] = %+v, want With violations/2 with geometry", cd.Segments[0])
	}
}

// An unreachable cluster's raw API error can name internal hosts/IPs and cert details; it must not
// reach the (Anonymous-readable) dashboard HTML, only the "Unreachable" status.
func TestUnreachableClusterErrorNotDisclosed(t *testing.T) {
	r := newSSRRenderer()
	d := aggregateDashboard([]clusterConstraints{
		{context: "gamma", err: errors.New("dial tcp 10.4.2.11:6443: connect: connection refused")},
	})

	var buf bytes.Buffer
	if err := r.pages["home"].ExecuteTemplate(&buf, "layout", map[string]any{"Layout": minimalLayout(), "Dashboard": d}); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "10.4.2.11") || strings.Contains(out, "connection refused") {
		t.Error("raw cluster error leaked into the dashboard HTML; it must stay server-side only")
	}
	if !strings.Contains(out, "Unreachable") {
		t.Error("expected the unreachable cluster to still render its Unreachable status")
	}
}

func TestClusterLabelAndURL(t *testing.T) {
	if got := clusterLabel(""); got != "current cluster" {
		t.Errorf("clusterLabel(\"\") = %q, want \"current cluster\"", got)
	}
	if got := clusterLabel("prod"); got != "prod" {
		t.Errorf("clusterLabel(\"prod\") = %q, want \"prod\"", got)
	}
	// The default context links to /constraints with no context segment.
	if got := constraintsURL("", "", ""); got != "/constraints" {
		t.Errorf("constraintsURL with nothing set = %q, want /constraints", got)
	}
	want := "/constraints/prod#K8sRequiredLabels--must-have-owner"
	if got := constraintsURL("prod", "K8sRequiredLabels", "must-have-owner"); got != want {
		t.Errorf("constraintsURL = %q, want %q", got, want)
	}
}

func TestHomeDashboardRenders(t *testing.T) {
	r := newSSRRenderer()

	d := dashboardData{
		TotalClusters: 2, ReachableClusters: 2, TotalConstraints: 4, TotalViolations: 6,
		ClustersDonut: newDonut("2", []donutSegment{
			{Label: "With violations", Count: 1, Class: "danger"},
			{Label: "Compliant", Count: 1, Class: "success"},
		}),
		EnforcementDonut: newDonut("4", []donutSegment{
			{Label: "Deny", Count: 4, Class: "accent"},
		}),
		Clusters: []dashboardCluster{
			{Name: "alpha", Selected: true, Reachable: true, ConstraintCount: 2, Violations: 5, ConstraintsURL: "/constraints/alpha", Status: "Violations", State: "bad"},
			{Name: "gamma", Reachable: false, ConstraintsURL: "/constraints/gamma", Status: "Unreachable", State: "warn"},
		},
		Violating: []dashboardConstraint{
			{Kind: "K8sLivenessProbe", Name: "liveness-probe", Violations: 5, Clusters: []dashboardConstraintCluster{
				{Cluster: "alpha", Violations: 2, URL: "/constraints/alpha#liveness-probe"},
				{Cluster: "beta", Violations: 3, URL: "/constraints/beta#liveness-probe"},
			}},
		},
	}
	data := map[string]any{"Layout": minimalLayout(), "Dashboard": d}

	var buf bytes.Buffer
	if err := r.pages["home"].ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatalf("home render failed: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"Total violations", "Constraints by enforcement", "donut-seg", "With violations",
		"liveness-probe", "alpha", "Unreachable",
		"/constraints/alpha#liveness-probe", "dashboard-table.js",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("home output missing %q", want)
		}
	}
}

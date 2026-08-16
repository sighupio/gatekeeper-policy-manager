// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Shared Kubernetes read helpers, the health probe, and the error answer shape. The views that use
// these live in ssr.go; there is no JSON API any more.
package main

import (
	"context"
	"net/http"
	"sort"

	"github.com/labstack/echo/v4"
	"golang.org/x/exp/slog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// The shape the auth middleware returns for an unauthenticated /api/* request.
type ErrorAnswer struct {
	ErrorMessage string `json:"error"`
	Action       string `json:"action"`
	Description  string `json:"description"`
	// Set only when signing in would fix the error, so a client can offer a button instead of
	// leaving the user to read a path out of the message. Omitted from every other error.
	LoginURL string `json:"login_url,omitempty"`
}

// Helper function to get custom resources from the Kubernetes API of the specified group, verison and resource.
// Parameters can be an empty string.
func getCustomResources(ctx context.Context, clientset dynamic.DynamicClient, group string, version string, resource string) (*unstructured.UnstructuredList, error) {
	r := schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
	return clientset.Resource(r).List(ctx, metav1.ListOptions{})
}

// Health probe. Always returns OK.
func getHealth(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Orders the constraints list: most violations first, then by name. A malformed object sorts as
// ("", 0) rather than logging once per comparison, which is what the previous inline comparator did.
func sortConstraints(response []map[string]interface{}) {
	key := func(o map[string]interface{}) (string, int64) {
		name, _, _ := unstructured.NestedString(o, "metadata", "name")
		violations, _, _ := unstructured.NestedInt64(o, "status", "totalViolations")
		return name, violations
	}
	sort.Slice(response, func(i, j int) bool {
		iName, iViolations := key(response[i])
		jName, jViolations := key(response[j])
		if iViolations != jViolations {
			return iViolations > jViolations
		}
		return iName < jName
	})
}

// Returns the events whose source.component is one of the given sources (Gatekeeper tags admission
// events with gatekeeper-webhook and audit events with gatekeeper-audit). An empty namespace lists
// events from every namespace.
func getKubernetesEvents(ctx context.Context, clientset dynamic.DynamicClient, namespace string, sources []string) (*[]unstructured.Unstructured, error) {
	// FieldSeletor is very limited in the supported fields, we can't filter like this:
	//   listOptions := metav1.ListOptions{
	// 	  FieldSelector: "involvedObject.metadata.source.component=gatekeeper-webhook", //Filter events related to Pods
	//   }
	// so we need to filter the events manually with the for loop below

	r := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}
	events, err := clientset.Resource(r).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	want := make(map[string]bool, len(sources))
	for _, s := range sources {
		want[s] = true
	}

	var filteredList []unstructured.Unstructured
	for i := range events.Items {
		source, found, err := unstructured.NestedString(events.Items[i].Object, "source", "component")
		if found && err == nil && want[source] {
			filteredList = append(filteredList, events.Items[i])
		} else if err != nil {
			slog.Debug("error getting event source", "event", events.Items[i].GetName(), "error", err)
		}
	}
	return &filteredList, nil
}

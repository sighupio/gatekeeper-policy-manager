// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The HTTP handlers for the API, and the answer shapes they share.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
	"golang.org/x/exp/slog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// The answer every handler gives when the requested kubeconfig context cannot be used.
func contextErrorAnswer(c echo.Context, err error) error {
	name := c.Param("context")
	slog.Error("resolving the Kubernetes context failed", "context", name, "error", err)
	return c.JSON(http.StatusInternalServerError, ErrorAnswer{
		ErrorMessage: fmt.Sprintf("Got an error while trying to switch to context %s", name),
		Action:       "Please check the context definition in the Kubeconfig file.",
		Description:  err.Error(),
	})
}

type ErrorAnswer struct {
	ErrorMessage string `json:"error"`
	Action       string `json:"action"`
	Description  string `json:"description"`
	// Set only when signing in would fix the error, so the frontend can offer a button instead of
	// leaving the user to read a path out of the message. Omitted from every other error.
	LoginURL string `json:"login_url,omitempty"`
}

// Builds the payload for a failed Kubernetes API call. When the failure turns out to be a TLS
// certificate problem it replaces the message and the suggested action with a pointer to
// GPM_SKIP_TLS_VERIFY, which is the usual fix on clusters whose CA lacks the AKI/SKI extensions.
func kubeAPIErrorAnswer(message string, action string, err error) ErrorAnswer {
	var (
		verificationErr *tls.CertificateVerificationError
		authorityErr    x509.UnknownAuthorityError
		hostnameErr     x509.HostnameError
		invalidCertErr  x509.CertificateInvalidError
	)
	if errors.As(err, &verificationErr) || errors.As(err, &authorityErr) ||
		errors.As(err, &hostnameErr) || errors.As(err, &invalidCertErr) {
		message = "TLS certificate verification failed while connecting to the Kubernetes API."
		action = "Set GPM_SKIP_TLS_VERIFY=true if the cluster CA is missing the AKI/SKI extensions, as happens on EKS. Use with caution."
	}
	return ErrorAnswer{ErrorMessage: message, Action: action, Description: err.Error()}
}

// Helper function to get custom resources from the Kubernetes API of the specified group, verison and resource.
// Parameters can be an empty string.
func getCustomResources(clientset dynamic.DynamicClient, group string, version string, resource string) (*unstructured.UnstructuredList, error) {
	r := schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
	return clientset.Resource(r).List(context.TODO(), metav1.ListOptions{})
}

// Health probe. Always returns OK.
func getHealth(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Returns a JSON with information about the auth configuration. The frontend calls this before
// login to decide whether to show the logout control, so it stays reachable without a session.
func getAuth(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]bool{"auth_enabled": authEnabled()})
}

// Returns a list of the available contexts in the kubeconfig file and the active context
func (s *server) getContexts(c echo.Context) error {
	// We need to format the response to align with the old Python backend (API v1)
	type context struct {
		Cluster string `json:"cluster"`
		User    string `json:"user"`
	}

	type kubeconfigContext struct {
		Name    string  `json:"name"`
		Context context `json:"context"`
	}

	contexts, current := s.k8s.contexts()

	kubeconfigContexts := []kubeconfigContext{}
	var currentKubeconfigContext kubeconfigContext
	for kc := range contexts {
		c := context{
			Cluster: contexts[kc].Cluster,
			User:    contexts[kc].AuthInfo,
		}
		fullContext := kubeconfigContext{
			Name:    kc,
			Context: c,
		}
		kubeconfigContexts = append(kubeconfigContexts, fullContext)
		// The kubeconfig's own default. There is no server-side "selected" context any more:
		// the frontend tracks that and names it in the request path.
		if kc == current {
			currentKubeconfigContext = fullContext
		}
	}

	v1Answer := []interface{}{kubeconfigContexts, currentKubeconfigContext}
	return c.JSON(http.StatusOK, v1Answer)
}

// Returns a JSON with all the available Gatekeeper Configuration objects.
// Gatekeeper only supports a single configuration object defined in the cluster but we return a list for future proofing.
func (s *server) getConfigs(c echo.Context) error {
	clients, err := s.clientsFor(c)
	if err != nil {
		return contextErrorAnswer(c, err)
	}
	configResources, err := getCustomResources(*clients.dynamic, "config.gatekeeper.sh", "v1alpha1", "configs")
	if err != nil {
		slog.Debug("getting config resources failed", "error", err)
		return c.JSON(http.StatusInternalServerError, kubeAPIErrorAnswer(
			"An error ocurred while getting config objects from Kubernetes API.",
			"Check that the Kubeconfig file is correct and that the Kubernetes API is accessible.",
			err))
	}
	return c.JSON(http.StatusOK, configResources.Items)
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

// Returns a JSON with all the Constraint Templates in the target cluster and a map with the all Constraints that exist
// for each Constraint Template.
func (s *server) getConstraintTemplates(c echo.Context) error {
	clients, err := s.clientsFor(c)
	if err != nil {
		return contextErrorAnswer(c, err)
	}

	var response struct {
		Constrainttemplates                []unstructured.Unstructured            `json:"constrainttemplates"`
		Constraints_by_constrainttemplates map[string][]unstructured.Unstructured `json:"constraints_by_constrainttemplates"`
	}
	// we need to initialize the variable otherwise assigning to a map memeber panics
	response.Constraints_by_constrainttemplates = make(map[string][]unstructured.Unstructured)

	// get all constraint templates
	constrainttemplates, err := getCustomResources(*clients.dynamic, "templates.gatekeeper.sh", "v1", "constrainttemplates")
	if err != nil {
		slog.Error("getting Constraint Templates resources failed", "error", err)
		return c.JSON(http.StatusInternalServerError, kubeAPIErrorAnswer(
			"An error ocurred while getting Constraint Templates objects from Kubernetes API",
			"Is Gatekeeper properly installed in the cluster?",
			err))
	}
	// map all the constraints available for each constraint template
	for _, ct := range constrainttemplates.Items {
		ctName := ct.GetName()
		constraints, err := getCustomResources(*clients.dynamic, "constraints.gatekeeper.sh", "v1beta1", ctName)
		if err != nil {
			slog.Debug("trying to get Constraints for ConstraintTemplate failed", "constraintTemplate", ctName, "error", err)
			constraints = &unstructured.UnstructuredList{} // if there are no constraints for the template, we return an empty list
		}
		response.Constraints_by_constrainttemplates[ctName] = constraints.Items
	}
	response.Constrainttemplates = constrainttemplates.Items
	return c.JSON(http.StatusOK, response)
}

// Will discover all the constraint Kinds and their objects and will return:
// - by default: a JSON with all the constraints objects sorted by 1. number of violations and 2. alphabetically.
// - when a "report" Query parameter is present in the URL: an HTML report of the violations made from a template.
func (s *server) getConstraints(c echo.Context) error {
	clients, err := s.clientsFor(c)
	if err != nil {
		return contextErrorAnswer(c, err)
	}

	var response []map[string]interface{}

	// constraints are a kind by themselves. The resource Kind is created dynamically by Gateeeper for each template.
	// we need to discover the available Kinds for the constraints first.
	availableConstraints, err := clients.discovery.ServerResourcesForGroupVersion("constraints.gatekeeper.sh/v1beta1")
	if err != nil {
		slog.Error("listing constraints kinds from Kubernetes API server failed", "error", err)
		return c.JSON(http.StatusInternalServerError, kubeAPIErrorAnswer(
			"An error ocurred while trying to list the Constraints",
			"Is Gatekeeper properly installed in the target Kubernetes cluster?",
			err))
	}

	for _, constraintKind := range availableConstraints.APIResources {
		// we are interested in the root resources only.
		// subresources (like <kind>/status) seem to have the categories field emtpy, so we use that to filter them out.
		if constraintKind.Categories != nil {
			constraints, err := getCustomResources(*clients.dynamic, "constraints.gatekeeper.sh", "v1beta1", constraintKind.SingularName)
			if err != nil {
				slog.Error("getting Constraint resources failed", "error", err)
				return c.JSON(http.StatusInternalServerError, kubeAPIErrorAnswer(
					"An error ocurred while getting constraint objects from Kubernetes API",
					"Is Gatekeeper properly deployed in the target cluster?",
					err))
			}
			for _, i := range constraints.Items {
				response = append(response, i.Object)
			}
		}
	}

	// Sort the constraints by 1. totalViolations and 2. by name for better UX and easier e2e testing.
	sortConstraints(response)

	// We support HTML reports only for now, so we don't check the param value, just that is present.
	if c.QueryParam("report") != "" {
		data := map[string]interface{}{
			"constraints":   response,
			"apiServerHost": clients.rest.Host,
			"timestamp":     time.Now().Format(time.ANSIC),
		}

		return c.Render(http.StatusOK, "report", data)
	}

	// v1 API compatibility
	// We need to return an empty list instead of null when there are no objects as the Python backend did
	// otherwise the frontend breaks
	if len(response) == 0 {
		return c.JSON(http.StatusOK, []string{})
	}
	return c.JSON(http.StatusOK, response)
}

// Returns a JSON with all the Constraint Templates in the target cluster and a map with the all Constraints that exist
// for each Constraint Template.
func (s *server) getMutations(c echo.Context) error {
	clients, err := s.clientsFor(c)
	if err != nil {
		return contextErrorAnswer(c, err)
	}

	// Mutators are well-known, but we could use dynamic client like we do for the constraints
	// to discover the available kinds.
	mutators := []string{"assign", "assignmetadata", "modifyset", "assignimage"}
	var response []interface{}

	for _, mutator := range mutators {
		slog.Debug("getting mutations", "kind", mutator)
		mutations, err := getCustomResources(*clients.dynamic, "mutations.gatekeeper.sh", "v1", mutator)
		if err != nil {
			// We get an error when there are no mutations defined in the cluster also,so we don't return
			slog.Error("getting mutator resources failed", "mutator", mutator, "error", err)
		} else {
			for _, i := range mutations.Items {
				response = append(response, i.Object)
			}
		}
	}

	if len(response) == 0 {
		return c.JSON(http.StatusOK, []string{})
	}
	return c.JSON(http.StatusOK, response)
}

// Returns a slice of unstructured objects with all the events generated by the 'gatekeeper-wbhook' source
// If namespace is an empty string, it returns the events from all namespaces.
func getKubernetesEvents(clientset dynamic.DynamicClient, namespace string, eventsSource string) (*[]unstructured.Unstructured, error) {
	// FieldSeletor is very limited in the supported fields, we can't filter like this:
	//   listOptions := metav1.ListOptions{
	// 	  FieldSelector: "involvedObject.metadata.source.component=gatekeeper-webhook", //Filter events related to Pods
	//   }
	// so we need to filter the events manually with the for loop below

	r := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}
	events, err := clientset.Resource(r).Namespace(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var filteredList []unstructured.Unstructured
	for i := range events.Items {
		source, found, err := unstructured.NestedString(events.Items[i].Object, "source", "component")
		if found && err == nil && source == eventsSource {
			filteredList = append(filteredList, events.Items[i])
		} else if err != nil {
			slog.Debug("error getting event source", "event", events.Items[i].GetName(), "error", err)
		}
	}
	return &filteredList, nil
}

// Returns the events from the configured source (GPM_EVENTS_SOURCE, "gatekeeper-webhook" by
// default) as unstructured objects. By default it reads the namespace GPM runs in; the "namespace"
// query parameter, or GPM_EVENTS_NAMESPACE, changes that.
//
// getKubernetesEvents reads the core v1 Event API and filters on source.component, which is
// correct for how Gatekeeper emits today. If Gatekeeper moves to events.k8s.io/v1
// (reportingController, not source.component) the filter matches nothing and the view goes silently
// empty. This path has no end-to-end test, so the events view is alpha.
func (s *server) getEvents(c echo.Context) error {
	clients, err := s.clientsFor(c)
	if err != nil {
		return contextErrorAnswer(c, err)
	}

	// TODO: maybe we should Lookup this once at start-time and save it instead of on each call to this func
	eventsSource := viper.GetString("events_source")

	// GPM_EVENTS_NAMESPACE wins over the query parameter. It is what the deployment's RBAC is cut
	// to, so letting a request widen it would only produce a 403 from the Kubernetes API.
	namespace := viper.GetString("events_namespace")
	if namespace == "" {
		namespace = c.QueryParam("namespace")
	}

	events, err := getKubernetesEvents(*clients.dynamic, namespace, eventsSource)
	if err != nil {
		slog.Error("got error while getting namespace events", "namespace", namespace, "source", eventsSource, "error", err)
		return c.JSON(http.StatusInternalServerError, kubeAPIErrorAnswer(
			"An error ocurred while getting events from Kubernetes API.",
			"Check that the Kubconfig file is correct and the Kubernetes API accessible.",
			err))
	}

	return c.JSON(http.StatusOK, events)
}

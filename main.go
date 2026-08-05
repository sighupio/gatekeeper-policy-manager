// Copyright (c) 2023 SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The backend for Gatekeeper Policy Manager, a simple to use web-based UI for OPA Gatekeeper
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd/api"

	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/spf13/viper"

	"golang.org/x/exp/slog"
)

// Holds everything the handlers need. They are methods on it rather than package-level functions
// so that nothing reaches for shared mutable state, and so tests can build one with fakes.
type server struct {
	k8s *clientRegistry
}

// Resolves the Kubernetes clients for the context named in the route, or the kubeconfig default
// when the route carries no :context.
func (s *server) clientsFor(c echo.Context) (*kubeClients, error) {
	return s.k8s.forContext(c.Param("context"))
}

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

// Where the built frontend lives, relative to the working directory.
const staticContentDir = "./static-content"

// The value GPM 1.x shipped as its default. It is published in the source tree, so a session
// cookie signed with it can be forged by anyone; enabling authentication with it is refused.
const insecureDefaultSecretKey = "g8k1p3rp0l1c7m4n4g3r"

// How long a session lasts when GPM_SESSION_MAX_AGE is unset or not a positive number.
const defaultSessionMaxAge = 60 * 60 * 8

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

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

// Helper function to get custom resources from the Kubernetes API of the specified group, verison and resource.
// Parameters can be an empty string.
func getCustomResources(clientset dynamic.DynamicClient, group string, version string, resource string) (*unstructured.UnstructuredList, error) {
	r := schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
	res, err := clientset.Resource(r).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// Serves a file from the built frontend, falling back to index.html so that client-side routing
// works. See https://create-react-app.dev/docs/deployment#serving-apps-with-client-side-routing.
// We could avoid this by serving the frontend from another process/container instead.
func serveIndex(c echo.Context) error {
	root, err := filepath.Abs(staticContentDir)
	if err != nil {
		slog.Error("could not resolve the static content directory", "error", err)
		return serveSPAShell(c)
	}

	// URL.Path, never RequestURI: the latter keeps the query string and is not normalised, so
	// joining it into a filesystem path lets a request like "/logout?x=/../../etc/passwd" walk out
	// of the static root. Clean resolves the "..", then the prefix check catches anything left.
	requested := path.Clean("/" + c.Request().URL.Path)
	target := filepath.Join(root, filepath.FromSlash(requested))
	if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
		slog.Warn("refusing to serve a path outside the static content directory",
			"requested", requested)
		return serveSPAShell(c)
	}

	if requested != "/" {
		if info, statErr := os.Stat(target); statErr == nil && !info.IsDir() {
			slog.Debug("found file, serving it", "path", target)
			return c.File(target)
		}
	}
	slog.Debug("file not found, falling back to index.html")
	return serveSPAShell(c)
}

// Serves the SPA entry point without consulting the request path at all.
func serveSPAShell(c echo.Context) error {
	return c.File(filepath.Join(staticContentDir, "index.html"))
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
	sort.Slice(response, func(i, j int) bool {
		iName, _, err := unstructured.NestedString(response[i], "metadata", "name")
		if err != nil {
			slog.Error("trying to get object name failed", "object", response[i], "error", err)
		}
		iViolations, _, err := unstructured.NestedInt64(response[i], "status", "totalViolations")
		if err != nil {
			slog.Error("trying to get the total violations counts failed", "constraint", iName, "error", err)
		}
		jName, _, err := unstructured.NestedString(response[j], "metadata", "name")
		if err != nil {
			slog.Error("trying to get object name failed", "object", response[i], "error", err)
		}
		jViolations, _, err := unstructured.NestedInt64(response[j], "status", "totalViolations")
		if err != nil {
			slog.Error("trying to get the total violations counts failed", "constraint", jName, "error", err)
		}
		if iViolations == jViolations {
			return strings.Compare(iName, jName) < 0
		}
		return iViolations > jViolations
	})

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
		res := fmt.Sprintf("%s.mutations.gatekeeper.sh", mutator)
		slog.Debug("getting mutations", "kind", res)
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

	if response != nil {
		return c.JSON(http.StatusOK, response)
	}
	return c.JSON(http.StatusOK, []string{})
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

// Returns a JSON with a list of all the events generated by 'gatekeeper-wbhook' (or the configured source)
// as unustructured objects.
// By default it gets all the events from the same namespace where GPM is running, but it can be changed
// with the "namespace" URL Query Paramter.
// TODO: I'm not sure we are getting the response from the API server in the right schema version.
//
//	See: https://v1-25.docs.kubernetes.io/docs/reference/kubernetes-api/cluster-resources/event-v1/
func (s *server) getEvents(c echo.Context) error {
	clients, err := s.clientsFor(c)
	if err != nil {
		return contextErrorAnswer(c, err)
	}

	// TODO: maybe we should Lookup this once at start-time and save it instead of on each call to this func
	eventsSource := viper.GetString("events_source")
	events, err := getKubernetesEvents(*clients.dynamic, c.QueryParam("namespace"), eventsSource)
	if err != nil {
		slog.Error("got error while getting namespace events", "namespace", c.QueryParam("namespace"), "source", eventsSource, "error", err)
		return c.JSON(http.StatusInternalServerError, kubeAPIErrorAnswer(
			"An error ocurred while getting events from Kubernetes API.",
			"Check that the Kubconfig file is correct and the Kubernetes API accessible.",
			err))
	}

	return c.JSON(http.StatusOK, events)
}

// Declares every setting GPM reads, with its default and its GPM_-prefixed environment variable.
//
// Separate from main() so that a test can put viper in exactly the state a running GPM would see,
// instead of one assembled out of whatever the previous test left behind.
func bindSettings() {
	viper.SetEnvPrefix("gpm") // will be uppercased automatically
	// BindEnv only errors when called without a key, so the error is unreachable here.
	_ = viper.BindEnv("log_level")
	viper.SetDefault("log_level", "INFO")
	_ = viper.BindEnv("listen_address")
	viper.SetDefault("listen_address", ":8080")
	_ = viper.BindEnv("events_source")
	viper.SetDefault("events_source", "gatekeeper-webhook")
	_ = viper.BindEnv("skip_tls_verify")
	viper.SetDefault("skip_tls_verify", false)

	// Authentication. Names match the Python backend's environment variables so that an existing
	// 1.x deployment can be pointed at this image without rewriting its configuration.
	_ = viper.BindEnv("auth_enabled")
	viper.SetDefault("auth_enabled", "Anonymous")
	_ = viper.BindEnv("secret_key")
	viper.SetDefault("secret_key", insecureDefaultSecretKey)
	_ = viper.BindEnv("preferred_url_scheme")
	viper.SetDefault("preferred_url_scheme", "http")
	_ = viper.BindEnv("session_max_age")
	viper.SetDefault("session_max_age", defaultSessionMaxAge)
	for _, k := range []string{
		"oidc_redirect_domain",
		"oidc_client_id",
		"oidc_client_secret",
		"oidc_issuer",
		"oidc_authorization_endpoint",
		"oidc_token_endpoint",
		"oidc_jwks_uri",
		"oidc_introspection_endpoint",
		"oidc_userinfo_endpoint",
		"oidc_end_session_endpoint",
	} {
		_ = viper.BindEnv(k)
	}
}

func main() {
	bindSettings()

	// Initilize Echo HTTP server
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	p := prometheus.NewPrometheus("echo", nil)
	// /metrics is reachable without a session so Prometheus can scrape it, which also means any
	// client can influence the labels. Host comes straight from the request header, so leaving it
	// in lets one caller create unbounded series in a process that never forgets them.
	p.RequestCounterHostLabelMappingFunc = func(echo.Context) string { return "" }
	p.Use(e)

	// Setup logging
	e.Use(middleware.Recover())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogURI:           true,
		LogStatus:        true,
		LogRemoteIP:      true,
		LogHost:          true,
		LogUserAgent:     true,
		LogError:         true,
		LogLatency:       true,
		LogContentLength: true,
		LogResponseSize:  true,
		LogMethod:        true,
		LogValuesFunc: func(c echo.Context, values middleware.RequestLoggerValues) error {
			slog.Info(
				"received request",
				"remote_ip", values.RemoteIP,
				"host", values.Host,
				"method", values.Method,
				"uri", values.URI,
				"user_agent", values.UserAgent,
				"status", values.Status,
				"error", values.Error,
				"latency", values.Latency,
				"latency_human", values.Latency.Microseconds(),
				"bytes_in", values.ContentLength,
				"bytes_out", values.ResponseSize,
			)
			return nil
		},
	}))

	programLevel := new(slog.LevelVar)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: programLevel}))
	slog.SetDefault(logger)

	slog.Info("starting Gatekeeper Policy Manager", "version", "v2.0.0-alpha1")
	switch strings.ToLower(viper.GetString("log_level")) {
	case "debug":
		slog.Info("changed log level", "log_level", "DEBUG")
		programLevel.Set(slog.LevelDebug)
	case "info":
		slog.Info("changed log level", "log_level", "INFO")
		programLevel.Set(slog.LevelInfo)
	case "warn":
		slog.Info("changed log level", "log_level", "WARN")
		programLevel.Set(slog.LevelWarn)
	case "error":
		slog.Info("changed log level", "log_level", "ERROR")
		programLevel.Set(slog.LevelError)
	default:
		slog.Warn("the requested log level is not a valid option", "log_level", "INFO", "requested_level", viper.GetString("log_level"))
		programLevel.Set(slog.LevelInfo)
	}

	// We compile the HTML templates here
	// This is used later to render templates in the routes (i.e. to render the HTML report in the `/constraints/?report=html` route).
	e.Renderer = &Template{
		templates: template.Must(template.ParseGlob("templates/*.html.gotpl")),
	}

	// CORS configuration for frontend development purposes
	if os.Getenv("APP_ENV") == "development" {
		origins := []string{"http://localhost:3000", "http://localhost:3001"}
		headers := []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept}
		slog.Warn("running in development mode, allowing CORS from other origins", "origins", origins, "headers", headers)
		e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: origins,
			AllowHeaders: headers,
		}))
	}

	// Authentication. When it is off, no session or auth middleware is installed at all, so the
	// unauthenticated path stays exactly as it was.
	var auth *authenticator
	if authEnabled() {
		if viper.GetString("secret_key") == insecureDefaultSecretKey {
			slog.Error("GPM_SECRET_KEY is still the default value from GPM 1.x. " +
				"It is published in the source tree, so anyone could forge a session cookie. " +
				"Set it to a long random string before enabling authentication.")
			os.Exit(1)
		}

		// Discovery talks to the identity provider before the server starts listening, so a slow or
		// unreachable provider would otherwise hang the pod with no health endpoint to report it.
		authCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var authErr error
		auth, authErr = newAuthenticator(authCtx)
		if authErr != nil {
			slog.Error("OIDC authentication could not be configured", "error", authErr)
			os.Exit(1)
		}
		e.Use(session.Middleware(newSessionStore()))
		e.Use(auth.middleware())
	} else {
		slog.Warn("authentication is disabled, GPM is readable by anyone who can reach it")
	}

	registry, err := newClientRegistry()
	if err != nil {
		slog.Error("Kubernetes client initialization failed", "error", err)
		os.Exit(1)
	}
	s := &server{k8s: registry}

	// Routes configuration

	if auth != nil {
		e.GET(callbackPath, auth.callback)
		e.GET("/login", auth.login)
		e.GET("/logout", auth.logout)
	}

	e.Static("/static/", "./static-content/static")
	// Fallback route for all non-matching URLs.
	// We need to serve index.html for react routing to work. See:
	// https://create-react-app.dev/docs/deployment#serving-apps-with-client-side-routing.
	// We could avoid this by serving the frontend from another process/container instead of from the backend
	e.GET("/*", serveIndex)

	e.GET("/health", getHealth)
	e.GET("/health/", getHealth)

	e.GET("/api/v1/auth", getAuth)
	e.GET("/api/v1/auth/", getAuth)

	e.GET("/api/v1/contexts", s.getContexts)
	e.GET("/api/v1/contexts/", s.getContexts)

	e.GET("/api/v1/configs", s.getConfigs)
	e.GET("/api/v1/configs/", s.getConfigs)
	e.GET("/api/v1/configs/:context", s.getConfigs)
	e.GET("/api/v1/configs/:context/", s.getConfigs)

	e.GET("/api/v1/constrainttemplates", s.getConstraintTemplates)
	e.GET("/api/v1/constrainttemplates/", s.getConstraintTemplates)
	e.GET("/api/v1/constrainttemplates/:context", s.getConstraintTemplates)
	e.GET("/api/v1/constrainttemplates/:context/", s.getConstraintTemplates)

	e.GET("/api/v1/constraints", s.getConstraints)
	e.GET("/api/v1/constraints/", s.getConstraints)
	e.GET("/api/v1/constraints/:context", s.getConstraints)
	e.GET("/api/v1/constraints/:context/", s.getConstraints)

	e.GET("/api/v1/mutations", s.getMutations)
	e.GET("/api/v1/mutations/", s.getMutations)
	e.GET("/api/v1/mutations/:context", s.getMutations)
	e.GET("/api/v1/mutations/:context/", s.getMutations)

	e.GET("/api/v1/events", s.getEvents)
	e.GET("/api/v1/events/", s.getEvents)
	e.GET("/api/v1/events/:context", s.getEvents)
	e.GET("/api/v1/events/:context/", s.getEvents)

	// Returns an object with the list of available contets and the currently selected context
	e.GET("/api/v2/contexts/", func(c echo.Context) error {
		type v2Answer struct {
			Current  string                  `json:"currentContext"`
			Contexts map[string]*api.Context `json:"contexts"`
		}

		contexts, current := s.k8s.contexts()
		return c.JSON(http.StatusOK, v2Answer{current, contexts})
	})

	address := viper.GetString("listen_address")

	slog.Info("starting HTTP server", "address", address)
	slog.Error("starting HTTP server failed", "error", e.Start(address), "address", address)
	os.Exit(1)
}

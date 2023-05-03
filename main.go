// Copyright (c) 2023 SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The backend for Gatekeeper Policy Manager, a simple to use web-based UI for OPA Gatekeeper
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"k8s.io/client-go/discovery"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"

	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	config             *rest.Config
	startingConfig     *api.Config
	clientset          *dynamic.DynamicClient
	discoveryClient    *discovery.DiscoveryClient
	logLevelFromString = map[string]log.Lvl{
		"DEBUG": log.DEBUG,
		"INFO":  log.INFO,
		"WARN":  log.WARN,
		"ERROR": log.ERROR,
	}
)

type ErrorAnswer struct {
	ErrorMessage string `json:"error"`
	Action       string `json:"action"`
	Description  string `json:"description"`
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

// Health probe. Always returns OK.
func getHealth(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Returns a JSON with information about the auth configuration.
// The Go backend does not support auth yet. So we always return auth disabled for compatibility with the old Python backend
func getAuth(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]bool{"auth_enabled": false})
}

// Returns a list of the available contexts in the kubeconfig and the current active context
func getContexts(c echo.Context) error {
	// We need to format the response to align with the old Python backend (API v1)
	// We could just return the startingConfig.Contexts object in a v2.

	type context struct {
		Cluster string `json:"cluster"`
		User    string `json:"user"`
	}

	type kubeconfigContext struct {
		Name    string  `json:"name"`
		Context context `json:"context"`
	}

	kubeconfigContexts := []kubeconfigContext{}
	var currentKubeconfigContext kubeconfigContext
	for kc := range startingConfig.Contexts {
		c := context{
			Cluster: startingConfig.Contexts[kc].Cluster,
			User:    startingConfig.Contexts[kc].AuthInfo,
		}
		fullContext := kubeconfigContext{
			Name:    kc,
			Context: c,
		}
		kubeconfigContexts = append(kubeconfigContexts, fullContext)
		if kc == startingConfig.CurrentContext {
			currentKubeconfigContext = fullContext
		}
	}
	v1Answer := []interface{}{kubeconfigContexts, currentKubeconfigContext}

	return c.JSON(http.StatusOK, v1Answer)
}

// Returns a JSON with all the available Gatekeeper Configuration objects.
// Gatekeeper only supports a single configuration object defined in the cluster but we return a list for future proofing.
func getConfigs(c echo.Context) error {
	if c.Param("context") != "" {
		c.Echo().Logger.Debug("switching to custom context ", c.Param("context"))
		err := switchKubernetesContext(c.Echo(), c.Param("context"))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorAnswer{
				ErrorMessage: fmt.Sprintf("Got an error while trying to switch to context %s", c.Param("context")),
				Action:       "Please check the context definition in the Kubeconfig file.",
				Description:  err.Error(),
			})
		}
	}
	configResources, err := getCustomResources(*clientset, "config.gatekeeper.sh", "v1alpha1", "configs")
	if err != nil {
		c.Echo().Logger.Debug("got error while getting config resources: ", err)
		return c.JSON(http.StatusInternalServerError, ErrorAnswer{
			ErrorMessage: "An error ocurred while getting config objects from Kubernetes API.",
			Description:  err.Error(),
			Action:       "Check that the Kubconfig file is correct and the Kubernetes API accessible."})
	}
	return c.JSON(http.StatusOK, configResources.Items)
}

// Returns a JSON with all the Constraint Templates in the target cluster and a map with the all Constraints that exist
// for each Constraint Template.
func getConstraintTemplates(c echo.Context) error {
	if c.Param("context") != "" {
		c.Echo().Logger.Debug("switching to custom context ", c.Param("context"))
		err := switchKubernetesContext(c.Echo(), c.Param("context"))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorAnswer{
				ErrorMessage: fmt.Sprintf("Got an error while trying to switch to context %s", c.Param("context")),
				Action:       "Please check the context definition in the Kubeconfig file.",
				Description:  err.Error(),
			})
		}
	}
	var response struct {
		Constrainttemplates                []unstructured.Unstructured            `json:"constrainttemplates"`
		Constraints_by_constrainttemplates map[string][]unstructured.Unstructured `json:"constraints_by_constrainttemplates"`
	}
	// we need to initialize the variable otherwise assigning to a map memeber panics
	response.Constraints_by_constrainttemplates = make(map[string][]unstructured.Unstructured)

	constrainttemplates, err := getCustomResources(*clientset, "templates.gatekeeper.sh", "v1beta1", "constrainttemplates")
	if err != nil {
		c.Echo().Logger.Error("got error while getting constraint templates resources: ", err)
		return c.JSON(http.StatusInternalServerError, ErrorAnswer{
			ErrorMessage: "An error ocurred while getting Constraint Templates objects from Kubernetes API",
			Action:       "Is Gatekeeper properly installed in the cluster?",
			Description:  err.Error(),
		})
	}
	c.Echo().Logger.Debugf("got %d constraint templates. Searching constraints for each one.", len(constrainttemplates.Items))
	for _, ct := range constrainttemplates.Items {
		ctName := ct.GetName()
		constraints, err := getCustomResources(*clientset, "constraints.gatekeeper.sh", "v1beta1", ctName)
		if err != nil {
			c.Echo().Logger.Debug("got error while trying to get constraints for template: ", ctName)
		}
		response.Constraints_by_constrainttemplates[ctName] = constraints.Items
	}
	response.Constrainttemplates = constrainttemplates.Items
	return c.JSON(http.StatusOK, response)
}

// Will discover all the constraint Kinds and their objects and will return:
// - by default: a JSON with all the constraints objects sorted by 1. number of violations and 2. alphabetically.
// - when a "report" Query parameter is present in the URL: an HTML report of the violations made from a template.
func getConstraints(c echo.Context) error {
	if c.Param("context") != "" {
		c.Echo().Logger.Debug("switching to custom context ", c.Param("context"))
		err := switchKubernetesContext(c.Echo(), c.Param("context"))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorAnswer{
				ErrorMessage: fmt.Sprintf("Got an error while trying to switch to context %s", c.Param("context")),
				Action:       "Please check the context definition in the Kubeconfig file.",
				Description:  err.Error(),
			})
		}
	}
	var response []map[string]interface{}

	// constraints are a kind by themselves. The resource Kind is created dynamically by Gateeeper for each template.
	// we need to discover the available Kinds for the constraints first.
	availableConstraints, err := discoveryClient.ServerResourcesForGroupVersion("constraints.gatekeeper.sh/v1beta1")
	if err != nil {
		c.Echo().Logger.Error("error while listing constraints kinds from Kubernetes API server: ", err)
		return c.JSON(http.StatusInternalServerError, ErrorAnswer{
			ErrorMessage: "An error ocurred while trying to list the Constraints",
			Action:       "Is Gatekeeper properly installed in the target Kubernetes cluster?",
			Description:  err.Error(),
		})
	}

	for _, constraintKind := range availableConstraints.APIResources {
		// we are interested in the root resources only.
		// subresources (like <kind>/status) seem to have the categories field emtpy, so we use that to filter them out.
		if constraintKind.Categories != nil {
			constraints, err := getCustomResources(*clientset, "constraints.gatekeeper.sh", "v1beta1", constraintKind.SingularName)
			if err != nil {
				c.Echo().Logger.Error("got error while getting constraint resources: ", err)
				return c.JSON(http.StatusInternalServerError, ErrorAnswer{ErrorMessage: "An error ocurred while getting constraint objects from Kubernetes API", Action: "Is Gatekeeper properly deployed in the target cluster?", Description: err.Error()})
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
			c.Echo().Logger.Errorf("got error while trying to get object name for %s: %s", response[i], err)
		}
		iViolations, _, err := unstructured.NestedInt64(response[i], "status", "totalViolations")
		if err != nil {
			c.Echo().Logger.Errorf("got error while trying to get the total violations counts for %s: %s", iName, err)
		}
		jName, _, err := unstructured.NestedString(response[j], "metadata", "name")
		if err != nil {
			c.Echo().Logger.Errorf("got error while trying to get object name for %s: %s", response[i], err)
		}
		jViolations, _, err := unstructured.NestedInt64(response[j], "status", "totalViolations")
		if err != nil {
			c.Echo().Logger.Errorf("got error while trying to get the total violations counts for %s: %s", jName, err)
		}
		if iViolations == jViolations {
			return strings.Compare(iName, jName) < 0
		}
		return iViolations > jViolations
	})

	// We support HTML reports only for now, so we don't check the param value, just that is present.
	if c.QueryParam("report") != "" {
		var data = map[string]interface{}{
			"constraints":   response,
			"apiServerHost": config.Host,
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
func getMutations(c echo.Context) error {
	if c.Param("context") != "" {
		c.Echo().Logger.Debug("switching to custom context ", c.Param("context"))
		err := switchKubernetesContext(c.Echo(), c.Param("context"))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorAnswer{
				ErrorMessage: fmt.Sprintf("Got an error while trying to switch to context %s", c.Param("context")),
				Action:       "Please check the context definition in the Kubeconfig file.",
				Description:  err.Error(),
			})
		}
	}

	// Mutators are well-known, but we could use dynamic client like we do for the constraints
	// to discover the available kinds.
	mutators := []string{"assign", "assignmetadata", "modifyset", "assignimage"}
	var response []interface{}

	for _, mutator := range mutators {
		res := fmt.Sprintf("%s.mutations.gatekeeper.sh", mutator)
		c.Echo().Logger.Debug("Getting mutations of kind: ", res)
		mutations, err := getCustomResources(*clientset, "mutations.gatekeeper.sh", "v1", mutator)
		if err != nil {
			// We get an error when there are no mutations defined in the cluster also
			c.Echo().Logger.Errorf("got error while getting mutator '%s' resources: %s", mutator, err)
			// return c.JSON(http.StatusInternalServerError, ErrorAnswer{
			// 	ErrorMessage: "An error ocurred while getting mutation objects from Kubernetes API",
			// 	Action:       "Is Gatekeeper properly deployed in the target cluster?",
			// 	Description:  err.Error(),
			// })
		} else {
			for _, i := range mutations.Items {
				c.Echo().Logger.Debugf("adding %s to response", i)
				response = append(response, i.Object)
			}
		}
	}
	if response != nil {
		return c.JSON(http.StatusOK, response)
	} else {
		return c.JSON(http.StatusOK, []string{})
	}
}

// Returns a slice of unstructured objects with all the events generated by the 'gatekeeper-wbhook' source
// If namespace is an empty string, it returns the events from all namespaces.
func getKubernetesEvents(clientset dynamic.DynamicClient, namespace string) (*[]unstructured.Unstructured, error) {
	// FieldSeletor is very limited in the supported fields, we can't filter like this:
	// listOptions := metav1.ListOptions{
	// 	FieldSelector: "involvedObject.metadata.source.component=gatekeeper-webhook", //Filter events related to Pods
	// }
	// so we need to filter the events manually with the for loop below
	r := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}
	events, err := clientset.Resource(r).Namespace(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var filteredList []unstructured.Unstructured
	for i := range events.Items {
		source, found, err := unstructured.NestedString(events.Items[i].Object, "source", "component")
		// TODO: maybe source should be configurable?
		if found && err == nil && source == "gatekeeper-webhook" {
			filteredList = append(filteredList, events.Items[i])
		} else if err != nil {
			// TODO: not sure we should return here, but leaving the return for testing purposes
			return nil, err
		}
	}
	return &filteredList, nil
}

// Returns a JSON with a list of all the events generated by 'gatekeeper-wbhook' as unustructured objects
// TODO: I'm not sure we are getting the response from the API server in the right schema version.
//
//	See: https://v1-25.docs.kubernetes.io/docs/reference/kubernetes-api/cluster-resources/event-v1/
func getEvents(c echo.Context) error {
	if c.Param("context") != "" {
		c.Echo().Logger.Debug("switching to custom context ", c.Param("context"))
		err := switchKubernetesContext(c.Echo(), c.Param("context"))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorAnswer{
				ErrorMessage: fmt.Sprintf("Got an error while trying to switch to context %s", c.Param("context")),
				Action:       "Please check the context definition in the Kubeconfig file.",
				Description:  err.Error(),
			})
		}
	}
	events, err := getKubernetesEvents(*clientset, c.QueryParam("namespace"))

	if err != nil {
		c.Echo().Logger.Errorf("got error while getting events namespace '%s': %s", c.QueryParam("namespace"), err)
		return c.JSON(http.StatusInternalServerError, ErrorAnswer{
			ErrorMessage: "An error ocurred while getting events from Kubernetes API.",
			Description:  err.Error(),
			Action:       "Check that the Kubconfig file is correct and the Kubernetes API accessible.",
		})
	}
	return c.JSON(http.StatusOK, events)
}

// Initializes the Kubernetes client, from one or more kubeconfigs or also an in-cluster client as a backup.
// The context paramter is optional, if emtpy will use the default context form the loaded kubeconfig.
func kubeClient(e *echo.Echo, context string) (*dynamic.DynamicClient, *rest.Config, *api.Config, error) {

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{CurrentContext: context}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	e.Logger.Info("trying to load kubeconfigs from ", kubeConfig.ConfigAccess().GetLoadingPrecedence())

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		e.Logger.Fatal("got error while creating Kubernetes client config: ", err)
	}

	startingConfig, err := kubeConfig.ConfigAccess().GetStartingConfig()
	if err != nil {
		e.Logger.Fatal("got error while getting contexts information from Kubeconfig: ", err)
	}

	// create the dynamic Kubernetes client
	clientset, err := dynamic.NewForConfig(config)
	if err != nil {
		e.Logger.Fatal("got error while creating Kubernetes client: ", err)
	}

	// discoveryClient is used to discover Cosntrains Kinds
	discoveryClient, err = discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		e.Logger.Fatal("error while creating constraints discovery Kubernetes client: ", err)
	}

	return clientset, config, startingConfig, err
}

// Switches the current context in the kubeconfig and replaces the relevant
// kuberneetes client objects. If the context is not available in the kubeconfig
// will return an error and left the kubernetes client objects intact.
func switchKubernetesContext(e *echo.Echo, c string) error {
	var err error
	if c == startingConfig.CurrentContext {
		return nil
	}
	if _, ok := startingConfig.Contexts[c]; !ok {
		return fmt.Errorf("context %s does not exist in the Kubeconfig file", c)
	}
	clientset, config, startingConfig, err = kubeClient(e, c)
	if err != nil {
		e.Logger.Errorf("got error initializating the Kubernetes cilent with custom context %s: %s", c, err)
		return err
	}
	return nil
}

func main() {
	// Initilize Echo HTTP server
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Logger())
	p := prometheus.NewPrometheus("echo", nil)
	p.Use(e)

	// Setup logging
	e.Logger.SetLevel(log.INFO)
	e.Logger.SetPrefix("gpm")
	if logLevelString, ok := os.LookupEnv("GPM_LOG_LEVEL"); ok {
		logLevel, ok := logLevelFromString[strings.ToUpper(logLevelString)]
		if !ok {
			e.Logger.Warnf("the specified log level '%s' is not a valid option, defaulting to INFO.", logLevelString)
		} else {
			e.Logger.SetLevel(logLevel)
		}
	}

	// We compile the HTML templates here
	// This is used later to render templates in the routes.
	// i.e. to render the HTML report in the `/constraints/?report=html` route.
	t := &Template{
		templates: template.Must(template.ParseGlob("templates/*.html.gotpl")),
	}
	e.Renderer = t

	if os.Getenv("APP_ENV") == "development" {
		e.Logger.Warn("running in development mode, allowing CORS from other origins")
		e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: []string{"http://localhost:3000"},
			AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
		}))
	}

	var err error
	clientset, config, startingConfig, err = kubeClient(e, "")
	if err != nil {
		e.Logger.Fatalf("got an error while initializating the Kubernetes cilent: %s", err)
	}

	// Routes configuration

	e.Static("/static/", "./static-content/static")

	// Fallback route for all non-matching URLs.
	// We need to serve index.html for react routing to work. See:
	// https://create-react-app.dev/docs/deployment#serving-apps-with-client-side-routing.
	// We could avoid this by serving the frontend from another process/container instead of from the backend
	e.GET("/*", func(c echo.Context) error {
		path := c.Request().RequestURI
		staticPath := "./static-content"
		indexPath := filepath.Join(staticPath, "index.html")
		filePath := filepath.Join(staticPath, path)
		_, fileError := os.Stat(filePath)

		// try to serve the static file, if not found serve index.html
		if path != "/" && fileError == nil {
			e.Logger.Debug("found file, serving it from ", filePath)
			return c.File(filePath)
		} else {
			e.Logger.Debug("file not found, falling back to ", indexPath)
			return c.File(indexPath)
		}
	})

	e.GET("/health", getHealth)
	e.GET("/health/", getHealth)

	e.GET("/api/v1/auth", getAuth)
	e.GET("/api/v1/auth/", getAuth)

	e.GET("/api/v1/contexts", getContexts)
	e.GET("/api/v1/contexts/", getContexts)

	// Returns an object with the list of available contets and the currently selected context
	e.GET("/api/v2/contexts/", func(c echo.Context) error {
		type v2Answer struct {
			Current  string                  `json:"currentContext"`
			Contexts map[string]*api.Context `json:"contexts"`
		}

		return c.JSON(http.StatusOK, v2Answer{startingConfig.CurrentContext, startingConfig.Contexts})
	})

	e.GET("/api/v1/configs", getConfigs)
	e.GET("/api/v1/configs/", getConfigs)
	e.GET("/api/v1/configs/:context", getConfigs)
	e.GET("/api/v1/configs/:context/", getConfigs)

	e.GET("/api/v1/constrainttemplates", getConstraintTemplates)
	e.GET("/api/v1/constrainttemplates/", getConstraintTemplates)
	e.GET("/api/v1/constrainttemplates/:context", getConstraintTemplates)
	e.GET("/api/v1/constrainttemplates/:context/", getConstraintTemplates)

	e.GET("/api/v1/constraints", getConstraints)
	e.GET("/api/v1/constraints/", getConstraints)
	e.GET("/api/v1/constraints/:context", getConstraints)
	e.GET("/api/v1/constraints/:context/", getConstraints)

	e.GET("/api/v1/mutations", getMutations)
	e.GET("/api/v1/mutations/", getMutations)
	e.GET("/api/v1/mutations/:context", getMutations)
	e.GET("/api/v1/mutations/:context/", getMutations)

	e.GET("/api/v1/events", getEvents)
	e.GET("/api/v1/events/", getEvents)
	e.GET("/api/v1/events/:context", getEvents)
	e.GET("/api/v1/events/:context/", getEvents)

	// start the web server
	e.Logger.Fatal(e.Start(":8080"))
}
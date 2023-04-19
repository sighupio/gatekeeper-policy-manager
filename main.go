// Copyright (c) 2023 SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// The backend for Gatekeeper Policy Manager, a simple to use web-based UI for OPA Gatekeeper

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
	"k8s.io/client-go/util/homedir"

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
	k8sctx    string
	k8sctxs   map[string]*api.Context
	clientset *dynamic.DynamicClient
	config    *rest.Config
	// Why I can't say this is constant, I don't know.
	logLevelFromString = map[string]log.Lvl{
		"DEBUG": log.DEBUG,
		"INFO":  log.INFO,
		"WARN":  log.WARN,
		"ERROR": log.ERROR,
	}
)

// FIXME: We should have a package with the structs for each version of the (internal) API
// for example, the ErrorMessage, the ConstraintsList, and so on.
type ErrorMessage struct {
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

func buildConfigWithContextFromFlags(context string, kubeconfigPath string) (*rest.Config, error) {
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{
			CurrentContext: context,
		}).ClientConfig()
}

func getCustomResources(clientset dynamic.DynamicClient, group string, version string, resource string) (*unstructured.UnstructuredList, error) {
	r := schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
	res, err := clientset.Resource(r).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func getHealth(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func getAuth(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]bool{"auth_enabled": false})
}

func getContexts(c echo.Context) error {
	// v1 answer is formed like this:
	// [
	//     [
	//         {
	//             "context": {
	//                 "cluster": "kind-gpm",
	//                 "user": "kind-gpm"
	//             },
	//             "name": "kind-gpm"
	//         }
	//     ],
	//     {
	//         "context": {
	//             "cluster": "kind-gpm",
	//             "user": "kind-gpm"
	//         },
	//         "name": "kind-gpm"
	//     }
	// ]
	//
	// where the [0] is a list of available contexts and [1] is the current context.
	// We need to format the response to align with the python version (API v1)

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
	for kc := range k8sctxs {
		c := context{
			Cluster: k8sctxs[kc].Cluster,
			User:    k8sctxs[kc].AuthInfo,
		}
		fullContext := kubeconfigContext{
			Name:    kc,
			Context: c,
		}
		kubeconfigContexts = append(kubeconfigContexts, fullContext)
		if kc == k8sctx {
			currentKubeconfigContext = fullContext
		}
	}
	v1Answer := []interface{}{kubeconfigContexts, currentKubeconfigContext}

	return c.JSON(http.StatusOK, v1Answer)
}

func getConfigs(c echo.Context) error {
	if c.Param("context") != "" {
		c.Echo().Logger.Debug("switching to custom context ", c.Param("context"))
		err := switchKubernetesContext(c.Echo(), c.Param("context"))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorMessage{
				ErrorMessage: fmt.Sprintf("Got an error while trying to switch to context %s", c.Param("context")),
				Action:       "Please check the context definition in the Kubeconfig file.",
				Description:  err.Error(),
			})
		}
	}
	configResources, err := getCustomResources(*clientset, "config.gatekeeper.sh", "v1alpha1", "configs")
	if err != nil {
		c.Echo().Logger.Debug("got error while getting config resources: ", err)
		return c.JSON(http.StatusInternalServerError, ErrorMessage{
			ErrorMessage: "An error ocurred while getting config objects from Kubernetes API.",
			Description:  err.Error(),
			Action:       "Check that the Kubconfig file is correct and the Kubernetes API accessible."})
	}
	return c.JSON(http.StatusOK, configResources.Items)
}

func getConstraintTemplates(c echo.Context) error {
	if c.Param("context") != "" {
		c.Echo().Logger.Debug("switching to custom context ", c.Param("context"))
		err := switchKubernetesContext(c.Echo(), c.Param("context"))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorMessage{
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
		c.Echo().Logger.Error("Got error while getting constraint templates resources: ", err)
		return c.JSON(http.StatusInternalServerError, ErrorMessage{
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
		// e.Logger.Debugf("mapping constraint '%s' to '%s' template ", constraints.Items, ctName)
		response.Constraints_by_constrainttemplates[ctName] = constraints.Items
	}
	// FIXME: check for 404 errors when the resoruces does not exists (e.g. CRD has not been applied yet).
	response.Constrainttemplates = constrainttemplates.Items
	return c.JSON(http.StatusOK, response)
}

func getConstraints(c echo.Context) error {
	if c.Param("context") != "" {
		c.Echo().Logger.Debug("switching to custom context ", c.Param("context"))
		err := switchKubernetesContext(c.Echo(), c.Param("context"))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorMessage{
				ErrorMessage: fmt.Sprintf("Got an error while trying to switch to context %s", c.Param("context")),
				Action:       "Please check the context definition in the Kubeconfig file.",
				Description:  err.Error(),
			})
		}
	}
	var response []map[string]interface{}

	// constraints are a kind by themselves. The resource Kind is created dynamically by Gateeeper for each template.
	// we need to discover the available Kinds for the constraints first.
	dc, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		c.Echo().Logger.Error("Error while creating constraints discovery client: ", err)
		return c.JSON(http.StatusInternalServerError, ErrorMessage{
			ErrorMessage: "An error ocurred while creating Constraints Discovery Client",
			Action:       "Is Gatekeeper properly installed in the cluster?",
			Description:  err.Error(),
		})
	}

	availableConstraints, err := dc.ServerResourcesForGroupVersion("constraints.gatekeeper.sh/v1beta1")
	if err != nil {
		c.Echo().Logger.Error("Error while listing constraints kinds from Kubernetes API server: ", err)
		// If there are no constraints deployed we get an error from the API server.
		// we return an emtpy list instead
		// return c.JSON(http.StatusOK, []string{})
		return c.JSON(http.StatusInternalServerError, ErrorMessage{
			ErrorMessage: "An error ocurred while trying to list the Constraints",
			Action:       "Is Gatekeeper properly installed in the target Kubernetes cluster?",
			Description:  err.Error(),
		})
	}

	for _, constraintKind := range availableConstraints.APIResources {
		// we are interested in the root resources only.
		// subresources (like <kind>/status) seem to have categories emtpy, so we can check for that to skip them.
		if constraintKind.Categories != nil {
			constraints, err := getCustomResources(*clientset, "constraints.gatekeeper.sh", "v1beta1", constraintKind.SingularName)
			if err != nil {
				c.Echo().Logger.Error("Got error while getting constraint resources: ", err)
				return c.JSON(http.StatusInternalServerError, ErrorMessage{ErrorMessage: "An error ocurred while getting constraint objects from Kubernetes API", Action: "Is Gatekeeper properly deployed in the target cluster?", Description: err.Error()})
			}
			// c.Echo().Logger.Debugf("found %d constraints for kind %s", len(constraints.Items), constraintKind.SingularName)
			for _, i := range constraints.Items {
				response = append(response, i.Object)
			}
		}
	}

	// we sort the constraints by 1. totalViolations and 2. by name
	sort.Slice(response, func(i, j int) bool {
		iName, _, err := unstructured.NestedString(response[i], "metadata", "name")
		if err != nil {
			c.Echo().Logger.Errorf("Got error while trying to get object name for %s: %s", response[i], err)
		}
		iViolations, _, err := unstructured.NestedInt64(response[i], "status", "totalViolations")
		if err != nil {
			c.Echo().Logger.Errorf("Got error while trying to get the total violations counts for %s: %s", iName, err)
		}
		jName, _, err := unstructured.NestedString(response[j], "metadata", "name")
		if err != nil {
			c.Echo().Logger.Errorf("Got error while trying to get object name for %s: %s", response[i], err)
		}
		jViolations, _, err := unstructured.NestedInt64(response[j], "status", "totalViolations")
		if err != nil {
			c.Echo().Logger.Errorf("Got error while trying to get the total violations counts for %s: %s", jName, err)
		}
		if iViolations == jViolations {
			return strings.Compare(iName, jName) < 0
		}
		return iViolations > jViolations
	})

	// we support HTML reports only for now, so we don't the param value
	if c.QueryParam("report") != "" {
		var data = map[string]interface{}{
			"constraints": response,
			"timestamp":   time.Now().Format(time.ANSIC),
		}

		return c.Render(http.StatusOK, "report", data)
	}

	// v1 API compatibility:
	// we need to return an empty list instead of null when there are no objects, otherwise the frontend breaks
	if len(response) == 0 {
		return c.JSON(http.StatusOK, []string{})
	}
	return c.JSON(http.StatusOK, response)
}

// getKubernetesEvents return a slice of unstructured objects with all the events generated by 'gatekeeper-wbhook'
// if namespace is empty, returns events in all namespaces.
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
			// TOOD: not sure we should return here, but leaving the return for testing purposes
			return nil, err
		}
	}
	return &filteredList, nil
}

// getEvents returns a JSON with a list with all the events generated by 'gatekeeper-wbhook' as unustructured objects
func getEvents(c echo.Context) error {
	if c.Param("context") != "" {
		c.Echo().Logger.Debug("switching to custom context ", c.Param("context"))
		err := switchKubernetesContext(c.Echo(), c.Param("context"))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorMessage{
				ErrorMessage: fmt.Sprintf("Got an error while trying to switch to context %s", c.Param("context")),
				Action:       "Please check the context definition in the Kubeconfig file.",
				Description:  err.Error(),
			})
		}
	}
	events, err := getKubernetesEvents(*clientset, c.QueryParam("namespace"))
	// c.Echo().Logger.Debug(events)

	if err != nil {
		c.Echo().Logger.Debugf("got error while getting events namespace '%s': %s", c.QueryParam("namespace"), err)
		return c.JSON(http.StatusInternalServerError, ErrorMessage{ErrorMessage: "An error ocurred while getting events from Kubernetes API.", Description: err.Error(), Action: "Check that the Kubconfig file is correct and the Kubernetes API accessible."})
	}
	return c.JSON(http.StatusOK, events)
}

func kubeClient(e *echo.Echo, context string) (*dynamic.DynamicClient, *rest.Config, error) {
	kubeconfig, ok := os.LookupEnv("KUBECONFIG")
	if !ok {
		e.Logger.Debug("KUBECONFIG environment variable is not set, falling back to $HOME/.kube/config")
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
			if _, err := os.Stat(kubeconfig); os.IsNotExist(err) {
				e.Logger.Warn("Kubeconfig file does not exists in path: ", kubeconfig)
				kubeconfig = ""
			}
		}
	} else {
		e.Logger.Infof("Using KUBECONFIG from path: %s", kubeconfig)
	}

	// FIXME: This needs to be refactored 👇🏻
	// we use the config2 to get all the context information, beacuse
	// I did not find a way to get it from the config that we use for the
	// dynamic client.
	e.Logger.Debug("HERE BE DRAGONS --- config2 for detecting contexts")
	configaccess := clientcmd.NewDefaultPathOptions()
	config2, err := configaccess.GetStartingConfig()
	if err != nil {
		e.Logger.Error("Got error while creating config2:", err)
	}

	k8sctx = config2.CurrentContext
	k8sctxs = config2.Contexts
	// To have a list of strings wiht the available contexts insteda
	// for k := range config2.Contexts {
	// 	k8sctxs = append(k8sctxs, k)
	// }
	e.Logger.Debugf("current context is: %s. Available contexts are: %s", k8sctx, k8sctxs)
	// end refactor 👆🏻

	// Use when context is different than default
	config, err := buildConfigWithContextFromFlags(context, kubeconfig)
	if err != nil {
		e.Logger.Error("Attempt to configure the Kubernetes client failed. Nothing else to do, giving up.")
		e.Logger.Fatal(err)
	}

	// create the dynamic Kubernetes client
	clientset, err := dynamic.NewForConfig(config)
	if err != nil {
		e.Logger.Fatal("got error while creating Kubernetes client: ", err.Error())
	}
	return clientset, config, err
}

func switchKubernetesContext(e *echo.Echo, c string) error {
	var err error
	if c == k8sctx {
		return nil
	}
	if _, ok := k8sctxs[c]; !ok {
		return fmt.Errorf("context %s does not exist in the Kubeconfig file", c)
	}
	clientset, config, err = kubeClient(e, c)
	if err != nil {
		e.Logger.Errorf("Got error initializating the Kubernetes cilent with custom context %s: %s", c, err)
		return err
	}
	k8sctx = c
	return nil
}

func main() {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Logger())
	p := prometheus.NewPrometheus("echo", nil)
	p.Use(e)

	e.Logger.SetLevel(log.DEBUG) // FIXME: DEFAULT TO INFO INSTEAD
	// e.Logger.SetLevel(log.INFO)
	e.Logger.SetPrefix("gpm")
	if logLevelString, ok := os.LookupEnv("GPM_LOG_LEVEL"); ok {
		logLevel, ok := logLevelFromString[strings.ToUpper(logLevelString)]
		if !ok {
			e.Logger.Warnf("the specified log level '%s' is not a valid option, defaulting to INFO.", logLevelString)
		} else {
			e.Logger.SetLevel(logLevel)
		}
	}

	// This is used later to render templates in the routes.
	// i.e. to render the HTML report in the `/constraints/?report=html` route.
	t := &Template{
		templates: template.Must(template.ParseGlob("templates/*.html.gotpl")),
	}
	e.Renderer = t

	if os.Getenv("APP_ENV") == "development" {
		e.Logger.Warn("Running in development mode, allowing CORS from other origins")
		e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: []string{"http://localhost:3000"},
			AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
		}))
	}

	var err error
	// we start with the default context from the kubeconfig file: ""
	clientset, config, err = kubeClient(e, "")
	if err != nil {
		e.Logger.Fatalf("Got an error while initializating the Kubernetes cilent: %s", err)
	}

	// Routes

	e.Static("/static/", "./static-content/static")

	// We need to serve the static files using this approach because of the React routes.
	// TODO: improve this approach. Ideally, having /static/ should be enough.
	e.GET("/*", func(c echo.Context) error {
		path := c.Request().RequestURI
		static_path := "./static-content"
		index_path := filepath.Join(static_path, "index.html")
		file_path := filepath.Join(static_path, path)
		path_err, _ := os.Stat(file_path)

		if path != "/" && path_err != nil {
			e.Logger.Debug("found file, serving it from ", file_path)
			return c.File(file_path)
		} else {
			e.Logger.Debug("file not found, falling back to ", index_path)
			return c.File(index_path)
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
		// TODO: maybe Contexts could be a []string instead of the full context information
		type v2Answer struct {
			Current  string                  `json:"currentContext"`
			Contexts map[string]*api.Context `json:"contexts"`
		}

		return c.JSON(http.StatusOK, v2Answer{k8sctx, k8sctxs})
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

	e.GET("/api/v1/events", getEvents)
	e.GET("/api/v1/events/", getEvents)
	e.GET("/api/v1/events/:context", getEvents)
	e.GET("/api/v1/events/:context/", getEvents)

	// start the web server
	e.Logger.Fatal(e.Start(":8080"))
}

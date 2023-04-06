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
	"k8s.io/client-go/tools/clientcmd"
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
	logLevelFromString = map[string]log.Lvl{
		"DEBUG": log.DEBUG,
		"INFO":  log.INFO,
		"WARN":  log.WARN,
		"ERROR": log.ERROR,
	}
)

type ErrorMessage struct {
	Error       string `json:"error"`
	Action      string `json:"action"`
	Description string `json:"description"`
}
type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func getCustomResources(clientset dynamic.DynamicClient, group string, version string, resource string) (*unstructured.UnstructuredList, error) {
	r := schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
	res, err := clientset.Resource(r).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		// fmt.Printf("error getting group '%s', version: '%s', resource: '%s'\n", group, version, resource)
		return nil, err
	}
	// fmt.Printf("got %d results for group '%s', version: '%s', resource: '%s'\n", len(res.Items), group, version, resource)
	return res, nil
}

func main() {
	e := echo.New()
	p := prometheus.NewPrometheus("echo", nil)
	p.Use(e)
	e.HideBanner = true
	e.Use(middleware.Logger())

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

	kubeconfig, ok := os.LookupEnv("KUBECONFIG")
	if !ok {
		e.Logger.Debug("KUBECONFIG environment variable is not set, falling back to $HOME/.kube/config")
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
			if _, err := os.Stat(kubeconfig); os.IsNotExist(err) {
				e.Logger.Warn("kubeconfig file does not exists in path: ", kubeconfig)
				kubeconfig = ""
			}
		}
	} else {
		e.Logger.Infof("using KUBECONFIG from path: %s", kubeconfig)
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		e.Logger.Error("attempt to configure the Kubernetes client failed. Nothing else to do, giving up.")
		e.Logger.Fatal(err)
	}

	// FIXME: This needs to be refactored, we can't change the current context because we still use the other client
	e.Logger.Debug("HERE BE DRAGONS --- config2 for detecting contexts")
	configaccess := clientcmd.NewDefaultPathOptions()
	config2, err := configaccess.GetStartingConfig()
	if err != nil {
		e.Logger.Error("got error while creating config2:", err)
	}

	k8sctx := config2.CurrentContext
	var k8sctxs []string
	for k := range config2.Contexts {
		k8sctxs = append(k8sctxs, k)
	}
	e.Logger.Debugf("current context is: %s. Available contexts are: %s", k8sctx, k8sctxs)
	// end refactor

	// create the dynamic Kubernetes client
	clientset, err := dynamic.NewForConfig(config)
	if err != nil {
		e.Logger.Fatal("got error while creating Kubernetes client: ", err.Error())
	}

	// This is used later to render templates in the routes.
	// i.e. to render the HTML report in the `/constraints/?report=html` route.
	t := &Template{
		templates: template.Must(template.ParseGlob("templates/*.html")),
	}
	e.Renderer = t

	// routes start here
	// FIXME: map routes to functions instead of inline

	e.Static("/static/", "./static-content/static")

	e.GET("/*", func(c echo.Context) error {
		// We need to serve the static files using this approach because of the React routes.
		// TODO: improve this approach. Ideally, having /static/ should be enough.
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

	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, "{\"status\": \"ok\"}")
	})

	e.GET("/api/v1/auth/", func(c echo.Context) error {
		type Auth struct {
			AuthEnabled bool `json:"auth_enabled"`
		}
		authResponse := &Auth{
			AuthEnabled: false,
		}
		// FIXME: actually implement auth :o)
		return c.JSON(http.StatusOK, authResponse)
	})

	e.GET("/api/v1/contexts/", func(c echo.Context) error {
		type kubeconfigContexts struct {
			Contexts []string `json:"contexts"`
		}
		k8sc := &kubeconfigContexts{
			Contexts: k8sctxs,
		}
		// FIXME: actually implement changing context :o)
		return c.JSON(http.StatusOK, k8sc)
	})

	e.GET("/api/v1/configs/", func(c echo.Context) error {
		configResources, err := getCustomResources(*clientset, "config.gatekeeper.sh", "v1alpha1", "configs")
		if err != nil {
			e.Logger.Debug("got error while getting config resources: ", err)
			// FIXME: use the right error format
			return c.JSON(http.StatusInternalServerError, fmt.Sprint("an error ocurred while getting config objects from Kubernetes API: ", err))
		}
		return c.JSON(http.StatusOK, configResources.Items)
	})

	e.GET("/api/v1/constrainttemplates/", func(c echo.Context) error {
		var response struct {
			Constrainttemplates                []unstructured.Unstructured            `json:"constrainttemplates"`
			Constraints_by_constrainttemplates map[string][]unstructured.Unstructured `json:"constraints_by_constrainttemplates"`
		}
		// we need to initialize the variable otherwise assigning to a map memeber panics
		response.Constraints_by_constrainttemplates = make(map[string][]unstructured.Unstructured)

		constrainttemplates, err := getCustomResources(*clientset, "templates.gatekeeper.sh", "v1beta1", "constrainttemplates")
		if err != nil {
			e.Logger.Debug("got error while getting constraint templates resources: ", err)
			return c.JSON(http.StatusInternalServerError, fmt.Sprint("an error ocurred while getting constraint templates objects from Kubernetes API: ", err))
		}
		e.Logger.Debugf("got %d constraint templates. Asking constraints for each one.", len(constrainttemplates.Items))
		e.Logger.Debug("getting Constraints for each Constraint Templates")
		for _, ct := range constrainttemplates.Items {
			ctName := ct.GetName()
			constraints, err := getCustomResources(*clientset, "constraints.gatekeeper.sh", "v1beta1", ctName)
			if err != nil {
				e.Logger.Debug("got error while trying to get constraints for template: ", ctName)
			}
			// e.Logger.Debugf("mapping constraint '%s' to '%s' template ", constraints.Items, ctName)
			response.Constraints_by_constrainttemplates[ctName] = constraints.Items
		}
		// FIXME: check for 404 errors when the resoruces does not exists (e.g. CRD has not been applied yet).
		response.Constrainttemplates = constrainttemplates.Items
		return c.JSON(http.StatusOK, response)
	})

	e.GET("/api/v1/constraints/", func(c echo.Context) error {
		var response []map[string]interface{}

		// constraints are a kind by themselves. The resource Kind is created dynamically by Gateeeper for each template.
		// we need to discover the available Kinds for the constraints first.
		dc, err := discovery.NewDiscoveryClientForConfig(config)
		if err != nil {
			e.Logger.Error("error while creating constraints discovery client: ", err)
			msg := fmt.Sprintf("{\"error\": \"an error ocurred while creating constraints discovery client\",  \"action\": \"Is Gatekeeper properly installed in the cluster?\", \"description\": %s}", err)
			return c.JSON(http.StatusInternalServerError, msg)
		}

		availableConstraints, err := dc.ServerResourcesForGroupVersion("constraints.gatekeeper.sh/v1beta1")
		if err != nil {
			e.Logger.Warn("error while listing constraints kinds from Kubernetes API server: ", err)
			// msg := fmt.Sprintf("{\"error\": \"an error ocurred while listing the Constraints\",  \"action\": \"Is Gatekeeper properly installed in the cluster?\", \"description\": %s}", err)
			return c.JSON(http.StatusOK, []string{})
		}

		// useful to debug the discovered objects:
		// return c.JSON(http.StatusOK, availableConstraints)

		for _, constraintKind := range availableConstraints.APIResources {
			// we are interested in the root resources only.
			// subresources (like <kind>/status) seem to have categories emtpy, so we can check for that to skip them.
			if constraintKind.Categories != nil {
				constraints, err := getCustomResources(*clientset, "constraints.gatekeeper.sh", "v1beta1", constraintKind.SingularName)
				if err != nil {
					e.Logger.Error("got error while getting constraint resources: ", err)
					return c.JSON(http.StatusInternalServerError, fmt.Sprint("an error ocurred while getting constraint objects from Kubernetes API: ", err))
				}
				// e.Logger.Debugf("found %d constraints for kind %s", len(constraints.Items), constraintKind.SingularName)
				for _, i := range constraints.Items {
					response = append(response, i.Object)
				}
			}
		}

		// we sort the constraints by 1. totalViolations and 2. by name
		sort.Slice(response, func(i, j int) bool {
			// FIXME: this could fail when Gatekeeper hasn't created the status field yet
			iViolations := response[i]["status"].(map[string]interface{})["totalViolations"].(int64)
			jViolations := response[j]["status"].(map[string]interface{})["totalViolations"].(int64)
			iName := response[i]["metadata"].(map[string]interface{})["name"].(string)
			jName := response[j]["metadata"].(map[string]interface{})["name"].(string)
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

		return c.JSON(http.StatusOK, response)
	})

	// start the web server
	e.Logger.Fatal(e.Start(":8080"))
}

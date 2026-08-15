// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Server-rendered UI (proof of concept). Templates and static assets are embedded, so this path
// does not depend on the working directory the way the React static-content serving does. It is
// additive: it lives under /ssr and touches none of the existing routes or handlers.
package main

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"sort"

	"github.com/labstack/echo/v4"
	"golang.org/x/exp/slog"
	"sigs.k8s.io/yaml"
)

//go:embed templates/ssr/*.html.gotpl
var ssrTemplateFS embed.FS

//go:embed static/ssr
var ssrStaticFS embed.FS

// The views wired to a server-rendered page, keyed by their route/nav name. Each entry is a
// template file that fills the "content" block of layout.html.gotpl. Add a view by adding a file
// here and a handler below; the layout, nav and asset wiring are shared.
var ssrPages = map[string]string{
	"configurations": "templates/ssr/configurations.html.gotpl",
	"mutations":      "templates/ssr/mutations.html.gotpl",
}

type ssrRenderer struct {
	pages map[string]*template.Template
}

// Parses one template set per page, each a clone of the shared layout plus that page's file. A
// single flat ParseGlob cannot do this: every page defines the "content" block, so they would
// collide in one set. Cloning keeps each page's "content" isolated while sharing the layout.
func newSSRRenderer() *ssrRenderer {
	funcs := template.FuncMap{
		// browserPath puts the reverse-proxy base path back on links; see basepath.go.
		"browserPath": browserPath,
		"toYAML":      toYAML,
	}
	layout := template.Must(
		template.New("layout").Funcs(funcs).ParseFS(ssrTemplateFS, "templates/ssr/layout.html.gotpl"),
	)
	r := &ssrRenderer{pages: make(map[string]*template.Template, len(ssrPages))}
	for name, file := range ssrPages {
		clone := template.Must(layout.Clone())
		r.pages[name] = template.Must(clone.ParseFS(ssrTemplateFS, file))
	}
	return r
}

func (r *ssrRenderer) render(c echo.Context, page string, data any) error {
	t, ok := r.pages[page]
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "unknown SSR page: "+page)
	}
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)
	return t.ExecuteTemplate(c.Response().Writer, "layout", data)
}

// toYAML renders a spec (a map[string]interface{} from an unstructured object) as readable YAML.
// sigs.k8s.io/yaml marshals through JSON, which is exactly the shape the handlers already hold.
func toYAML(v any) string {
	if v == nil {
		return ""
	}
	out, err := yaml.Marshal(v)
	if err != nil {
		return "could not render YAML: " + err.Error()
	}
	return string(out)
}

// --- layout view models ---------------------------------------------------------------------

type navLink struct {
	Name   string
	Href   string
	Active bool
}

type ctxOption struct {
	Name     string
	URL      string
	Selected bool
}

type ssrLayout struct {
	Title       string
	Version     string
	AssetBase   string // browserPath("/ssr/static"), where the CSS and Alpine live
	Nav         []navLink
	Contexts    []ctxOption
	HasContexts bool
	AuthEnabled bool
	LogoutURL   string
}

// The top-nav destinations. Only Configurations is server-rendered so far; the rest still point at
// the existing React SPA so the shell stays navigable during the migration. As each view is ported
// its ssr flag flips and its path moves under /ssr.
var ssrNavRoutes = []struct {
	Key  string
	Name string
	Path string
	SSR  bool
}{
	{"home", "Home", "/", false},
	{"constrainttemplates", "Constraint Templates", "/constrainttemplates", false},
	{"constraints", "Constraints", "/constraints", false},
	{"mutations", "Mutations", "/ssr/mutations", true},
	{"events", "Events", "/events", false},
	{"configurations", "Configurations", "/ssr/configurations", true},
}

// Builds the data every SSR page shares: nav with the active item highlighted, the context switcher
// options, and the footer. switchBase is the current view's path without a context, so the switcher
// can send the user to the same view under a different context.
func (s *server) ssrLayoutData(c echo.Context, active, switchBase, title string) ssrLayout {
	contexts, current := s.k8s.contexts()
	selected := c.Param("context")
	if selected == "" {
		selected = current
	}

	names := make([]string, 0, len(contexts))
	for n := range contexts {
		names = append(names, n)
	}
	sort.Strings(names)

	nav := make([]navLink, 0, len(ssrNavRoutes))
	for _, r := range ssrNavRoutes {
		href := browserPath(r.Path)
		if r.Path != "/" {
			href = browserPath(r.Path + "/" + url.PathEscape(selected))
		}
		nav = append(nav, navLink{Name: r.Name, Href: href, Active: r.Key == active})
	}

	options := make([]ctxOption, 0, len(names))
	for _, n := range names {
		options = append(options, ctxOption{
			Name:     n,
			URL:      browserPath(switchBase + "/" + url.PathEscape(n)),
			Selected: n == selected,
		})
	}

	return ssrLayout{
		Title:       title,
		Version:     appVersion,
		AssetBase:   browserPath("/ssr/static"),
		Nav:         nav,
		Contexts:    options,
		HasContexts: len(options) > 0,
		AuthEnabled: authEnabled(),
		LogoutURL:   browserPath("/logout"),
	}
}

// --- handlers -------------------------------------------------------------------------------

// getSSRConfigurations renders the Configurations view. It reads the very same Gatekeeper Config
// objects as the JSON handler getConfigs, then hands them to the template instead of to c.JSON.
func (s *server) getSSRConfigurations(c echo.Context) error {
	layout := s.ssrLayoutData(c, "configurations", "/ssr/configurations", "Configurations")

	data := map[string]any{"Layout": layout}

	clients, err := s.clientsFor(c)
	if err != nil {
		slog.Error("SSR configurations: resolving context failed", "error", err)
		data["Error"] = "GPM could not switch to the requested Kubernetes context. Make sure the kubeconfig defines it correctly."
		return s.ssr.render(c, "configurations", data)
	}

	configResources, err := getCustomResources(c.Request().Context(), *clients.dynamic,
		"config.gatekeeper.sh", "v1alpha1", "configs")
	if err != nil {
		slog.Error("SSR configurations: getting config resources failed", "error", err)
		data["Error"] = "GPM could not get the configuration objects from the Kubernetes API. Make sure the API is reachable."
		return s.ssr.render(c, "configurations", data)
	}

	items := make([]map[string]any, 0, len(configResources.Items))
	for i := range configResources.Items {
		items = append(items, configResources.Items[i].Object)
	}
	data["Configs"] = items
	return s.ssr.render(c, "configurations", data)
}

// getSSRMutations renders the Mutations view. It reads the very same Gatekeeper mutator objects as
// the JSON handler getMutations (assign, assignmetadata, modifyset, assignimage under
// mutations.gatekeeper.sh/v1), then hands them to the template instead of to c.JSON.
func (s *server) getSSRMutations(c echo.Context) error {
	layout := s.ssrLayoutData(c, "mutations", "/ssr/mutations", "Mutations")

	data := map[string]any{"Layout": layout}

	clients, err := s.clientsFor(c)
	if err != nil {
		slog.Error("SSR mutations: resolving context failed", "error", err)
		data["Error"] = "GPM could not switch to the requested Kubernetes context. Make sure the kubeconfig defines it correctly."
		return s.ssr.render(c, "mutations", data)
	}

	// Mutators are well-known; the same list the JSON handler uses. A missing kind just means no
	// such mutations are defined, so we log and continue rather than failing the whole page.
	mutators := []string{"assign", "assignmetadata", "modifyset", "assignimage"}
	items := make([]map[string]any, 0)
	for _, mutator := range mutators {
		mutations, err := getCustomResources(c.Request().Context(), *clients.dynamic,
			"mutations.gatekeeper.sh", "v1", mutator)
		if err != nil {
			slog.Error("SSR mutations: getting mutator resources failed", "mutator", mutator, "error", err)
			continue
		}
		for i := range mutations.Items {
			items = append(items, mutations.Items[i].Object)
		}
	}
	data["Mutations"] = items
	return s.ssr.render(c, "mutations", data)
}

// registerSSR wires the server-rendered POC: embedded static assets and the ported views. Called
// from main after the server is built. Echo matches these specific paths ahead of the "/*"
// SPA-fallback, so nothing here shadows the existing app.
func registerSSR(e *echo.Echo, s *server) {
	assets, err := fs.Sub(ssrStaticFS, "static/ssr")
	if err != nil {
		slog.Error("SSR static assets could not be mounted", "error", err)
		return
	}
	e.GET("/ssr/static/*", echo.WrapHandler(
		http.StripPrefix("/ssr/static/", http.FileServer(http.FS(assets)))))

	e.GET("/ssr/configurations", s.getSSRConfigurations)
	e.GET("/ssr/configurations/:context", s.getSSRConfigurations)

	e.GET("/ssr/mutations", s.getSSRMutations)
	e.GET("/ssr/mutations/:context", s.getSSRMutations)
}

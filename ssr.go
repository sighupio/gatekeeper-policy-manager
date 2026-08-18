// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Server-rendered UI. Templates and static assets are embedded, so this path does not depend on
// the working directory. Every view is rendered here on the server; there is no client-side SPA.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
	"golang.org/x/exp/slog"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	"home":                "templates/ssr/home.html.gotpl",
	"configurations":      "templates/ssr/configurations.html.gotpl",
	"mutations":           "templates/ssr/mutations.html.gotpl",
	"constrainttemplates": "templates/ssr/constrainttemplates.html.gotpl",
	"constraints":         "templates/ssr/constraints.html.gotpl",
	"events":              "templates/ssr/events.html.gotpl",
	"error":               "templates/ssr/error.html.gotpl",
	"notfound":            "templates/ssr/notfound.html.gotpl",
	"loggedout":           "templates/ssr/loggedout.html.gotpl",
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
		"toJSON":      toJSON,
		"highlight":   highlight,
		"linkify":     linkify,
		"annotation":  annotation,
		// constraintAnchor keeps the card id, the sidebar link and every cross-link in step.
		"constraintAnchor": constraintAnchor,
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
	return r.renderStatus(c, http.StatusOK, page, data)
}

// renderStatus is render with an explicit status code, for the error and 404 pages.
func (r *ssrRenderer) renderStatus(c echo.Context, status int, page string, data any) error {
	t, ok := r.pages[page]
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "unknown SSR page: "+page)
	}
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	c.Response().WriteHeader(status)
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

// chromaFormatter emits highlighted tokens as <span class="…"> in chroma's classes mode and does
// NOT wrap them in its own <pre>, so the output slots inside the existing <pre class="code chroma">.
// The classes are styled by static/ssr/chroma.css.
var chromaFormatter = chromahtml.New(chromahtml.WithClasses(true), chromahtml.WithLineNumbers(true))

// highlight renders code with server-side syntax highlighting. lang picks the chroma lexer (e.g.
// "yaml", "json", "rego"); an unknown language falls back to the plain lexer so the code still
// renders, just unhighlighted. Returning template.HTML is safe: chroma HTML-escapes the source
// (<, >, & become entities) before wrapping tokens in spans, so no source can inject markup.
func highlight(code, lang string) template.HTML {
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return template.HTML(template.HTMLEscapeString(code))
	}
	var buf bytes.Buffer
	// styles.Fallback is required by the formatter API but unused in classes mode (colors come from
	// the stylesheet, not inline styles).
	if err := chromaFormatter.Format(&buf, styles.Fallback, iterator); err != nil {
		return template.HTML(template.HTMLEscapeString(code))
	}
	return template.HTML(buf.String())
}

// urlRE matches bare http(s) URLs in plain text so linkify can turn them into anchors.
var urlRE = regexp.MustCompile(`https?://[^\s<>"]+`)

// linkify HTML-escapes a description and renders any http(s) URL in it as a link, as the React view
// did. It escapes every segment (text and href) itself, so returning template.HTML is safe: no part
// of the input reaches the page unescaped. Trailing sentence punctuation is kept out of the link.
func linkify(s string) template.HTML {
	var b strings.Builder
	last := 0
	for _, m := range urlRE.FindAllStringIndex(s, -1) {
		b.WriteString(template.HTMLEscapeString(s[last:m[0]]))
		u := s[m[0]:m[1]]
		trimmed := strings.TrimRight(u, ".,;:!?)]}'\"")
		tail := u[len(trimmed):]
		esc := template.HTMLEscapeString(trimmed)
		b.WriteString(`<a href="` + esc + `" target="_blank" rel="noopener noreferrer">` + esc + `</a>`)
		b.WriteString(template.HTMLEscapeString(tail))
		last = m[1]
	}
	b.WriteString(template.HTMLEscapeString(s[last:]))
	return template.HTML(b.String())
}

// annotation reads one metadata annotation from a raw API object. The Mutations and Configurations
// views render the objects directly rather than through a view model, and walking
// .metadata.annotations.description there aborts the template on any object that carries no
// annotations at all -- which is most of them.
func annotation(obj any, name string) string {
	m, ok := obj.(map[string]any)
	if !ok {
		return ""
	}
	v, _, _ := unstructured.NestedString(m, "metadata", "annotations", name)
	return v
}

// The report renders the raw Constraint objects, so it has to read them as defensively as
// ssrConstraintModel does. A Constraint applied since the last audit carries no
// status.totalViolations at all, and `gt` against that missing value aborts the whole template: one
// un-audited Constraint turned the downloaded report into a 500 error page. 1.x printed "unknown"
// for it instead, and these three helpers let the report do the same.
func reportViolationsKnown(c any) bool {
	obj, ok := c.(map[string]any)
	if !ok {
		return false
	}
	_, found, err := unstructured.NestedInt64(obj, "status", "totalViolations")
	return found && err == nil
}

// reportTotalViolations is the audited count, and 0 for a Constraint that has not been audited.
func reportTotalViolations(c any) int64 {
	obj, ok := c.(map[string]any)
	if !ok {
		return 0
	}
	n, found, err := unstructured.NestedInt64(obj, "status", "totalViolations")
	if !found || err != nil {
		return 0
	}
	return n
}

// reportEnforcement is the violation's action, collapsed the way enforcementMode collapses the
// spec. Today's CRD carries `default: deny`, so the API server always materialises the field and
// every violation record has it -- verified against a live cluster. This keeps the report honest
// against an older Gatekeeper whose CRD has no default, where the UI would say DENY (it normalises)
// and the report would print nothing.
func reportEnforcement(v any) string {
	obj, ok := v.(map[string]any)
	if !ok {
		return enforcementMode("")
	}
	action, _, _ := unstructured.NestedString(obj, "enforcementAction")
	return enforcementMode(action)
}

// reportViolations is the violation list, and empty when there is none. Gatekeeper can report a
// count with no list behind it, and `len` on that missing value is another way to abort the render.
func reportViolations(c any) []any {
	obj, ok := c.(map[string]any)
	if !ok {
		return nil
	}
	vs, found, err := unstructured.NestedSlice(obj, "status", "violations")
	if !found || err != nil {
		return nil
	}
	return vs
}

// toJSON marshals a value for a <script type="application/json"> data island. encoding/json escapes
// <, > and & by default, so a "</script>" inside a violation message cannot break out of the tag;
// template.JS then lets html/template emit the result verbatim instead of re-escaping it.
func toJSON(v any) template.JS {
	out, err := json.Marshal(v)
	if err != nil {
		return template.JS("[]")
	}
	return template.JS(out)
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
	AssetBase   string // browserPath("/static"), where the CSS and Alpine live
	Nav         []navLink
	Contexts    []ctxOption
	HasContexts bool
	AuthEnabled bool
	LogoutURL   string
}

// The top-nav destinations, at their real paths. Home has no nav entry -- the logo links back to
// it.
var ssrNavRoutes = []struct {
	Key  string
	Name string
	Path string
}{
	{"constrainttemplates", "Constraint Templates", "/constrainttemplates"},
	{"constraints", "Constraints", "/constraints"},
	{"mutations", "Mutations", "/mutations"},
	{"events", "Events", "/events"},
	{"configurations", "Configurations", "/configurations"},
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
		AssetBase:   browserPath("/static"),
		Nav:         nav,
		Contexts:    options,
		HasContexts: len(options) > 0,
		AuthEnabled: authEnabled(),
		LogoutURL:   browserPath("/logout"),
	}
}

// --- handlers -------------------------------------------------------------------------------

// getConfigurations renders the Configurations view. It reads the very same Gatekeeper Config
// objects as the JSON handler getConfigs, then hands them to the template instead of to c.JSON.
func (s *server) getConfigurations(c echo.Context) error {
	layout := s.ssrLayoutData(c, "configurations", "/configurations", "Configurations")

	data := map[string]any{"Layout": layout}

	clients, err := s.clientsFor(c)
	if err != nil {
		slog.Error("SSR configurations: resolving context failed", "error", err)
		setViewError(data, "GPM could not switch to the requested Kubernetes context. Make sure the kubeconfig defines it correctly.", err)
		return s.ssr.render(c, "configurations", data)
	}

	configResources, err := getCustomResources(c.Request().Context(), *clients.dynamic,
		"config.gatekeeper.sh", "v1alpha1", "configs")
	if err != nil {
		slog.Error("SSR configurations: getting config resources failed", "error", err)
		setViewError(data, "GPM could not get the configuration objects from the Kubernetes API. Make sure the API is reachable.", err)
		return s.ssr.render(c, "configurations", data)
	}

	items := make([]map[string]any, 0, len(configResources.Items))
	for i := range configResources.Items {
		items = append(items, configResources.Items[i].Object)
	}
	data["Configs"] = items
	return s.ssr.render(c, "configurations", data)
}

// getMutations renders the Mutations view. It reads the very same Gatekeeper mutator objects as
// the JSON handler getMutations (assign, assignmetadata, modifyset, assignimage under
// mutations.gatekeeper.sh/v1), then hands them to the template instead of to c.JSON.
func (s *server) getMutations(c echo.Context) error {
	layout := s.ssrLayoutData(c, "mutations", "/mutations", "Mutations")

	data := map[string]any{"Layout": layout}

	clients, err := s.clientsFor(c)
	if err != nil {
		slog.Error("SSR mutations: resolving context failed", "error", err)
		setViewError(data, "GPM could not switch to the requested Kubernetes context. Make sure the kubeconfig defines it correctly.", err)
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

// --- Constraint Templates view ---------------------------------------------------------------

// ssrConstraintTemplate is the flat shape the template renders. The rego lookup (rego, or the
// first Rego entry under code[]) and the related-constraints join are awkward in a gotpl, so the
// handler resolves them here.
type ssrConstraintTemplate struct {
	Name          string
	Kind          string
	Created       string
	Description   string
	Target        string
	Rego          string
	Libs          []string
	Schema        map[string]any // openAPIV3Schema.properties; nil when the template takes no parameters
	Constraints   []string       // names of the Constraints that use this template
	StatusCreated bool           // status.created: Gatekeeper compiled the template into a CRD
	Raw           map[string]any // the whole object, for the "Full YAML" details
}

// extractRego returns a target's inline rego, falling back to the first Rego engine entry under
// code[] (the newer ConstraintTemplate shape), mirroring the React view.
func extractRego(target map[string]any) string {
	if rego, found, _ := unstructured.NestedString(target, "rego"); found && rego != "" {
		return rego
	}
	code, found, _ := unstructured.NestedSlice(target, "code")
	if !found {
		return ""
	}
	for _, c := range code {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if engine, _, _ := unstructured.NestedString(cm, "engine"); engine == "Rego" {
			rego, _, _ := unstructured.NestedString(cm, "source", "rego")
			return rego
		}
	}
	return ""
}

func ssrConstraintTemplateModel(ct map[string]any, related []unstructured.Unstructured) ssrConstraintTemplate {
	m := ssrConstraintTemplate{Raw: ct}
	m.Name, _, _ = unstructured.NestedString(ct, "metadata", "name")
	m.Kind, _, _ = unstructured.NestedString(ct, "spec", "crd", "spec", "names", "kind")
	m.Created, _, _ = unstructured.NestedString(ct, "metadata", "creationTimestamp")
	m.StatusCreated, _, _ = unstructured.NestedBool(ct, "status", "created")
	m.Description, _, _ = unstructured.NestedString(ct, "metadata", "annotations", "description")

	// React only ever reads the first target.
	if targets, found, _ := unstructured.NestedSlice(ct, "spec", "targets"); found && len(targets) > 0 {
		if t0, ok := targets[0].(map[string]any); ok {
			m.Target, _, _ = unstructured.NestedString(t0, "target")
			m.Rego = extractRego(t0)
			m.Libs, _, _ = unstructured.NestedStringSlice(t0, "libs")
		}
	}

	if props, found, _ := unstructured.NestedMap(ct, "spec", "crd", "spec", "validation", "openAPIV3Schema", "properties"); found {
		m.Schema = props
	}

	for i := range related {
		m.Constraints = append(m.Constraints, related[i].GetName())
	}
	return m
}

// getConstraintTemplates renders the Constraint Templates view. It reads the same objects as the
// JSON handler getConstraintTemplates: the templates, plus the Constraints that use each one.
func (s *server) getConstraintTemplates(c echo.Context) error {
	layout := s.ssrLayoutData(c, "constrainttemplates", "/constrainttemplates", "Constraint Templates")

	data := map[string]any{"Layout": layout}

	clients, err := s.clientsFor(c)
	if err != nil {
		slog.Error("SSR constraint templates: resolving context failed", "error", err)
		setViewError(data, "GPM could not switch to the requested Kubernetes context. Make sure the kubeconfig defines it correctly.", err)
		return s.ssr.render(c, "constrainttemplates", data)
	}

	ctx := c.Request().Context()
	cts, err := getCustomResources(ctx, *clients.dynamic, "templates.gatekeeper.sh", "v1", "constrainttemplates")
	if err != nil {
		slog.Error("SSR constraint templates: getting resources failed", "error", err)
		setViewError(data, "GPM could not get the Constraint Template objects from the Kubernetes API. Make sure Gatekeeper is installed in the cluster.", err)
		return s.ssr.render(c, "constrainttemplates", data)
	}

	templates := make([]ssrConstraintTemplate, 0, len(cts.Items))
	for i := range cts.Items {
		name := cts.Items[i].GetName()
		// A missing constraint kind just means the template has no constraints yet, so we log and
		// continue with an empty list rather than failing the whole page.
		constraints, err := getCustomResources(ctx, *clients.dynamic, "constraints.gatekeeper.sh", "v1beta1", name)
		if err != nil {
			slog.Debug("SSR constraint templates: getting related constraints failed", "constraintTemplate", name, "error", err)
			constraints = &unstructured.UnstructuredList{}
		}
		templates = append(templates, ssrConstraintTemplateModel(cts.Items[i].Object, constraints.Items))
	}
	data["Templates"] = templates
	return s.ssr.render(c, "constrainttemplates", data)
}

// --- Constraints view -------------------------------------------------------------------------

// ssrConstraintViolation is one row of the per-constraint violations table. The JSON tags matter:
// this shape is serialized into the page's data island and read back by the Alpine table.
type ssrConstraintViolation struct {
	EnforcementAction string `json:"enforcementAction"`
	Kind              string `json:"kind"`
	Namespace         string `json:"namespace"`
	Name              string `json:"name"`
	Message           string `json:"message"`
}

// ssrConstraintPod mirrors one status.byPod entry: which audit pod reported, at what generation,
// and whether it is enforcing the constraint.
type ssrConstraintPod struct {
	ID                 string
	ObservedGeneration string
	Enforced           bool
}

// ssrConstraint is the flat shape the template renders per constraint.
type ssrConstraint struct {
	Name              string
	Kind              string
	Created           string
	HasSpec           bool
	EnforcementAction string
	EnforcementMode   string // deny | warn | dryrun, mirroring the React enforcement icon mapping
	Match             map[string]any
	Parameters        map[string]any

	ViolationsKnown bool  // status.totalViolations present; absent means Gatekeeper has not audited yet
	TotalViolations int64 // status.totalViolations
	ReturnedCount   int   // len(Violations); the audit limit can make this smaller than TotalViolations
	AuditLimited    bool  // TotalViolations > ReturnedCount
	Violations      []ssrConstraintViolation

	AuditTimestamp string
	Pods           []ssrConstraintPod

	Raw map[string]any
}

// enforcementMode collapses spec.enforcementAction to the three modes the UI shows, matching the
// React getEnforcementActionRenderData default (anything but dryrun/warn is "deny").
func enforcementMode(action string) string {
	switch action {
	case "dryrun":
		return "dryrun"
	case "warn":
		return "warn"
	default:
		return "deny"
	}
}

func ssrConstraintModel(o map[string]any) ssrConstraint {
	m := ssrConstraint{Raw: o}
	m.Name, _, _ = unstructured.NestedString(o, "metadata", "name")
	m.Kind, _, _ = unstructured.NestedString(o, "kind")
	m.Created, _, _ = unstructured.NestedString(o, "metadata", "creationTimestamp")

	_, m.HasSpec, _ = unstructured.NestedMap(o, "spec")
	m.EnforcementAction, _, _ = unstructured.NestedString(o, "spec", "enforcementAction")
	m.EnforcementMode = enforcementMode(m.EnforcementAction)
	if match, found, _ := unstructured.NestedMap(o, "spec", "match"); found {
		m.Match = match
	}
	if params, found, _ := unstructured.NestedMap(o, "spec", "parameters"); found {
		m.Parameters = params
	}

	if tv, found, _ := unstructured.NestedInt64(o, "status", "totalViolations"); found {
		m.ViolationsKnown = true
		m.TotalViolations = tv
	}
	m.AuditTimestamp, _, _ = unstructured.NestedString(o, "status", "auditTimestamp")

	if vs, found, _ := unstructured.NestedSlice(o, "status", "violations"); found {
		for _, v := range vs {
			vm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			viol := ssrConstraintViolation{}
			viol.EnforcementAction, _, _ = unstructured.NestedString(vm, "enforcementAction")
			viol.Kind, _, _ = unstructured.NestedString(vm, "kind")
			viol.Namespace, _, _ = unstructured.NestedString(vm, "namespace")
			viol.Name, _, _ = unstructured.NestedString(vm, "name")
			viol.Message, _, _ = unstructured.NestedString(vm, "message")
			m.Violations = append(m.Violations, viol)
		}
	}
	m.ReturnedCount = len(m.Violations)
	m.AuditLimited = m.ViolationsKnown && m.TotalViolations > int64(m.ReturnedCount)

	if pods, found, _ := unstructured.NestedSlice(o, "status", "byPod"); found {
		for _, p := range pods {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			pod := ssrConstraintPod{}
			pod.ID, _, _ = unstructured.NestedString(pm, "id")
			pod.Enforced, _, _ = unstructured.NestedBool(pm, "enforced")
			if g, found, _ := unstructured.NestedInt64(pm, "observedGeneration"); found {
				pod.ObservedGeneration = strconv.FormatInt(g, 10)
			}
			m.Pods = append(m.Pods, pod)
		}
	}
	return m
}

// listConstraintsConcurrency caps the in-flight per-Kind list calls, so a cluster with dozens of
// Constraint Kinds does not open dozens of simultaneous API connections.
const listConstraintsConcurrency = 16

// listConstraints discovers every Constraint Kind Gatekeeper created under
// constraints.gatekeeper.sh/v1beta1 and returns all Constraint objects across those Kinds. The
// constraints view and the multi-cluster dashboard both read Constraints this way.
//
// Each Kind is a separate API round-trip, and a cluster can define dozens of them (most empty), so
// the lists run concurrently: sequentially this is N*RTT, which is seconds against a remote cluster.
func listConstraints(ctx context.Context, clients *kubeClients) ([]map[string]interface{}, error) {
	// Constraint Kinds are created dynamically by Gatekeeper per template, so discover them first.
	kinds, err := clients.discovery.ServerResourcesForGroupVersion("constraints.gatekeeper.sh/v1beta1")
	if err != nil {
		return nil, fmt.Errorf("listing constraint kinds: %w", err)
	}

	names := make([]string, 0, len(kinds.APIResources))
	for _, k := range kinds.APIResources {
		// Subresources (like <kind>/status) have no categories; skip them.
		if k.Categories == nil {
			continue
		}
		names = append(names, k.SingularName)
	}

	perKind := make([][]map[string]interface{}, len(names))
	errs := make([]error, len(names))
	sem := make(chan struct{}, listConstraintsConcurrency)
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			list, err := getCustomResources(ctx, *clients.dynamic, "constraints.gatekeeper.sh", "v1beta1", name)
			if err != nil {
				errs[i] = fmt.Errorf("getting %s constraints: %w", name, err)
				return
			}
			items := make([]map[string]interface{}, 0, len(list.Items))
			for j := range list.Items {
				items = append(items, list.Items[j].Object)
			}
			perKind[i] = items
		}(i, name)
	}
	wg.Wait()

	var raw []map[string]interface{}
	for i := range perKind {
		if errs[i] != nil {
			return nil, errs[i]
		}
		raw = append(raw, perKind[i]...)
	}
	return raw, nil
}

// getConstraints: discover the constraint Kinds under constraints.gatekeeper.sh/v1beta1, list each,
// then sortConstraints (most violations first, then by name). The report link in the sidebar points
// at the existing HTML report the JSON handler serves with ?report=html.
func (s *server) getConstraints(c echo.Context) error {
	layout := s.ssrLayoutData(c, "constraints", "/constraints", "Constraints")

	data := map[string]any{"Layout": layout}

	clients, err := s.clientsFor(c)
	if err != nil {
		slog.Error("SSR constraints: resolving context failed", "error", err)
		setViewError(data, "GPM could not switch to the requested Kubernetes context. Make sure the kubeconfig defines it correctly.", err)
		return s.ssr.render(c, "constraints", data)
	}

	ctx := c.Request().Context()

	raw, err := listConstraints(ctx, clients)
	if err != nil {
		slog.Error("SSR constraints: reading constraints failed", "error", err)
		setViewError(data, "GPM could not read the Constraints from the Kubernetes API. Make sure Gatekeeper is installed in the cluster.", err)
		return s.ssr.render(c, "constraints", data)
	}

	sortConstraints(raw)

	// The context named in the path, or the kubeconfig default. It names the cluster the report
	// describes and disambiguates the report URL, both on a multi-context kubeconfig.
	selected := c.Param("context")
	if selected == "" {
		_, selected = s.k8s.contexts()
	}

	// The printable HTML report shares this data path. When ?report is present, render it instead
	// of the interactive view.
	if c.QueryParam("report") != "" {
		return c.Render(http.StatusOK, "report", map[string]any{
			"constraints":   raw,
			"apiServerHost": clients.rest.Host,
			"context":       selected,
			"timestamp":     time.Now().Format(time.ANSIC),
		})
	}

	models := make([]ssrConstraint, 0, len(raw))
	for _, o := range raw {
		models = append(models, ssrConstraintModel(o))
	}
	data["Constraints"] = models

	// The printable report is this same view with ?report set.
	if selected != "" {
		data["ReportURL"] = browserPath("/constraints/" + url.PathEscape(selected) + "?report=html")
	} else {
		data["ReportURL"] = browserPath("/constraints?report=html")
	}
	return s.ssr.render(c, "constraints", data)
}

// --- Events view ------------------------------------------------------------------------------

// ssrEvent is the flat shape the events table renders. Gatekeeper carries most of the detail in
// annotations, and the timestamps need server-side formatting, so the handler resolves it here.
type ssrEvent struct {
	Name           string
	Reason         string
	Message        string
	Count          string
	Action         string
	ConstraintKind string
	ConstraintName string
	FirstTimestamp string
	LastTimestamp  string

	ObjKind      string
	ObjName      string
	ObjNamespace string

	EventType       string
	Process         string
	RequestUsername string

	ResourceAPIVersion string
	ResourceGroup      string
	ResourceKind       string
	ResourceName       string
	ResourceNamespace  string

	SourceComponent string
	SourceHost      string
}

// formatTimestamp turns an RFC3339 Kubernetes timestamp into a readable 24-hour UTC string,
// leaving anything it cannot parse untouched.
func formatTimestamp(v string) string {
	if v == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return v
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}

func ssrEventModel(e map[string]any) ssrEvent {
	ann := func(k string) string {
		v, _, _ := unstructured.NestedString(e, "metadata", "annotations", k)
		return v
	}

	m := ssrEvent{}
	m.Name, _, _ = unstructured.NestedString(e, "metadata", "name")
	m.Reason, _, _ = unstructured.NestedString(e, "reason")
	m.Message, _, _ = unstructured.NestedString(e, "message")
	if count, found, _ := unstructured.NestedInt64(e, "count"); found {
		m.Count = strconv.FormatInt(count, 10)
	}

	first, _, _ := unstructured.NestedString(e, "firstTimestamp")
	last, _, _ := unstructured.NestedString(e, "lastTimestamp")
	m.FirstTimestamp = formatTimestamp(first)
	m.LastTimestamp = formatTimestamp(last)

	m.Action = ann("constraint_action")
	m.ConstraintKind = ann("constraint_kind")
	m.ConstraintName = ann("constraint_name")
	m.EventType = ann("event_type")
	m.Process = ann("process")
	m.RequestUsername = ann("request_username")
	m.ResourceAPIVersion = ann("resource_api_version")
	m.ResourceGroup = ann("resource_group")
	m.ResourceKind = ann("resource_kind")
	m.ResourceName = ann("resource_name")
	m.ResourceNamespace = ann("resource_namespace")

	m.ObjKind, _, _ = unstructured.NestedString(e, "involvedObject", "kind")
	m.ObjName, _, _ = unstructured.NestedString(e, "involvedObject", "name")
	m.ObjNamespace, _, _ = unstructured.NestedString(e, "involvedObject", "namespace")

	m.SourceComponent, _, _ = unstructured.NestedString(e, "source", "component")
	m.SourceHost, _, _ = unstructured.NestedString(e, "source", "host")
	return m
}

// getEvents renders the Events view. It reads the same events as the JSON handler getEvents:
// core v1 Events filtered to the configured source (GPM_EVENTS_SOURCE), in the configured or
// requested namespace. Like getEvents, this is an alpha feature; see its note in handlers.go.
func (s *server) getEvents(c echo.Context) error {
	layout := s.ssrLayoutData(c, "events", "/events", "Events")

	data := map[string]any{"Layout": layout}

	clients, err := s.clientsFor(c)
	if err != nil {
		slog.Error("SSR events: resolving context failed", "error", err)
		setViewError(data, "GPM could not switch to the requested Kubernetes context. Make sure the kubeconfig defines it correctly.", err)
		return s.ssr.render(c, "events", data)
	}

	// GPM_EVENTS_SOURCE is a comma-separated list of event source components. Gatekeeper tags
	// admission events with gatekeeper-webhook and audit events with gatekeeper-audit; the default
	// shows both.
	var sources []string
	for _, src := range strings.Split(viper.GetString("events_source"), ",") {
		if src = strings.TrimSpace(src); src != "" {
			sources = append(sources, src)
		}
	}
	// GPM_EVENTS_NAMESPACE wins over the query parameter.
	namespace := viper.GetString("events_namespace")
	if namespace == "" {
		namespace = c.QueryParam("namespace")
	}

	events, err := getKubernetesEvents(c.Request().Context(), *clients.dynamic, namespace, sources)
	if err != nil {
		slog.Error("SSR events: getting events failed", "namespace", namespace, "sources", sources, "error", err)
		setViewError(data, "GPM could not get the events from the Kubernetes API. Make sure the API is reachable.", err)
		return s.ssr.render(c, "events", data)
	}

	models := make([]ssrEvent, 0, len(*events))
	for i := range *events {
		models = append(models, ssrEventModel((*events)[i].Object))
	}
	data["Events"] = models
	return s.ssr.render(c, "events", data)
}

// --- Home / Error / NotFound -----------------------------------------------------------------

// getHome renders the landing page. It carries no data of its own: the cards link into the five
// views, and ssrLayoutData already builds those links context-aware in .Layout.Nav, so the template
// reuses them.
func (s *server) getHome(c echo.Context) error {
	layout := s.ssrLayoutData(c, "home", "/home", "Home")
	// The dashboard is fleet-wide, so a per-context switcher would be a no-op here. Drop it; the
	// cluster cards are how you drill into one cluster.
	layout.Contexts = nil
	layout.HasContexts = false
	dashboard := s.buildDashboard(c.Request().Context())
	return s.ssr.render(c, "home", map[string]any{"Layout": layout, "Dashboard": dashboard})
}

// --- Multi-cluster dashboard ------------------------------------------------------------------

// dashboardCluster is one cluster's roll-up on the home dashboard. The json tags feed the sortable
// clusters table, which Alpine renders client-side from a data island.
type dashboardCluster struct {
	Name            string `json:"name"`     // display name; the unnamed in-cluster context reads as "current cluster"
	Selected        bool   `json:"selected"` // the kubeconfig's current-context
	Reachable       bool   `json:"reachable"`
	ConstraintCount int    `json:"constraints"`
	Violations      int    `json:"violations"`
	ConstraintsURL  string `json:"url"`    // link to this cluster's constraints view
	Status          string `json:"status"` // Violations | Compliant | Unreachable (sortable label)
	State           string `json:"state"`  // bad | ok | warn (drives the status dot color)
	// The raw fetch error is deliberately not carried here: it can name internal API-server hosts,
	// IPs and cert details, and the dashboard is reachable without a session under Anonymous auth.
	// It is logged server-side in fetchClusterConstraints; the table only shows "Unreachable".
}

// dashboardConstraintCluster is one cluster's share of a Constraint's cross-cluster violations.
type dashboardConstraintCluster struct {
	Cluster    string `json:"cluster"`
	Violations int    `json:"violations"`
	URL        string `json:"url"` // deep link to the Constraint on that cluster's constraints page
}

// dashboardConstraint aggregates one Constraint (by kind and name) across every cluster.
type dashboardConstraint struct {
	Kind         string                       `json:"kind"`
	Name         string                       `json:"name"`
	Violations   int                          `json:"violations"`
	ClusterCount int                          `json:"clusterCount"` // sortable "Clusters" column
	Clusters     []dashboardConstraintCluster `json:"clusters"`
}

// dashboardData is the whole home dashboard: the grand totals, two donut charts, a per-cluster
// roll-up, and a per-Constraint breakdown of everything that is violating, most-violated first.
type dashboardData struct {
	Clusters          []dashboardCluster
	Violating         []dashboardConstraint
	ClustersDonut     donut // clusters split into violating / compliant / unreachable
	EnforcementDonut  donut // constraints split by enforcement mode (deny / warn / dry run)
	TotalClusters     int
	ReachableClusters int
	TotalConstraints  int
	TotalViolations   int
	GeneratedUnixMs   int64 // when this data was fetched, for the "updated Ns ago" hint (may be cached)
}

// donutSegment is one slice of a donut chart: its share of the ring (as SVG stroke geometry over a
// circle whose circumference is normalised to 100) and its legend entry.
type donutSegment struct {
	Label      string
	Count      int
	Class      string // danger | warn | success | indigo | muted -> a color in CSS
	DashArray  string // "<len> <gap>" over a circumference of 100
	DashOffset string // rotates the slice to continue where the previous one ended
}

// donut is a server-rendered SVG donut: a number in the middle (the card heading names it) and its
// segments.
type donut struct {
	Center   string
	Segments []donutSegment
}

// newDonut computes the ring geometry for the non-empty slices. With the SVG circle's circumference
// normalised to 100, a slice's dash length is its percentage and the offset is 25 (12 o'clock) minus
// the percentages already drawn, so the slices sit clockwise from the top with no gaps.
func newDonut(center string, slices []donutSegment) donut {
	total := 0
	for _, s := range slices {
		total += s.Count
	}
	d := donut{Center: center}
	if total == 0 {
		return d
	}
	var drawn float64
	for _, s := range slices {
		if s.Count == 0 {
			continue
		}
		pct := float64(s.Count) / float64(total) * 100
		s.DashArray = fmt.Sprintf("%.4f %.4f", pct, 100-pct)
		s.DashOffset = fmt.Sprintf("%.4f", 25-drawn)
		drawn += pct
		d.Segments = append(d.Segments, s)
	}
	return d
}

// clusterConstraints is the raw per-cluster fetch that aggregateDashboard rolls up.
type clusterConstraints struct {
	context     string
	selected    bool
	reachable   bool
	err         error
	constraints []ssrConstraint
}

// dashboardCache holds the last-built dashboard for a short TTL. The dashboard is fleet-wide (the
// same for every viewer) and fans out to every cluster, so caching it briefly coalesces repeated
// loads into a single fan-out. That matters because /home is reachable without a session under the
// default Anonymous auth, so it is a cheap unauthenticated lever otherwise.
type dashboardCache struct {
	mu       sync.Mutex
	data     dashboardData
	computed time.Time // zero until the dashboard has been built at least once
}

const dashboardCacheTTL = 10 * time.Second

// buildDashboard returns the cached dashboard when it is still fresh, otherwise rebuilds it. The lock
// is held across the rebuild on purpose: concurrent loads then coalesce onto one fan-out instead of
// each launching its own. Data can be up to dashboardCacheTTL stale, which is fine — Gatekeeper's
// audit lags by ~a minute anyway.
func (s *server) buildDashboard(ctx context.Context) dashboardData {
	s.dashCache.mu.Lock()
	defer s.dashCache.mu.Unlock()

	if !s.dashCache.computed.IsZero() && time.Since(s.dashCache.computed) < dashboardCacheTTL {
		return s.dashCache.data
	}

	// Build under a context detached from the caller's cancellation. The result is cached and served
	// to every viewer, so a client that disconnects mid-rebuild must not poison the shared entry with
	// a "context canceled" (every-cluster-unreachable) dashboard. The per-cluster timeout in
	// fetchClusterConstraints still bounds the fan-out.
	data := s.computeDashboard(context.WithoutCancel(ctx))
	now := time.Now()
	data.GeneratedUnixMs = now.UnixMilli()
	s.dashCache.data = data
	s.dashCache.computed = now
	return data
}

// computeDashboard reads the Constraints of every kubeconfig context in parallel, each bounded by its
// own timeout so one unreachable cluster cannot hang the page, then aggregates them. An unreachable
// cluster becomes an error row rather than failing the whole view.
func (s *server) computeDashboard(ctx context.Context) dashboardData {
	contexts, current := s.k8s.contexts()

	names := make([]string, 0, len(contexts))
	for n := range contexts {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) == 0 {
		// In-cluster or a kubeconfig with no named contexts: the single default cluster.
		names = []string{defaultKubeContext}
	}

	// ponytail: one goroutine per cluster, unbounded. A kubeconfig holds a handful of clusters, not
	// hundreds; add a worker pool only if that stops being true.
	results := make([]clusterConstraints, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			results[i] = s.fetchClusterConstraints(ctx, name, name == current)
		}(i, name)
	}
	wg.Wait()

	return aggregateDashboard(results)
}

// fetchClusterConstraints resolves one context and lists its Constraints under a 10s timeout.
func (s *server) fetchClusterConstraints(ctx context.Context, name string, selected bool) clusterConstraints {
	res := clusterConstraints{context: name, selected: selected}

	clients, err := s.k8s.forContext(name)
	if err != nil {
		slog.Warn("dashboard: resolving cluster failed", "cluster", name, "error", err)
		res.err = err
		return res
	}

	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	raw, err := listConstraints(cctx, clients)
	if err != nil {
		slog.Warn("dashboard: reading cluster constraints failed", "cluster", name, "error", err)
		res.err = err
		return res
	}

	res.reachable = true
	res.constraints = make([]ssrConstraint, 0, len(raw))
	for _, o := range raw {
		res.constraints = append(res.constraints, ssrConstraintModel(o))
	}
	return res
}

// aggregateDashboard turns the per-cluster fetches into the dashboard model. It is free of any
// Kubernetes client so it can be unit-tested directly.
func aggregateDashboard(results []clusterConstraints) dashboardData {
	d := dashboardData{TotalClusters: len(results)}

	// Aggregate violating Constraints by kind+name across clusters, keeping first-seen order with a
	// slice plus an index map so the output is deterministic before the final sort.
	type key struct{ kind, name string }
	index := map[key]int{}
	var violating []dashboardConstraint

	// Donut tallies.
	var clustersViolating, clustersCompliant, clustersUnreachable int
	var deny, warn, dryrun int

	for _, r := range results {
		cluster := dashboardCluster{
			Name:           clusterLabel(r.context),
			Selected:       r.selected,
			Reachable:      r.reachable,
			ConstraintsURL: constraintsURL(r.context, "", ""),
		}
		if r.err != nil {
			cluster.Status, cluster.State = "Unreachable", "warn"
			clustersUnreachable++
			d.Clusters = append(d.Clusters, cluster)
			continue
		}

		d.ReachableClusters++
		cluster.ConstraintCount = len(r.constraints)
		for _, c := range r.constraints {
			d.TotalConstraints++
			switch c.EnforcementMode {
			case "warn":
				warn++
			case "dryrun":
				dryrun++
			default:
				deny++
			}
			if !c.ViolationsKnown || c.TotalViolations == 0 {
				continue
			}
			v := int(c.TotalViolations)
			cluster.Violations += v
			d.TotalViolations += v

			k := key{c.Kind, c.Name}
			i, ok := index[k]
			if !ok {
				i = len(violating)
				index[k] = i
				violating = append(violating, dashboardConstraint{Kind: c.Kind, Name: c.Name})
			}
			violating[i].Violations += v
			violating[i].Clusters = append(violating[i].Clusters, dashboardConstraintCluster{
				Cluster:    cluster.Name,
				Violations: v,
				URL:        constraintsURL(r.context, c.Kind, c.Name),
			})
		}
		if cluster.Violations > 0 {
			cluster.Status, cluster.State = "Violations", "bad"
			clustersViolating++
		} else {
			cluster.Status, cluster.State = "Compliant", "ok"
			clustersCompliant++
		}
		d.Clusters = append(d.Clusters, cluster)
	}

	// Most-violated first, then by name for a stable order.
	sort.SliceStable(violating, func(i, j int) bool {
		if violating[i].Violations != violating[j].Violations {
			return violating[i].Violations > violating[j].Violations
		}
		return violating[i].Name < violating[j].Name
	})
	for i := range violating {
		violating[i].ClusterCount = len(violating[i].Clusters)
	}
	d.Violating = violating

	d.ClustersDonut = newDonut(strconv.Itoa(d.TotalClusters), []donutSegment{
		{Label: "With violations", Count: clustersViolating, Class: "danger"},
		{Label: "Compliant", Count: clustersCompliant, Class: "success"},
		{Label: "Unreachable", Count: clustersUnreachable, Class: "warn"},
	})
	d.EnforcementDonut = newDonut(strconv.Itoa(d.TotalConstraints), []donutSegment{
		{Label: "Deny", Count: deny, Class: "indigo"},
		{Label: "Warn", Count: warn, Class: "warn"},
		{Label: "Dry run", Count: dryrun, Class: "muted"},
	})
	return d
}

// clusterLabel is the display name for a context. The unnamed default (in-cluster) context has no
// name, so give it a readable label.
func clusterLabel(context string) string {
	if context == "" {
		return "current cluster"
	}
	return context
}

// constraintAnchor is the fragment a Constraint card answers to. The name alone is not unique: two
// Constraints of different Kinds may share one, and that produced two cards with the same id, so one
// was unreachable, its sidebar entry could never be marked, and a shared link (issue #1324) landed on
// the wrong card. Kind plus name is unique for these cluster-scoped objects, and unlike metadata.uid
// it survives the object being recreated, which is the property a shared link needs. Falls back to
// the name alone when the Kind is unknown, which only happens for an Event that did not record it.
func constraintAnchor(kind, name string) string {
	if kind == "" {
		return name
	}
	return kind + "--" + name
}

// constraintsURL links to the constraints view for a context, optionally anchored at a Constraint.
func constraintsURL(kubeContext, kind, name string) string {
	path := "/constraints"
	if kubeContext != "" {
		path += "/" + url.PathEscape(kubeContext)
	}
	if name != "" {
		path += "#" + constraintAnchor(kind, name)
	}
	return browserPath(path)
}

// ssrErrorView is the flat shape the error page renders. It mirrors the fields the React Error page
// shows (message, action, description, and an optional login link when a session expired).
type ssrErrorView struct {
	Message     string
	Action      string
	Description string
	LoginURL    string // set only when signing in fixes the error; renders a "Log in" button
	BackURL     string // where "Go back" points; defaults to home
}

// publicLayout builds the layout for a page that is served without a session: the signed-out page
// and the error and 404 pages. It drops the context switcher, so an anonymous visitor to one of
// these public paths cannot read the operator's context names off a multi-context kubeconfig. The
// React app served a data-free shell on these paths; this keeps that property.
func (s *server) publicLayout(c echo.Context, title string) ssrLayout {
	l := s.ssrLayoutData(c, "", "/home", title)
	l.Contexts = nil
	l.HasContexts = false
	return l
}

// setViewError puts an operator-facing message on the view data together with the error underneath
// it. Issue #631: the generic sentence alone hides why the call failed, so someone whose Gatekeeper
// or API server answers unexpectedly cannot tell a 403 from a missing CRD without reading the pod
// logs. The 1.x UI showed this detail and the rewrite dropped it.
//
// The detail goes on the view pages only, never on the session-less error and 404 pages: those are
// reachable without a session, which is why publicLayout already strips the context names from
// them.
//
// The detail names the API server address, and an RBAC denial names the identity GPM calls with.
// With OIDC on, only a signed-in operator reads that. With authentication off GPM is already
// readable by anyone who can reach it (main.go says so at startup) and it renders every constraint,
// violation and namespace in the cluster, so the address adds little to what is on the page.
//
// A certificate failure gets the GPM_SKIP_TLS_VERIFY message the JSON API's kubeAPIErrorAnswer gave
// until 9c9e27f removed it. Every view routes through here, so it is back everywhere.
func setViewError(data map[string]any, message string, err error) {
	var (
		verificationErr *tls.CertificateVerificationError
		authorityErr    x509.UnknownAuthorityError
		hostnameErr     x509.HostnameError
		invalidCertErr  x509.CertificateInvalidError
	)
	if errors.As(err, &verificationErr) || errors.As(err, &authorityErr) ||
		errors.As(err, &hostnameErr) || errors.As(err, &invalidCertErr) {
		message = "GPM could not verify the Kubernetes API server's TLS certificate. " +
			"Set GPM_SKIP_TLS_VERIFY=true if the cluster CA is missing the AKI/SKI extensions, as happens on EKS. Use with caution."
	}
	data["Error"] = message
	data["ErrorDetail"] = err.Error()
}

// renderError renders the shared error page with the given status. login sensibly defaults BackURL
// to home when the caller leaves it empty.
func (s *server) renderError(c echo.Context, status int, e ssrErrorView) error {
	if e.BackURL == "" {
		e.BackURL = browserPath("/")
	}
	layout := s.publicLayout(c, "Error")
	return s.ssr.renderStatus(c, status, "error", map[string]any{"Layout": layout, "Err": e})
}

// renderNotFound renders the shared 404 page.
func (s *server) renderNotFound(c echo.Context) error {
	layout := s.publicLayout(c, "Not found")
	return s.ssr.renderStatus(c, http.StatusNotFound, "notfound", map[string]any{"Layout": layout})
}

// registerViews wires the server-rendered UI: the embedded static assets and every view at its real
// path. Called from main after the server is built. Home is the landing page at "/". The 404 and
// error pages are not routes; the global echo.HTTPErrorHandler in main renders them.
func registerViews(e *echo.Echo, s *server) {
	assets, err := fs.Sub(ssrStaticFS, "static/ssr")
	if err != nil {
		slog.Error("SSR static assets could not be mounted", "error", err)
		return
	}
	e.GET("/static/*", echo.WrapHandler(
		http.StripPrefix("/static/", http.FileServer(http.FS(assets)))))

	// Home is the landing page. "/home" and "/home/:context" also resolve to it so the context
	// switcher has a target that keeps the selected context.
	e.GET("/", s.getHome)
	e.GET("/home", s.getHome)
	e.GET("/home/:context", s.getHome)

	e.GET("/configurations", s.getConfigurations)
	e.GET("/configurations/:context", s.getConfigurations)

	e.GET("/mutations", s.getMutations)
	e.GET("/mutations/:context", s.getMutations)

	e.GET("/constrainttemplates", s.getConstraintTemplates)
	e.GET("/constrainttemplates/:context", s.getConstraintTemplates)

	e.GET("/constraints", s.getConstraints)
	e.GET("/constraints/:context", s.getConstraints)

	e.GET("/events", s.getEvents)
	e.GET("/events/:context", s.getEvents)
}

// renderLoggedOut renders the "you are signed out" page. It is what the local logout path lands
// on (see auth.go): a public page, so it does not bounce a just-logged-out user back to the IdP.
func (s *server) renderLoggedOut(c echo.Context) error {
	layout := s.publicLayout(c, "Signed out")
	return s.ssr.renderStatus(c, http.StatusOK, "loggedout",
		map[string]any{"Layout": layout, "LoginURL": browserPath("/login")})
}

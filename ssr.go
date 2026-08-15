// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Server-rendered UI (proof of concept). Templates and static assets are embedded, so this path
// does not depend on the working directory the way the React static-content serving does. It is
// additive: it lives under /ssr and touches none of the existing routes or handlers.
package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"sort"
	"strconv"
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
var chromaFormatter = chromahtml.New(chromahtml.WithClasses(true), chromahtml.PreventSurroundingPre(true))

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
	AssetBase   string // browserPath("/ssr/static"), where the CSS and Alpine live
	Nav         []navLink
	Contexts    []ctxOption
	HasContexts bool
	AuthEnabled bool
	LogoutURL   string
}

// The top-nav destinations. Every view is server-rendered now. Home has no nav entry -- the logo
// links back to it. As each view is ported its ssr flag flips and its path moves under /ssr.
var ssrNavRoutes = []struct {
	Key  string
	Name string
	Path string
	SSR  bool
}{
	{"constrainttemplates", "Constraint Templates", "/ssr/constrainttemplates", true},
	{"constraints", "Constraints", "/ssr/constraints", true},
	{"mutations", "Mutations", "/ssr/mutations", true},
	{"events", "Events", "/ssr/events", true},
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

// --- Constraint Templates view ---------------------------------------------------------------

// ssrConstraintTemplate is the flat shape the template renders. The rego lookup (rego, or the
// first Rego entry under code[]) and the related-constraints join are awkward in a gotpl, so the
// handler resolves them here.
type ssrConstraintTemplate struct {
	Name        string
	Kind        string
	Created     string
	Description string
	Target      string
	Rego        string
	Libs        []string
	Schema      map[string]any // openAPIV3Schema.properties; nil when the template takes no parameters
	Constraints []string       // names of the Constraints that use this template
	Raw         map[string]any // the whole object, for the "Full YAML" details
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

// getSSRConstraintTemplates renders the Constraint Templates view. It reads the same objects as the
// JSON handler getConstraintTemplates: the templates, plus the Constraints that use each one.
func (s *server) getSSRConstraintTemplates(c echo.Context) error {
	layout := s.ssrLayoutData(c, "constrainttemplates", "/ssr/constrainttemplates", "Constraint Templates")

	data := map[string]any{"Layout": layout}

	clients, err := s.clientsFor(c)
	if err != nil {
		slog.Error("SSR constraint templates: resolving context failed", "error", err)
		data["Error"] = "GPM could not switch to the requested Kubernetes context. Make sure the kubeconfig defines it correctly."
		return s.ssr.render(c, "constrainttemplates", data)
	}

	ctx := c.Request().Context()
	cts, err := getCustomResources(ctx, *clients.dynamic, "templates.gatekeeper.sh", "v1", "constrainttemplates")
	if err != nil {
		slog.Error("SSR constraint templates: getting resources failed", "error", err)
		data["Error"] = "GPM could not get the Constraint Template objects from the Kubernetes API. Make sure Gatekeeper is installed in the cluster."
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

// getSSRConstraints renders the Constraints view. It walks the same data path as the JSON handler
// getConstraints: discover the constraint Kinds under constraints.gatekeeper.sh/v1beta1, list each,
// then sortConstraints (most violations first, then by name). The report link in the sidebar points
// at the existing HTML report the JSON handler serves with ?report=html.
func (s *server) getSSRConstraints(c echo.Context) error {
	layout := s.ssrLayoutData(c, "constraints", "/ssr/constraints", "Constraints")

	data := map[string]any{"Layout": layout}

	clients, err := s.clientsFor(c)
	if err != nil {
		slog.Error("SSR constraints: resolving context failed", "error", err)
		data["Error"] = "GPM could not switch to the requested Kubernetes context. Make sure the kubeconfig defines it correctly."
		return s.ssr.render(c, "constraints", data)
	}

	ctx := c.Request().Context()

	// Constraint Kinds are created dynamically by Gatekeeper per template, so discover them first.
	availableConstraints, err := clients.discovery.ServerResourcesForGroupVersion("constraints.gatekeeper.sh/v1beta1")
	if err != nil {
		slog.Error("SSR constraints: listing constraint kinds failed", "error", err)
		data["Error"] = "GPM could not list the Constraints. Make sure Gatekeeper is installed in the cluster."
		return s.ssr.render(c, "constraints", data)
	}

	var raw []map[string]interface{}
	for _, constraintKind := range availableConstraints.APIResources {
		// Subresources (like <kind>/status) have no categories; skip them, as getConstraints does.
		if constraintKind.Categories == nil {
			continue
		}
		constraints, err := getCustomResources(ctx, *clients.dynamic, "constraints.gatekeeper.sh", "v1beta1", constraintKind.SingularName)
		if err != nil {
			slog.Error("SSR constraints: getting constraint resources failed", "kind", constraintKind.SingularName, "error", err)
			data["Error"] = "GPM could not get the constraint objects from the Kubernetes API. Make sure Gatekeeper is deployed in the cluster."
			return s.ssr.render(c, "constraints", data)
		}
		for i := range constraints.Items {
			raw = append(raw, constraints.Items[i].Object)
		}
	}

	sortConstraints(raw)

	models := make([]ssrConstraint, 0, len(raw))
	for _, o := range raw {
		models = append(models, ssrConstraintModel(o))
	}
	data["Constraints"] = models

	// The HTML report the JSON handler serves. It names the context in the path, so the report is
	// unambiguous on a multi-context kubeconfig; fall back to the kubeconfig's current context.
	selected := c.Param("context")
	if selected == "" {
		_, selected = s.k8s.contexts()
	}
	if selected != "" {
		data["ReportURL"] = browserPath("/api/v1/constraints/" + url.PathEscape(selected) + "?report=html")
	} else {
		data["ReportURL"] = browserPath("/api/v1/constraints?report=html")
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

// getSSREvents renders the Events view. It reads the same events as the JSON handler getEvents:
// core v1 Events filtered to the configured source (GPM_EVENTS_SOURCE), in the configured or
// requested namespace. Like getEvents, this is an alpha feature; see its note in handlers.go.
func (s *server) getSSREvents(c echo.Context) error {
	layout := s.ssrLayoutData(c, "events", "/ssr/events", "Events")

	data := map[string]any{"Layout": layout}

	clients, err := s.clientsFor(c)
	if err != nil {
		slog.Error("SSR events: resolving context failed", "error", err)
		data["Error"] = "GPM could not switch to the requested Kubernetes context. Make sure the kubeconfig defines it correctly."
		return s.ssr.render(c, "events", data)
	}

	eventsSource := viper.GetString("events_source")
	// GPM_EVENTS_NAMESPACE wins over the query parameter, exactly as getEvents does.
	namespace := viper.GetString("events_namespace")
	if namespace == "" {
		namespace = c.QueryParam("namespace")
	}

	events, err := getKubernetesEvents(c.Request().Context(), *clients.dynamic, namespace, eventsSource)
	if err != nil {
		slog.Error("SSR events: getting events failed", "namespace", namespace, "source", eventsSource, "error", err)
		data["Error"] = "GPM could not get the events from the Kubernetes API. Make sure the API is reachable."
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

// getSSRHome renders the landing page. It carries no data of its own: the cards link into the five
// views, and ssrLayoutData already builds those links context-aware in .Layout.Nav, so the template
// reuses them.
func (s *server) getSSRHome(c echo.Context) error {
	layout := s.ssrLayoutData(c, "home", "/ssr/home", "Home")
	return s.ssr.render(c, "home", map[string]any{"Layout": layout})
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

// renderSSRError renders the shared error page with the given status. login sensibly defaults BackURL
// to home when the caller leaves it empty.
func (s *server) renderSSRError(c echo.Context, status int, e ssrErrorView) error {
	if e.BackURL == "" {
		e.BackURL = browserPath("/ssr/home")
	}
	layout := s.ssrLayoutData(c, "", "/ssr/home", "Error")
	return s.ssr.renderStatus(c, status, "error", map[string]any{"Layout": layout, "Err": e})
}

// renderSSRNotFound renders the shared 404 page.
func (s *server) renderSSRNotFound(c echo.Context) error {
	layout := s.ssrLayoutData(c, "", "/ssr/home", "Not found")
	return s.ssr.renderStatus(c, http.StatusNotFound, "notfound", map[string]any{"Layout": layout})
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

	e.GET("/ssr/constrainttemplates", s.getSSRConstraintTemplates)
	e.GET("/ssr/constrainttemplates/:context", s.getSSRConstraintTemplates)

	e.GET("/ssr/constraints", s.getSSRConstraints)
	e.GET("/ssr/constraints/:context", s.getSSRConstraints)

	e.GET("/ssr/events", s.getSSREvents)
	e.GET("/ssr/events/:context", s.getSSREvents)

	e.GET("/ssr/home", s.getSSRHome)
	e.GET("/ssr/home/:context", s.getSSRHome)

	// Bare /ssr is not a view; send it to the landing so the entry point is obvious while the SSR
	// UI lives under /ssr. (At the cutover the views move to their real paths and this goes away.)
	e.GET("/ssr", func(c echo.Context) error {
		return c.Redirect(http.StatusFound, browserPath("/ssr/home"))
	})

	// Demo routes so the error and 404 pages are reachable and reviewable. The global
	// echo.HTTPErrorHandler that drives these for real (404 -> notfound, 5xx -> error for HTML,
	// JSON for /api/*) is a cutover concern; see docs/dev/drop-react-plan.md, Retirement.
	e.GET("/ssr/error", func(c echo.Context) error {
		return s.renderSSRError(c, http.StatusInternalServerError, ssrErrorView{
			Message:     "Something went wrong",
			Action:      "Try again, and if the problem continues check the GPM logs.",
			Description: "This is a demo of the server-rendered error page.",
			LoginURL:    browserPath("/login"),
		})
	})
	e.GET("/ssr/notfound", s.renderSSRNotFound)
}

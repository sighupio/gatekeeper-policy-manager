// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The HTML violations report renderer.
package main

import (
	"embed"
	"html/template"
	"io"

	"github.com/labstack/echo/v4"
)

//go:embed templates/constraints-report.html.gotpl
var reportTemplateFS embed.FS

type Template struct {
	templates *template.Template
}

// Builds the renderer for the HTML report. html/template, never text/template: the report
// interpolates cluster-controlled data (constraint and resource names, namespaces, violation
// messages), and only html/template escapes it per HTML context. The template is embedded, so the
// renderer does not depend on the working directory.
func newRenderer() *Template {
	return &Template{templates: template.Must(
		template.ParseFS(reportTemplateFS, "templates/constraints-report.html.gotpl"))}
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

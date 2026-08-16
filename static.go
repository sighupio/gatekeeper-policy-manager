// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The HTML violations report renderer.
package main

import (
	"html/template"
	"io"

	"github.com/labstack/echo/v4"
)

type Template struct {
	templates *template.Template
}

// Builds the renderer for the HTML report. html/template, never text/template: the report
// interpolates cluster-controlled data (constraint and resource names, namespaces, violation
// messages), and only html/template escapes it per HTML context. Kept here, not inline in main, so
// tests render exactly as production does.
func newRenderer() *Template {
	return &Template{templates: template.Must(template.ParseGlob("templates/*.html.gotpl"))}
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

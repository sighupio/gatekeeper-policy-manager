// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Serving the built frontend: the SPA shell, its assets, and the HTML report template.
package main

import (
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/labstack/echo/v4"
	"golang.org/x/exp/slog"
)

// Where the built frontend lives, relative to the working directory.
const staticContentDir = "./static-content"

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
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

	// URL.Path, never RequestURI: the latter keeps the query string and is not normalized, so
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

// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Serving the built frontend: the SPA shell, its assets, and the HTML report template.
package main

import (
	"io"
	"net/http"
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
	root, err := os.OpenRoot(staticContentDir)
	if err != nil {
		slog.Error("could not open the static content directory", "error", err)
		return serveSPAShell(c)
	}
	defer func() { _ = root.Close() }()

	// os.Root refuses at the syscall level anything that escapes the directory, whether by ".." or
	// by a symlink pointing out of it. That replaces the old lexical prefix check, which trusted
	// os.Stat and c.File to stay inside and both follow symlinks. URL.Path, not RequestURI: the
	// latter carries the query string, so "/logout?x=/../../etc/passwd" would reach the filesystem.
	requested := strings.TrimPrefix(path.Clean("/"+c.Request().URL.Path), "/")
	if requested == "" {
		return serveSPAShell(c)
	}

	f, err := root.Open(requested)
	if err != nil {
		slog.Debug("file not served, falling back to index.html", "requested", requested, "error", err)
		return serveSPAShell(c)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		return serveSPAShell(c)
	}

	// ServeContent picks the content type from the name and handles range requests, the same as
	// echo's c.File, but from an already-opened handle inside the root rather than a path.
	http.ServeContent(c.Response(), c.Request(), info.Name(), info.ModTime(), f)
	return nil
}

// Serves the SPA entry point without consulting the request path at all.
func serveSPAShell(c echo.Context) error {
	return c.File(filepath.Join(staticContentDir, "index.html"))
}

// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"path"
	"strings"
	"log/slog"

	"github.com/spf13/viper"
)

// The subpath GPM is served from, as the browser sees it: "" for the domain root, or something
// like "/gpm". Always without a trailing slash.
//
// The reverse proxy removes this prefix before GPM ever sees a request, so the backend routes,
// c.Request().URL.Path and everything GPM matches on are prefix-less. Only the paths GPM hands
// back to the browser -- redirects and the login URL in an error answer -- need it put back on,
// and getting that wrong sends the user outside the proxy's location, where nothing answers.
//
// Set from the PUBLIC_URL the frontend was built with, so the two cannot disagree.
func basePath() string {
	// path.Clean does the whole job: the leading slash makes a bare "gpm" absolute, and Clean then
	// collapses repeated slashes and resolves "." and "..". Trimming one trailing slash instead
	// leaves "//" as "/", which puts the session cookie back at origin-wide scope and makes
	// browserPath("/login") return the off-site "//login" -- from nothing worse than a typo.
	p := path.Clean("/" + strings.TrimSpace(viper.GetString("base_path")))
	if p == "/" {
		return ""
	}
	// Browsers treat "\" as a segment delimiter for http and https (WHATWG URL), so a base path of
	// "\evil.com" makes browserPath("/login") into "/\evil.com/login", which resolves off site.
	// safeRedirectTarget already rejects a "/\" prefix on redirect targets; this is the one place
	// the same rule was not applied.
	if strings.Contains(p, `\`) {
		slog.Error("GPM_BASE_PATH contains a backslash, which browsers read as a path delimiter; ignoring it",
			"configured", p)
		return ""
	}
	return p
}

// Turns a path GPM sees into the one the browser has to ask for. The identity function when GPM is
// served from the domain root, which is why the root deployment is unaffected by any of this.
func browserPath(p string) string {
	base := basePath()
	if base == "" {
		return p
	}
	if p == "" {
		return base + "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return base + p
}

// The Path attribute for the session cookie: the subpath GPM is served from, or "/" at the root.
// Without the trailing slash, so that it matches the base path itself as well as everything under
// it.
func cookiePath() string {
	if base := basePath(); base != "" {
		return base
	}
	return "/"
}

// The reverse of browserPath, for a path arriving from the browser. Anything that is not under the
// base path is returned unchanged and left for safeRedirectTarget to reject or accept on its own
// terms.
func backendPath(p string) string {
	base := basePath()
	if base == "" || p == "" {
		return p
	}
	if p == base {
		return "/"
	}
	if rest, found := strings.CutPrefix(p, base+"/"); found {
		return "/" + rest
	}
	return p
}

// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"strings"

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
	p := strings.TrimSpace(viper.GetString("base_path"))
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

// Turns a path GPM sees into the one the browser has to ask for. The identity function when GPM is
// served from the domain root, which is why the root deployment is unaffected by any of this.
func browserPath(path string) string {
	base := basePath()
	if base == "" {
		return path
	}
	if path == "" {
		return base + "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

// The reverse of browserPath, for a path arriving from the browser. Anything that is not under the
// base path is returned unchanged and left for safeRedirectTarget to reject or accept on its own
// terms.
func backendPath(path string) string {
	base := basePath()
	if base == "" || path == "" {
		return path
	}
	if path == base {
		return "/"
	}
	if rest, found := strings.CutPrefix(path, base+"/"); found {
		return "/" + rest
	}
	return path
}

// Copyright (c) 2023 SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The backend for Gatekeeper Policy Manager, a simple to use web-based UI for OPA Gatekeeper
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/spf13/viper"
)

// Holds everything the handlers need. They are methods on it rather than package-level functions
// so that nothing reaches for shared mutable state, and so tests can build one with fakes.
type server struct {
	k8s       *clientRegistry
	ssr       *ssrRenderer
	dashCache dashboardCache

	// The SubjectAccessReview checker, built on first use (#261). RBAC filtering only engages
	// against a single cluster, so there is one of these; the mutex covers the concurrent first
	// requests that would otherwise build it twice.
	authzMu sync.Mutex
	authz   *accessChecker
}

// registerSystemRoutes adds every route that is not a view. They are in one function, called by
// main and by the test that walks the router, because a route the RBAC guard does not recognise is
// served unchecked (#261) -- and a route added here would otherwise be outside that test.
func registerSystemRoutes(e *echo.Echo, auth *authenticator) {
	if auth != nil {
		e.GET(callbackPath, auth.callback)
		e.GET("/login", auth.login)
		e.GET("/logout", auth.logout)
	}
	e.GET("/health", getHealth)
}

// The single source of truth for the version string shown in logs and the UI.
const appVersion = "v2.0.0"

// Resolves the Kubernetes clients for the context named in the route, or the kubeconfig default
// when the route carries no :context.
func (s *server) clientsFor(c echo.Context) (*kubeClients, error) {
	return s.k8s.forContext(c.Param("context"))
}

// The value GPM 1.x shipped as its default. It is published in the source tree, so a session
// cookie signed with it can be forged by anyone; enabling authentication with it is refused.
const insecureDefaultSecretKey = "g8k1p3rp0l1c7m4n4g3r"

// How long a session lasts when GPM_SESSION_MAX_AGE is unset or not a positive number.
const defaultSessionMaxAge = 60 * 60 * 8

// Shortest GPM_SECRET_KEY accepted when authentication is on. The key is the entropy behind the
// session cookie's signature and encryption, so a trivially short one is forgeable.
const minSecretKeyLength = 16

// Reports why GPM_SECRET_KEY is unfit for enabling authentication, or "" when it is fine. The 1.x
// default is published in the source tree, so a cookie signed with it can be forged by anyone.
func secretKeyError(key string) string {
	switch {
	case key == insecureDefaultSecretKey:
		return "GPM_SECRET_KEY is still the published GPM 1.x default, so anyone can forge a session cookie."
	case len(key) < minSecretKeyLength:
		return fmt.Sprintf("GPM_SECRET_KEY is shorter than %d characters, too short to protect the session cookie.", minSecretKeyLength)
	}
	return ""
}

// Applies GPM_LOG_LEVEL. slog's own parsing takes DEBUG, INFO, WARN and ERROR in any case. An
// unusable value falls back to INFO and says so.
func setLogLevel(level *slog.LevelVar, configured string) {
	if err := level.UnmarshalText([]byte(configured)); err != nil {
		slog.Warn("the requested log level is not a valid option", "log_level", "INFO",
			"requested_level", configured)
		level.Set(slog.LevelInfo)
	}
}

// Declares every setting GPM reads, with its default and its GPM_-prefixed environment variable.
//
// Separate from main() so that a test can put viper in exactly the state a running GPM would see,
// instead of one assembled out of whatever the previous test left behind.
func bindSettings() {
	viper.SetEnvPrefix("gpm") // will be uppercased automatically
	// BindEnv only errors when called without a key, so the error is unreachable here.
	_ = viper.BindEnv("log_level")
	viper.SetDefault("log_level", "INFO")
	_ = viper.BindEnv("listen_address")
	viper.SetDefault("listen_address", ":8080")
	// Comma-separated event source components to show. Gatekeeper tags admission (webhook) events
	// with gatekeeper-webhook and audit events with gatekeeper-audit; show both by default.
	// RBAC-aligned views (#261). Off by default, and inert unless authentication is on and GPM
	// talks to a single cluster: without an OIDC identity there is nobody to authorize, and in
	// multi-cluster mode one identity would have to be valid in every aggregated cluster.
	_ = viper.BindEnv("rbac_filtering")
	viper.SetDefault("rbac_filtering", false)
	// Which ID-token claim carries the name the API server knows this person by, and the prefix the
	// cluster's --oidc-username-prefix puts in front of it. A mismatch denies everything, by design.
	_ = viper.BindEnv("rbac_username_claim")
	_ = viper.BindEnv("rbac_username_prefix")
	_ = viper.BindEnv("rbac_groups_claim")
	viper.SetDefault("rbac_groups_claim", "groups")
	_ = viper.BindEnv("rbac_groups_prefix")

	_ = viper.BindEnv("events_source")
	viper.SetDefault("events_source", "gatekeeper-webhook,gatekeeper-audit")
	// Which namespace to read events from. Empty means every namespace, which needs a cluster-wide
	// read on events; naming one lets the deployment get by with a Role in that namespace.
	_ = viper.BindEnv("events_namespace")
	viper.SetDefault("events_namespace", "")
	_ = viper.BindEnv("skip_tls_verify")
	viper.SetDefault("skip_tls_verify", false)
	// The subpath GPM is served from. The image sets this from the PUBLIC_URL the frontend was
	// built with, so it is normally not something anyone has to configure by hand.
	_ = viper.BindEnv("base_path")
	viper.SetDefault("base_path", "")

	// Authentication. Names match the Python backend's environment variables so that an existing
	// 1.x deployment can be pointed at this image without rewriting its configuration.
	_ = viper.BindEnv("auth_enabled")
	viper.SetDefault("auth_enabled", "Anonymous")
	_ = viper.BindEnv("secret_key")
	viper.SetDefault("secret_key", insecureDefaultSecretKey)
	_ = viper.BindEnv("preferred_url_scheme")
	viper.SetDefault("preferred_url_scheme", "http")
	_ = viper.BindEnv("session_max_age")
	viper.SetDefault("session_max_age", defaultSessionMaxAge)
	for _, k := range []string{
		"oidc_redirect_domain",
		"oidc_client_id",
		"oidc_client_secret",
		"oidc_scopes",
		"oidc_issuer",
		"oidc_authorization_endpoint",
		"oidc_token_endpoint",
		"oidc_jwks_uri",
		"oidc_introspection_endpoint",
		"oidc_userinfo_endpoint",
		"oidc_end_session_endpoint",
	} {
		_ = viper.BindEnv(k)
	}
}

func main() {
	bindSettings()

	// Initilize Echo HTTP server
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	p := prometheus.NewPrometheus("echo", nil)
	// /metrics is reachable without a session so Prometheus can scrape it, which also means any
	// client can influence the labels. Host comes straight from the request header, so leaving it
	// in lets one caller create unbounded series in a process that never forgets them.
	p.RequestCounterHostLabelMappingFunc = func(echo.Context) string { return "" }
	p.Use(e)

	// Response security headers. echo's defaults (X-Content-Type-Options nosniff, X-Frame-Options
	// SAMEORIGIN, the legacy XSS header) plus a Content Security Policy. No HSTS: that is the
	// operator's TLS decision, not GPM's to force.
	//
	// The policy keeps every script and stylesheet same-origin and blocks framing and plugins. Two
	// deliberate relaxations, both documented for the security review:
	//   - script-src 'unsafe-eval': Alpine.js evaluates its x- expressions with the Function
	//     constructor. Removing it needs Alpine's CSP build and rewriting every inline expression as
	//     a registered component method.
	//   - style-src 'unsafe-inline': the standalone printable violations report is self-contained
	//     with an inline <style>. No style value anywhere derives from user or cluster data (all
	//     data is HTML-escaped text content), so inline style is not an injection vector here.
	// img-src allows data: for the small SVG icons the stylesheet inlines as data URIs.
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:      "1; mode=block",
		ContentTypeNosniff: "nosniff",
		XFrameOptions:      "SAMEORIGIN",
		ContentSecurityPolicy: "default-src 'self'; " +
			"script-src 'self' 'unsafe-eval'; " +
			"style-src 'self' 'unsafe-inline'; " +
			"img-src 'self' data:; " +
			"font-src 'self'; " +
			"connect-src 'self'; " +
			"base-uri 'self'; " +
			"form-action 'self'; " +
			"frame-ancestors 'self'; " +
			"object-src 'none'",
	}))

	// Setup logging
	e.Use(middleware.Recover())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogURI:           true,
		LogStatus:        true,
		LogRemoteIP:      true,
		LogHost:          true,
		LogUserAgent:     true,
		LogError:         true,
		LogLatency:       true,
		LogContentLength: true,
		LogResponseSize:  true,
		LogMethod:        true,
		LogValuesFunc: func(c echo.Context, values middleware.RequestLoggerValues) error {
			slog.Info(
				"received request",
				"remote_ip", values.RemoteIP,
				"host", values.Host,
				"method", values.Method,
				"uri", values.URI,
				"user_agent", values.UserAgent,
				"status", values.Status,
				"error", values.Error,
				"latency", values.Latency,
				"latency_human", values.Latency.Microseconds(),
				"bytes_in", values.ContentLength,
				"bytes_out", values.ResponseSize,
			)
			return nil
		},
	}))

	programLevel := new(slog.LevelVar)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: programLevel}))
	slog.SetDefault(logger)

	slog.Info("starting Gatekeeper Policy Manager", "version", appVersion)
	setLogLevel(programLevel, viper.GetString("log_level"))

	// Renders the HTML violations report at /constraints?report=html.
	e.Renderer = newRenderer()

	// Authentication. When it is off, no session or auth middleware is installed at all, so the
	// unauthenticated path stays exactly as it was.
	var auth *authenticator
	if authEnabled() {
		if msg := secretKeyError(viper.GetString("secret_key")); msg != "" {
			slog.Error(msg, "action", "set GPM_SECRET_KEY to a long random string before enabling authentication")
			os.Exit(1)
		}

		// Discovery talks to the identity provider before the server starts listening, so a slow or
		// unreachable provider would otherwise hang the pod with no health endpoint to report it.
		authCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var authErr error
		auth, authErr = newAuthenticator(authCtx)
		if authErr != nil {
			slog.Error("OIDC authentication could not be configured", "error", authErr)
			os.Exit(1)
		}
		e.Use(session.Middleware(newSessionStore()))
		e.Use(auth.middleware())
	} else {
		slog.Warn("authentication is disabled, GPM is readable by anyone who can reach it")
	}

	registry, err := newClientRegistry()
	if err != nil {
		slog.Error("Kubernetes client initialization failed", "error", err)
		os.Exit(1)
	}
	s := &server{k8s: registry, ssr: newSSRRenderer()}

	// Registered after the session and auth middleware, so the identity is there to read. Inert
	// unless GPM_RBAC_FILTERING is on, and it decides what a request may reach rather than only what
	// the navbar shows (#261). Echo applies Use middleware to every request, so registering it here
	// -- after the routes it guards are declared below -- still covers them.
	e.Use(s.rbacMiddleware())
	// Refuse an unsupported combination rather than quietly serving everything (#261).
	if err := s.checkConfig(); err != nil {
		slog.Error("GPM cannot start with this configuration", "error", err)
		os.Exit(1)
	}

	// The server-rendered UI: every view at its real path, plus the embedded static assets. See ssr.go.
	registerViews(e, s)

	// Global error handler. For an HTML request it renders the server-side pages (404 -> notfound,
	// anything else -> the error page); for the /api/* JSON endpoints it keeps echo's JSON default.
	// There is no SPA fallback anymore, so an unmatched path reaches here as a 404.
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		if isAPIPath(c.Request().URL.Path) {
			e.DefaultHTTPErrorHandler(err, c)
			return
		}
		code := http.StatusInternalServerError
		if he, ok := err.(*echo.HTTPError); ok {
			code = he.Code
		}
		if code == http.StatusNotFound {
			_ = s.renderNotFound(c)
			return
		}
		_ = s.renderError(c, code, ssrErrorView{
			Message: "Something went wrong",
			Action:  "Try again, and if the problem continues check the GPM logs.",
		})
	}

	// Routes configuration

	if auth != nil {
		// The local logout path renders the SSR "signed out" page; wire it now that s exists.
		auth.renderLoggedOut = s.renderLoggedOut
		auth.renderError = s.renderError
	}

	// One rewrite instead of registering every route twice. Pre, so it runs before routing.
	e.Pre(middleware.RemoveTrailingSlash())

	registerSystemRoutes(e, auth)

	address := viper.GetString("listen_address")

	// echo's default server sets no timeouts, so a slow or idle client can hold a connection open
	// forever. ReadHeaderTimeout in particular bounds a slowloris-style stall.
	e.Server.ReadHeaderTimeout = 10 * time.Second
	e.Server.ReadTimeout = 30 * time.Second
	e.Server.WriteTimeout = 60 * time.Second
	e.Server.IdleTimeout = 120 * time.Second

	slog.Info("starting HTTP server", "address", address)
	slog.Error("starting HTTP server failed", "error", e.Start(address), "address", address)
	os.Exit(1)
}

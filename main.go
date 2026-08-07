// Copyright (c) 2023 SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The backend for Gatekeeper Policy Manager, a simple to use web-based UI for OPA Gatekeeper
package main

import (
	"context"
	"net/http"
	"os"
	"text/template"
	"time"

	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd/api"

	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/spf13/viper"

	"golang.org/x/exp/slog"
)

// Holds everything the handlers need. They are methods on it rather than package-level functions
// so that nothing reaches for shared mutable state, and so tests can build one with fakes.
type server struct {
	k8s *clientRegistry
}

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
	_ = viper.BindEnv("events_source")
	viper.SetDefault("events_source", "gatekeeper-webhook")
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

	slog.Info("starting Gatekeeper Policy Manager", "version", "v2.0.0-alpha1")
	setLogLevel(programLevel, viper.GetString("log_level"))

	// We compile the HTML templates here
	// This is used later to render templates in the routes (i.e. to render the HTML report in the `/constraints/?report=html` route).
	e.Renderer = &Template{
		templates: template.Must(template.ParseGlob("templates/*.html.gotpl")),
	}

	// CORS configuration for frontend development purposes
	if os.Getenv("APP_ENV") == "development" {
		origins := []string{"http://localhost:3000", "http://localhost:3001"}
		headers := []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept}
		slog.Warn("running in development mode, allowing CORS from other origins", "origins", origins, "headers", headers)
		e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: origins,
			AllowHeaders: headers,
		}))
	}

	// Authentication. When it is off, no session or auth middleware is installed at all, so the
	// unauthenticated path stays exactly as it was.
	var auth *authenticator
	if authEnabled() {
		if viper.GetString("secret_key") == insecureDefaultSecretKey {
			slog.Error("GPM_SECRET_KEY is still the default value from GPM 1.x. " +
				"It is published in the source tree, so anyone could forge a session cookie. " +
				"Set it to a long random string before enabling authentication.")
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
	s := &server{k8s: registry}

	// Routes configuration

	if auth != nil {
		e.GET(callbackPath, auth.callback)
		e.GET("/login", auth.login)
		e.GET("/logout", auth.logout)
	}

	// One rewrite instead of registering every route twice. Pre, so it runs before routing.
	e.Pre(middleware.RemoveTrailingSlash())

	e.Static("/static/", "./static-content/static")
	// Fallback route for all non-matching URLs.
	// We need to serve index.html for react routing to work. See:
	// https://create-react-app.dev/docs/deployment#serving-apps-with-client-side-routing.
	// We could avoid this by serving the frontend from another process/container instead of from the backend
	e.GET("/*", serveIndex)

	e.GET("/health", getHealth)

	e.GET("/api/v1/auth", getAuth)

	e.GET("/api/v1/contexts", s.getContexts)

	e.GET("/api/v1/configs", s.getConfigs)
	e.GET("/api/v1/configs/:context", s.getConfigs)

	e.GET("/api/v1/constrainttemplates", s.getConstraintTemplates)
	e.GET("/api/v1/constrainttemplates/:context", s.getConstraintTemplates)

	e.GET("/api/v1/constraints", s.getConstraints)
	e.GET("/api/v1/constraints/:context", s.getConstraints)

	e.GET("/api/v1/mutations", s.getMutations)
	e.GET("/api/v1/mutations/:context", s.getMutations)

	e.GET("/api/v1/events", s.getEvents)
	e.GET("/api/v1/events/:context", s.getEvents)

	// Returns an object with the list of available contets and the currently selected context
	e.GET("/api/v2/contexts/", func(c echo.Context) error {
		type v2Answer struct {
			Current  string                  `json:"currentContext"`
			Contexts map[string]*api.Context `json:"contexts"`
		}

		contexts, current := s.k8s.contexts()
		return c.JSON(http.StatusOK, v2Answer{current, contexts})
	})

	address := viper.GetString("listen_address")

	slog.Info("starting HTTP server", "address", address)
	slog.Error("starting HTTP server failed", "error", e.Start(address), "address", address)
	os.Exit(1)
}

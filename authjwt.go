// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// JWT authentication for Gatekeeper Policy Manager. An authenticating proxy in front of GPM decides
// who the user is, and proves it with a signed assertion. Pomerium calls that header
// X-Pomerium-Jwt-Assertion; oauth2-proxy sends the provider's ID token in Authorization.
//
// GPM verifies the signature against the proxy's JWKS on every request, and refuses a request that
// carries no valid assertion. That is what makes the mode safe when the GPM Service is reachable
// without going through the proxy, which it is on every cluster: a plain X-Forwarded-User header
// would be forged in one line, and the network is not something GPM can check.
//
// There is no login flow and no session. The assertion arrives on each request, or the request is
// refused, so this file holds no cookie handling and GPM_SECRET_KEY is not read in this mode.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
)

// The header Pomerium uses. Configurable, because oauth2-proxy and other proxies name it otherwise.
const defaultJWTHeaderName = "X-Pomerium-Jwt-Assertion"

// Where the middleware leaves the verified principal for the rest of the request. identityFor reads
// it; nothing else should.
const jwtPrincipalKey = "gpm_jwt_principal"

// How often one repeated claim misconfiguration may be logged. The claims are decoded per request,
// so an unrationed warning would bury every other line in the log.
const jwtWarnInterval = 30 * time.Second

// jwtPrincipal is who the assertion said is asking. The RBAC fields hold the raw claim values,
// without the cluster's prefixes: prefixedIdentity applies those on read, so a corrected prefix
// takes effect when GPM restarts.
type jwtPrincipal struct {
	// Both empty unless GPM_RBAC_FILTERING is on: building them costs a second claim decode per
	// request. The struct is still set on every request, because its presence is what tells
	// identityFor that a verified assertion named this person.
	rbacUser   string
	rbacGroups []string
}

type jwtAuthenticator struct {
	header   string
	verifier *oidc.IDTokenVerifier
	// Renders the refusal page, wired by main once the SSR renderer exists. The /api/* answer stays
	// JSON, as it does in OIDC mode.
	renderError func(c echo.Context, status int, e ssrErrorView) error

	// Rations the repeated warnings out of identityFromClaims.
	warnMu   sync.Mutex
	warnNext map[string]time.Time
	now      func() time.Time // swapped in tests
}

// newJWTAuthenticator validates the configuration and builds the verifier. It makes no network call:
// the keys are fetched on the first request that carries an assertion, and go-oidc caches and
// rotates them from then on.
func newJWTAuthenticator(ctx context.Context) (*jwtAuthenticator, error) {
	jwks := viper.GetString("jwt_jwk_set_url")
	if jwks == "" {
		return nil, errors.New("GPM_JWT_JWK_SET_URL must be set when GPM_AUTH_ENABLED=JWT. " +
			"For Pomerium it is https://<pomerium-host>/.well-known/pomerium/jwks.json")
	}
	// https only. These keys decide who every user is, so a plaintext fetch lets anyone on the path
	// serve their own key set and then mint an identity of their choosing, groups included. An
	// in-cluster Service URL is no exception: it is still on-path for a compromised node.
	u, err := url.Parse(jwks)
	if err != nil || !u.IsAbs() || u.Scheme != "https" {
		return nil, fmt.Errorf("GPM_JWT_JWK_SET_URL must be an absolute https URL, but is %q", jwks)
	}

	// Required, not optional. A proxy signs every route it serves with one key and names the route
	// in the aud claim. Without this check GPM accepts an assertion minted for a different route,
	// which lets a person the proxy allows elsewhere in as themselves, and turns the proxy's
	// per-route policy into "anybody the proxy knows".
	audience := viper.GetString("jwt_audience")
	if audience == "" {
		return nil, errors.New("GPM_JWT_AUDIENCE must be set when GPM_AUTH_ENABLED=JWT: without it " +
			"GPM accepts an assertion the proxy minted for any other route it serves, which " +
			"bypasses that route's policy. Set it to the host GPM is served on, for example " +
			"gpm.example.com")
	}

	// Optional. Pomerium writes either the bare host or https://<host>/ into iss, depending on its
	// jwt_issuer_format, so requiring it here would break a deployment for a setting GPM cannot see.
	issuer := viper.GetString("jwt_issuer")

	header := viper.GetString("jwt_header_name")
	if header == "" {
		header = defaultJWTHeaderName
	}

	// Background, not the startup context: go-oidc reads the context as configuration and drops its
	// cancellation, but passing a context with a deadline still reads as though key refresh stopped
	// at startup plus 30 seconds. Nothing here is fetched now.
	keys := oidc.NewRemoteKeySet(context.WithoutCancel(ctx), jwks)

	verifier := oidc.NewVerifier(issuer, keys, &oidc.Config{
		ClientID:        audience,
		SkipIssuerCheck: issuer == "",
		// go-oidc defaults to RS256 alone. Pomerium signs with ES256, and an assertion in an
		// unlisted algorithm is rejected as malformed rather than as unverified, which reads as a
		// broken proxy.
		SupportedSigningAlgs: []string{oidc.RS256, oidc.ES256},
	})

	slog.Info("JWT authentication enabled", "header", header, "jwks", jwks, "audience", audience,
		"issuer_checked", issuer != "")
	if issuer == "" {
		// Safe behind one proxy, whose key set serves only its own routes, because the audience
		// already names this route. Not safe behind a shared identity provider, where one key set
		// and one client ID cover many tenants and the issuer is what separates them.
		slog.Warn("GPM_JWT_ISSUER is not set, so GPM accepts any issuer whose key is in the key set. "+
			"Set it when the key set belongs to an identity provider rather than to the proxy in "+
			"front of GPM", "jwks", jwks)
	}
	if logout := viper.GetString("jwt_logout_url"); logout == "" {
		slog.Info("GPM_JWT_LOGOUT_URL is not set, so GPM shows no Log out control. " +
			"GPM holds no session in this mode, so only the proxy can end one")
	}

	return &jwtAuthenticator{
		header:   header,
		verifier: verifier,
		warnNext: map[string]time.Time{},
		now:      time.Now,
	}, nil
}

// assertion pulls the raw JWT out of the configured header. A Bearer prefix is stripped, so
// GPM_JWT_HEADER_NAME=Authorization works with oauth2-proxy's --set-authorization-header.
func (a *jwtAuthenticator) assertion(c echo.Context) string {
	raw := strings.TrimSpace(c.Request().Header.Get(a.header))
	if after, found := strings.CutPrefix(raw, "Bearer "); found {
		return strings.TrimSpace(after)
	}
	return raw
}

// shouldWarn reports whether this message may be logged now, and starts a new quiet window when it
// may. Split from warn so the rationing can be tested without reading the log.
func (a *jwtAuthenticator) shouldWarn(msg string) bool {
	a.warnMu.Lock()
	defer a.warnMu.Unlock()
	now := a.now()
	if now.Before(a.warnNext[msg]) {
		return false
	}
	a.warnNext[msg] = now.Add(jwtWarnInterval)
	return true
}

// warn rations one repeated message. identityFromClaims reports a misconfigured claim, and it runs
// on every request here rather than once per login.
func (a *jwtAuthenticator) warn(msg string, args ...any) {
	if a.shouldWarn(msg) {
		slog.Warn(msg, args...)
	}
}

// Gate every non-public route on a valid assertion.
func (a *jwtAuthenticator) middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if isPublicPath(c.Request().URL.Path) {
				return next(c)
			}

			raw := a.assertion(c)
			if raw == "" {
				// Almost always a request that did not come through the proxy, so the advice names
				// that rather than the header: somebody reading this reached GPM directly.
				return a.refuse(c, "GPM did not receive an identity for this request.",
					"Open GPM through its usual address, so that the authenticating proxy can identify you.",
					fmt.Sprintf("No %s header was present.", a.header))
			}

			token, err := a.verifier.Verify(c.Request().Context(), raw)
			if err != nil {
				// Logged in full, deliberately not returned: the detail names the audience and the
				// issuer GPM expects, and this page is reachable by anyone who can connect.
				slog.Warn("rejecting an identity assertion", "error", err, "header", a.header)
				return a.refuse(c, "GPM could not verify the identity for this request.",
					"Reload the page. If it still fails, contact a cluster administrator.",
					"The assertion from the authenticating proxy did not pass verification.")
			}

			c.Set(jwtPrincipalKey, a.principal(token))
			return next(c)
		}
	}
}

// principal reads the claims GPM needs out of a verified assertion.
func (a *jwtAuthenticator) principal(token *oidc.IDToken) *jwtPrincipal {
	var named struct {
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
	}
	if err := token.Claims(&named); err != nil {
		slog.Debug("could not decode the assertion claims, falling back to the subject", "error", err)
	}
	// The same order the OIDC callback uses, so a person's name does not change with the mode.
	user := firstNonEmpty(named.PreferredUsername, named.Email, named.Name, token.Subject)

	p := &jwtPrincipal{}
	if !rbacFilteringEnabled() {
		return p
	}
	var all map[string]any
	if err := token.Claims(&all); err != nil {
		// No identity rather than a guessed one: an unreadable payload must not authorize anybody.
		a.warn("could not decode the assertion claims for the RBAC identity, so this person sees "+
			"only the scoped view", "error", err)
		return p
	}
	p.rbacUser, p.rbacGroups = identityFromClaims(all, user, a.warn)
	return p
}

// refuse answers a request GPM could not identify. A browser gets the error page, because this is
// ordinary navigation rather than a redirect anywhere useful: GPM has no login of its own to send
// them to, and the proxy is what has to identify them.
func (a *jwtAuthenticator) refuse(c echo.Context, message, action, description string) error {
	if isAPIPath(c.Request().URL.Path) {
		return c.JSON(http.StatusUnauthorized, ErrorAnswer{
			ErrorMessage: message,
			Action:       action,
			Description:  description,
		})
	}
	return a.renderError(c, http.StatusUnauthorized, ssrErrorView{
		Heading:     "Not signed in",
		Message:     message,
		Action:      action,
		Description: description,
	})
}

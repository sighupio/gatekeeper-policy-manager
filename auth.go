// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// OIDC authentication for Gatekeeper Policy Manager. Configuration mirrors the Python backend's
// environment variables so that 1.x deployments keep working unchanged.
package main

import (
	"context"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
	"golang.org/x/exp/slog"
	"golang.org/x/oauth2"
)

const (
	sessionName = "gpm-session"
	// Path the identity provider redirects back to. Kept identical to the Python backend so an
	// existing client registration keeps working.
	callbackPath = "/oidc-auth"

	sessionKeyUser        = "user"
	sessionKeyState       = "state"
	sessionKeyNonce       = "nonce"
	sessionKeyDestination = "destination"
	sessionKeyVerifier    = "pkce_verifier"

	// Set for one hop when the callback restarts a login, so a second failure in a row stops
	// instead of bouncing the browser between GPM and the provider forever.
	retryCookieName = "gpm-login-retry"
)

// Holds everything needed to run the OIDC flow. Nil when authentication is disabled.
type authenticator struct {
	oauth2   oauth2.Config
	verifier *oidc.IDTokenVerifier
	// Empty when the provider does not advertise one, in which case logout is local only.
	endSessionEndpoint string
	// Renders the "signed out" page at the end of the local logout path. main wires this to the
	// SSR renderer; auth.go stays free of the template layer.
	renderLoggedOut func(echo.Context) error
}

// Reports whether the operator asked for OIDC. Anything other than "OIDC" (including the Python
// backend's "Anonymous") leaves GPM unauthenticated.
func authEnabled() bool {
	return strings.EqualFold(viper.GetString("auth_enabled"), "OIDC")
}

// Builds the provider configuration, either from discovery on the issuer or from the endpoints
// given explicitly. Explicit endpoints win, matching the Python backend: setting any one of them
// disables discovery, so all of them have to be provided together.
func newAuthenticator(ctx context.Context) (*authenticator, error) {
	issuer := viper.GetString("oidc_issuer")
	clientID := viper.GetString("oidc_client_id")
	redirectDomain := viper.GetString("oidc_redirect_domain")

	if redirectDomain == "" {
		return nil, errors.New("GPM_OIDC_REDIRECT_DOMAIN must be set when authentication is enabled")
	}
	if clientID == "" {
		return nil, errors.New("GPM_OIDC_CLIENT_ID must be set when authentication is enabled")
	}

	// GPM_OIDC_REDIRECT_DOMAIN is the scheme and host only; the subpath comes from GPM_BASE_PATH,
	// so a subpath deployment does not need it spelled out in two places.
	redirectURL, err := url.JoinPath(redirectDomain, browserPath(callbackPath))
	if err != nil {
		return nil, fmt.Errorf("building the redirect URL from GPM_OIDC_REDIRECT_DOMAIN failed: %w", err)
	}

	manual := map[string]string{
		"GPM_OIDC_AUTHORIZATION_ENDPOINT": viper.GetString("oidc_authorization_endpoint"),
		"GPM_OIDC_TOKEN_ENDPOINT":         viper.GetString("oidc_token_endpoint"),
		"GPM_OIDC_JWKS_URI":               viper.GetString("oidc_jwks_uri"),
	}
	anyManual := false
	for _, v := range manual {
		if v != "" {
			anyManual = true
		}
	}

	var (
		endpoint           oauth2.Endpoint
		verifier           *oidc.IDTokenVerifier
		endSessionEndpoint = viper.GetString("oidc_end_session_endpoint")
	)

	if anyManual {
		var missing []string
		for name, v := range manual {
			if v == "" {
				missing = append(missing, name)
			}
		}
		// The issuer is still required here: it is what the ID token's iss claim is checked
		// against, and go-oidc compares it literally, so an empty one rejects every token.
		if issuer == "" {
			missing = append(missing, "GPM_OIDC_ISSUER")
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			return nil, fmt.Errorf(
				"the OIDC endpoints were configured manually, so discovery is disabled and these must be set too: %s",
				strings.Join(missing, ", "))
		}

		slog.Info("configuring the OIDC provider from the endpoints given, discovery is disabled")
		endpoint = oauth2.Endpoint{
			AuthURL:  manual["GPM_OIDC_AUTHORIZATION_ENDPOINT"],
			TokenURL: manual["GPM_OIDC_TOKEN_ENDPOINT"],
		}
		keySet := oidc.NewRemoteKeySet(ctx, manual["GPM_OIDC_JWKS_URI"])
		verifier = oidc.NewVerifier(issuer, keySet, &oidc.Config{ClientID: clientID})
	} else {
		if issuer == "" {
			return nil, errors.New("GPM_OIDC_ISSUER must be set when authentication is enabled")
		}
		slog.Info("discovering the OIDC provider configuration", "issuer", issuer)
		provider, err := oidc.NewProvider(ctx, issuer)
		if err != nil {
			return nil, fmt.Errorf("discovering the OIDC provider at %q failed: %w", issuer, err)
		}
		endpoint = provider.Endpoint()
		verifier = provider.Verifier(&oidc.Config{ClientID: clientID})

		if endSessionEndpoint == "" {
			// Not part of the core discovery struct, so read it out of the raw document.
			var claims struct {
				EndSessionEndpoint string `json:"end_session_endpoint"`
			}
			if err := provider.Claims(&claims); err == nil {
				endSessionEndpoint = claims.EndSessionEndpoint
			}
		}
	}

	slog.Info("OIDC authentication enabled", "client_id", clientID, "redirect_url", redirectURL)

	return &authenticator{
		oauth2: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: viper.GetString("oidc_client_secret"),
			Endpoint:     endpoint,
			RedirectURL:  redirectURL,
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
		verifier:           verifier,
		endSessionEndpoint: endSessionEndpoint,
	}, nil
}

// Derives the pair of keys the cookie store needs from GPM_SECRET_KEY: one to sign the cookie, one
// to encrypt it. They must differ, and GPM_SECRET_KEY is a single string of whatever length the
// operator chose, so HKDF expands it into 96 bytes and the halves are taken from that. 32 bytes for
// the block key selects AES-256.
//
// No salt, deliberately: the keys have to come out the same on every replica and after every
// restart, or a session stops decoding the moment a request reaches a different pod.
//
// Changing the label or the lengths invalidates every existing session, so treat them as part of
// the cookie format.
func sessionKeys(secret string) (hashKey, blockKey []byte) {
	const info = "gpm session cookie keys v1"

	k, err := hkdf.Key(sha256.New, []byte(secret), nil, info, 96)
	if err != nil {
		// Only for a length HKDF cannot produce, and 96 is a constant well inside the limit.
		panic(fmt.Sprintf("deriving the session keys failed: %v", err))
	}
	return k[:64], k[64:]
}

// Cookie store for the session. GPM_SECRET_KEY is the same variable the Python backend used, but
// the key material and the cookie format are not, so a 1.x cookie does not decode here.
//
// A cookie that will not decode means the user is not logged in, and every place that reads one
// treats it that way. Rotating GPM_SECRET_KEY invalidates them all at once, so answering with an
// error instead would let one operator action strand every user for a full GPM_SESSION_MAX_AGE,
// with no server-side repair.
func newSessionStore() sessions.Store {
	// GetInt yields 0 for an empty or unparseable value, and 0 tells securecookie to skip the
	// timestamp check entirely — a typo would hand out sessions that never expire.
	maxAge := viper.GetInt("session_max_age")
	if maxAge <= 0 {
		slog.Warn("GPM_SESSION_MAX_AGE is not a positive number of seconds, falling back to the default",
			"configured", viper.GetString("session_max_age"), "using", defaultSessionMaxAge)
		maxAge = defaultSessionMaxAge
	}

	// Two keys: without the block key the value base64-decodes to readable gob -- the username, and
	// during the login leg the state, the nonce and the PKCE verifier.
	hashKey, blockKey := sessionKeys(viper.GetString("secret_key"))
	store := sessions.NewCookieStore(hashKey, blockKey)
	store.Options = &sessions.Options{
		// Scoped to the subpath, not the whole origin. A subpath is normally chosen because
		// something else already answers on that host, and "/" would hand that application a
		// session it can replay against GPM. HttpOnly does not help: the leak is to the other
		// server, not to script.
		Path:     cookiePath(),
		HttpOnly: true,
		// Lax rather than Strict: the browser arrives back on the callback from the provider as a
		// top-level navigation, and Strict would drop the cookie on that hop.
		SameSite: http.SameSiteLaxMode,
		// Trust the redirect domain over the scheme hint: the domain is required, and an operator
		// who serves GPM over TLS but leaves GPM_PREFERRED_URL_SCHEME at its default would
		// otherwise ship a session cookie without Secure.
		Secure: strings.EqualFold(viper.GetString("preferred_url_scheme"), "https") ||
			strings.HasPrefix(strings.ToLower(viper.GetString("oidc_redirect_domain")), "https://"),
		MaxAge: maxAge,
	}
	// Setting Options alone only changes the cookie attribute the browser sees. The securecookie
	// codecs carry their own max age, which is the *server side* check, and replacing Options wholesale
	// leaves them at the 30-day default. This is what actually expires a session.
	store.MaxAge(maxAge)

	return store
}

// Moves a still-valid session off the origin root, for a deployment that has just been given a
// base path. Its cookie decodes perfectly -- the keys do not depend on the base path -- so the
// request is authenticated and none of the login paths run. Without this the session token keeps
// going to every other application on the host for a full GPM_SESSION_MAX_AGE, which is the leak
// the scoping exists to close.
//
// Save first, expire second. Expiring the root cookie without writing the scoped one would log the
// user out, because the root cookie may be the only session they have.
func (a *authenticator) migrateSessionOffTheRoot(c echo.Context, sess *sessions.Session) {
	if basePath() == "" {
		return
	}
	// Nothing to move once the browser is sending the scoped cookie. Options came from the store,
	// so this is the scoped path; a request that still carries only the root cookie has no way to
	// say so, which is why the save is unconditional and idempotent.
	sess.Options.MaxAge = viper.GetInt("session_max_age")
	if sess.Options.MaxAge <= 0 {
		sess.Options.MaxAge = defaultSessionMaxAge
	}
	if err := sess.Save(c.Request(), c.Response()); err != nil {
		slog.Error("could not rewrite the session onto the base path", "error", err)
		return
	}
	a.clearLegacyRootCookie(c)
}

// Writes the one-hop marker the callback uses to avoid restarting a login twice. A negative maxAge
// deletes it. Scoped like the session cookie so it goes away with the rest.
func (a *authenticator) setRetryMarker(c echo.Context, maxAge int) {
	value := "1"
	if maxAge < 0 {
		value = ""
	}
	http.SetCookie(c.Response(), &http.Cookie{
		Name:     retryCookieName,
		Value:    value,
		Path:     cookiePath(),
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// Expires a session cookie scoped to the origin root. GPM cannot overwrite one -- deletion has to
// match the path, and GPM now writes to the subpath -- and until it expires the browser keeps
// sending it to every other application on the host. One appears whenever a deployment moves to a
// subpath: GPM_BASE_PATH set on a running instance, or a PUBLIC_URL image replacing one without.
//
// Subpath only. At the root, that cookie is the live session.
func (a *authenticator) clearLegacyRootCookie(c echo.Context) {
	if basePath() == "" {
		return
	}
	http.SetCookie(c.Response(), &http.Cookie{
		Name:     sessionName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}

func randomString() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Reduces a requested redirect target to something safe to hand to the browser, falling back to
// the root. Only same-site absolute paths survive.
//
// Prefix checks alone are not enough. Browsers strip tab, carriage return and line feed from a URL
// before resolving it, so "/\t/evil.com" reaches the browser looking relative and is resolved as
// the protocol-relative "//evil.com" — an off-site redirect carrying the trust of a real login.
func safeRedirectTarget(target string) string {
	if strings.ContainsAny(target, "\t\r\n") {
		return "/"
	}

	u, err := url.Parse(target)
	if err != nil || u.IsAbs() || u.Host != "" || u.Opaque != "" {
		return "/"
	}
	if !strings.HasPrefix(u.Path, "/") ||
		strings.HasPrefix(u.Path, "//") ||
		strings.HasPrefix(u.Path, `/\`) {
		return "/"
	}
	return u.String()
}

// True for the handful of paths that must stay reachable without a session: the health probe, the
// endpoint the frontend calls to discover whether auth is on at all, the logout page, and the
// static assets that make up the login-time UI.
//
// Both the raw path and its cleaned form have to pass. Echo routes on the raw one, so checking
// only the cleaned form opens "/api/v1/../../static/x", and checking only the raw form opens
// "/static/../api/v1/constraints".
func isPublicPath(p string) bool {
	return isAllowlistedPath(p) && isAllowlistedPath(path.Clean(p))
}

func isAllowlistedPath(p string) bool {
	switch {
	case p == "/health", p == "/health/":
		return true
	case p == callbackPath, p == "/logout", p == "/login":
		return true
	// Prometheus scrapes this with no session. It carries request counters only, no policy data,
	// and turning authentication on should not silently stop metrics collection.
	case p == "/metrics":
		return true
	case strings.HasPrefix(p, "/static/"):
		return true
	}
	return false
}

// Requests under /api are answered by fetch() in the frontend, where a cross-origin redirect to
// the identity provider surfaces as an opaque network error. Those get a readable 401 instead;
// only real navigation is redirected.
func isAPIPath(p string) bool {
	return strings.HasPrefix(p, "/api/")
}

// Gate every non-public route on a session, starting the OIDC flow when there is not one.
func (a *authenticator) middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			requestPath := c.Request().URL.Path
			if isPublicPath(requestPath) {
				return next(c)
			}

			sess, err := session.Get(sessionName, c)
			if err == nil && sess.Values[sessionKeyUser] != nil {
				a.migrateSessionOffTheRoot(c, sess)
				return next(c)
			}

			if isAPIPath(requestPath) {
				// The browser keeps sending a root-scoped cookie to every other application on
				// the host until something expires it, and an API call is usually the first
				// request the frontend makes.
				a.clearLegacyRootCookie(c)
				return c.JSON(http.StatusUnauthorized, ErrorAnswer{
					ErrorMessage: "Your session expired, or you are not logged in.",
					Action:       "Sign in again to continue.",
					Description:  "GPM found no valid OIDC session for this request.",
					LoginURL:     browserPath("/login"),
				})
			}
			return a.startLogin(c, c.Request().URL.RequestURI())
		}
	}
}

// Starts the login flow explicitly. Having a route for this gives the frontend somewhere to send
// the user after an API call comes back 401, instead of asking them to reload the page.
// An optional ?next= says where to land afterwards.
func (a *authenticator) login(c echo.Context) error {
	next := safeRedirectTarget(backendPath(c.QueryParam("next")))

	// Already signed in: nothing to do but go where they were headed.
	if sess, err := session.Get(sessionName, c); err == nil && sess.Values[sessionKeyUser] != nil {
		return c.Redirect(http.StatusFound, browserPath(next))
	}
	return a.startLogin(c, next)
}

// Sends the browser to the identity provider, remembering where it was headed.
func (a *authenticator) startLogin(c echo.Context, destination string) error {
	state, err := randomString()
	if err != nil {
		return fmt.Errorf("generating the OIDC state failed: %w", err)
	}
	nonce, err := randomString()
	if err != nil {
		return fmt.Errorf("generating the OIDC nonce failed: %w", err)
	}

	destination = safeRedirectTarget(destination)

	// PKCE. GPM_OIDC_CLIENT_SECRET is optional, so a public client would otherwise have nothing
	// but TLS and the registered redirect URI protecting the authorization code.
	verifier := oauth2.GenerateVerifier()

	// See newSessionStore: an undecodable cookie means logged out. gorilla hands back a usable
	// empty session alongside the error; reset it in case a partial decode left anything behind.
	sess, err := session.Get(sessionName, c)
	if sess == nil {
		// Only when the session middleware is not installed, which is a wiring mistake rather than
		// anything a request can cause. Nothing can be stored, so the flow cannot continue.
		return fmt.Errorf("the session store is not available: %w", err)
	}
	if err != nil {
		slog.Debug("could not read the existing session, starting a fresh one", "error", err)
		sess.Values = map[interface{}]interface{}{}
	}
	sess.Values[sessionKeyState] = state
	sess.Values[sessionKeyNonce] = nonce
	sess.Values[sessionKeyVerifier] = verifier
	sess.Values[sessionKeyDestination] = destination
	if err := sess.Save(c.Request(), c.Response()); err != nil {
		return fmt.Errorf("saving the OIDC session failed: %w", err)
	}

	a.clearLegacyRootCookie(c)

	slog.Debug("starting the OIDC login flow", "destination", destination)
	return c.Redirect(http.StatusFound,
		a.oauth2.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)))
}

// Handles the redirect back from the identity provider.
func (a *authenticator) callback(c echo.Context) error {
	ctx := c.Request().Context()

	// A top-level navigation, so raw JSON in the address bar is the wrong answer. Without the state,
	// the nonce and the verifier this login cannot finish, so start a clean one.
	sess, err := session.Get(sessionName, c)
	if sess == nil {
		slog.Error("the session store is not available while completing the login", "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorAnswer{
			ErrorMessage: "GPM could not read the session during login.",
			Action:       "Log in again.",
			Description:  err.Error(),
		})
	}
	if err != nil {
		// One restart only. Something can shadow this cookie with one GPM cannot overwrite -- a
		// more specific path, or a Domain attribute -- and then every restart lands back here and
		// asks the provider for another authorization. Bounded, the common case still recovers in
		// one hop: a key rotation between the start of a login and its callback.
		if _, seen := c.Cookie(retryCookieName); seen == nil {
			slog.Warn("the session is still unreadable after restarting the login, giving up",
				"error", err)
			a.setRetryMarker(c, -1)
			return c.JSON(http.StatusUnauthorized, ErrorAnswer{
				ErrorMessage: "GPM could not read the session during login.",
				Action:       "Make sure that your browser accepts cookies from this site, then log in again.",
				Description:  err.Error(),
				LoginURL:     browserPath("/login"),
			})
		}
		slog.Debug("the session could not be read on the callback, restarting the login", "error", err)
		a.setRetryMarker(c, 300)
		return a.startLogin(c, "/")
	}

	// Got a readable session, so whatever the marker was for is over.
	if _, seen := c.Cookie(retryCookieName); seen == nil {
		a.setRetryMarker(c, -1)
	}

	if errParam := c.QueryParam("error"); errParam != "" {
		slog.Error("the identity provider returned an error", "error", errParam,
			"description", c.QueryParam("error_description"))
		return c.JSON(http.StatusUnauthorized, ErrorAnswer{
			ErrorMessage: fmt.Sprintf("OIDC error: %s", errParam),
			Action:       "Something is wrong with your OIDC session. Log out, then log in again.",
			Description:  c.QueryParam("error_description"),
		})
	}

	// Guards against CSRF: the state has to match the one stored before the redirect.
	want, _ := sess.Values[sessionKeyState].(string)
	if want == "" || c.QueryParam("state") != want {
		slog.Warn("the OIDC state did not match, rejecting the callback")
		return c.JSON(http.StatusUnauthorized, ErrorAnswer{
			ErrorMessage: "GPM could not verify the login.",
			Action:       "Log in again from the start.",
			Description:  "The OIDC state parameter did not match the one stored in the session.",
		})
	}

	verifier, _ := sess.Values[sessionKeyVerifier].(string)
	token, err := a.oauth2.Exchange(ctx, c.QueryParam("code"), oauth2.VerifierOption(verifier))
	if err != nil {
		// Logged in full, deliberately not returned: /login is public, so anyone could start a
		// flow and replay a bad code to read whatever the token endpoint said back. oauth2's own
		// error already carries the provider's response body, which is plenty for an operator.
		slog.Error("exchanging the OIDC authorization code failed", "error", err)
		return c.JSON(http.StatusUnauthorized, ErrorAnswer{
			ErrorMessage: "GPM could not complete the login with the identity provider.",
			Action:       "Log in again. If it still fails, look at GPM's OIDC client configuration and the server logs.",
			Description:  "The identity provider rejected the authorization code.",
		})
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return c.JSON(http.StatusUnauthorized, ErrorAnswer{
			ErrorMessage: "The identity provider did not return an ID token.",
			Action:       "Make sure that GPM's client can use the openid scope.",
			Description:  "No id_token was present in the token response.",
		})
	}

	idToken, err := a.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		slog.Error("verifying the OIDC ID token failed", "error", err)
		return c.JSON(http.StatusUnauthorized, ErrorAnswer{
			ErrorMessage: "GPM could not verify the ID token from the identity provider.",
			Action:       "Make sure that GPM's issuer and client ID match the provider's configuration.",
			Description:  err.Error(),
		})
	}

	wantNonce, _ := sess.Values[sessionKeyNonce].(string)
	if idToken.Nonce != wantNonce {
		slog.Warn("the OIDC nonce did not match, rejecting the callback")
		return c.JSON(http.StatusUnauthorized, ErrorAnswer{
			ErrorMessage: "GPM could not verify the login.",
			Action:       "Log in again from the start.",
			Description:  "The OIDC nonce did not match the one stored in the session.",
		})
	}

	var claims struct {
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		slog.Debug("could not decode the ID token claims, falling back to the subject", "error", err)
	}

	user := firstNonEmpty(claims.PreferredUsername, claims.Email, claims.Name, idToken.Subject)
	destination := "/"
	if d, ok := sess.Values[sessionKeyDestination].(string); ok {
		destination = safeRedirectTarget(d)
	}

	delete(sess.Values, sessionKeyState)
	delete(sess.Values, sessionKeyNonce)
	delete(sess.Values, sessionKeyDestination)
	delete(sess.Values, sessionKeyVerifier)
	sess.Values[sessionKeyUser] = user
	if err := sess.Save(c.Request(), c.Response()); err != nil {
		return fmt.Errorf("saving the OIDC session failed: %w", err)
	}

	slog.Info("user logged in", "user", user)
	return c.Redirect(http.StatusFound, browserPath(destination))
}

// Clears the local session and, when the provider advertises one, continues to its end-session
// endpoint so the login is dropped there too.
//
// The provider sends the browser back here afterwards, so this has to be idempotent: the hop back
// finds no session and falls through to rendering the logout page. Redirecting unconditionally
// would bounce between GPM and the provider forever.
func (a *authenticator) logout(c echo.Context) error {
	hadSession := false

	// Clear it whether or not it decoded: that is the one a stuck user most needs rid of.
	sess, err := session.Get(sessionName, c)
	if sess == nil {
		slog.Error("the session store is not available, logging out locally only", "error", err)
		return a.renderLoggedOut(c)
	}
	if err != nil {
		slog.Debug("could not read the session being logged out, clearing it anyway", "error", err)
	}
	if user, ok := sess.Values[sessionKeyUser].(string); ok && user != "" {
		hadSession = true
		slog.Info("user logged out", "user", user)
	}
	sess.Values = map[interface{}]interface{}{}
	sess.Options.MaxAge = -1
	if err := sess.Save(c.Request(), c.Response()); err != nil {
		slog.Error("clearing the session failed", "error", err)
	}
	a.clearLegacyRootCookie(c)

	if hadSession && a.endSessionEndpoint != "" {
		redirect := viper.GetString("oidc_redirect_domain")
		u, err := url.Parse(a.endSessionEndpoint)
		if err == nil {
			q := u.Query()
			if redirect != "" {
				logoutTarget, joinErr := url.JoinPath(redirect, browserPath("/logout"))
				if joinErr == nil {
					q.Set("post_logout_redirect_uri", logoutTarget)
				}
			}
			q.Set("client_id", a.oauth2.ClientID)
			u.RawQuery = q.Encode()
			return c.Redirect(http.StatusFound, u.String())
		}
		slog.Error("could not parse the end session endpoint, logging out locally only", "error", err)
	}

	// No provider logout: render the server-side "signed out" page.
	return a.renderLoggedOut(c)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

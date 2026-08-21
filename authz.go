// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/gob"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
	authorizationv1 "k8s.io/api/authorization/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/client-go/restmapper"
)

// RBAC-aligned views (#261). GPM keeps reading the cluster with its own ServiceAccount and asks the
// API server, per logged-in person, what that person may see. It never impersonates them: a
// SubjectAccessReview answers the question without granting GPM the right to act as anybody.
//
// Every path here fails closed. A review that errors, an identity GPM cannot build, a missing grant:
// each one hides data rather than showing it.

func init() {
	// The session cookie is gob-encoded, and the groups are the only slice GPM puts in it.
	gob.Register([]string{})
}

// rbacIdentity is the subject of a SubjectAccessReview: who the API server thinks is asking.
type rbacIdentity struct {
	Username string
	Groups   []string
}

// valid reports whether this identity can be reviewed at all. An empty username denies everything,
// so GPM says so instead of sending a review that cannot succeed.
func (i rbacIdentity) valid() bool { return i.Username != "" }

// String is what the "scoped to your access" chip shows on hover, and what the logs record when
// somebody reports that GPM shows them nothing.
func (i rbacIdentity) String() string {
	if len(i.Groups) == 0 {
		return i.Username
	}
	return fmt.Sprintf("%s (%s)", i.Username, strings.Join(i.Groups, ", "))
}

// rbacClaims pulls the API server's idea of this person out of the ID token. The operator names the
// claims, because only they know how the cluster's --oidc-username-claim is configured. The display
// name is the fallback for the username: on the common setup both are the same claim.
func rbacClaims(idToken *oidc.IDToken, displayName string) (string, []string) {
	var all map[string]any
	if err := idToken.Claims(&all); err != nil {
		slog.Debug("could not decode the ID token claims for the RBAC identity", "error", err)
		return displayName, nil
	}

	return identityFromClaims(all, displayName)
}

// identityFromClaims maps a decoded ID token onto the username and groups the reviews will carry.
// Split from rbacClaims because an oidc.IDToken cannot be built outside the library, and this is
// the half worth testing.
func identityFromClaims(all map[string]any, displayName string) (string, []string) {
	username := displayName
	if claim := viper.GetString("rbac_username_claim"); claim != "" {
		if v, ok := all[claim].(string); ok && v != "" {
			username = v
		} else {
			slog.Warn("the configured RBAC username claim is missing from the ID token",
				"claim", claim, "falling_back_to", displayName)
		}
	}

	var groups []string
	if claim := viper.GetString("rbac_groups_claim"); claim != "" {
		// Silence here reads as "this person is in no groups", which narrows their access without
		// telling anybody. The username claim already warns; the groups claim has two more ways to
		// go wrong, because most providers add it only when a groups mapper is configured.
		switch raw := all[claim].(type) {
		case []any:
			for _, g := range raw {
				if s, ok := g.(string); ok && s != "" {
					groups = append(groups, s)
				}
			}
		case nil:
			slog.Warn("the configured RBAC groups claim is missing from the ID token, so the reviews carry no groups. "+
				"Most providers add the claim only when a groups mapper is configured.", "claim", claim)
		default:
			slog.Warn("the configured RBAC groups claim is not a list, so the reviews carry no groups",
				"claim", claim, "type", fmt.Sprintf("%T", raw))
		}
	}
	return username, groups
}

// rbacIdentityFrom rebuilds the identity from the session, applying the prefixes the cluster's
// --oidc-username-prefix and --oidc-groups-prefix add. The prefixes are applied here, not at login,
// so correcting a misconfigured prefix takes effect when GPM restarts instead of when every user
// logs in again.
func rbacIdentityFrom(sess *sessions.Session) rbacIdentity {
	user, _ := sess.Values[sessionKeyRBACUser].(string)
	groups, _ := sess.Values[sessionKeyRBACGroups].([]string)

	id := rbacIdentity{}
	if user != "" {
		id.Username = viper.GetString("rbac_username_prefix") + user
	}
	if prefix := viper.GetString("rbac_groups_prefix"); prefix != "" {
		for _, g := range groups {
			id.Groups = append(id.Groups, prefix+g)
		}
	} else {
		id.Groups = groups
	}
	return id
}

// accessChecker answers "may this person read this?" with SubjectAccessReviews, and remembers the
// answers briefly. A page asks the same question once per namespace and kind, and without the cache
// a cluster with many violations would send hundreds of reviews per render.
type accessChecker struct {
	clients *kubeClients

	mu     sync.Mutex
	cache  *cache.LRUExpireCache
	now    func() time.Time // swapped in tests
	denied bool             // the API server refused GPM its own reviews; cleared when one succeeds
	// While the grant is missing, every row would send its own rejected review and log its own
	// line. These hold the asking and the logging down to one of each per accessCacheTTL, which
	// still lets a restored grant take effect within the same window.
	deniedUntil time.Time
	nextReport  map[string]time.Time

	mapperMu sync.RWMutex
	mapper   meta.RESTMapper
	// When discovery was last walked, successful or not. It rations both the rebuild a missing Kind
	// asks for and the retry after a failure.
	mapperAt time.Time
}

type accessKey struct {
	// The subject, in two fields and quoted. One rendered string cannot be trusted here: the
	// username is claim-controlled, so somebody named "bob (admins)" would share a key with bob,
	// who really is in admins, and read their answers.
	username  string
	groups    string
	namespace string
	group     string
	resource  string
	verb      string
}

// Short enough that revoking a RoleBinding takes effect while somebody is still looking at the page,
// long enough that one render reuses its own answers.
const accessCacheTTL = 30 * time.Second

func newAccessChecker(clients *kubeClients) *accessChecker {
	a := &accessChecker{clients: clients, now: time.Now}
	a.cache = cache.NewLRUExpireCacheWithClock(accessCacheMax, checkerClock{a})
	return a
}

// checkerClock gives the cache the same clock the rest of the checker reads, including the one a
// test swaps in after construction.
type checkerClock struct{ a *accessChecker }

func (c checkerClock) Now() time.Time { return c.a.now() }

// canAccess reports whether the identity may read one resource in one namespace, and whether GPM
// could determine that at all. An empty namespace asks about the cluster scope. Both a denial and a
// failure hide the row; the second return separates them so the view can say that something is
// missing because a review failed, rather than pretending the cluster is clean.
func (a *accessChecker) canAccess(ctx context.Context, id rbacIdentity, namespace, group, resource, verb string) (allowed, determined bool) {
	if !id.valid() {
		return false, false
	}
	// Sorted, so the same person keys the same way whatever order the provider lists their groups.
	groups := slices.Clone(id.Groups)
	slices.Sort(groups)
	key := accessKey{
		username:  fmt.Sprintf("%q", id.Username),
		groups:    fmt.Sprintf("%q", groups),
		namespace: namespace,
		group:     group,
		resource:  resource,
		verb:      verb,
	}
	allowed, cached, hold := a.lookup(key)
	if cached {
		return allowed, true
	}
	if hold {
		// GPM has just been told it may not ask. Asking again for this row would be one more
		// rejected call and one more log line, for an answer already known to be unavailable.
		return false, false
	}

	review := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:   id.Username,
			Groups: id.Groups,
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace: namespace,
				Group:     group,
				Resource:  resource,
				Verb:      verb,
			},
		},
	}

	answer, err := a.clients.authz.SubjectAccessReviews().Create(ctx, review, metav1.CreateOptions{})
	if err != nil {
		a.reportReviewFailure(err)
		return false, false
	}
	a.recordSuccess(key, answer.Status.Allowed)
	return answer.Status.Allowed, true
}

// lookup answers from the cache and reports whether GPM is holding off asking, in one pass over the
// lock. Two calls would take the same mutex twice on the busiest path in the package.
func (a *accessChecker) lookup(key accessKey) (allowed, cached, hold bool) {
	if answer, ok := a.cache.Get(key); ok {
		return answer.(bool), true, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return false, false, a.denied && a.now().Before(a.deniedUntil)
}

// misconfigured reports whether GPM's own ServiceAccount cannot create SubjectAccessReviews. The
// views surface this instead of showing an empty page: the operator has turned the feature on
// without the grant that makes it work.
func (a *accessChecker) misconfigured() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.denied
}

// Enough for a large cluster's namespaces and kinds across many readers, and small enough that a
// stream of new identities cannot grow the cache for the life of the process.
const accessCacheMax = 10000

func (a *accessChecker) recordSuccess(key accessKey, allowed bool) {
	a.cache.Add(key, allowed, accessCacheTTL)
	// A review got through, so whatever the operator fixed, it is fixed. Without this the banner
	// outlives the problem and only a restart clears it, which teaches people to restart GPM.
	a.mu.Lock()
	a.denied = false
	a.mu.Unlock()
}

// A failed review is either a missing grant, which an operator must fix, or a transient API error.
// The first is worth latching so the views can explain themselves; both hide data meanwhile.
func (a *accessChecker) reportReviewFailure(err error) {
	if !apierrors.IsForbidden(err) {
		// Gated as well: an API server that is briefly unreachable fails every row of the page, and
		// three hundred copies of one outage bury whatever the operator came to read.
		if a.shouldReport("review") {
			slog.Error("a SubjectAccessReview failed, hiding the row it covers", "error", err)
		}
		return
	}

	a.mu.Lock()
	a.denied = true
	a.deniedUntil = a.now().Add(accessCacheTTL)
	a.mu.Unlock()

	if a.shouldReport("refused") {
		slog.Error("GPM cannot create SubjectAccessReviews, so the RBAC-aligned views can show nothing. "+
			"Grant the GPM ServiceAccount `create subjectaccessreviews` (the system:auth-delegator ClusterRole).",
			"error", err)
	}
}

// shouldReport rations one kind of failure log to one line per window. A failure that hides one row
// hides every row, so the second line of a burst says nothing the first did not -- but each kind
// keeps its own budget, or a flapping API server would swallow the line that names the missing grant.
func (a *accessChecker) shouldReport(kind string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	if now.Before(a.nextReport[kind]) {
		return false
	}
	if a.nextReport == nil {
		a.nextReport = map[string]time.Time{}
	}
	a.nextReport[kind] = now.Add(accessCacheTTL)
	return true
}

// --- what this person may see ------------------------------------------------------------------

// The context key holding the views this request may reach, resolved once by the middleware.
const allowedViewsKey = "gpm_allowed_views"

// allowedViews reports which views the current request may reach, keyed by view. A request that
// never passed the middleware -- the error and not-found pages -- can reach everything, because
// those pages carry no cluster data.
func allowedViews(c echo.Context) map[string]bool {
	if m, ok := c.Get(allowedViewsKey).(map[string]bool); ok {
		return m
	}
	all := make(map[string]bool, len(ssrViews))
	for _, v := range ssrViews {
		all[v.Key] = true
	}
	return all
}

// rbacFilteringEnabled reports whether the operator asked for the feature. checkConfig has already
// refused to start in a mode that cannot enforce it, so this is the switch alone.
func (s *server) rbacFilteringEnabled() bool {
	return viper.GetBool("rbac_filtering") && authEnabled()
}

// rbacMiddleware resolves, once per request, which views this person may reach, and refuses the
// rest. The navbar renders from the same answer, so what is shown and what is served cannot drift.
func (s *server) rbacMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !s.rbacFilteringEnabled() {
				return next(c)
			}

			path := backendPath(c.Request().URL.Path)
			if isPublicPath(path) {
				// Login, logout, the callback and the assets: no session to read yet, and nothing
				// to scope.
				return next(c)
			}

			view, known := viewForPath(path)
			if !known {
				// Not a view: the 404 page decides, and it renders no navbar, so nothing here needs
				// an answer. Resolving first would let any path a scanner invents cost a round of
				// reviews.
				return next(c)
			}

			allowed := s.resolveAllowedViews(c)
			c.Set(allowedViewsKey, allowed)
			if allowed[view.Key] {
				return next(c)
			}

			// Home is where a browser lands by default, so send them somewhere they can be rather
			// than refusing the front door. Resources is always reachable.
			if view.Key == "home" {
				return c.Redirect(http.StatusFound, browserPath(firstReachable(allowed)))
			}
			slog.Info("refused a view this user may not see", "path", path)
			return s.renderError(c, http.StatusForbidden, ssrErrorView{
				Heading: "Not allowed",
				Message: "You do not have access to this view.",
				Action: "GPM shows you the views that your Kubernetes account can read, and the " +
					"objects inside them.",
				// Nothing about what the other views hold: a refusal that describes what it is
				// hiding tells the reader more than the page it refused.
				Description: "If you think this is a mistake, contact a cluster administrator.",
				BackURL:     browserPath(firstReachable(allowed)),
			})
		}
	}
}

// firstReachable is where to send someone who asked for a view they cannot have. Nav order, so they
// land on the most policy-shaped thing they can read; Resources is the guaranteed floor.
func firstReachable(allowed map[string]bool) string {
	for _, v := range ssrViews {
		if v.Name != "" && allowed[v.Key] {
			return v.Path
		}
	}
	return "/resources"
}

// resolveAllowedViews asks the API server one question per gated view. Anything short of a clear
// yes -- an unreadable session, an identity GPM cannot build, a review that fails -- means no.
func (s *server) resolveAllowedViews(c echo.Context) map[string]bool {
	allowed := make(map[string]bool, len(ssrViews))
	for _, v := range ssrViews {
		if !v.gated() {
			allowed[v.Key] = true
		}
	}

	sess, err := session.Get(sessionName, c)
	if err != nil {
		slog.Warn("could not read the session while resolving access, showing only the scoped view", "error", err)
		return allowed
	}
	id := rbacIdentityFrom(sess)
	if !id.valid() {
		slog.Warn("no RBAC identity in the session, showing only the scoped view",
			"hint", "check GPM_RBAC_USERNAME_CLAIM against the claims the provider issues")
		return allowed
	}
	clients, err := s.clientsFor(c)
	if err != nil {
		slog.Warn("could not reach the cluster while resolving access, showing only the scoped view", "error", err)
		return allowed
	}

	checker := s.checkerFor(clients)
	ctx := c.Request().Context()
	for _, v := range ssrViews {
		if !v.gated() {
			continue
		}
		ok, _ := checker.canAccess(ctx, id, "", v.Group, v.Resource, "list")
		allowed[v.Key] = ok
	}
	return allowed
}

// validateBool reads a GPM_ boolean strictly. viper reads anything it cannot parse as false, so a
// misspelt GPM_RBAC_FILTERING=treu silently leaves every view unrestricted, and a misspelt
// GPM_SKIP_TLS_VERIFY quietly means the opposite of what was typed. Both switch off a protection,
// so a value that is not a boolean is a configuration error rather than a default.
func validateBool(key string) error {
	raw := strings.TrimSpace(viper.GetString(key))
	if raw == "" {
		return nil
	}
	if _, err := strconv.ParseBool(raw); err != nil {
		return fmt.Errorf("GPM_%s must be true or false, but is %q",
			strings.ToUpper(key), raw)
	}
	return nil
}

// checkConfig validates the settings that decide whether a protection is on, and refuses to start
// when one of them cannot be read the way it was meant.
func (s *server) checkConfig() error {
	if err := validateBool("skip_tls_verify"); err != nil {
		return err
	}
	if err := validateBool("rbac_filtering"); err != nil {
		return err
	}
	return s.checkRBACConfig()
}

// checkRBACConfig refuses to start when RBAC filtering is asked for in a mode that cannot enforce
// it. Running anyway would serve every view to everyone while the operator believes the opposite,
// which is worse than not starting. Reports counts, never context names: those end up in tickets.
func (s *server) checkRBACConfig() error {
	if !viper.GetBool("rbac_filtering") {
		return nil
	}
	if !authEnabled() {
		return fmt.Errorf("GPM_RBAC_FILTERING needs authentication: there is no identity to authorize " +
			"without it. Set GPM_AUTH_ENABLED=OIDC, or unset GPM_RBAC_FILTERING")
	}
	if contexts, _ := s.k8s.contexts(); len(contexts) > 1 {
		return fmt.Errorf("GPM_RBAC_FILTERING does not support more than one cluster: the kubeconfig "+
			"names %d contexts, and one identity cannot be authorized across clusters. Run GPM "+
			"in-cluster, point KUBECONFIG at a file with a single context, or unset GPM_RBAC_FILTERING",
			len(contexts))
	}
	slog.Info("RBAC-aligned views are on: a user without cluster-wide access sees the Resources view, " +
		"scoped to the namespaces they can read")
	return nil
}

// checkerFor returns the access checker for this client set, building it on first use. The cache
// inside it is per cluster, so a new client set gets a new checker rather than inheriting answers.
func (s *server) checkerFor(clients *kubeClients) *accessChecker {
	s.authzMu.Lock()
	defer s.authzMu.Unlock()
	if s.authz == nil || s.authz.clients != clients {
		s.authz = newAccessChecker(clients)
	}
	return s.authz
}

// A SubjectAccessReview asks about a resource ("deployments"), and a Gatekeeper violation names a
// Kind ("Deployment"). The mapper closes that gap. It is built from discovery on first use and kept:
// rebuilding it per row would put a discovery round trip in front of every violation.
func (a *accessChecker) resourceFor(group, kind string) (string, bool) {
	gk := schema.GroupKind{Group: group, Kind: kind}
	mapper, ok := a.restMapper(false)
	if ok {
		if mapping, err := mapper.RESTMapping(gk); err == nil {
			return mapping.Resource.Resource, true
		}
		// The mapper does not know this Kind. It can predate a CRD installed since GPM started, so
		// ask for a rebuilt one and try again. Rebuilds are rationed, and a failed one keeps the
		// mapper already in hand rather than leaving the checker with none.
		mapper, ok = a.restMapper(true)
	}
	if !ok {
		return "", false
	}
	mapping, err := mapper.RESTMapping(gk)
	if err != nil {
		// A Kind that is genuinely gone: a removed CRD still named by the audit. Fail closed, and
		// say so once per Kind per window -- the same Kind can hold dozens of rows, and the fact is
		// about the Kind rather than about any one of them. A budget each, so a second missing Kind
		// is named too, and a repeat every window keeps the reason visible while the rows are.
		if a.shouldReport("mapping:" + gk.String()) {
			slog.Warn("no API mapping for a violation's Kind, hiding every row of it",
				"group", group, "kind", kind, "error", err)
		}
		return "", false
	}
	return mapping.Resource.Resource, true
}

// restMapper hands out the Kind-to-resource mapper, building it on first use. With refresh set the
// caller has met a Kind the mapper does not know and wants a newer one; that costs a discovery walk,
// so it happens at most once per window. A build that fails returns the mapper already in hand: an
// unknown Kind plus one bad discovery call must not cost the page every other row.
func (a *accessChecker) restMapper(refresh bool) (meta.RESTMapper, bool) {
	// The common case is a mapper that is already built and good enough, and a page asks once per
	// row: a read lock lets the workers do that at the same time instead of in turn.
	a.mapperMu.RLock()
	mapper, at := a.mapper, a.mapperAt
	a.mapperMu.RUnlock()
	if mapper != nil && (!refresh || a.now().Before(at.Add(accessCacheTTL))) {
		return mapper, true
	}

	mapper, err := a.buildMapper(refresh)
	// Logged after the mapper lock is released: shouldReport takes the cache mutex, and holding one
	// while taking the other would establish an order the rest of this file does not follow.
	if err != nil && a.shouldReport("discovery") {
		slog.Error("could not read the API groups, so no violation can be authorized", "error", err)
	}
	return mapper, mapper != nil
}

// buildMapper walks discovery under the write lock, so a page's worth of rows waits for one walk
// rather than starting one each. It returns whatever mapper the checker holds afterwards.
func (a *accessChecker) buildMapper(refresh bool) (meta.RESTMapper, error) {
	a.mapperMu.Lock()
	defer a.mapperMu.Unlock()

	// Re-read under the write lock: another goroutine may have built it while this one queued.
	recent := a.now().Before(a.mapperAt.Add(accessCacheTTL))
	if recent || (a.mapper != nil && !refresh) {
		// Either the mapper is good enough, or discovery already ran inside this window. Walking it
		// again for every row would multiply one outage, or one unknown Kind, by the size of the page.
		return a.mapper, nil
	}

	a.mapperAt = a.now()
	groups, err := restmapper.GetAPIGroupResources(a.clients.discovery)
	if err != nil {
		return a.mapper, err
	}
	a.mapper = restmapper.NewDiscoveryRESTMapper(groups)
	return a.mapper, nil
}

// canSeeViolation is the question the Resources view asks per row. The verb is `list`, not `get`,
// because the page enumerates: it shows the names of the objects that broke a policy. A role that
// grants `get` without `list` withholds exactly that enumeration, and GPM must withhold it too.
func (a *accessChecker) canSeeViolation(ctx context.Context, id rbacIdentity, namespace, group, kind string) (allowed, determined bool) {
	resource, ok := a.resourceFor(group, kind)
	if !ok {
		return false, false
	}
	return a.canAccess(ctx, id, namespace, group, resource, "list")
}

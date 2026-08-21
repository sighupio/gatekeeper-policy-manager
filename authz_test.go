// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/sessions"
	"github.com/spf13/viper"
	authorizationv1 "k8s.io/api/authorization/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// checkerWith builds an accessChecker whose SubjectAccessReviews are answered by decide, and records
// every review the code sends so the tests can assert on the question, not only on the answer.
func checkerWith(t *testing.T, decide func(*authorizationv1.SubjectAccessReview) (bool, error)) (*accessChecker, *[]*authorizationv1.SubjectAccessReview) {
	t.Helper()
	var sent []*authorizationv1.SubjectAccessReview
	client := sarClient(func(r *authorizationv1.SubjectAccessReview) (bool, error) {
		sent = append(sent, r)
		return decide(r)
	})
	return newAccessChecker(&kubeClients{authz: client.AuthorizationV1()}), &sent
}

// sarClient is a fake clientset whose SubjectAccessReviews are answered by decide. Every test that
// fakes the API server goes through here, so the reactor is written once.
func sarClient(decide func(*authorizationv1.SubjectAccessReview) (bool, error)) *fake.Clientset {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "subjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		review := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SubjectAccessReview)
		allowed, err := decide(review)
		if err != nil {
			return true, nil, err
		}
		review.Status.Allowed = allowed
		return true, review, nil
	})
	return client
}

func TestCanAccessAsksTheApiServerAndObeysTheAnswer(t *testing.T) {
	a, sent := checkerWith(t, func(r *authorizationv1.SubjectAccessReview) (bool, error) {
		return r.Spec.ResourceAttributes.Namespace == "apps-prod", nil
	})
	id := rbacIdentity{Username: "dev", Groups: []string{"team-a"}}

	if allowed, _ := a.canAccess(context.Background(), id, "apps-prod", "apps", "deployments", "get"); !allowed {
		t.Error("a namespace the API server allows must be visible")
	}
	if allowed, _ := a.canAccess(context.Background(), id, "kube-system", "apps", "deployments", "get"); allowed {
		t.Error("a namespace the API server denies must not be visible")
	}

	if len(*sent) != 2 {
		t.Fatalf("expected two reviews, got %d", len(*sent))
	}
	first := (*sent)[0].Spec
	if first.User != "dev" || len(first.Groups) != 1 || first.Groups[0] != "team-a" {
		t.Errorf("the review must carry the identity, got user=%q groups=%v", first.User, first.Groups)
	}
	if ra := first.ResourceAttributes; ra.Group != "apps" || ra.Resource != "deployments" || ra.Verb != "get" {
		t.Errorf("the review must carry the resource, got %+v", ra)
	}
}

func TestCanAccessFailsClosed(t *testing.T) {
	for _, tt := range []struct {
		name   string
		id     rbacIdentity
		decide func(*authorizationv1.SubjectAccessReview) (bool, error)
	}{
		{
			"an identity with no username cannot be reviewed, so it sees nothing",
			rbacIdentity{Groups: []string{"team-a"}},
			func(*authorizationv1.SubjectAccessReview) (bool, error) { return true, nil },
		},
		{
			"a review that errors hides the row",
			rbacIdentity{Username: "dev"},
			func(*authorizationv1.SubjectAccessReview) (bool, error) { return true, errors.New("apiserver is down") },
		},
		{
			"a review GPM is not allowed to create hides the row",
			rbacIdentity{Username: "dev"},
			func(*authorizationv1.SubjectAccessReview) (bool, error) {
				return true, apierrors.NewForbidden(schema.GroupResource{Resource: "subjectaccessreviews"}, "", errors.New("no"))
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, _ := checkerWith(t, tt.decide)
			allowed, determined := a.canAccess(context.Background(), tt.id, "apps-prod", "apps", "deployments", "get")
			if allowed {
				t.Error("access was granted where it must have been denied")
			}
			if determined {
				t.Error("a failure must report that GPM could not tell, so the view can say rows are missing")
			}
		})
	}
}

func TestMissingGrantIsReportedNotSilent(t *testing.T) {
	a, _ := checkerWith(t, func(*authorizationv1.SubjectAccessReview) (bool, error) {
		return false, apierrors.NewForbidden(schema.GroupResource{Resource: "subjectaccessreviews"}, "", errors.New("nope"))
	})
	if a.misconfigured() {
		t.Fatal("nothing has failed yet")
	}
	_, _ = a.canAccess(context.Background(), rbacIdentity{Username: "dev"}, "apps-prod", "apps", "deployments", "get")
	if !a.misconfigured() {
		t.Error("a forbidden review means the operator has not granted create subjectaccessreviews, and the views must be able to say so")
	}
}

// The banner must not outlive the problem: an operator who grants the permission while GPM is
// running has to see the page recover, or the only remedy they learn is a restart.
func TestFixingTheGrantClearsTheWarning(t *testing.T) {
	broken := true
	a, _ := checkerWith(t, func(*authorizationv1.SubjectAccessReview) (bool, error) {
		if broken {
			return false, apierrors.NewForbidden(schema.GroupResource{Resource: "subjectaccessreviews"}, "", errors.New("no"))
		}
		return true, nil
	})
	id := rbacIdentity{Username: "dev"}

	clock := time.Now()
	a.now = func() time.Time { return clock }

	_, _ = a.canAccess(context.Background(), id, "apps-prod", "apps", "deployments", "get")
	if !a.misconfigured() {
		t.Fatal("a forbidden review must raise the warning")
	}

	broken = false
	// Inside the window GPM does not ask again, so the warning stands: one refusal means every row
	// would be refused, and asking per row is what floods the log.
	_, _ = a.canAccess(context.Background(), id, "apps-stage", "apps", "deployments", "get")
	if !a.misconfigured() {
		t.Error("the warning cleared without a review getting through")
	}

	// When the window passes, the next question reaches the API server and the fix is noticed --
	// no restart, at most one window of delay.
	clock = clock.Add(accessCacheTTL + time.Second)
	_, _ = a.canAccess(context.Background(), id, "apps-stage", "apps", "deployments", "get")
	if a.misconfigured() {
		t.Error("the warning survived the fix, so the page keeps blaming an operator who already acted")
	}
}

// A missing grant refuses every row alike. GPM asks once per window and takes the same answer for
// the rest, so a page with hundreds of objects costs one rejected review, not hundreds.
func TestARefusedCheckerStopsAsking(t *testing.T) {
	a, sent := checkerWith(t, func(*authorizationv1.SubjectAccessReview) (bool, error) {
		return false, apierrors.NewForbidden(schema.GroupResource{Resource: "subjectaccessreviews"}, "", errors.New("no"))
	})
	clock := time.Now()
	a.now = func() time.Time { return clock }

	for i := range 50 {
		// Distinct questions: the answer cache cannot be what holds the count down.
		_, _ = a.canAccess(context.Background(), rbacIdentity{Username: "dev"},
			fmt.Sprintf("ns-%d", i), "apps", "deployments", "get")
	}
	if len(*sent) != 1 {
		t.Errorf("50 rows sent %d reviews, want 1", len(*sent))
	}

	clock = clock.Add(accessCacheTTL + time.Second)
	_, _ = a.canAccess(context.Background(), rbacIdentity{Username: "dev"}, "ns-0", "apps", "deployments", "get")
	if len(*sent) != 2 {
		t.Errorf("the window must reopen for one probe, sent %d reviews", len(*sent))
	}
}

// The same refusal must not be written to the log once per row either.
func TestARefusedCheckerLogsOncePerWindow(t *testing.T) {
	a, _ := checkerWith(t, func(*authorizationv1.SubjectAccessReview) (bool, error) {
		return false, apierrors.NewForbidden(schema.GroupResource{Resource: "subjectaccessreviews"}, "", errors.New("no"))
	})
	clock := time.Now()
	a.now = func() time.Time { return clock }

	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, &slog.HandlerOptions{Level: slog.LevelError})))
	t.Cleanup(func() { slog.SetDefault(restore) })

	// Driven through reportReviewFailure rather than canAccess: suppression already holds the
	// second row back, so going through canAccess would prove nothing about the log gate itself.
	refused := apierrors.NewForbidden(schema.GroupResource{Resource: "subjectaccessreviews"}, "", errors.New("no"))
	for range 50 {
		a.reportReviewFailure(refused)
	}
	if n := strings.Count(logged.String(), "cannot create SubjectAccessReviews"); n != 1 {
		t.Errorf("50 refusals wrote %d log lines, want 1", n)
	}

	// The next window says it again, so a problem that lasts does not fall silent.
	clock = clock.Add(accessCacheTTL + time.Second)
	a.reportReviewFailure(refused)
	if n := strings.Count(logged.String(), "cannot create SubjectAccessReviews"); n != 2 {
		t.Errorf("a new window wrote %d lines in total, want 2", n)
	}
}

func TestAnswersAreCachedPerIdentityAndExpire(t *testing.T) {
	a, sent := checkerWith(t, func(*authorizationv1.SubjectAccessReview) (bool, error) { return true, nil })
	clock := time.Now()
	a.now = func() time.Time { return clock }

	dev := rbacIdentity{Username: "dev", Groups: []string{"team-a"}}
	_, _ = a.canAccess(context.Background(), dev, "apps-prod", "apps", "deployments", "get")
	_, _ = a.canAccess(context.Background(), dev, "apps-prod", "apps", "deployments", "get")
	if len(*sent) != 1 {
		t.Errorf("the same question must be asked once, sent %d reviews", len(*sent))
	}

	// Same person, different groups: a different subject, so a different answer.
	a.canAccess(context.Background(), rbacIdentity{Username: "dev", Groups: []string{"team-b"}},
		"apps-prod", "apps", "deployments", "get")
	if len(*sent) != 2 {
		t.Errorf("a different group set must not reuse the cached answer, sent %d reviews", len(*sent))
	}

	// A revoked RoleBinding has to take effect while the page is still open.
	clock = clock.Add(accessCacheTTL + time.Second)
	_, _ = a.canAccess(context.Background(), dev, "apps-prod", "apps", "deployments", "get")
	if len(*sent) != 3 {
		t.Errorf("the answer must expire, sent %d reviews", len(*sent))
	}
}

// The cache key is the subject. A username is claim-controlled, so a rendered "name (groups)" key
// lets somebody named "bob (admins)" share bob's entries and read answers the API server never gave
// them -- no review involved, straight out of the cache.
func TestCacheKeyCannotBeForged(t *testing.T) {
	a, sent := checkerWith(t, func(r *authorizationv1.SubjectAccessReview) (bool, error) {
		return slices.Contains(r.Spec.Groups, "admins"), nil
	})
	member := rbacIdentity{Username: "bob", Groups: []string{"admins"}}
	forger := rbacIdentity{Username: "bob (admins)"}

	if allowed, _ := a.canAccess(context.Background(), member, "kube-system", "", "secrets", "get"); !allowed {
		t.Fatal("the real group member must be allowed")
	}
	if allowed, _ := a.canAccess(context.Background(), forger, "kube-system", "", "secrets", "get"); allowed {
		t.Error("a second identity read the first one's cached answer")
	}
	if len(*sent) != 2 {
		t.Errorf("each subject must be reviewed on its own, sent %d reviews for 2 subjects", len(*sent))
	}
}

// Group order is the provider's business, not a reason to ask the API server twice.
func TestGroupOrderDoesNotSplitTheCache(t *testing.T) {
	a, sent := checkerWith(t, func(*authorizationv1.SubjectAccessReview) (bool, error) { return true, nil })
	for _, groups := range [][]string{{"platform", "payments"}, {"payments", "platform"}} {
		_, _ = a.canAccess(context.Background(), rbacIdentity{Username: "dev", Groups: groups},
			"apps-prod", "apps", "deployments", "get")
	}
	if len(*sent) != 1 {
		t.Errorf("the same subject must be asked once, sent %d reviews", len(*sent))
	}
}

// The cap is ours to pass, even though the eviction policy is the library's. A page-sized burst of
// distinct identities must not grow the cache past it.
func TestTheCacheStaysBounded(t *testing.T) {
	a, _ := checkerWith(t, func(*authorizationv1.SubjectAccessReview) (bool, error) { return true, nil })
	for i := range accessCacheMax + accessCacheMax/10 {
		_, _ = a.canAccess(context.Background(), rbacIdentity{Username: fmt.Sprintf("dev-%d", i)},
			"apps-prod", "apps", "deployments", "list")
	}
	if n := len(a.cache.Keys()); n > accessCacheMax {
		t.Errorf("the cache holds %d entries, past the %d cap", n, accessCacheMax)
	}
}

func TestEachViewAsksAboutItsOwnData(t *testing.T) {
	a, sent := checkerWith(t, func(*authorizationv1.SubjectAccessReview) (bool, error) { return true, nil })
	id := rbacIdentity{Username: "ops"}
	for _, v := range ssrViews {
		if !v.gated() {
			continue
		}
		_, _ = a.canAccess(context.Background(), id, "", v.Group, v.Resource, "list")
	}

	asked := map[string]string{}
	for _, r := range *sent {
		ra := r.Spec.ResourceAttributes
		if ra.Namespace != "" {
			t.Errorf("a view's question must be cluster-scoped, got namespace %q for %s", ra.Namespace, ra.Resource)
		}
		if ra.Verb != "list" {
			t.Errorf("a view asks whether you can list its data, got verb %q", ra.Verb)
		}
		asked[ra.Group+"/"+ra.Resource] = ra.Verb
	}
	for _, want := range []string{
		"templates.gatekeeper.sh/constrainttemplates",
		"constraints.gatekeeper.sh/*",
		"mutations.gatekeeper.sh/*",
		"config.gatekeeper.sh/configs",
		"/events",
	} {
		if _, ok := asked[want]; !ok {
			t.Errorf("no view asked about %q", want)
		}
	}
}

// A groups claim that never arrives narrows everybody's access silently: the reviews go out with no
// groups, every group RoleBinding misses, and the page just looks small. The username claim already
// warns when it is missing, and these two failures are more likely -- most providers leave groups
// out of the token until a mapper is configured.
func TestAGroupsClaimThatDoesNotArriveIsReported(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Set("rbac_groups_claim", "groups")

	for name, tt := range map[string]struct {
		claims map[string]any
		want   string
		groups int
	}{
		"absent from the token": {map[string]any{"sub": "dev"}, "missing from the ID token", 0},
		"a string, not a list":  {map[string]any{"groups": "platform,payments"}, "not a list", 0},
		"in no groups at all":   {map[string]any{"groups": []any{}}, "", 0},
		"a proper list":         {map[string]any{"groups": []any{"platform"}}, "", 1},
	} {
		t.Run(name, func(t *testing.T) {
			var logged bytes.Buffer
			restore := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})))
			t.Cleanup(func() { slog.SetDefault(restore) })

			_, groups := identityFromClaims(tt.claims, "dev")
			if len(groups) != tt.groups {
				t.Errorf("groups = %v, want %d of them", groups, tt.groups)
			}
			switch {
			case tt.want == "" && strings.Contains(logged.String(), "groups claim"):
				t.Errorf("a usable claim must not warn, got %s", logged.String())
			case tt.want != "" && !strings.Contains(logged.String(), tt.want):
				t.Errorf("the operator was not told the claim is unusable, got %s", logged.String())
			}
		})
	}
}

// mappableChecker is a checker whose fake API server knows the Kinds given, and records the verb of
// every review it is asked.
func mappableChecker(t *testing.T, kinds ...metav1.APIResourceList) (*accessChecker, *fake.Clientset, func() []string) {
	t.Helper()
	// Locked: the reviews can come from several goroutines, because resolveAccess asks them a few
	// at a time. Without this the first concurrent test through here fails under -race, in the
	// helper rather than in the code it is testing.
	var mu sync.Mutex
	var verbs []string
	client := sarClient(func(r *authorizationv1.SubjectAccessReview) (bool, error) {
		mu.Lock()
		verbs = append(verbs, r.Spec.ResourceAttributes.Verb)
		mu.Unlock()
		return true, nil
	})
	for i := range kinds {
		client.Resources = append(client.Resources, &kinds[i])
	}
	// A snapshot behind the same lock the reactor writes under, so a caller can read the verbs while
	// reviews are still in flight without racing the workers.
	snapshot := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(verbs)
	}
	return newAccessChecker(&kubeClients{authz: client.AuthorizationV1(), discovery: client.Discovery()}), client, snapshot
}

var deploymentsExist = metav1.APIResourceList{
	GroupVersion: "apps/v1",
	APIResources: []metav1.APIResource{{Name: "deployments", Kind: "Deployment", Namespaced: true}},
}

// The Resources view enumerates: it prints the names of the objects that broke a policy. That is the
// `list` verb. A role that grants `get` without `list` withholds the enumeration on purpose, and GPM
// must withhold it too rather than answering a question nobody asked.
func TestARowIsAuthorizedByListingNotByGetting(t *testing.T) {
	a, _, verbs := mappableChecker(t, deploymentsExist)

	if allowed, _ := a.canSeeViolation(context.Background(), rbacIdentity{Username: "dev"},
		"mine", "apps", "Deployment"); !allowed {
		t.Fatal("the row should have been allowed")
	}
	if asked := verbs(); len(asked) != 1 || asked[0] != "list" {
		t.Errorf("a row on a page that enumerates must be authorized with list, got %v", asked)
	}
}

// A Kind the mapper does not know can be a CRD installed while GPM was running. Hiding its rows
// until somebody restarts the pod is not an answer.
func TestAKindInstalledLaterStopsBeingHidden(t *testing.T) {
	a, client, _ := mappableChecker(t, deploymentsExist)
	clock := time.Now()
	a.now = func() time.Time { return clock }

	if _, ok := a.resourceFor("acme.example.com", "Widget"); ok {
		t.Fatal("a Kind nothing knows about must not resolve")
	}

	client.Resources = append(client.Resources, &metav1.APIResourceList{
		GroupVersion: "acme.example.com/v1",
		APIResources: []metav1.APIResource{{Name: "widgets", Kind: "Widget", Namespaced: true}},
	})
	if _, ok := a.resourceFor("acme.example.com", "Widget"); ok {
		t.Error("inside the window the mapper is kept, so discovery is not walked again per row")
	}

	clock = clock.Add(accessCacheTTL + time.Second)
	resource, ok := a.resourceFor("acme.example.com", "Widget")
	if !ok || resource != "widgets" {
		t.Errorf("after the window the new CRD should resolve, got %q ok=%v", resource, ok)
	}
}

// One line per Kind per window. Per Kind, so a second missing CRD is named rather than hidden behind
// the first; per window, so the reason stays visible while the rows are missing instead of scrolling
// away after one line -- and never once per row, which is what the audit would produce.
func TestAMissingKindIsReportedOncePerKindPerWindow(t *testing.T) {
	a, _, _ := mappableChecker(t, deploymentsExist)
	clock := time.Now()
	a.now = func() time.Time { return clock }

	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(restore) })
	lines := func(kind string) int { return strings.Count(logged.String(), `"kind":"`+kind+`"`) }

	// Ten rows of one missing Kind: one line.
	for range 10 {
		_, _ = a.resourceFor("acme.example.com", "Widget")
	}
	if lines("Widget") != 1 {
		t.Errorf("ten rows of one Kind wrote %d lines, want 1", lines("Widget"))
	}

	// A second missing Kind has its own budget, so it is named too.
	_, _ = a.resourceFor("acme.example.com", "Gizmo")
	if lines("Gizmo") != 1 {
		t.Errorf("the second missing Kind was not named, got %d lines", lines("Gizmo"))
	}

	// The next window says it again: the rows are still missing, so the reason should still be there.
	clock = clock.Add(accessCacheTTL + time.Second)
	_, _ = a.resourceFor("acme.example.com", "Widget")
	if lines("Widget") != 2 {
		t.Errorf("the next window wrote %d lines for a Kind that is still missing, want 2", lines("Widget"))
	}
}

// An unreachable API server fails every row alike, and one outage is one line.
func TestATransientFailureLogsOncePerWindow(t *testing.T) {
	a, _ := checkerWith(t, func(*authorizationv1.SubjectAccessReview) (bool, error) { return true, nil })
	clock := time.Now()
	a.now = func() time.Time { return clock }

	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, &slog.HandlerOptions{Level: slog.LevelError})))
	t.Cleanup(func() { slog.SetDefault(restore) })

	for range 50 {
		a.reportReviewFailure(errors.New("apiserver is unreachable"))
	}
	if n := strings.Count(logged.String(), "hiding the row it covers"); n != 1 {
		t.Errorf("50 failed reviews wrote %d log lines, want 1", n)
	}
	clock = clock.Add(accessCacheTTL + time.Second)
	a.reportReviewFailure(errors.New("apiserver is unreachable"))
	if n := strings.Count(logged.String(), "hiding the row it covers"); n != 2 {
		t.Errorf("a new window should say it again, total %d", n)
	}
}

// The window belongs to one cluster's checker. If it ever became global, one misconfigured cluster
// would silence the reviews for every other one.
func TestSuppressionIsPerChecker(t *testing.T) {
	refused, _ := checkerWith(t, func(*authorizationv1.SubjectAccessReview) (bool, error) {
		return false, apierrors.NewForbidden(schema.GroupResource{Resource: "subjectaccessreviews"}, "", errors.New("no"))
	})
	other, sent := checkerWith(t, func(*authorizationv1.SubjectAccessReview) (bool, error) { return true, nil })

	id := rbacIdentity{Username: "dev"}
	_, _ = refused.canAccess(context.Background(), id, "apps-prod", "apps", "deployments", "list")
	if !refused.misconfigured() {
		t.Fatal("the refused checker should have latched")
	}

	if allowed, determined := other.canAccess(context.Background(), id, "apps-prod", "apps", "deployments", "list"); !allowed || !determined {
		t.Error("another cluster's checker stopped asking because this one was refused")
	}
	if len(*sent) != 1 {
		t.Errorf("the second checker sent %d reviews, want 1", len(*sent))
	}
}

// brokenDiscovery answers every discovery walk with an error, and counts the walks. Embedding the
// interface satisfies the methods restmapper never calls.
type brokenDiscovery struct {
	discovery.DiscoveryInterface
	walks int
}

func (b *brokenDiscovery) ServerGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
	b.walks++
	return nil, nil, errors.New("apiserver is down")
}

// A rebuild is asked for when a Kind is unknown. If discovery is down at that moment, the mapper
// already in hand is the best there is -- throwing it away would hide every other Kind too.
func TestAFailedRebuildKeepsTheMapperInHand(t *testing.T) {
	a, _, _ := mappableChecker(t, deploymentsExist)
	clock := time.Now()
	a.now = func() time.Time { return clock }

	if res, ok := a.resourceFor("apps", "Deployment"); !ok || res != "deployments" {
		t.Fatalf("the known Kind should resolve, got %q ok=%v", res, ok)
	}

	// A discovery that really fails. The fake clientset's reactors are invoked for discovery but
	// their error is dropped, so a reactor here would test a rebuild that quietly succeeded.
	a.clients.discovery = &brokenDiscovery{}
	clock = clock.Add(accessCacheTTL + time.Second)
	if _, ok := a.resourceFor("acme.example.com", "Widget"); ok {
		t.Error("a Kind nothing knows about must not resolve")
	}
	if res, ok := a.resourceFor("apps", "Deployment"); !ok || res != "deployments" {
		t.Errorf("the failed rebuild threw away a working mapper: got %q ok=%v", res, ok)
	}
}

// Discovery is a walk of every API group. One outage must not be multiplied by the size of the page.
func TestDiscoveryIsWalkedOncePerWindow(t *testing.T) {
	a, _, _ := mappableChecker(t)
	broken := &brokenDiscovery{}
	a.clients.discovery = broken
	clock := time.Now()
	a.now = func() time.Time { return clock }

	for range 50 {
		_, _ = a.resourceFor("apps", "Deployment")
	}
	if broken.walks != 1 {
		t.Errorf("50 rows walked discovery %d times, want 1", broken.walks)
	}

	clock = clock.Add(accessCacheTTL + time.Second)
	_, _ = a.resourceFor("apps", "Deployment")
	if broken.walks != 2 {
		t.Errorf("the next window should try again, walks=%d", broken.walks)
	}
}

// The line that names the missing grant is the one an operator can act on. A flapping API server
// must not spend its budget.
func TestEachFailureKeepsItsOwnLogBudget(t *testing.T) {
	a, _ := checkerWith(t, func(*authorizationv1.SubjectAccessReview) (bool, error) { return true, nil })
	clock := time.Now()
	a.now = func() time.Time { return clock }

	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, &slog.HandlerOptions{Level: slog.LevelError})))
	t.Cleanup(func() { slog.SetDefault(restore) })

	a.reportReviewFailure(errors.New("apiserver is unreachable"))
	a.reportReviewFailure(apierrors.NewForbidden(schema.GroupResource{Resource: "subjectaccessreviews"}, "", errors.New("no")))

	if n := strings.Count(logged.String(), "hiding the row it covers"); n != 1 {
		t.Errorf("the transient failure logged %d times, want 1", n)
	}
	if n := strings.Count(logged.String(), "cannot create SubjectAccessReviews"); n != 1 {
		t.Errorf("the missing grant logged %d times, want 1 -- a flap must not swallow it", n)
	}
}

func TestIdentityAppliesThePrefixesOnRead(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Set("rbac_username_prefix", "oidc:")
	viper.Set("rbac_groups_prefix", "oidc:")

	sess := sessions.NewSession(nil, "gpm")
	sess.Values = map[any]any{
		sessionKeyRBACUser:   "dev@example.test",
		sessionKeyRBACGroups: []string{"platform", "payments"},
	}

	id := rbacIdentityFrom(sess)
	if id.Username != "oidc:dev@example.test" {
		t.Errorf("username prefix not applied: %q", id.Username)
	}
	if len(id.Groups) != 2 || id.Groups[0] != "oidc:platform" || id.Groups[1] != "oidc:payments" {
		t.Errorf("group prefix not applied: %v", id.Groups)
	}

	// The prefixes are read from configuration every time, so fixing a typo needs a restart, not a
	// new login from everybody.
	viper.Set("rbac_username_prefix", "")
	if again := rbacIdentityFrom(sess); again.Username != "dev@example.test" {
		t.Errorf("the stored value must be raw, got %q", again.Username)
	}
}

func TestAnEmptySessionYieldsAnIdentityThatSeesNothing(t *testing.T) {
	id := rbacIdentityFrom(sessions.NewSession(nil, "gpm"))
	if id.valid() {
		t.Error("no session means no identity, and an identity that cannot be reviewed must not be valid")
	}
}

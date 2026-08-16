// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"
)

// A stand-in Kubernetes API that records the paths it is asked for. The namespace GPM reads from
// appears in the request path, so the recorded path is the behavior under test: there is no way
// to tell a cluster-wide list from a namespaced one by looking at the answer.
type recordingAPI struct {
	server *httptest.Server

	mu    sync.Mutex
	paths []string
}

func newRecordingAPI(t *testing.T) *recordingAPI {
	t.Helper()

	api := &recordingAPI{}
	api.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.mu.Lock()
		api.paths = append(api.paths, r.URL.Path)
		api.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"apiVersion":"v1","kind":"EventList","items":[]}`)
	}))
	t.Cleanup(api.server.Close)

	return api
}

func (a *recordingAPI) requested() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.paths...)
}

// Builds a server whose Kubernetes clients talk to the stand-in API.
func newEventsTestServer(t *testing.T, api *recordingAPI) *server {
	t.Helper()

	useTestKubeconfig(t, fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: fake
clusters:
  - name: fake-cluster
    cluster:
      server: %s
contexts:
  - name: fake
    context:
      cluster: fake-cluster
      user: fake-user
users:
  - name: fake-user
    user:
      token: fake-token
`, api.server.URL))

	registry, err := newClientRegistry()
	if err != nil {
		t.Fatalf("building the registry failed: %v", err)
	}
	return &server{k8s: registry, ssr: newSSRRenderer()}
}

// Drives the events view handler. The namespace resolution under test lives in getEvents (it is
// what the JSON getEvents used to hold); the assertions check which namespace the handler asked the
// Kubernetes API for, not the rendered page.
func callGetEvents(t *testing.T, s *server, query string) {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/events"+query, nil)
	rec := httptest.NewRecorder()

	if err := s.getEvents(e.NewContext(req, rec)); err != nil {
		t.Fatalf("the handler returned an error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
}

func TestEventsReadEveryNamespaceByDefault(t *testing.T) {
	api := newRecordingAPI(t)
	// useTestKubeconfig runs inside, so the settings have to be reset first.
	useTestSettings(t)
	s := newEventsTestServer(t, api)

	callGetEvents(t, s, "")

	if got, want := api.requested(), []string{"/api/v1/events"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("asked the API for %v, want %v — a cluster-wide list", got, want)
	}
}

// The deployment's RBAC is cut to GPM_EVENTS_NAMESPACE, so GPM must not ask for anything wider.
func TestEventsNamespaceLimitsTheRequest(t *testing.T) {
	api := newRecordingAPI(t)
	useTestSettings(t)
	s := newEventsTestServer(t, api)
	viper.Set("events_namespace", "gatekeeper-system")

	callGetEvents(t, s, "")

	want := "/api/v1/namespaces/gatekeeper-system/events"
	if got := api.requested(); len(got) != 1 || got[0] != want {
		t.Errorf("asked the API for %v, want %q", got, want)
	}
}

// Without a configured namespace the query parameter still selects one, which is what the API has
// always accepted.
func TestEventsQueryParameterSelectsANamespaceWhenNoneIsConfigured(t *testing.T) {
	api := newRecordingAPI(t)
	useTestSettings(t)
	s := newEventsTestServer(t, api)

	callGetEvents(t, s, "?namespace=kube-system")

	want := "/api/v1/namespaces/kube-system/events"
	if got := api.requested(); len(got) != 1 || got[0] != want {
		t.Errorf("asked the API for %v, want %q", got, want)
	}
}

// A request must not be able to widen or move what the deployment was configured and authorized
// for. Honoring the parameter here would only earn a 403 from the Kubernetes API.
func TestEventsQueryParameterCannotEscapeTheConfiguredNamespace(t *testing.T) {
	for _, param := range []string{"?namespace=kube-system", "?namespace="} {
		t.Run("param="+param, func(t *testing.T) {
			api := newRecordingAPI(t)
			useTestSettings(t)
			s := newEventsTestServer(t, api)
			viper.Set("events_namespace", "gatekeeper-system")

			callGetEvents(t, s, param)

			want := "/api/v1/namespaces/gatekeeper-system/events"
			if got := api.requested(); len(got) != 1 || got[0] != want {
				t.Errorf("asked the API for %v, want %q", got, want)
			}
		})
	}
}

// The list call must honor the request context, so a client that disconnects (or a request that
// hits the server's WriteTimeout) cancels the in-flight Kubernetes call instead of finishing it.
// With the old context.TODO() a cancelled context was ignored and this returned no error.
func TestGetKubernetesEventsHonorsContextCancellation(t *testing.T) {
	api := newRecordingAPI(t)
	useTestSettings(t)
	s := newEventsTestServer(t, api)
	clients, err := s.k8s.forContext(defaultKubeContext)
	if err != nil {
		t.Fatalf("resolving clients failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before the call

	if _, err := getKubernetesEvents(ctx, *clients.dynamic, "", "gatekeeper-webhook"); err == nil {
		t.Error("a cancelled context did not abort the events list; the context is not threaded")
	}
}

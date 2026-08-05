// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/labstack/echo/v4"
)

// A kubeconfig with two contexts pointing at different API servers, so a test can tell which one a
// set of clients is talking to by looking at the host.
const twoClusterKubeconfig = `apiVersion: v1
kind: Config
current-context: alpha
clusters:
  - name: alpha-cluster
    cluster:
      server: https://alpha.example:6443
  - name: beta-cluster
    cluster:
      server: https://beta.example:6443
contexts:
  - name: alpha
    context:
      cluster: alpha-cluster
      user: alpha-user
  - name: beta
    context:
      cluster: beta-cluster
      user: beta-user
users:
  - name: alpha-user
    user:
      token: alpha-token
  - name: beta-user
    user:
      token: beta-token
`

// Points client-go at a kubeconfig written for this test only.
func useTestKubeconfig(t *testing.T, contents string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing the test kubeconfig failed: %v", err)
	}
	t.Setenv("KUBECONFIG", path)
}

func TestRegistryResolvesEachContextToItsOwnCluster(t *testing.T) {
	useTestKubeconfig(t, twoClusterKubeconfig)

	registry, err := newClientRegistry()
	if err != nil {
		t.Fatalf("building the registry failed: %v", err)
	}

	for name, want := range map[string]string{
		"":      "https://alpha.example:6443", // no context named: the kubeconfig's own default
		"alpha": "https://alpha.example:6443",
		"beta":  "https://beta.example:6443",
	} {
		clients, err := registry.forContext(name)
		if err != nil {
			t.Fatalf("resolving context %q failed: %v", name, err)
		}
		if clients.rest.Host != want {
			t.Errorf("context %q resolved to host %q, want %q", name, clients.rest.Host, want)
		}
	}
}

// The reason this registry exists. Resolving one context used to overwrite the clients every
// request shared, so a request for "beta" changed which cluster a concurrent request for "alpha"
// was reading. Each resolution must be independent of the ones around it.
func TestResolvingOneContextDoesNotAffectAnother(t *testing.T) {
	useTestKubeconfig(t, twoClusterKubeconfig)

	registry, err := newClientRegistry()
	if err != nil {
		t.Fatalf("building the registry failed: %v", err)
	}

	alpha, err := registry.forContext("alpha")
	if err != nil {
		t.Fatalf("resolving alpha failed: %v", err)
	}

	if _, err := registry.forContext("beta"); err != nil {
		t.Fatalf("resolving beta failed: %v", err)
	}

	if alpha.rest.Host != "https://alpha.example:6443" {
		t.Errorf("resolving beta moved the alpha clients to %q", alpha.rest.Host)
	}

	// And the same is true of the clients a request with no context in its path gets.
	def, err := registry.forContext(defaultKubeContext)
	if err != nil {
		t.Fatalf("resolving the default context failed: %v", err)
	}
	if def.rest.Host != "https://alpha.example:6443" {
		t.Errorf("the default context resolved to %q after resolving beta", def.rest.Host)
	}
}

func TestRegistryCachesTheClientsPerContext(t *testing.T) {
	useTestKubeconfig(t, twoClusterKubeconfig)

	registry, err := newClientRegistry()
	if err != nil {
		t.Fatalf("building the registry failed: %v", err)
	}

	first, err := registry.forContext("beta")
	if err != nil {
		t.Fatalf("resolving beta failed: %v", err)
	}
	second, err := registry.forContext("beta")
	if err != nil {
		t.Fatalf("resolving beta a second time failed: %v", err)
	}

	// Pointer equality: every set owns an http.Transport that nothing closes, so rebuilding one
	// per request would leak a connection pool per request.
	if first != second {
		t.Error("resolving the same context twice built two sets of clients")
	}
}

func TestRegistryRejectsAnUnknownContext(t *testing.T) {
	useTestKubeconfig(t, twoClusterKubeconfig)

	registry, err := newClientRegistry()
	if err != nil {
		t.Fatalf("building the registry failed: %v", err)
	}

	_, err = registry.forContext("does-not-exist")
	if err == nil {
		t.Fatal("resolving a context that is not in the kubeconfig returned no error")
	}
	// The name has to be rejected against the kubeconfig rather than left to fail somewhere inside
	// client-go: the message reaches the user, and a client-go failure is not guaranteed (with no
	// kubeconfig at all it falls back to the in-cluster client and any name would "work").
	if want := "context 'does-not-exist' not found in Kubeconfig file"; err.Error() != want {
		t.Errorf("got error %q, want %q", err, want)
	}
}

// Run with -race, this fails if the map is touched without holding the lock.
func TestRegistryIsSafeForConcurrentUse(t *testing.T) {
	useTestKubeconfig(t, twoClusterKubeconfig)

	registry, err := newClientRegistry()
	if err != nil {
		t.Fatalf("building the registry failed: %v", err)
	}

	names := []string{"", "alpha", "beta", "does-not-exist"}
	hosts := map[string]string{
		"":      "https://alpha.example:6443",
		"alpha": "https://alpha.example:6443",
		"beta":  "https://beta.example:6443",
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		for _, name := range names {
			wg.Add(1)
			go func(name string) {
				defer wg.Done()

				clients, err := registry.forContext(name)
				want, known := hosts[name]
				if !known {
					if err == nil {
						t.Errorf("resolving unknown context %q returned no error", name)
					}
					return
				}
				if err != nil {
					t.Errorf("resolving context %q failed: %v", name, err)
					return
				}
				if clients.rest.Host != want {
					t.Errorf("context %q resolved to host %q, want %q", name, clients.rest.Host, want)
				}
			}(name)
		}
	}
	wg.Wait()
}

// Pins the wiring between the route table and the registry: the handlers take the cluster to read
// from the `:context` path parameter, and a request without one gets the kubeconfig's default.
func TestHandlersResolveTheContextFromThePath(t *testing.T) {
	useTestKubeconfig(t, twoClusterKubeconfig)

	registry, err := newClientRegistry()
	if err != nil {
		t.Fatalf("building the registry failed: %v", err)
	}
	s := &server{k8s: registry}

	e := echo.New()
	e.GET("/api/v1/configs", func(echo.Context) error { return nil })
	e.GET("/api/v1/configs/:context", func(echo.Context) error { return nil })

	for path, want := range map[string]string{
		"/api/v1/configs":      "https://alpha.example:6443",
		"/api/v1/configs/beta": "https://beta.example:6443",
	} {
		c := e.NewContext(httptest.NewRequest(http.MethodGet, path, nil), httptest.NewRecorder())
		e.Router().Find(http.MethodGet, path, c)

		clients, err := s.clientsFor(c)
		if err != nil {
			t.Fatalf("resolving the clients for %s failed: %v", path, err)
		}
		if clients.rest.Host != want {
			t.Errorf("%s resolved to host %q, want %q", path, clients.rest.Host, want)
		}
	}
}

func TestRegistryReportsTheKubeconfigContexts(t *testing.T) {
	useTestKubeconfig(t, twoClusterKubeconfig)

	registry, err := newClientRegistry()
	if err != nil {
		t.Fatalf("building the registry failed: %v", err)
	}

	contexts, current := registry.contexts()
	if current != "alpha" {
		t.Errorf("current context is %q, want alpha", current)
	}
	if len(contexts) != 2 {
		t.Fatalf("got %d contexts, want 2", len(contexts))
	}
	if contexts["beta"].Cluster != "beta-cluster" {
		t.Errorf("context beta points at cluster %q, want beta-cluster", contexts["beta"].Cluster)
	}

	// Resolving a context must not change what the kubeconfig reports, which is what the context
	// switcher in the UI is built from.
	if _, err := registry.forContext("beta"); err != nil {
		t.Fatalf("resolving beta failed: %v", err)
	}
	if _, current := registry.contexts(); current != "alpha" {
		t.Errorf("resolving beta changed the reported current context to %q", current)
	}
}

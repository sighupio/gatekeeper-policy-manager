// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/spf13/viper"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// The key the kubeconfig's own current-context is cached under. Requests that name no context get
// this one.
const defaultKubeContext = ""

// Client-side API rate limits, well above client-go's 5/10 default so GPM's read-only, list-heavy
// views do not serialize behind the limiter. The apiserver's own flow control is the real guard.
const (
	kubeClientQPS   = 50
	kubeClientBurst = 100
)

// Everything needed to talk to one cluster. The client-go clients are safe for concurrent use, so
// a single set is shared by every request targeting the same kubeconfig context.
type kubeClients struct {
	dynamic   *dynamic.DynamicClient
	discovery *discovery.DiscoveryClient
	rest      *rest.Config
}

// Builds a kubeClients per kubeconfig context and keeps them for the process lifetime.
//
// Requests used to reach a named context by reassigning package-level clients. That raced between
// concurrent requests, and one user switching context changed which cluster another user's
// in-flight request was reading. Nothing is reassigned here: a request resolves the set it needs
// and the registry owns them.
//
// Caching is not only an optimisation. Each set owns an http.Transport with its own connection
// pool and nothing ever closes it, so rebuilding per request would leak connections. The number of
// entries is bounded by the number of contexts in the kubeconfig.
type clientRegistry struct {
	mu      sync.RWMutex
	clients map[string]*kubeClients

	// The parsed kubeconfig. Identical whichever context is selected, so it is read once at
	// startup and never replaced. Treat as read-only.
	kubeconfig *api.Config
}

// Loads the kubeconfig and prepares the clients for its current context. Fails if no cluster can
// be reached at all, which is fatal for GPM.
func newClientRegistry() (*clientRegistry, error) {
	clients, kubeconfig, err := buildKubeClients(defaultKubeContext)
	if err != nil {
		return nil, err
	}

	return &clientRegistry{
		clients:    map[string]*kubeClients{defaultKubeContext: clients},
		kubeconfig: kubeconfig,
	}, nil
}

// Returns the clients for a kubeconfig context, building them on first use. An empty name means
// the kubeconfig's current context.
func (r *clientRegistry) forContext(name string) (*kubeClients, error) {
	r.mu.RLock()
	clients, cached := r.clients[name]
	r.mu.RUnlock()
	if cached {
		return clients, nil
	}

	if _, known := r.kubeconfig.Contexts[name]; !known {
		return nil, fmt.Errorf("context '%s' not found in Kubeconfig file", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// Another request may have built this while we waited for the write lock.
	if clients, cached := r.clients[name]; cached {
		return clients, nil
	}

	slog.Debug("building Kubernetes clients for context", "context", name)
	clients, _, err := buildKubeClients(name)
	if err != nil {
		return nil, err
	}
	r.clients[name] = clients

	return clients, nil
}

// The context names available in the kubeconfig, and which one is its default.
func (r *clientRegistry) contexts() (map[string]*api.Context, string) {
	return r.kubeconfig.Contexts, r.kubeconfig.CurrentContext
}

// Creates the clients for one kubeconfig context, or an in-cluster client when there is no
// kubeconfig. An empty context means the kubeconfig's default.
//
// This returns everything it builds and assigns nothing: an earlier version assigned the
// package-level discovery client as a side effect while also returning values, which is how the
// clients ended up shared between requests in the first place.
func buildKubeClients(kubeContext string) (*kubeClients, *api.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{CurrentContext: kubeContext}
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	slog.Info("trying to load kubeconfigs", "paths", loader.ConfigAccess().GetLoadingPrecedence())
	restConfig, err := loader.ClientConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("creating Kubernetes client failed: %w", err)
	}

	if viper.GetBool("skip_tls_verify") {
		slog.Warn("GPM_SKIP_TLS_VERIFY is set: disabling TLS certificate verification for Kubernetes API calls")
		// These fields belong to the embedded TLSClientConfig. client-go rejects a config that
		// is insecure and carries a CA at the same time, so the CA has to go with it.
		restConfig.Insecure = true
		restConfig.CAFile = ""
		restConfig.CAData = nil
	}

	// client-go defaults to a 5 QPS / 10 burst client-side rate limiter. The constraints view and the
	// dashboard list every Constraint Kind (a cluster can have dozens), so at 5 QPS those calls
	// serialize into seconds against a remote cluster. GPM is a read-only UI; raise the ceiling and
	// let the apiserver's own flow control (APF) protect it.
	restConfig.QPS = kubeClientQPS
	restConfig.Burst = kubeClientBurst

	kubeconfig, err := loader.ConfigAccess().GetStartingConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("getting contexts information from Kubeconfig failed: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("creating dynamic Kubernetes client failed: %w", err)
	}

	// Used to discover the Constraint kinds, which Gatekeeper creates per template.
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("creating constraints discovery Kubernetes client failed: %w", err)
	}

	return &kubeClients{dynamic: dynamicClient, discovery: discoveryClient, rest: restConfig}, kubeconfig, nil
}

package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/bnjoroge/docktree/internal/ports"
	"github.com/bnjoroge/docktree/internal/state"
)

// Router maps worktree instance names to their allocated backend ports
// and proxies HTTP requests by Host header.
type Router struct {
	mu     sync.RWMutex
	routes map[string]route // instance-name → backend
}

type route struct {
	backend string // "127.0.0.1:41237"
	branch  string
}

// NewRouter creates an empty Router.
func NewRouter() *Router {
	return &Router{routes: make(map[string]route)}
}

// Refresh re-reads global instances and port assignments to rebuild routing.
func (r *Router) Refresh() error {
	instances, err := state.LoadGlobalInstances("")
	if err != nil {
		return fmt.Errorf("load instances: %w", err)
	}
	registry, err := ports.NewRegistry().Load()
	if err != nil {
		return fmt.Errorf("load ports: %w", err)
	}

	newRoutes := make(map[string]route, len(instances))
	for name, inst := range instances {
		assignments := registry[name]
		// Prefer HTTP ports (80, 8080) over HTTPS (443), over arbitrary ports.
		var best *ports.Assignment
		for _, a := range assignments {
			a := a // capture loop variable
			if a.HostPort > 0 {
				if best == nil {
					best = &a
					continue
				}
				isHTTP := a.ContainerPort == 80 || a.ContainerPort == 8080
				bestIsHTTP := best.ContainerPort == 80 || best.ContainerPort == 8080
				if isHTTP && !bestIsHTTP {
					best = &a
				}
			}
		}
		if best != nil {
			host := best.HostIP
				if host == "" {
					host = "127.0.0.1"
			}
				newRoutes[name] = route{
				backend: fmt.Sprintf("%s:%d", host, best.HostPort),
					branch:  inst.Branch,
			}
		}
	}


	r.mu.Lock()
	r.routes = newRoutes
	r.mu.Unlock()
	return nil
}

// ServeHTTP routes by Host header: extract instance name from
// "<name>.localhost" and proxy to the matching backend.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	host := req.Host
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	host = strings.TrimSuffix(host, ".localhost")

	r.mu.RLock()
	rt, ok := r.routes[host]
	r.mu.RUnlock()

	if !ok {
		r.writeAvailable(w, host)
		return
	}

	target := &url.URL{
		Scheme: "http",
		Host:   rt.backend,
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ServeHTTP(w, req)
}

// writeAvailable returns a 404 with a JSON list of available instances.
func (r *Router) writeAvailable(w http.ResponseWriter, requested string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	type entry struct {
		Name   string `json:"name"`
		Branch string `json:"branch"`
		URL    string `json:"url"`
	}
	entries := make([]entry, 0, len(r.routes))
	for name, rt := range r.routes {
		entries = append(entries, entry{
			Name:   name,
			Branch: rt.branch,
			URL:    fmt.Sprintf("http://%s.localhost", name),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]any{
		"error":     fmt.Sprintf("no instance %q", requested),
		"available": entries,
	})
}

// Routes returns a snapshot of current routes for display.
func (r *Router) Routes() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.routes))
	for name, rt := range r.routes {
		out[name] = rt.backend
	}
	return out
}

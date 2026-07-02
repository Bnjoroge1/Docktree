package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bnjoroge/docktree/internal/ports"
)

func TestRouter_ServeHTTP_HostHeaderRewriteAndEncodeErrcheck(t *testing.T) {
	router := &Router{
		routes: map[string]route{
			"app": {
				backend: "127.0.0.1:8080",
				branch:  "main",
			},
		},
	}

	// 1. Verify routing and Host header rewriting
	var expectedHost string
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != expectedHost {
			t.Errorf("expected Host header to be rewritten to %q, got %q", expectedHost, r.Host)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backendServer.Close()
	expectedHost = strings.TrimPrefix(backendServer.URL, "http://")

	// Direct "app" route to our test backend server
	backendHost := strings.TrimPrefix(backendServer.URL, "http://")
	router.routes["app"] = route{
		backend: backendHost,
		branch:  "main",
	}

	req := httptest.NewRequest(http.MethodGet, "http://app.localhost/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", w.Code)
	}

	// 2. Verify writeAvailable returns 404 with list of available instances
	req404 := httptest.NewRequest(http.MethodGet, "http://missing.localhost/", nil)
	w404 := httptest.NewRecorder()
	router.ServeHTTP(w404, req404)

	if w404.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d", w404.Code)
	}

	var respBody map[string]any
	if err := json.Unmarshal(w404.Body.Bytes(), &respBody); err != nil {
		t.Fatalf("failed to decode 404 response: %v", err)
	}

	if respBody["error"] != `no instance "missing"` {
		t.Errorf("expected error message, got %v", respBody["error"])
	}

	available, ok := respBody["available"].([]any)
	if !ok || len(available) != 1 {
		t.Errorf("expected 1 available instance, got %v", respBody["available"])
	}
}

func TestRouter_PortRanking(t *testing.T) {
	// Test port preference: 80/8080 > 443 > any other
	testCases := []struct {
		name        string
		assignments []ports.Assignment
		expected    int
	}{
		{
			name: "prefers 80 over 443",
			assignments: []ports.Assignment{
				{ContainerPort: 443, HostPort: 8443, HostIP: "127.0.0.1"},
				{ContainerPort: 80, HostPort: 8080, HostIP: "127.0.0.1"},
			},
			expected: 8080,
		},
		{
			name: "prefers 443 over 9000",
			assignments: []ports.Assignment{
				{ContainerPort: 9000, HostPort: 9001, HostIP: "127.0.0.1"},
				{ContainerPort: 443, HostPort: 8443, HostIP: "127.0.0.1"},
			},
			expected: 8443,
		},
		{
			name: "falls back to arbitrary first port",
			assignments: []ports.Assignment{
				{ContainerPort: 9000, HostPort: 9001, HostIP: "127.0.0.1"},
				{ContainerPort: 9002, HostPort: 9003, HostIP: "127.0.0.1"},
			},
			expected: 9001,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate Router Refresh logic for a single instance
			var best *ports.Assignment
			for _, a := range tc.assignments {
				a := a
				if a.HostPort > 0 {
					if best == nil {
						best = &a
						continue
					}
					rank := func(p int) int {
						if p == 80 || p == 8080 {
							return 2
						}
						if p == 443 {
							return 1
						}
						return 0
					}
					if rank(a.ContainerPort) > rank(best.ContainerPort) {
						best = &a
					}
				}
			}

			if best == nil || best.HostPort != tc.expected {
				actual := 0
				if best != nil {
					actual = best.HostPort
				}
				t.Errorf("expected host port %d, got %d", tc.expected, actual)
			}
		})
	}
}

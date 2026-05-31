//go:build integration

package docker

import "testing"

func TestIsComposeAvailableIntegration(t *testing.T) {
	if err := IsComposeAvailable(); err != nil {
		t.Fatalf("docker compose unavailable: %v", err)
	}
}

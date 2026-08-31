package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bnjoroge/docktree/internal/docker"
	"github.com/bnjoroge/docktree/internal/ports"
	"github.com/bnjoroge/docktree/internal/state"
)

func TestIsWorktreeInstanceName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"typical instance", "repo-feature-abc123", true},
		{"repo and branch contain dashes", "my-repo-feature-auth-abc123", true},
		{"platform tier is excluded", "docktree-platform-myrepo", false},
		{"platform tier with hex-like slug", "docktree-platform-abcd123", false},
		{"missing hash suffix", "repo-feature", false},
		{"hash too short", "repo-feature-abcd1", false},
		{"hash not hex", "repo-feature-ghijkl", false},
		{"uppercase hash", "repo-feature-ABC123", false},
		{"plain compose project", "myapp", false},
		{"single dash segment", "app-abcdef", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isWorktreeInstanceName(tt.in); got != tt.want {
				t.Fatalf("isWorktreeInstanceName(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestDiscoverCleanCandidatesFindsOrphanedResourcesWithoutStateOrPorts(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
args="$*"
case "$1 $2" in
  "ps -a")
    exit 0
    ;;
  "network ls")
    printf 'repo-feature-abc123_default\tbridge\tcom.docker.compose.project=repo-feature-abc123\n'
    ;;
  "volume ls")
    printf 'repo-feature-abc123_data\tlocal\tcom.docker.compose.project=repo-feature-abc123\n'
    ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))

	registry := ports.NewRegistry()
	if err := registry.Lock(); err != nil {
		t.Fatal(err)
	}
	defer registry.Unlock()

	candidates, err := discoverCleanCandidates(registry, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %#v", len(candidates), candidates)
	}
	candidate := candidates[0]
	if candidate.Name != "repo-feature-abc123" {
		t.Fatalf("unexpected candidate name %q", candidate.Name)
	}
	if candidate.StateFound {
		t.Fatal("expected no state record for orphan")
	}
	if candidate.Ports != 0 {
		t.Fatalf("expected no port claims, got %d", candidate.Ports)
	}
	if candidate.Reason != "orphaned resources" {
		t.Fatalf("unexpected reason %q", candidate.Reason)
	}
	if len(candidate.Resources.Networks) != 1 || len(candidate.Resources.Volumes) != 1 {
		t.Fatalf("unexpected resources: %#v", candidate.Resources)
	}
}

func TestDiscoverCleanCandidatesIgnoresForeignAndPlatformProjects(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
case "$1 $2" in
  "ps -a")
    exit 0
    ;;
  "network ls")
    printf 'myapp_default\tbridge\tcom.docker.compose.project=myapp\n'
    printf 'docktree-platform-myapp_default\tbridge\tcom.docker.compose.project=docktree-platform-myapp\n'
    ;;
  "volume ls")
    printf 'myapp_data\tlocal\tcom.docker.compose.project=myapp\n'
    ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))

	registry := ports.NewRegistry()
	if err := registry.Lock(); err != nil {
		t.Fatal(err)
	}
	defer registry.Unlock()

	candidates, err := discoverCleanCandidates(registry, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected no candidates, got %d: %#v", len(candidates), candidates)
	}
}

func TestApplyCleanCandidatesKeepsClaimsWhenRemovalFails(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
case "$1 $2" in
  "ps -a")
    printf 'c1\tinst-web\n'
    ;;
  "network ls")
    exit 0
    ;;
  "volume ls")
    exit 0
    ;;
  "rm -f")
    exit 1
    ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	configDir := filepath.Join(root, "config")
	t.Setenv("XDG_CONFIG_HOME", configDir)

	registry := ports.NewRegistry()
	if err := registry.Save(map[string][]ports.Assignment{
		"repo-feature-abc123": {{Service: "web", ContainerPort: 80, HostIP: "127.0.0.1", HostPort: 41000}},
	}); err != nil {
		t.Fatal(err)
	}
	inst := state.Instance{
		Name:         "repo-feature-abc123",
		ProjectName:  "repo-feature-abc123",
		RepoRoot:     filepath.Join(root, "gone-repo"),
		WorktreeRoot: filepath.Join(root, "gone-worktree"),
		LastActiveAt: time.Now().UTC(),
		ComposeFiles: []string{"compose.yml"},
	}
	if err := state.UpsertGlobalInstance("", &inst); err != nil {
		t.Fatal(err)
	}

	candidates := []cleanCandidate{{
		Name:       "repo-feature-abc123",
		Reason:     "orphaned resources",
		Resources:  docker.ProjectResources{Containers: []docker.Resource{{ID: "c1", Name: "inst-web"}}},
		StateFound: true,
		Instance:   &inst,
	}}
	applied, err := applyCleanCandidates(registry, candidates, false)
	if err == nil {
		t.Fatal("expected error from failed removal")
	}
	if len(applied) != 0 {
		t.Fatalf("expected no applied candidates, got %#v", applied)
	}

	// The port claim must survive a failed removal so a later clean can
	// rediscover and retry the instance.
	portMap, err := registry.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := portMap["repo-feature-abc123"]; !ok {
		t.Fatalf("port claim was retired despite failed removal: %#v", portMap)
	}
	instances, err := state.LoadGlobalInstances("")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := instances["repo-feature-abc123"]; !ok {
		t.Fatalf("global instance was removed despite failed removal: %#v", instances)
	}
}

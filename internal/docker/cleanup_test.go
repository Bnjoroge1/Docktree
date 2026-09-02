package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListAndRemoveProjectResources(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "docker.log")
	script := filepath.Join(dir, "docker")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
case "$1 $2" in
  "ps -a")
    printf 'c1\tinst-web\nc2\tinst-api\n'
    ;;
  "network ls")
    printf 'n1\tinst_default\n'
    ;;
  "volume ls")
    printf 'inst-data\n'
    ;;
  "rm -f")
    printf '%s\n' "$*" >> "$DOCKER_TEST_LOG"
    ;;
  "network rm")
    printf '%s\n' "$*" >> "$DOCKER_TEST_LOG"
    ;;
  "volume rm")
    printf '%s\n' "$*" >> "$DOCKER_TEST_LOG"
    ;;
  *)
    exit 0
    ;;
esac
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKER_TEST_LOG", logPath)

	resources, err := ListProjectResources("inst", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources.Containers) != 2 || len(resources.Networks) != 1 || len(resources.Volumes) != 1 {
		t.Fatalf("unexpected resources: %#v", resources)
	}
	removed, err := RemoveProjectResources("inst", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.Containers) != 2 || len(removed.Networks) != 1 || len(removed.Volumes) != 1 {
		t.Fatalf("unexpected removed resources: %#v", removed)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logData)
	for _, want := range []string{"rm -f c1", "rm -f c2", "network rm inst_default", "volume rm -f inst-data"} {
		if !containsLine(logText, want) {
			t.Fatalf("missing %q in log:\n%s", want, logText)
		}
	}
}

func TestRemoveProjectResourcesAccumulatesFailures(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "docker")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
case "$1 $2" in
  "ps -a")
    printf 'c1\tinst-web\nc2\tinst-api\n'
    ;;
  "network ls")
    printf 'n1\tinst_default\nn2\tinst_extra\n'
    ;;
  "volume ls")
    printf 'inst-data\n'
    ;;
  "rm -f")
    case "$3" in
      c1) exit 1 ;;
    esac
    printf '%s\n' "$*" >> "$DOCKER_TEST_LOG"
    ;;
  "network rm")
    case "$3" in
      inst_default) exit 1 ;;
    esac
    printf '%s\n' "$*" >> "$DOCKER_TEST_LOG"
    ;;
  "volume rm")
    printf '%s\n' "$*" >> "$DOCKER_TEST_LOG"
    ;;
  *)
    exit 0
    ;;
esac
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKER_TEST_LOG", filepath.Join(dir, "docker.log"))

	removed, err := RemoveProjectResources("inst", true)
	if err == nil {
		t.Fatal("expected aggregate error, got nil")
	}
	if !strings.Contains(err.Error(), "remove container inst-web") {
		t.Fatalf("error missing container failure: %v", err)
	}
	if !strings.Contains(err.Error(), "remove network inst_default") {
		t.Fatalf("error missing network failure: %v", err)
	}
	if len(removed.Containers) != 2 || len(removed.Networks) != 2 || len(removed.Volumes) != 1 {
		t.Fatalf("unexpected removed resources: %#v", removed)
	}
	logData, err := os.ReadFile(filepath.Join(dir, "docker.log"))
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logData)
	// The failing container and network were skipped, but every other resource
	// (including the volume after the failed network) was still attempted.
	for _, want := range []string{"rm -f c2", "network rm inst_extra", "volume rm -f inst-data"} {
		if !containsLine(logText, want) {
			t.Fatalf("missing %q in log:\n%s", want, logText)
		}
	}
	if containsLine(logText, "rm -f c1") || containsLine(logText, "network rm inst_default") {
		t.Fatalf("failed resource logged as removed:\n%s", logText)
	}
}

func TestNetworkPruneHelpers(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "docker.log")
	script := filepath.Join(dir, "docker")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
case "$1 $2" in
  "network ls")
    printf 'n1\nn2\nn3\n'
    ;;
  "network prune")
    printf '%s\n' "$*" >> "$DOCKER_TEST_LOG"
    ;;
  *)
    exit 0
    ;;
esac
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKER_TEST_LOG", logPath)

	count, err := CountBridgeNetworks()
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("CountBridgeNetworks() = %d, want 3", count)
	}
	if err := PruneUnusedNetworks(); err != nil {
		t.Fatal(err)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !containsLine(string(logData), "network prune --force") {
		t.Fatalf("missing network prune command in log:\n%s", string(logData))
	}
}

func TestListDocktreeProjects(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "docker")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
if [ "$1 $2" = "ps -a" ]; then
  printf 'docktree.managed=true,docktree.instance=alpha,com.docker.compose.project=alpha\n'
  printf 'docktree.managed=true,docktree.instance=beta,com.docker.compose.project=beta\n'
  printf 'docktree.managed=true,docktree.tier=platform,com.docker.compose.project=docktree-platform-myrepo\n'
  printf 'com.docker.compose.project=foreign-project-abc123\n'
fi
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	projects, err := ListDocktreeProjects()
	if err != nil {
		t.Fatal(err)
	}
	// Only resources carrying docktree.instance are managed projects; the
	// platform tier and foreign compose projects are excluded.
	if len(projects) != 2 || projects[0] != "alpha" || projects[1] != "beta" {
		t.Fatalf("unexpected projects: %#v", projects)
	}
}

func containsLine(text, want string) bool {
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line == want {
			return true
		}
	}
	return false
}

func TestListDocktreeVolumes(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "docker")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
if [ "$1 $2" = "volume ls" ]; then
  printf 'vol1\tlocal\tdocktree.managed=true,docktree.instance=alpha,com.docker.compose.project=alpha,com.docker.compose.volume=db_data\n'
  printf 'vol2\tlocal\tdocktree.managed=true,docktree.instance=beta,com.docker.compose.project=beta\n'
  printf 'vol3\tlocal\tno-docktree-label=true\n'
  printf 'vol4\tlocal\tdocktree.managed=true,docktree.tier=platform,com.docker.compose.project=docktree-platform-myrepo\n'
  printf 'vol5\tlocal\tcom.docker.compose.project=foreign-abc123\n'
fi
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	volumes, err := ListDocktreeVolumes()
	if err != nil {
		t.Fatal(err)
	}
	// vol3 carries no docktree label, vol4 is platform tier, and vol5 is a
	// foreign compose project with only the generic project label — none are
	// Docktree-owned, so only vol1/vol2 are reported.
	if len(volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d: %#v", len(volumes), volumes)
	}
	if volumes[0].Name != "vol1" || volumes[0].ProjectName != "alpha" || volumes[0].VolumeName != "db_data" || volumes[0].Driver != "local" {
		t.Errorf("unexpected volume 0: %#v", volumes[0])
	}
	if volumes[1].Name != "vol2" || volumes[1].ProjectName != "beta" || volumes[1].VolumeName != "vol2" || volumes[1].Driver != "local" {
		t.Errorf("unexpected volume 1: %#v", volumes[1])
	}
}

func TestListDocktreeNetworks(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "docker")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
if [ "$1 $2" = "network ls" ]; then
  printf 'net1\tbridge\tdocktree.managed=true,docktree.instance=alpha,com.docker.compose.project=alpha\n'
  printf 'net2\tbridge\tdocktree.instance=beta\n'
  printf 'net3\tbridge\tno-docktree-label=true\n'
  printf 'net4\tbridge\tdocktree.managed=true,docktree.tier=platform,com.docker.compose.project=docktree-platform-myrepo\n'
  printf 'net5\tbridge\tcom.docker.compose.project=foreign-abc123\n'
fi
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	networks, err := ListDocktreeNetworks()
	if err != nil {
		t.Fatal(err)
	}
	// net3 has no docktree label, net4 is platform tier, and net5 is a
	// foreign compose project — only net1/net2 are Docktree-owned.
	if len(networks) != 2 {
		t.Fatalf("expected 2 networks, got %d: %#v", len(networks), networks)
	}
	if networks[0].Name != "net1" || networks[0].ProjectName != "alpha" || networks[0].Driver != "bridge" {
		t.Errorf("unexpected network 0: %#v", networks[0])
	}
	if networks[1].Name != "net2" || networks[1].ProjectName != "beta" || networks[1].Driver != "bridge" {
		t.Errorf("unexpected network 1: %#v", networks[1])
	}
}

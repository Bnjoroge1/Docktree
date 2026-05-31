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
	for _, want := range []string{"rm -f c1 c2", "network rm inst_default", "volume rm -f inst-data"} {
		if !containsLine(logText, want) {
			t.Fatalf("missing %q in log:\n%s", want, logText)
		}
	}
}

func TestListDocktreeProjects(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "docker")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
if [ "$1 $2" = "ps -a" ]; then
  printf 'docktree.managed=true,docktree.instance=alpha,com.docker.compose.project=alpha\n'
  printf 'docktree.managed=true,docktree.instance=beta,com.docker.compose.project=beta\n'
fi
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	projects, err := ListDocktreeProjects()
	if err != nil {
		t.Fatal(err)
	}
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

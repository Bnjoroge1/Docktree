//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bnjoroge/docktree/internal/cli"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/ports"
	"github.com/bnjoroge/docktree/internal/state"
)

func TestCommandFlowWithFakeDockerAndTwoWorktrees(t *testing.T) {
	sourceCompose, err := filepath.Abs(filepath.Join("..", "testdata", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	copyFile(t, sourceCompose, filepath.Join(repo, "compose.yml"))
	run(t, repo, "git", "init", "-b", "main")
	run(t, repo, "git", "config", "user.email", "docktree@example.test")
	run(t, repo, "git", "config", "user.name", "Docktree Test")
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "-m", "init")

	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(root, "docker-state")
	writeFakeDocker(t, filepath.Join(fakeBin, "docker"), stateFile)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))

	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}

	if code, stdout, errText := runCLI("init", "--json"); code != output.ExitOK || errText != "" || !json.Valid([]byte(stdout)) {
		t.Fatalf("init code=%d err=%s", code, errText)
	}
	if code, stdout, errText := runCLI("up", "-h"); code != output.ExitOK || errText != "" || !strings.Contains(stdout, "--sync") || !strings.Contains(stdout, "--create") {
		t.Fatalf("up help code=%d err=%s out=%s", code, errText, stdout)
	}
	if _, err := os.Stat(filepath.Join(repo, ".docktree", "generated")); err != nil {
		t.Fatalf("state dir not created: %v", err)
	}
	firstProject := ""
	if code, stdout, errText := runCLI("up", "--json"); code != output.ExitOK || errText != "" || !json.Valid([]byte(stdout)) {
		t.Fatalf("up code=%d err=%s", code, errText)
	} else {
		var firstUp upJSON
		if err := json.Unmarshal([]byte(stdout), &firstUp); err != nil {
			t.Fatal(err)
		}
		firstProject = firstUp.Instance.ProjectName
	}
	if code, _, _ := runCLI("up"); code != output.ExitNoop {
		t.Fatalf("second up code=%d, want noop", code)
	}
	code, stdout, errText := runCLI("ports", "--json")
	if code != output.ExitOK || errText != "" {
		t.Fatalf("ports code=%d err=%s", code, errText)
	}
	var portsResult struct {
		Ports []struct {
			HostPort int `json:"host_port"`
		} `json:"ports"`
	}
	if err := json.Unmarshal([]byte(stdout), &portsResult); err != nil {
		t.Fatalf("ports output not json: %v\n%s", err, stdout)
	}
	if len(portsResult.Ports) != 2 {
		t.Fatalf("expected two ports: %#v", portsResult)
	}
	for _, port := range portsResult.Ports {
		if port.HostPort == 8080 || port.HostPort == 3000 {
			t.Fatalf("host port was not remapped: %#v", portsResult)
		}
	}
	if code, stdout, errText = runCLI("status", "--json"); code != output.ExitOK || errText != "" || !json.Valid([]byte(stdout)) {
		t.Fatalf("status code=%d valid=%v err=%s out=%s", code, json.Valid([]byte(stdout)), errText, stdout)
	}
	if code, stdout, errText = runCLI("down", "--json"); code != output.ExitOK || errText != "" || !json.Valid([]byte(stdout)) {
		t.Fatalf("down code=%d err=%s", code, errText)
	}
	if code, _, _ = runCLI("down"); code != output.ExitNoop {
		t.Fatalf("second down code=%d, want noop", code)
	}
	firstPorts := map[int]bool{}
	for _, port := range portsResult.Ports {
		firstPorts[port.HostPort] = true
	}

	worktree := filepath.Join(root, "repo-feature")
	run(t, repo, "git", "worktree", "add", "-b", "feature/auth", worktree)
	copyFile(t, sourceCompose, filepath.Join(worktree, "compose.yml"))
	if err := os.Chdir(worktree); err != nil {
		t.Fatal(err)
	}
	code, stdout, errText = runCLI("up", "--json")
	if code != output.ExitOK || errText != "" || !json.Valid([]byte(stdout)) {
		t.Fatalf("worktree up code=%d err=%s", code, errText)
	}
	var secondUp upJSON
	if err := json.Unmarshal([]byte(stdout), &secondUp); err != nil {
		t.Fatal(err)
	}
	if secondUp.Instance.ProjectName == "" {
		t.Fatalf("missing second instance name: %s", stdout)
	}
	if secondUp.Instance.ProjectName == firstProject {
		t.Fatalf("second worktree reused first project name %q", firstProject)
	}
	for _, port := range secondUp.Ports {
		if firstPorts[port.HostPort] {
			t.Fatalf("second worktree reused first worktree port %d", port.HostPort)
		}
	}
	overrideFiles, err := filepath.Glob(filepath.Join(worktree, ".docktree", "generated", "*.override.yml"))
	if err != nil || len(overrideFiles) != 1 {
		t.Fatalf("override files = %#v err=%v", overrideFiles, err)
	}
	data, err := os.ReadFile(overrideFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "feature-auth") || strings.Contains(string(data), "myapp_web") || !strings.Contains(string(data), "docktree/") {
		t.Fatalf("override did not rewrite names correctly:\n%s", data)
	}
}

func TestUpSyncRunsSetupInPlace(t *testing.T) {
	sourceCompose, err := filepath.Abs(filepath.Join("..", "testdata", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	copyFile(t, sourceCompose, filepath.Join(repo, "compose.yml"))
	if err := os.WriteFile(filepath.Join(repo, "docktree.yml"), []byte("setup:\n  run:\n    - printf sync > synced.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "git", "init", "-b", "main")
	run(t, repo, "git", "config", "user.email", "docktree@example.test")
	run(t, repo, "git", "config", "user.name", "Docktree Test")
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "-m", "init")

	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(root, "docker-state")
	writeFakeDocker(t, filepath.Join(fakeBin, "docker"), stateFile)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))

	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}

	code, stdout, errText := runCLI("up", "--sync", "--json")
	if code != output.ExitOK || errText != "" || !json.Valid([]byte(stdout)) {
		t.Fatalf("sync up code=%d err=%s out=%s", code, errText, stdout)
	}
	var syncedUp upJSON
	if err := json.Unmarshal([]byte(stdout), &syncedUp); err != nil {
		t.Fatal(err)
	}
	if syncedUp.Instance.ProjectName == "" {
		t.Fatalf("unexpected sync json shape: %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(repo, "synced.txt")); err != nil {
		t.Fatalf("sync marker missing: %v", err)
	}
}

func TestCleanCommandWithFakeDockerState(t *testing.T) {
	sourceCompose, err := filepath.Abs(filepath.Join("..", "testdata", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	copyFile(t, sourceCompose, filepath.Join(repo, "compose.yml"))
	run(t, repo, "git", "init", "-b", "main")
	run(t, repo, "git", "config", "user.email", "docktree@example.test")
	run(t, repo, "git", "config", "user.name", "Docktree Test")
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "-m", "init")

	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(root, "docker-state")
	writeFakeDocker(t, filepath.Join(fakeBin, "docker"), stateFile)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))

	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	code, stdout, errText := runCLI("up", "--json")
	if code != output.ExitOK || errText != "" || !json.Valid([]byte(stdout)) {
		t.Fatalf("up code=%d err=%s", code, errText)
	}
	var up upJSON
	if err := json.Unmarshal([]byte(stdout), &up); err != nil {
		t.Fatal(err)
	}
	project := up.Instance.ProjectName
	if project == "" {
		t.Fatalf("missing project in up result: %s", stdout)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	code, stdout, errText = runCLI("clean", "--yes", "--json")
	if code != output.ExitOK || errText != "" || !json.Valid([]byte(stdout)) {
		t.Fatalf("clean code=%d err=%s out=%s", code, errText, stdout)
	}
	if strings.Contains(readFile(t, filepath.Join(root, "docker.log")), project+"_default") {
		// expected log content, just keep file available for debugging
	}
	registry, err := ports.NewRegistry().Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry[project]; ok {
		t.Fatalf("ports were not released: %#v", registry)
	}
	instances, err := state.LoadGlobalInstances("")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := instances[project]; ok {
		t.Fatalf("global instance was not removed: %#v", instances)
	}
}

type upJSON struct {
	Instance struct {
		ProjectName string `json:"project_name"`
	} `json:"instance"`
	Ports []struct {
		HostPort int `json:"host_port"`
	} `json:"ports"`
}

func runCLI(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := cli.Run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFakeDocker(t *testing.T, path, stateFile string) {
	t.Helper()
	script := `#!/bin/sh
echo "$@" >> "$FAKE_DOCKER_LOG"
args="$*"
project_from_args() {
  prev=""
  for arg in "$@"; do
    if [ "$prev" = "-p" ]; then
      printf '%s' "$arg"
      return
    fi
    prev="$arg"
  done
}
mkdir -p "$FAKE_DOCKER_ROOT/projects"
project="$(project_from_args "$@")"
if echo "$args" | grep -q "compose" && echo "$args" | grep -q " ps " && echo "$args" | grep -q -- "--format json"; then
  if [ -n "$project" ] && [ -f "$FAKE_DOCKER_ROOT/projects/$project" ]; then
    printf '[{"Name":"web","State":"running"}]\n'
  else
    printf '[]\n'
  fi
  exit 0
fi
case "$1 $2" in
  "ps -a")
    if echo "$args" | grep -q 'docktree.managed=true'; then
      for file in "$FAKE_DOCKER_ROOT"/projects/*; do
        [ -f "$file" ] || continue
        name="$(basename "$file")"
        printf 'docktree.managed=true,docktree.instance=%s,com.docker.compose.project=%s\n' "$name" "$name"
      done
      exit 0
    fi
    filter="$(printf '%s' "$args" | sed -n 's/.*label=com.docker.compose.project=\([^ ]*\).*/\1/p')"
    if [ -n "$filter" ] && [ -f "$FAKE_DOCKER_ROOT/projects/$filter" ]; then
      printf '%s__web\t%s-web\n' "$filter" "$filter"
      printf '%s__api\t%s-api\n' "$filter" "$filter"
    fi
    exit 0
    ;;
  "network ls")
    filter="$(printf '%s' "$args" | sed -n 's/.*label=com.docker.compose.project=\([^ ]*\).*/\1/p')"
    if [ -n "$filter" ] && [ -f "$FAKE_DOCKER_ROOT/projects/$filter" ]; then
      printf 'n1\t%s_default\n' "$filter"
    fi
    exit 0
    ;;
  "volume ls")
    filter="$(printf '%s' "$args" | sed -n 's/.*label=com.docker.compose.project=\([^ ]*\).*/\1/p')"
    if [ -n "$filter" ] && [ -f "$FAKE_DOCKER_ROOT/projects/$filter" ]; then
      printf '%s_data\n' "$filter"
    fi
    exit 0
    ;;
  "rm -f")
    for arg in "$@"; do
      case "$arg" in
        *__*)
          project_name="${arg%%__*}"
          rm -f "$FAKE_DOCKER_ROOT/projects/$project_name"
          ;;
      esac
    done
    exit 0
    ;;
  "network rm")
    for arg in "$@"; do
      case "$arg" in
        *_default)
          project_name="${arg%_default}"
          rm -f "$FAKE_DOCKER_ROOT/projects/$project_name"
          ;;
      esac
    done
    exit 0
    ;;
  "volume rm")
    exit 0
    ;;
esac
case "$args" in
  *" up -d"*)
    if [ -n "$project" ]; then
      touch "$FAKE_DOCKER_ROOT/projects/$project"
    else
      touch "$FAKE_DOCKER_STATE"
    fi
    printf 'started\n'
    ;;
  *" down"*)
    if [ -n "$project" ]; then
      rm -f "$FAKE_DOCKER_ROOT/projects/$project"
    fi
    rm -f "$FAKE_DOCKER_STATE"
    printf 'stopped\n'
    ;;
  *)
    printf '{}\n'
    ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_DOCKER_STATE", stateFile)
	t.Setenv("FAKE_DOCKER_ROOT", filepath.Dir(stateFile))
	t.Setenv("FAKE_DOCKER_LOG", filepath.Join(filepath.Dir(stateFile), "docker.log"))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

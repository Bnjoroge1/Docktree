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
	"time"

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

	if code, stdout, errText := runCLI("up", "-h"); code != output.ExitOK || errText != "" || !strings.Contains(stdout, "--sync") || !strings.Contains(stdout, "--create") {
		t.Fatalf("up help code=%d err=%s out=%s", code, errText, stdout)
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
	if _, err := os.Stat(filepath.Join(repo, ".docktree", "generated")); err != nil {
		t.Fatalf("state dir not created: %v", err)
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

// TestIdentityStableAcrossBranchChanges is the regression test for issue #62:
// once a worktree has created an instance, its persisted project name is
// authoritative. Branch checkout, rename, and detached HEAD moves must never
// change the identity used by up, ports, volumes, env, or dry-run, and must
// never create a second global instance or port-registry entry.
func TestIdentityStableAcrossBranchChanges(t *testing.T) {
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
		t.Fatalf("up code=%d err=%s out=%s", code, errText, stdout)
	}
	var firstUp upJSON
	if err := json.Unmarshal([]byte(stdout), &firstUp); err != nil {
		t.Fatal(err)
	}
	project := firstUp.Instance.ProjectName
	if project == "" {
		t.Fatalf("missing project name: %s", stdout)
	}

	portsFor := func() string {
		t.Helper()
		code, stdout, errText := runCLI("ports", "--json")
		if code != output.ExitOK || errText != "" {
			t.Fatalf("ports code=%d err=%s", code, errText)
		}
		var result struct {
			Instance string `json:"instance"`
			Entries  []struct {
				Ports []struct {
					HostPort int `json:"host_port"`
				} `json:"ports"`
			} `json:"entries"`
		}
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("ports output not json: %v\n%s", err, stdout)
		}
		if len(result.Entries) != 1 || len(result.Entries[0].Ports) != 2 {
			t.Fatalf("expected two allocated ports: %s", stdout)
		}
		return result.Instance
	}

	// Branch checkout in the same worktree.
	run(t, repo, "git", "checkout", "-b", "feature/b")
	if got := portsFor(); got != project {
		t.Fatalf("ports after checkout: instance %q, want %q", got, project)
	}

	code, stdout, errText = runCLI("volumes", "--json")
	if code != output.ExitOK || errText != "" {
		t.Fatalf("volumes code=%d err=%s", code, errText)
	}
	var volumesResult struct {
		Instance string `json:"instance"`
	}
	if err := json.Unmarshal([]byte(stdout), &volumesResult); err != nil {
		t.Fatalf("volumes output not json: %v\n%s", err, stdout)
	}
	if volumesResult.Instance != project {
		t.Fatalf("volumes after checkout: instance %q, want %q", volumesResult.Instance, project)
	}

	code, stdout, errText = runCLI("env", "list", "--json")
	if code != output.ExitOK || errText != "" {
		t.Fatalf("env list code=%d err=%s", code, errText)
	}
	var envResult struct {
		Instance string `json:"instance"`
	}
	if err := json.Unmarshal([]byte(stdout), &envResult); err != nil {
		t.Fatalf("env output not json: %v\n%s", err, stdout)
	}
	if envResult.Instance != project {
		t.Fatalf("env after checkout: instance %q, want %q", envResult.Instance, project)
	}

	code, stdout, errText = runCLI("up", "--dry-run", "--json")
	if code != output.ExitOK || errText != "" {
		t.Fatalf("up --dry-run code=%d err=%s", code, errText)
	}
	var dryRun struct {
		InstanceName string `json:"instance_name"`
	}
	if err := json.Unmarshal([]byte(stdout), &dryRun); err != nil {
		t.Fatalf("dry-run output not json: %v\n%s", err, stdout)
	}
	if dryRun.InstanceName != project {
		t.Fatalf("up --dry-run after checkout: instance %q, want %q", dryRun.InstanceName, project)
	}

	// A real up after the branch switch must target the saved project, not
	// fork a second Compose project under a new name.
	code, stdout, errText = runCLI("up", "--json")
	if code != output.ExitOK || errText != "" {
		t.Fatalf("second up code=%d err=%s", code, errText)
	}
	var secondUp upJSON
	if err := json.Unmarshal([]byte(stdout), &secondUp); err != nil {
		t.Fatal(err)
	}
	if secondUp.Instance.ProjectName != project {
		t.Fatalf("second up project %q, want %q", secondUp.Instance.ProjectName, project)
	}
	logData := readFile(t, filepath.Join(root, "docker.log"))
	for _, line := range strings.Split(logData, "\n") {
		// --profile flags can sit between -p <project> and up -d, so check the
		// project selector independently of the subcommand.
		if strings.Contains(line, " up -d") && !strings.Contains(line, "-p "+project) {
			t.Fatalf("up targeted a different project after branch switch: %s", line)
		}
	}

	// Branch rename.
	run(t, repo, "git", "branch", "-m", "feature/c")
	if got := portsFor(); got != project {
		t.Fatalf("ports after rename: instance %q, want %q", got, project)
	}

	// Detached HEAD.
	run(t, repo, "git", "checkout", "--detach", "HEAD")
	if got := portsFor(); got != project {
		t.Fatalf("ports after detached HEAD: instance %q, want %q", got, project)
	}

	// Exactly one global instance, one port-registry entry — both keyed by the
	// original project name.
	instances, err := state.LoadGlobalInstances("")
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected one global instance, got %#v", instances)
	}
	if _, ok := instances[project]; !ok {
		t.Fatalf("global instance missing for %q: %#v", project, instances)
	}
	registry, err := ports.NewRegistry().Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(registry) != 1 {
		t.Fatalf("expected one port-registry entry, got %#v", registry)
	}
	if _, ok := registry[project]; !ok {
		t.Fatalf("port registry missing for %q: %#v", project, registry)
	}

	saved, err := state.LoadInstance(state.StatePath(repo, ".docktree"))
	if err != nil {
		t.Fatal(err)
	}
	if saved.ProjectName != project {
		t.Fatalf("saved project name %q, want %q", saved.ProjectName, project)
	}
}

// TestCleanKeepsLiveStateAfterIdentityRollover is the cleanup-hazard
// regression test for issue #62: a stale global record that shares its
// StateDirectory with the live record (created by a pre-fix identity
// rollover) must be removable without deleting the current worktree's
// .docktree directory.
func TestCleanKeepsLiveStateAfterIdentityRollover(t *testing.T) {
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
	if code != output.ExitOK || errText != "" {
		t.Fatalf("up code=%d err=%s", code, errText)
	}
	var up upJSON
	if err := json.Unmarshal([]byte(stdout), &up); err != nil {
		t.Fatal(err)
	}
	live := up.Instance.ProjectName
	if live == "" {
		t.Fatalf("missing project name: %s", stdout)
	}

	// Simulate the pre-fix rollover: a stale identity record and its port
	// allocations survive alongside the live record, both pointing at the
	// same worktree state directory. The stale record has no Docker
	// resources, so clean sees it as stale port allocations.
	now := time.Now().UTC()
	instances, err := state.LoadGlobalInstances("")
	if err != nil {
		t.Fatal(err)
	}
	liveInst := instances[live]
	instances["repo-feature-old-000000"] = state.Instance{
		Name:           "repo-feature-old-000000",
		ProjectName:    "repo-feature-old-000000",
		WorktreeRoot:   liveInst.WorktreeRoot,
		StateDirectory: liveInst.StateDirectory,
		Branch:         "feature/old",
		CreatedAt:      now,
		LastActiveAt:   now,
	}
	if err := state.SaveGlobalInstances("", instances); err != nil {
		t.Fatal(err)
	}
	registry := ports.NewRegistry()
	all, err := registry.Load()
	if err != nil {
		t.Fatal(err)
	}
	all["repo-feature-old-000000"] = []ports.Assignment{
		{Service: "web", ContainerPort: 80, HostIP: "127.0.0.1", HostPort: 41999},
	}
	if err := registry.Save(all); err != nil {
		t.Fatal(err)
	}

	code, stdout, errText = runCLI("clean", "--yes", "--json")
	if code != output.ExitOK || errText != "" || !json.Valid([]byte(stdout)) {
		t.Fatalf("clean code=%d err=%s out=%s", code, errText, stdout)
	}
	var cleanResult struct {
		Instances []struct {
			Instance string `json:"instance"`
			Reason   string `json:"reason"`
		} `json:"instances"`
	}
	if err := json.Unmarshal([]byte(stdout), &cleanResult); err != nil {
		t.Fatal(err)
	}
	if len(cleanResult.Instances) != 1 || cleanResult.Instances[0].Instance != "repo-feature-old-000000" {
		t.Fatalf("expected only the stale record as a clean candidate: %s", stdout)
	}

	// The stale record is gone…
	instances, err = state.LoadGlobalInstances("")
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected one global instance after clean, got %#v", instances)
	}
	if _, ok := instances[live]; !ok {
		t.Fatalf("live instance %q was removed: %#v", live, instances)
	}
	all, err = registry.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := all["repo-feature-old-000000"]; ok {
		t.Fatalf("stale port allocations were not released: %#v", all)
	}

	// …but the live worktree's state directory must survive.
	if _, err := os.Stat(filepath.Join(liveInst.StateDirectory, "state.json")); err != nil {
		t.Fatalf("clean removed the live worktree's state dir: %v", err)
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

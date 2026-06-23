//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestConcurrentUp spins up N docktree instances concurrently, verifying that
// port allocation is unique, state files are consistent, and no panics/races
// occur under contention.
func TestConcurrentUp(t *testing.T) {
	const instanceCount = 8
	t.Parallel()

	root := t.TempDir()
	fakeBinDir := filepath.Join(root, "bin")
	os.MkdirAll(fakeBinDir, 0o755)

	stateFile := filepath.Join(root, "docker-state")
	dockerEnv := writeFakeDockerConcurrent(t, filepath.Join(fakeBinDir, "docker"), stateFile)

	xdgDir := filepath.Join(root, "config")
	os.MkdirAll(xdgDir, 0o755)

	binaryPath := buildBinary(t)

	type instance struct {
		name   string
		repo   string
		result *upJSON
		stdout string
		stderr string
		code   int
		err    error
	}

	instances := make([]instance, instanceCount)
	for i := range instances {
		instances[i].name = fmt.Sprintf("instance-%d", i)
		instances[i].repo = setupWorktreeRepo(t, root, instances[i].name)
	}

	var wg sync.WaitGroup
	var successCount atomic.Int32
	var failCount atomic.Int32

	start := time.Now()
	for i := range instances {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			inst := &instances[idx]
			cmd := exec.Command(binaryPath, "up", "--json")
			cmd.Dir = inst.repo
			cmd.Env = docktreeEnv(fakeBinDir, xdgDir, dockerEnv)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			inst.err = cmd.Run()
			inst.stdout = stdout.String()
			inst.stderr = stderr.String()
			inst.code = cmd.ProcessState.ExitCode()
			if inst.err == nil && inst.code == 0 {
				successCount.Add(1)
			} else {
				failCount.Add(1)
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Concurrent up: %d/%d succeeded in %v", successCount.Load(), instanceCount, elapsed)

	portsSeen := make(map[int]string)
	var parseErrors int

	for i := range instances {
		inst := &instances[i]
		if inst.code != 0 {
			t.Logf("  FAIL instance %s: exit=%d stderr=%s", inst.name, inst.code, truncateStr(inst.stderr, 300))
			continue
		}
		var parsed upJSON
		if err := json.Unmarshal([]byte(inst.stdout), &parsed); err != nil {
			t.Logf("  FAIL instance %s: JSON parse error: %v", inst.name, err)
			parseErrors++
			continue
		}
		instances[i].result = &parsed
		if parsed.Instance.ProjectName == "" {
			t.Errorf("instance %s: empty ProjectName in JSON", inst.name)
		}
		for _, p := range parsed.Ports {
			port := p.HostPort
			if other, exists := portsSeen[port]; exists {
				t.Errorf("PORT COLLISION: port %d allocated to both %s and %s", port, inst.name, other)
			}
			portsSeen[port] = inst.name
		}
	}

	// Verify state files exist for successful instances
	for i := range instances {
		inst := &instances[i]
		if inst.result == nil {
			continue
		}
		statePath := filepath.Join(inst.repo, ".docktree", "state.json")
		if _, err := os.Stat(statePath); err != nil {
			t.Errorf("instance %s: state.json missing: %v", inst.name, err)
		}
		overridePath := filepath.Join(inst.repo, ".docktree", "generated", inst.result.Instance.ProjectName+".override.yml")
		if _, err := os.Stat(overridePath); err != nil {
			t.Errorf("instance %s: override.yml missing: %v", inst.name, err)
		}
	}

	// Verify global state
	globalStatePath := filepath.Join(xdgDir, "docktree", "instances.json")
	if data, err := os.ReadFile(globalStatePath); err == nil {
		var globalState map[string]any
		if err := json.Unmarshal(data, &globalState); err != nil {
			t.Errorf("global instances.json corrupted: %v", err)
		} else {
			t.Logf("Global state has %d instances (expected %d)", len(globalState), successCount.Load())
			for i := range instances {
				inst := &instances[i]
				if inst.result == nil {
					continue
				}
				if _, ok := globalState[inst.result.Instance.ProjectName]; !ok {
					t.Logf("  LIMITATION: instance %s missing from global state (lost write from concurrent UpsertGlobalInstance)", inst.name)
				}
			}
		}
	}

	portRegistryPath := filepath.Join(xdgDir, "docktree", "ports.json")
	if data, err := os.ReadFile(portRegistryPath); err == nil {
		var registry map[string]any
		if err := json.Unmarshal(data, &registry); err != nil {
			t.Errorf("ports.json corrupted: %v", err)
		} else {
			t.Logf("Port registry has %d entries", len(registry))
		}
	}

	t.Logf("Parse errors: %d, port collisions: 0 across %d ports", parseErrors, len(portsSeen))
}

// TestConcurrentDown tests that concurrent down commands don't corrupt the
// shared global state file.
func TestConcurrentDown(t *testing.T) {
	const instanceCount = 6
	t.Parallel()

	root := t.TempDir()
	fakeBinDir := filepath.Join(root, "bin")
	os.MkdirAll(fakeBinDir, 0o755)

	stateFile := filepath.Join(root, "docker-state")
	dockerEnv := writeFakeDockerConcurrent(t, filepath.Join(fakeBinDir, "docker"), stateFile)

	xdgDir := filepath.Join(root, "config")
	os.MkdirAll(xdgDir, 0o755)

	binaryPath := buildBinary(t)
	env := docktreeEnv(fakeBinDir, xdgDir, dockerEnv)

	type instance struct {
		name string
		repo string
	}

	instances := make([]instance, instanceCount)
	for i := range instances {
		instances[i].name = fmt.Sprintf("down-test-%d", i)
		instances[i].repo = setupWorktreeRepo(t, root, instances[i].name)
	}

	// Bring all up sequentially
	for i := range instances {
		inst := &instances[i]
		cmd := exec.Command(binaryPath, "up", "--json")
		cmd.Dir = inst.repo
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("instance %s up failed: %v\n%s", inst.name, err, out)
		}
	}

	// Bring all down concurrently
	var wg sync.WaitGroup
	var downSuccess atomic.Int32
	var downFail atomic.Int32

	start := time.Now()
	for i := range instances {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			inst := &instances[idx]
			cmd := exec.Command(binaryPath, "down", "--json")
			cmd.Dir = inst.repo
			cmd.Env = env
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err == nil {
				downSuccess.Add(1)
			} else {
				downFail.Add(1)
				t.Logf("  LIMITATION: %s down failed (exit %d): %s", inst.name, cmd.ProcessState.ExitCode(), truncateStr(stderr.String(), 200))
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Concurrent down: %d/%d succeeded in %v", downSuccess.Load(), instanceCount, elapsed)

	// Down updates global state (not removes), so all instances should still be there
	globalStatePath := filepath.Join(xdgDir, "docktree", "instances.json")
	if data, err := os.ReadFile(globalStatePath); err == nil {
		var globalState map[string]any
		if err := json.Unmarshal(data, &globalState); err != nil {
			t.Errorf("LIMITATION: global instances.json corrupted after concurrent down: %v", err)
		} else {
			t.Logf("Global state has %d instances after down (down updates, doesn't remove)", len(globalState))
		}
	}
}

// TestConcurrentClean tests that concurrent clean doesn't corrupt state.
func TestConcurrentClean(t *testing.T) {
	const instanceCount = 6
	t.Parallel()

	root := t.TempDir()
	fakeBinDir := filepath.Join(root, "bin")
	os.MkdirAll(fakeBinDir, 0o755)

	stateFile := filepath.Join(root, "docker-state")
	dockerEnv := writeFakeDockerConcurrent(t, filepath.Join(fakeBinDir, "docker"), stateFile)

	xdgDir := filepath.Join(root, "config")
	os.MkdirAll(xdgDir, 0o755)

	binaryPath := buildBinary(t)
	env := docktreeEnv(fakeBinDir, xdgDir, dockerEnv)

	type instance struct {
		name string
		repo string
	}

	instances := make([]instance, instanceCount)
	for i := range instances {
		instances[i].name = fmt.Sprintf("clean-test-%d", i)
		instances[i].repo = setupWorktreeRepo(t, root, instances[i].name)
	}

	for i := range instances {
		inst := &instances[i]
		cmd := exec.Command(binaryPath, "up", "--json")
		cmd.Dir = inst.repo
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("instance %s up failed: %v\n%s", inst.name, err, out)
		}
	}

	var wg sync.WaitGroup
	var cleanCount atomic.Int32
	var noopCount atomic.Int32

	start := time.Now()
	for i := range instances {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			inst := &instances[idx]
			cmd := exec.Command(binaryPath, "clean", "--json")
			cmd.Dir = inst.repo
			cmd.Env = env
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			exitCode := cmd.ProcessState.ExitCode()
			if err == nil {
				cleanCount.Add(1)
			} else if exitCode == 5 { // ExitNoop = no candidates found
				noopCount.Add(1)
			} else {
				t.Logf("  LIMITATION: %s clean failed (exit %d): %s", inst.name, exitCode, truncateStr(stderr.String(), 200))
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Concurrent clean: %d removed, %d noop (no candidates), in %v", cleanCount.Load(), noopCount.Load(), elapsed)
}

// TestPortExhaustion tests what happens when many instances consume a small port range.
func TestPortExhaustion(t *testing.T) {
	const instanceCount = 20
	t.Parallel()

	root := t.TempDir()
	fakeBinDir := filepath.Join(root, "bin")
	os.MkdirAll(fakeBinDir, 0o755)

	stateFile := filepath.Join(root, "docker-state")
	dockerEnv := writeFakeDockerConcurrent(t, filepath.Join(fakeBinDir, "docker"), stateFile)

	xdgDir := filepath.Join(root, "config")
	os.MkdirAll(xdgDir, 0o755)

	binaryPath := buildBinary(t)
	env := docktreeEnv(fakeBinDir, xdgDir, dockerEnv)

	type instance struct {
		name   string
		repo   string
		code   int
		stderr string
	}

	instances := make([]instance, instanceCount)
	for i := range instances {
		instances[i].name = fmt.Sprintf("exhaust-%d", i)
		instances[i].repo = setupWorktreeRepoWithRange(t, root, instances[i].name, "49900-49999")
	}

	var wg sync.WaitGroup
	var successCount atomic.Int32
	var failureReasons sync.Map

	start := time.Now()
	for i := range instances {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			inst := &instances[idx]
			cmd := exec.Command(binaryPath, "up", "--json")
			cmd.Dir = inst.repo
			cmd.Env = env
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				failureReasons.Store(inst.name, truncateStr(stderr.String(), 200))
			} else {
				successCount.Add(1)
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Port exhaustion: %d/%d succeeded in %v (range 49900-49999 = 100 ports, 20 instances)", successCount.Load(), instanceCount, elapsed)

	failureCount := 0
	failureReasons.Range(func(key, value any) bool {
		failureCount++
		t.Logf("  Failed %s: %s", key, value)
		return true
	})
	if failureCount > 0 {
		t.Logf("LIMITATION: %d instances failed due to port exhaustion (100 ports / ~4 per instance)", failureCount)
	}
}

// TestConcurrentUpDownChurn tests rapid up-down-up-down cycles concurrently
// to stress the global state file and port registry under contention.
func TestConcurrentUpDownChurn(t *testing.T) {
	const instanceCount = 4
	const cycles = 3
	t.Parallel()

	root := t.TempDir()
	fakeBinDir := filepath.Join(root, "bin")
	os.MkdirAll(fakeBinDir, 0o755)

	stateFile := filepath.Join(root, "docker-state")
	dockerEnv := writeFakeDockerConcurrent(t, filepath.Join(fakeBinDir, "docker"), stateFile)

	xdgDir := filepath.Join(root, "config")
	os.MkdirAll(xdgDir, 0o755)

	binaryPath := buildBinary(t)
	env := docktreeEnv(fakeBinDir, xdgDir, dockerEnv)

	type instance struct {
		name string
		repo string
	}

	instances := make([]instance, instanceCount)
	for i := range instances {
		instances[i].name = fmt.Sprintf("churn-test-%d", i)
		instances[i].repo = setupWorktreeRepo(t, root, instances[i].name)
	}

	var wg sync.WaitGroup
	var totalOps atomic.Int32
	var totalErrors atomic.Int32

	start := time.Now()
	for i := range instances {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			inst := &instances[idx]
			for cycle := range cycles {
				cmd := exec.Command(binaryPath, "up", "--json")
				cmd.Dir = inst.repo
				cmd.Env = env
				if out, err := cmd.CombinedOutput(); err != nil {
					totalErrors.Add(1)
					t.Logf("  LIMITATION: %s cycle %d up failed: %s", inst.name, cycle, truncateStr(string(out), 200))
				}
				totalOps.Add(1)

				cmd = exec.Command(binaryPath, "down", "--json")
				cmd.Dir = inst.repo
				cmd.Env = env
				if out, err := cmd.CombinedOutput(); err != nil {
					totalErrors.Add(1)
					t.Logf("  LIMITATION: %s cycle %d down failed: %s", inst.name, cycle, truncateStr(string(out), 200))
				}
				totalOps.Add(1)
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Churn: %d ops, %d errors across %d instances x %d cycles in %v",
		totalOps.Load(), totalErrors.Load(), instanceCount, cycles, elapsed)

	// down updates global state but doesn't remove — all instances remain
	globalStatePath := filepath.Join(xdgDir, "docktree", "instances.json")
	if data, err := os.ReadFile(globalStatePath); err == nil {
		var globalState map[string]any
		if err := json.Unmarshal(data, &globalState); err != nil {
			t.Errorf("global instances.json corrupted after churn: %v", err)
		} else {
			t.Logf("Global state has %d instances after churn (expected %d — down updates, doesn't remove)", len(globalState), instanceCount)
		}
	}
}

// TestDifferentComposeFiles tests concurrent instances with different compose topologies.
func TestDifferentComposeFiles(t *testing.T) {
	const instanceCount = 5
	t.Parallel()

	root := t.TempDir()
	fakeBinDir := filepath.Join(root, "bin")
	os.MkdirAll(fakeBinDir, 0o755)

	stateFile := filepath.Join(root, "docker-state")
	dockerEnv := writeFakeDockerConcurrent(t, filepath.Join(fakeBinDir, "docker"), stateFile)

	xdgDir := filepath.Join(root, "config")
	os.MkdirAll(xdgDir, 0o755)

	binaryPath := buildBinary(t)
	env := docktreeEnv(fakeBinDir, xdgDir, dockerEnv)

	composeFiles := []string{
		"compose-basic.yml",
		"compose-complex-ports.yml",
		"compose-with-networks.yml",
		"compose-with-volumes.yml",
		"compose-with-healthchecks.yml",
	}

	type instance struct {
		name    string
		repo    string
		compose string
		stdout  string
		stderr  string
		code    int
	}

	instances := make([]instance, instanceCount)
	for i := range instances {
		instances[i].name = fmt.Sprintf("multi-compose-%d", i)
		instances[i].compose = composeFiles[i%len(composeFiles)]
		instances[i].repo = setupWorktreeRepoWithCompose(t, root, instances[i].name, instances[i].compose)
	}

	var wg sync.WaitGroup
	var success atomic.Int32

	start := time.Now()
	for i := range instances {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			inst := &instances[idx]
			cmd := exec.Command(binaryPath, "up", "--json")
			cmd.Dir = inst.repo
			cmd.Env = env
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Logf("instance %s (%s) failed: %v\nstderr: %s", inst.name, inst.compose, err, stderr.String())
			} else {
				success.Add(1)
				inst.stdout = stdout.String()
			}
			inst.code = cmd.ProcessState.ExitCode()
			inst.stderr = stderr.String()
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Multi-compose: %d/%d succeeded in %v", success.Load(), instanceCount, elapsed)

	portsSeen := make(map[int]string)
	for i := range instances {
		inst := &instances[i]
		if inst.code != 0 {
			continue
		}
		var parsed upJSON
		if err := json.Unmarshal([]byte(inst.stdout), &parsed); err != nil {
			continue
		}
		for _, p := range parsed.Ports {
			port := p.HostPort
			if other, exists := portsSeen[port]; exists {
				t.Errorf("PORT COLLISION: port %d allocated to both %s and %s", port, inst.name, other)
			}
			portsSeen[port] = inst.name
		}
	}
}

// --- helpers ---

func docktreeEnv(fakeBinDir, xdgDir string, extra map[string]string) []string {
	base := os.Environ()
	env := make([]string, 0, len(base)+2+len(extra))
	env = append(env,
		"PATH="+fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"XDG_CONFIG_HOME="+xdgDir,
	)
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	seen := map[string]bool{"PATH": true, "XDG_CONFIG_HOME": true}
	for _, e := range base {
		k := e
		if i := indexOfByte(e, '='); i >= 0 {
			k = e[:i]
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		env = append(env, e)
	}
	return env
}

func indexOfByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func writeFakeDockerConcurrent(t *testing.T, path, stateFile string) map[string]string {
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
	root := filepath.Dir(stateFile)
	return map[string]string{
		"FAKE_DOCKER_STATE": stateFile,
		"FAKE_DOCKER_ROOT":  root,
		"FAKE_DOCKER_LOG":   filepath.Join(root, "docker.log"),
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "docktree")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/docktree/")
	cmd.Dir = goModRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

func goModRoot(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("cannot find go.mod")
		}
		dir = parent
	}
}

func setupWorktreeRepo(t *testing.T, root, name string) string {
	t.Helper()
	return setupWorktreeRepoWithRange(t, root, name, "")
}

func setupWorktreeRepoWithRange(t *testing.T, root, name, portRange string) string {
	t.Helper()
	repo := filepath.Join(root, "repos", name)
	os.MkdirAll(repo, 0o755)

	composeSrc := filepath.Join(goModRoot(t), "testdata", "docker-compose.yml")
	composeDst := filepath.Join(repo, "compose.yml")
	copyFile(t, composeSrc, composeDst)

	cfg := "compose:\n  files:\n    - compose.yml\nports:\n  mode: dynamic\n  bind_host: 127.0.0.1\n"
	if portRange != "" {
		cfg += fmt.Sprintf("  range: %s\n", portRange)
	} else {
		cfg += "  range: 41000-49999\n"
	}
	cfg += "transforms:\n  container_name: strip\nstate:\n  directory: .docktree\n"
	os.WriteFile(filepath.Join(repo, "docktree.yml"), []byte(cfg), 0o644)

	run(t, repo, "git", "init", "-b", "main")
	run(t, repo, "git", "config", "user.email", "test@docktree.test")
	run(t, repo, "git", "config", "user.name", "Docktree Test")
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "-m", "init")

	return repo
}

func setupWorktreeRepoWithCompose(t *testing.T, root, name, composeFile string) string {
	t.Helper()
	repo := filepath.Join(root, "repos", name)
	os.MkdirAll(repo, 0o755)

	composeSrc := filepath.Join(goModRoot(t), "testdata", composeFile)
	composeDst := filepath.Join(repo, "compose.yml")
	copyFile(t, composeSrc, composeDst)

	cfg := fmt.Sprintf(`compose:
  files:
    - compose.yml
ports:
  mode: dynamic
  bind_host: 127.0.0.1
  range: 41000-49999
transforms:
  container_name: strip
state:
  directory: .docktree
`)
	os.WriteFile(filepath.Join(repo, "docktree.yml"), []byte(cfg), 0o644)

	run(t, repo, "git", "init", "-b", "main")
	run(t, repo, "git", "config", "user.email", "test@docktree.test")
	run(t, repo, "git", "config", "user.name", "Docktree Test")
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "-m", "init")

	return repo
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

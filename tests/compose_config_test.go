//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/bnjoroge/docktree/internal/cli"
	"github.com/bnjoroge/docktree/internal/compose"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/ports"
	"github.com/bnjoroge/docktree/internal/state"
)

func TestGeneratedOverridesPassDockerComposeConfig(t *testing.T) {
	fixtures := composeFixtures(t)
	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			project, err := compose.ParseFile(fixture)
			if err != nil {
				t.Fatal(err)
			}
			override, err := compose.GenerateOverride(project, "docktree-config-test", assignmentsFor(project), nil)
			if err != nil {
				t.Fatal(err)
			}
			overridePath := filepath.Join(t.TempDir(), "override.yml")
			if err := compose.WriteOverride(override, overridePath); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("docker", "compose", "-f", fixture, "-f", overridePath, "-p", "docktree-config-test", "config")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("docker compose config failed: %v\n%s", err, out)
			}
			text := string(out)
			if strings.Contains(text, "myapp_web") || strings.Contains(text, "sample_") {
				t.Fatalf("container_name was not rewritten in rendered config:\n%s", text)
			}
			if hasBuild(project) && !strings.Contains(text, "docktree/") {
				t.Fatalf("built images were not rewritten in rendered config:\n%s", text)
			}
			for service, svc := range project.Services {
				for _, port := range svc.Ports {
					if port.Published != 0 && strings.Contains(text, "published: \""+itoa(port.Published)+"\"") {
						t.Fatalf("original published port leaked for %s:%d:\n%s", service, port.Published, text)
					}
				}
			}
		})
	}
}

func composeFixtures(t *testing.T) []string {
	t.Helper()
	fixtures, err := filepath.Glob(filepath.Join("..", "testdata", "compose-variants", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	fixtures = append(fixtures, filepath.Join("..", "testdata", "docker-compose.yml"))
	for i, fixture := range fixtures {
		abs, err := filepath.Abs(fixture)
		if err != nil {
			t.Fatal(err)
		}
		fixtures[i] = abs
	}
	if len(fixtures) < 24 {
		t.Fatalf("expected at least 24 compose fixtures, got %d", len(fixtures))
	}
	return fixtures
}

func assignmentsFor(project *compose.ComposeProject) []ports.Assignment {
	var assignments []ports.Assignment
	next := 43000
	for service, svc := range project.Services {
		for _, port := range svc.Ports {
			hostIP := port.HostIP
			if hostIP == "" {
				hostIP = "127.0.0.1"
			}
			assignments = append(assignments, ports.Assignment{
				Service:       service,
				ContainerPort: port.Target,
				HostIP:        hostIP,
				HostPort:      next,
			})
			next++
		}
	}
	return assignments
}

func hasBuild(project *compose.ComposeProject) bool {
	for _, svc := range project.Services {
		if svc.Build != nil {
			return true
		}
	}
	return false
}

func itoa(value int) string {
	return strconv.Itoa(value)
}

func TestDockerComposeUpRuntimeSmoke(t *testing.T) {
	dir := t.TempDir()
	contextDir := filepath.Join(dir, "app")
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte(`FROM busybox:1.36
CMD ["sh", "-c", "sleep 300"]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(dir, "compose.yml")
	if err := os.WriteFile(composePath, []byte(`services:
  built:
    build: ./app
    image: docktree/runtime-smoke:source
    container_name: runtime_smoke_built
  pulled:
    image: busybox:1.36
    command: ["sh", "-c", "sleep 300"]
    container_name: runtime_smoke_pulled
`), 0o644); err != nil {
		t.Fatal(err)
	}
	project := "docktree-runtime-smoke"
	projectModel, err := compose.ParseFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	override, err := compose.GenerateOverride(projectModel, project, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	overridePath := filepath.Join(dir, "docktree.override.yml")
	if err := compose.WriteOverride(override, overridePath); err != nil {
		t.Fatal(err)
	}
	defer exec.Command("docker", "compose", "-f", composePath, "-f", overridePath, "-p", project, "down", "-v", "--remove-orphans").Run()
	pull := exec.Command("docker", "compose", "-f", composePath, "-f", overridePath, "-p", project, "pull", "pulled")
	if out, err := pull.CombinedOutput(); err != nil {
		t.Fatalf("docker compose pull failed: %v\n%s", err, out)
	}
	up := exec.Command("docker", "compose", "-f", composePath, "-f", overridePath, "-p", project, "up", "-d", "--build")
	if out, err := up.CombinedOutput(); err != nil {
		t.Fatalf("docker compose up failed: %v\n%s", err, out)
	}
	ps := exec.Command("docker", "compose", "-f", composePath, "-f", overridePath, "-p", project, "ps", "--format", "json")
	out, err := ps.CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose ps failed: %v\n%s", err, out)
	}
	if strings.Count(strings.ToLower(string(out)), "running") < 2 {
		t.Fatalf("service did not reach running state:\n%s", out)
	}
	if strings.Contains(string(out), "runtime_smoke_") {
		t.Fatalf("source container_name leaked into runtime config:\n%s", out)
	}
}

func TestCleanRemovesDeletedWorktreeResources(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	composeText := `services:
  app:
    image: busybox:1.36
    command: ["sh", "-c", "sleep 300"]
    container_name: sample_clean_app
    ports:
      - "127.0.0.1:8080:8080"
    volumes:
      - app-data:/data
volumes:
  app-data: {}
`
	if err := os.WriteFile(filepath.Join(repo, "compose.yml"), []byte(composeText), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "git", "init", "-b", "main")
	run(t, repo, "git", "config", "user.email", "docktree@example.test")
	run(t, repo, "git", "config", "user.name", "Docktree Test")
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "-m", "init")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))

	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{"up", "--json"}, &stdout, &stderr)
	if code != output.ExitOK || !json.Valid(stdout.Bytes()) {
		t.Fatalf("up failed code=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	var up upJSON
	if err := json.Unmarshal(stdout.Bytes(), &up); err != nil {
		t.Fatal(err)
	}
	project := up.Instance.ProjectName
	if project == "" {
		t.Fatalf("missing project name from up output: %s", stdout.String())
	}
	defer exec.Command("docker", "compose", "-p", project, "down", "-v", "--remove-orphans").Run()

	portRegistry, err := ports.NewRegistry().Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(portRegistry[project]) == 0 {
		t.Fatalf("expected port allocations for %s", project)
	}
	globalInstances, err := state.LoadGlobalState("")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := globalInstances[project]; !ok {
		t.Fatalf("expected global instance for %s", project)
	}

	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"clean", "--yes", "--volumes", "--json"}, &stdout, &stderr)
	if code != output.ExitOK || stderr.Len() != 0 || !json.Valid(stdout.Bytes()) {
		t.Fatalf("clean failed code=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if out, err := exec.Command("docker", "ps", "-a", "--filter", "label=com.docker.compose.project="+project, "--format", "{{.ID}}").CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "" {
		t.Fatalf("containers still present err=%v out=%s", err, out)
	}
	if out, err := exec.Command("docker", "network", "ls", "--filter", "label=com.docker.compose.project="+project, "--format", "{{.ID}}").CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "" {
		t.Fatalf("networks still present err=%v out=%s", err, out)
	}
	if out, err := exec.Command("docker", "volume", "ls", "--filter", "label=com.docker.compose.project="+project, "--format", "{{.Name}}").CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "" {
		t.Fatalf("volumes still present err=%v out=%s", err, out)
	}
	portRegistry, err = ports.NewRegistry().Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := portRegistry[project]; ok {
		t.Fatalf("ports still present after clean: %#v", portRegistry)
	}
	globalInstances, err = state.LoadGlobalState("")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := globalInstances[project]; ok {
		t.Fatalf("global instance still present after clean: %#v", globalInstances)
	}
}

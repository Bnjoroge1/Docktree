package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/ports"
	"github.com/bnjoroge/docktree/internal/state"
)

func renderJSONForTest(t *testing.T, data any) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	output.New(&buf, true).Render(data, humanRenderer())
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json output did not decode: %v\n%s", err, buf.String())
	}
	return got
}

func TestStatusResultJSONRendering(t *testing.T) {
	got := renderJSONForTest(t, StatusResult{
		Instance: &state.Instance{Name: "docktree-main", ProjectName: "docktree-main", Branch: "main"},
		Raw:      json.RawMessage(`[{"Service":"api","State":"running"}]`),
	})
	instance := got["instance"].(map[string]any)
	if instance["project_name"] != "docktree-main" {
		t.Fatalf("project_name = %#v", instance["project_name"])
	}
	raw := got["raw"].([]any)
	service := raw[0].(map[string]any)
	if service["Service"] != "api" || service["State"] != "running" {
		t.Fatalf("unexpected raw status payload: %#v", raw)
	}
}

func TestUpResultJSONRendering(t *testing.T) {
	got := renderJSONForTest(t, UpResult{
		Instance:        &state.Instance{Name: "docktree-main", ProjectName: "docktree-main", Branch: "main"},
		ComposeFiles:    []string{"compose.yml"},
		OverrideFile:    ".docktree/generated/docktree-main.override.yml",
		Services:        []string{"api"},
		SharedServices:  []string{"postgres"},
		IsolatedVolumes: []string{"api-data"},
		Ports: []ports.Assignment{{
			Service:       "api",
			ContainerPort: 8080,
			HostIP:        "127.0.0.1",
			HostPort:      41000,
		}},
	})
	instance := got["instance"].(map[string]any)
	if instance["project_name"] != "docktree-main" {
		t.Fatalf("project_name = %#v", instance["project_name"])
	}
	portsPayload := got["ports"].([]any)
	port := portsPayload[0].(map[string]any)
	if port["service"] != "api" || port["host_port"] != float64(41000) {
		t.Fatalf("unexpected ports payload: %#v", portsPayload)
	}
	services := got["services"].([]any)
	if services[0] != "api" {
		t.Fatalf("unexpected services payload: %#v", services)
	}
}

func TestUpResultJSONRenderingHint(t *testing.T) {
	got := renderJSONForTest(t, UpResult{
		Instance: &state.Instance{Name: "x", ProjectName: "x"},
		Hint:     "tip: detected shareable services (postgres). Ask your AI agent to set up a shared platform tier in docktree.yml.",
	})
	if got["hint"] != "tip: detected shareable services (postgres). Ask your AI agent to set up a shared platform tier in docktree.yml." {
		t.Fatalf("hint = %#v", got["hint"])
	}
	// Empty hint must be omitted entirely (omitempty).
	bare := renderJSONForTest(t, UpResult{Instance: &state.Instance{Name: "x", ProjectName: "x"}})
	if _, ok := bare["hint"]; ok {
		t.Fatalf("empty hint should be omitted, got %#v", bare["hint"])
	}
}

func TestUpResultHumanRendererPrintsHint(t *testing.T) {
	var buf bytes.Buffer
	output.New(&buf, false).Render(UpResult{
		Instance: &state.Instance{Name: "x", ProjectName: "x"},
		Hint:     "tip: detected shareable services (postgres, redis). Ask your AI agent to set up a shared platform tier in docktree.yml.",
	}, humanRenderer())
	out := buf.String()
	if !contains(out, "tip: detected shareable services (postgres, redis)") {
		t.Fatalf("human render missing hint:\n%s", out)
	}

	// Empty hint -> no "tip:" line.
	buf.Reset()
	output.New(&buf, false).Render(UpResult{
		Instance: &state.Instance{Name: "x", ProjectName: "x"},
	}, humanRenderer())
	if contains(buf.String(), "tip:") {
		t.Fatalf("empty hint should not render a tip line:\n%s", buf.String())
	}
}

// contains avoids pulling in strings just for one test.
func contains(haystack, needle string) bool {
	return bytes.Contains([]byte(haystack), []byte(needle))
}

func TestDownAllHumanRendererHandlesNilInstance(t *testing.T) {
	var buf bytes.Buffer
	output.New(&buf, false).Render(DownResult{
		Services: []string{"demo-main", "demo-feature"},
	}, humanRenderer())
	out := buf.String()
	if !contains(out, "Docktree stopped all matching instances") {
		t.Fatalf("human render missing aggregate stop message:\n%s", out)
	}
	if !contains(out, "demo-main, demo-feature") {
		t.Fatalf("human render missing stopped instance names:\n%s", out)
	}

	buf.Reset()
	output.New(&buf, false).Render(DownResult{
		DryRun:   true,
		Services: []string{"demo-main", "demo-feature"},
	}, humanRenderer())
	out = buf.String()
	if !contains(out, "Docktree dry run - would stop all matching instances") {
		t.Fatalf("human render missing aggregate dry-run message:\n%s", out)
	}
}

func TestPortsResultJSONRendering(t *testing.T) {
	got := renderJSONForTest(t, PortsResult{
		All: true,
		Entries: []PortsEntry{{
			Instance: "docktree-main",
			Ports: []ports.Assignment{{
				Service:       "web",
				ContainerPort: 3000,
				HostIP:        "127.0.0.1",
				HostPort:      41001,
			}},
		}},
	})
	if got["all"] != true {
		t.Fatalf("all = %#v", got["all"])
	}
	entries := got["entries"].([]any)
	entry := entries[0].(map[string]any)
	if entry["instance"] != "docktree-main" {
		t.Fatalf("instance = %#v", entry["instance"])
	}
	portsPayload := entry["ports"].([]any)
	port := portsPayload[0].(map[string]any)
	if port["service"] != "web" || port["container_port"] != float64(3000) || port["host_port"] != float64(41001) {
		t.Fatalf("unexpected ports payload: %#v", portsPayload)
	}
}

func TestVolumesResultJSONRendering(t *testing.T) {
	got := renderJSONForTest(t, VolumesResult{
		All: true,
		Entries: []VolumesEntry{{
			Instance: "docktree-main",
			Volume:   "db_data",
			Name:     "docktree-main-db_data",
			Driver:   "local",
		}},
	})
	if got["all"] != true {
		t.Fatalf("all = %#v", got["all"])
	}
	entries := got["entries"].([]any)
	entry := entries[0].(map[string]any)
	if entry["instance"] != "docktree-main" || entry["volume"] != "db_data" || entry["driver"] != "local" {
		t.Fatalf("unexpected volumes payload: %#v", entries)
	}
}

func TestCleanResultJSONRendering(t *testing.T) {
	got := renderJSONForTest(t, CleanResult{
		DryRun:  true,
		Volumes: true,
		Instances: []CleanItem{{
			Instance:   "docktree-old",
			Reason:     "worktree path gone",
			Ports:      2,
			Containers: 1,
			Networks:   1,
			Volumes:    1,
		}},
		Totals: CleanTotals{Instances: 1, Ports: 2, Containers: 1, Networks: 1, Volumes: 1},
	})
	if got["dry_run"] != true || got["volumes"] != true {
		t.Fatalf("unexpected clean flags: %#v", got)
	}
	instances := got["instances"].([]any)
	item := instances[0].(map[string]any)
	if item["instance"] != "docktree-old" || item["reason"] != "worktree path gone" || item["volumes"] != float64(1) {
		t.Fatalf("unexpected clean item: %#v", item)
	}
	totals := got["totals"].(map[string]any)
	if totals["instances"] != float64(1) || totals["ports"] != float64(2) {
		t.Fatalf("unexpected clean totals: %#v", totals)
	}
}

func TestCreateResultJSONRendering(t *testing.T) {
	got := renderJSONForTest(t, CreateResult{
		RepoRoot:     "/repo/docktree",
		WorktreeRoot: "/repo/docktree.worktrees/feature-auth",
		Branch:       "feature/auth",
		Copied:       []string{".env.example"},
		Symlinked:    []string{"uploads"},
		Ran:          []string{"npm install"},
	})
	if got["repo_root"] != "/repo/docktree" || got["worktree_root"] != "/repo/docktree.worktrees/feature-auth" || got["branch"] != "feature/auth" {
		t.Fatalf("unexpected create payload: %#v", got)
	}
	copied := got["copied"].([]any)
	if copied[0] != ".env.example" {
		t.Fatalf("unexpected copied payload: %#v", copied)
	}
	ran := got["ran"].([]any)
	if ran[0] != "npm install" {
		t.Fatalf("unexpected ran payload: %#v", ran)
	}
}

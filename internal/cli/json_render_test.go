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

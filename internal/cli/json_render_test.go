package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/bnjoroge/docktree/internal/output"
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

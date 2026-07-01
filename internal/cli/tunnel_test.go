package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractTunnelURL(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "cloudflared plain line",
			line: "2026-07-01T00:12:34Z INF  |  https://curly-cat.trycloudflare.com",
			want: "https://curly-cat.trycloudflare.com",
		},
		{
			name: "ngrok JSON line",
			line: `{"url":"https://abc123.ngrok-free.app","msg":"started tunnel"}`,
			want: "https://abc123.ngrok-free.app",
		},
		{
			name: "ngrok legacy domain",
			line: "url=https://xyz.ngrok.io",
			want: "https://xyz.ngrok.io",
		},
		{
			name: "no tunnel URL",
			line: "some random log output",
			want: "",
		},
		{
			name: "URL with trailing comma",
			line: "https://foo.trycloudflare.com,status=ok",
			want: "https://foo.trycloudflare.com",
		},
		{
			name: "URL with trailing brace",
			line: `{"url":"https://bar.ngrok-free.app"}`,
			want: "https://bar.ngrok-free.app",
		},
		{
			name: "URL with trailing pipe",
			line: "https://baz.trycloudflare.com|something",
			want: "https://baz.trycloudflare.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTunnelURL(tt.line)
			if got != tt.want {
				t.Errorf("extractTunnelURL(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestTunnelStateRoundTrip(t *testing.T) {
	ts := TunnelState{
		PID:       12345,
		URL:       "https://example.trycloudflare.com",
		Provider:  "cloudflare",
		Port:      41006,
		StartedAt: "2026-07-01T00:00:00Z",
		StartTime: "Wed Jul  1 00:00:00 2026",
		LogPath:   "/tmp/tunnel.log",
	}

	data, err := json.MarshalIndent(ts, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored TunnelState
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if restored.PID != ts.PID {
		t.Errorf("PID = %d, want %d", restored.PID, ts.PID)
	}
	if restored.URL != ts.URL {
		t.Errorf("URL = %q, want %q", restored.URL, ts.URL)
	}
	if restored.Port != ts.Port {
		t.Errorf("Port = %d, want %d", restored.Port, ts.Port)
	}
	if restored.StartTime != ts.StartTime {
		t.Errorf("StartTime = %q, want %q", restored.StartTime, ts.StartTime)
	}
	if restored.LogPath != ts.LogPath {
		t.Errorf("LogPath = %q, want %q", restored.LogPath, ts.LogPath)
	}
}

func TestLegacyTunnelStateBackwardCompat(t *testing.T) {
	// Old tunnel.json without start_time or log_path fields.
	legacyJSON := `{
  "pid": 8554,
  "url": "https://old.trycloudflare.com",
  "provider": "cloudflare",
  "port": 41001,
  "started_at": "2026-06-30T23:33:09Z"
}`

	var ts TunnelState
	if err := json.Unmarshal([]byte(legacyJSON), &ts); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}

	if ts.PID != 8554 {
		t.Errorf("PID = %d, want 8554", ts.PID)
	}
	// Missing fields should be zero-valued.
	if ts.StartTime != "" {
		t.Errorf("StartTime = %q, want empty (legacy compat)", ts.StartTime)
	}
	if ts.LogPath != "" {
		t.Errorf("LogPath = %q, want empty (legacy compat)", ts.LogPath)
	}
}

func TestTunnelStateSaveLoad(t *testing.T) {
	dir := t.TempDir()
	worktreeRoot := filepath.Join(dir, "worktree")
	stateDir := ".docktree"
	statePath := filepath.Join(worktreeRoot, stateDir)

	if err := os.MkdirAll(statePath, 0o755); err != nil {
		t.Fatal(err)
	}

	ts := &TunnelState{
		PID:       42,
		URL:       "https://test.example.com",
		Provider:  "ngrok",
		Port:      8080,
		StartedAt: "2026-07-01T12:00:00Z",
		StartTime: "Wed Jul  1 12:00:00 2026",
		LogPath:   "/tmp/test-tunnel.log",
	}

	if err := saveTunnelState(worktreeRoot, stateDir, ts); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadTunnelState(worktreeRoot, stateDir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded == nil {
		t.Fatal("loaded nil")
	}

	if loaded.PID != ts.PID || loaded.URL != ts.URL || loaded.StartTime != ts.StartTime {
		t.Errorf("round-trip mismatch: got %+v, want %+v", loaded, ts)
	}
}

func TestTunnelStateLoadNonexistent(t *testing.T) {
	dir := t.TempDir()
	ts, err := LoadTunnelState(dir, ".docktree")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ts != nil {
		t.Errorf("expected nil for nonexistent state, got %+v", ts)
	}
}

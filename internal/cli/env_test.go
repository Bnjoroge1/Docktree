package cli

import (
	"reflect"
	"testing"
)

func TestParseEnvOptions(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    envOptions
		wantErr string
	}{
		{
			name: "no args shows help",
			args: []string{},
			want: envOptions{help: true},
		},
		{
			name: "help flag keeps parsed state",
			args: []string{"set", "A=1", "--restart", "api", "-h"},
			want: envOptions{action: "set", help: true, pairs: []envPair{{key: "A", value: "1"}}, restarts: []string{"api"}},
		},
		{
			name: "help flag alone",
			args: []string{"--help"},
			want: envOptions{help: true},
		},
		{
			name: "set single pair",
			args: []string{"set", "CHAT_TOKENIZER_MODEL=Qwen/Qwen3.6-35B-A3B"},
			want: envOptions{action: "set", pairs: []envPair{{key: "CHAT_TOKENIZER_MODEL", value: "Qwen/Qwen3.6-35B-A3B"}}},
		},
		{
			name: "set multiple pairs empty value allowed",
			args: []string{"set", "A=1", "B="},
			want: envOptions{action: "set", pairs: []envPair{{key: "A", value: "1"}, {key: "B", value: ""}}},
		},
		{
			name: "set with service and restart",
			args: []string{"set", "A=1", "--service", "api", "--service=web", "--restart", "api-zone-b", "--restart=api"},
			want: envOptions{action: "set", pairs: []envPair{{key: "A", value: "1"}}, services: []string{"api", "web"}, restarts: []string{"api-zone-b", "api"}},
		},
		{
			name:    "set missing pair",
			args:    []string{"set", "--service", "api"},
			wantErr: "env set requires at least one KEY=VALUE pair",
		},
		{
			name:    "set rejects bare key",
			args:    []string{"set", "KEY"},
			wantErr: "env set takes KEY=VALUE pairs, got \"KEY\"",
		},
		{
			name:    "set rejects empty key",
			args:    []string{"set", "=value"},
			wantErr: "env set takes KEY=VALUE pairs, got \"=value\"",
		},
		{
			name:    "set rejects pair before action",
			args:    []string{"A=1"},
			wantErr: `unknown env action "A=1" (expected set, unset, or list)`,
		},
		{
			name: "unset single key",
			args: []string{"unset", "CHAT_TOKENIZER_MODEL"},
			want: envOptions{action: "unset", keys: []string{"CHAT_TOKENIZER_MODEL"}},
		},
		{
			name: "unset scoped with restart",
			args: []string{"unset", "A", "B", "--service=api", "--restart", "api"},
			want: envOptions{action: "unset", keys: []string{"A", "B"}, services: []string{"api"}, restarts: []string{"api"}},
		},
		{
			name:    "unset rejects pair",
			args:    []string{"unset", "A=1"},
			wantErr: "env unset takes variable names (KEY), not KEY=VALUE pairs, got \"A=1\"",
		},
		{
			name:    "unset missing key",
			args:    []string{"unset"},
			wantErr: "env unset requires at least one KEY",
		},
		{
			name: "list bare",
			args: []string{"list"},
			want: envOptions{action: "list"},
		},
		{
			name:    "list rejects arguments",
			args:    []string{"list", "api"},
			wantErr: "env list takes no arguments, got \"api\"",
		},
		{
			name:    "list rejects service flag",
			args:    []string{"list", "--service", "api"},
			wantErr: "env list does not accept --service or --restart",
		},
		{
			name:    "unknown action",
			args:    []string{"remove", "A"},
			wantErr: `unknown env action "remove" (expected set, unset, or list)`,
		},
		{
			name:    "unknown flag",
			args:    []string{"set", "A=1", "--force"},
			wantErr: `unknown env flag "--force"`,
		},
		{
			name:    "service requires value",
			args:    []string{"set", "A=1", "--service"},
			wantErr: "--service requires a service name",
		},
		{
			name:    "service refuses flag as value",
			args:    []string{"set", "A=1", "--service", "--restart"},
			wantErr: "--service requires a service name",
		},
		{
			name:    "service equals empty",
			args:    []string{"set", "A=1", "--service="},
			wantErr: "--service requires a service name",
		},
		{
			name:    "restart equals empty",
			args:    []string{"set", "A=1", "--restart="},
			wantErr: "--restart requires a service name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseEnvOptions(tt.args)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestValidateEnvServices(t *testing.T) {
	known := []string{"api", "redis", "web"}
	if err := validateEnvServices(nil, known, "--service"); err != nil {
		t.Fatalf("empty should pass: %v", err)
	}
	if err := validateEnvServices([]string{"api", "web"}, known, "--service"); err != nil {
		t.Fatalf("known services should pass: %v", err)
	}
	err := validateEnvServices([]string{"api", "ghost"}, known, "--restart")
	if err == nil {
		t.Fatal("unknown service should fail")
	}
	want := `--restart: unknown service "ghost"; this project has: api, redis, web`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestApplyEnvSetAndUnset(t *testing.T) {
	env := map[string]map[string]string{}

	changed := applyEnvSet(env, []string{"api", "web"}, "K", "1")
	if len(changed) != 2 {
		t.Fatalf("changed = %#v", changed)
	}
	if env["api"]["K"] != "1" || env["web"]["K"] != "1" {
		t.Fatalf("env = %#v", env)
	}

	// Duplicate target only applies once.
	changed = applyEnvSet(env, []string{"api", "api"}, "K", "2")
	if len(changed) != 1 || env["api"]["K"] != "2" {
		t.Fatalf("changed = %#v, env = %#v", changed, env)
	}

	// Unset of an overridden key removes it everywhere by default.
	removed := applyEnvUnset(env, []string{"api", "web"}, "K")
	if len(removed) != 2 {
		t.Fatalf("removed = %#v", removed)
	}
	if len(env) != 0 {
		t.Fatalf("service maps should be dropped when empty: %#v", env)
	}

	// Unsetting a key with no override is a no-op.
	if removed := applyEnvUnset(env, []string{"api"}, "MISSING"); len(removed) != 0 {
		t.Fatalf("removed = %#v", removed)
	}
	if removed := applyEnvUnset(env, []string{"ghost"}, "K"); len(removed) != 0 {
		t.Fatalf("removed = %#v", removed)
	}

	// Scoped unset leaves the other service alone.
	applyEnvSet(env, []string{"api", "web"}, "K", "3")
	removed = applyEnvUnset(env, []string{"api"}, "K")
	if len(removed) != 1 || removed[0].Value != "3" {
		t.Fatalf("removed = %#v", removed)
	}
	if _, ok := env["api"]; ok {
		t.Fatalf("api entry should be gone: %#v", env["api"])
	}
	if env["web"]["K"] != "3" {
		t.Fatalf("web should keep its override: %#v", env)
	}
}

func TestPruneEnvServices(t *testing.T) {
	env := map[string]map[string]string{
		"api":  {"K": "1"},
		"gone": {"K": "2"},
	}
	pruned := pruneEnvServices(env, []string{"api"})
	if !reflect.DeepEqual(pruned, []string{"gone"}) {
		t.Fatalf("pruned = %v", pruned)
	}
	if _, ok := env["gone"]; ok {
		t.Fatalf("stale service kept: %#v", env)
	}
	if env["api"]["K"] != "1" {
		t.Fatalf("known service damaged: %#v", env)
	}
	if got := pruneEnvServices(nil, []string{"api"}); got != nil {
		t.Fatalf("nil env: %v", got)
	}
}

func TestEnvEntriesDeterministic(t *testing.T) {
	env := map[string]map[string]string{
		"web": {"B": "2", "A": "1"},
		"api": {"C": "3"},
	}
	got := envEntries(env)
	want := []EnvEntry{
		{Service: "api", Key: "C", Value: "3"},
		{Service: "web", Key: "A", Value: "1"},
		{Service: "web", Key: "B", Value: "2"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	// Empty store renders as a non-nil empty list (JSON [], not null).
	if got := envEntries(nil); got == nil || len(got) != 0 {
		t.Fatalf("got %#v", got)
	}
}

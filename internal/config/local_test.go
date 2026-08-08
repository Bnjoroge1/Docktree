package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadLocalOverridesMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overrides.yml")

	ov, err := LoadLocalOverrides(path)
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if !reflect.DeepEqual(ov, OverridesConfig{}) {
		t.Fatalf("expected empty OverridesConfig, got %+v", ov)
	}
}

func TestWriteAndLoadLocalOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overrides.yml")

	input := OverridesConfig{SkipServices: []string{"ui"}}
	if err := WriteLocalOverrides(path, input); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	ov, err := LoadLocalOverrides(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if !reflect.DeepEqual(ov.SkipServices, []string{"ui"}) {
		t.Fatalf("expected skip_services [ui], got %v", ov.SkipServices)
	}
}

func TestWriteAndLoadLocalOverridesWithEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overrides.yml")

	input := OverridesConfig{Environment: map[string]map[string]string{
		"api": {"CHAT_TOKENIZER_MODEL": "Qwen/Qwen3.6-35B-A3B", "LOG_LEVEL": "debug"},
		"web": {"FEATURE_X": "1"},
	}}
	if err := WriteLocalOverrides(path, input); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	ov, err := LoadLocalOverrides(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if !reflect.DeepEqual(ov.Environment, input.Environment) {
		t.Fatalf("environment round trip mismatch: %#v", ov.Environment)
	}
}

func TestWriteAndLoadLocalOverridesEmptyEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overrides.yml")

	// After unsetting everything the environment section must disappear
	// entirely (omitempty), loading back as nil.
	if err := WriteLocalOverrides(path, OverridesConfig{SkipServices: []string{"ui"}}); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	ov, err := LoadLocalOverrides(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if ov.Environment != nil {
		t.Fatalf("expected nil environment, got %#v", ov.Environment)
	}
}

func TestMergeEnvironmentLocalWinsPerKey(t *testing.T) {
	base := map[string]map[string]string{
		"api": {"A": "base", "B": "base"},
	}
	local := map[string]map[string]string{
		"api": {"A": "local"},
		"web": {"C": "local"},
	}

	merged := MergeEnvironment(base, local)

	want := map[string]map[string]string{
		"api": {"A": "local", "B": "base"},
		"web": {"C": "local"},
	}
	if !reflect.DeepEqual(merged, want) {
		t.Fatalf("merge mismatch: %#v", merged)
	}
	// Inputs must not be mutated through the merged result.
	merged["api"]["X"] = "new"
	if _, ok := base["api"]["X"]; ok {
		t.Fatal("merge shares maps with base input")
	}
	if _, ok := local["api"]["X"]; ok {
		t.Fatal("merge shares maps with local input")
	}
}

func TestMergeEnvironmentCorners(t *testing.T) {
	if got := MergeEnvironment(nil, nil); got != nil {
		t.Fatalf("nil+nil: %#v", got)
	}
	localOnly := map[string]map[string]string{"api": {"A": "1"}}
	if got := MergeEnvironment(nil, localOnly); !reflect.DeepEqual(got, localOnly) {
		t.Fatalf("nil base: %#v", got)
	}
	baseOnly := map[string]map[string]string{"api": {"A": "1"}}
	if got := MergeEnvironment(baseOnly, nil); !reflect.DeepEqual(got, baseOnly) {
		t.Fatalf("nil local: %#v", got)
	}
	if got := MergeEnvironment(map[string]map[string]string{"api": {}}, map[string]map[string]string{"web": {}}); got != nil {
		t.Fatalf("empty service maps should collapse to nil, got %#v", got)
	}
}

func TestMergeLocalOverridesMergesEnvironment(t *testing.T) {
	cfg := &Config{Overrides: OverridesConfig{Environment: map[string]map[string]string{
		"api": {"A": "base"},
	}}}
	local := OverridesConfig{Environment: map[string]map[string]string{
		"api": {"A": "local", "B": "local"},
	}}

	MergeLocalOverrides(cfg, local)

	want := map[string]map[string]string{"api": {"A": "local", "B": "local"}}
	if !reflect.DeepEqual(cfg.Overrides.Environment, want) {
		t.Fatalf("expected %v, got %v", want, cfg.Overrides.Environment)
	}
}

func TestMergeLocalOverridesUnionsSkipServices(t *testing.T) {
	cfg := &Config{Overrides: OverridesConfig{SkipServices: []string{"ui"}}}
	local := OverridesConfig{SkipServices: []string{"caddy"}}

	MergeLocalOverrides(cfg, local)

	want := []string{"caddy", "ui"}
	if !reflect.DeepEqual(cfg.Overrides.SkipServices, want) {
		t.Fatalf("expected %v, got %v", want, cfg.Overrides.SkipServices)
	}
}

func TestMergeLocalOverridesDedupes(t *testing.T) {
	cfg := &Config{Overrides: OverridesConfig{SkipServices: []string{"ui"}}}
	local := OverridesConfig{SkipServices: []string{"ui"}}

	MergeLocalOverrides(cfg, local)

	want := []string{"ui"}
	if !reflect.DeepEqual(cfg.Overrides.SkipServices, want) {
		t.Fatalf("expected %v, got %v", want, cfg.Overrides.SkipServices)
	}
}

func TestMergeLocalOverridesReplacesProfiles(t *testing.T) {
	cfg := &Config{Overrides: OverridesConfig{Profiles: []string{"seed"}}}
	local := OverridesConfig{Profiles: []string{"debug"}}

	MergeLocalOverrides(cfg, local)

	want := []string{"debug"}
	if !reflect.DeepEqual(cfg.Overrides.Profiles, want) {
		t.Fatalf("expected %v, got %v", want, cfg.Overrides.Profiles)
	}
}

func TestMergeLocalOverridesNoOpWhenEmpty(t *testing.T) {
	cfg := &Config{Overrides: OverridesConfig{SkipServices: []string{"ui"}, Profiles: []string{"seed"}}}
	local := OverridesConfig{}

	MergeLocalOverrides(cfg, local)

	if !reflect.DeepEqual(cfg.Overrides.SkipServices, []string{"ui"}) {
		t.Fatalf("expected skip_services unchanged, got %v", cfg.Overrides.SkipServices)
	}
	if !reflect.DeepEqual(cfg.Overrides.Profiles, []string{"seed"}) {
		t.Fatalf("expected profiles unchanged, got %v", cfg.Overrides.Profiles)
	}
}

func TestLocalOverridesPathDefaultsStateDir(t *testing.T) {
	got := LocalOverridesPath("/worktree", "")
	want := filepath.Join("/worktree", ".docktree", "overrides.yml")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestWriteLocalOverridesCreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "overrides.yml")

	if err := WriteLocalOverrides(path, OverridesConfig{SkipServices: []string{"ui"}}); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

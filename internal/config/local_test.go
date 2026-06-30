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

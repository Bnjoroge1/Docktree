package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingUsesDefaults(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Ports.Range != "41000-49999" {
		t.Fatalf("unexpected default range: %q", cfg.Ports.Range)
	}
	if len(cfg.Setup.Copy) != 1 || cfg.Setup.Copy[0] != ".env" {
		t.Fatalf("unexpected default setup.copy: %#v", cfg.Setup.Copy)
	}
	if len(cfg.Setup.Symlink) != 1 || cfg.Setup.Symlink[0] != "node_modules" {
		t.Fatalf("unexpected default setup.symlink: %#v", cfg.Setup.Symlink)
	}
	if cfg.Compose.Files != nil {
		t.Fatalf("compose files default should mean auto-detect, got %#v", cfg.Compose.Files)
	}
	if cfg.Worktrees.Root != "../${repo}.worktrees" {
		t.Fatalf("unexpected default worktrees.root: %q", cfg.Worktrees.Root)
	}
}

func TestLoadMergesPartialConfig(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "docktree.yml"), []byte("ports:\n  range: 42000-42010\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Ports.Range != "42000-42010" {
		t.Fatalf("range not loaded: %q", cfg.Ports.Range)
	}
	if cfg.Ports.BindHost != "127.0.0.1" || cfg.State.Directory != ".docktree" {
		t.Fatalf("defaults not merged: %#v", cfg)
	}
	if cfg.Worktrees.Root != "../${repo}.worktrees" {
		t.Fatalf("default worktrees.root not merged: %q", cfg.Worktrees.Root)
	}
}

func TestLoadMergesWorktreesConfig(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "docktree.yml"), []byte("worktrees:\n  root: ./.worktrees\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Worktrees.Root != "./.worktrees" {
		t.Fatalf("worktrees.root not loaded: %q", cfg.Worktrees.Root)
	}
}

func TestLoadMergesSetupConfig(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "docktree.yml"), []byte("setup:\n  copy:\n    - .env\n  symlink:\n    - node_modules\n  run:\n    - git submodule update --init --recursive\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Setup.Copy) != 1 || cfg.Setup.Copy[0] != ".env" {
		t.Fatalf("setup.copy not loaded: %#v", cfg.Setup.Copy)
	}
	if len(cfg.Setup.Symlink) != 1 || cfg.Setup.Symlink[0] != "node_modules" {
		t.Fatalf("setup.symlink not loaded: %#v", cfg.Setup.Symlink)
	}
	if len(cfg.Setup.Run) != 1 || cfg.Setup.Run[0] != "git submodule update --init --recursive" {
		t.Fatalf("setup.run not loaded: %#v", cfg.Setup.Run)
	}
}

func TestLoadMergesOverrides(t *testing.T) {
	dir := t.TempDir()
	content := `overrides:
  skip_services:
    - infisical
    - ui
    - caddy
  drop_dependencies:
    - infisical
  profiles:
    - seed
    - debug
`
	err := os.WriteFile(filepath.Join(dir, "docktree.yml"), []byte(content), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Overrides.SkipServices) != 3 {
		t.Fatalf("skip_services not loaded: %#v", cfg.Overrides.SkipServices)
	}
	if cfg.Overrides.SkipServices[0] != "infisical" || cfg.Overrides.SkipServices[1] != "ui" || cfg.Overrides.SkipServices[2] != "caddy" {
		t.Fatalf("skip_services order/content wrong: %#v", cfg.Overrides.SkipServices)
	}
	if len(cfg.Overrides.DropDependencies) != 1 || cfg.Overrides.DropDependencies[0] != "infisical" {
		t.Fatalf("drop_dependencies not loaded: %#v", cfg.Overrides.DropDependencies)
	}
	if len(cfg.Overrides.Profiles) != 2 || cfg.Overrides.Profiles[0] != "seed" || cfg.Overrides.Profiles[1] != "debug" {
		t.Fatalf("profiles not loaded: %#v", cfg.Overrides.Profiles)
	}
}

func TestLoadMergesOverridesEmptySliceOverride(t *testing.T) {
	// Explicitly empty slice should override the default nil.
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "docktree.yml"), []byte("overrides:\n  skip_services: []\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Overrides.SkipServices == nil || len(cfg.Overrides.SkipServices) != 0 {
		t.Fatalf("expected empty skip_services slice, got %#v", cfg.Overrides.SkipServices)
	}
}

func TestValidateOverridesSkipsSharedService(t *testing.T) {
	dir := t.TempDir()
	content := `overrides:
  skip_services:
    - db

shared:
  services:
    db:
      kind: postgres
      tenancy: per_database
      url_envs:
        - DATABASE_URL
`
	err := os.WriteFile(filepath.Join(dir, "docktree.yml"), []byte(content), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Load(dir)
	if err == nil {
		t.Fatal("expected error for shared service in skip_services")
	}
	want := `overrides.skip_services: cannot skip shared service "db"`
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error containing %q, got %q", want, err.Error())
	}
}

func TestValidateOverridesDuplicateSkip(t *testing.T) {
	dir := t.TempDir()
	content := `overrides:
  skip_services:
    - ui
    - ui
`
	err := os.WriteFile(filepath.Join(dir, "docktree.yml"), []byte(content), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Load(dir)
	if err == nil {
		t.Fatal("expected error for duplicate skip_services")
	}
	want := "overrides.skip_services: duplicate entries: ui"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error containing %q, got %q", want, err.Error())
	}
}

func TestValidateOverridesDuplicateDropDependencies(t *testing.T) {
	dir := t.TempDir()
	content := `overrides:
  drop_dependencies:
    - infisical
    - infisical
`
	err := os.WriteFile(filepath.Join(dir, "docktree.yml"), []byte(content), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Load(dir)
	if err == nil {
		t.Fatal("expected error for duplicate drop_dependencies")
	}
	want := "overrides.drop_dependencies: duplicate entries: infisical"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error containing %q, got %q", want, err.Error())
	}
}

func TestLoadEmptyOverridesOK(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "docktree.yml"), []byte("ports:\n  range: 42000-42010\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Overrides.SkipServices != nil || cfg.Overrides.DropDependencies != nil || cfg.Overrides.Profiles != nil {
		t.Fatalf("expected nil override slices when absent, got %#v", cfg.Overrides)
	}
}

func TestScaffoldOmitsEmptyOverrides(t *testing.T) {
	cfg := Defaults()
	dir := t.TempDir()
	scaffolded, err := Scaffold(dir, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !scaffolded {
		t.Fatal("expected scaffolded=true")
	}
	data, err := os.ReadFile(filepath.Join(dir, "docktree.yml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if strings.Contains(s, "overrides:") {
		t.Fatalf("scaffolded YAML should not contain 'overrides:', got:\n%s", s)
	}
}

func TestScaffoldOmitsEmptySlices(t *testing.T) {
	cfg := Defaults()
	dir := t.TempDir()
	scaffolded, err := Scaffold(dir, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !scaffolded {
		t.Fatal("expected scaffolded=true")
	}
	data, err := os.ReadFile(filepath.Join(dir, "docktree.yml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if strings.Contains(s, "files: []") {
		t.Fatalf("scaffolded YAML should not contain 'files: []', got:\n%s", s)
	}
	if strings.Contains(s, "services: []") {
		t.Fatalf("scaffolded YAML should not contain 'services: []', got:\n%s", s)
	}
	if strings.Contains(s, "share: []") {
		t.Fatalf("scaffolded YAML should not contain 'share: []', got:\n%s", s)
	}
	if strings.Contains(s, "run: []") {
		t.Fatalf("scaffolded YAML should not contain 'run: []', got:\n%s", s)
	}
	if !strings.Contains(s, "copy:") {
		t.Fatalf("scaffolded YAML should keep non-empty defaults like 'copy:', got:\n%s", s)
	}
}

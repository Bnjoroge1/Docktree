package config

import (
	"os"
	"path/filepath"
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
	if cfg.Compose.Files != nil {
		t.Fatalf("compose files default should mean auto-detect, got %#v", cfg.Compose.Files)
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

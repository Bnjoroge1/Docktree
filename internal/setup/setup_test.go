package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bnjoroge/docktree/internal/config"
)

func TestPrepareCopiesAndSymlinksPaths(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, ".env"), []byte("PORT=3000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "node_modules", "pkg.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Setup.Copy = []string{".env"}
	cfg.Setup.Symlink = []string{"node_modules"}
	if err := Prepare(Options{SourceDir: source, TargetDir: target, Config: &cfg}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, ".env")); err != nil {
		t.Fatalf("copied .env missing: %v", err)
	}
	linked := filepath.Join(target, "node_modules")
	info, err := os.Lstat(linked)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink for node_modules, got mode %v", info.Mode())
	}
	resolved, err := os.Readlink(linked)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != filepath.Join(source, "node_modules") {
		t.Fatalf("symlink target = %q, want %q", resolved, filepath.Join(source, "node_modules"))
	}
}

func TestPrepareRunsCommandsInTargetDir(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	cfg := config.Defaults()
	cfg.Setup.Run = []string{"printf 'ok' > prepared.txt"}
	if err := Prepare(Options{SourceDir: source, TargetDir: target, Config: &cfg}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "prepared.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ok" {
		t.Fatalf("prepared.txt = %q, want ok", string(data))
	}
}

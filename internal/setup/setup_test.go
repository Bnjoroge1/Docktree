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

func TestPrepareSkipsCopyAndSymlinkForSameDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PORT=3000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Setup.Run = []string{"printf 'ok' > prepared.txt"}
	if err := Prepare(Options{SourceDir: dir, TargetDir: dir, Config: &cfg}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "prepared.txt")); err != nil {
		t.Fatalf("prepared.txt missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env")); err != nil {
		t.Fatalf(".env missing after prepare: %v", err)
	}
}
func TestStaleFilesDetectsDifferentContent(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, ".env"), []byte("PORT=3000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Target has different content.
	if err := os.WriteFile(filepath.Join(target, ".env"), []byte("PORT=4000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Setup.Copy = []string{".env"}
	stale := StaleFiles(source, target, &cfg)
	if len(stale) != 1 || stale[0] != ".env" {
		t.Fatalf("StaleFiles = %v, want [.env]", stale)
	}
}

func TestStaleFilesDetectsMissingTarget(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, ".env"), []byte("PORT=3000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Target has no .env.
	cfg := config.Defaults()
	cfg.Setup.Copy = []string{".env"}
	stale := StaleFiles(source, target, &cfg)
	if len(stale) != 1 || stale[0] != ".env" {
		t.Fatalf("StaleFiles = %v, want [.env]", stale)
	}
}

func TestStaleFilesReturnsEmptyWhenIdentical(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	data := []byte("PORT=3000\n")
	if err := os.WriteFile(filepath.Join(source, ".env"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, ".env"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Setup.Copy = []string{".env"}
	stale := StaleFiles(source, target, &cfg)
	if len(stale) != 0 {
		t.Fatalf("StaleFiles = %v, want empty", stale)
	}
}

func TestStaleFilesSkipsMissingSource(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	// Source has no .env — nothing to sync.
	cfg := config.Defaults()
	cfg.Setup.Copy = []string{".env"}
	stale := StaleFiles(source, target, &cfg)
	if len(stale) != 0 {
		t.Fatalf("StaleFiles = %v, want empty", stale)
	}
}

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComposeFileReMatches(t *testing.T) {
	match := []string{
		"compose.yml",
		"compose.yaml",
		"docker-compose.yml",
		"docker-compose.yaml",
		"docker-compose.dev.yml",
		"docker-compose.dev.yaml",
		"compose.override.yml",
		"compose.override.yaml",
	}
	for _, name := range match {
		if !composeFileRe.MatchString(name) {
			t.Errorf("expected %q to match", name)
		}
	}
}

func TestComposeFileReRejects(t *testing.T) {
	reject := []string{
		"platform-compose.yml",
		"worktree-compose.yml",
		"mycompose.yml",
		"compose.json",
		"compose.txt",
		"README.md",
		"Dockerfile",
	}
	for _, name := range reject {
		if composeFileRe.MatchString(name) {
			t.Errorf("expected %q to NOT match", name)
		}
	}
}

func TestDiscoverComposeFilesFindsSubdir(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "infra")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "docker-compose.yml"), []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	candidates := discoverComposeFiles(root)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %v", len(candidates), candidates)
	}
	if filepath.Base(candidates[0]) != "docker-compose.yml" {
		t.Errorf("unexpected candidate: %s", candidates[0])
	}
}

func TestDiscoverComposeFilesSkipsRoot(t *testing.T) {
	root := t.TempDir()
	// Place a compose file in the root — should be ignored (already found by compose-go).
	if err := os.WriteFile(filepath.Join(root, "compose.yml"), []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	candidates := discoverComposeFiles(root)
	if len(candidates) != 0 {
		t.Fatalf("expected 0 candidates for root-only file, got %d: %v", len(candidates), candidates)
	}
}

func TestDiscoverComposeFilesSkipsDocktree(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, ".docktree", "generated")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(generated, "compose.yml"), []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	candidates := discoverComposeFiles(root)
	if len(candidates) != 0 {
		t.Fatalf("expected 0 candidates in .docktree, got %d: %v", len(candidates), candidates)
	}
}

func TestDiscoverComposeFilesSkipsTestdata(t *testing.T) {
	root := t.TempDir()
	td := filepath.Join(root, "testdata")
	if err := os.MkdirAll(td, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(td, "docker-compose.yml"), []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	candidates := discoverComposeFiles(root)
	if len(candidates) != 0 {
		t.Fatalf("expected 0 candidates in testdata, got %d: %v", len(candidates), candidates)
	}
}

func TestDiscoverComposeFilesMultipleDirs(t *testing.T) {
	root := t.TempDir()
	for _, sub := range []string{"infra", "deploy"} {
		dir := filepath.Join(root, sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("version: '3'\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	candidates := discoverComposeFiles(root)
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d: %v", len(candidates), candidates)
	}
}

func TestDiscoverComposeFilesDedupesSameDir(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "infra")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// Two compose files in the same directory — should return only one candidate.
	for _, name := range []string{"compose.yml", "compose.override.yml"} {
		if err := os.WriteFile(filepath.Join(sub, name), []byte("version: '3'\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	candidates := discoverComposeFiles(root)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate (deduped), got %d: %v", len(candidates), candidates)
	}
}

func TestDiscoverComposeFilesDepthLimit(t *testing.T) {
	root := t.TempDir()
	// 4 levels deep — should be skipped.
	deep := filepath.Join(root, "a", "b", "c", "d")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deep, "docker-compose.yml"), []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	candidates := discoverComposeFiles(root)
	if len(candidates) != 0 {
		t.Fatalf("expected 0 candidates beyond depth limit, got %d: %v", len(candidates), candidates)
	}
}

func TestDiscoverComposeFilesRejectsGenerated(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "output")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// Generated-style names should not match.
	if err := os.WriteFile(filepath.Join(sub, "platform-compose.yml"), []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	candidates := discoverComposeFiles(root)
	if len(candidates) != 0 {
		t.Fatalf("expected 0 candidates for generated name, got %d: %v", len(candidates), candidates)
	}
}

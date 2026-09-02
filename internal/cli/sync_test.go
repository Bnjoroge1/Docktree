package cli

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/state"
)

// TestRunSyncSkipsWorktreesWithNoStaleFiles is the regression test for the
// dropped `continue` in runSync: a worktree whose setup.copy files already
// match the source must not produce a SyncItem, and with nothing to sync the
// command must report ExitNoop rather than an empty overwrite.
func TestRunSyncSkipsWorktreesWithNoStaleFiles(t *testing.T) {
	globalDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", globalDir)

	mainRoot := t.TempDir()
	worktree := t.TempDir()

	// docktree.yml declares a setup.copy entry, and both roots hold identical
	// content for it, so StaleFiles returns nothing.
	if err := os.WriteFile(filepath.Join(mainRoot, "docktree.yml"), []byte("setup:\n  copy:\n    - .env\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{mainRoot, worktree} {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("A=1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	inst := &state.Instance{
		Name:         "repo-feature-abc123",
		RepoRoot:     mainRoot,
		WorktreeRoot: worktree,
	}
	if err := state.UpsertGlobalInstance(filepath.Join(globalDir, "docktree"), inst); err != nil {
		t.Fatal(err)
	}

	ctx := &Context{
		Args:     []string{"sync", "--force"},
		Renderer: output.New(io.Discard, true),
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	}
	res, code, err := runSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if code != output.ExitNoop {
		t.Fatalf("nothing stale should be a no-op: code = %d, want %d", code, output.ExitNoop)
	}
	sr, ok := res.(SyncResult)
	if !ok {
		t.Fatalf("unexpected result type %T", res)
	}
	if len(sr.Items) != 0 {
		t.Fatalf("stale-free worktree produced sync items: %#v", sr.Items)
	}
}

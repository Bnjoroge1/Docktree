package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bnjoroge/docktree/internal/config"
	dockgit "github.com/bnjoroge/docktree/internal/git"
	"github.com/bnjoroge/docktree/internal/state"
)

// TestResolveInstanceNamePrefersSavedProjectIdentity is the regression test
// for issue #62: once a worktree has created an instance, its persisted
// project name is authoritative — branch checkout, rename, and detached HEAD
// moves must never change the Compose project identity.
func TestResolveInstanceNamePrefersSavedProjectIdentity(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "worktrees", "feature-a")
	cfg := config.Defaults()
	repo := dockgit.RepoInfo{RepoRoot: root, WorktreeRoot: worktree, Branch: "feature/a"}

	// No instance state yet: identity is derived from the current branch.
	first, err := resolveInstanceName(repo, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := dockgit.InstanceName(dockgit.RepoName(root), dockgit.WorktreeName("feature/a", worktree), root, worktree)
	if first != want {
		t.Fatalf("pre-state name = %q, want %q", first, want)
	}

	// Simulate the first `docktree up`.
	stateDir := state.StatePath(worktree, cfg.State.Directory)
	if err := state.SaveInstance(stateDir, &state.Instance{
		Name:           first,
		ProjectName:    first,
		Branch:         "feature/a",
		WorktreeRoot:   worktree,
		StateDirectory: stateDir,
	}); err != nil {
		t.Fatal(err)
	}

	// Branch checkout in the same worktree must not change the identity.
	repo.Branch = "feature/b"
	got, err := resolveInstanceName(repo, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got != first {
		t.Fatalf("after checkout: name = %q, want %q", got, first)
	}

	// Branch rename must not change the identity either.
	repo.Branch = "feature/c"
	got, err = resolveInstanceName(repo, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got != first {
		t.Fatalf("after rename: name = %q, want %q", got, first)
	}

	// Detached HEAD moving between commits must not change the identity.
	repo.Branch = "a1b2c3d"
	got, err = resolveInstanceName(repo, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got != first {
		t.Fatalf("after detached HEAD: name = %q, want %q", got, first)
	}
}

// TestResolveInstanceNameFallsBackToLegacyNameField covers records written
// before ProjectName existed: the old Name field is the saved identity.
func TestResolveInstanceNameFallsBackToLegacyNameField(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "wt")
	cfg := config.Defaults()
	stateDir := state.StatePath(worktree, cfg.State.Directory)
	legacy := "legacy-project-name"
	if err := state.SaveInstance(stateDir, &state.Instance{Name: legacy, Branch: "main"}); err != nil {
		t.Fatal(err)
	}
	repo := dockgit.RepoInfo{RepoRoot: root, WorktreeRoot: worktree, Branch: "other/branch"}
	got, err := resolveInstanceName(repo, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got != legacy {
		t.Fatalf("name = %q, want legacy %q", got, legacy)
	}
}

// TestResolveInstanceNamePropagatesCorruptState ensures a worktree whose
// state cannot be read fails loudly instead of silently deriving a new
// identity that would fork the Compose project.
func TestResolveInstanceNamePropagatesCorruptState(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "wt")
	cfg := config.Defaults()
	stateDir := state.StatePath(worktree, cfg.State.Directory)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := dockgit.RepoInfo{RepoRoot: root, WorktreeRoot: worktree, Branch: "main"}
	if _, err := resolveInstanceName(repo, &cfg); err == nil {
		t.Fatal("expected error for corrupt state.json")
	}
}

// TestResolveInstanceNameErrorsOnEmptyIdentity guards against the identity
// fork a nameless-but-valid state record would cause: deriving a branch-based
// name here would silently start a second Compose project next to the original
// one, contradicting the fail-loudly behavior of the corrupt-state path.
func TestResolveInstanceNameErrorsOnEmptyIdentity(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "wt")
	cfg := config.Defaults()
	stateDir := state.StatePath(worktree, cfg.State.Directory)
	if err := state.SaveInstance(stateDir, &state.Instance{Branch: "main"}); err != nil {
		t.Fatal(err)
	}
	repo := dockgit.RepoInfo{RepoRoot: root, WorktreeRoot: worktree, Branch: "feature/b"}
	if _, err := resolveInstanceName(repo, &cfg); err == nil {
		t.Fatal("expected error for state record with no saved identity")
	}
}

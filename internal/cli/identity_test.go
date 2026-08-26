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

// TestStateDirOwnedByOther guards `docktree clean`: after an identity
// rollover both the old and the new global record can point at one worktree.
// Clean must drop the stale record without deleting the live state directory.
func TestStateDirOwnedByOther(t *testing.T) {
	shared := state.Instance{Name: "new", WorktreeRoot: "/wt", StateDirectory: "/wt/.docktree"}
	instances := map[string]state.Instance{
		"old": {Name: "old", WorktreeRoot: "/wt", StateDirectory: "/wt/.docktree"},
		"new": shared,
	}

	if !stateDirOwnedByOther(instances, "old", &state.Instance{WorktreeRoot: "/wt", StateDirectory: "/wt/.docktree"}) {
		t.Fatal("old record should be owned by the live new record")
	}
	// While both records exist, the new record's dir is likewise owned by the
	// stale record. applyCleanCandidates deletes each record from the map as
	// it removes it, so the dir is only deleted once the last owner is gone.
	if !stateDirOwnedByOther(instances, "new", &shared) {
		t.Fatal("while the stale record still exists, the state dir is owned")
	}

	// Records with distinct state directories never own each other's state.
	distinct := map[string]state.Instance{
		"a": {Name: "a", WorktreeRoot: "/wt1", StateDirectory: "/wt1/.docktree"},
		"b": {Name: "b", WorktreeRoot: "/wt2", StateDirectory: "/wt2/.docktree"},
	}
	if stateDirOwnedByOther(distinct, "a", &state.Instance{WorktreeRoot: "/wt1", StateDirectory: "/wt1/.docktree"}) {
		t.Fatal("distinct state dirs must not be treated as shared")
	}
	if stateDirOwnedByOther(distinct, "a", &state.Instance{WorktreeRoot: "/wt1"}) {
		t.Fatal("legacy record with no state dir must not claim a dir")
	}

	// Fallback: an instance without StateDirectory resolves to
	// <worktree>/.docktree, matching how RemoveStateDir locates it.
	legacy := map[string]state.Instance{
		"a": {Name: "a", WorktreeRoot: "/wt", StateDirectory: "/wt/.docktree"},
	}
	if !stateDirOwnedByOther(legacy, "old", &state.Instance{Name: "old", WorktreeRoot: "/wt"}) {
		t.Fatal("legacy record should resolve its default state dir")
	}
}

package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoadInstance(t *testing.T) {
	dir := t.TempDir()
	inst := &Instance{Name: "repo-branch-abcdef", ProjectName: "repo-branch-abcdef", CreatedAt: time.Now().UTC()}
	if err := SaveInstance(dir, inst); err != nil {
		t.Fatal(err)
	}
	got, err := LoadInstance(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != inst.Name || got.ProjectName != inst.ProjectName {
		t.Fatalf("round trip mismatch: %#v", got)
	}
}

func TestHashFilesStableAndChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compose.yml")
	if err := os.WriteFile(path, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := HashFiles([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashFiles([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("hash not stable: %s %s", a, b)
	}
	if err := os.WriteFile(path, []byte("services:\n  web: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := HashFiles([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if c == a {
		t.Fatalf("hash did not change after file update")
	}
}

func TestGlobalInstancesRoundTripAndRemove(t *testing.T) {
	dir := t.TempDir()
	inst := Instance{Name: "repo-branch-abcdef", WorktreeRoot: "/tmp/worktree"}
	if err := UpsertGlobalInstance(dir, &inst); err != nil {
		t.Fatal(err)
	}
	got, err := LoadGlobalInstances(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got["repo-branch-abcdef"].WorktreeRoot != inst.WorktreeRoot {
		t.Fatalf("bad round trip: %#v", got)
	}
	if err := RemoveGlobalInstance(dir, "repo-branch-abcdef"); err != nil {
		t.Fatal(err)
	}
	got, err = LoadGlobalInstances(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["repo-branch-abcdef"]; ok {
		t.Fatalf("instance was not removed: %#v", got)
	}
}

func TestRemoveStateDir(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".docktree")
	if err := os.MkdirAll(filepath.Join(stateDir, "generated"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RemoveStateDir(&Instance{StateDirectory: stateDir}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Fatalf("state dir still exists: %v", err)
	}
}

// TestUpsertGlobalInstanceDedupesStaleIdentity is the regression test for
// issue #62: when a worktree's identity changes (branch checkout/rename), the
// stale record that points at the same worktree must not survive alongside the
// new one — duplicate records sharing a StateDirectory would let `docktree
// clean` delete live state through the obsolete identity.
func TestUpsertGlobalInstanceDedupesStaleIdentity(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "wt", ".docktree")
	if err := UpsertGlobalInstance(dir, &Instance{Name: "repo-feature-a-abc123", WorktreeRoot: filepath.Join(dir, "wt"), StateDirectory: stateDir}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertGlobalInstance(dir, &Instance{Name: "repo-feature-b-def456", WorktreeRoot: filepath.Join(dir, "wt"), StateDirectory: stateDir}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadGlobalInstances(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("stale identity was not deduped: %#v", got)
	}
	if _, ok := got["repo-feature-b-def456"]; !ok {
		t.Fatalf("new identity missing after upsert: %#v", got)
	}
}

func TestUpsertGlobalInstanceDedupesByStateDir(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "wt", ".docktree")
	// Legacy record with a different worktree path but the same state dir.
	if err := UpsertGlobalInstance(dir, &Instance{Name: "old-name", WorktreeRoot: filepath.Join(dir, "wt-old"), StateDirectory: stateDir}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertGlobalInstance(dir, &Instance{Name: "new-name", WorktreeRoot: filepath.Join(dir, "wt"), StateDirectory: stateDir}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadGlobalInstances(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one record after state-dir dedupe: %#v", got)
	}
	if _, ok := got["new-name"]; !ok {
		t.Fatalf("new identity missing after upsert: %#v", got)
	}
}

func TestUpsertGlobalInstanceKeepsDistinctWorktrees(t *testing.T) {
	dir := t.TempDir()
	if err := UpsertGlobalInstance(dir, &Instance{Name: "repo-a-abc123", WorktreeRoot: filepath.Join(dir, "wt-a"), StateDirectory: filepath.Join(dir, "wt-a", ".docktree")}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertGlobalInstance(dir, &Instance{Name: "repo-b-def456", WorktreeRoot: filepath.Join(dir, "wt-b"), StateDirectory: filepath.Join(dir, "wt-b", ".docktree")}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadGlobalInstances(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("distinct worktrees must keep separate records: %#v", got)
	}
}

// TestUpsertGlobalInstanceKeepsSiblingSubprojects is the regression test for
// issue #61: independent subprojects legitimately share one worktree root, so
// registering one must never evict the other.
func TestUpsertGlobalInstanceKeepsSiblingSubprojects(t *testing.T) {
	dir := t.TempDir()
	worktree := filepath.Join(dir, "wt")
	for _, sub := range []string{"project-a", "project-b"} {
		inst := &Instance{
			Name:           "repo-" + sub + "-main-abc123",
			WorktreeRoot:   worktree,
			Subpath:        sub,
			StateDirectory: filepath.Join(worktree, sub, ".docktree"),
		}
		if err := UpsertGlobalInstance(dir, inst); err != nil {
			t.Fatal(err)
		}
	}
	got, err := LoadGlobalInstances(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("sibling subprojects must keep separate records: %#v", got)
	}

	// A root-scoped record in the same worktree is a third, distinct project.
	if err := UpsertGlobalInstance(dir, &Instance{
		Name:           "repo-main-def456",
		WorktreeRoot:   worktree,
		StateDirectory: filepath.Join(worktree, ".docktree"),
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ = LoadGlobalInstances(dir); len(got) != 3 {
		t.Fatalf("root project must not evict subprojects: %#v", got)
	}

	// Re-registering one subproject under a new identity still evicts only its
	// own stale record.
	if err := UpsertGlobalInstance(dir, &Instance{
		Name:           "repo-project-a-feature-999999",
		WorktreeRoot:   worktree,
		Subpath:        "project-a",
		StateDirectory: filepath.Join(worktree, "project-a", ".docktree"),
	}); err != nil {
		t.Fatal(err)
	}
	got, err = LoadGlobalInstances(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("rename must replace only the same subproject record: %#v", got)
	}
	if _, ok := got["repo-project-a-main-abc123"]; ok {
		t.Fatal("stale project-a identity survived")
	}
	if _, ok := got["repo-project-b-main-abc123"]; !ok {
		t.Fatal("project-b record was evicted by project-a rename")
	}
}

func TestUpsertGlobalInstanceKeepsLegacyRecordsWithEmptyPaths(t *testing.T) {
	dir := t.TempDir()
	if err := UpsertGlobalInstance(dir, &Instance{Name: "repo-a-abc123"}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertGlobalInstance(dir, &Instance{Name: "repo-b-def456"}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadGlobalInstances(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("records without path info must not be deduped: %#v", got)
	}
}

// TestUpsertGlobalInstanceResolvesRelativeStateDirs guards against a relative
// StateDirectory on two different worktrees being treated as the same
// directory (the comparison must resolve each against its own WorktreeRoot).
func TestUpsertGlobalInstanceResolvesRelativeStateDirs(t *testing.T) {
	dir := t.TempDir()
	if err := UpsertGlobalInstance(dir, &Instance{Name: "repo-a-abc123", WorktreeRoot: filepath.Join(dir, "wt-a"), StateDirectory: ".docktree"}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertGlobalInstance(dir, &Instance{Name: "repo-b-def456", WorktreeRoot: filepath.Join(dir, "wt-b"), StateDirectory: ".docktree"}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadGlobalInstances(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("relative state dirs on distinct worktrees must not collide: %#v", got)
	}
}

func TestInstanceStateDir(t *testing.T) {
	if got := InstanceStateDir(nil); got != "" {
		t.Fatalf("nil instance: got %q", got)
	}
	if got := InstanceStateDir(&Instance{}); got != "" {
		t.Fatalf("no anchors: got %q", got)
	}
	want := filepath.Join("/wt", ".docktree")
	if got := InstanceStateDir(&Instance{WorktreeRoot: "/wt"}); got != want {
		t.Fatalf("default: got %q, want %q", got, want)
	}
	if got := InstanceStateDir(&Instance{WorktreeRoot: "/wt", StateDirectory: "/elsewhere/state"}); got != "/elsewhere/state" {
		t.Fatalf("persisted: got %q", got)
	}
}

func TestRemoveStateDirIfUnreferenced(t *testing.T) {
	dir := t.TempDir()
	shared := filepath.Join(dir, "wt", ".docktree")
	if err := os.MkdirAll(filepath.Join(shared, "generated"), 0o755); err != nil {
		t.Fatal(err)
	}
	live := Instance{Name: "repo-feature-b-def456", WorktreeRoot: filepath.Join(dir, "wt"), StateDirectory: shared}
	stale := Instance{Name: "repo-feature-a-abc123", WorktreeRoot: filepath.Join(dir, "wt"), StateDirectory: shared}
	// Simulate the pre-fix rollover: both records coexist in the store,
	// pointing at one state directory (the dedupe in UpsertGlobalInstance
	// would prevent this going forward).
	if err := SaveGlobalInstances(dir, map[string]Instance{"repo-feature-b-def456": live, "repo-feature-a-abc123": stale}); err != nil {
		t.Fatal(err)
	}
	// The stale record's dir is still owned by the live record.
	if err := RemoveStateDirIfUnreferenced(dir, &stale); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(shared); err != nil {
		t.Fatalf("live state dir was removed: %v", err)
	}

	// A record whose dir no other instance references is removed.
	orphan := Instance{Name: "repo-orphan-000000", WorktreeRoot: filepath.Join(dir, "gone"), StateDirectory: filepath.Join(dir, "gone", ".docktree")}
	if err := os.MkdirAll(orphan.StateDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RemoveStateDirIfUnreferenced(dir, &orphan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan.StateDirectory); !os.IsNotExist(err) {
		t.Fatalf("unreferenced state dir was not removed: %v", err)
	}
}

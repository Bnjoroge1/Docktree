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

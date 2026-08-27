package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bnjoroge/docktree/internal/config"
	dockgit "github.com/bnjoroge/docktree/internal/git"
	"github.com/bnjoroge/docktree/internal/state"
)

// monorepo builds a repository with two independent subprojects plus a nested
// package inside the first, matching the layout in issue #61.
func monorepo(t *testing.T) string {
	t.Helper()
	repo := initGitRepo(t)
	for _, project := range []string{"project-a", "project-b"} {
		dir := filepath.Join(repo, project)
		if err := os.MkdirAll(filepath.Join(dir, "packages", "pkg1"), 0o755); err != nil {
			t.Fatal(err)
		}
		cfg := "compose:\n  files:\n    - compose.yml\n"
		if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
		compose := "services:\n  web:\n    image: alpine\n"
		if err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte(compose), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

func TestResolveRepoSelectsNearestSubproject(t *testing.T) {
	repo := monorepo(t)
	tests := []struct {
		cwd  string
		want string
	}{
		{repo, ""},
		{filepath.Join(repo, "project-a"), "project-a"},
		{filepath.Join(repo, "project-b"), "project-b"},
		{filepath.Join(repo, "project-a", "packages", "pkg1"), "project-a"},
	}
	for _, tt := range tests {
		t.Chdir(tt.cwd)
		got, err := resolveRepo("")
		if err != nil {
			t.Fatalf("cwd %s: %v", tt.cwd, err)
		}
		if got.Subpath != tt.want {
			t.Fatalf("cwd %s: subpath = %q, want %q", tt.cwd, got.Subpath, tt.want)
		}
		wantConfig := filepath.Join(got.RepoRoot, filepath.FromSlash(tt.want))
		if got.ConfigRoot != wantConfig {
			t.Fatalf("cwd %s: config root = %q, want %q", tt.cwd, got.ConfigRoot, wantConfig)
		}
		if got.ProjectRoot != filepath.Join(got.WorktreeRoot, filepath.FromSlash(tt.want)) {
			t.Fatalf("cwd %s: project root = %q", tt.cwd, got.ProjectRoot)
		}
	}
}

// A repository-root docktree.yml keeps owning nested directories, so existing
// single-project repos are unaffected by subproject discovery.
func TestResolveRepoRootConfigWinsForNestedDirs(t *testing.T) {
	repo := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, config.FileName), []byte("compose:\n  files: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(repo, "packages", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	got, err := resolveRepo("")
	if err != nil {
		t.Fatal(err)
	}
	if got.Subpath != "" {
		t.Fatalf("subpath = %q, want empty", got.Subpath)
	}
}

func TestResolveRepoNoConfigAnywhere(t *testing.T) {
	repo := initGitRepo(t)
	nested := filepath.Join(repo, "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	got, err := resolveRepo("")
	if err != nil {
		t.Fatal(err)
	}
	if got.Subpath != "" || got.ConfigRoot != got.RepoRoot || got.ProjectRoot != got.WorktreeRoot {
		t.Fatalf("expected repository-root scope, got %+v", got)
	}
}

func TestResolveRepoExplicitConfig(t *testing.T) {
	repo := monorepo(t)
	t.Chdir(repo)

	got, err := resolveRepo(filepath.Join("project-b", config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if got.Subpath != "project-b" {
		t.Fatalf("subpath = %q, want project-b", got.Subpath)
	}

	// A directory selector is equivalent to naming the file inside it.
	got, err = resolveRepo("project-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Subpath != "project-a" {
		t.Fatalf("subpath = %q, want project-a", got.Subpath)
	}

	// --config must not silently fall back to repository-root scope.
	if _, err := resolveRepo(filepath.Join("project-a", "nope.yml")); err == nil {
		t.Fatal("expected error for missing config file")
	}
	if _, err := resolveRepo(filepath.Join("project-a", "packages")); err == nil {
		t.Fatal("expected error for directory without docktree.yml")
	}
	outside := t.TempDir()
	if _, err := resolveRepo(outside); err == nil {
		t.Fatal("expected error for config outside the worktree")
	}
}

// Discovery from a subproject must not leak the sibling project's identity or
// state, which is the acceptance criterion for concurrent subproject runs.
func TestSubprojectInstancesAreIndependent(t *testing.T) {
	repo := monorepo(t)
	cfg := config.Defaults()

	names := map[string]string{}
	for _, project := range []string{"", "project-a", "project-b"} {
		t.Chdir(filepath.Join(repo, filepath.FromSlash(project)))
		scope, err := resolveRepo("")
		if err != nil {
			t.Fatal(err)
		}
		name, err := resolveInstanceName(scope, &cfg)
		if err != nil {
			t.Fatal(err)
		}
		names[project] = name
	}
	if names["project-a"] == names["project-b"] {
		t.Fatalf("sibling projects share identity %q", names["project-a"])
	}
	if names[""] == names["project-a"] {
		t.Fatalf("root and subproject share identity %q", names[""])
	}
}

func TestEnsureGitignoreQualifiesSubprojectStateDir(t *testing.T) {
	root := t.TempDir()
	repo := dockgit.RepoInfo{RepoRoot: root, WorktreeRoot: root}.WithSubpath("project-a")
	if err := ensureGitignore(repo, ".docktree"); err != nil {
		t.Fatal(err)
	}
	// A second project appends its own entry rather than replacing the first.
	if err := ensureGitignore(repo.WithSubpath("project-b"), ".docktree"); err != nil {
		t.Fatal(err)
	}
	// Repeat calls are idempotent.
	if err := ensureGitignore(repo, ".docktree"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	want := "project-a/.docktree/\nproject-b/.docktree/\n"
	if string(data) != want {
		t.Fatalf(".gitignore = %q, want %q", string(data), want)
	}
}

func TestInstanceScopeHelpers(t *testing.T) {
	inst := stateInstance("/repo", "/repo/wt", "project-a")
	if got := instanceConfigRoot(inst); got != filepath.Join("/repo", "project-a") {
		t.Fatalf("config root = %q", got)
	}
	if got := instanceProjectRoot(inst); got != filepath.Join("/repo", "wt", "project-a") {
		t.Fatalf("project root = %q", got)
	}
	// Instances recorded before subproject support carry no subpath and must
	// resolve exactly to their stored roots.
	legacy := stateInstance("/repo", "/repo/wt", "")
	if got := instanceConfigRoot(legacy); got != "/repo" {
		t.Fatalf("legacy config root = %q", got)
	}
	if got := instanceProjectRoot(legacy); got != filepath.Join("/repo", "wt") {
		t.Fatalf("legacy project root = %q", got)
	}
}

func TestParseGlobalFlags(t *testing.T) {
	jsonMode, cfgPath, rest, err := parseGlobalFlags([]string{"--json", "--config", "project-a/docktree.yml", "up", "--sync"})
	if err != nil {
		t.Fatal(err)
	}
	if !jsonMode || cfgPath != "project-a/docktree.yml" {
		t.Fatalf("jsonMode=%v config=%q", jsonMode, cfgPath)
	}
	if len(rest) != 2 || rest[0] != "up" || rest[1] != "--sync" {
		t.Fatalf("rest = %v", rest)
	}

	_, cfgPath, rest, err = parseGlobalFlags([]string{"--config=project-b", "status"})
	if err != nil {
		t.Fatal(err)
	}
	if cfgPath != "project-b" || len(rest) != 1 {
		t.Fatalf("config=%q rest=%v", cfgPath, rest)
	}

	if _, _, _, err := parseGlobalFlags([]string{"--config"}); err == nil {
		t.Fatal("expected error for --config without a value")
	}
}

func stateInstance(repoRoot, worktreeRoot, subpath string) *state.Instance {
	return &state.Instance{RepoRoot: repoRoot, WorktreeRoot: worktreeRoot, Subpath: subpath}
}

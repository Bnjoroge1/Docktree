package git

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestInstanceNameStableAndSafe(t *testing.T) {
	got := InstanceName("My Repo", "feature/auth", "/tmp/repo", "/tmp/repo-wt", "")
	again := InstanceName("My Repo", "feature/auth", "/tmp/repo", "/tmp/repo-wt", "")
	if got != again {
		t.Fatalf("name changed across runs: %q != %q", got, again)
	}
	if strings.Contains(got, "/") || strings.Contains(got, " ") {
		t.Fatalf("name was not slugged: %q", got)
	}
}

func TestInstanceNameTruncatesAt64CharsKeepingHash(t *testing.T) {
	tests := []struct {
		name     string
		repoName string
		workName string
		repoPath string
		workPath string
		subpath  string
	}{
		{
			name:     "long worktree",
			repoName: "repository",
			workName: strings.Repeat("branch-", 20),
			repoPath: "/tmp/repository",
			workPath: "/tmp/worktree",
		},
		{
			name:     "long repo",
			repoName: strings.Repeat("my-", 20) + "repo",
			workName: "main",
			repoPath: "/tmp/long-repo",
			workPath: "/tmp/long-repo-wt",
		},
		{
			name:     "both long",
			repoName: strings.Repeat("repo-", 12),
			workName: strings.Repeat("branch-", 12),
			repoPath: "/tmp/both",
			workPath: "/tmp/both-wt",
		},
		{
			name:     "repo name takes half",
			repoName: "a-really-long-repository-name-that-takes-half",
			workName: "feature-branch-x",
			repoPath: "/tmp/half",
			workPath: "/tmp/half-wt",
		},
		{
			name:     "short repo gives more space to worktree",
			repoName: "a",
			workName: strings.Repeat("branch-", 20),
			repoPath: "/tmp/short",
			workPath: "/tmp/short-wt",
		},
		{
			name:     "truncate slug trailing dash",
			repoName: "repo---",
			workName: "main",
			repoPath: "/tmp/trail",
			workPath: "/tmp/trail-wt",
		},
		{
			name:     "subproject with long segments",
			repoName: strings.Repeat("repo-", 12),
			workName: strings.Repeat("branch-", 12),
			repoPath: "/tmp/mono",
			workPath: "/tmp/mono-wt",
			subpath:  strings.Repeat("services/api-", 8),
		},
		{
			name:     "subproject trailing dash",
			repoName: "repo",
			workName: "main",
			repoPath: "/tmp/mono2",
			workPath: "/tmp/mono2-wt",
			subpath:  "packages/a---",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InstanceName(tt.repoName, tt.workName, tt.repoPath, tt.workPath, tt.subpath)
			if len(got) > 64 {
				t.Fatalf("name too long: %d %q", len(got), got)
			}
			parts := strings.Split(got, "-")
			if len(parts[len(parts)-1]) != 6 {
				t.Fatalf("hash suffix was not preserved: %q", got)
			}
			if strings.Contains(got, "--") || strings.HasPrefix(got, "-") || strings.HasSuffix(got, "-") {
				t.Fatalf("name has leading, trailing, or double dash: %q", got)
			}
		})
	}
}

func TestInstanceNameHashesRepoPath(t *testing.T) {
	a := InstanceName("repo", "feature/auth", "/tmp/one", "/tmp/one", "")
	b := InstanceName("repo", "feature/auth", "/tmp/two", "/tmp/two", "")
	if a == b {
		t.Fatalf("same branch in different repos produced same name: %q", a)
	}
}

// TestInstanceNameRootScopeUnchanged pins the pre-subproject naming scheme.
// Existing worktrees must keep their Compose project names across upgrade, so
// an empty subpath has to reproduce these bytes exactly.
func TestInstanceNameRootScopeUnchanged(t *testing.T) {
	tests := []struct {
		repoName string
		workName string
		repoPath string
		workPath string
		want     string
	}{
		{"docktree", "main", "/repos/docktree", "/repos/docktree", "docktree-main-a7bf50"},
		{"My Repo", "feature/auth", "/tmp/repo", "/tmp/repo-wt", "my-repo-feature-auth-1fd715"},
	}
	for _, tt := range tests {
		got := InstanceName(tt.repoName, tt.workName, tt.repoPath, tt.workPath, "")
		if got != tt.want {
			t.Fatalf("InstanceName(%q, %q, %q, %q, \"\") = %q, want %q", tt.repoName, tt.workName, tt.repoPath, tt.workPath, got, tt.want)
		}
	}
}

// TestInstanceNameSubpathIsolatesSubprojects covers the core issue #61
// requirement: two subprojects in one worktree must never share an identity,
// and each must differ from the repository-root identity.
func TestInstanceNameSubpathIsolatesSubprojects(t *testing.T) {
	root := InstanceName("mono", "main", "/repos/mono", "/repos/mono", "")
	a := InstanceName("mono", "main", "/repos/mono", "/repos/mono", "project-a")
	b := InstanceName("mono", "main", "/repos/mono", "/repos/mono", "project-b")
	nested := InstanceName("mono", "main", "/repos/mono", "/repos/mono", "project-a/packages/a1")
	for i, pair := range [][2]string{{root, a}, {root, b}, {a, b}, {a, nested}} {
		if pair[0] == pair[1] {
			t.Fatalf("case %d: identities collided: %q", i, pair[0])
		}
	}
	if !strings.Contains(a, "project-a") {
		t.Fatalf("subproject slug missing from %q", a)
	}
	// The same subproject in a different worktree is still distinct.
	other := InstanceName("mono", "feature", "/repos/mono", "/repos/mono-wt", "project-a")
	if other == a {
		t.Fatalf("same subproject in different worktrees collided: %q", a)
	}
}

func TestNormalizeSubpath(t *testing.T) {
	tests := map[string]string{
		"":                    "",
		".":                   "",
		"/":                   "",
		"project-a":           "project-a",
		"/project-a/":         "project-a",
		"project-a/packages/": "project-a/packages",
		"project-a/./b":       "project-a/b",
		"project-a/..":        "",
		"project-a/../project-b": "project-b",
		"foo/bar/../..":       "",
		"  apps/api  ":        "apps/api",
	}
	for in, want := range tests {
		if got := NormalizeSubpath(in); got != want {
			t.Fatalf("NormalizeSubpath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMainRepoRootFromMainRepo(t *testing.T) {
	root, err := MainRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root == "" {
		t.Fatal("expected non-empty root")
	}
}

func TestMainRepoRootMatchesRepoRoot(t *testing.T) {
	repo, err := DetectRepo()
	if err != nil {
		t.Skip("not in a git repo")
	}
	mainRoot, err := MainRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	mainRoot = filepath.Clean(mainRoot)
	repoRoot := filepath.Clean(repo.RepoRoot)
	if mainRoot != repoRoot {
		t.Logf("running from main repo: both should match, got mainRoot=%q repoRoot=%q", mainRoot, repoRoot)
	}

	mainRootForPath, err := MainRepoRootForPath(repo.RepoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(mainRootForPath) != mainRoot {
		t.Errorf("MainRepoRootForPath(%q) = %q, want %q", repo.RepoRoot, mainRootForPath, mainRoot)
	}
}

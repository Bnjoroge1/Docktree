package git

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestInstanceNameStableAndSafe(t *testing.T) {
	got := InstanceName("My Repo", "feature/auth", "/tmp/repo", "/tmp/repo-wt")
	again := InstanceName("My Repo", "feature/auth", "/tmp/repo", "/tmp/repo-wt")
	if got != again {
		t.Fatalf("name changed across runs: %q != %q", got, again)
	}
	if strings.Contains(got, "/") || strings.Contains(got, " ") {
		t.Fatalf("name was not slugged: %q", got)
	}
}

func TestInstanceNameTruncatesAt64CharsKeepingHash(t *testing.T) {
	tests := []struct {
		name       string
		repoName   string
		workName   string
		repoPath   string
		workPath   string
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InstanceName(tt.repoName, tt.workName, tt.repoPath, tt.workPath)
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
	a := InstanceName("repo", "feature/auth", "/tmp/one", "/tmp/one")
	b := InstanceName("repo", "feature/auth", "/tmp/two", "/tmp/two")
	if a == b {
		t.Fatalf("same branch in different repos produced same name: %q", a)
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

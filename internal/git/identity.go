package git

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var invalidProjectNameChars = regexp.MustCompile(`[^a-z0-9_-]+`)

type RepoInfo struct {
	RepoRoot     string
	WorktreeRoot string
	Branch       string
	Prefix       string
}

type WorktreeInfo struct {
	Path   string
	Branch string
	Main   bool
}

func DetectRepo() (RepoInfo, error) {
	repoRoot, err := gitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		return RepoInfo{}, err
	}
	prefix, err := gitOutput("rev-parse", "--show-prefix")
	if err != nil {
		return RepoInfo{}, err
	}
	branch, err := gitOutput("branch", "--show-current")
	if err != nil {
		return RepoInfo{}, err
	}
	if branch == "" {
		branch, _ = gitOutput("rev-parse", "--short", "HEAD")
	}
	root := filepath.Clean(repoRoot)
	return RepoInfo{
		RepoRoot:     root,
		WorktreeRoot: root,
		Branch:       branch,
		Prefix:       prefix,
	}, nil
}

func DetectWorktree() (WorktreeInfo, error) {
	repo, err := DetectRepo()
	if err != nil {
		return WorktreeInfo{}, err
	}
	out, err := gitOutput("worktree", "list", "--porcelain")
	if err != nil {
		return WorktreeInfo{}, err
	}
	entries := parseWorktreeList(out)
	for i, entry := range entries {
		if samePath(entry.Path, repo.WorktreeRoot) {
			entry.Main = i == 0
			return entry, nil
		}
	}
	return WorktreeInfo{Path: repo.WorktreeRoot, Branch: repo.Branch, Main: true}, nil
}

// InstanceName returns a Compose-safe, stable project name.
func InstanceName(repoName, worktreeName, repoPath, worktreePath string) string {
	repo := slugify(repoName)
	worktree := slugify(worktreeName)
	if repo == "" {
		repo = "repo"
	}
	if worktree == "" {
		worktree = "worktree"
	}

	//hash repo + worktree to avoid duplicate project names.
	sum := sha1.Sum([]byte(repoPath + "\x00" + worktreePath))
	hash := hex.EncodeToString(sum[:])[:6]
	suffix := "-" + hash

	// docker compose spec has a project limit of 64 chars so we divide the remainder of
	//name(after the hash) by 2 and share between the repo name and worktree
	avail := 64 - 1 - len(suffix)
	repo = truncateSlug(repo, avail/2)
	worktree = truncateSlug(worktree, avail-len(repo))
	return repo + "-" + worktree + suffix
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = invalidProjectNameChars.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-_")
	return value
}

func truncateSlug(value string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(value) <= max {
		return value
	}
	value = strings.Trim(value[:max], "-_")
	if value == "" {
		return "x"
	}
	return value
}

func gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(string(out)), nil
}

func gitOutputForPath(path string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", path}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git -C %s %s: %s", path, strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(string(out)), nil
}

// MainRepoRootForPath returns the path to the main repository root of the git repository containing path.
func MainRepoRootForPath(path string) (string, error) {
	out, err := gitOutputForPath(path, "worktree", "list", "--porcelain")
	if err != nil {
		return gitOutputForPath(path, "rev-parse", "--show-toplevel")
	}
	entries := parseWorktreeList(out)
	if len(entries) == 0 {
		return gitOutputForPath(path, "rev-parse", "--show-toplevel")
	}
	return entries[0].Path, nil
}

func parseWorktreeList(out string) []WorktreeInfo {
	var entries []WorktreeInfo
	var current *WorktreeInfo
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if current != nil {
				entries = append(entries, *current)
				current = nil
			}
			continue
		}
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		if key == "worktree" {
			if current != nil {
				entries = append(entries, *current)
			}
			current = &WorktreeInfo{Path: filepath.Clean(value)}
			continue
		}
		if current == nil {
			continue
		}
		if key == "branch" {
			current.Branch = strings.TrimPrefix(value, "refs/heads/")
		}
	}
	if current != nil {
		entries = append(entries, *current)
	}
	return entries
}

// MainRepoRoot returns the path to the main repository root.
// If the current directory is inside a linked worktree, it returns
// the main worktree's root. Otherwise it returns the current repo root.
func MainRepoRoot() (string, error) {
	out, err := gitOutput("worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	entries := parseWorktreeList(out)
	if len(entries) == 0 {
		return gitOutput("rev-parse", "--show-toplevel")
	}
	return entries[0].Path, nil
}

func samePath(a, b string) bool {
	aa, errA := filepath.EvalSymlinks(a)
	bb, errB := filepath.EvalSymlinks(b)
	if errA == nil {
		a = aa
	}
	if errB == nil {
		b = bb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func RepoName(path string) string {
	name := filepath.Base(filepath.Clean(path))
	if name == "." || name == string(os.PathSeparator) {
		return "repo"
	}
	return name
}

func WorktreeName(branch, path string) string {
	if branch != "" {
		return branch
	}
	name := filepath.Base(filepath.Clean(path))
	if name == "." || name == string(os.PathSeparator) {
		return "worktree"
	}
	return name
}

func ExitCode(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	if code, convErr := strconv.Atoi(err.Error()); convErr == nil {
		return code
	}
	return 1
}

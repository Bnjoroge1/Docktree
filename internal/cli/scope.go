package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bnjoroge/docktree/internal/config"
	dockgit "github.com/bnjoroge/docktree/internal/git"
	"github.com/bnjoroge/docktree/internal/state"
)

// resolveRepo detects the current git repository and selects the Docktree
// subproject that owns the current directory. Selection walks upward from the
// working directory to the worktree root and picks the nearest docktree.yml;
// configPath (from the global --config flag) overrides that search.
//
// When no docktree.yml exists anywhere up the tree the returned scope is the
// repository root, which is byte-for-byte the pre-subproject behaviour.
func resolveRepo(configPath string) (dockgit.RepoInfo, error) {
	repo, err := dockgit.DetectRepo()
	if err != nil {
		return dockgit.RepoInfo{}, err
	}
	sub, err := resolveSubpath(repo, configPath)
	if err != nil {
		return dockgit.RepoInfo{}, err
	}
	return repo.WithSubpath(sub), nil
}

func resolveSubpath(repo dockgit.RepoInfo, configPath string) (string, error) {
	stop := realPath(repo.WorktreeRoot)
	if configPath != "" {
		return explicitSubpath(stop, configPath)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", nil
	}
	realCwd := realPath(cwd)
	root, ok := config.DiscoverRoot(realCwd, stop)
	if !ok {
		mainStop := realPath(repo.RepoRoot)
		if mainStop != stop {
			if rel, err := filepath.Rel(stop, realCwd); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				mainCwd := filepath.Join(mainStop, rel)
				if mainRoot, ok := config.DiscoverRoot(mainCwd, mainStop); ok {
					return subpathWithin(mainStop, mainRoot)
				}
			}
		}
		return "", nil
	}
	return subpathWithin(stop, root)
}

// explicitSubpath resolves --config to a subproject path. The target must
// exist and live inside the current worktree; anything else is a usage error
// rather than a silent fallback to repository-root scope.
func explicitSubpath(worktreeRoot, configPath string) (string, error) {
	abs := configPath
	if !filepath.IsAbs(abs) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		abs = filepath.Join(cwd, abs)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("--config %s: %w", configPath, err)
	}
	dir := abs
	if !info.IsDir() {
		if filepath.Base(abs) != config.FileName {
			return "", fmt.Errorf("--config %s: not a %s file", configPath, config.FileName)
		}
		dir = filepath.Dir(abs)
	} else if _, err := os.Stat(filepath.Join(dir, config.FileName)); err != nil {
		return "", fmt.Errorf("--config %s: no %s in that directory", configPath, config.FileName)
	}
	sub, err := subpathWithin(worktreeRoot, realPath(dir))
	if err != nil {
		return "", fmt.Errorf("--config %s: %w", configPath, err)
	}
	return sub, nil
}

func subpathWithin(worktreeRoot, dir string) (string, error) {
	rel, err := filepath.Rel(worktreeRoot, dir)
	if err != nil {
		return "", err
	}
	sub := dockgit.NormalizeSubpath(rel)
	if sub == ".." || strings.HasPrefix(sub, "../") {
		return "", fmt.Errorf("%s is outside the current worktree %s", dir, worktreeRoot)
	}
	return sub, nil
}

// realPath resolves symlinks so that macOS /var vs /private/var (and similar)
// never produce two different subpaths for one directory.
func realPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

// instanceConfigRoot is where a recorded instance's docktree.yml lives.
func instanceConfigRoot(inst *state.Instance) string {
	if inst == nil {
		return ""
	}
	return filepath.Join(inst.RepoRoot, filepath.FromSlash(inst.Subpath))
}

// instanceProjectRoot is where a recorded instance's compose files and state
// live.
func instanceProjectRoot(inst *state.Instance) string {
	if inst == nil {
		return ""
	}
	return filepath.Join(inst.WorktreeRoot, filepath.FromSlash(inst.Subpath))
}

func loadInstanceConfig(inst *state.Instance) (*config.Config, error) {
	return config.Load(instanceConfigRoot(inst))
}

// initConfigRoot picks the directory `docktree init` writes into. Unlike
// resolveRepo it does not search for an existing docktree.yml: init creates a
// new project rooted at the current directory (or at --config's directory),
// mirrored into the main checkout so linked worktrees read the same file.
func initConfigRoot(repo dockgit.RepoInfo, configPath string) (string, error) {
	dir := configPath
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		dir = cwd
	} else {
		if !filepath.IsAbs(dir) {
			cwd, err := os.Getwd()
			if err != nil {
				return "", err
			}
			dir = filepath.Join(cwd, dir)
		}
		if filepath.Base(dir) == config.FileName {
			dir = filepath.Dir(dir)
		} else if info, err := os.Stat(dir); err == nil && !info.IsDir() {
			dir = filepath.Dir(dir)
		}
	}
	sub, err := subpathWithin(realPath(repo.WorktreeRoot), realPath(dir))
	if err != nil {
		return "", err
	}
	scoped := repo.WithSubpath(sub)
	if info, err := os.Stat(scoped.ConfigRoot); err != nil || !info.IsDir() {
		return "", fmt.Errorf("cannot write %s: %s does not exist in the main checkout", config.FileName, scoped.ConfigRoot)
	}
	return scoped.ConfigRoot, nil
}

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bnjoroge/docktree/internal/config"
	dockgit "github.com/bnjoroge/docktree/internal/git"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/setup"
	"github.com/bnjoroge/docktree/internal/state"
)

func runPrepare(ctx *Context) (any, int, error) {
	options, err := parseNoArgHelpOptions("prepare", ctx.Args[1:])
	if err != nil {
		return nil, output.ExitUsage, err
	}
	if options.help {
		return prepareHelpDoc(), output.ExitOK, nil
	}

	repo, err := resolveRepo(ctx.ConfigPath)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	cfg, err := loadCanonicalConfigWithWarnings(repo, ctx.Stderr)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if err := setup.Prepare(setup.Options{
		SourceDir: canonicalConfigRoot(repo),
		TargetDir: projectRoot(repo),
		Config:    cfg,
		Stdout:    ctx.Stdout,
		Stderr:    ctx.Stderr,
	}); err != nil {
		return nil, output.ExitConfig, err
	}
	return PrepareResult{
		RepoRoot:     repo.RepoRoot,
		WorktreeRoot: repo.WorktreeRoot,
		Copied:       append([]string(nil), cfg.Setup.Copy...),
		Symlinked:    append([]string(nil), cfg.Setup.Symlink...),
		Ran:          append([]string(nil), cfg.Setup.Run...),
	}, output.ExitOK, nil
}

func runCreate(ctx *Context) (any, int, error) {
	options, err := parseCreateOptions(ctx.Args[1:])
	if err != nil {
		return nil, output.ExitUsage, err
	}
	if options.help {
		return createHelpDoc(), output.ExitOK, nil
	}
	repo, err := resolveRepo(ctx.ConfigPath)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	cfg, err := loadCanonicalConfig(repo)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	worktreeRoot, err := createPreparedWorktree(repo, cfg, options.branch, ctx.Stdout, ctx.Stderr)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	return CreateResult{
		RepoRoot:     repo.RepoRoot,
		WorktreeRoot: worktreeRoot,
		Branch:       options.branch,
		Copied:       append([]string(nil), cfg.Setup.Copy...),
		Symlinked:    append([]string(nil), cfg.Setup.Symlink...),
		Ran:          append([]string(nil), cfg.Setup.Run...),
	}, output.ExitOK, nil
}

// createPreparedWorktree adds a git worktree and runs setup into the selected
// subproject inside it. Worktree placement is always repository-level; only the
// setup source/target pair is subproject-scoped.
func createPreparedWorktree(repo dockgit.RepoInfo, cfg *config.Config, branch string, stdout, stderr io.Writer) (string, error) {
	worktreeRoot, err := worktreePath(repo.RepoRoot, cfg, branch)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(worktreeRoot), 0o755); err != nil {
		return "", err
	}
	cmd := exec.Command("git", "worktree", "add", "-b", branch, worktreeRoot)
	cmd.Dir = repo.RepoRoot
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	if err := setup.Prepare(setup.Options{
		SourceDir: canonicalConfigRoot(repo),
		TargetDir: projectRoot(repo.WithWorktree(worktreeRoot, branch)),
		Config:    cfg,
		Stdout:    stdout,
		Stderr:    stderr,
	}); err != nil {
		return "", err
	}
	return worktreeRoot, nil
}

func ensureCreateComposeInputsCommitted(repo dockgit.RepoInfo, cfg *config.Config, fileOverride string, includeDocktreeConfig bool) error {
	repoRoot := repo.RepoRoot
	project := projectRoot(repo)
	var files []string
	if fileOverride != "" {
		if filepath.IsAbs(fileOverride) {
			files = []string{fileOverride}
		} else {
			files = []string{filepath.Join(project, fileOverride)}
		}
	} else {
		resolved, err := composeFiles(project, cfg)
		if err != nil {
			return err
		}
		files = resolved
	}
	if includeDocktreeConfig {
		files = append(files, filepath.Join(canonicalConfigRoot(repo), config.FileName))
	}

	var issues []string
	for _, file := range files {
		if issue, ok := committedInputIssue(repoRoot, file); ok {
			issues = append(issues, issue)
		}
	}
	if len(issues) == 0 {
		return nil
	}
	return fmt.Errorf("docktree up --create requires referenced compose/config files to be committed before creating a worktree:\n  - %s\ncommit these files, then rerun docktree up --create", strings.Join(issues, "\n  - "))
}

func committedInputIssue(repoRoot, path string) (string, bool) {
	if _, err := os.Stat(path); err != nil {
		return fmt.Sprintf("%s (does not exist)", path), true
	}
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	inHead := exec.Command("git", "cat-file", "-e", "HEAD:"+rel)
	inHead.Dir = repoRoot
	if err := inHead.Run(); err != nil {
		return fmt.Sprintf("%s (not committed in HEAD)", rel), true
	}
	clean := exec.Command("git", "diff", "--quiet", "HEAD", "--", rel)
	clean.Dir = repoRoot
	if err := clean.Run(); err != nil {
		return fmt.Sprintf("%s (has uncommitted changes)", rel), true
	}
	return "", false
}

func removeCreatedWorktree(repoRoot, worktreeRoot, branch string, stderr io.Writer) error {
	var errs []error
	remove := exec.Command("git", "worktree", "remove", "--force", worktreeRoot)
	remove.Dir = repoRoot
	remove.Stderr = stderr
	if err := remove.Run(); err != nil {
		errs = append(errs, err)
	}
	if branch != "" {
		deleteBranch := exec.Command("git", "branch", "-D", branch)
		deleteBranch.Dir = repoRoot
		deleteBranch.Stderr = stderr
		if err := deleteBranch.Run(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func worktreePath(repoRoot string, cfg *config.Config, branch string) (string, error) {
	repoName := dockgit.RepoName(repoRoot)
	branchSlug := slugWorktreeBranch(branch)
	rootTemplate := cfg.Worktrees.Root
	if rootTemplate == "" {
		rootTemplate = config.Defaults().Worktrees.Root
	}
	replacer := strings.NewReplacer(
		"${repo}", repoName,
		"${branch}", branch,
		"${branch_slug}", branchSlug,
	)
	containsBranchVar := strings.Contains(rootTemplate, "${branch}") || strings.Contains(rootTemplate, "${branch_slug}")
	root := replacer.Replace(rootTemplate)
	if !filepath.IsAbs(root) {
		root = filepath.Join(repoRoot, root)
	}
	root = filepath.Clean(root)
	if containsBranchVar {
		return root, nil
	}
	return filepath.Join(root, branchSlug), nil
}

func slugWorktreeBranch(branch string) string {
	branch = strings.ToLower(strings.TrimSpace(branch))
	branch = strings.ReplaceAll(branch, string(filepath.Separator), "-")
	branch = strings.ReplaceAll(branch, "/", "-")
	branch = strings.ReplaceAll(branch, "\\", "-")
	var b strings.Builder
	lastDash := false
	for _, r := range branch {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			if r == '-' {
				if lastDash {
					continue
				}
				lastDash = true
			} else {
				lastDash = false
			}
			b.WriteRune(r)
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	value := strings.Trim(b.String(), "-_")
	if value == "" {
		return "worktree"
	}
	return value
}

// canonicalVolumesShare reads volumes.share from the canonical config root so
// every worktree of a project agrees on which volumes are shared.
func canonicalVolumesShare(repo dockgit.RepoInfo) []string {
	repoCfg, err := loadCanonicalConfig(repo)
	if err != nil {
		return nil
	}
	return repoCfg.Volumes.Share
}

func loadConfigWithSharedWarnings(dir string, stderr io.Writer) (*config.Config, error) {
	cfg, err := config.LoadUnvalidated(dir)
	if err != nil {
		return nil, err
	}
	if err := config.ValidateShared(cfg.Shared, cfg.Volumes.Share); err != nil && stderr != nil {
		fmt.Fprintf(stderr, "warning: %v\n", err)
	}
	return cfg, nil
}

// canonicalConfigRoot returns the directory holding the selected project's
// docktree.yml: the main checkout root joined with the subproject path. Linked
// worktrees therefore read the same config as the main checkout.
func canonicalConfigRoot(repo dockgit.RepoInfo) string {
	if repo.ConfigRoot != "" {
		return repo.ConfigRoot
	}
	return repo.WithSubpath(repo.Subpath).ConfigRoot
}

// projectRoot returns the directory the selected project's compose files,
// state, and setup targets resolve against.
func projectRoot(repo dockgit.RepoInfo) string {
	if repo.ProjectRoot != "" {
		return repo.ProjectRoot
	}
	return repo.WithSubpath(repo.Subpath).ProjectRoot
}

func loadCanonicalConfig(repo dockgit.RepoInfo) (*config.Config, error) {
	return config.Load(canonicalConfigRoot(repo))
}

func loadCanonicalConfigWithWarnings(repo dockgit.RepoInfo, stderr io.Writer) (*config.Config, error) {
	return loadConfigWithSharedWarnings(canonicalConfigRoot(repo), stderr)
}

func loadMergedConfig(repo dockgit.RepoInfo) (*config.Config, error) {
	cfg, err := loadCanonicalConfig(repo)
	if err != nil {
		return nil, err
	}
	local, err := config.LoadLocalOverrides(config.LocalOverridesPath(projectRoot(repo), cfg.State.Directory))
	if err != nil {
		return nil, fmt.Errorf("worktree local overrides: %w", err)
	}
	config.MergeLocalOverrides(cfg, local)
	if err := config.ValidateOverrides(cfg.Overrides, cfg.Shared); err != nil {
		return nil, err
	}
	return cfg, nil
}

// resolveInstanceName returns the authoritative Compose project identity for a
// worktree. Once a worktree has created a Docktree instance, the persisted
// project name is authoritative for the lifetime of that worktree; the
// branch-derived name is only used before any instance state exists. Branch
// checkouts and renames therefore never change the Compose project identity.
func resolveInstanceName(repo dockgit.RepoInfo, cfg *config.Config) (string, error) {
	stateDir := state.StatePath(projectRoot(repo), cfg.State.Directory)
	inst, err := state.LoadInstance(stateDir)
	switch {
	case err == nil && inst != nil:
		if inst.ProjectName != "" {
			return inst.ProjectName, nil
		}
		if inst.Name != "" {
			return inst.Name, nil
		}
		return "", fmt.Errorf("worktree state at %s has no saved project name; refusing to derive a new identity (run `docktree clean` or remove the state directory to start fresh)", stateDir)
	case !errors.Is(err, os.ErrNotExist):
		return "", err
	}
	return dockgit.InstanceName(dockgit.RepoName(repo.RepoRoot), dockgit.WorktreeName(repo.Branch, repo.WorktreeRoot), repo.RepoRoot, repo.WorktreeRoot, repo.Subpath), nil
}

func commonIdentity(ctx *Context) (dockgit.RepoInfo, *config.Config, string, error) {
	repo, err := resolveRepo(ctx.ConfigPath)
	if err != nil {
		return dockgit.RepoInfo{}, nil, "", err
	}
	cfg, err := loadMergedConfig(repo)
	if err != nil {
		return dockgit.RepoInfo{}, nil, "", err
	}
	instance, err := resolveInstanceName(repo, cfg)
	if err != nil {
		return dockgit.RepoInfo{}, nil, "", err
	}
	return repo, cfg, instance, nil
}

// ensureGitignore keeps the worktree's root .gitignore covering the selected
// project's state directory. Subprojects get a subpath-qualified entry so one
// .gitignore serves every project in the worktree.
func ensureGitignore(repo dockgit.RepoInfo, stateDir string) error {
	if filepath.IsAbs(stateDir) {
		return nil
	}
	path := filepath.Join(repo.WorktreeRoot, ".gitignore")
	entry := strings.Trim(filepath.ToSlash(stateDir), "/")
	if repo.Subpath != "" {
		entry = repo.Subpath + "/" + entry
	}
	entry += "/"
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return os.WriteFile(path, []byte(entry+"\n"), 0o644)
	}
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == entry || trimmed == strings.TrimSuffix(entry, "/") {
			return nil
		}
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		if _, err := file.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = file.WriteString(entry + "\n")
	return err
}

func activeComposeFiles(worktreeRoot string, cfg *config.Config, inst *state.Instance) []string {
	// Prefer the files the instance was actually started with (recorded at up
	// time). Falling back to docktree.yml would break down/status whenever the
	// instance was started with `-f` pointing at different files.
	var files []string
	if len(inst.ComposeFiles) > 0 {
		files = append(files, inst.ComposeFiles...)
	} else {
		discovered, err := composeFiles(worktreeRoot, cfg)
		if err != nil {
			return nil
		}
		files = discovered
	}
	stateDir := state.StatePath(worktreeRoot, cfg.State.Directory)
	clear := filepath.Join(stateDir, "generated", inst.ProjectName+".clear.yml")
	if _, err := os.Stat(clear); err == nil {
		files = append(files, clear)
	}
	override := filepath.Join(stateDir, "generated", inst.ProjectName+".override.yml")
	if _, err := os.Stat(override); err == nil {
		files = append(files, override)
	}
	return files
}

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

	repo, err := dockgit.DetectRepo()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	cfg, err := loadCanonicalConfigWithWarnings(repo, ctx.Stderr)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if err := setup.Prepare(setup.Options{
		SourceDir: canonicalConfigRoot(repo),
		TargetDir: repo.WorktreeRoot,
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
	repo, err := dockgit.DetectRepo()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	configRoot := canonicalConfigRoot(repo)
	cfg, err := config.Load(configRoot)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	worktreeRoot, err := createPreparedWorktree(configRoot, cfg, options.branch, ctx.Stdout, ctx.Stderr)
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

func createPreparedWorktree(repoRoot string, cfg *config.Config, branch string, stdout, stderr io.Writer) (string, error) {
	worktreeRoot, err := worktreePath(repoRoot, cfg, branch)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(worktreeRoot), 0o755); err != nil {
		return "", err
	}
	cmd := exec.Command("git", "worktree", "add", "-b", branch, worktreeRoot)
	cmd.Dir = repoRoot
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	if err := setup.Prepare(setup.Options{
		SourceDir: repoRoot,
		TargetDir: worktreeRoot,
		Config:    cfg,
		Stdout:    stdout,
		Stderr:    stderr,
	}); err != nil {
		return "", err
	}
	return worktreeRoot, nil
}

func ensureCreateComposeInputsCommitted(repoRoot, worktreeRoot string, cfg *config.Config, fileOverride string, includeDocktreeConfig bool) error {
	var files []string
	if fileOverride != "" {
		if filepath.IsAbs(fileOverride) {
			files = []string{fileOverride}
		} else {
			files = []string{filepath.Join(worktreeRoot, fileOverride)}
		}
	} else {
		resolved, err := composeFiles(worktreeRoot, cfg)
		if err != nil {
			return err
		}
		files = resolved
	}
	if includeDocktreeConfig {
		files = append(files, filepath.Join(repoRoot, "docktree.yml"))
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

func repoRootVolumesShare() []string {
	mainRoot, err := dockgit.MainRepoRoot()
	if err != nil {
		return nil
	}
	repoCfg, err := config.Load(mainRoot)
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

// canonicalConfigRoot returns the main repo root when running inside a
// linked worktree, or repo.RepoRoot when in the main repo itself. This
// ensures worktree commands and platform read the same docktree.yml.
func canonicalConfigRoot(repo dockgit.RepoInfo) string {
	mainRoot, err := dockgit.MainRepoRootForPath(repo.WorktreeRoot)
	if err == nil && mainRoot != "" {
		return mainRoot
	}
	return repo.RepoRoot
}

func loadCanonicalConfig(repo dockgit.RepoInfo) (*config.Config, error) {
	return config.Load(canonicalConfigRoot(repo))
}

func loadCanonicalConfigWithWarnings(repo dockgit.RepoInfo, stderr io.Writer) (*config.Config, error) {
	return loadConfigWithSharedWarnings(canonicalConfigRoot(repo), stderr)
}

func loadMergedConfig(repo dockgit.RepoInfo, worktreeRoot string) (*config.Config, error) {
	cfg, err := loadCanonicalConfig(repo)
	if err != nil {
		return nil, err
	}
	local, err := config.LoadLocalOverrides(config.LocalOverridesPath(worktreeRoot, cfg.State.Directory))
	if err != nil {
		return nil, fmt.Errorf("worktree local overrides: %w", err)
	}
	config.MergeLocalOverrides(cfg, local)
	if err := config.ValidateOverrides(cfg.Overrides, cfg.Shared); err != nil {
		return nil, err
	}
	return cfg, nil
}

func commonIdentity() (dockgit.RepoInfo, *config.Config, string, error) {
	repo, err := dockgit.DetectRepo()
	if err != nil {
		return dockgit.RepoInfo{}, nil, "", err
	}
	cfg, err := loadMergedConfig(repo, repo.WorktreeRoot)
	if err != nil {
		return dockgit.RepoInfo{}, nil, "", err
	}
	instance := dockgit.InstanceName(dockgit.RepoName(repo.RepoRoot), dockgit.WorktreeName(repo.Branch, repo.WorktreeRoot), repo.RepoRoot, repo.WorktreeRoot)
	return repo, cfg, instance, nil
}

func ensureGitignore(worktreeRoot, stateDir string) error {
	path := filepath.Join(worktreeRoot, ".gitignore")
	entry := strings.Trim(stateDir, "/") + "/"
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

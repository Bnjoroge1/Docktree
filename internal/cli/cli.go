package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bnjoroge/docktree/internal/compose"
	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/docker"
	dockgit "github.com/bnjoroge/docktree/internal/git"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/ports"
	"github.com/bnjoroge/docktree/internal/setup"
	"github.com/bnjoroge/docktree/internal/state"
)

const version = "0.1.0-dev"

type commandFunc func(*Context) (any, int, error)

type Context struct {
	Args     []string
	Renderer *output.Renderer
	Stdout   io.Writer
	Stderr   io.Writer
}

type UpResult struct {
	Instance        *state.Instance    `json:"instance"`
	CreatedWorktree string             `json:"created_worktree,omitempty"`
	ComposeFiles    []string           `json:"compose_files"`
	OverrideFile    string             `json:"override_file"`
	ClearFile       string             `json:"clear_file,omitempty"`
	Ports           []ports.Assignment `json:"ports,omitempty"`
	Services        []string           `json:"services"`
	IsolatedVolumes []string           `json:"isolated_volumes,omitempty"`
	EnvWarnings     []compose.Warning  `json:"env_warnings,omitempty"`
	Scaffolded      bool               `json:"scaffolded,omitempty"`
	Synced          bool               `json:"synced,omitempty"`
	AlreadyRunning  bool               `json:"already_running,omitempty"`
}

type DownResult struct {
	Instance       *state.Instance `json:"instance,omitempty"`
	AlreadyStopped bool            `json:"already_stopped,omitempty"`
}

type StatusResult struct {
	Instance *state.Instance `json:"instance,omitempty"`
	Raw      json.RawMessage `json:"raw,omitempty"`
	Text     string          `json:"text,omitempty"`
	Stopped  bool            `json:"stopped,omitempty"`
}

type PortsResult struct {
	Instance string             `json:"instance"`
	Ports    []ports.Assignment `json:"ports"`
}

type PrepareResult struct {
	RepoRoot     string   `json:"repo_root"`
	WorktreeRoot string   `json:"worktree_root"`
	Copied       []string `json:"copied,omitempty"`
	Symlinked    []string `json:"symlinked,omitempty"`
	Ran          []string `json:"ran,omitempty"`
}

type CreateResult struct {
	RepoRoot     string   `json:"repo_root"`
	WorktreeRoot string   `json:"worktree_root"`
	Branch       string   `json:"branch"`
	Copied       []string `json:"copied,omitempty"`
	Symlinked    []string `json:"symlinked,omitempty"`
	Ran          []string `json:"ran,omitempty"`
}

// helps us track stale/orphaned docktree instances
type CleanItem struct {
	Instance   string `json:"instance"`
	Reason     string `json:"reason"`
	Ports      int    `json:"ports"`
	Containers int    `json:"containers"`
	Networks   int    `json:"networks"`
	Volumes    int    `json:"volumes,omitempty"`
}

type CleanTotals struct {
	Instances  int `json:"instances"`
	Ports      int `json:"ports"`
	Containers int `json:"containers"`
	Networks   int `json:"networks"`
	Volumes    int `json:"volumes,omitempty"`
}

type CleanResult struct {
	DryRun    bool        `json:"dry_run"`
	Removed   bool        `json:"removed"`
	Volumes   bool        `json:"volumes"`
	Instances []CleanItem `json:"instances"`
	Totals    CleanTotals `json:"totals"`
}

func Run(args []string, stdout, stderr io.Writer) int {
	jsonMode, rest := parseGlobalFlags(args)
	renderer := output.New(stdout, jsonMode)
	ctx := &Context{Args: rest, Renderer: renderer, Stdout: stdout, Stderr: stderr}
	if len(rest) == 0 {
		printHelp(stdout)
		return output.ExitOK
	}
	commands := map[string]commandFunc{
		"up":      runUp,
		"create":  runCreate,
		"down":    runDown,
		"status":  runStatus,
		"ports":   runPorts,
		"clean":   runClean,
		"prepare": runPrepare,
	}
	switch rest[0] {
	case "help", "-h", "--help":
		printHelp(stdout)
		return output.ExitOK
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "docktree %s\n", version)
		return output.ExitOK
	}
	fn, ok := commands[rest[0]]
	if !ok {
		fmt.Fprintf(stderr, "unknown command %q\n\n", rest[0])
		printHelp(stderr)
		return output.ExitUsage
	}
	result, code, err := fn(ctx)
	if err != nil {
		target := renderer
		target.Writer = stderr
		target.Error(errorCode(code), err.Error(), nil)
		return code
	}
	if result != nil {
		renderer.Render(result, humanRenderer())
	}
	return code
}

func runUp(ctx *Context) (any, int, error) {
	options, err := parseUpOptions(ctx.Args[1:])
	if err != nil {
		return nil, output.ExitUsage, err
	}
	if options.help {
		printUpHelp(ctx.Stdout)
		return nil, output.ExitOK, nil
	}
	repo, cfg, instanceName, err := commonIdentity()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	var scaffolded bool
	var createdWorktree string
	var synced bool
	if options.create != "" {
		scaffolded, err = config.Scaffold(repo.RepoRoot, cfg)
		if err != nil {
			return nil, output.ExitConfig, err
		}
		if scaffolded {
			cfg, err = config.Load(repo.RepoRoot)
			if err != nil {
				return nil, output.ExitConfig, err
			}
		}
		createdWorktree, err = createPreparedWorktree(repo.RepoRoot, cfg, options.create, ctx.Stdout, ctx.Stderr)
		if err != nil {
			return nil, output.ExitConfig, err
		}
		repo = dockgit.RepoInfo{RepoRoot: repo.RepoRoot, WorktreeRoot: createdWorktree, Branch: options.create}
		instanceName = dockgit.InstanceName(dockgit.RepoName(repo.RepoRoot), dockgit.WorktreeName(repo.Branch, repo.WorktreeRoot), repo.RepoRoot, repo.WorktreeRoot)
	}
	stateDir := state.StatePath(repo.WorktreeRoot, cfg.State.Directory)
	inst, _ := state.LoadInstance(stateDir)
	var envWarnings []compose.Warning
	if inst == nil {
		scaffolded, err = config.Scaffold(repo.RepoRoot, cfg)
		if err != nil {
			return nil, output.ExitConfig, err
		}
		if scaffolded {
			cfg, err = config.Load(repo.RepoRoot)
			if err != nil {
				return nil, output.ExitConfig, err
			}
		}
		envWarnings, err = compose.CheckEnvFile(repo.WorktreeRoot)
		if err != nil {
			return nil, output.ExitConfig, err
		}
		if err := ensureGitignore(repo.WorktreeRoot, cfg.State.Directory); err != nil {
			return nil, output.ExitConfig, err
		}
	}
	if options.sync && options.create == "" {
		if err := setup.Prepare(setup.Options{
			SourceDir: repo.RepoRoot,
			TargetDir: repo.WorktreeRoot,
			Config:    cfg,
			Stdout:    ctx.Stdout,
			Stderr:    ctx.Stderr,
		}); err != nil {
			return nil, output.ExitConfig, err
		}
		synced = true
	}
	var files []string
	if options.file != "" {
		path := options.file
		if !filepath.IsAbs(path) {
			path = filepath.Join(repo.WorktreeRoot, path)
		}
		files = []string{path}
	} else {
		files, err = composeFiles(repo.WorktreeRoot, cfg)
		if err != nil {
			if scaffolded {
				return nil, output.ExitConfig, fmt.Errorf("no compose file found: create docker-compose.yml or set compose.files in docktree.yml")
			}
			return nil, output.ExitConfig, err
		}
	}
	if inst != nil {
		runningState, err := composeRunStateForInstance(inst, cfg)
		if err != nil {
			return nil, output.ExitDocker, err
		}
		if runningState == composeRunRunning {
			currentHash, err := state.HashFiles(files)
			if err != nil {
				return nil, output.ExitConfig, err
			}
			if currentHash == inst.ComposeFileHash {
				return UpResult{Instance: inst, Synced: synced, AlreadyRunning: true}, output.ExitNoop, nil
			}
		}
	}
	project, err := parseAll(files)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	portRange, err := ports.ParseRange(cfg.Ports.Range)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	registry := ports.NewRegistry()
	if err := registry.Lock(); err != nil {
		return nil, output.ExitConflict, err
	}
	locked := true
	defer func() {
		if locked {
			_ = registry.Unlock()
		}
	}()
	if err := state.EnsureStateDir(repo.WorktreeRoot, cfg.State.Directory); err != nil {
		return nil, output.ExitConfig, err
	}
	overrideFile := filepath.Join(stateDir, "generated", instanceName+".override.yml")
	hash, err := state.HashFiles(files)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	now := time.Now().UTC()
	if inst == nil {
		inst = &state.Instance{CreatedAt: now}
	}
	inst.Name = instanceName
	inst.ProjectName = instanceName
	inst.RepoRoot = repo.RepoRoot
	inst.WorktreeRoot = repo.WorktreeRoot
	inst.StateDirectory = stateDir
	inst.Branch = repo.Branch
	inst.LastActiveAt = now
	inst.ComposeFileHash = hash
	dockerStdout := ctx.Stdout
	if ctx.Renderer.JSON {
		dockerStdout = io.Discard
	}
	clearFile := filepath.Join(stateDir, "generated", instanceName+".clear.yml")
	clear := compose.GeneratePortClear(project)
	if clear != nil {
		if err := compose.WriteClearOverride(clear, clearFile); err != nil {
			return nil, output.ExitConfig, err
		}
	}
	var assignments []ports.Assignment
	composeFiles := append([]string{}, files...)
	if clear != nil {
		composeFiles = append(composeFiles, clearFile)
	}
	composeFiles = append(composeFiles, overrideFile)
	cmd := docker.ComposeCommand{ProjectName: instanceName, Files: composeFiles, CommandArgs: []string{"up", "-d"}}
	for attempt := 0; attempt < 10; attempt++ {
		assignments, err = registry.Allocate(instanceName, portRequests(project, cfg.Ports.BindHost), portRange)
		if err != nil {
			return nil, output.ExitConflict, err
		}
		if err := registry.Unlock(); err != nil {
			return nil, output.ExitConflict, err
		}
		locked = false
		override, err := compose.GenerateOverride(project, instanceName, assignments, cfg.Volumes.Share)
		if err != nil {
			return nil, output.ExitConfig, err
		}
		if err := compose.WriteOverride(override, overrideFile); err != nil {
			return nil, output.ExitConfig, err
		}
		if err := docker.Run(cmd, dockerStdout, ctx.Stderr); err != nil {
			if docker.IsPortBindError(err) && attempt < 9 {
				if err := registry.Lock(); err != nil {
					return nil, output.ExitConflict, err
				}
				locked = true
				if releaseErr := registry.Release(instanceName); releaseErr != nil {
					locked = false
					_ = registry.Unlock()
					return nil, output.ExitConflict, releaseErr
				}
				if err := registry.Unlock(); err != nil {
					return nil, output.ExitConflict, err
				}
				locked = false
				continue
			}
			return nil, output.ExitDocker, err
		}
		break
	}
	if locked {
		if err := registry.Unlock(); err != nil {
			return nil, output.ExitConflict, err
		}
		locked = false
	}
	if err := state.SaveInstance(stateDir, inst); err != nil {
		return nil, output.ExitConfig, err
	}
	if err := state.UpsertGlobalInstance("", inst); err != nil {
		return nil, output.ExitConfig, err
	}
	return UpResult{Instance: inst, CreatedWorktree: createdWorktree, ComposeFiles: files, OverrideFile: overrideFile, ClearFile: clearFile, Ports: assignments, Services: serviceNames(project), IsolatedVolumes: isolatedVolumes(project, cfg.Volumes.Share), EnvWarnings: envWarnings, Scaffolded: scaffolded, Synced: synced}, output.ExitOK, nil
}

func runDown(ctx *Context) (any, int, error) {
	repo, err := dockgit.DetectRepo()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	cfg, err := config.Load(repo.RepoRoot)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	stateDir := state.StatePath(repo.WorktreeRoot, cfg.State.Directory)
	inst, err := state.LoadInstance(stateDir)
	if errors.Is(err, os.ErrNotExist) {
		return DownResult{AlreadyStopped: true}, output.ExitNoop, nil
	}
	if err != nil {
		return nil, output.ExitConfig, err
	}
	runningState, err := composeRunStateForInstance(inst, cfg)
	if err != nil {
		return nil, output.ExitDocker, err
	}
	if runningState == composeRunStopped {
		return DownResult{Instance: inst, AlreadyStopped: true}, output.ExitNoop, nil
	}
	if runningState == composeRunUnknown {
		return nil, output.ExitDocker, fmt.Errorf("unable to determine whether %s is running", inst.ProjectName)
	}
	dockerStdout := ctx.Stdout
	if ctx.Renderer.JSON {
		dockerStdout = io.Discard
	}
	cmd := docker.ComposeCommand{ProjectName: inst.ProjectName, Files: activeComposeFiles(repo.WorktreeRoot, cfg, inst), CommandArgs: []string{"down"}}
	if err := docker.Run(cmd, dockerStdout, ctx.Stderr); err != nil {
		return nil, output.ExitDocker, err
	}
	inst.LastActiveAt = time.Now().UTC()
	if err := state.SaveInstance(stateDir, inst); err != nil {
		return nil, output.ExitConfig, err
	}
	if err := state.UpsertGlobalInstance("", inst); err != nil {
		return nil, output.ExitConfig, err
	}
	return DownResult{Instance: inst}, output.ExitOK, nil
}

func runStatus(ctx *Context) (any, int, error) {
	repo, err := dockgit.DetectRepo()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	cfg, err := config.Load(repo.RepoRoot)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	inst, err := state.LoadInstance(state.StatePath(repo.WorktreeRoot, cfg.State.Directory))
	if errors.Is(err, os.ErrNotExist) {
		return StatusResult{Stopped: true}, output.ExitNoop, nil
	}
	if err != nil {
		return nil, output.ExitConfig, err
	}
	out, err := docker.RunCapture(docker.ComposeCommand{ProjectName: inst.ProjectName, Files: activeComposeFiles(repo.WorktreeRoot, cfg, inst), CommandArgs: []string{"ps", "--format", "json"}})
	if err != nil {
		return nil, output.ExitDocker, err
	}
	result := StatusResult{Instance: inst, Text: strings.TrimSpace(out)}
	if json.Valid([]byte(out)) {
		result.Raw = json.RawMessage(out)
	}
	return result, output.ExitOK, nil
}

func runPorts(ctx *Context) (any, int, error) {
	_, _, instanceName, err := commonIdentity()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	all, err := ports.NewRegistry().Load()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	return PortsResult{Instance: instanceName, Ports: all[instanceName]}, output.ExitOK, nil
}

func runClean(ctx *Context) (any, int, error) {
	options, err := parseCleanOptions(ctx.Args[1:])
	if err != nil {
		return nil, output.ExitUsage, err
	}
	portRegistry := ports.NewRegistry()
	if err := portRegistry.Lock(); err != nil {
		return nil, output.ExitDocker, err
	}
	candidates, err := discoverCleanCandidates(portRegistry, options.volumes)
	unlockErr := portRegistry.Unlock()
	if err != nil {
		return nil, output.ExitDocker, err
	}
	if unlockErr != nil {
		return nil, output.ExitDocker, unlockErr
	}
	result := cleanResultFromCandidates(candidates, options.dryRun, options.volumes, false)
	if len(candidates) == 0 {
		return result, output.ExitNoop, nil
	}
	if options.dryRun {
		return result, output.ExitOK, nil
	}
	if !options.yes {
		if !ctx.Renderer.IsTTY {
			return nil, output.ExitUsage, fmt.Errorf("clean requires --yes or --dry-run in non-interactive mode")
		}
		if !confirmClean(ctx.Stdout) {
			return cleanResultFromCandidates(candidates, false, options.volumes, false), output.ExitNoop, nil
		}
	}
	applied, err := applyCleanCandidates(portRegistry, candidates, options.volumes)
	if err != nil {
		return nil, output.ExitDocker, err
	}
	return cleanResultFromCandidates(applied, false, options.volumes, true), output.ExitOK, nil
}

func runPrepare(ctx *Context) (any, int, error) {
	repo, err := dockgit.DetectRepo()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	cfg, err := config.Load(repo.RepoRoot)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if err := setup.Prepare(setup.Options{
		SourceDir: repo.RepoRoot,
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
	repo, err := dockgit.DetectRepo()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	cfg, err := config.Load(repo.RepoRoot)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	worktreeRoot, err := createPreparedWorktree(repo.RepoRoot, cfg, options.branch, ctx.Stdout, ctx.Stderr)
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

func commonIdentity() (dockgit.RepoInfo, *config.Config, string, error) {
	repo, err := dockgit.DetectRepo()
	if err != nil {
		return dockgit.RepoInfo{}, nil, "", err
	}
	cfg, err := config.Load(repo.RepoRoot)
	if err != nil {
		return dockgit.RepoInfo{}, nil, "", err
	}
	instance := dockgit.InstanceName(dockgit.RepoName(repo.RepoRoot), dockgit.WorktreeName(repo.Branch, repo.WorktreeRoot), repo.RepoRoot, repo.WorktreeRoot)
	return repo, cfg, instance, nil
}

func parseGlobalFlags(args []string) (bool, []string) {
	jsonMode := false
	var rest []string
	for _, arg := range args {
		if arg == "--json" {
			jsonMode = true
			continue
		}
		rest = append(rest, arg)
	}
	return jsonMode, rest
}

func composeFiles(dir string, cfg *config.Config) ([]string, error) {
	if len(cfg.Compose.Files) > 0 {
		files := make([]string, 0, len(cfg.Compose.Files))
		for _, file := range cfg.Compose.Files {
			if filepath.IsAbs(file) {
				files = append(files, file)
			} else {
				files = append(files, filepath.Join(dir, file))
			}
		}
		return files, nil
	}
	return compose.FindComposeFiles(dir)
}

func parseAll(files []string) (*compose.ComposeProject, error) {
	return compose.LoadProject(files)
}

func portRequests(project *compose.ComposeProject, bindHost string) []ports.PortRequest {
	var requests []ports.PortRequest
	for service, svc := range project.Services {
		for _, port := range svc.Ports {
			if port.Published == 0 {
				continue
			}
			hostIP := port.HostIP
			if hostIP == "" {
				hostIP = bindHost
			}
			requests = append(requests, ports.PortRequest{Service: service, ContainerPort: port.Target, HostIP: hostIP})
		}
	}
	return requests
}

type composeRunState int

const (
	composeRunUnknown composeRunState = iota
	composeRunStopped
	composeRunRunning
)

func composeRunStateForInstance(inst *state.Instance, cfg *config.Config) (composeRunState, error) {
	out, err := docker.RunCapture(docker.ComposeCommand{ProjectName: inst.ProjectName, Files: activeComposeFiles(inst.WorktreeRoot, cfg, inst), CommandArgs: []string{"ps", "--format", "json"}})
	if err != nil {
		return composeRunUnknown, err
	}
	return parseComposeRunState(out)
}

func parseComposeRunState(out string) (composeRunState, error) {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return composeRunStopped, nil
	}
	var entries []struct {
		State string `json:"State"`
	}
	if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
		return composeRunUnknown, err
	}
	for _, entry := range entries {
		if strings.EqualFold(strings.TrimSpace(entry.State), "running") {
			return composeRunRunning, nil
		}
	}
	return composeRunStopped, nil
}

func activeComposeFiles(worktreeRoot string, cfg *config.Config, inst *state.Instance) []string {
	files, err := composeFiles(worktreeRoot, cfg)
	if err != nil {
		return nil
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

func serviceNames(project *compose.ComposeProject) []string {
	names := make([]string, 0, len(project.Services))
	for name := range project.Services {
		names = append(names, name)
	}
	return names
}

func containerNames(project *compose.ComposeProject) map[string]string {
	names := map[string]string{}
	for name, svc := range project.Services {
		if svc.ContainerName != "" {
			names[name] = svc.ContainerName
		}
	}
	return names
}

func builtImages(project *compose.ComposeProject) []string {
	var images []string
	for name, svc := range project.Services {
		if svc.Build != nil {
			if svc.Image != "" {
				images = append(images, name+"="+svc.Image)
			} else {
				images = append(images, name)
			}
		}
	}
	return images
}

func isolatedVolumes(project *compose.ComposeProject, shareList []string) []string {
	shared := map[string]bool{}
	for _, v := range shareList {
		shared[v] = true
	}
	var isolated []string
	for name, vol := range project.Volumes {
		if vol.External && !shared[name] {
			isolated = append(isolated, name)
		}
	}
	slices.Sort(isolated)
	return isolated
}

func humanRenderer() func(io.Writer, any) {
	return func(w io.Writer, data any) {
		switch v := data.(type) {
		case UpResult:
			if v.Synced && v.AlreadyRunning {
				fmt.Fprintf(w, "Docktree synced setup for %s and it is already running.\n", v.Instance.ProjectName)
				return
			}
			if v.Synced {
				fmt.Fprintf(w, "Docktree synced setup for %s\n", v.Instance.ProjectName)
			}
			if v.AlreadyRunning {
				fmt.Fprintf(w, "Docktree %s is already running.\n", v.Instance.ProjectName)
				return
			}
			if v.CreatedWorktree != "" {
				fmt.Fprintf(w, "Docktree created worktree %s\n", v.CreatedWorktree)
			}
			fmt.Fprintf(w, "Docktree started %s\n", v.Instance.ProjectName)
			for _, assignment := range v.Ports {
				fmt.Fprintf(w, "  %s %s:%d -> %d\n", assignment.Service, assignment.HostIP, assignment.HostPort, assignment.ContainerPort)
			}
			if len(v.IsolatedVolumes) > 0 {
				fmt.Fprintf(w, "  External volumes isolated: %s\n", strings.Join(v.IsolatedVolumes, ", "))
				fmt.Fprintf(w, "  To share a volume across worktrees, add to docktree.yml:\n")
				fmt.Fprintf(w, "    volumes:\n")
				fmt.Fprintf(w, "      share:\n")
				for _, vol := range v.IsolatedVolumes {
					fmt.Fprintf(w, "        - %s\n", vol)
				}
			}
			if v.Scaffolded {
				fmt.Fprintln(w, "  Created docktree.yml with defaults")
			}
			for _, warning := range v.EnvWarnings {
				fmt.Fprintf(w, "  Warning: %s\n", warning.Message)
			}
		case DownResult:
			if v.AlreadyStopped {
				fmt.Fprintln(w, "Docktree is already stopped.")
				return
			}
			fmt.Fprintf(w, "Docktree stopped %s\n", v.Instance.ProjectName)
		case StatusResult:
			if v.Stopped {
				fmt.Fprintln(w, "Docktree is stopped.")
				return
			}
			fmt.Fprintln(w, v.Text)
		case PortsResult:
			fmt.Fprintf(w, "Docktree ports for %s\n", v.Instance)
			for _, assignment := range v.Ports {
				fmt.Fprintf(w, "  %s %s:%d -> %d\n", assignment.Service, assignment.HostIP, assignment.HostPort, assignment.ContainerPort)
			}
		case PrepareResult:
			fmt.Fprintf(w, "Docktree prepared %s\n", v.WorktreeRoot)
		case CreateResult:
			fmt.Fprintf(w, "Docktree created worktree %s for %s\n", v.WorktreeRoot, v.Branch)
		case CleanResult:
			if len(v.Instances) == 0 {
				fmt.Fprintln(w, "Docktree found no stale resources.")
				return
			}
			if v.DryRun {
				fmt.Fprintln(w, "Docktree dry run - nothing will be removed")
			} else if v.Removed {
				fmt.Fprintln(w, "Docktree removed stale resources")
			} else {
				fmt.Fprintln(w, "Docktree found stale resources")
			}
			for _, item := range v.Instances {
				fmt.Fprintf(w, "  %s (%s): %d ports, %d containers, %d networks", item.Instance, item.Reason, item.Ports, item.Containers, item.Networks)
				if v.Volumes {
					fmt.Fprintf(w, ", %d volumes", item.Volumes)
				}
				fmt.Fprintln(w)
			}
		default:
			_ = json.NewEncoder(w).Encode(data)
		}
	}
}

func errorCode(code int) string {
	switch code {
	case output.ExitUsage:
		return "usage"
	case output.ExitConfig:
		return "config"
	case output.ExitDocker:
		return "docker"
	case output.ExitNoop:
		return "noop"
	case output.ExitConflict:
		return "conflict"
	default:
		return "error"
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, `docktree coordinates Docker Compose services across git worktrees.

Usage:
  docktree [--json] <command>

Commands:
  create     Create a worktree and prepare its local Docker setup
  up         Start the current worktree's Compose project (or --create <branch>)
  down       Stop the current worktree's Compose project
  status     Show managed worktree services
  ports      Show allocated host ports
  prepare    Prepare the current worktree's local Docker setup
  clean      Remove stale Docktree-managed resources
  help       Show this help text
  version    Print the docktree version`)
}

type cleanOptions struct {
	dryRun  bool
	yes     bool
	volumes bool
}

type cleanCandidate struct {
	Name       string
	Reason     string
	Ports      int
	Resources  docker.ProjectResources
	Instance   *state.Instance
	StateFound bool
}

type upOptions struct {
	help   bool
	file   string
	create string
	sync   bool
}

type createOptions struct {
	branch string
}

func parseUpOptions(args []string) (upOptions, error) {
	var options upOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			options.help = true
			return options, nil
		case arg == "-f" || arg == "--file":
			if i+1 >= len(args) {
				return upOptions{}, fmt.Errorf("%s requires a value", arg)
			}
			options.file = args[i+1]
			i++
		case strings.HasPrefix(arg, "--file="):
			options.file = strings.TrimPrefix(arg, "--file=")
		case arg == "--create":
			if i+1 >= len(args) {
				return upOptions{}, fmt.Errorf("%s requires a branch name", arg)
			}
			options.create = args[i+1]
			i++
		case strings.HasPrefix(arg, "--create="):
			options.create = strings.TrimPrefix(arg, "--create=")
		case arg == "--sync":
			options.sync = true
		default:
			return upOptions{}, fmt.Errorf("unknown up flag %q", arg)
		}
	}
	return options, nil
}

func printUpHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree up [options]

Start the current worktree's Compose project.

Options:
  -f, --file <path>     Use a specific Compose file
  --create <branch>     Create and prepare a new worktree before starting
  --sync                Run setup copy/symlink/run steps before starting
  -h, --help            Show this help text`)
}

func parseCreateOptions(args []string) (createOptions, error) {
	if len(args) != 1 {
		return createOptions{}, fmt.Errorf("usage: docktree create <branch>")
	}
	if strings.HasPrefix(args[0], "-") {
		return createOptions{}, fmt.Errorf("usage: docktree create <branch>")
	}
	return createOptions{branch: args[0]}, nil
}

func parseCleanOptions(args []string) (cleanOptions, error) {
	var options cleanOptions
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			options.dryRun = true
		case "--yes":
			options.yes = true
		case "--volumes":
			options.volumes = true
		default:
			return cleanOptions{}, fmt.Errorf("unknown clean flag %q", arg)
		}
	}
	return options, nil
}

func discoverCleanCandidates(portRegistry *ports.Registry, includeVolumes bool) ([]cleanCandidate, error) {
	instances, err := state.LoadGlobalInstances("")
	if err != nil {
		return nil, err
	}
	portMap, err := portRegistry.Load()
	if err != nil {
		return nil, err
	}
	managedProjects, err := docker.ListDocktreeProjects()
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for name := range instances {
		names[name] = true
	}
	for name := range portMap {
		names[name] = true
	}
	for _, name := range managedProjects {
		names[name] = true
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	slices.Sort(ordered)
	var candidates []cleanCandidate
	for _, name := range ordered {
		resources, err := docker.ListProjectResources(name, includeVolumes)
		if err != nil {
			return nil, err
		}
		var inst *state.Instance
		stateFound := false
		if saved, ok := instances[name]; ok {
			copied := saved
			inst = &copied
			stateFound = true
		}
		reason := staleReason(inst, stateFound, len(portMap[name]), resources)
		if reason == "" {
			continue
		}
		candidates = append(candidates, cleanCandidate{
			Name:       name,
			Reason:     reason,
			Ports:      len(portMap[name]),
			Resources:  resources,
			Instance:   inst,
			StateFound: stateFound,
		})
	}
	return candidates, nil
}

func staleReason(inst *state.Instance, stateFound bool, portCount int, resources docker.ProjectResources) string {
	resourceCount := len(resources.Containers) + len(resources.Networks) + len(resources.Volumes)
	if stateFound {
		if inst.WorktreeRoot == "" {
			return "missing worktree path"
		}
		if _, err := os.Stat(inst.WorktreeRoot); errors.Is(err, os.ErrNotExist) {
			return "worktree path gone"
		}
		if !inst.LastActiveAt.IsZero() && time.Since(inst.LastActiveAt) > 14*24*time.Hour {
			return fmt.Sprintf("idle %d days", int(time.Since(inst.LastActiveAt).Hours()/24))
		}
		if resourceCount == 0 && portCount > 0 {
			return "stale port allocations"
		}
		return ""
	}
	if resourceCount > 0 && portCount > 0 {
		return "orphaned resources and port allocations"
	}
	if resourceCount > 0 {
		return "orphaned resources"
	}
	if portCount > 0 {
		return "stale port allocations"
	}
	return ""
}

func cleanResultFromCandidates(candidates []cleanCandidate, dryRun, volumes, removed bool) CleanResult {
	result := CleanResult{DryRun: dryRun, Volumes: volumes, Removed: removed}
	for _, candidate := range candidates {
		item := CleanItem{
			Instance:   candidate.Name,
			Reason:     candidate.Reason,
			Ports:      candidate.Ports,
			Containers: len(candidate.Resources.Containers),
			Networks:   len(candidate.Resources.Networks),
			Volumes:    len(candidate.Resources.Volumes),
		}
		result.Instances = append(result.Instances, item)
		result.Totals.Instances++
		result.Totals.Ports += item.Ports
		result.Totals.Containers += item.Containers
		result.Totals.Networks += item.Networks
		result.Totals.Volumes += item.Volumes
	}
	return result
}

func applyCleanCandidates(portRegistry *ports.Registry, candidates []cleanCandidate, includeVolumes bool) ([]cleanCandidate, error) {
	if err := portRegistry.Lock(); err != nil {
		return nil, err
	}
	portMap, err := portRegistry.Load()
	if err != nil {
		_ = portRegistry.Unlock()
		return nil, err
	}
	instances, err := state.LoadGlobalInstances("")
	if err != nil {
		_ = portRegistry.Unlock()
		return nil, err
	}
	var applied []cleanCandidate
	for _, candidate := range candidates {
		currentCandidate := candidate
		currentCandidate.Ports = len(portMap[candidate.Name])
		if saved, ok := instances[candidate.Name]; ok {
			copied := saved
			currentCandidate.Instance = &copied
			currentCandidate.StateFound = true
		} else {
			currentCandidate.Instance = nil
			currentCandidate.StateFound = false
		}
		if staleReason(currentCandidate.Instance, currentCandidate.StateFound, currentCandidate.Ports, currentCandidate.Resources) == "" {
			continue
		}
		if err := portRegistry.Release(candidate.Name); err != nil {
			_ = portRegistry.Unlock()
			return nil, err
		}
		if err := state.RemoveGlobalInstance("", candidate.Name); err != nil {
			_ = portRegistry.Unlock()
			return nil, err
		}
		applied = append(applied, currentCandidate)
	}
	if err := portRegistry.Unlock(); err != nil {
		return nil, err
	}
	for _, candidate := range applied {
		if _, err := docker.RemoveProjectResources(candidate.Name, includeVolumes); err != nil {
			return nil, err
		}
		if candidate.Instance != nil {
			if err := state.RemoveStateDir(candidate.Instance); err != nil {
				return nil, err
			}
		}
	}
	return applied, nil
}

func confirmClean(w io.Writer) bool {
	fmt.Fprint(w, "Remove these stale resources? [y/N] ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes"
}

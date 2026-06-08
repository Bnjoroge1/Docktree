package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	composecli "github.com/compose-spec/compose-go/v2/cli"

	"github.com/bnjoroge/docktree/internal/compose"
	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/docker"
	dockgit "github.com/bnjoroge/docktree/internal/git"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/ports"
	"github.com/bnjoroge/docktree/internal/provision"
	"github.com/bnjoroge/docktree/internal/setup"
	"github.com/bnjoroge/docktree/internal/state"
	"github.com/bnjoroge/docktree/internal/tui"
	"gopkg.in/yaml.v3"
)

const version = "0.1.0-dev"

type commandFunc func(*Context) (any, int, error)

type Context struct {
	Args     []string
	Renderer *output.Renderer
	Stdout   io.Writer
	Stderr   io.Writer
	Steps    *tui.StepPrinter
}

type UpResult struct {
	Instance        *state.Instance    `json:"instance"`
	CreatedWorktree string             `json:"created_worktree,omitempty"`
	ComposeFiles    []string           `json:"compose_files"`
	OverrideFile    string             `json:"override_file"`
	ClearFile       string             `json:"clear_file,omitempty"`
	Ports           []ports.Assignment `json:"ports,omitempty"`
	Services        []string           `json:"services"`
	SharedServices  []string           `json:"shared_services,omitempty"`
	IsolatedVolumes []string           `json:"isolated_volumes,omitempty"`
	EnvWarnings     []compose.Warning  `json:"env_warnings,omitempty"`
	Scaffolded      bool               `json:"scaffolded,omitempty"`
	Synced          bool               `json:"synced,omitempty"`
	AlreadyRunning  bool               `json:"already_running,omitempty"`
}

type ValidateResult struct {
	Valid           bool               `json:"valid"`
	Services        []string           `json:"services"`
	Ports           []ports.Assignment `json:"ports,omitempty"`
	IsolatedVolumes []string           `json:"isolated_volumes,omitempty"`
	EnvWarnings     []compose.Warning  `json:"env_warnings,omitempty"`
	Errors          []string           `json:"errors,omitempty"`
}

type DryRunResult struct {
	DryRun          bool               `json:"dry_run"`
	InstanceName    string             `json:"instance_name"`
	ComposeFiles    []string           `json:"compose_files"`
	Services        []string           `json:"services"`
	Ports           []ports.Assignment `json:"ports,omitempty"`
	IsolatedVolumes []string           `json:"isolated_volumes,omitempty"`
	EnvWarnings     []compose.Warning  `json:"env_warnings,omitempty"`
	OverridePreview string             `json:"override_preview,omitempty"`
	ClearPreview    string             `json:"clear_preview,omitempty"`
}

type DownResult struct {
	Instance       *state.Instance `json:"instance,omitempty"`
	AlreadyStopped bool            `json:"already_stopped,omitempty"`
	DryRun         bool            `json:"dry_run,omitempty"`
	Services       []string        `json:"services,omitempty"`
	ComposeFiles   []string        `json:"compose_files,omitempty"`
	DroppedTenants []string        `json:"dropped_tenants,omitempty"`
}

type StopResult struct {
	Instance       *state.Instance `json:"instance,omitempty"`
	AlreadyStopped bool            `json:"already_stopped,omitempty"`
	DryRun         bool            `json:"dry_run,omitempty"`
	Services       []string        `json:"services,omitempty"`
	ComposeFiles   []string        `json:"compose_files,omitempty"`
}

type ComposePassthroughResult struct {
	Project      string   `json:"project"`
	ComposeFiles []string `json:"compose_files"`
	Subcommand   string   `json:"subcommand"`
	Args         []string `json:"args,omitempty"`
}

type StatusResult struct {
	Instance *state.Instance `json:"instance,omitempty"`
	Raw      json.RawMessage `json:"raw,omitempty"`
	Text     string          `json:"text,omitempty"`
	Stopped  bool            `json:"stopped,omitempty"`
}

type PortsResult struct {
	Instance string       `json:"instance,omitempty"`
	All      bool         `json:"all,omitempty"`
	Entries  []PortsEntry `json:"entries,omitempty"`
}

type PortsEntry struct {
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

type composePsEntry struct {
	Service    string               `json:"Service"`
	Name       string               `json:"Name"`
	State      string               `json:"State"`
	Status     string               `json:"Status"`
	Health     string               `json:"Health"`
	Image      string               `json:"Image"`
	Publishers []composePsPublisher `json:"Publishers"`
}

type composePsPublisher struct {
	URL           string `json:"URL"`
	TargetPort    int    `json:"TargetPort"`
	PublishedPort int    `json:"PublishedPort"`
	Protocol      string `json:"Protocol"`
}

func Run(args []string, stdout, stderr io.Writer) int {
	jsonMode, rest := parseGlobalFlags(args)
	renderer := output.New(stdout, jsonMode)
	ctx := &Context{Args: rest, Renderer: renderer, Stdout: stdout, Stderr: stderr}
	if len(rest) == 0 {
		printHelp(stdout)
		return output.ExitOK
	}
	var result any
	var code int
	var err error

	switch rest[0] {
	case "help", "-h", "--help":
		printHelp(stdout)
		return output.ExitOK
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "%s\n", tui.MutedS("docktree "+version))
		return output.ExitOK
	case "up":
		result, code, err = runWithProgress(ctx, runUp)
	case "down":
		result, code, err = runWithProgress(ctx, runDown)
	default:
		commands := map[string]commandFunc{
			"stop":     runStop,
			"logs":     runLogs,
			"exec":     runExec,
			"run":      runComposeRun,
			"status":   runStatus,
			"ports":    runPorts,
			"clean":    runClean,
			"create":   runCreate,
			"prepare":  runPrepare,
			"platform": runPlatform,
		}
		fn, ok := commands[rest[0]]
		if !ok {
			fmt.Fprintf(stderr, "unknown command %q\n\n", rest[0])
			printHelp(stderr)
			return output.ExitUsage
		}
		result, code, err = fn(ctx)
	}
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

// runWithProgress runs fn with step-by-step progress on stderr.
// Docker stdout/stderr is captured to avoid interleaving.
// Progress steps are cleared before the final result is rendered.
func runWithProgress(ctx *Context, fn commandFunc) (any, int, error) {
	isTTY := ctx.Renderer.IsTTY && !ctx.Renderer.JSON
	if !isTTY {
		return fn(ctx)
	}
	steps := tui.NewStepPrinter(os.Stderr, true)
	ctx.Steps = steps

	var stderrBuf bytes.Buffer
	oldStdout := ctx.Stdout
	oldStderr := ctx.Stderr
	ctx.Stdout = io.Discard
	ctx.Stderr = &stderrBuf
	defer func() {
		ctx.Stdout = oldStdout
		ctx.Stderr = oldStderr
		ctx.Steps = nil
	}()

	result, code, runErr := fn(ctx)

	if runErr != nil {
		steps.Clear()
		io.Copy(oldStderr, &stderrBuf)
	} else {
		steps.Clear()
	}
	return result, code, runErr
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
	if options.validate && options.dryRun {
		return nil, output.ExitUsage, fmt.Errorf("--validate and --dry-run are mutually exclusive")
	}
	repo, cfg, instanceName, err := commonIdentity()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	steps := ctx.Steps
	if steps != nil {
		steps.Header("Starting services…", instanceName)
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
			if steps != nil {
				steps.Done("Scaffolded docktree.yml")
			}
		}
		createdWorktree, err = createPreparedWorktree(repo.RepoRoot, cfg, options.create, ctx.Stdout, ctx.Stderr)
		if err != nil {
			return nil, output.ExitConfig, err
		}
		if steps != nil {
			steps.Done("Created worktree " + tui.AccentS(options.create))
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
			if steps != nil {
				steps.Done("Scaffolded docktree.yml")
			}
		}
		envWarnings, err = compose.CheckEnvFile(repo.WorktreeRoot)
		if err != nil {
			return nil, output.ExitConfig, err
		}
		if steps != nil {
			steps.Done("Checked .env conflicts")
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
		if steps != nil {
			steps.Done("Synced setup files")
		}
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
			return nil, output.ExitConfig, err
		}
	}
	// Shared-services mode: synthesize a worktree-specific compose file that
	// omits platform services, joins the platform network, and rewrites
	// declared url_envs to point at the per-worktree tenant database.
	// Synthesis always runs so validate/dry-run see the correct service set.
	// Platform startup is deferred until after those read-only paths.
	if len(cfg.Shared.Services) > 0 {
		if err := state.EnsureStateDir(repo.WorktreeRoot, cfg.State.Directory); err != nil {
			return nil, output.ExitConfig, err
		}
		rawProj, _, lerr := compose.LoadFull(files)
		if lerr != nil {
			return nil, output.ExitConfig, lerr
		}
		mainRoot, err := dockgit.MainRepoRootForPath(repo.RepoRoot)
		if err != nil {
			return nil, output.ExitConfig, err
		}
		repoSlug := dockgit.RepoName(mainRoot)
		// Build per-tenant database names so SynthesizeWorktree can rewrite
		// declared URL envs (e.g. DATABASE_URL, DB_CONNECTION_URI) in place.
		tenantDBs := make(map[string]map[string]string, len(cfg.Shared.Services))
		for svcName, svc := range cfg.Shared.Services {
			if svc.Tenancy != "per_database" {
				continue
			}
			logicalDBs := make(map[string]string, len(svc.DatabaseTargets()))
			for logicalName := range svc.DatabaseTargets() {
				logicalDBs[logicalName] = provision.TenantNameForDatabase(repoSlug, instanceName, logicalName)
			}
			tenantDBs[svcName] = logicalDBs
		}
		// Warn when a per_database service  has no declared url_envs — tenant DB exists but no worktree service will
		// have its connection string rewritten automatically.
		for svcName, svc := range cfg.Shared.Services {
			if svc.Tenancy != "per_database" || len(svc.Databases) > 0 || len(svc.URLEnvs) > 0 {
				continue
			}
			referenced := false
			for _, wtSvc := range rawProj.Services {
				for _, v := range wtSvc.Environment {
					if v != nil && strings.Contains(*v, svcName) {
						referenced = true
						break
					}
				}
				if referenced {
					break
				}
			}
			if referenced {
				envWarnings = append(envWarnings, compose.Warning{
					Key:     "shared." + svcName + ".url_envs",
					Message: "service " + svcName + " uses tenancy: per_database but url_envs is not declared. DATABASE_URL will NOT be rewritten — all worktrees will hit the same database. Add url_envs: [DATABASE_URL] (or your connection env name) to docktree.yml to fix isolation.",
				})
			}
		}
		wtProj, serr := compose.SynthesizeWorktree(rawProj, cfg.Shared, repoSlug,
			compose.SynthesizeWorktreeOptions{TenantDBs: tenantDBs})
		if serr != nil {
			return nil, output.ExitConfig, serr
		}
		wtComposePath := filepath.Join(stateDir, "generated", instanceName+"-worktree-compose.yml")
		if werr := compose.WriteComposeFile(wtProj, wtComposePath); werr != nil {
			return nil, output.ExitConfig, werr
		}
		files = []string{wtComposePath}
		if steps != nil {
			steps.Done("Generated worktree compose")
		}
	}
	project, err := parseAll(files)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	// validate and dry-run are read-only previews: handle them before the
	// already-running short-circuit so they work whether or not the stack is up.
	if options.validate {
		return runValidate(project, files, cfg, repo, envWarnings)
	}
	if options.dryRun {
		return runDryRun(project, files, cfg, repo, instanceName, envWarnings)
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
				all, _ := ports.NewRegistry().Load()
				return UpResult{Instance: inst, Synced: synced, AlreadyRunning: true, Ports: all[instanceName]}, output.ExitNoop, nil
			}
		}
	}
	// Platform must be up before we start worktree containers.
	if len(cfg.Shared.Services) > 0 {
		mainRoot, err := dockgit.MainRepoRootForPath(repo.RepoRoot)
		if err != nil {
			return nil, output.ExitConfig, err
		}
		if _, _, platErr := ensurePlatformUp(ctx, instanceName, dockgit.RepoName(mainRoot)); platErr != nil {
			return nil, output.ExitDocker, platErr
		}
		if steps != nil {
			steps.Done("Platform stack ready")
		}
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
	inst.ComposeFiles = absComposeFiles(files)
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
		if steps != nil && attempt == 0 {
			steps.Done("Allocated ports")
			for _, a := range assignments {
				if a.HostPort == 0 {
					steps.Sub(fmt.Sprintf("%-12s%s", a.Service, tui.DimS("(no host port)")))
				} else {
					steps.Sub(fmt.Sprintf("%-12s%s %s %s",
						a.Service,
						tui.DimS(fmt.Sprintf("%d", a.ContainerPort)),
						tui.DimS("→"),
						tui.AccentS(fmt.Sprintf("%d", a.HostPort))))
				}
			}
		}
		if err := registry.Unlock(); err != nil {
			return nil, output.ExitConflict, err
		}
		locked = false
		override, err := compose.GenerateOverride(project, instanceName, assignments, repoRootVolumesShare())
		if err != nil {
			return nil, output.ExitConfig, err
		}
		if err := compose.WriteOverride(override, overrideFile); err != nil {
			return nil, output.ExitConfig, err
		}
		if steps != nil && attempt == 0 {
			steps.Done("Generated override")
		}
		var spin *tui.SpinStep
		if steps != nil {
			spin = steps.StartSpin("docker compose up -d")
		}
		runErr := docker.Run(cmd, dockerStdout, ctx.Stderr)
		if spin != nil {
			spin.Stop()
		}
		if runErr != nil {
			if docker.IsPortBindError(runErr) && attempt < 9 {
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
			return nil, output.ExitDocker, runErr
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
	sharedSvcNames := make([]string, 0, len(cfg.Shared.Services))
	for name := range cfg.Shared.Services {
		sharedSvcNames = append(sharedSvcNames, name)
	}
	sort.Strings(sharedSvcNames)
	return UpResult{Instance: inst, CreatedWorktree: createdWorktree, ComposeFiles: files, OverrideFile: overrideFile, ClearFile: clearFile, Ports: assignments, Services: serviceNames(project), SharedServices: sharedSvcNames, IsolatedVolumes: isolatedVolumes(project, repoRootVolumesShare()), EnvWarnings: envWarnings, Scaffolded: scaffolded, Synced: synced}, output.ExitOK, nil
}

func runDown(ctx *Context) (any, int, error) {
	options, err := parseDownOptions(ctx.Args[1:])
	if err != nil {
		return nil, output.ExitUsage, err
	}
	if options.help {
		printDownHelp(ctx.Stdout)
		return nil, output.ExitOK, nil
	}
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
	composeFiles := activeComposeFiles(repo.WorktreeRoot, cfg, inst)
	if options.dryRun {
		services := options.services
		if len(services) == 0 {
			project, err := parseAll(composeFiles)
			if err != nil {
				return nil, output.ExitConfig, err
			}
			services = serviceNames(project)
		}
		return DownResult{
			Instance:     inst,
			DryRun:       true,
			Services:     services,
			ComposeFiles: composeFiles,
		}, output.ExitOK, nil
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
	steps := ctx.Steps
	if steps != nil {
		steps.Header("Stopping services…", inst.ProjectName)
	}
	dockerStdout := ctx.Stdout
	if ctx.Renderer.JSON {
		dockerStdout = io.Discard
	}
	// Drop tenant databases before stopping containers — containers must still
	// be running for psql to reach postgres.
	var droppedTenants []string
	if options.volumes && len(cfg.Shared.Services) > 0 {
		plan, planErr := buildPlatformPlan()
		if planErr != nil {
			return nil, output.ExitConfig, planErr
		}
		for _, binding := range tenantBindingsForInstance(plan, inst) {
			fmt.Fprintf(ctx.Stderr, "Dropping tenant database: %s\n", binding.TenantDB)
			if err := provision.Deprovision(binding.Config); err != nil {
				fmt.Fprintf(ctx.Stderr, "warning: failed to drop %s: %v\n", binding.TenantDB, err)
			} else {
				droppedTenants = append(droppedTenants, binding.TenantDB)
			}
		}
	}
	downArgs := []string{"down"}
	if len(options.services) > 0 {
		downArgs = append(downArgs, options.services...)
	}
	cmd := docker.ComposeCommand{ProjectName: inst.ProjectName, Files: composeFiles, CommandArgs: downArgs}
	var spin *tui.SpinStep
	if steps != nil {
		spin = steps.StartSpin("docker compose down")
	}
	if err := docker.Run(cmd, dockerStdout, ctx.Stderr); err != nil {
		if spin != nil {
			spin.Stop()
		}
		return nil, output.ExitDocker, err
	}
	if spin != nil {
		spin.Stop()
	}
	inst.LastActiveAt = time.Now().UTC()
	if err := state.SaveInstance(stateDir, inst); err != nil {
		return nil, output.ExitConfig, err
	}
	if err := state.UpsertGlobalInstance("", inst); err != nil {
		return nil, output.ExitConfig, err
	}
	services := options.services
	if len(services) == 0 {
		services = []string{"all"}
	}
	return DownResult{Instance: inst, Services: services, DroppedTenants: droppedTenants}, output.ExitOK, nil
}

func runStop(ctx *Context) (any, int, error) {
	options, err := parseStopOptions(ctx.Args[1:])
	if err != nil {
		return nil, output.ExitUsage, err
	}
	if options.help {
		printStopHelp(ctx.Stdout)
		return nil, output.ExitOK, nil
	}
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
		return StopResult{AlreadyStopped: true}, output.ExitNoop, nil
	}
	if err != nil {
		return nil, output.ExitConfig, err
	}
	composeFiles := activeComposeFiles(repo.WorktreeRoot, cfg, inst)
	if options.dryRun {
		services := options.services
		if len(services) == 0 {
			project, err := parseAll(composeFiles)
			if err != nil {
				return nil, output.ExitConfig, err
			}
			services = serviceNames(project)
		}
		return StopResult{
			Instance:     inst,
			DryRun:       true,
			Services:     services,
			ComposeFiles: composeFiles,
		}, output.ExitOK, nil
	}
	runningState, err := composeRunStateForInstance(inst, cfg)
	if err != nil {
		return nil, output.ExitDocker, err
	}
	if runningState == composeRunStopped {
		return StopResult{Instance: inst, AlreadyStopped: true}, output.ExitNoop, nil
	}
	if runningState == composeRunUnknown {
		return nil, output.ExitDocker, fmt.Errorf("unable to determine whether %s is running", inst.ProjectName)
	}
	stopArgs := []string{"stop"}
	if len(options.services) > 0 {
		stopArgs = append(stopArgs, options.services...)
	}
	dockerStdout := ctx.Stdout
	if ctx.Renderer.JSON {
		dockerStdout = io.Discard
	}
	cmd := docker.ComposeCommand{ProjectName: inst.ProjectName, Files: composeFiles, CommandArgs: stopArgs}
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
	services := options.services
	if len(services) == 0 {
		services = []string{"all"}
	}
	return StopResult{Instance: inst, Services: services}, output.ExitOK, nil
}

func runComposePassthrough(ctx *Context, subcommand string, args []string, helpFn func(io.Writer)) (any, int, error) {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "-h" || args[0] == "--help")) {
		helpFn(ctx.Stdout)
		return nil, output.ExitOK, nil
	}
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
	if err != nil {
		return nil, output.ExitConfig, err
	}
	composeFiles := activeComposeFiles(repo.WorktreeRoot, cfg, inst)
	composeArgs := append([]string{subcommand}, args...)
	cmd := docker.ComposeCommand{
		ProjectName: inst.ProjectName,
		Files:       composeFiles,
		CommandArgs: composeArgs,
	}
	if err := docker.Run(cmd, ctx.Stdout, ctx.Stderr); err != nil {
		return ComposePassthroughResult{
			Project:      inst.ProjectName,
			ComposeFiles: composeFiles,
			Subcommand:   subcommand,
			Args:         args,
		}, output.ExitDocker, err
	}
	return ComposePassthroughResult{
		Project:      inst.ProjectName,
		ComposeFiles: composeFiles,
		Subcommand:   subcommand,
		Args:         args,
	}, output.ExitOK, nil
}

func runLogs(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "logs", ctx.Args[1:], printLogsHelp)
}

func runExec(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "exec", ctx.Args[1:], printExecHelp)
}

func runComposeRun(ctx *Context) (any, int, error) {
	args := ctx.Args[1:]
	if len(args) == 0 || (len(args) == 1 && (args[0] == "-h" || args[0] == "--help")) {
		printRunHelp(ctx.Stdout)
		return nil, output.ExitOK, nil
	}
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
	if err != nil {
		return nil, output.ExitConfig, err
	}
	composeFiles := activeComposeFiles(repo.WorktreeRoot, cfg, inst)
	composeArgs := append([]string{"run", "--rm"}, args...)
	cmd := docker.ComposeCommand{
		ProjectName: inst.ProjectName,
		Files:       composeFiles,
		CommandArgs: composeArgs,
	}
	if err := docker.Run(cmd, ctx.Stdout, ctx.Stderr); err != nil {
		return ComposePassthroughResult{
			Project:      inst.ProjectName,
			ComposeFiles: composeFiles,
			Subcommand:   "run",
			Args:         args,
		}, output.ExitDocker, err
	}
	return ComposePassthroughResult{
		Project:      inst.ProjectName,
		ComposeFiles: composeFiles,
		Subcommand:   "run",
		Args:         args,
	}, output.ExitOK, nil
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
	options, err := parsePortsOptions(ctx.Args[1:])
	if err != nil {
		return nil, output.ExitUsage, err
	}
	if options.help {
		printPortsHelp(ctx.Stdout)
		return nil, output.ExitOK, nil
	}
	if options.all {
		all, err := ports.NewRegistry().Load()
		if err != nil {
			return nil, output.ExitConfig, err
		}
		instanceOrder := sortedKeys(all)
		entries := make([]PortsEntry, 0, len(instanceOrder))
		for _, name := range instanceOrder {
			entries = append(entries, PortsEntry{Instance: name, Ports: all[name]})
		}
		return PortsResult{All: true, Entries: entries}, output.ExitOK, nil
	}
	_, _, instanceName, err := commonIdentity()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	all, err := ports.NewRegistry().Load()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	return PortsResult{Instance: instanceName, Entries: []PortsEntry{{Instance: instanceName, Ports: all[instanceName]}}}, output.ExitOK, nil
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
	var applied []cleanCandidate
	if !ctx.Renderer.IsTTY || ctx.Renderer.JSON {
		applied, err = applyCleanCandidates(portRegistry, candidates, options.volumes)
	} else {
		done := make(chan struct{})
		go func() {
			applied, err = applyCleanCandidates(portRegistry, candidates, options.volumes)
			close(done)
		}()
		spinner := &tui.SimpleSpinner{}
		spinner.Start("Removing stale resources…")
		<-done
		spinner.Stop()
	}
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

func runValidate(project *compose.ComposeProject, files []string, cfg *config.Config, repo dockgit.RepoInfo, envWarnings []compose.Warning) (any, int, error) {
	var errs []string
	if len(project.Services) == 0 {
		errs = append(errs, "no services defined in compose file")
	}
	for name, svc := range project.Services {
		if svc.Build != nil && svc.Build.Context != "" {
			// compose-go resolves build.context to an absolute path; only join
			// with the worktree root when it's still relative.
			ctxPath := svc.Build.Context
			if !filepath.IsAbs(ctxPath) {
				ctxPath = filepath.Join(repo.WorktreeRoot, ctxPath)
			}
			if _, err := os.Stat(ctxPath); err != nil {
				errs = append(errs, fmt.Sprintf("service %q: build context %q not found", name, svc.Build.Context))
			}
		}
	}
	instanceName := dockgit.InstanceName(dockgit.RepoName(repo.RepoRoot), dockgit.WorktreeName(repo.Branch, repo.WorktreeRoot), repo.RepoRoot, repo.WorktreeRoot)
	portRange, err := ports.ParseRange(cfg.Ports.Range)
	if err != nil {
		errs = append(errs, fmt.Sprintf("invalid port range %q: %v", cfg.Ports.Range, err))
		return ValidateResult{Valid: false, Services: serviceNames(project), Errors: errs}, output.ExitOK, nil
	}
	registry := ports.NewRegistry()
	if err := registry.Lock(); err != nil {
		errs = append(errs, fmt.Sprintf("cannot acquire port registry lock: %v", err))
		return ValidateResult{Valid: false, Services: serviceNames(project), Errors: errs}, output.ExitOK, nil
	}
	existing, _ := registry.Load()
	hadAllocation := len(existing[instanceName]) > 0
	assignments, allocErr := registry.Allocate(instanceName, portRequests(project, cfg.Ports.BindHost), portRange)
	if allocErr != nil {
		errs = append(errs, fmt.Sprintf("port allocation failed: %v", allocErr))
	}
	// validate is a read-only preview: don't leave a reservation behind for an
	// instance that wasn't already allocated.
	if !hadAllocation && allocErr == nil {
		_ = registry.Release(instanceName)
	}
	_ = registry.Unlock()
	isolated := isolatedVolumes(project, repoRootVolumesShare())
	_, overrideErr := compose.GenerateOverride(project, instanceName, assignments, repoRootVolumesShare())
	if overrideErr != nil {
		errs = append(errs, fmt.Sprintf("override generation failed: %v", overrideErr))
	}
	clear := compose.GeneratePortClear(project)
	if clear == nil && len(portRequests(project, cfg.Ports.BindHost)) > 0 {
		errs = append(errs, "port clear generation returned nil despite having published ports")
	}
	return ValidateResult{
		Valid:           len(errs) == 0,
		Services:        serviceNames(project),
		Ports:           assignments,
		IsolatedVolumes: isolated,
		EnvWarnings:     envWarnings,
		Errors:          errs,
	}, output.ExitOK, nil
}

func runDryRun(project *compose.ComposeProject, files []string, cfg *config.Config, repo dockgit.RepoInfo, instanceName string, envWarnings []compose.Warning) (any, int, error) {
	portRange, err := ports.ParseRange(cfg.Ports.Range)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	registry := ports.NewRegistry()
	if err := registry.Lock(); err != nil {
		return nil, output.ExitConflict, fmt.Errorf("cannot acquire port registry lock: %v", err)
	}
	existing, _ := registry.Load()
	hadAllocation := len(existing[instanceName]) > 0
	assignments, err := registry.Allocate(instanceName, portRequests(project, cfg.Ports.BindHost), portRange)
	if err != nil {
		_ = registry.Unlock()
		return nil, output.ExitConflict, err
	}
	// Dry-run must not disturb the registry. Only undo an allocation the
	// dry-run itself created; never release a running instance's reservations.
	if !hadAllocation {
		if err := registry.Release(instanceName); err != nil {
			_ = registry.Unlock()
			return nil, output.ExitConflict, err
		}
	}
	_ = registry.Unlock()
	override, err := compose.GenerateOverride(project, instanceName, assignments, repoRootVolumesShare())
	if err != nil {
		return nil, output.ExitConfig, err
	}
	overrideYAML, err := yaml.Marshal(override)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	var clearPreview string
	clear := compose.GeneratePortClear(project)
	if clear != nil {
		clearYAML, err := yaml.Marshal(clear)
		if err != nil {
			return nil, output.ExitConfig, err
		}
		clearPreview = string(clearYAML)
	}
	return DryRunResult{
		DryRun:          true,
		InstanceName:    instanceName,
		ComposeFiles:    files,
		Services:        serviceNames(project),
		Ports:           assignments,
		IsolatedVolumes: isolatedVolumes(project, repoRootVolumesShare()),
		EnvWarnings:     envWarnings,
		OverridePreview: string(overrideYAML),
		ClearPreview:    clearPreview,
	}, output.ExitOK, nil
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

	// Pre-resolve COMPOSE_FILE relative entries against dir (not cwd).
	var configs []string
	if raw := strings.TrimSpace(os.Getenv("COMPOSE_FILE")); raw != "" {
		sep := string(os.PathListSeparator)
		if strings.Contains(raw, ";") && !strings.Contains(raw, sep) {
			sep = ";"
		}
		for _, entry := range strings.Split(raw, sep) {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			if !filepath.IsAbs(entry) {
				entry = filepath.Join(dir, entry)
			}
			configs = append(configs, entry)
		}
	}

	opts, err := composecli.NewProjectOptions(configs,
		composecli.WithWorkingDirectory(dir),
		composecli.WithDefaultConfigPath,
	)
	if err != nil {
		return nil, err
	}

	// compose-go walks up directories; only accept files under dir.
	cleanDir := filepath.Clean(dir) + string(filepath.Separator)
	var found []string
	for _, p := range opts.ConfigPaths {
		if strings.HasPrefix(filepath.Clean(p), cleanDir) || p == filepath.Clean(dir) {
			found = append(found, p)
		}
	}

	if len(found) == 0 {
		return nil, fmt.Errorf("no compose file found in %s\n\nCreate docker-compose.yml or compose.yml, or set compose.files in docktree.yml", dir)
	}
	return found, nil
}

// absComposeFiles resolves compose file paths to absolute form so they remain
// valid when read back from state regardless of the caller's working directory.
func absComposeFiles(files []string) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		if abs, err := filepath.Abs(f); err == nil {
			out = append(out, abs)
		} else {
			out = append(out, f)
		}
	}
	return out
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
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
			return composeRunUnknown, err
		}
	} else {
		for _, line := range strings.Split(trimmed, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var entry struct {
				State string `json:"State"`
			}
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				return composeRunUnknown, err
			}
			entries = append(entries, entry)
		}
	}
	for _, entry := range entries {
		if strings.EqualFold(strings.TrimSpace(entry.State), "running") {
			return composeRunRunning, nil
		}
	}
	return composeRunStopped, nil
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

func shortenImage(image string) string {
	parts := strings.Split(image, "/")
	img := parts[len(parts)-1]
	if idx := strings.LastIndex(img, ":"); idx != -1 {
		tag := img[idx+1:]
		if tag == "latest" {
			img = img[:idx]
		}
	}
	return img
}

// renderPortList draws the allocated ports as a bordered table:
// SERVICE | PORT | URL  (internal services show "(internal)" in the URL column).
func renderPortList(w io.Writer, portAssignments []ports.Assignment) {
	var tbl tui.Table
	tbl.Headers = []string{"SERVICE", "PORT", "URL"}
	for _, a := range portAssignments {
		if a.HostPort == 0 {
			tbl.Rows = append(tbl.Rows, []string{a.Service, "—", "(internal)"})
		} else {
			url := fmt.Sprintf("http://%s:%d", a.HostIP, a.HostPort)
			tbl.Rows = append(tbl.Rows, []string{a.Service, fmt.Sprintf("%d", a.HostPort), url})
		}
	}
	fmt.Fprintln(w, tbl.RenderBorderedStyled(func(row, col int, val string) string {
		if row == -1 {
			return tui.DimS(val)
		}
		switch col {
		case 0:
			return tui.OKS(val)
		case 1:
			if val == "—" {
				return tui.DimS(val)
			}
			return tui.AccentS(val)
		case 2:
			if val == "(internal)" {
				return tui.DimS(val)
			}
			return tui.URLS(val)
		}
		return val
	}))
}

func humanRenderer() func(io.Writer, any) {
	return func(w io.Writer, data any) {
		switch v := data.(type) {
		case UpResult:
			projectName := v.Instance.ProjectName
			if v.AlreadyRunning {
				if v.Synced {
					fmt.Fprintf(w, "%s %s %s\n",
						tui.OKS("✓"), tui.MutedS("Synced"), tui.AccentS(projectName))
				} else {
					fmt.Fprintf(w, "%s %s is already running.\n",
						tui.BrandS("Docktree"), tui.AccentS(projectName))
				}
				if len(v.Ports) > 0 {
					fmt.Fprintln(w)
					renderPortList(w, v.Ports)
				}
				return
			}

			fmt.Fprintf(w, "%s Started %s", tui.OKS("✓"), tui.AccentS(projectName))
			if v.Synced {
				fmt.Fprintf(w, " %s", tui.Badge("synced", "SYNCED"))
			}
			fmt.Fprintln(w)

			if len(v.Ports) > 0 {
				fmt.Fprintln(w)
				fmt.Fprintln(w, tui.DimS("Allocated ports"))
				renderPortList(w, v.Ports)
			}
			if len(v.SharedServices) > 0 {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s  %s\n",
					tui.DimS("Platform services:"),
					tui.InfoS(strings.Join(v.SharedServices, ", ")))
			}

			if v.CreatedWorktree != "" || len(v.ComposeFiles) > 0 || v.OverrideFile != "" {
				fmt.Fprintln(w)
			}
			if v.CreatedWorktree != "" {
				fmt.Fprintf(w, "%s  %s\n", tui.DimS("Created worktree"), v.CreatedWorktree)
			}
			if len(v.ComposeFiles) > 0 {
				fmt.Fprintf(w, "%s  %s\n", tui.DimS("Compose files:"), strings.Join(v.ComposeFiles, ", "))
			}
			if v.OverrideFile != "" {
				fmt.Fprintf(w, "%s       %s\n", tui.DimS("Override:"), v.OverrideFile)
			}

			if v.Scaffolded {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s Scaffolded %s\n",
					tui.OKS("✓"), tui.AccentS("docktree.yml"))
			}
			for _, warning := range v.EnvWarnings {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s %s\n",
					tui.WarningS("⚠ Warning:"), tui.DimS(warning.Message))
			}
			if len(v.IsolatedVolumes) > 0 {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s %s\n",
					tui.DimS("Isolated volumes:"), strings.Join(v.IsolatedVolumes, ", "))
			}
		case DownResult:
			if len(v.DroppedTenants) > 0 {
				for _, db := range v.DroppedTenants {
					fmt.Fprintf(w, "%s Dropped tenant database: %s\n",
						tui.WarningS("!"), tui.MutedS(db))
				}
			}
			if v.AlreadyStopped {
				if v.Instance != nil {
					fmt.Fprintf(w, "%s %s is already stopped.\n",
						tui.BrandS("Docktree"), tui.AccentS(v.Instance.ProjectName))
				} else {
					fmt.Fprintf(w, "%s is already stopped.\n", tui.BrandS("Docktree"))
				}
				return
			}
			if v.DryRun {
				fmt.Fprintf(w, "Docktree dry run - would stop %s\n", v.Instance.ProjectName)
				fmt.Fprintf(w, "  Services: %s\n", strings.Join(v.Services, ", "))
				fmt.Fprintf(w, "  Compose files:\n")
				for _, f := range v.ComposeFiles {
					fmt.Fprintf(w, "    %s\n", f)
				}
				return
			}
			fmt.Fprintf(w, "Docktree stopped %s\n", v.Instance.ProjectName)
			if len(v.Services) > 0 {
				fmt.Fprintf(w, "  Services: %s\n", strings.Join(v.Services, ", "))
			}
		case StopResult:
			if v.AlreadyStopped {
				fmt.Fprintln(w, "Docktree is already stopped.")
				return
			}
			if v.DryRun {
				fmt.Fprintf(w, "Docktree dry run - would stop %s (containers only, not removed)\n", v.Instance.ProjectName)
				fmt.Fprintf(w, "  Services: %s\n", strings.Join(v.Services, ", "))
				fmt.Fprintf(w, "  Compose files:\n")
				for _, f := range v.ComposeFiles {
					fmt.Fprintf(w, "    %s\n", f)
				}
				return
			}
			fmt.Fprintf(w, "Docktree stopped %s (containers only, not removed)\n", v.Instance.ProjectName)
			if len(v.Services) > 0 {
				fmt.Fprintf(w, "  Services: %s\n", strings.Join(v.Services, ", "))
			}
		case ComposePassthroughResult:
			// Output already streamed by docker compose; nothing to render in human mode.
		case ValidateResult:
			if v.Valid {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.OKS("config is valid"))
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s  %s\n", tui.DimS("Services:"), strings.Join(v.Services, ", "))
				if len(v.Ports) > 0 {
					fmt.Fprintln(w)
					fmt.Fprintf(w, "%s\n", tui.DimS("Ports:"))
					for _, a := range v.Ports {
						fmt.Fprintf(w, "  %-14s%s %s %s\n",
							tui.TextS(a.Service),
							tui.MutedS(fmt.Sprintf("%d", a.ContainerPort)),
							tui.DimS("→"),
							tui.AccentS(fmt.Sprintf("%d", a.HostPort)))
					}
				}
				if len(v.IsolatedVolumes) > 0 {
					fmt.Fprintln(w)
					fmt.Fprintf(w, "%s  %s\n",
						tui.DimS("Isolated volumes:"), strings.Join(v.IsolatedVolumes, ", "))
				}
			} else {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.ErrorS("config has errors:"))
				for _, e := range v.Errors {
					fmt.Fprintf(w, "  %s %s\n", tui.ErrorS("✗"), e)
				}
			}
			for _, warning := range v.EnvWarnings {
				fmt.Fprintf(w, "  %s %s: %s\n",
					tui.WarningS("⚠"), tui.DimS(warning.Key), warning.Message)
			}
		case DryRunResult:
			fmt.Fprintf(w, "%s %s %s\n",
				tui.BrandS("Docktree"), tui.MutedS("dry run for"), tui.AccentS(v.InstanceName))
			fmt.Fprintln(w)
			fmt.Fprintf(w, "%s  %s\n", tui.DimS("Services:"), strings.Join(v.Services, ", "))
			fmt.Fprintf(w, "%s\n", tui.DimS("Compose files:"))
			for _, f := range v.ComposeFiles {
				fmt.Fprintf(w, "  %s\n", tui.AccentS(f))
			}
			if len(v.Ports) > 0 {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s\n", tui.DimS("Port assignments:"))
				for _, a := range v.Ports {
					fmt.Fprintf(w, "  %-14s%s %s %s\n",
						tui.TextS(a.Service),
						tui.MutedS(fmt.Sprintf("%d", a.ContainerPort)),
						tui.DimS("→"),
						tui.AccentS(fmt.Sprintf("%d", a.HostPort)))
				}
			}
			if len(v.IsolatedVolumes) > 0 {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s  %s\n",
					tui.DimS("Isolated volumes:"), strings.Join(v.IsolatedVolumes, ", "))
			}
			if v.OverridePreview != "" {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s\n", tui.WarningS("Override preview:"))
				fmt.Fprintln(w, v.OverridePreview)
			}
			if v.ClearPreview != "" {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s\n", tui.DimS("Port clear preview:"))
				fmt.Fprintln(w, v.ClearPreview)
			}
			for _, warning := range v.EnvWarnings {
				fmt.Fprintf(w, "  %s %s: %s\n",
					tui.WarningS("⚠"), tui.DimS(warning.Key), warning.Message)
			}
		case StatusResult:
			if v.Stopped {
				fmt.Fprintf(w, "%s is stopped.\n", tui.BrandS("Docktree"))
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s\n", tui.MutedS("Run `docktree up` to start this worktree."))
				return
			}
			if v.Raw != nil {
				var services []composePsEntry
				_ = json.Unmarshal(v.Raw, &services)
				if len(services) == 0 {
					fmt.Fprintf(w, "%s %s  %s\n",
						tui.ErrorS("●"), tui.AccentS(v.Instance.ProjectName), tui.Badge("stopped", "STOPPED"))
					fmt.Fprintf(w, "%s  %s\n",
						tui.DimS("Branch:"), tui.MutedS(v.Instance.Branch))
					fmt.Fprintln(w)
					fmt.Fprintf(w, "%s\n", tui.MutedS("Run `docktree up` to start services."))
					return
				}
				if true {
					running := 0
					for _, s := range services {
						if strings.EqualFold(s.State, "running") {
							running++
						}
					}
					statusLabel := tui.OKS("running")
					statusBadge := tui.Badge("ok", "RUNNING")
					if running < len(services) && running > 0 {
						statusLabel = tui.WarningS("partial")
						statusBadge = tui.Badge("warning", "PARTIAL")
					} else if running == 0 {
						statusLabel = tui.ErrorS("stopped")
						statusBadge = tui.Badge("error", "STOPPED")
					}
					_ = statusLabel

					if v.Instance != nil {
						fmt.Fprintf(w, "%s %s  %s\n",
							tui.OKS("●"), tui.AccentS(v.Instance.ProjectName), statusBadge)
						fmt.Fprintf(w, "%s  %s    %s\n",
							tui.DimS("Branch:"), tui.MutedS(v.Instance.Branch),
							tui.MutedS(fmt.Sprintf("%d/%d services", running, len(services))))
					}
					fmt.Fprintln(w)

					var svcTbl tui.Table
					svcTbl.Headers = []string{"SERVICE", "IMAGE", "STATE", "STATUS"}
					for _, s := range services {
						img := shortenImage(s.Image)
						status := s.Status
						if status == "" {
							status = "—"
						}
						svcTbl.Rows = append(svcTbl.Rows, []string{s.Service, img, s.State, status})
					}
					fmt.Fprintln(w, svcTbl.RenderBorderedStyled(func(row, col int, val string) string {
						if row == -1 {
							return tui.DimS(val)
						}
						switch col {
						case 0:
							return tui.TextS(val)
						case 1:
							return tui.MutedS(val)
						case 2:
							switch {
							case strings.EqualFold(val, "running"):
								return tui.OKS(val)
							case strings.EqualFold(val, "exited"), strings.EqualFold(val, "restarting"):
								return tui.ErrorS(val)
							default:
								return tui.WarningS(val)
							}
						case 3:
							return tui.DimS(val)
						}
						return val
					}))

					var hasPublishers bool
					for _, s := range services {
						if len(s.Publishers) > 0 {
							for _, p := range s.Publishers {
								if p.PublishedPort > 0 {
									hasPublishers = true
									break
								}
							}
						}
						if hasPublishers {
							break
						}
					}
					if hasPublishers {
						fmt.Fprintln(w)
						var portTbl tui.Table
						portTbl.Headers = []string{"SERVICE", "PORT", "URL"}
						for _, s := range services {
							for _, p := range s.Publishers {
								if p.PublishedPort > 0 {
									url := fmt.Sprintf("http://%s:%d", p.URL, p.PublishedPort)
									portTbl.Rows = append(portTbl.Rows, []string{
										s.Service,
										fmt.Sprintf("%d", p.PublishedPort),
										url,
									})
								}
							}
						}
						fmt.Fprintln(w, portTbl.RenderBorderedStyled(func(row, col int, val string) string {
							if row == -1 {
								return tui.DimS(val)
							}
							switch col {
							case 0:
								return tui.OKS(val)
							case 1:
								return tui.AccentS(val)
							case 2:
								return tui.URLS(val)
							}
							return val
						}))
					}
					return
				}
			}
			if v.Instance != nil {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.AccentS(v.Instance.ProjectName))
				fmt.Fprintf(w, "%s  %s\n", tui.DimS("Branch:"), v.Instance.Branch)
			}
			if v.Text != "" {
				fmt.Fprintln(w, v.Text)
			}
		case PortsResult:
			if v.All {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.MutedS("ports (all instances)"))
			} else {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.AccentS(v.Instance))
			}
			if len(v.Entries) == 0 {
				break
			}
			fmt.Fprintln(w)
			var tbl tui.Table
			if v.All {
				tbl.Headers = []string{"INSTANCE", "SERVICE", "CONTAINER", "HOST", "BIND", "URL"}
				for _, entry := range v.Entries {
					for _, a := range entry.Ports {
						url := fmt.Sprintf("http://%s:%d", a.HostIP, a.HostPort)
						tbl.Rows = append(tbl.Rows, []string{
							entry.Instance, a.Service,
							fmt.Sprintf("%d", a.ContainerPort), fmt.Sprintf("%d", a.HostPort),
							a.HostIP, url,
						})
					}
				}
			} else {
				tbl.Headers = []string{"SERVICE", "CONTAINER", "HOST", "BIND", "URL"}
				for _, entry := range v.Entries {
					for _, a := range entry.Ports {
						url := fmt.Sprintf("http://%s:%d", a.HostIP, a.HostPort)
						tbl.Rows = append(tbl.Rows, []string{
							a.Service,
							fmt.Sprintf("%d", a.ContainerPort), fmt.Sprintf("%d", a.HostPort),
							a.HostIP, url,
						})
					}
				}
			}
			fmt.Fprintln(w, tbl.RenderBorderedStyled(func(row, col int, val string) string {
				if row == -1 {
					return tui.DimS(val)
				}
				if v.All {
					switch col {
					case 0:
						return tui.MutedS(val)
					case 1:
						return tui.OKS(val)
					case 2:
						return tui.DimS(val)
					case 3:
						return tui.AccentS(val)
					case 5:
						return tui.URLS(val)
					}
				} else {
					switch col {
					case 0:
						return tui.OKS(val)
					case 1:
						return tui.DimS(val)
					case 2:
						return tui.AccentS(val)
					case 4:
						return tui.URLS(val)
					}
				}
				return val
			}))
		case PrepareResult:
			fmt.Fprintf(w, "%s %s %s\n",
				tui.BrandS("Docktree"), tui.MutedS("preparing"), tui.AccentS(v.WorktreeRoot))
			fmt.Fprintln(w)
			fmt.Fprintf(w, "%s    %s\n", tui.DimS("Git repo:"), v.RepoRoot)
			fmt.Fprintf(w, "%s   %s\n", tui.DimS("Worktree:"), v.WorktreeRoot)
			if len(v.Ran) > 0 {
				fmt.Fprintln(w)
				for _, step := range v.Ran {
					fmt.Fprintf(w, "  %s %s\n", tui.OKS("✓"), tui.MutedS(step))
				}
			}
		case CreateResult:
			fmt.Fprintf(w, "%s created worktree %s for %s\n",
				tui.BrandS("Docktree"), tui.AccentS(v.WorktreeRoot), tui.AccentS(v.Branch))
			fmt.Fprintln(w)
			fmt.Fprintf(w, "  %s    %s\n", tui.DimS("Git worktree"), tui.MutedS(v.Branch))
			fmt.Fprintf(w, "  %s            %s\n", tui.DimS("Path"), tui.MutedS(v.WorktreeRoot))
			if len(v.Ran) > 0 {
				fmt.Fprintln(w)
				for _, step := range v.Ran {
					fmt.Fprintf(w, "  %s %s\n", tui.OKS("✓"), tui.MutedS(step))
				}
			}
			fmt.Fprintln(w)
			fmt.Fprintf(w, "%s %s %s\n",
				tui.MutedS("Run"), tui.AccentS("docktree up"), tui.MutedS("in the new worktree to start services."))
		case CleanResult:
			if len(v.Instances) == 0 {
				fmt.Fprintf(w, "%s found no stale resources.\n", tui.BrandS("Docktree"))
				return
			}
			if v.DryRun {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.MutedS("dry run — nothing will be removed"))
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s\n", tui.MutedS("Would remove:"))
				for _, item := range v.Instances {
					resources := fmt.Sprintf("→ %d ports, %d containers, %d networks", item.Ports, item.Containers, item.Networks)
					fmt.Fprintf(w, "  %s  %s  %s\n",
						tui.DimS("instance"), tui.AccentS(item.Instance), tui.MutedS(resources))
				}
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s  %s\n", tui.MutedS("Total:"),
					tui.MutedS(fmt.Sprintf("%d ports, %d containers, %d networks",
						v.Totals.Ports, v.Totals.Containers, v.Totals.Networks)))
				return
			}
			if v.Removed {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.MutedS("removed stale resources"))
			} else {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.MutedS("scanning for stale resources..."))
			}
			fmt.Fprintln(w)
			fmt.Fprintf(w, "  %s  %s  %s\n",
				tui.WarningS("INSTANCE"), tui.WarningS("REASON"), tui.WarningS("RESOURCES"))
			for _, item := range v.Instances {
				resources := fmt.Sprintf("%d ports, %d containers, %d networks", item.Ports, item.Containers, item.Networks)
				if v.Volumes {
					resources = fmt.Sprintf("%s, %d volumes", resources, item.Volumes)
				}
				fmt.Fprintf(w, "  %s  %s  %s\n",
					tui.MutedS(item.Instance), tui.DimS(item.Reason), tui.MutedS(resources))
			}
			if v.Removed {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "  %s %s\n", tui.OKS("✓"), tui.MutedS("Removed stale resources"))
				fmt.Fprintf(w, "%s\n",
					tui.MutedS(fmt.Sprintf("%d ports freed. %d instances removed.",
						v.Totals.Ports, v.Totals.Instances)))
			}
		case PlatformResult:
			if v.Skipped {
				fmt.Fprintf(w, "%s %s\n", tui.BrandS("Docktree"), tui.MutedS(v.Reason))
				return
			}
			switch v.Action {
			case "up":
				if v.Running {
					fmt.Fprintf(w, "%s Platform %s\n", tui.OKS("✓"), tui.AccentS(v.Project))
				}
			case "down":
				fmt.Fprintf(w, "%s Stopped platform %s\n", tui.OKS("✓"), tui.AccentS(v.Project))
			case "status":
				state := "stopped"
				if v.Running {
					state = "running"
				}
				fmt.Fprintf(w, "%s Platform %s  %s\n",
					tui.BrandS("Docktree"), tui.AccentS(v.Project), tui.Badge(state, strings.ToUpper(state)))
				fmt.Fprintf(w, "  %s  %s\n", tui.DimS("network:"), tui.MutedS(v.Network))
				for _, svc := range v.Services {
					fmt.Fprintf(w, "  %s  %s\n", tui.DimS("service:"), tui.OKS(svc))
				}
			}
		case PlatformTenantsResult:
			if len(v.Tenants) == 0 {
				fmt.Fprintf(w, "%s No tenant databases found.\n", tui.BrandS("Docktree"))
				return
			}
			var tbl tui.Table
			tbl.Headers = []string{"INSTANCE", "SERVICE", "LOGICAL DB", "TENANT DB", "EXISTS"}
			for _, e := range v.Tenants {
				existsStr := tui.OKS("yes")
				if !e.Exists {
					existsStr = tui.WarningS("no")
				}
				logical := e.LogicalDB
				if logical == "" {
					logical = "default"
				}
				tbl.Rows = append(tbl.Rows, []string{
					truncate(e.Instance, 35),
					e.Service,
					truncate(logical, 18),
					truncate(e.TenantDB, 40),
					existsStr,
				})
			}
			fmt.Fprintln(w, tbl.RenderBorderedStyled(func(row, col int, val string) string {
				if row == -1 {
					return tui.DimS(val)
				}
				switch col {
				case 0:
					return tui.MutedS(val)
				case 1:
					return tui.AccentS(val)
				case 2, 3:
					return tui.TextS(val)
				}
				return val
			}))
		default:
			_ = json.NewEncoder(w).Encode(data)
		}
	}
}

// truncate returns s truncated to max runes, with "…" appended if truncated.
func truncate(s string, max int) string {
	if max <= 3 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
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
	maxCmd := 9
	fmt.Fprintf(w, "%s\n\n", tui.MutedS("docktree coordinates Docker Compose services across git worktrees."))
	fmt.Fprintf(w, "%s\n", tui.TextS("Usage:"))
	fmt.Fprintf(w, "  %s\n\n", tui.AccentS("docktree [--json] <command>"))
	fmt.Fprintf(w, "%s\n", tui.TextS("Commands:"))
	printHelpCmd(w, maxCmd, "create", "Create a worktree and prepare its local Docker setup")
	printHelpCmd(w, maxCmd, "up", "Start the current worktree's Compose project (or --create <branch>)")
	printHelpCmd(w, maxCmd, "down", "Stop the current worktree's Compose project (or specific services)")
	printHelpCmd(w, maxCmd, "stop", "Stop running containers without removing them")
	printHelpCmd(w, maxCmd, "logs", "Pass through to docker compose logs")
	printHelpCmd(w, maxCmd, "exec", "Pass through to docker compose exec")
	printHelpCmd(w, maxCmd, "run", "Pass through to docker compose run --rm")
	printHelpCmd(w, maxCmd, "status", "Show managed worktree services")
	printHelpCmd(w, maxCmd, "ports", "Show allocated host ports (use --all for all worktrees)")
	printHelpCmd(w, maxCmd, "prepare", "Prepare the current worktree's local Docker setup")
	printHelpCmd(w, maxCmd, "platform", "Manage the repo-scoped shared services platform")
	printHelpCmd(w, maxCmd, "clean", "Remove stale Docktree-managed resources")
	printHelpCmd(w, maxCmd, "help", "Show this help text")
	printHelpCmd(w, maxCmd, "version", "Print the docktree version")
}

func printHelpCmd(w io.Writer, max int, cmd, desc string) {
	pad := strings.Repeat(" ", max-len(cmd)+2)
	fmt.Fprintf(w, "  %s%s%s\n", tui.OKS(cmd), pad, desc)
}

type portsOptions struct {
	all  bool
	help bool
}

func parsePortsOptions(args []string) (portsOptions, error) {
	var options portsOptions
	for _, arg := range args {
		switch arg {
		case "-a", "--all":
			options.all = true
		case "-h", "--help":
			options.help = true
		default:
			return portsOptions{}, fmt.Errorf("unknown ports flag %q", arg)
		}
	}
	return options, nil
}

func printPortsHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree ports [options]

Show allocated host ports.

Options:
  -a, --all    Show ports for all worktree instances
  -h, --help   Show this help text`)
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
type downOptions struct {
	help     bool
	dryRun   bool
	volumes  bool
	services []string
}

func parseDownOptions(args []string) (downOptions, error) {
	var options downOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			options.help = true
			return options, nil
		case arg == "--dry-run":
			options.dryRun = true
		case arg == "-v" || arg == "--volumes":
			options.volumes = true
		default:
			if strings.HasPrefix(arg, "-") {
				return downOptions{}, fmt.Errorf("unknown down flag %q", arg)
			}
			options.services = append(options.services, arg)
		}
	}
	return options, nil
}

func printDownHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree down [options] [service...]

Stop the current worktree's Compose project, or specific services.

Options:
  -v, --volumes  Also drop per-worktree tenant databases (postgres per_database).
                 Data is permanently deleted. Platform volumes are kept.
  --dry-run      Show what would be stopped without making changes
  -h, --help     Show this help text

Arguments:
  service        One or more service names to stop (default: all services)`)
}

type stopOptions struct {
	help     bool
	dryRun   bool
	services []string
}

func parseStopOptions(args []string) (stopOptions, error) {
	var options stopOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			options.help = true
			return options, nil
		case arg == "--dry-run":
			options.dryRun = true
		default:
			if strings.HasPrefix(arg, "-") {
				return stopOptions{}, fmt.Errorf("unknown stop flag %q", arg)
			}
			options.services = append(options.services, arg)
		}
	}
	return options, nil
}

func printStopHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree stop [options] [service...]

Stop running containers without removing them (unlike down).

Options:
  --dry-run    Show what would be stopped without making changes
  -h, --help   Show this help text

Arguments:
  service      One or more service names to stop (default: all services)`)
}

func printLogsHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree logs [options] [service...]

Pass through to docker compose logs with the current worktree's project context.

Options and arguments are passed through directly to docker compose logs.
Common options: --follow (-f), --tail N, --since, --timestamps

Examples:
  docktree logs                  # tail all services
  docktree logs api             # tail the api service
  docktree logs api --tail 50   # last 50 lines
  docktree logs -f db            # follow db logs`)
}

func printExecHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree exec <service> -- <command> [args...]

Pass through to docker compose exec with the current worktree's project context.
Options and arguments are passed through directly to docker compose exec.

Examples:
  docktree exec db -- psql -U postgres
  docktree exec api -- sh
  docktree exec --index 1 api -- bash`)
}

func printRunHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree run [options] <service> -- <command> [args...]

Pass through to docker compose run --rm with the current worktree's project context.
Containers are removed after the command exits (--rm is always included).
Options and arguments are passed through directly to docker compose run.

Examples:
  docktree run api -- rake db:migrate
  docktree run db -- psql -U postgres
  docktree run --no-deps api -- rspec`)
}

type upOptions struct {
	help     bool
	file     string
	create   string
	sync     bool
	validate bool
	dryRun   bool
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
		case arg == "--validate":
			options.validate = true
		case arg == "--dry-run":
			options.dryRun = true
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
  --validate            Check config, ports, and compose validity without starting
  --dry-run             Show what would happen without making changes
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

func sortedKeys(m map[string][]ports.Assignment) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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

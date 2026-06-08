package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

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
	if options.all {
		return runDownAll(ctx, options, &repo)
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
	if options.volumes {
		downArgs = append(downArgs, "-v")
	}
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

func runDownAll(ctx *Context, options downOptions, repo *dockgit.RepoInfo) (any, int, error) {
	instances, _ := state.LoadGlobalInstances("")
	var repoInstances []*state.Instance
	for i := range instances {
		inst := instances[i]
		if inst.RepoRoot == repo.RepoRoot {
			repoInstances = append(repoInstances, &inst)
		}
	}
	if len(repoInstances) == 0 {
		return DownResult{AlreadyStopped: true}, output.ExitNoop, nil
	}
	if options.dryRun {
		var names []string
		for _, inst := range repoInstances {
			names = append(names, inst.ProjectName)
		}
		return DownResult{DryRun: true, Services: names}, output.ExitOK, nil
	}
	steps := ctx.Steps
	dockerStdout := ctx.Stdout
	if ctx.Renderer.JSON {
		dockerStdout = io.Discard
	}
	var allDroppedTenants []string
	var allDroppedVolumes []string
	for _, inst := range repoInstances {
		cfg, err := config.Load(inst.RepoRoot)
		if err != nil {
			fmt.Fprintf(ctx.Stderr, "warning: skipping %s: %v\n", inst.Name, err)
			continue
		}
		composeFiles := activeComposeFiles(inst.WorktreeRoot, cfg, inst)
		runningState, err := composeRunStateForInstance(inst, cfg)
		if err != nil {
			fmt.Fprintf(ctx.Stderr, "warning: skipping %s: %v\n", inst.Name, err)
			continue
		}
		if runningState == composeRunStopped {
			continue
		}
		if steps != nil {
			steps.Header("Stopping services…", inst.ProjectName)
		}
		if options.volumes && len(cfg.Shared.Services) > 0 {
			plan, planErr := buildPlatformPlan()
			if planErr == nil {
				for _, binding := range tenantBindingsForInstance(plan, inst) {
					fmt.Fprintf(ctx.Stderr, "Dropping tenant database: %s\n", binding.TenantDB)
					if err := provision.Deprovision(binding.Config); err != nil {
						fmt.Fprintf(ctx.Stderr, "warning: failed to drop %s: %v\n", binding.TenantDB, err)
					} else {
						allDroppedTenants = append(allDroppedTenants, binding.TenantDB)
					}
				}
			}
		}
		downArgs := []string{"down"}
		if options.volumes {
			downArgs = append(downArgs, "-v")
		}
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
			fmt.Fprintf(ctx.Stderr, "warning: failed to stop %s: %v\n", inst.Name, err)
			continue
		}
		if spin != nil {
			spin.Stop()
		}
		if options.volumes {
			resources, err := docker.ListProjectResources(inst.ProjectName, true)
			if err == nil {
				for _, vol := range resources.Volumes {
					allDroppedVolumes = append(allDroppedVolumes, vol.Name)
				}
			}
		}
		inst.LastActiveAt = time.Now().UTC()
		stateDir := state.StatePath(inst.WorktreeRoot, cfg.State.Directory)
		if err := state.SaveInstance(stateDir, inst); err != nil {
			fmt.Fprintf(ctx.Stderr, "warning: failed to save state for %s: %v\n", inst.Name, err)
		}
		if err := state.UpsertGlobalInstance("", inst); err != nil {
			fmt.Fprintf(ctx.Stderr, "warning: failed to update global state for %s: %v\n", inst.Name, err)
		}
	}
	services := options.services
	if len(services) == 0 {
		services = []string{"all"}
	}
	return DownResult{Services: services, DroppedTenants: allDroppedTenants, DroppedVolumes: allDroppedVolumes}, output.ExitOK, nil
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

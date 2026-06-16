package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	// When the worktree has no shared.services of its own, inherit them
	// from the main repo's docktree.yml so a single config covers every
	// linked worktree.
	if len(cfg.Shared.Services) == 0 {
		mainRoot, mErr := dockgit.MainRepoRoot()
		if mErr == nil && mainRoot != repo.RepoRoot {
			mainCfg, mErr := config.Load(mainRoot)
			if mErr == nil && len(mainCfg.Shared.Services) > 0 {
				cfg.Shared.Services = mainCfg.Shared.Services
			}
		}
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
		if err := setup.Prepare(setup.Options{SourceDir: repo.RepoRoot, TargetDir: repo.WorktreeRoot, Config: cfg, Stdout: ctx.Stdout, Stderr: ctx.Stderr}); err != nil {
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
	if len(cfg.Shared.Services) > 0 {
		if err := state.EnsureStateDir(repo.WorktreeRoot, cfg.State.Directory); err != nil {
			return nil, output.ExitConfig, err
		}
		rawProj, _, lerr := compose.LoadFull(files)
		if lerr != nil {
			return nil, output.ExitConfig, lerr
		}
		mainRoot, err := dockgit.MainRepoRoot()
		if err != nil {
			return nil, output.ExitConfig, err
		}
		repoSlug := dockgit.RepoName(mainRoot)
		tenantDBs := make(map[string]map[string]string, len(cfg.Shared.Services))
		for svcName, svc := range cfg.Shared.Services {
			targets := svc.DatabaseTargets()
			if len(targets) == 0 {
				continue
			}
			logicalDBs := make(map[string]string, len(targets))
			for logicalName, dbTarget := range targets {
				logicalDBs[logicalName] = provision.ResolveTenantName(dbTarget.Tenancy, repoSlug, instanceName, logicalName)
			}
			tenantDBs[svcName] = logicalDBs
		}
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
				envWarnings = append(envWarnings, compose.Warning{Key: "shared." + svcName + ".url_envs", Message: "service " + svcName + " uses tenancy: per_database but url_envs is not declared. DATABASE_URL will NOT be rewritten — all worktrees will hit the same database. Add url_envs: [DATABASE_URL] (or your connection env name) to docktree.yml to fix isolation."})
			}
		}
		wtProj, serr := compose.SynthesizeWorktree(rawProj, cfg.Shared, repoSlug, compose.SynthesizeWorktreeOptions{TenantDBs: tenantDBs})
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
	if len(cfg.Shared.Services) > 0 {
		mainRoot, err := dockgit.MainRepoRoot()
		if err != nil {
			return nil, output.ExitConfig, err
		}
		if _, _, platErr := ensurePlatformUp(ctx, instanceName, dockgit.RepoName(mainRoot), repo.RepoRoot); platErr != nil {
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
					steps.Sub(fmt.Sprintf("%-12s%s %s %s", a.Service, tui.DimS(fmt.Sprintf("%d", a.ContainerPort)), tui.DimS("→"), tui.AccentS(fmt.Sprintf("%d", a.HostPort))))
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

func runValidate(project *compose.ComposeProject, files []string, cfg *config.Config, repo dockgit.RepoInfo, envWarnings []compose.Warning) (any, int, error) {
	var errs []string
	if len(project.Services) == 0 {
		errs = append(errs, "no services defined in compose file")
	}
	for name, svc := range project.Services {
		if svc.Build != nil && svc.Build.Context != "" {
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
	return ValidateResult{Valid: len(errs) == 0, Services: serviceNames(project), Ports: assignments, IsolatedVolumes: isolated, EnvWarnings: envWarnings, Errors: errs}, output.ExitOK, nil
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
	return DryRunResult{DryRun: true, InstanceName: instanceName, ComposeFiles: files, Services: serviceNames(project), Ports: assignments, IsolatedVolumes: isolatedVolumes(project, repoRootVolumesShare()), EnvWarnings: envWarnings, OverridePreview: string(overrideYAML), ClearPreview: clearPreview}, output.ExitOK, nil
}

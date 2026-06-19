package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/docker"
	dockgit "github.com/bnjoroge/docktree/internal/git"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/provision"
	"github.com/bnjoroge/docktree/internal/state"
	"github.com/bnjoroge/docktree/internal/tui"
)

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
		return DownResult{Instance: inst, DryRun: true, Services: services, ComposeFiles: composeFiles}, output.ExitOK, nil
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
	var droppedTenants []string
	if options.volumes && len(cfg.Shared.Services) > 0 {
		plan, planErr := buildPlatformPlan("")
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
			plan, planErr := buildPlatformPlan("")
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
		return StopResult{Instance: inst, DryRun: true, Services: services, ComposeFiles: composeFiles}, output.ExitOK, nil
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

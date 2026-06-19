package cli

import (
	"io"

	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/docker"
	dockgit "github.com/bnjoroge/docktree/internal/git"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/state"
)

func runComposePassthrough(ctx *Context, subcommand string, args []string, allowEmptyArgs bool, helpFn func(io.Writer)) (any, int, error) {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		helpFn(ctx.Stdout)
		return nil, output.ExitOK, nil
	}
	if len(args) == 0 && !allowEmptyArgs {
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
	return runComposePassthrough(ctx, "logs", ctx.Args[1:], true, printLogsHelp)
}

func runExec(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "exec", ctx.Args[1:], false, printExecHelp)
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

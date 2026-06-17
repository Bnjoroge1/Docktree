package cli

import (
	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/docker"
	dockgit "github.com/bnjoroge/docktree/internal/git"
	"github.com/bnjoroge/docktree/internal/output"
)

func runConfig(ctx *Context) (any, int, error) {
	args := ctx.Args[1:]
	if len(args) == 0 || (len(args) == 1 && (args[0] == "-h" || args[0] == "--help")) {
		printConfigHelp(ctx.Stdout)
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
	files, err := composeFiles(repo.WorktreeRoot, cfg)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	composeArgs := append([]string{"config"}, args...)
	var cmd docker.ComposeCommand
	cmd.Files = files
	cmd.CommandArgs = composeArgs
	if err := docker.Run(cmd, ctx.Stdout, ctx.Stderr); err != nil {
		return nil, output.ExitDocker, err
	}
	return nil, output.ExitOK, nil
}

func runImages(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "images", ctx.Args[1:], printImagesHelp)
}

func runTop(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "top", ctx.Args[1:], printTopHelp)
}

func runLs(ctx *Context) (any, int, error) {
	args := ctx.Args[1:]
	if len(args) == 0 || (len(args) == 1 && (args[0] == "-h" || args[0] == "--help")) {
		printLsHelp(ctx.Stdout)
		return nil, output.ExitOK, nil
	}
	composeArgs := append([]string{"ls"}, args...)
	cmd := docker.ComposeCommand{
		CommandArgs: composeArgs,
	}
	if err := docker.Run(cmd, ctx.Stdout, ctx.Stderr); err != nil {
		return nil, output.ExitDocker, err
	}
	return nil, output.ExitOK, nil
}

func runPort(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "port", ctx.Args[1:], printPortHelp)
}

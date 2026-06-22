package cli

import (
	"strings"

	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/docker"
	dockgit "github.com/bnjoroge/docktree/internal/git"
	"github.com/bnjoroge/docktree/internal/output"
)

func runConfig(ctx *Context) (any, int, error) {
	args := ctx.Args[1:]
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
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
	composeArgs := configComposeArgs(args, ctx.Renderer.JSON)
	var cmd docker.ComposeCommand
	cmd.Files = files
	cmd.CommandArgs = composeArgs
	if err := docker.Run(cmd, ctx.Stdout, ctx.Stderr); err != nil {
		return nil, output.ExitDocker, err
	}
	return nil, output.ExitOK, nil
}

func runImages(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "images", ctx.Args[1:], true, printImagesHelp)
}

func runTop(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "top", ctx.Args[1:], true, printTopHelp)
}

func runLs(ctx *Context) (any, int, error) {
	args := ctx.Args[1:]
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
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
	return runComposePassthrough(ctx, "port", ctx.Args[1:], false, printPortHelp)
}

func configComposeArgs(args []string, jsonMode bool) []string {
	composeArgs := []string{"config"}
	if jsonMode && !hasConfigFormatArg(args) {
		composeArgs = append(composeArgs, "--format", "json")
	}
	return append(composeArgs, args...)
}

func hasConfigFormatArg(args []string) bool {
	for i, arg := range args {
		if arg == "--format" && i+1 < len(args) {
			return true
		}
		if strings.HasPrefix(arg, "--format=") {
			return true
		}
	}
	return false
}

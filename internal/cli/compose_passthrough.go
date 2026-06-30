package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

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
		// No saved instance — tabular commands return empty results;
		// other commands error out.
		if subcommand != "" {
			switch subcommand {
			case "images":
				return ImagesResult{Entries: []ImagesEntry{}}, output.ExitOK, nil
			case "top":
				return TopResult{Rows: nil}, output.ExitOK, nil
			}
		}
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
	args = stripRunSeparator(args)
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

func stripRunSeparator(args []string) []string {
	for i, arg := range args {
		if arg != "--" {
			continue
		}
		stripped := make([]string, 0, len(args)-1)
		stripped = append(stripped, args[:i]...)
		stripped = append(stripped, args[i+1:]...)
		return stripped
	}
	return args
}

func printDockerHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree docker <subcommand> [args...]

Run any docker compose subcommand with the current worktree's project name
and compose files pre-filled. Useful for flags and subcommands that docktree
does not wrap directly.

Examples:
  docktree docker up -h
  docktree docker up --no-deps api
  docktree docker ps
  docktree docker scale api=3

  -h, --help   Show this help text`)
}

func runDocker(ctx *Context) (any, int, error) {
	args := ctx.Args[1:]
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printDockerHelp(ctx.Stdout)
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

	// Intercept tabular subcommands and render with docktree table formatting.
	// Only intercept clean runs — pass through if the user gave output-shaping flags.
	if !hasOutputShapingFlags(args[1:]) {
	switch args[0] {
	case "images":
		return runDockerImages(inst.ProjectName, composeFiles, args)
	case "top":
		return runDockerTop(inst.ProjectName, composeFiles, args)
	case "ls":
		return runDockerLs(composeFiles, args)
	}
	}

	cmd := docker.ComposeCommand{
		ProjectName: inst.ProjectName,
		Files:       composeFiles,
		CommandArgs: args,
	}
	if err := docker.Run(cmd, ctx.Stdout, ctx.Stderr); err != nil {
		return ComposePassthroughResult{
			Project:      inst.ProjectName,
			ComposeFiles: composeFiles,
			Subcommand:   args[0],
			Args:         args[1:],
		}, output.ExitDocker, err
	}
	return ComposePassthroughResult{
		Project:      inst.ProjectName,
		ComposeFiles: composeFiles,
		Subcommand:   args[0],
		Args:         args[1:],
	}, output.ExitOK, nil
}

func runDockerImages(projectName string, composeFiles, args []string) (any, int, error) {
	cmd := docker.ComposeCommand{
		ProjectName: projectName,
		Files:       composeFiles,
		CommandArgs: append([]string{"images", "--format", "json"}, args[1:]...),
	}
	out, err := docker.RunCapture(cmd)
	if err != nil {
		return ImagesResult{}, output.ExitDocker, err
	}
	if strings.TrimSpace(out) == "null" || strings.TrimSpace(out) == "" {
		return ImagesResult{Entries: []ImagesEntry{}}, output.ExitOK, nil
	}
	var entries []ImagesEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		return nil, output.ExitConfig, fmt.Errorf("parsing images JSON: %w", err)
	}
	return ImagesResult{ProjectName: projectName, Entries: entries}, output.ExitOK, nil
}



func runDockerTop(projectName string, composeFiles, args []string) (any, int, error) {
	cmd := docker.ComposeCommand{
		ProjectName: projectName,
		Files:       composeFiles,
		CommandArgs: append([]string{"top"}, args[1:]...),
	}
	out, err := docker.RunCapture(cmd)
	if err != nil {
		return TopResult{}, output.ExitDocker, err
	}
	rows := parseTopOutput(out)
	return TopResult{Rows: rows}, output.ExitOK, nil
}

func runDockerLs(composeFiles, args []string) (any, int, error) {
	composeArgs := append([]string{"ls", "--format", "json"}, args[1:]...)
	cmd := docker.ComposeCommand{
		CommandArgs: composeArgs,
	}
	out, err := docker.RunCapture(cmd)
	if err != nil {
		return nil, output.ExitDocker, err
	}
	if strings.TrimSpace(out) == "" {
		return LsResult{Entries: []LsEntry{}}, output.ExitOK, nil
	}
	var entries []LsEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		return nil, output.ExitConfig, fmt.Errorf("parsing ls JSON: %w", err)
	}
	return LsResult{Entries: entries}, output.ExitOK, nil
}

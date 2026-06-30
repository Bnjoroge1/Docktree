package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/docker"
	dockgit "github.com/bnjoroge/docktree/internal/git"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/state"
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
	args := ctx.Args[1:]
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		printImagesHelp(ctx.Stdout)
		return nil, output.ExitOK, nil
	}
	// If the user passed flags that control output format, fall back to raw passthrough.
	if hasOutputShapingFlags(args) {
		return runComposePassthrough(ctx, "images", args, true, printImagesHelp)
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
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, output.ExitConfig, fmt.Errorf("failed to load state: %w", err)
		}
		return ImagesResult{Entries: []ImagesEntry{}}, output.ExitOK, nil
	}
	composeFiles := activeComposeFiles(repo.WorktreeRoot, cfg, inst)
	cmd := docker.ComposeCommand{
		ProjectName: inst.ProjectName,
		Files:       composeFiles,
		CommandArgs: append([]string{"images", "--format", "json"}, args...),
	}
	out, err := docker.RunCapture(cmd)
	if err != nil {
		return ImagesResult{}, output.ExitDocker, err
	}
	if strings.TrimSpace(out) == "null" || strings.TrimSpace(out) == "" {
		return ImagesResult{Entries: []ImagesEntry{}}, output.ExitOK, nil
	}
	var entries []ImagesEntry
	dec := json.NewDecoder(strings.NewReader(out))
	for dec.More() {
		var entry ImagesEntry
		if err := dec.Decode(&entry); err != nil {
			return nil, output.ExitConfig, fmt.Errorf("parsing images JSON: %w", err)
		}
		entries = append(entries, entry)
	}
	return ImagesResult{ProjectName: inst.ProjectName, Entries: entries}, output.ExitOK, nil
}

func runTop(ctx *Context) (any, int, error) {
	args := ctx.Args[1:]
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		printTopHelp(ctx.Stdout)
		return nil, output.ExitOK, nil
	}
	if hasOutputShapingFlags(args) {
		return runComposePassthrough(ctx, "top", args, true, printTopHelp)
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
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, output.ExitConfig, fmt.Errorf("failed to load state: %w", err)
		}
		return TopResult{Rows: nil}, output.ExitOK, nil
	}
	composeFiles := activeComposeFiles(repo.WorktreeRoot, cfg, inst)
	cmd := docker.ComposeCommand{
		ProjectName: inst.ProjectName,
		Files:       composeFiles,
		CommandArgs: append([]string{"top"}, args...),
	}
	out, err := docker.RunCapture(cmd)
	if err != nil {
		return TopResult{}, output.ExitDocker, err
	}
	rows := parseTopOutput(out)
	return TopResult{Rows: rows}, output.ExitOK, nil
}

func runLs(ctx *Context) (any, int, error) {
	args := ctx.Args[1:]
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		printLsHelp(ctx.Stdout)
		return nil, output.ExitOK, nil
	}
	if hasOutputShapingFlags(args) {
		composeArgs := append([]string{"ls"}, args...)
		cmd := docker.ComposeCommand{CommandArgs: composeArgs}
		if err := docker.Run(cmd, ctx.Stdout, ctx.Stderr); err != nil {
			return nil, output.ExitDocker, err
		}
		return nil, output.ExitOK, nil
	}
	composeArgs := append([]string{"ls", "--format", "json"}, args...)
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
	for _, arg := range args {
		if arg == "--format" || strings.HasPrefix(arg, "--format=") {
			return true
		}
	}
	return false
}

// hasOutputShapingFlags returns true if args contain flags that change the
// output format (--format, -q/--quiet, --no-trunc). When the user passes
// these, we fall back to raw passthrough instead of capturing and re‑rendering.
func hasOutputShapingFlags(args []string) bool {
	for _, a := range args {
		switch a {
		case "-q", "--quiet", "--no-trunc":
			return true
		default:
			if a == "--format" || strings.HasPrefix(a, "--format=") {
				return true
			}
		}
	}
	return false
}

// parseTopOutput parses the text table output of docker compose top.
// Columns: SERVICE, #, UID, PID, PPID, C, STIME, TTY, TIME, CMD
func parseTopOutput(out string) []TopRow {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return nil
	}
	// Skip header line
	var rows []TopRow
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		rows = append(rows, TopRow{
			Service: fields[0],
			Num:     fields[1],
			UID:     fields[2],
			PID:     fields[3],
			PPID:    fields[4],
			CPU:     fields[5],
			STime:   fields[6],
			TTY:     fields[7],
			Time:    fields[8],
			Cmd:     strings.Join(fields[9:], " "),
		})
	}
	return rows
}

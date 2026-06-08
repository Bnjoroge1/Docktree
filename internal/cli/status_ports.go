package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/docker"
	dockgit "github.com/bnjoroge/docktree/internal/git"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/ports"
	"github.com/bnjoroge/docktree/internal/state"
	"github.com/bnjoroge/docktree/internal/tui"
)

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

// renderPortList draws the allocated ports as a bordered table:
// SERVICE | PORT | URL  (internal services show "(internal)" in the URL column).
func renderPortList(w io.Writer, portAssignments []ports.Assignment) {
	var tbl tui.Table
	tbl.TermWidth = termWidth(w)
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

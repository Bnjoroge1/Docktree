package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
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
	options, err := parseStatusOptions(ctx.Args[1:])
	if err != nil {
		return nil, output.ExitUsage, err
	}
	if options.help {
		return statusHelpDoc(), output.ExitOK, nil
	}

	if options.all {
		return runStatusAll(ctx)
	}

	repo, err := dockgit.DetectRepo()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	cfg, err := loadConfigWithSharedWarnings(repo.RepoRoot, ctx.Stderr)
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
	return statusForInstance(ctx, inst, cfg)
}

func runStatusAll(ctx *Context) (any, int, error) {
	instances, err := state.LoadGlobalInstances("")
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if len(instances) == 0 {
		return StatusAllResult{}, output.ExitNoop, nil
	}

	// Sort instances by name for stable output.
	names := make([]string, 0, len(instances))
	for name := range instances {
		names = append(names, name)
	}
	sort.Strings(names)

	var entries []StatusAllEntry
	for _, name := range names {
		inst := instances[name]
		entry := StatusAllEntry{
			Instance: inst.Name,
			Branch:   inst.Branch,
		}

		// Check if worktree path still exists.
		if inst.WorktreeRoot == "" {
			entries = append(entries, entry)
			continue
		}
		if _, err := os.Stat(inst.WorktreeRoot); err != nil {
			entries = append(entries, entry)
			continue
		}

		cfg, err := config.Load(inst.RepoRoot)
		if err != nil {
			entries = append(entries, entry)
			continue
		}

		out, err := docker.RunCapture(docker.ComposeCommand{
			ProjectName: inst.ProjectName,
			Files:       activeComposeFiles(inst.WorktreeRoot, cfg, &inst),
			CommandArgs: []string{"ps", "--format", "json"},
		})
		if err != nil {
			entries = append(entries, entry)
			continue
		}

		services := parseComposePS(out)
		entry.TotalServices = len(services)
		for _, svc := range services {
			switch {
			case strings.EqualFold(svc.State, "running"):
				entry.RunningCount++
			case strings.EqualFold(svc.State, "paused"):
				entry.PausedCount++
			}
		}
		entry.Running = entry.RunningCount > 0
		entry.Paused = entry.PausedCount > 0 && entry.RunningCount == 0
		entry.ServiceCount = entry.TotalServices
		entries = append(entries, entry)
	}

	return StatusAllResult{Entries: entries}, output.ExitOK, nil
}

func statusForInstance(ctx *Context, inst *state.Instance, cfg *config.Config) (any, int, error) {
	out, err := docker.RunCapture(docker.ComposeCommand{ProjectName: inst.ProjectName, Files: activeComposeFiles(inst.WorktreeRoot, cfg, inst), CommandArgs: []string{"ps", "--format", "json"}})
	if err != nil {
		return nil, output.ExitDocker, err
	}
	result := StatusResult{Instance: inst, Text: strings.TrimSpace(out)}
	// docker compose ps --format json outputs NDJSON (one JSON object per line).
	// Parse it into a JSON array so the renderer can unmarshal it.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var arr []json.RawMessage
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !json.Valid([]byte(line)) {
			continue
		}
		arr = append(arr, json.RawMessage(line))
	}
	if len(arr) > 0 {
		data, _ := json.Marshal(arr)
		result.Raw = data
	} else if inst.ComposeFileHash != "" {
		// Compose project exists but no containers running — services are down.
		result.Stopped = true
	}
	return result, output.ExitOK, nil
}

type composePSEntry struct {
	Service string `json:"Service"`
	State   string `json:"State"`
}

func parseComposePS(out string) []composePSEntry {
	var services []composePSEntry
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !json.Valid([]byte(line)) {
			continue
		}
		var entry composePSEntry
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			services = append(services, entry)
		}
	}
	return services
}

func runPorts(ctx *Context) (any, int, error) {
	options, err := parsePortsOptions(ctx.Args[1:])
	if err != nil {
		return nil, output.ExitUsage, err
	}
	if options.help {
		return portsHelpDoc(), output.ExitOK, nil
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
	repo, err := dockgit.DetectRepo()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if _, err := loadConfigWithSharedWarnings(repo.RepoRoot, ctx.Stderr); err != nil {
		return nil, output.ExitConfig, err
	}
	instanceName := dockgit.InstanceName(dockgit.RepoName(repo.RepoRoot), dockgit.WorktreeName(repo.Branch, repo.WorktreeRoot), repo.RepoRoot, repo.WorktreeRoot)
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

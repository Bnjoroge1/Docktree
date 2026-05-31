package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bnjoroge/docktree/internal/compose"
	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/docker"
	dockgit "github.com/bnjoroge/docktree/internal/git"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/ports"
	"github.com/bnjoroge/docktree/internal/state"
)

const version = "0.1.0-dev"

type commandFunc func(*Context) (any, int, error)

type Context struct {
	Args     []string
	Renderer *output.Renderer
	Stdout   io.Writer
	Stderr   io.Writer
}

type UpResult struct {
	Instance         *state.Instance    `json:"instance"`
	ComposeFiles     []string           `json:"compose_files"`
	OverrideFile     string             `json:"override_file"`
	Ports            []ports.Assignment `json:"ports,omitempty"`
	Services         []string           `json:"services"`
	IsolatedVolumes  []string           `json:"isolated_volumes,omitempty"`
	EnvWarnings      []compose.Warning  `json:"env_warnings,omitempty"`
	Scaffolded       bool               `json:"scaffolded,omitempty"`
	AlreadyRunning   bool               `json:"already_running,omitempty"`
}

type DownResult struct {
	Instance       *state.Instance `json:"instance,omitempty"`
	AlreadyStopped bool            `json:"already_stopped,omitempty"`
}

type StatusResult struct {
	Instance *state.Instance `json:"instance,omitempty"`
	Raw      json.RawMessage `json:"raw,omitempty"`
	Text     string          `json:"text,omitempty"`
	Stopped  bool            `json:"stopped,omitempty"`
}

type PortsResult struct {
	Instance string             `json:"instance"`
	Ports    []ports.Assignment `json:"ports"`
}

//helps us track stale/orphaned docktree instances
type CleanItem struct {
	Instance   string `json:"instance"`
	Reason     string `json:"reason"`
	Ports      int    `json:"ports"`
	Containers int    `json:"containers"`
	Networks   int    `json:"networks"`
	Volumes    int    `json:"volumes,omitempty"`
}

type CleanTotals struct {
	Instances  int `json:"instances"`
	Ports      int `json:"ports"`
	Containers int `json:"containers"`
	Networks   int `json:"networks"`
	Volumes    int `json:"volumes,omitempty"`
}

type CleanResult struct {
	DryRun    bool        `json:"dry_run"`
	Removed   bool        `json:"removed"`
	Volumes   bool        `json:"volumes"`
	Instances []CleanItem `json:"instances"`
	Totals    CleanTotals `json:"totals"`
}

func Run(args []string, stdout, stderr io.Writer) int {
	jsonMode, rest := parseGlobalFlags(args)
	renderer := output.New(stdout, jsonMode)
	ctx := &Context{Args: rest, Renderer: renderer, Stdout: stdout, Stderr: stderr}
	if len(rest) == 0 {
		printHelp(stdout)
		return output.ExitOK
	}
	commands := map[string]commandFunc{
		"up":     runUp,
		"down":   runDown,
		"status": runStatus,
		"ports":  runPorts,
		"clean":  runClean,
	}
	switch rest[0] {
	case "help", "-h", "--help":
		printHelp(stdout)
		return output.ExitOK
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "docktree %s\n", version)
		return output.ExitOK
	}
	fn, ok := commands[rest[0]]
	if !ok {
		fmt.Fprintf(stderr, "unknown command %q\n\n", rest[0])
		printHelp(stderr)
		return output.ExitUsage
	}
	result, code, err := fn(ctx)
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

func runUp(ctx *Context) (any, int, error) {
	options, err := parseUpOptions(ctx.Args[1:])
	if err != nil {
		return nil, output.ExitUsage, err
	}
	repo, cfg, instanceName, err := commonIdentity()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	stateDir := state.StatePath(repo.WorktreeRoot, cfg.State.Directory)
	inst, _ := state.LoadInstance(stateDir)
	var scaffolded bool
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
		}
		envWarnings, err = compose.CheckEnvFile(repo.WorktreeRoot)
		if err != nil {
			return nil, output.ExitConfig, err
		}
		if err := ensureGitignore(repo.WorktreeRoot, cfg.State.Directory); err != nil {
			return nil, output.ExitConfig, err
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
			if scaffolded {
				return nil, output.ExitConfig, fmt.Errorf("no compose file found: create docker-compose.yml or set compose.files in docktree.yml")
			}
			return nil, output.ExitConfig, err
		}
	}
	if inst != nil && isRunning(inst, cfg) {
		currentHash, err := state.HashFiles(files)
		if err != nil {
			return nil, output.ExitConfig, err
		}
		if currentHash == inst.ComposeFileHash {
			return UpResult{Instance: inst, AlreadyRunning: true}, output.ExitNoop, nil
		}
	}
	project, err := parseAll(files)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	portRange, err := ports.ParseRange(cfg.Ports.Range)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	registry := ports.NewRegistry()
	if err := registry.Lock(); err != nil {
		return nil, output.ExitConflict, err
	}
	defer registry.Unlock()
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
	if err := state.SaveInstance(stateDir, inst); err != nil {
		return nil, output.ExitConfig, err
	}
	if err := state.UpsertGlobalInstance("", inst); err != nil {
		return nil, output.ExitConfig, err
	}
	dockerStdout := ctx.Stdout
	if ctx.Renderer.JSON {
		dockerStdout = io.Discard
	}
	var assignments []ports.Assignment
	cmd := docker.ComposeCommand{ProjectName: instanceName, Files: append(append([]string{}, files...), overrideFile), CommandArgs: []string{"up", "-d"}}
	for attempt := 0; attempt < 10; attempt++ {
		assignments, err = registry.Allocate(instanceName, portRequests(project, cfg.Ports.BindHost), portRange)
		if err != nil {
			return nil, output.ExitConflict, err
		}
		override, err := compose.GenerateOverride(project, instanceName, assignments, cfg.Volumes.Share)
		if err != nil {
			return nil, output.ExitConfig, err
		}
		if err := compose.WriteOverride(override, overrideFile); err != nil {
			return nil, output.ExitConfig, err
		}
		if err := docker.Run(cmd, dockerStdout, ctx.Stderr); err != nil {
			if docker.IsPortBindError(err) && attempt < 9 {
				if releaseErr := registry.Release(instanceName); releaseErr != nil {
					return nil, output.ExitConflict, releaseErr
				}
				continue
			}
			return nil, output.ExitDocker, err
		}
		break
	}
	return UpResult{Instance: inst, ComposeFiles: files, OverrideFile: overrideFile, Ports: assignments, Services: serviceNames(project), IsolatedVolumes: isolatedVolumes(project, cfg.Volumes.Share), EnvWarnings: envWarnings, Scaffolded: scaffolded}, output.ExitOK, nil
}

func runDown(ctx *Context) (any, int, error) {
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
		return DownResult{AlreadyStopped: true}, output.ExitNoop, nil
	}
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if !isRunning(inst, cfg) {
		return DownResult{Instance: inst, AlreadyStopped: true}, output.ExitNoop, nil
	}
	dockerStdout := ctx.Stdout
	if ctx.Renderer.JSON {
		dockerStdout = io.Discard
	}
	cmd := docker.ComposeCommand{ProjectName: inst.ProjectName, Files: activeComposeFiles(repo.WorktreeRoot, cfg, inst), CommandArgs: []string{"down"}}
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
	return DownResult{Instance: inst}, output.ExitOK, nil
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
	_, _, instanceName, err := commonIdentity()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	all, err := ports.NewRegistry().Load()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	return PortsResult{Instance: instanceName, Ports: all[instanceName]}, output.ExitOK, nil
}

func runClean(ctx *Context) (any, int, error) {
	options, err := parseCleanOptions(ctx.Args[1:])
	if err != nil {
		return nil, output.ExitUsage, err
	}
	candidates, err := discoverCleanCandidates(options.volumes)
	if err != nil {
		return nil, output.ExitDocker, err
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
	if err := applyCleanCandidates(candidates, options.volumes); err != nil {
		return nil, output.ExitDocker, err
	}
	return cleanResultFromCandidates(candidates, false, options.volumes, true), output.ExitOK, nil
}

func commonIdentity() (dockgit.RepoInfo, *config.Config, string, error) {
	repo, err := dockgit.DetectRepo()
	if err != nil {
		return dockgit.RepoInfo{}, nil, "", err
	}
	cfg, err := config.Load(repo.RepoRoot)
	if err != nil {
		return dockgit.RepoInfo{}, nil, "", err
	}
	instance := dockgit.InstanceName(dockgit.RepoName(repo.RepoRoot), dockgit.WorktreeName(repo.Branch, repo.WorktreeRoot), repo.RepoRoot, repo.WorktreeRoot)
	return repo, cfg, instance, nil
}

func parseGlobalFlags(args []string) (bool, []string) {
	jsonMode := false
	var rest []string
	for _, arg := range args {
		if arg == "--json" {
			jsonMode = true
			continue
		}
		rest = append(rest, arg)
	}
	return jsonMode, rest
}

func composeFiles(dir string, cfg *config.Config) ([]string, error) {
	if len(cfg.Compose.Files) > 0 {
		files := make([]string, 0, len(cfg.Compose.Files))
		for _, file := range cfg.Compose.Files {
			if filepath.IsAbs(file) {
				files = append(files, file)
			} else {
				files = append(files, filepath.Join(dir, file))
			}
		}
		return files, nil
	}
	return compose.FindComposeFiles(dir)
}

func parseAll(files []string) (*compose.ComposeProject, error) {
	return compose.LoadProject(files)
}

func portRequests(project *compose.ComposeProject, bindHost string) []ports.PortRequest {
	var requests []ports.PortRequest
	for service, svc := range project.Services {
		for _, port := range svc.Ports {
			if port.Published == 0 {
				continue
			}
			hostIP := port.HostIP
			if hostIP == "" {
				hostIP = bindHost
			}
			requests = append(requests, ports.PortRequest{Service: service, ContainerPort: port.Target, HostIP: hostIP})
		}
	}
	return requests
}

func isRunning(inst *state.Instance, cfg *config.Config) bool {
	out, err := docker.RunCapture(docker.ComposeCommand{ProjectName: inst.ProjectName, Files: activeComposeFiles(inst.WorktreeRoot, cfg, inst), CommandArgs: []string{"ps", "--format", "json"}})
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(out), "running")
}

func activeComposeFiles(worktreeRoot string, cfg *config.Config, inst *state.Instance) []string {
	files, err := composeFiles(worktreeRoot, cfg)
	if err != nil {
		return nil
	}
	override := filepath.Join(state.StatePath(worktreeRoot, cfg.State.Directory), "generated", inst.ProjectName+".override.yml")
	if _, err := os.Stat(override); err == nil {
		files = append(files, override)
	}
	return files
}

func ensureGitignore(worktreeRoot, stateDir string) error {
	path := filepath.Join(worktreeRoot, ".gitignore")
	entry := strings.Trim(stateDir, "/") + "/"
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return os.WriteFile(path, []byte(entry+"\n"), 0o644)
	}
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == entry || trimmed == strings.TrimSuffix(entry, "/") {
			return nil
		}
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		if _, err := file.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = file.WriteString(entry + "\n")
	return err
}

func serviceNames(project *compose.ComposeProject) []string {
	names := make([]string, 0, len(project.Services))
	for name := range project.Services {
		names = append(names, name)
	}
	return names
}

func containerNames(project *compose.ComposeProject) map[string]string {
	names := map[string]string{}
	for name, svc := range project.Services {
		if svc.ContainerName != "" {
			names[name] = svc.ContainerName
		}
	}
	return names
}

func builtImages(project *compose.ComposeProject) []string {
	var images []string
	for name, svc := range project.Services {
		if svc.Build != nil {
			if svc.Image != "" {
				images = append(images, name+"="+svc.Image)
			} else {
				images = append(images, name)
			}
		}
	}
	return images
}

func isolatedVolumes(project *compose.ComposeProject, shareList []string) []string {
	shared := map[string]bool{}
	for _, v := range shareList {
		shared[v] = true
	}
	var isolated []string
	for name, vol := range project.Volumes {
		if vol.External && !shared[name] {
			isolated = append(isolated, name)
		}
	}
	slices.Sort(isolated)
	return isolated
}

func humanRenderer() func(io.Writer, any) {
	return func(w io.Writer, data any) {
		switch v := data.(type) {
		case UpResult:
			if v.AlreadyRunning {
				fmt.Fprintf(w, "Docktree %s is already running.\n", v.Instance.ProjectName)
				return
			}
			fmt.Fprintf(w, "Docktree started %s\n", v.Instance.ProjectName)
			for _, assignment := range v.Ports {
				fmt.Fprintf(w, "  %s %s:%d -> %d\n", assignment.Service, assignment.HostIP, assignment.HostPort, assignment.ContainerPort)
			}
			if len(v.IsolatedVolumes) > 0 {
				fmt.Fprintf(w, "  External volumes isolated: %s\n", strings.Join(v.IsolatedVolumes, ", "))
				fmt.Fprintf(w, "  To share a volume across worktrees, add to docktree.yml:\n")
				fmt.Fprintf(w, "    volumes:\n")
				fmt.Fprintf(w, "      share:\n")
				for _, vol := range v.IsolatedVolumes {
					fmt.Fprintf(w, "        - %s\n", vol)
				}
			}
			if v.Scaffolded {
				fmt.Fprintln(w, "  Created docktree.yml with defaults")
			}
			for _, warning := range v.EnvWarnings {
				fmt.Fprintf(w, "  Warning: %s\n", warning.Message)
			}
		case DownResult:
			if v.AlreadyStopped {
				fmt.Fprintln(w, "Docktree is already stopped.")
				return
			}
			fmt.Fprintf(w, "Docktree stopped %s\n", v.Instance.ProjectName)
		case StatusResult:
			if v.Stopped {
				fmt.Fprintln(w, "Docktree is stopped.")
				return
			}
			fmt.Fprintln(w, v.Text)
		case PortsResult:
			fmt.Fprintf(w, "Docktree ports for %s\n", v.Instance)
			for _, assignment := range v.Ports {
				fmt.Fprintf(w, "  %s %s:%d -> %d\n", assignment.Service, assignment.HostIP, assignment.HostPort, assignment.ContainerPort)
			}
		case CleanResult:
			if len(v.Instances) == 0 {
				fmt.Fprintln(w, "Docktree found no stale resources.")
				return
			}
			if v.DryRun {
				fmt.Fprintln(w, "Docktree dry run - nothing will be removed")
			} else if v.Removed {
				fmt.Fprintln(w, "Docktree removed stale resources")
			} else {
				fmt.Fprintln(w, "Docktree found stale resources")
			}
			for _, item := range v.Instances {
				fmt.Fprintf(w, "  %s (%s): %d ports, %d containers, %d networks", item.Instance, item.Reason, item.Ports, item.Containers, item.Networks)
				if v.Volumes {
					fmt.Fprintf(w, ", %d volumes", item.Volumes)
				}
				fmt.Fprintln(w)
			}
		default:
			_ = json.NewEncoder(w).Encode(data)
		}
	}
}

func errorCode(code int) string {
	switch code {
	case output.ExitUsage:
		return "usage"
	case output.ExitConfig:
		return "config"
	case output.ExitDocker:
		return "docker"
	case output.ExitNoop:
		return "noop"
	case output.ExitConflict:
		return "conflict"
	default:
		return "error"
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, `docktree coordinates Docker Compose services across git worktrees.

Usage:
  docktree [--json] <command>

Commands:
  up         Start the current worktree's Compose project
  down       Stop the current worktree's Compose project
  status     Show managed worktree services
  ports      Show allocated host ports
  clean      Remove stale Docktree-managed resources
  help       Show this help text
  version    Print the docktree version`)
}

type cleanOptions struct {
	dryRun  bool
	yes     bool
	volumes bool
}

type cleanCandidate struct {
	Name       string
	Reason     string
	Ports      int
	Resources  docker.ProjectResources
	Instance   *state.Instance
	StateFound bool
}

type upOptions struct {
	file string
}

func parseUpOptions(args []string) (upOptions, error) {
	var options upOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-f" || arg == "--file":
			if i+1 >= len(args) {
				return upOptions{}, fmt.Errorf("%s requires a value", arg)
			}
			options.file = args[i+1]
			i++
		case strings.HasPrefix(arg, "--file="):
			options.file = strings.TrimPrefix(arg, "--file=")
		default:
			return upOptions{}, fmt.Errorf("unknown up flag %q", arg)
		}
	}
	return options, nil
}

func parseCleanOptions(args []string) (cleanOptions, error) {
	var options cleanOptions
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			options.dryRun = true
		case "--yes":
			options.yes = true
		case "--volumes":
			options.volumes = true
		default:
			return cleanOptions{}, fmt.Errorf("unknown clean flag %q", arg)
		}
	}
	return options, nil
}

func discoverCleanCandidates(includeVolumes bool) ([]cleanCandidate, error) {
	instances, err := state.LoadGlobalInstances("")
	if err != nil {
		return nil, err
	}
	portRegistry := ports.NewRegistry()
	if err := portRegistry.Lock(); err != nil {
		return nil, err
	}
	defer portRegistry.Unlock()
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

func applyCleanCandidates(candidates []cleanCandidate, includeVolumes bool) error {
	portRegistry := ports.NewRegistry()
	if err := portRegistry.Lock(); err != nil {
		return err
	}
	defer portRegistry.Unlock()
	for _, candidate := range candidates {
		if _, err := docker.RemoveProjectResources(candidate.Name, includeVolumes); err != nil {
			return err
		}
		if err := portRegistry.Release(candidate.Name); err != nil {
			return err
		}
		if err := state.RemoveGlobalInstance("", candidate.Name); err != nil {
			return err
		}
		if candidate.Instance != nil {
			if err := state.RemoveStateDir(candidate.Instance); err != nil {
				return err
			}
		}
	}
	return nil
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

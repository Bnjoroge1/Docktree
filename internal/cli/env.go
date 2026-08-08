package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bnjoroge/docktree/internal/compose"
	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/docker"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/ports"
	"github.com/bnjoroge/docktree/internal/state"
)

// envOptions captures one `docktree env` invocation:
//
//	docktree env set KEY=VALUE [--service <svc>]... [--restart <svc>]...
//	docktree env unset KEY... [--service <svc>]... [--restart <svc>]...
//	docktree env list
type envOptions struct {
	action   string // "set", "unset", or "list"
	help     bool
	pairs    []envPair // set only
	keys     []string  // unset only
	services []string
	restarts []string
}

type envPair struct {
	key   string
	value string
}

func runEnv(ctx *Context) (any, int, error) {
	options, err := parseEnvOptions(ctx.Args[1:])
	if err != nil {
		return nil, output.ExitUsage, err
	}
	if options.help {
		return envHelpDoc(), output.ExitOK, nil
	}

	repo, cfg, instanceName, err := commonIdentity()
	if err != nil {
		return nil, output.ExitConfig, err
	}

	if options.action == "list" {
		return EnvResult{Action: "list", Instance: instanceName, Overrides: envEntries(cfg.Overrides.Environment)}, output.ExitOK, nil
	}

	stateDir := state.StatePath(repo.WorktreeRoot, cfg.State.Directory)
	inst, err := state.LoadInstance(stateDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, output.ExitConfig, fmt.Errorf("no Docktree instance found for this worktree; run `docktree up` first — docktree env regenerates the existing override without reallocating ports")
	}
	if err != nil {
		return nil, output.ExitConfig, err
	}

	// Regenerate from the exact compose files the instance was started with;
	// fall back to docktree.yml discovery for instances recorded before
	// ComposeFiles was persisted.
	files := inst.ComposeFiles
	if len(files) == 0 {
		if files, err = composeFiles(repo.WorktreeRoot, cfg); err != nil {
			return nil, output.ExitConfig, err
		}
	}
	project, err := parseAll(files)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	project = compose.FilterProfiles(project, inst.Profiles)
	if project == nil {
		return nil, output.ExitConfig, fmt.Errorf("failed to filter project profiles")
	}

	knownServices := serviceNames(project)
	slices.Sort(knownServices)
	if err := validateEnvServices(options.services, knownServices, "--service"); err != nil {
		return nil, output.ExitUsage, err
	}
	if err := validateEnvServices(options.restarts, knownServices, "--restart"); err != nil {
		return nil, output.ExitUsage, err
	}

	overridesPath := config.LocalOverridesPath(repo.WorktreeRoot, cfg.State.Directory)
	local, err := config.LoadLocalOverrides(overridesPath)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if local.Environment == nil {
		local.Environment = map[string]map[string]string{}
	}
	if cfg.Overrides.Environment == nil && options.action == "set" {
		cfg.Overrides.Environment = map[string]map[string]string{}
	}

	// Mutate the worktree store and the merged view in lockstep so the
	// regenerated override below matches exactly what was persisted.
	var changed []EnvEntry
	switch options.action {
	case "set":
		targets := options.services
		if len(targets) == 0 {
			targets = knownServices
		}
		for _, pair := range options.pairs {
			applyEnvSet(local.Environment, targets, pair.key, pair.value)
			changed = append(changed, applyEnvSet(cfg.Overrides.Environment, targets, pair.key, pair.value)...)
		}
	case "unset":
		for _, key := range options.keys {
			targets := options.services
			if len(targets) == 0 {
				targets = envServices(local.Environment)
			}
			removed := applyEnvUnset(local.Environment, targets, key)
			applyEnvUnset(cfg.Overrides.Environment, targets, key)
			// Unsetting a key that has no override is a no-op, not an error.
			changed = append(changed, removed...)
		}
	}
	// Drop stored overrides for services that no longer exist in the project.
	pruned := pruneEnvServices(local.Environment, knownServices)
	pruneEnvServices(cfg.Overrides.Environment, knownServices)

	if len(local.Environment) == 0 {
		local.Environment = nil
	}
	if len(changed) == 0 && len(pruned) == 0 {
		return EnvResult{Action: options.action, Instance: instanceName, Overrides: envEntries(cfg.Overrides.Environment), NoChange: true}, output.ExitOK, nil
	}
	if err := config.WriteLocalOverrides(overridesPath, local); err != nil {
		return nil, output.ExitConfig, err
	}

	code, err := regenerateEnvOverride(project, cfg, repo.WorktreeRoot, stateDir, instanceName)
	if err != nil {
		return nil, code, err
	}
	overrideFile := filepath.Join(stateDir, "generated", instanceName+".override.yml")

	var restarted []string
	if len(options.restarts) > 0 {
		runFiles := append([]string{}, files...)
		clearFile := filepath.Join(stateDir, "generated", instanceName+".clear.yml")
		if _, err := os.Stat(clearFile); err == nil {
			runFiles = append(runFiles, clearFile)
		}
		runFiles = append(runFiles, overrideFile)
		upArgs := append([]string{"up", "-d", "--no-deps"}, options.restarts...)
		cmd := docker.ComposeCommand{ProjectName: instanceName, Files: runFiles, Profiles: inst.Profiles, CommandArgs: upArgs}
		dockerStdout := ctx.Stdout
		if ctx.Renderer.JSON {
			dockerStdout = io.Discard
		}
		if err := docker.Run(cmd, nil, dockerStdout, ctx.Stderr); err != nil {
			return nil, output.ExitDocker, err
		}
		restarted = options.restarts
	}

	return EnvResult{
		Action:        options.action,
		Instance:      instanceName,
		Changed:       changed,
		Overrides:     envEntries(cfg.Overrides.Environment),
		Restarted:     restarted,
		OverrideFile:  overrideFile,
		OverridesFile: overridesPath,
	}, output.ExitOK, nil
}

// regenerateEnvOverride rebuilds the generated compose override using the
// ports already allocated to this instance. It never reallocates: without
// existing assignments the stack has never been up, so there is nothing to
// re-render honestly.
func regenerateEnvOverride(project *compose.ComposeProject, cfg *config.Config, worktreeRoot, stateDir, instanceName string) (int, error) {
	registry := ports.NewRegistry()
	assignments, ok, err := registry.ExistingAssignments(instanceName, portRequests(project, cfg.Ports.BindHost))
	if err != nil {
		return output.ExitConflict, err
	}
	if !ok {
		return output.ExitConflict, fmt.Errorf("no existing port assignments for instance %q; run `docktree up` first", instanceName)
	}
	override, err := compose.GenerateOverride(project, instanceName, assignments, repoRootVolumesShare())
	if err != nil {
		return output.ExitConfig, err
	}
	compose.ApplyEnvOverrides(override, cfg.Overrides.Environment)
	overrideFile := filepath.Join(stateDir, "generated", instanceName+".override.yml")
	if err := compose.WriteOverride(override, overrideFile); err != nil {
		return output.ExitConfig, err
	}
	return output.ExitOK, nil
}

func validateEnvServices(requested, known []string, flag string) error {
	set := make(map[string]bool, len(known))
	for _, name := range known {
		set[name] = true
	}
	for _, name := range requested {
		if !set[name] {
			return fmt.Errorf("%s: unknown service %q; this project has: %s", flag, name, strings.Join(known, ", "))
		}
	}
	return nil
}

// ---- pure overrides-store helpers -------------------------------------------

// envEntries flattens the store into a deterministic service,key-sorted list.
// The result is always non-nil so --json renders [] instead of null.
func envEntries(env map[string]map[string]string) []EnvEntry {
	entries := make([]EnvEntry, 0)
	for _, service := range sortedEnvServices(env) {
		vars := env[service]
		keys := make([]string, 0, len(vars))
		for key := range vars {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			entries = append(entries, EnvEntry{Service: service, Key: key, Value: vars[key]})
		}
	}
	return entries
}

// envServices lists services that currently have at least one override.
func envServices(env map[string]map[string]string) []string {
	return sortedEnvServices(env)
}

func sortedEnvServices(env map[string]map[string]string) []string {
	services := make([]string, 0, len(env))
	for service, vars := range env {
		if len(vars) == 0 {
			continue
		}
		services = append(services, service)
	}
	slices.Sort(services)
	return services
}

// applyEnvSet records key=value for each target service and returns the
// effective changes.
func applyEnvSet(env map[string]map[string]string, services []string, key, value string) []EnvEntry {
	changed := make([]EnvEntry, 0, len(services))
	seen := map[string]bool{}
	for _, service := range services {
		if seen[service] {
			continue
		}
		seen[service] = true
		vars := env[service]
		if vars == nil {
			vars = map[string]string{}
			env[service] = vars
		}
		vars[key] = value
		changed = append(changed, EnvEntry{Service: service, Key: key, Value: value})
	}
	return changed
}

// applyEnvUnset removes key from each target service, dropping service maps
// left empty, and returns what was actually removed.
func applyEnvUnset(env map[string]map[string]string, services []string, key string) []EnvEntry {
	var removed []EnvEntry
	seen := map[string]bool{}
	for _, service := range services {
		if seen[service] {
			continue
		}
		seen[service] = true
		vars := env[service]
		if vars == nil {
			continue
		}
		value, ok := vars[key]
		if !ok {
			continue
		}
		delete(vars, key)
		if len(vars) == 0 {
			delete(env, service)
		}
		removed = append(removed, EnvEntry{Service: service, Key: key, Value: value})
	}
	return removed
}

// pruneEnvServices drops stored overrides for services no longer in the
// project (renamed or deleted), and returns the pruned service names.
func pruneEnvServices(env map[string]map[string]string, known []string) []string {
	if len(env) == 0 {
		return nil
	}
	set := make(map[string]bool, len(known))
	for _, name := range known {
		set[name] = true
	}
	var pruned []string
	for service := range env {
		if !set[service] {
			pruned = append(pruned, service)
			delete(env, service)
		}
	}
	slices.Sort(pruned)
	return pruned
}

// ---- option parsing ----------------------------------------------------------

func parseEnvOptions(args []string) (envOptions, error) {
	var options envOptions
	actionSeen := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			options.help = true
			return options, nil
		case !actionSeen:
			switch arg {
			case "set", "unset", "list":
				options.action = arg
				actionSeen = true
			default:
				if strings.HasPrefix(arg, "-") {
					return envOptions{}, fmt.Errorf("unknown env flag %q", arg)
				}
				return envOptions{}, fmt.Errorf("unknown env action %q (expected set, unset, or list)", arg)
			}
		case arg == "--service" || arg == "--restart":
			if i+1 >= len(args) || args[i+1] == "" || strings.HasPrefix(args[i+1], "-") {
				return envOptions{}, fmt.Errorf("%s requires a service name", arg)
			}
			options.addServiceFlag(arg, args[i+1])
			i++
		case strings.HasPrefix(arg, "--service="):
			service := strings.TrimPrefix(arg, "--service=")
			if service == "" {
				return envOptions{}, fmt.Errorf("--service requires a service name")
			}
			options.services = append(options.services, service)
		case strings.HasPrefix(arg, "--restart="):
			service := strings.TrimPrefix(arg, "--restart=")
			if service == "" {
				return envOptions{}, fmt.Errorf("--restart requires a service name")
			}
			options.restarts = append(options.restarts, service)
		default:
			if strings.HasPrefix(arg, "-") {
				return envOptions{}, fmt.Errorf("unknown env flag %q", arg)
			}
			if err := options.addPositional(arg); err != nil {
				return envOptions{}, err
			}
		}
	}
	if !actionSeen {
		options.help = true
		return options, nil
	}
	return options, options.validate()
}

func (o *envOptions) addServiceFlag(flag, service string) {
	if flag == "--service" {
		o.services = append(o.services, service)
	} else {
		o.restarts = append(o.restarts, service)
	}
}

func (o *envOptions) addPositional(arg string) error {
	switch o.action {
	case "set":
		key, value, found := strings.Cut(arg, "=")
		if !found || key == "" {
			return fmt.Errorf("env set takes KEY=VALUE pairs, got %q", arg)
		}
		o.pairs = append(o.pairs, envPair{key: key, value: value})
	case "unset":
		if strings.Contains(arg, "=") {
			return fmt.Errorf("env unset takes variable names (KEY), not KEY=VALUE pairs, got %q", arg)
		}
		o.keys = append(o.keys, arg)
	case "list":
		return fmt.Errorf("env list takes no arguments, got %q", arg)
	}
	return nil
}

func (o envOptions) validate() error {
	switch o.action {
	case "set":
		if len(o.pairs) == 0 {
			return fmt.Errorf("env set requires at least one KEY=VALUE pair")
		}
	case "unset":
		if len(o.keys) == 0 {
			return fmt.Errorf("env unset requires at least one KEY")
		}
	case "list":
		if len(o.services) > 0 || len(o.restarts) > 0 {
			return fmt.Errorf("env list does not accept --service or --restart")
		}
	}
	return nil
}

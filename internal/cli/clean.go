package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/bnjoroge/docktree/internal/docker"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/ports"
	"github.com/bnjoroge/docktree/internal/state"
	"github.com/bnjoroge/docktree/internal/tui"
)

func runClean(ctx *Context) (any, int, error) {
	options, err := parseCleanOptions(ctx.Args[1:])
	if err != nil {
		return nil, output.ExitUsage, err
	}
	portRegistry := ports.NewRegistry()
	if err := portRegistry.Lock(); err != nil {
		return nil, output.ExitDocker, err
	}
	candidates, err := discoverCleanCandidates(portRegistry, options.volumes)
	unlockErr := portRegistry.Unlock()
	if err != nil {
		return nil, output.ExitDocker, err
	}
	if unlockErr != nil {
		return nil, output.ExitDocker, unlockErr
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
	var applied []cleanCandidate
	if !ctx.Renderer.IsTTY || ctx.Renderer.JSON {
		applied, err = applyCleanCandidates(portRegistry, candidates, options.volumes)
	} else {
		done := make(chan struct{})
		go func() {
			applied, err = applyCleanCandidates(portRegistry, candidates, options.volumes)
			close(done)
		}()
		spinner := &tui.SimpleSpinner{}
		spinner.Start("Removing stale resources…")
		<-done
		spinner.Stop()
	}
	if err != nil {
		return nil, output.ExitDocker, err
	}
	return cleanResultFromCandidates(applied, false, options.volumes, true), output.ExitOK, nil
}

func discoverCleanCandidates(portRegistry *ports.Registry, includeVolumes bool) ([]cleanCandidate, error) {
	instances, err := state.LoadGlobalInstances("")
	if err != nil {
		return nil, err
	}
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
		candidates = append(candidates, cleanCandidate{Name: name, Reason: reason, Ports: len(portMap[name]), Resources: resources, Instance: inst, StateFound: stateFound})
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
		item := CleanItem{Instance: candidate.Name, Reason: candidate.Reason, Ports: candidate.Ports, Containers: len(candidate.Resources.Containers), Networks: len(candidate.Resources.Networks), Volumes: len(candidate.Resources.Volumes)}
		result.Instances = append(result.Instances, item)
		result.Totals.Instances++
		result.Totals.Ports += item.Ports
		result.Totals.Containers += item.Containers
		result.Totals.Networks += item.Networks
		result.Totals.Volumes += item.Volumes
	}
	return result
}

func applyCleanCandidates(portRegistry *ports.Registry, candidates []cleanCandidate, includeVolumes bool) ([]cleanCandidate, error) {
	if err := portRegistry.Lock(); err != nil {
		return nil, err
	}
	portMap, err := portRegistry.Load()
	if err != nil {
		_ = portRegistry.Unlock()
		return nil, err
	}
	instances, err := state.LoadGlobalInstances("")
	if err != nil {
		_ = portRegistry.Unlock()
		return nil, err
	}
	var applied []cleanCandidate
	for _, candidate := range candidates {
		currentCandidate := candidate
		currentCandidate.Ports = len(portMap[candidate.Name])
		if saved, ok := instances[candidate.Name]; ok {
			copied := saved
			currentCandidate.Instance = &copied
			currentCandidate.StateFound = true
		} else {
			currentCandidate.Instance = nil
			currentCandidate.StateFound = false
		}
		if staleReason(currentCandidate.Instance, currentCandidate.StateFound, currentCandidate.Ports, currentCandidate.Resources) == "" {
			continue
		}
		if err := portRegistry.Release(candidate.Name); err != nil {
			_ = portRegistry.Unlock()
			return nil, err
		}
		if err := state.RemoveGlobalInstance("", candidate.Name); err != nil {
			_ = portRegistry.Unlock()
			return nil, err
		}
		applied = append(applied, currentCandidate)
	}
	if err := portRegistry.Unlock(); err != nil {
		return nil, err
	}
	for _, candidate := range applied {
		if _, err := docker.RemoveProjectResources(candidate.Name, includeVolumes); err != nil {
			return nil, err
		}
		if candidate.Instance != nil {
			if err := state.RemoveStateDir(candidate.Instance); err != nil {
				return nil, err
			}
		}
	}
	return applied, nil
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

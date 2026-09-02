package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
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
	if options.help {
		return cleanHelpDoc(), output.ExitOK, nil
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
		ctx.Renderer.Render(result, humanRenderer())
		fmt.Fprintln(ctx.Stdout)
		if !confirmClean(ctx.Stdout) {
			return nil, output.ExitNoop, nil
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
	// Resource-rooted fallback: even when state, port claims, and labelled
	// containers are all gone (e.g. after a partially failed teardown), the
	// networks and volumes Docktree generated still carry its
	// `docktree.instance` ownership label. docker.ListDocktree{Networks,Volumes}
	// only report resources bearing that label and exclude the platform tier,
	// so foreign compose projects and the shared platform stack are never
	// swept; the instance-name shape check below is a defensive second gate.
	// Volume-rooted candidates are only added when volumes will actually be
	// removed, so a volume-only orphan is not re-reported forever by a run
	// without --volumes.
	if includeVolumes {
		volumes, err := docker.ListDocktreeVolumes()
		if err != nil {
			return nil, err
		}
		for _, vol := range volumes {
			if isWorktreeInstanceName(vol.ProjectName) {
				names[vol.ProjectName] = true
			}
		}
	}
	networks, err := docker.ListDocktreeNetworks()
	if err != nil {
		return nil, err
	}
	for _, net := range networks {
		if isWorktreeInstanceName(net.ProjectName) {
			names[net.ProjectName] = true
		}
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

// docktreeNameRe matches Docktree instance names: <repo>-<worktree>-<6 hex
// chars>. It is a defensive shape sanity check layered on top of the
// label-based ownership guarantee in docker.ListDocktree* — anything that
// does not match the full three-segment shape ending in the 6-char path hash
// is left alone.
var docktreeNameRe = regexp.MustCompile(`^[a-z0-9_-]+-[a-z0-9_-]+-[a-f0-9]{6}$`)

// isWorktreeInstanceName reports whether a project name matches the Docktree
// worktree-instance shape. Platform-tier exclusion is handled by label at the
// docker listing layer (docktree.tier=platform), not by name prefix, so a
// legitimate worktree instance whose repo/worktree slugs render as
// "docktree-platform-<hash>" is still recognized here.
func isWorktreeInstanceName(name string) bool {
	return docktreeNameRe.MatchString(name)
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
	var applied []cleanCandidate
	var errs []error
	for _, candidate := range candidates {
		result, err := applyCleanCandidate(portRegistry, candidate, includeVolumes)
		if err != nil {
			errs = append(errs, err)
		}
		if result != nil {
			applied = append(applied, *result)
		}
	}
	return applied, errors.Join(errs...)
}

// applyCleanCandidate tears down a single candidate. The port-registry lock is
// held for the whole candidate so a concurrent `up` cannot allocate this
// instance's ports mid-teardown, and the candidate's staleness is re-checked
// against fresh state under that lock so an `up` that finished registering the
// instance since discovery is left untouched. Docker resources are removed
// before any tracking metadata is retired; if removal fails the claim and
// record stay behind so a later `clean` can rediscover and retry the orphan.
// A returned candidate was fully retired; a nil candidate with a nil error was
// intentionally skipped (no longer stale, or deferred to a `--volumes` run).
func applyCleanCandidate(portRegistry *ports.Registry, candidate cleanCandidate, includeVolumes bool) (*cleanCandidate, error) {
	if err := portRegistry.Lock(); err != nil {
		return nil, fmt.Errorf("%s: lock port registry: %w", candidate.Name, err)
	}
	unlocked := false
	unlock := func() error {
		if unlocked {
			return nil
		}
		unlocked = true
		return portRegistry.Unlock()
	}
	defer func() { _ = unlock() }()

	portMap, err := portRegistry.Load()
	if err != nil {
		return nil, fmt.Errorf("%s: load port registry: %w", candidate.Name, err)
	}
	instances, err := state.LoadGlobalInstances("")
	if err != nil {
		return nil, fmt.Errorf("%s: load global instances: %w", candidate.Name, err)
	}
	candidate.Ports = len(portMap[candidate.Name])
	if saved, ok := instances[candidate.Name]; ok {
		copied := saved
		candidate.Instance = &copied
		candidate.StateFound = true
	} else {
		candidate.Instance = nil
		candidate.StateFound = false
	}
	// Re-validate against fresh state: a concurrent `up` may have reclaimed
	// this instance since discovery, in which case it is no longer stale.
	if staleReason(candidate.Instance, candidate.StateFound, candidate.Ports, candidate.Resources) == "" {
		return nil, nil
	}

	removed, err := docker.RemoveProjectResources(candidate.Name, includeVolumes)
	if err != nil {
		// Resources remain; keep the claim and record so the orphan stays
		// recoverable on a later run.
		return nil, fmt.Errorf("%s: %w", candidate.Name, err)
	}
	candidate.Resources = removed

	// A plain `clean` neither lists nor removes volumes. If this project still
	// owns volumes, retiring its claim and record now would strand them as
	// permanently unreachable orphans, so keep the tracking metadata and let a
	// later `clean --volumes` finish the teardown.
	if !includeVolumes {
		residual, err := docker.ListProjectResources(candidate.Name, true)
		if err != nil {
			return nil, fmt.Errorf("%s: check residual volumes: %w", candidate.Name, err)
		}
		if len(residual.Volumes) > 0 {
			return nil, nil
		}
	}

	// Retire the global state record first. If it fails, keep the port claim
	// so the candidate stays discoverable ("stale port allocations") and is
	// retried; releasing the ports here would leave a resource-free record
	// that looks healthy and is never revisited.
	if err := state.RemoveGlobalInstance("", candidate.Name); err != nil {
		return nil, fmt.Errorf("%s: remove global instance: %w", candidate.Name, err)
	}

	var errs []error
	if err := portRegistry.Release(candidate.Name); err != nil {
		errs = append(errs, fmt.Errorf("%s: release allocated ports: %w", candidate.Name, err))
	}
	if err := unlock(); err != nil {
		errs = append(errs, fmt.Errorf("%s: unlock port registry: %w", candidate.Name, err))
	}
	if candidate.Instance != nil {
		if err := state.RemoveStateDir(candidate.Instance); err != nil {
			errs = append(errs, fmt.Errorf("%s: remove state directory: %w", candidate.Name, err))
		}
	}
	return &candidate, errors.Join(errs...)
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

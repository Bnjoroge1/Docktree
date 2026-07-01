package compose

import (
	"fmt"
	"sort"
	"strings"

	composetypes "github.com/compose-spec/compose-go/v2/types"
)

// ServiceFilter selects which services to remove from a compose project and
// which dependency edges to prune.
type ServiceFilter struct {
	Skip     []string
	DropDeps []string
}

// FilterServices creates a new compose project with the requested services removed
// and depends_on references to skipped/dropped services pruned. It returns an error
// if any of the named services do not exist in the project.
func FilterServices(raw *composetypes.Project, filter ServiceFilter) (*composetypes.Project, error) {
	if raw == nil {
		return nil, fmt.Errorf("nil compose project")
	}

	skipSet := make(map[string]bool, len(filter.Skip))
	for _, name := range filter.Skip {
		if _, ok := raw.Services[name]; !ok {
			return nil, fmt.Errorf("skip_services: unknown service %q", name)
		}
		skipSet[name] = true
	}

	dropSet := make(map[string]bool, len(filter.DropDeps))
	for _, name := range filter.DropDeps {
		if _, ok := raw.Services[name]; !ok {
			return nil, fmt.Errorf("drop_dependencies: unknown service %q", name)
		}
		dropSet[name] = true
	}

	// Skipped services are removed entirely, so any depends_on edge pointing to
	// them must also be dropped. Normalize dropSet to include skipSet.
	for name := range skipSet {
		dropSet[name] = true
	}

	out := &composetypes.Project{
		Name:        raw.Name,
		Services:    composetypes.Services{},
		Networks:    raw.Networks,
		Volumes:     raw.Volumes,
		Secrets:     raw.Secrets,
		Configs:     raw.Configs,
		Models:      raw.Models,
		Extensions:  raw.Extensions,
		WorkingDir:  raw.WorkingDir,
		Environment: raw.Environment,
	}

	for name, svc := range raw.Services {
		if skipSet[name] {
			continue
		}
		clone := svc
		if len(clone.DependsOn) > 0 {
			pruned := composetypes.DependsOnConfig{}
			for depName, depCfg := range clone.DependsOn {
				if dropSet[depName] {
					continue
				}
				pruned[depName] = depCfg
			}
			clone.DependsOn = pruned
		}
		out.Services[name] = clone
	}

	return out, nil
}

// String returns a deterministic log/render summary of the filter.
func (f ServiceFilter) String() string {
	var parts []string
	if len(f.Skip) > 0 {
		parts = append(parts, "skip="+strings.Join(sortedSlice(f.Skip), ","))
	}
	if len(f.DropDeps) > 0 {
		parts = append(parts, "drop_deps="+strings.Join(sortedSlice(f.DropDeps), ","))
	}
	return strings.Join(parts, " ")
}

// FilterProfiles returns a project containing only services without a profile,
// or with a profile that matches the active list.
func FilterProfiles(raw *composetypes.Project, activeProfiles []string) *composetypes.Project {
	if raw == nil {
		return nil
	}
	active := make(map[string]bool, len(activeProfiles))
	for _, p := range activeProfiles {
		active[p] = true
	}
	err := raw.WithServices(activeProfiles)
	if err != nil {
		return raw // fallback
	}
	return raw
}

func sortedSlice(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

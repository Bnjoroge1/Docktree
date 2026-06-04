package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/bnjoroge/docktree/internal/config"
	composetypes "github.com/compose-spec/compose-go/v2/types"
)

// PlatformComposeProject is a type alias so callers outside this package can
// refer to the synthesized platform project without importing compose-go.
type PlatformComposeProject = composetypes.Project

// PlatformProjectName returns the docker-compose project name for the
// repo-scoped platform tier. One platform stack per repo.
func PlatformProjectName(repoSlug string) string {
	return "docktree-platform-" + repoSlug
}

// PlatformNetworkName returns the external docker network name worktree
// services join to reach platform services via DNS aliases.
func PlatformNetworkName(repoSlug string) string {
	return "docktree-platform-" + repoSlug + "-net"
}

// SynthesizePlatform returns a compose Project containing only the
// shared services, configured for the repo-scoped platform stack.
//
// Each kept service:
//   - is attached to the platform external network with its compose-service
//     name (plus any configured aliases) as DNS aliases
//   - is stripped of host-port publishes (platform services are reached over
//     the platform network from worktree containers, not from the host)
//   - is stripped of the worktree-only depends_on edges (those services
//     aren't in this project)
//   - carries discovery labels: docktree.managed, docktree.tier=platform,
//     docktree.repo=<slug>, docktree.shared.kind=<kind>
//
// Volumes are preserved if any kept service uses them. Other top-level
// volumes/networks/secrets/configs are dropped.
func SynthesizePlatform(raw *composetypes.Project, shared config.SharedConfig, repoSlug string) (*composetypes.Project, error) {
	if raw == nil {
		return nil, fmt.Errorf("nil compose project")
	}
	if len(shared.Services) == 0 {
		return nil, fmt.Errorf("no shared services declared")
	}
	if repoSlug == "" {
		return nil, fmt.Errorf("empty repo slug")
	}

	netName := PlatformNetworkName(repoSlug)
	projectName := PlatformProjectName(repoSlug)

	out := &composetypes.Project{
		Name:     projectName,
		Services: composetypes.Services{},
		Networks: composetypes.Networks{},
		Volumes:  composetypes.Volumes{},
	}

	usedVolumes := map[string]bool{}
	kept := map[string]bool{}

	for name, svc := range raw.Services {
		decl, isShared := shared.Services[name]
		if !isShared {
			continue
		}
		kept[name] = true
		clone := svc
		// Strip host-bound things — platform services talk over the
		// repo network only.
		clone.Ports = nil
		clone.DependsOn = nil
		// Replace networks with the platform network + aliases.
		aliases := append([]string{name}, decl.Aliases...)
		clone.Networks = map[string]*composetypes.ServiceNetworkConfig{
			netName: {Aliases: aliases},
		}
		// Container name: pin to a deterministic platform-scoped name so
		// repeated `platform up` calls don't collide with worktree
		// containers. compose's default scheme would prefix with project
		// name anyway; we override to keep labels readable.
		clone.ContainerName = projectName + "-" + name
		clone.Labels = mergeLabels(clone.Labels, map[string]string{
			"docktree.managed":      "true",
			"docktree.tier":         "platform",
			"docktree.repo":         repoSlug,
			"docktree.shared.kind":  decl.Kind,
			"docktree.shared.name":  name,
		})
		// Track volume references so we keep their top-level declaration.
		for _, vol := range clone.Volumes {
			if vol.Type == "volume" && vol.Source != "" {
				usedVolumes[vol.Source] = true
			}
		}
		out.Services[name] = clone
	}

	if len(out.Services) == 0 {
		return nil, fmt.Errorf("no services in user compose match shared.services declaration")
	}

	// Single platform network — external (created out-of-band by platform up
	// or already by another worktree's earlier `up`).
	out.Networks[netName] = composetypes.NetworkConfig{
		Name:     netName,
		External: composetypes.External(true),
	}

	for volName, vol := range raw.Volumes {
		if !usedVolumes[volName] {
			continue
		}
		clone := vol
		// Keep the volume scoped to this platform project — let compose
		// prefix it with the project name as usual; do not pin Name=
		// explicitly so two repos' platforms don't collide on a shared
		// volume name.
		clone.Name = ""
		clone.External = composetypes.External(false)
		out.Volumes[volName] = clone
	}

	return out, nil
}

// SynthesizeWorktree returns a compose Project with shared services removed,
// depends_on edges pointing at them stripped, and the platform external
// network declared so remaining services can reach the platform tier by DNS.
//
// Every remaining service is attached to the platform network so its DNS
// resolves the shared service hostnames without per-service env scanning.
// (Cheap to do, harder to get wrong, matches the plan's "DNS just works".)
//
// If no service references the platform tier (no depends_on, no envs), the
// returned project still works — the network membership is a no-op for those
// services.
// SynthesizeWorktreeOptions carries optional parameters for worktree synthesis
// that vary per-worktree (tenant names, etc.).
type SynthesizeWorktreeOptions struct {
	// TenantDBs maps shared service name → per-worktree database name.
	// When provided and a service has url_envs declared, those env vars in
	// worktree services are rewritten to use the tenant database name.
	TenantDBs map[string]string
}

func SynthesizeWorktree(raw *composetypes.Project, shared config.SharedConfig, repoSlug string, opts ...SynthesizeWorktreeOptions) (*composetypes.Project, error) {
	if raw == nil {
		return nil, fmt.Errorf("nil compose project")
	}
	if repoSlug == "" {
		return nil, fmt.Errorf("empty repo slug")
	}

	out := &composetypes.Project{
		Name:        raw.Name,
		Services:    composetypes.Services{},
		Networks:    composetypes.Networks{},
		Volumes:     composetypes.Volumes{},
		Secrets:     raw.Secrets,
		Configs:     raw.Configs,
		WorkingDir:  raw.WorkingDir,
		Environment: raw.Environment,
	}

	// No shared services? Round-trip the project unchanged so callers can
	// always use the synthesized file uniformly.
	if len(shared.Services) == 0 {
		for name, svc := range raw.Services {
			out.Services[name] = svc
		}
		for name, net := range raw.Networks {
			out.Networks[name] = net
		}
		for name, vol := range raw.Volumes {
			out.Volumes[name] = vol
		}
		return out, nil
	}

	var opt SynthesizeWorktreeOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	netName := PlatformNetworkName(repoSlug)
	platformSet := map[string]bool{}
	for name := range shared.Services {
		platformSet[name] = true
	}

	for name, svc := range raw.Services {
		if platformSet[name] {
			continue
		}
		clone := svc
		// Strip depends_on edges to platform services. Compose would
		// fail-fast otherwise because the target service doesn't exist
		// in this project.
		if len(clone.DependsOn) > 0 {
			pruned := composetypes.DependsOnConfig{}
			for depName, depCfg := range clone.DependsOn {
				if platformSet[depName] {
					continue
				}
				pruned[depName] = depCfg
			}
			clone.DependsOn = pruned
		}
		// Attach to the platform network in addition to its existing
		// networks. compose's default behaviour gives services that
		// declare no networks the "default" network; once we add an
		// explicit network we have to re-add default ourselves or the
		// service loses it. So: preserve existing entries, add platform.
		if clone.Networks == nil {
			clone.Networks = map[string]*composetypes.ServiceNetworkConfig{
				"default": nil,
			}
		}
		clone.Networks[netName] = nil
		// Rewrite declared URL env vars to point at the per-worktree tenant DB.
		if len(clone.Environment) > 0 && opt.TenantDBs != nil {
			for svcName, svcDecl := range shared.Services {
				if len(svcDecl.URLEnvs) == 0 || svcDecl.Tenancy != "per_database" {
					continue
				}
				tenantDB, ok := opt.TenantDBs[svcName]
				if !ok || tenantDB == "" {
					continue
				}
				// clone.Environment is MappingWithEquals; convert to plain map,
				// rewrite, convert back.
				plain := make(map[string]string, len(clone.Environment))
				for k, v := range clone.Environment {
					if v != nil {
						plain[k] = *v
					}
				}
				rewritten, _ := RewriteURLEnvs(plain, svcDecl.URLEnvs, tenantDB)
				for k, v := range rewritten {
					v := v // capture
					if _, exists := clone.Environment[k]; exists {
						clone.Environment[k] = &v
					}
				}
			}
		}
		out.Services[name] = clone
	}

	// Preserve user-declared networks/volumes; drop only those tied
	// exclusively to removed platform services. Heuristic in v1: keep
	// everything the user declared, since removing a network/volume the
	// user listed could break things subtly. If platform services
	// referenced a top-level volume, the worktree services may still
	// reference it (e.g. shared seed data) — let compose error out
	// rather than silently dropping.
	for name, net := range raw.Networks {
		out.Networks[name] = net
	}
	for name, vol := range raw.Volumes {
		out.Volumes[name] = vol
	}
	// Make sure compose's implicit default network exists explicitly,
	// because we touched ServiceConfig.Networks above (which suppresses
	// the implicit default).
	if _, ok := out.Networks["default"]; !ok {
		out.Networks["default"] = composetypes.NetworkConfig{}
	}
	// Declare the platform network as external — it is created by
	// platform up (or pre-existing).
	out.Networks[netName] = composetypes.NetworkConfig{
		Name:     netName,
		External: composetypes.External(true),
	}
	return out, nil
}

// WriteComposeFile marshals a compose Project to disk via compose-go's
// canonical marshaller. Parent directory is created if missing.
func WriteComposeFile(project *composetypes.Project, path string) error {
	if project == nil {
		return fmt.Errorf("nil compose project")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := project.MarshalYAML()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func mergeLabels(base, additions map[string]string) map[string]string {
	merged := map[string]string{}
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range additions {
		merged[k] = v
	}
	return merged
}

// SortedServiceNames returns the service names of a project in stable order,
// helpful for deterministic test assertions and human output.
func SortedServiceNames(p *composetypes.Project) []string {
	names := make([]string, 0, len(p.Services))
	for name := range p.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

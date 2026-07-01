package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bnjoroge/docktree/internal/config"
	composetypes "github.com/compose-spec/compose-go/v2/types"
)

// PlatformComposeProject is a type alias so callers outside this package can
// refer to the synthesized platform project without importing compose-go.
type PlatformComposeProject = composetypes.Project

// PlatformProjectName returns the docker-compose project name for the
// repo-scoped platform tier. One platform stack per repo.
func PlatformProjectName(repoSlug string) string {
	return "docktree-platform-" + normalizeComposeToken(repoSlug)
}

// PlatformNetworkName returns the external docker network name worktree
// services join to reach platform services via DNS aliases.
func PlatformNetworkName(repoSlug string) string {
	return "docktree-platform-" + normalizeComposeToken(repoSlug) + "-net"
}

func normalizeComposeToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "repo"
	}
	var b strings.Builder
	b.Grow(len(value))
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_':
			if b.Len() == 0 || lastDash {
				continue
			}
			b.WriteRune('-')
			lastDash = true
		default:
			if b.Len() == 0 || lastDash {
				continue
			}
			b.WriteRune('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-_")
	if out == "" {
		return "repo"
	}
	return out
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
		return nil, fmt.Errorf("no compose project")
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
		Name:       projectName,
		Services:   composetypes.Services{},
		Networks:   composetypes.Networks{},
		Volumes:    composetypes.Volumes{},
		WorkingDir: raw.WorkingDir,
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
		// platform services shouldnt talk to the internet
		clone.Ports = nil
		clone.DependsOn = nil
		// Replace networks with the platform network + aliases.
		aliases := append([]string{name}, decl.Aliases...)
		clone.Networks = map[string]*composetypes.ServiceNetworkConfig{
			netName: {Aliases: aliases},
		}
		// only reason we override is to keep the labels stable/
		clone.ContainerName = projectName + "-" + name
		clone.Labels = mergeLabels(clone.Labels, map[string]string{
			"docktree.managed":     "true",
			"docktree.tier":        "platform",
			"docktree.repo":        repoSlug,
			"docktree.shared.kind": decl.Kind,
			"docktree.shared.name": name,
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

	out.Networks[netName] = composetypes.NetworkConfig{
		Name:     netName,
		External: composetypes.External(true),
	}

	for volName, vol := range raw.Volumes {
		if !usedVolumes[volName] {
			continue
		}
		clone := vol
		// docker compose will autoamatically create a unique name for this.
		clone.Name = ""
		clone.External = composetypes.External(false)
		out.Volumes[volName] = clone
	}

	return out, nil
}

// SynthesizeWorktreeOptions carries per-worktree tenant database names.
type SynthesizeWorktreeOptions struct {
	// TenantDBs maps shared service name -> logical database key -> per-worktree
	// tenant database name. The empty logical database key preserves the legacy
	// single-database per service model.
	TenantDBs map[string]map[string]string
	// RawInput marks projects loaded with SkipInterpolation. Raw projects
	// already contain the user's compose syntax exactly as written and must not
	// have dollar signs escaped again before serialization.
	RawInput bool
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
	escapeDollars := !opt.RawInput

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
		// Deep-copy Environment so escaping does not mutate the original
		// project's map (clone := svc is a shallow copy; Go maps are refs).
		if len(svc.Environment) > 0 {
			clone.Environment = make(map[string]*string, len(svc.Environment))
			for k, v := range svc.Environment {
				if v != nil {
					value := *v
					if escapeDollars {
						value = escapeDollar(value)
					}
					clone.Environment[k] = &value
				} else {
					clone.Environment[k] = nil
				}
			}
		}
		if escapeDollars && len(clone.Command) > 0 {
			clone.Command = escapeCommand(clone.Command)
		}
		if escapeDollars && len(clone.Entrypoint) > 0 {
			clone.Entrypoint = escapeCommand(clone.Entrypoint)
		}
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
		// networks.  we have to re-add default ourselves or the service loses it.
		if clone.Networks == nil {
			clone.Networks = map[string]*composetypes.ServiceNetworkConfig{
				"default": nil,
			}
		}
		clone.Networks[netName] = nil

		if len(clone.Environment) > 0 && opt.TenantDBs != nil {
			plain := make(map[string]string, len(clone.Environment))
			for k, v := range clone.Environment {
				if v != nil {
					plain[k] = *v
				}
			}
			for svcName, svcDecl := range shared.Services {
				logicalDBs, ok := opt.TenantDBs[svcName]
				if !ok {
					continue
				}
				for logicalName, dbDecl := range svcDecl.DatabaseTargets() {
					tenantDB := logicalDBs[logicalName]
					if tenantDB == "" || len(dbDecl.URLEnvs) == 0 {
						continue
					}
					urlEnvs := dbDecl.URLEnvs
					if opt.RawInput {
						urlEnvs = rewriteableRawURLEnvs(plain, dbDecl.URLEnvs)
					}
					if len(urlEnvs) == 0 {
						continue
					}
					rewritten, err := RewriteURLEnvs(plain, urlEnvs, tenantDB)
					if err != nil {
						return nil, err
					}
					plain = rewritten
				}
			}
			for k, v := range plain {
				if _, exists := clone.Environment[k]; exists {
					value := v
					clone.Environment[k] = &value
				}
			}
		}

		// Inject raw tenant database names for services that construct their
		// connection URL at runtime inside a shell command (e.g. wrapped by a
		// secrets manager like infisical). url_envs cannot reach those because
		// the URL is assembled after the container starts. db_name_envs targets
		// just the database-name variable (e.g. POSTGRES_DB), which the shell
		// command already references via $$POSTGRES_DB. Credentials continue to
		// come from the secrets manager; docktree only overrides the DB name.
		if opt.TenantDBs != nil {
			for svcName, svcDecl := range shared.Services {
				logicalDBs, ok := opt.TenantDBs[svcName]
				if !ok {
					continue
				}
				for logicalName, dbDecl := range svcDecl.DatabaseTargets() {
					if len(dbDecl.DBNameEnvs) == 0 {
						continue
					}
					tenantDB := logicalDBs[logicalName]
					if tenantDB == "" {
						continue
					}
					if clone.Environment == nil {
						clone.Environment = composetypes.MappingWithEquals{}
					}
					for _, envName := range dbDecl.DBNameEnvs {
						db := tenantDB
						clone.Environment[envName] = &db
					}
				}
			}
		}
		out.Services[name] = clone
	}

	// Preserve user-declared networks/volumes; drop only those tied
	// exclusively to removed platform services.
	for name, net := range raw.Networks {
		out.Networks[name] = net
	}
	for name, vol := range raw.Volumes {
		out.Volumes[name] = vol
	}
	// Make sure compose's implicit default network exists explicitly,
	// because we touched ServiceConfig.Networks above (which suppresses he implicit default).
	if _, ok := out.Networks["default"]; !ok {
		out.Networks["default"] = composetypes.NetworkConfig{}
	}
	out.Networks[netName] = composetypes.NetworkConfig{
		Name:     netName,
		External: composetypes.External(true),
	}
	return out, nil
}

func rewriteableRawURLEnvs(envs map[string]string, urlEnvs []string) []string {
	out := make([]string, 0, len(urlEnvs))
	for _, envName := range urlEnvs {
		value, ok := envs[envName]
		if !ok || strings.Contains(value, "$") {
			continue
		}
		out = append(out, envName)
	}
	return out
}

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

func RebaseEnvFiles(project *composetypes.Project, composeFilePath string) error {
	if project == nil {
		return fmt.Errorf("nil compose project")
	}
	outputDir := filepath.Dir(composeFilePath)
	baseDir := project.WorkingDir
	if baseDir == "" {
		baseDir = outputDir
	}
	for name, svc := range project.Services {
		if len(svc.EnvFiles) == 0 {
			continue
		}
		clone := svc
		clone.EnvFiles = make([]composetypes.EnvFile, len(svc.EnvFiles))
		for i, envFile := range svc.EnvFiles {
			clone.EnvFiles[i] = envFile
			if envFile.Path == "" {
				continue
			}
			abs := envFile.Path
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(baseDir, envFile.Path)
			}
			abs = filepath.Clean(abs)
			rel, err := filepath.Rel(outputDir, abs)
			if err != nil {
				clone.EnvFiles[i].Path = abs
				continue
			}
			clone.EnvFiles[i].Path = filepath.ToSlash(rel)
		}
		project.Services[name] = clone
	}
	return nil
}

func RawURLEnvIsolationWarnings(resolved, raw *composetypes.Project, shared config.SharedConfig) []Warning {
	if resolved == nil || raw == nil || len(shared.Services) == 0 {
		return nil
	}
	platformSet := make(map[string]bool, len(shared.Services))
	urlEnvSet := map[string]bool{}
	for svcName, svc := range shared.Services {
		platformSet[svcName] = true
		for _, db := range svc.DatabaseTargets() {
			if db.Tenancy != "per_database" {
				continue
			}
			for _, envName := range db.URLEnvs {
				urlEnvSet[envName] = true
			}
		}
	}
	if len(urlEnvSet) == 0 {
		return nil
	}

	var warnings []Warning
	for serviceName, resolvedSvc := range resolved.Services {
		if platformSet[serviceName] {
			continue
		}
		rawSvc, ok := raw.Services[serviceName]
		if !ok {
			continue
		}
		for envName := range urlEnvSet {
			if _, resolvedExists := resolvedSvc.Environment[envName]; !resolvedExists {
				continue
			}
			value, rawExists := rawSvc.Environment[envName]
			if !rawExists {
				warnings = append(warnings, rawURLIsolationWarning(serviceName, envName, "is supplied outside explicit environment"))
				continue
			}
			if value == nil || strings.Contains(*value, "$") {
				warnings = append(warnings, rawURLIsolationWarning(serviceName, envName, "is unresolved"))
			}
		}
	}
	return warnings
}

func rawURLIsolationWarning(serviceName, envName, source string) Warning {
	return Warning{
		Key:     "shared.url_envs." + serviceName + "." + envName,
		Message: "per_database url_env " + envName + " for service " + serviceName + " " + source + "; generated compose preserves it to avoid leaking secrets, so Docktree cannot rewrite the tenant database name there. Use db_name_envs or construct the URL at runtime from the injected database name.",
	}
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

func SortedServiceNames(p *composetypes.Project) []string {
	names := make([]string, 0, len(p.Services))
	for name := range p.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func escapeDollar(s string) string {
	if !strings.Contains(s, "$") {
		return s
	}
	return strings.ReplaceAll(s, "$", "$$")
}

func escapeCommand(cmd composetypes.ShellCommand) composetypes.ShellCommand {
	out := make(composetypes.ShellCommand, len(cmd))
	for i, v := range cmd {
		out[i] = escapeDollar(v)
	}
	return out
}

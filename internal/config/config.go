package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the project-level docktree.yml model.
type Config struct {
	Compose    ComposeConfig    `yaml:"compose"`
	Identity   IdentityConfig   `yaml:"identity"`
	Worktrees  WorktreesConfig  `yaml:"worktrees"`
	Setup      SetupConfig      `yaml:"setup"`
	Shared     SharedConfig     `yaml:"shared,omitempty"`
	Volumes    VolumeSetConfig  `yaml:"volumes"`
	Ports      PortsConfig      `yaml:"ports"`
	Transforms TransformConfig  `yaml:"transforms"`
	State      StateConfig      `yaml:"state"`
}

type ComposeConfig struct {
	Files []string `yaml:"files,omitempty"`
}

type IdentityConfig struct {
	ProjectPrefix string `yaml:"project_prefix,omitempty"`
}

type WorktreesConfig struct {
	Root string `yaml:"root,omitempty"`
}

type SetupConfig struct {
	Copy    []string `yaml:"copy,omitempty"`
	Symlink []string `yaml:"symlink,omitempty"`
	Run     []string `yaml:"run,omitempty"`
}

// SharedConfig declares which compose services run in the repo-scoped
// platform tier instead of being duplicated per worktree.
//
// Empty == no platform tier; behaviour is identical to the defayult isolated mode.
type SharedConfig struct {
	Services map[string]SharedService `yaml:"services,omitempty"`
}

// escribes one logical database hosted inside a shared SQL
// service. The map key in SharedService.Databases is the logical database name
// Docktree appends to the per-worktree tenant identifier.
type SharedDatabase struct {
	// URLEnvs is the list of environment variable names whose values contain a
	// connection URL that should be rewritten to this logical database.
	URLEnvs []string `yaml:"url_envs,omitempty"`

	// Template is the optional source database for `CREATE DATABASE ... TEMPLATE`.
	Template string `yaml:"template,omitempty"`
}

// describes one platform-tier service. The docker compose service name is what
//docker compose will resolve via DNS
type SharedService struct {
	// Kind selects the provisioning + tenancy semantics. Required.
	// Valid: postgres, mysql, redis, s3, generic.
	Kind string `yaml:"kind"`

	// thIS is how a worktree gets logical isolation inside the shared
	// service. Required. Valid values depend on Kind (see ValidateShared).
	Tenancy string `yaml:"tenancy"`

	// env var Docktree writes into each worktree service
	// carrying the per-tenant identifier (database name, key prefix, etc.).
	// Optional; defaulted per Kind.
	TenantEnv string `yaml:"tenant_env,omitempty"`


	Template string `yaml:"template,omitempty"`

	// Aliases adds extra DNS names this service answers to on the platform
	// network, beyond the service name itself. This is for apps that
	// connect to a hostname different from the compose service name.
	Aliases []string `yaml:"aliases,omitempty"`

	//This should only be used if databases below is not declare
	URLEnvs []string `yaml:"url_envs,omitempty"`

	
	Databases map[string]SharedDatabase `yaml:"databases,omitempty"`
}

// DatabaseTargets returns the logical databases Docktree should provision for
// this shared service. Legacy single-database services return a single target
// with the empty key so tenant names remain backward-compatible.
func (svc SharedService) DatabaseTargets() map[string]SharedDatabase {
	if len(svc.Databases) > 0 {
		out := make(map[string]SharedDatabase, len(svc.Databases))
		for name, db := range svc.Databases {
			out[name] = SharedDatabase{
				URLEnvs:  append([]string(nil), db.URLEnvs...),
				Template: db.Template,
			}
		}
		return out
	}
	if svc.Tenancy != "per_database" {
		return nil
	}
	return map[string]SharedDatabase{"": {
		URLEnvs:  append([]string(nil), svc.URLEnvs...),
		Template: svc.Template,
	}}
}

type VolumeSetConfig struct {
	Share []string `yaml:"share,omitempty"`
}

type PortsConfig struct {
	Mode     string `yaml:"mode"`
	BindHost string `yaml:"bind_host"`
	Range    string `yaml:"range"`
}

type TransformConfig struct {
	ContainerName   string `yaml:"container_name"`
	BuiltImage      string `yaml:"built_image"`
	DockerSocket    string `yaml:"docker_socket"`
	ExternalNetwork string `yaml:"external_network"`
	NamedVolume     string `yaml:"named_volume"`
}

type StateConfig struct {
	Directory string `yaml:"directory"`
}

// Defaults returns the defaults from the project plan.
func Defaults() Config {
	return Config{
		Compose: ComposeConfig{
			Files: nil,
		},
		Worktrees: WorktreesConfig{
			Root: "../${repo}.worktrees",
		},
		Setup: SetupConfig{
			Copy:    []string{".env"},
			Symlink: []string{"node_modules"},
		},
		Ports: PortsConfig{
			Mode:     "dynamic",
			BindHost: "127.0.0.1",
			Range:    "41000-49999",
		},
		Transforms: TransformConfig{
			ContainerName: "strip",
			BuiltImage:    "rewrite",
			DockerSocket:  "warn",
		},
		State: StateConfig{
			Directory: ".docktree",
		},
	}
}

func Load(dir string) (*Config, error) {
	cfg := Defaults()
	path := filepath.Join(dir, "docktree.yml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &cfg, nil
	}
	if err != nil {
		return nil, err
	}
	var user Config
	if err := yaml.Unmarshal(data, &user); err != nil {
		return nil, err
	}
	merge(&cfg, user)
	if err := ValidateShared(cfg.Shared, cfg.Volumes.Share); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func Scaffold(dir string, cfg *Config) (bool, error) {
	path := filepath.Join(dir, "docktree.yml")
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(path, data, 0o644)
}

func merge(base *Config, user Config) {
	if user.Compose.Files != nil {
		base.Compose.Files = user.Compose.Files
	}
	if user.Identity.ProjectPrefix != "" {
		base.Identity.ProjectPrefix = user.Identity.ProjectPrefix
	}
	if user.Worktrees.Root != "" {
		base.Worktrees.Root = user.Worktrees.Root
	}
	if user.Setup.Copy != nil {
		base.Setup.Copy = user.Setup.Copy
	}
	if user.Setup.Symlink != nil {
		base.Setup.Symlink = user.Setup.Symlink
	}
	if user.Setup.Run != nil {
		base.Setup.Run = user.Setup.Run
	}
	if user.Shared.Services != nil {
		base.Shared.Services = user.Shared.Services
	}
	if user.Volumes.Share != nil {
		base.Volumes.Share = user.Volumes.Share
	}
	if user.Ports.Mode != "" {
		base.Ports.Mode = user.Ports.Mode
	}
	if user.Ports.BindHost != "" {
		base.Ports.BindHost = user.Ports.BindHost
	}
	if user.Ports.Range != "" {
		base.Ports.Range = user.Ports.Range
	}
	if user.Transforms.ContainerName != "" {
		base.Transforms.ContainerName = user.Transforms.ContainerName
	}
	if user.Transforms.BuiltImage != "" {
		base.Transforms.BuiltImage = user.Transforms.BuiltImage
	}
	if user.Transforms.DockerSocket != "" {
		base.Transforms.DockerSocket = user.Transforms.DockerSocket
	}
	if user.Transforms.ExternalNetwork != "" {
		base.Transforms.ExternalNetwork = user.Transforms.ExternalNetwork
	}
	if user.Transforms.NamedVolume != "" {
		base.Transforms.NamedVolume = user.Transforms.NamedVolume
	}
	if user.State.Directory != "" {
		base.State.Directory = user.State.Directory
	}
}

// Valid kinds + tenancy combos. Map from kind to its set of allowed tenancy
// values. v1 keeps Redis/S3/generic to full_share — the plan defers
// per-key-prefix and per-bucket isolation to a later release.
var allowedTenancyByKind = map[string]map[string]bool{
	"postgres": {"per_database": true, "full_share": true},
	"mysql":    {"per_database": true, "full_share": true},
	"redis":    {"full_share": true},
	"s3":       {"full_share": true},
	"generic":  {"full_share": true},
}

// DefaultTenantEnv returns the per-kind default name for TenantEnv when the
// user has not specified one.
func DefaultTenantEnv(kind string) string {
	switch kind {
	case "postgres", "mysql":
		return "DOCKTREE_DB"
	case "redis":
		return "REDIS_KEY_PREFIX"
	case "s3":
		return "S3_BUCKET"
	default:
		return ""
	}
}

// ValidateShared enforces the schema rules for shared.services and surfaces
// conflicts with volumes.share. Returns the first failure found, deterministic
// across runs so error messages are reproducible.
func ValidateShared(shared SharedConfig, sharedVolumes []string) error {
	if len(shared.Services) == 0 {
		return nil
	}
	names := make([]string, 0, len(shared.Services))
	for name := range shared.Services {
		names = append(names, name)
	}
	sort.Strings(names)

	owners := map[string]string{} // alias -> service that registered it
	urlEnvOwners := map[string]string{}

	for _, name := range names {
		svc := shared.Services[name]
		if name == "" {
			return fmt.Errorf("shared.services: service key cannot be empty")
		}
		if svc.Kind == "" {
			return fmt.Errorf("shared.services.%s.kind is required", name)
		}
		allowed, ok := allowedTenancyByKind[svc.Kind]
		if !ok {
			return fmt.Errorf("shared.services.%s.kind %q is not supported (use postgres, mysql, redis, s3, generic)", name, svc.Kind)
		}
		if svc.Tenancy == "" {
			return fmt.Errorf("shared.services.%s.tenancy is required", name)
		}
		if !allowed[svc.Tenancy] {
			allowedList := sortedKeys(allowed)
			return fmt.Errorf("shared.services.%s.tenancy %q is not valid for kind %q (use %s)", name, svc.Tenancy, svc.Kind, strings.Join(allowedList, ", "))
		}
		if (svc.Template != "" || len(svc.URLEnvs) > 0) && len(svc.Databases) > 0 {
			return fmt.Errorf("shared.services.%s cannot mix top-level url_envs/template with databases; choose one model", name)
		}
		if len(svc.Databases) > 0 {
			if svc.Kind != "postgres" && svc.Kind != "mysql" {
				return fmt.Errorf("shared.services.%s.databases only applies to postgres/mysql, not %s", name, svc.Kind)
			}
			if svc.Tenancy != "per_database" {
				return fmt.Errorf("shared.services.%s.databases requires tenancy per_database", name)
			}
			dbNames := make([]string, 0, len(svc.Databases))
			for dbName := range svc.Databases {
				dbNames = append(dbNames, dbName)
			}
			sort.Strings(dbNames)
			for _, dbName := range dbNames {
				db := svc.Databases[dbName]
				if dbName == "" {
					return fmt.Errorf("shared.services.%s.databases: database key cannot be empty", name)
				}
				if len(db.URLEnvs) == 0 {
					return fmt.Errorf("shared.services.%s.databases.%s.url_envs must declare at least one env var", name, dbName)
				}
				for _, envName := range db.URLEnvs {
					if envName == "" {
						return fmt.Errorf("shared.services.%s.databases.%s.url_envs cannot contain empty entries", name, dbName)
					}
					owner := fmt.Sprintf("shared.services.%s.databases.%s", name, dbName)
					if prev, taken := urlEnvOwners[envName]; taken && prev != owner {
						return fmt.Errorf("url_env %q is claimed by both %s and %s", envName, prev, owner)
					}
					urlEnvOwners[envName] = owner
				}
			}
		} else {
			if svc.Template != "" && svc.Kind != "postgres" && svc.Kind != "mysql" {
				return fmt.Errorf("shared.services.%s.template only applies to postgres/mysql, not %s", name, svc.Kind)
			}
			for _, envName := range svc.URLEnvs {
				if envName == "" {
					return fmt.Errorf("shared.services.%s.url_envs cannot contain empty entries", name)
				}
				owner := fmt.Sprintf("shared.services.%s", name)
				if prev, taken := urlEnvOwners[envName]; taken && prev != owner {
					return fmt.Errorf("url_env %q is claimed by both %s and %s", envName, prev, owner)
				}
				urlEnvOwners[envName] = owner
			}
		}

		aliases := append([]string{name}, svc.Aliases...)
		for _, alias := range aliases {
			if owner, taken := owners[alias]; taken && owner != name {
				return fmt.Errorf("shared.services: alias %q claimed by both %q and %q", alias, owner, name)
			}
			owners[alias] = name
		}
	}

	for _, v := range sharedVolumes {
		if _, taken := shared.Services[v]; taken {
			return fmt.Errorf("service %q appears in both shared.services and volumes.share; remove from volumes.share (platform tier already owns the data)", v)
		}
	}
	return nil
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

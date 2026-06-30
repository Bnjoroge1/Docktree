package config

import (
	"errors"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// LocalOverridesPath returns the path to the worktree-local overrides file.
func LocalOverridesPath(worktreeRoot, stateDir string) string {
	if stateDir == "" {
		stateDir = ".docktree"
	}
	return filepath.Join(worktreeRoot, stateDir, "overrides.yml")
}

// LoadLocalOverrides reads a worktree-local overrides file if it exists.
// Missing files are treated as empty and return a zero OverridesConfig.
func LoadLocalOverrides(path string) (OverridesConfig, error) {
	var out OverridesConfig
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	var wrapper struct {
		Overrides OverridesConfig `yaml:"overrides"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return out, err
	}
	return wrapper.Overrides, nil
}

// WriteLocalOverrides writes the local overrides file atomically.
// It only writes the local overrides section, never the full config.
func WriteLocalOverrides(path string, ov OverridesConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	wrapper := struct {
		Overrides OverridesConfig `yaml:"overrides,omitempty"`
	}{Overrides: ov}
	data, err := yaml.Marshal(wrapper)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// MergeLocalOverrides merges a local OverridesConfig into a base config.
// SkipServices and DropDependencies are unioned; Profiles is replaced if local is non-empty.
func MergeLocalOverrides(base *Config, local OverridesConfig) {
	if len(local.SkipServices) > 0 {
		base.Overrides.SkipServices = UnionStrings(base.Overrides.SkipServices, local.SkipServices)
	}
	if len(local.DropDependencies) > 0 {
		base.Overrides.DropDependencies = UnionStrings(base.Overrides.DropDependencies, local.DropDependencies)
	}
	if len(local.Profiles) > 0 {
		base.Overrides.Profiles = local.Profiles
	}
}

func UnionStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	for _, s := range a {
		seen[s] = true
	}
	for _, s := range b {
		seen[s] = true
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

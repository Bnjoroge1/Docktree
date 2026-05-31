package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the project-level docktree.yml model.
type Config struct {
	Compose    ComposeConfig    `yaml:"compose"`
	Identity   IdentityConfig   `yaml:"identity"`
	Setup      SetupConfig      `yaml:"setup"`
	Shared     ServiceSetConfig `yaml:"shared"`
	Isolated   ServiceSetConfig `yaml:"isolated"`
	Volumes    VolumeSetConfig  `yaml:"volumes"`
	Ports      PortsConfig      `yaml:"ports"`
	Transforms TransformConfig  `yaml:"transforms"`
	State      StateConfig      `yaml:"state"`
}

type ComposeConfig struct {
	Files []string `yaml:"files"`
}

type IdentityConfig struct {
	ProjectPrefix string `yaml:"project_prefix"`
}

type SetupConfig struct {
	Copy    []string `yaml:"copy"`
	Symlink []string `yaml:"symlink"`
	Run     []string `yaml:"run"`
}

type ServiceSetConfig struct {
	Services []string `yaml:"services"`
}

type VolumeSetConfig struct {
	Share []string `yaml:"share"`
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
	if user.Isolated.Services != nil {
		base.Isolated.Services = user.Isolated.Services
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

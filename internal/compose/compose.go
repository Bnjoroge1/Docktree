package compose

// Override is the generated Compose overlay Docktree writes beside user files.
type Override struct {
	Services map[string]ServiceOverride `yaml:"services,omitempty"`
	Networks map[string]NetworkOverride `yaml:"networks,omitempty"`
	Volumes  map[string]VolumeOverride  `yaml:"volumes,omitempty"`
}

type ComposeProject struct {
	Services map[string]Service `yaml:"services"`
	Networks map[string]Network `yaml:"networks,omitempty"`
	Volumes  map[string]Volume  `yaml:"volumes,omitempty"`
}

type Service struct {
	ContainerName string            `yaml:"container_name,omitempty"`
	Image         string            `yaml:"image,omitempty"`
	Build         *BuildConfig      `yaml:"build,omitempty"`
	Ports         []PortMapping     `yaml:"ports,omitempty"`
	Environment   map[string]string `yaml:"environment,omitempty"`
	Networks      []string          `yaml:"networks,omitempty"`
	Volumes       []string          `yaml:"volumes,omitempty"`
	DependsOn     []string          `yaml:"depends_on,omitempty"`
}

type BuildConfig struct {
	Context string `yaml:"context,omitempty"`
}

type Network struct {
	External bool   `yaml:"external,omitempty"`
	Name     string `yaml:"name,omitempty"`
}

type Volume struct {
	External bool   `yaml:"external,omitempty"`
	Name     string `yaml:"name,omitempty"`
}


type ServiceOverride struct {
	ContainerName *string           `yaml:"container_name,omitempty"`
	Image         string            `yaml:"image,omitempty"`
	Labels        map[string]string `yaml:"labels,omitempty"`
	Ports         PortOverride      `yaml:"ports,omitempty"`
	Networks      []string          `yaml:"networks,omitempty"`
}

type NetworkOverride struct {
	External bool   `yaml:"external,omitempty"`
	Name     string `yaml:"name,omitempty"`
}

type VolumeOverride struct {
	Name     string `yaml:"name,omitempty"`
	External *bool  `yaml:"external,omitempty"`
}

type PortMapping struct {
	Target    int    `yaml:"target"`
	Published int    `yaml:"published"`
	HostIP    string `yaml:"host_ip,omitempty"`
	Protocol  string `yaml:"protocol,omitempty"`
}

package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/compose-spec/compose-go/v2/dotenv"
	composeloader "github.com/compose-spec/compose-go/v2/loader"
	composetypes "github.com/compose-spec/compose-go/v2/types"
)

// LoadFull returns both the rich compose-go Project (full fidelity, suitable
// for re-serialization to a synthesized compose file) and the reduced Docktree
// model (sufficient for port allocation and the legacy override path).
//
// Callers that don't need full fidelity should use LoadProject.
func LoadFull(files []string) (*composetypes.Project, *ComposeProject, error) {
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("no compose files provided")
	}
	workingDir := filepath.Dir(files[0])
	details, err := composeloader.LoadConfigFiles(context.Background(), files, workingDir)
	if err != nil {
		return nil, nil, err
	}
	env, err := composeEnvironment(workingDir)
	if err != nil {
		return nil, nil, err
	}
	details.Environment = env
	projectName := filepath.Base(filepath.Clean(workingDir))
	if projectName == "." || projectName == string(os.PathSeparator) || projectName == "" {
		projectName = "docktree"
	}
	project, err := composeloader.LoadWithContext(context.Background(), *details, func(options *composeloader.Options) {
		options.SetProjectName(projectName, false)
		options.SkipConsistencyCheck = true
		// Load all services, including those with profiles, so the reduced model
		// is complete. Docker Compose still applies profile selection at runtime
		// via the --profile flags we forward.
		options.Profiles = []string{"*"}
	})
	if err != nil {
		return nil, nil, err
	}
	reduced, err := fromComposeGo(project)
	if err != nil {
		return nil, nil, err
	}
	return project, reduced, nil
}

func LoadProject(files []string) (*ComposeProject, error) {
	_, reduced, err := LoadFull(files)
	return reduced, err
}

func ParseFile(path string) (*ComposeProject, error) {
	return LoadProject([]string{path})
}

// we convert specific things we care about from the compose-go fmt
func fromComposeGo(project *composetypes.Project) (*ComposeProject, error) {
	if project == nil {
		return nil, fmt.Errorf("compose project is nil")
	}
	converted := &ComposeProject{
		Services: map[string]Service{},
		Networks: map[string]Network{},
		Volumes:  map[string]Volume{},
	}
	for name, svc := range project.Services {
		converted.Services[name] = convertService(svc)
	}
	for name, network := range project.Networks {
		converted.Networks[name] = Network{External: bool(network.External), Name: network.Name}
	}
	for name, volume := range project.Volumes {
		converted.Volumes[name] = Volume{External: bool(volume.External), Name: volume.Name}
	}
	return converted, nil
}

func convertService(svc composetypes.ServiceConfig) Service {
	converted := Service{
		ContainerName: svc.ContainerName,
		Image:         svc.Image,
		NetworkMode:   svc.NetworkMode,
		Environment:   map[string]string{},
	}
	if svc.Build != nil {
		converted.Build = &BuildConfig{Context: svc.Build.Context}
	}
	// Merging multiple compose files concatenates port arrays rather than
	// deduping, so two files publishing the same host:container port for one
	// service yield identical PortMappings. Collapse exact duplicates here so
	// the generated override stays valid (Compose rejects non-unique ports).
	seenPorts := map[PortMapping]bool{}
	for _, port := range svc.Ports {
		published := 0
		if port.Published != "" {
			if parsed, err := strconv.Atoi(port.Published); err == nil {
				published = parsed
			}
		}
		mapping := PortMapping{
			Target:    int(port.Target),
			Published: published,
			HostIP:    port.HostIP,
			Protocol:  normalizeProtocol(port.Protocol),
		}
		if seenPorts[mapping] {
			continue
		}
		seenPorts[mapping] = true
		converted.Ports = append(converted.Ports, mapping)
	}
	for key, value := range svc.Environment {
		if value == nil {
			converted.Environment[key] = ""
			continue
		}
		converted.Environment[key] = *value
	}
	for name := range svc.Networks {
		converted.Networks = append(converted.Networks, name)
	}
	for _, volume := range svc.Volumes {
		converted.Volumes = append(converted.Volumes, volume.String())
	}
	for name := range svc.DependsOn {
		converted.DependsOn = append(converted.DependsOn, name)
	}
	if len(svc.Profiles) > 0 {
		converted.Profiles = make([]string, len(svc.Profiles))
		copy(converted.Profiles, svc.Profiles)
	}
	return converted
}

func composeEnvironment(workingDir string) (map[string]string, error) {
	env := map[string]string{}
	// Seed with process environment so .env values like ${HOST} resolve
	// from exported shell variables during parsing.
	for _, pair := range os.Environ() {
		key, value, ok := splitEnv(pair)
		if ok {
			env[key] = value
		}
	}
	envPath := filepath.Join(workingDir, ".env")
	if _, err := os.Stat(envPath); err == nil {
		dotEnv, err := dotenv.GetEnvFromFile(env, []string{envPath})
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", envPath, err)
		}
		// Shell env takes precedence over .env (docker compose semantics).
		for key, value := range dotEnv {
			if _, exists := env[key]; !exists {
				env[key] = value
			}
		}
	}
	return env, nil
}

func splitEnv(value string) (string, string, bool) {
	for i := 0; i < len(value); i++ {
		if value[i] == '=' {
			return value[:i], value[i+1:], true
		}
	}
	return "", "", false
}

func LoadFullClean(files []string) (*composetypes.Project, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no compose files provided")
	}
	workingDir := filepath.Dir(files[0])
	details, err := composeloader.LoadConfigFiles(context.Background(), files, workingDir)
	if err != nil {
		return nil, err
	}
	projectName := filepath.Base(filepath.Clean(workingDir))
	if projectName == "." || projectName == string(os.PathSeparator) || projectName == "" {
		projectName = "docktree"
	}
	project, err := composeloader.LoadWithContext(context.Background(), *details, func(options *composeloader.Options) {
		options.SetProjectName(projectName, false)
		options.SkipConsistencyCheck = true
		options.SkipInterpolation = true
		options.SkipResolveEnvironment = true
		options.Profiles = []string{"*"}
	})
	if err != nil {
		return nil, err
	}
	return project, nil
}

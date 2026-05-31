package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	composeloader "github.com/compose-spec/compose-go/v2/loader"
	composetypes "github.com/compose-spec/compose-go/v2/types"
)

func LoadProject(files []string) (*ComposeProject, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no compose files provided")
	}
	workingDir := filepath.Dir(files[0])
	details, err := composeloader.LoadConfigFiles(context.Background(), files, workingDir)
	if err != nil {
		return nil, err
	}
	details.Environment = composeEnvironment()
	projectName := filepath.Base(filepath.Clean(workingDir))
	if projectName == "." || projectName == string(os.PathSeparator) || projectName == "" {
		projectName = "docktree"
	}
	project, err := composeloader.LoadWithContext(context.Background(), *details, func(options *composeloader.Options) {
		options.SetProjectName(projectName, false)
	})
	if err != nil {
		return nil, err
	}
	return fromComposeGo(project)
}

func ParseFile(path string) (*ComposeProject, error) {
	return LoadProject([]string{path})
}

//we convert specific things we care about from the compose-go fmt
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
		Environment:   map[string]string{},
	}
	if svc.Build != nil {
		converted.Build = &BuildConfig{Context: svc.Build.Context}
	}
	for _, port := range svc.Ports {
		published := 0
		if port.Published != "" {
			if parsed, err := strconv.Atoi(port.Published); err == nil {
				published = parsed
			}
		}
		converted.Ports = append(converted.Ports, PortMapping{
			Target:    int(port.Target),
			Published: published,
			HostIP:    port.HostIP,
			Protocol:  normalizeProtocol(port.Protocol),
		})
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
	return converted
}

func composeEnvironment() map[string]string {
	env := map[string]string{}
	for _, pair := range os.Environ() {
		key, value, ok := splitEnv(pair)
		if ok {
			env[key] = value
		}
	}
	return env
}

func splitEnv(value string) (string, string, bool) {
	for i := 0; i < len(value); i++ {
		if value[i] == '=' {
			return value[:i], value[i+1:], true
		}
	}
	return "", "", false
}

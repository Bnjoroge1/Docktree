package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bnjoroge/docktree/internal/ports"
	"gopkg.in/yaml.v3"
)

func GenerateOverride(project *ComposeProject, instanceName string, assignments []ports.Assignment, sharedVolumes []string) (*Override, error) {
	if project == nil {
		return nil, fmt.Errorf("compose project is nil")
	}
	byService := map[string][]ports.Assignment{}
	for _, assignment := range assignments {
		byService[assignment.Service] = append(byService[assignment.Service], assignment)
	}
	override := &Override{Services: map[string]ServiceOverride{}, Volumes: map[string]VolumeOverride{}}
	for name, svc := range project.Services {
		serviceOverride := ServiceOverride{}
		changed := false

		//we override container name for the sevice if we defined it yo this current instance
		if svc.ContainerName != "" {
			containerName := instanceName + "-" + name
			serviceOverride.ContainerName = &containerName
			changed = true
		}
		if svc.Build != nil && svc.Image != "" {
			serviceOverride.Image = rewriteImage(instanceName, name, svc.Image)
			changed = true
		}
		if svc.Build != nil && svc.Image == "" {
			serviceOverride.Image = "docktree/" + instanceName + "/" + name + ":latest"
			changed = true
		}
		if mapped := rewritePorts(svc.Ports, byService[name]); len(mapped) > 0 {
			serviceOverride.Ports = PortOverride(mapped)
			changed = true
		}
		serviceOverride.Labels = map[string]string{
			"docktree.managed":  "true",
			"docktree.instance": instanceName,
			"docktree.repo":     repoPart(instanceName),
		}
		changed = true
		if changed {
			override.Services[name] = serviceOverride
		}
	}
	sharedSet := map[string]bool{}
	for _, v := range sharedVolumes {
		sharedSet[v] = true
	}
	for volName, vol := range project.Volumes {
		if vol.External && !sharedSet[volName] {
			external := false
			override.Volumes[volName] = VolumeOverride{
				Name:     instanceName + "-" + volName,
				External: &external,
			}
		}
	}
	return override, nil
}

func WriteOverride(override *Override, path string) error {
	if override == nil {
		return fmt.Errorf("override is nil")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(override)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// GeneratePortClear returns an override that resets ports for every service
// that has at least one explicitly published port. This must be applied
// before the main override so Docker Compose replaces rather than merges.
func GeneratePortClear(project *ComposeProject) *ClearOverride {
	clear := &ClearOverride{Services: map[string]ClearServiceOverride{}}
	for name, svc := range project.Services {
		for _, port := range svc.Ports {
			if port.Published != 0 {
				clear.Services[name] = ClearServiceOverride{Ports: ResetSequence{}}
				break
			}
		}
	}
	if len(clear.Services) == 0 {
		return nil
	}
	return clear
}

func WriteClearOverride(clear *ClearOverride, path string) error {
	if clear == nil {
		return fmt.Errorf("clear override is nil")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(clear)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func rewritePorts(original []PortMapping, assignments []ports.Assignment) []PortMapping {
	if len(original) == 0 || len(assignments) == 0 {
		return nil
	}
	remaining := append([]ports.Assignment(nil), assignments...)
	var rewritten []PortMapping
	for _, port := range original {
		idx := -1
		for i, assignment := range remaining {
			if assignment.ContainerPort == port.Target && sameHostIP(assignment.HostIP, port.HostIP) {
				idx = i
				break
			}
		}
		if idx == -1 {
			continue
		}
		assignment := remaining[idx]
		remaining = append(remaining[:idx], remaining[idx+1:]...)
		if port.Protocol == "" {
			port.Protocol = "tcp"
		}
		port.Published = assignment.HostPort
		if assignment.HostIP != "" {
			port.HostIP = assignment.HostIP
		}
		rewritten = append(rewritten, port)
	}
	return rewritten
}

func rewriteImage(instanceName, service, image string) string {
	tag := "latest"
	if idx := strings.LastIndex(image, ":"); idx > -1 && !strings.Contains(image[idx:], "/") {
		tag = image[idx+1:]
	}
	return "docktree/" + instanceName + "/" + service + ":" + tag
}

func sameHostIP(a, b string) bool {
	return a == b || (a == "127.0.0.1" && b == "") || (a == "" && b == "127.0.0.1")
}

func repoPart(instanceName string) string {
	parts := strings.Split(instanceName, "-")
	if len(parts) == 0 {
		return instanceName
	}
	return parts[0]
}

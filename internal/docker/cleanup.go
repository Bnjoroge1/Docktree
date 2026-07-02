package docker

import (
	"bytes"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

type Resource struct {
	ID   string
	Name string
}

type ProjectResources struct {
	Containers []Resource
	Networks   []Resource
	Volumes    []Resource
}

func ListDocktreeProjects() ([]string, error) {
	lines, err := dockerLines("ps", "-a", "--filter", "label=docktree.managed=true", "--format", "{{.Labels}}")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var projects []string
	for _, line := range lines {
		labels := parseLabelString(line)
		project := labels["docktree.instance"]
		if project == "" {
			project = labels["com.docker.compose.project"]
		}
		if project == "" || seen[project] {
			continue
		}
		seen[project] = true
		projects = append(projects, project)
	}
	slices.Sort(projects)
	return projects, nil
}

func ListProjectResources(project string, includeVolumes bool) (ProjectResources, error) {
	containers, err := listResources("ps", "-a", "--filter", "label=com.docker.compose.project="+project, "--format", "{{.ID}}\t{{.Names}}")
	if err != nil {
		return ProjectResources{}, err
	}
	networks, err := listResources("network", "ls", "--filter", "label=com.docker.compose.project="+project, "--format", "{{.ID}}\t{{.Name}}")
	if err != nil {
		return ProjectResources{}, err
	}
	result := ProjectResources{Containers: containers, Networks: networks}
	if includeVolumes {
		volumes, err := listResources("volume", "ls", "--filter", "label=com.docker.compose.project="+project, "--format", "{{.Name}}")
		if err != nil {
			return ProjectResources{}, err
		}
		result.Volumes = volumes
	}
	return result, nil
}

func RemoveProjectResources(project string, includeVolumes bool) (ProjectResources, error) {
	resources, err := ListProjectResources(project, includeVolumes)
	if err != nil {
		return ProjectResources{}, err
	}
	if len(resources.Containers) > 0 {
		args := []string{"rm", "-f"}
		for _, resource := range resources.Containers {
			args = append(args, resource.ID)
		}
		if err := dockerRun(args...); err != nil {
			return ProjectResources{}, err
		}
	}
	if len(resources.Networks) > 0 {
		args := []string{"network", "rm"}
		for _, resource := range resources.Networks {
			args = append(args, resource.Name)
		}
		if err := dockerRun(args...); err != nil {
			return ProjectResources{}, err
		}
	}
	if includeVolumes && len(resources.Volumes) > 0 {
		args := []string{"volume", "rm", "-f"}
		for _, resource := range resources.Volumes {
			args = append(args, resource.Name)
		}
		if err := dockerRun(args...); err != nil {
			return ProjectResources{}, err
		}
	}
	return resources, nil
}

func listResources(args ...string) ([]Resource, error) {
	lines, err := dockerLines(args...)
	if err != nil {
		return nil, err
	}
	var resources []Resource
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 1 {
			resources = append(resources, Resource{Name: parts[0]})
			continue
		}
		resources = append(resources, Resource{ID: parts[0], Name: parts[1]})
	}
	return resources, nil
}

func CountBridgeNetworks() (int, error) {
	lines, err := dockerLines("network", "ls", "--filter", "driver=bridge", "--format", "{{.ID}}")
	if err != nil {
		return 0, err
	}
	return len(lines), nil
}

func PruneUnusedNetworks() error {
	return dockerRun("network", "prune", "--force")
}

func dockerLines(args ...string) ([]string, error) {
	cmd := exec.Command("docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return nil, err
	}
	text := strings.TrimSpace(stdout.String())
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}

func dockerRun(args ...string) error {
	cmd := exec.Command("docker", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return nil
}

func parseLabelString(value string) map[string]string {
	labels := map[string]string{}
	for _, item := range strings.Split(value, ",") {
		key, val, ok := strings.Cut(strings.TrimSpace(item), "=")
		if ok {
			labels[key] = val
		}
	}
	return labels
}

type VolumeInfo struct {
	Name        string
	Driver      string
	ProjectName string
	VolumeName  string
}

func ListDocktreeVolumes() ([]VolumeInfo, error) {
	lines, err := dockerLines("volume", "ls", "--format", "{{.Name}}\t{{.Driver}}\t{{.Labels}}")
	if err != nil {
		return nil, err
	}
	var volumes []VolumeInfo
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		driver := parts[1]
		var labelsStr string
		if len(parts) >= 3 {
			labelsStr = parts[2]
		}
		labels := parseLabelString(labelsStr)
		project := labels["docktree.instance"]
		if project == "" {
			project = labels["com.docker.compose.project"]
		}
		if project == "" {
			continue
		}
		volName := labels["com.docker.compose.volume"]
		if volName == "" {
			volName = name
		}
		volumes = append(volumes, VolumeInfo{
			Name:        name,
			Driver:      driver,
			ProjectName: project,
			VolumeName:  volName,
		})
	}
	return volumes, nil
}

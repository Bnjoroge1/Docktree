package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	composecli "github.com/compose-spec/compose-go/v2/cli"

	"github.com/bnjoroge/docktree/internal/compose"
	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/docker"
	"github.com/bnjoroge/docktree/internal/ports"
	"github.com/bnjoroge/docktree/internal/state"
)

func composeFiles(dir string, cfg *config.Config) ([]string, error) {
	if len(cfg.Compose.Files) > 0 {
		files := make([]string, 0, len(cfg.Compose.Files))
		for _, file := range cfg.Compose.Files {
			if filepath.IsAbs(file) {
				files = append(files, file)
			} else {
				files = append(files, filepath.Join(dir, file))
			}
		}
		return files, nil
	}

	// Pre-resolve COMPOSE_FILE relative entries against dir (not cwd).
	var configs []string
	if raw := strings.TrimSpace(os.Getenv("COMPOSE_FILE")); raw != "" {
		sep := string(os.PathListSeparator)
		if strings.Contains(raw, ";") && !strings.Contains(raw, sep) {
			sep = ";"
		}
		for _, entry := range strings.Split(raw, sep) {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			if !filepath.IsAbs(entry) {
				entry = filepath.Join(dir, entry)
			}
			configs = append(configs, entry)
		}
	}

	opts, err := composecli.NewProjectOptions(configs,
		composecli.WithWorkingDirectory(dir),
		composecli.WithDefaultConfigPath,
	)
	if err != nil {
		return nil, err
	}

	// compose-go walks up directories; only accept files under dir.
	cleanDir := filepath.Clean(dir) + string(filepath.Separator)
	var found []string
	for _, p := range opts.ConfigPaths {
		if strings.HasPrefix(filepath.Clean(p), cleanDir) || p == filepath.Clean(dir) {
			found = append(found, p)
		}
	}

	if len(found) == 0 {
		return nil, fmt.Errorf("no compose file found in %s\n\nCreate docker-compose.yml or compose.yml, or set compose.files in docktree.yml", dir)
	}
	return found, nil
}

// absComposeFiles resolves compose file paths to absolute form so they remain
// valid when read back from state regardless of the caller's working directory.
func absComposeFiles(files []string) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		if abs, err := filepath.Abs(f); err == nil {
			out = append(out, abs)
		} else {
			out = append(out, f)
		}
	}
	return out
}

func parseAll(files []string) (*compose.ComposeProject, error) {
	return compose.LoadProject(files)
}

func portRequests(project *compose.ComposeProject, bindHost string) []ports.PortRequest {
	var requests []ports.PortRequest
	for service, svc := range project.Services {
		for _, port := range svc.Ports {
			if port.Published == 0 {
				continue
			}
			hostIP := port.HostIP
			if hostIP == "" {
				hostIP = bindHost
			}
			requests = append(requests, ports.PortRequest{Service: service, ContainerPort: port.Target, HostIP: hostIP})
		}
	}
	return requests
}

type composeRunState int

const (
	composeRunUnknown composeRunState = iota
	composeRunStopped
	composeRunRunning
)

func composeRunStateForInstance(inst *state.Instance, cfg *config.Config) (composeRunState, error) {
	out, err := docker.RunCapture(docker.ComposeCommand{ProjectName: inst.ProjectName, Files: activeComposeFiles(inst.WorktreeRoot, cfg, inst), CommandArgs: []string{"ps", "--format", "json"}})
	if err != nil {
		return composeRunUnknown, err
	}
	return parseComposeRunState(out)
}

func parseComposeRunState(out string) (composeRunState, error) {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return composeRunStopped, nil
	}
	var entries []struct {
		State string `json:"State"`
	}
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
			return composeRunUnknown, err
		}
	} else {
		for _, line := range strings.Split(trimmed, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var entry struct {
				State string `json:"State"`
			}
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				return composeRunUnknown, err
			}
			entries = append(entries, entry)
		}
	}
	for _, entry := range entries {
		if strings.EqualFold(strings.TrimSpace(entry.State), "running") {
			return composeRunRunning, nil
		}
	}
	return composeRunStopped, nil
}

func serviceNames(project *compose.ComposeProject) []string {
	names := make([]string, 0, len(project.Services))
	for name := range project.Services {
		names = append(names, name)
	}
	return names
}

func containerNames(project *compose.ComposeProject) map[string]string {
	names := map[string]string{}
	for name, svc := range project.Services {
		if svc.ContainerName != "" {
			names[name] = svc.ContainerName
		}
	}
	return names
}

func builtImages(project *compose.ComposeProject) []string {
	var images []string
	for name, svc := range project.Services {
		if svc.Build != nil {
			if svc.Image != "" {
				images = append(images, name+"="+svc.Image)
			} else {
				images = append(images, name)
			}
		}
	}
	return images
}

func isolatedVolumes(project *compose.ComposeProject, shareList []string) []string {
	shared := map[string]bool{}
	for _, v := range shareList {
		shared[v] = true
	}
	var isolated []string
	for name, vol := range project.Volumes {
		if vol.External && !shared[name] {
			isolated = append(isolated, name)
		}
	}
	slices.Sort(isolated)
	return isolated
}

func shortenImage(image string) string {
	parts := strings.Split(image, "/")
	img := parts[len(parts)-1]
	if idx := strings.LastIndex(img, ":"); idx != -1 {
		tag := img[idx+1:]
		if tag == "latest" {
			img = img[:idx]
		}
	}
	return img
}

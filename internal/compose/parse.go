package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func FindComposeFiles(dir string) ([]string, error) {
	if value := strings.TrimSpace(os.Getenv("COMPOSE_FILE")); value != "" {
		sep := string(os.PathListSeparator)
		if strings.Contains(value, ";") && !strings.Contains(value, sep) {
			sep = ";"
		}
		var files []string
		for _, part := range strings.Split(value, sep) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if !filepath.IsAbs(part) {
				part = filepath.Join(dir, part)
			}
			files = append(files, part)
		}
		return files, nil
	}
	candidates := []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}
	overrides := map[string]string{
		"compose.yaml":        "compose.override.yaml",
		"compose.yml":         "compose.override.yml",
		"docker-compose.yaml": "docker-compose.override.yaml",
		"docker-compose.yml":  "docker-compose.override.yml",
	}
	for _, candidate := range candidates {
		path := filepath.Join(dir, candidate)
		if _, err := os.Stat(path); err == nil {
			files := []string{path}
			override := filepath.Join(dir, overrides[candidate])
			if _, err := os.Stat(override); err == nil {
				files = append(files, override)
			}
			return files, nil
		}
	}
	return nil, fmt.Errorf("no compose.yaml, compose.yml, docker-compose.yaml, or docker-compose.yml found in %s", dir)
}

func PortRequests(project *ComposeProject, defaultHostIP string) []PortMapping {
	var requests []PortMapping
	for _, svc := range project.Services {
		for _, port := range svc.Ports {
			if port.Published == 0 {
				continue
			}
			if port.HostIP == "" {
				port.HostIP = defaultHostIP
			}
			requests = append(requests, port)
		}
	}
	return requests
}

func normalizeProtocol(value string) string {
	if value == "" {
		return "tcp"
	}
	return value
}

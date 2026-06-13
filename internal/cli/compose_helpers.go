package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	composecli "github.com/compose-spec/compose-go/v2/cli"

	"github.com/bnjoroge/docktree/internal/compose"
	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/docker"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/ports"
	"github.com/bnjoroge/docktree/internal/state"
	"github.com/bnjoroge/docktree/internal/tui"
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
		candidates := discoverComposeFiles(dir)
		switch len(candidates) {
		case 0:
		case 1:
			rel, _ := filepath.Rel(dir, candidates[0])
			if rel == "" {
				rel = candidates[0]
			}
			fmt.Fprintf(os.Stderr, "%s Found compose file in %s\n",
				tui.DimS("▸"),
				tui.AccentS(rel))
			fmt.Fprintf(os.Stderr, "%s To make this explicit, add to docktree.yml:\n", tui.DimS("▸"))
			fmt.Fprintf(os.Stderr, "  compose:\n    files:\n      - %s\n\n", rel)
			found = []string{candidates[0]}
		default:
			if !output.IsTerminal(os.Stdin) {
				var lines []string
				for _, c := range candidates {
					rel, _ := filepath.Rel(dir, c)
					if rel == "" {
						rel = c
					}
					lines = append(lines, "  - "+rel)
				}
				return nil, fmt.Errorf("no compose file in project root, and multiple candidates found:\n%s\n\nSet compose.files in docktree.yml to pick one",
					strings.Join(lines, "\n"))
			}
			selected, err := promptComposeFile(dir, candidates)
			if err != nil {
				return nil, err
			}
			found = []string{selected}
		}
		}

	// compose-go default behavior (including parent directories) is used
	// as a last resort to match standard docker compose upward-walk behavior.
	if len(found) == 0 && len(opts.ConfigPaths) > 0 {
		found = opts.ConfigPaths
	}

	if len(found) == 0 {
		return nil, fmt.Errorf("no compose file found in %s or any parent directory\n\nCreate docker-compose.yml or compose.yml, or set compose.files in docktree.yml", dir)
	}
	return found, nil
}

// composeFileRe matches standard compose file basenames.
var composeFileRe = regexp.MustCompile(`^(docker-)?compose[^/]*\.ya?ml$`)

var skipDirs = map[string]bool{
	".docktree":    true,
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"testdata":     true,
	"test":         true,
	"tests":        true,
	"fixtures":     true,
	".cache":       true,
}

func discoverComposeFiles(root string) []string {
	var candidates []string
	seenDirs := map[string]bool{}

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if skipDirs[name] {
				return filepath.SkipDir
			}
			// Don't descend deeper than 3 levels.
			rel, _ := filepath.Rel(root, path)
			if rel != "." && strings.Count(rel, string(filepath.Separator)) >= 3 {
				return filepath.SkipDir
			}
			return nil
		}
		if !composeFileRe.MatchString(d.Name()) {
			return nil
		}
		// Skip files in the root — those would have been found already.
		if filepath.Dir(path) == filepath.Clean(root) {
			return nil
		}
		dir := filepath.Dir(path)
		if seenDirs[dir] {
			return nil
		}
		seenDirs[dir] = true
		candidates = append(candidates, path)
		return nil
	})

	slices.Sort(candidates)
	return candidates
}

// promptComposeFile presents discovered compose files to the user and asks
// which one to use.
func promptComposeFile(root string, candidates []string) (string, error) {
	fmt.Fprintf(os.Stderr, "\n%s No compose file in project root. Found in subdirectories:\n\n",
		tui.WarningS("!"))
	for i, c := range candidates {
		rel, _ := filepath.Rel(root, c)
		if rel == "" {
			rel = c
		}
		fmt.Fprintf(os.Stderr, "  %s %s\n",
			tui.AccentS(fmt.Sprintf("[%d]", i+1)),
			rel)
	}
	fmt.Fprintf(os.Stderr, "\nWhich one? [1]: ")

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading selection: %w", err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return candidates[0], nil
	}
	var idx int
	if _, err := fmt.Sscanf(line, "%d", &idx); err != nil || idx < 1 || idx > len(candidates) {
		return "", fmt.Errorf("invalid selection %q — expected 1-%d", line, len(candidates))
	}
	return candidates[idx-1], nil
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

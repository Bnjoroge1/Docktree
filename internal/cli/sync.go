package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/docker"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/setup"
	"github.com/bnjoroge/docktree/internal/state"
	"github.com/bnjoroge/docktree/internal/tui"
)

func runSync(ctx *Context) (any, int, error) {
	options, err := parseSyncOptions(ctx.Args[1:])
	if err != nil {
		return nil, output.ExitUsage, err
	}
	if options.help {
		printSyncHelp(ctx.Stdout)
		return nil, output.ExitOK, nil
	}

	instances, err := state.LoadGlobalInstances("")
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if len(instances) == 0 {
		return SyncResult{}, output.ExitNoop, nil
	}

	// Group instances by repo root so we load config once per repo.
	type repoGroup struct {
		instances []state.Instance
		config    *config.Config
	}
	groups := make(map[string]*repoGroup)
	for _, inst := range instances {
		root := inst.RepoRoot
		if root == "" {
			continue
		}
		if _, err := os.Stat(inst.WorktreeRoot); err != nil {
			continue // worktree gone
		}
		g, ok := groups[root]
		if !ok {
			g = &repoGroup{}
			groups[root] = g
		}
		g.instances = append(g.instances, inst)
	}

	var items []SyncItem
	for root, g := range groups {
		cfg, err := loadConfigForRepo(root)
		if err != nil {
			continue
		}
		if len(cfg.Setup.Copy) == 0 {
			continue
		}
		for _, inst := range g.instances {
			if filepath.Clean(inst.WorktreeRoot) == filepath.Clean(root) {
				continue
			}
			item := SyncItem{
				Instance:     inst.Name,
				WorktreeRoot: inst.WorktreeRoot,
				MainRoot:     root,
				Branch:       inst.Branch,
			}
			stale := setup.StaleFiles(root, inst.WorktreeRoot, cfg)
			if len(stale) == 0 {
				continue 
			}
			item.Files = stale
			items = append(items, item)
		}
	}

	result := SyncResult{Items: items}
	if len(items) == 0 {
		return result, output.ExitNoop, nil
	}

	if options.dryRun {
		return result, output.ExitOK, nil
	}

	// Confirm unless --force or non-TTY.
	if !options.force && ctx.Renderer.IsTTY {
		fmt.Fprintf(ctx.Stderr, "%s %s\n",
			tui.WarningS("Sync will overwrite"),
			tui.MutedS(fmt.Sprintf("%d file(s) across %d worktree(s)", countFiles(items), len(items))))
		fmt.Fprintf(ctx.Stderr, "%s\n\n", tui.DimS("Pass --force to skip this prompt"))
		if !confirmSync(ctx.Stderr) {
			return SyncResult{}, output.ExitNoop, nil
		}
	}

	syncedCount := 0
	for i := range items {
		item := &items[i]
		if item.Skipped != "" {
			continue
		}
		if _, err := loadConfigForRepo(item.MainRoot); err != nil {
			item.Skipped = "failed to load config"
			continue
		}
		// avoid swapping files mid-restart.
		allInstances, _ := state.LoadGlobalInstances("")
		if inst, ok := allInstances[item.Instance]; ok {
			if isWorktreeRunning(&inst) {
				item.Skipped = "worktree is running"
				continue
			}
		}
		for _, rel := range item.Files {
			source := filepath.Join(item.MainRoot, rel)
			target := filepath.Join(item.WorktreeRoot, rel)
			if err := copyFile(source, target); err != nil {
				item.Skipped = fmt.Sprintf("copy failed: %v", err)
				break
			}
		}
		if item.Skipped == "" {
			syncedCount += len(item.Files)
		}
	}
	result.Synced = syncedCount > 0
	return result, output.ExitOK, nil
}

func loadConfigForRepo(repoRoot string) (*config.Config, error) {
	return config.Load(repoRoot)
}

func isWorktreeRunning(inst *state.Instance) bool {
	if inst == nil || inst.WorktreeRoot == "" {
		return false
	}
	if _, err := os.Stat(inst.WorktreeRoot); err != nil {
		return false
	}
	// Check if docker compose ps returns any running containers.
	out, err := docker.RunCapture(docker.ComposeCommand{
		ProjectName: inst.ProjectName,
		CommandArgs: []string{"ps", "--format", "json"},
	})
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !json.Valid([]byte(line)) {
			continue
		}
		var entry struct {
			State string `json:"State"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err == nil && strings.EqualFold(entry.State, "running") {
			return true
		}
	}
	return false
}

func countFiles(items []SyncItem) int {
	n := 0
	for _, item := range items {
		n += len(item.Files)
	}
	return n
}

func confirmSync(w io.Writer) bool {
	fmt.Fprintf(w, "%s ", tui.DimS("Continue? [y/N]"))
	var answer string
	fmt.Fscanln(os.Stdin, &answer)
	return answer == "y" || answer == "Y" || answer == "yes"
}

// copyFile copies a single file, preserving permissions. Creates parent dirs.
func copyFile(source, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return os.Chmod(target, info.Mode())
}

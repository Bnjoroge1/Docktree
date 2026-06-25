package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/bnjoroge/docktree/internal/compose"
	"github.com/bnjoroge/docktree/internal/config"
	dockgit "github.com/bnjoroge/docktree/internal/git"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/setup"
	"github.com/bnjoroge/docktree/internal/tui"
	"gopkg.in/yaml.v3"
)

type initOptions struct {
	help   bool
	dryRun bool
	force  bool
	apply  bool
}

func parseInitOptions(args []string) (initOptions, error) {
	var opts initOptions
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			opts.help = true
		case "--dry-run":
			opts.dryRun = true
		case "--force":
			opts.force = true
		case "--apply":
			opts.apply = true
		default:
			return initOptions{}, fmt.Errorf("unknown init flag %q", arg)
		}
	}
	return opts, nil
}

func runInit(ctx *Context) (any, int, error) {
	opts, err := parseInitOptions(ctx.Args[1:])
	if err != nil {
		return nil, output.ExitUsage, err
	}
	if opts.help {
		return initHelpDoc(), output.ExitOK, nil
	}
	if opts.apply && opts.dryRun {
		return nil, output.ExitUsage, fmt.Errorf("--apply and --dry-run are mutually exclusive")
	}

	// Resolve to main repo, not current worktree — config lives there.
	repo, err := dockgit.DetectRepo()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	mainRoot, err := dockgit.MainRepoRootForPath(repo.WorktreeRoot)
	if err == nil && mainRoot != "" {
		repo.RepoRoot = mainRoot
	}
	cfgRoot := repo.RepoRoot

	cfg, err := config.Load(cfgRoot)
	if err != nil {
		return nil, output.ExitConfig, err
	}

	files, err := composeFiles(cfgRoot, cfg)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if len(files) == 0 {
		return nil, output.ExitConfig, fmt.Errorf("no compose files found in %s", cfgRoot)
	}

	rawProj, reduced, err := compose.LoadFull(files)
	if err != nil {
		return nil, output.ExitConfig, err
	}

	candidates := setup.DetectShareable(reduced)

	relFiles := make([]string, 0, len(files))
	for _, f := range files {
		rel, rerr := filepath.Rel(cfgRoot, f)
		if rerr != nil {
			rel = f
		}
		relFiles = append(relFiles, rel)
	}

	envFiles := detectGitignoredEnvFiles(cfgRoot)
	symlinkDirs := detectSymlinkableDirs(cfgRoot)
	proposal := buildProposal(relFiles, envFiles, symlinkDirs, candidates)

	var warnings []InitWarning
	for _, c := range candidates {
		if detectSecretsWrapper(rawProj, c.ServiceName) {
			warnings = append(warnings, InitWarning{
				Path:    "shared.services." + c.ServiceName,
				Message: "A consumer service uses a secrets wrapper (Infisical, Doppler, etc.), so " + c.ServiceName + " connection envs may be invisible to Docktree. Ensure the app reads DATABASE_URL from the environment, or use tenancy: isolated.",
			})
		}
	}

	todos := buildTodos(candidates)

	if opts.apply {
		return runInitApply(ctx, cfgRoot, relFiles, envFiles, symlinkDirs, candidates, todos)
	}

	result := InitResult{Todos: todos, Warnings: warnings}

	if opts.dryRun {
		result.Written = "- (stdout)"
		if ctx.Renderer.JSON {
			return result, output.ExitOK, nil
		}
		fmt.Fprint(ctx.Stdout, proposal)
		return nil, output.ExitOK, nil
	}

	// Default writes .proposed; --force writes the real config from the model
	// (mirrors --apply but uses candidate defaults without requiring stdin).
	outPath := filepath.Join(cfgRoot, "docktree.yml.proposed")
	if opts.force {
		outPath = filepath.Join(cfgRoot, "docktree.yml")
		forceCfg := config.Defaults()
		forceCfg.Compose.Files = relFiles
		if len(envFiles) > 0 {
			forceCfg.Setup.Copy = envFiles
		}
		if len(symlinkDirs) > 0 {
			forceCfg.Setup.Symlink = symlinkDirs
		}
		if len(candidates) > 0 {
			svcs := make(map[string]config.SharedService, len(candidates))
			for _, c := range candidates {
				svcs[c.ServiceName] = config.SharedService{
					Kind:    c.Kind,
					Tenancy: "full_share",
					URLEnvs: c.URLEnvs,
				}
			}
			forceCfg.Shared.Services = svcs
		}
		if err := config.ValidateShared(forceCfg.Shared, forceCfg.Volumes.Share); err != nil {
			return nil, output.ExitConfig, fmt.Errorf("invalid config: %w", err)
		}
		data, merr := yaml.Marshal(forceCfg)
		if merr != nil {
			return nil, output.ExitConfig, merr
		}
		if werr := os.WriteFile(outPath, data, 0o644); werr != nil {
			return nil, output.ExitConfig, werr
		}
	} else {
		if err := os.WriteFile(outPath, []byte(proposal), 0o644); err != nil {
			return nil, output.ExitConfig, err
		}
	}
	result.Written = outPath

	if ctx.Renderer.JSON {
		return result, output.ExitOK, nil
	}

	fmt.Fprintf(ctx.Stdout, "%s Wrote %s\n", tui.OKS("✓"), tui.AccentS(outPath))
	if len(todos) > 0 {
		fmt.Fprintf(ctx.Stdout, "\n%s %d decisions need your input. Ask your AI agent to walk through them, or edit the file manually.\n", tui.DimS("→"), len(todos))
	}
	for _, w := range warnings {
		fmt.Fprintf(ctx.Stdout, "\n%s %s\n", tui.WarningS("⚠"), tui.DimS(w.Message))
	}
	return nil, output.ExitOK, nil
}

// YAML is marshalled from the config model — no string substitution, so
// nested paths are never ambiguous.
func runInitApply(ctx *Context, cfgRoot string, files, envFiles, symlinkDirs []string, candidates []setup.ServiceCandidate, todos []InitTodo) (any, int, error) {
	answers, err := readAnswers(os.Stdin)
	if err != nil {
		return nil, output.ExitUsage, fmt.Errorf("failed to read --apply answers: %w", err)
	}

	cfg := config.Defaults()
	cfg.Compose.Files = files
	if len(envFiles) > 0 {
		cfg.Setup.Copy = envFiles
	}
	if len(symlinkDirs) > 0 {
		cfg.Setup.Symlink = symlinkDirs
	}

	if len(candidates) > 0 {
		svcs := make(map[string]config.SharedService, len(candidates))
		for _, c := range candidates {
			svc := config.SharedService{Kind: c.Kind}
			tenancyPath := "shared.services." + c.ServiceName + ".tenancy"
			if v, ok := answers[tenancyPath]; ok {
				s, sok := v.(string)
				if !sok {
					return nil, output.ExitConfig, fmt.Errorf("answer for %s must be a string, got %T", tenancyPath, v)
				}
				svc.Tenancy = s
			} else {
				svc.Tenancy = "full_share"
			}
			envsPath := "shared.services." + c.ServiceName + ".url_envs"
			if v, ok := answers[envsPath]; ok {
				arr, aok := v.([]any)
				if !aok {
					return nil, output.ExitConfig, fmt.Errorf("answer for %s must be an array, got %T", envsPath, v)
				}
				strs := make([]string, 0, len(arr))
				for _, el := range arr {
					s, eok := el.(string)
					if !eok {
						return nil, output.ExitConfig, fmt.Errorf("answer for %s contains non-string element %T", envsPath, el)
					}
					strs = append(strs, s)
				}
				svc.URLEnvs = strs
			} else if len(c.URLEnvs) > 0 {
				svc.URLEnvs = c.URLEnvs
			}
			svcs[c.ServiceName] = svc
		}
		cfg.Shared.Services = svcs
	}

	if err := config.ValidateShared(cfg.Shared, cfg.Volumes.Share); err != nil {
		return nil, output.ExitConfig, fmt.Errorf("invalid config: %w", err)
	}

	data, merr := yaml.Marshal(cfg)
	if merr != nil {
		return nil, output.ExitConfig, merr
	}

	outPath := filepath.Join(cfgRoot, "docktree.yml")
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return nil, output.ExitConfig, err
	}
	if _, verr := config.Load(cfgRoot); verr != nil {
		return nil, output.ExitConfig, fmt.Errorf("generated config is invalid: %w", verr)
	}
	result := InitResult{Written: outPath}
	if ctx.Renderer.JSON {
		return result, output.ExitOK, nil
	}
	fmt.Fprintf(ctx.Stdout, "%s Wrote %s\n", tui.OKS("✓"), tui.AccentS(outPath))
	return nil, output.ExitOK, nil
}

func buildProposal(files, envFiles, symlinkDirs []string, candidates []setup.ServiceCandidate) string {
	var b strings.Builder

	b.WriteString("compose:\n")
	b.WriteString("  files:\n")
	for _, f := range files {
		fmt.Fprintf(&b, "    - %s\n", f)
	}

	b.WriteString("\nsetup:\n")
	if len(envFiles) > 0 {
		b.WriteString("  copy:\n")
		for _, f := range envFiles {
			fmt.Fprintf(&b, "    - %s\n", f)
		}
	} else {
		b.WriteString("  # copy:\n")
		b.WriteString("  #   - .env\n")
	}
	if len(symlinkDirs) > 0 {
		b.WriteString("  symlink:\n")
		for _, d := range symlinkDirs {
			fmt.Fprintf(&b, "    - %s\n", d)
		}
	} else {
		b.WriteString("  # symlink:\n")
		b.WriteString("  #   - node_modules\n")
	}
	b.WriteString("  # run:\n")
	b.WriteString("  #   - npm ci\n")

	if len(candidates) > 0 {
		b.WriteString("\nshared:\n")
		b.WriteString("  services:\n")
		for _, c := range candidates {
			fmt.Fprintf(&b, "    %s:\n", c.ServiceName)
			fmt.Fprintf(&b, "      kind: %s\n", c.Kind)
			fmt.Fprintf(&b, "      # TODO: choose tenancy mode\n")
			fmt.Fprintf(&b, "      tenancy: full_share  # full_share | per_database\n")
			if len(c.URLEnvs) > 0 {
				fmt.Fprintf(&b, "      # TODO: confirm these are the env vars your app reads\n")
				fmt.Fprintf(&b, "      url_envs:\n")
				for _, e := range c.URLEnvs {
					fmt.Fprintf(&b, "        - %s\n", e)
				}
			}
		}
	}

	b.WriteString("\n# ports:\n")
	b.WriteString("#   bind_host: 127.0.0.1\n")
	b.WriteString("#   range: 41000-49999\n")

	b.WriteString("\n# state:\n")
	b.WriteString("#   directory: .docktree\n")

	return b.String()
}

func buildTodos(candidates []setup.ServiceCandidate) []InitTodo {
	var todos []InitTodo
	for _, c := range candidates {
		todos = append(todos, InitTodo{
			Path:     "shared.services." + c.ServiceName + ".tenancy",
			Question: fmt.Sprintf("Service %q looks like %s (%s). Should it run in the shared platform tier (one instance for all worktrees) or be isolated per worktree?", c.ServiceName, c.Kind, c.Image),
			Kind:     "enum",
			Options:  tenancyOptions(c.Kind),
		})
		if len(c.URLEnvs) > 0 {
			todos = append(todos, InitTodo{
				Path:     "shared.services." + c.ServiceName + ".url_envs",
				Question: fmt.Sprintf("These env vars on consumer services reference %q and look like connection strings. Confirm your app reads them so Docktree can rewrite them per worktree.", c.ServiceName),
				Kind:     "string_list",
				Options:  c.URLEnvs,
			})
		}
	}
	return todos
}

func tenancyOptions(kind string) []string {
	switch kind {
	case "postgres", "mysql", "mongodb":
		return []string{"full_share", "per_database"}
	default:
		return []string{"full_share"}
	}
}

func readAnswers(r io.Reader) (map[string]any, error) {
	data, err := io.ReadAll(bufio.NewReader(r))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var answers map[string]any
	if err := json.Unmarshal(data, &answers); err != nil {
		return nil, fmt.Errorf("invalid JSON on stdin: %w", err)
	}
	return answers, nil
}

func detectGitignoredEnvFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var found []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, ".env") {
			continue
		}
		if strings.HasSuffix(name, ".example") || strings.HasSuffix(name, ".sample") || strings.HasSuffix(name, ".template") {
			continue
		}
		full := filepath.Join(dir, name)
		if isGitignored(full) {
			found = append(found, name)
		}
	}
	sort.Strings(found)
	return found
}

func isGitignored(path string) bool {
	cmd := exec.Command("git", "check-ignore", "-q", path)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

func detectSymlinkableDirs(dir string) []string {
	known := []string{"node_modules", "vendor", "target", "__pycache__", ".venv", ".next", ".nuxt", "dist", "build"}
	var found []string
	for _, name := range known {
		full := filepath.Join(dir, name)
		if info, err := os.Stat(full); err == nil && info.IsDir() {
			found = append(found, name)
		}
	}
	sort.Strings(found)
	return found
}
// A consumer service running through a secrets wrapper (infisical run,
// doppler run, etc.) means the DB URL is injected by the wrapper and
// invisible to Docktree. Warn when any consumer's command/entrypoint
// contains a wrapper keyword.
func detectSecretsWrapper(rawProj *composetypes.Project, candidateName string) bool {
	if rawProj == nil {
		return false
	}
	wrappers := []string{"infisical", "doppler", "vault", "aws secretsmanager"}
	for _, svc := range rawProj.Services {
		if svc.Name == candidateName {
			continue
		}
		cmdParts := append([]string{}, svc.Command...)
		cmdParts = append(cmdParts, svc.Entrypoint...)
		cmdStr := strings.ToLower(strings.Join(cmdParts, " "))
		for _, w := range wrappers {
			if strings.Contains(cmdStr, w) {
				return true
			}
		}
	}
	return false
}

func initHelpDoc() HelpDoc {
	return HelpDoc{
		Command:  "init",
		Synopsis: "Generate a docktree.yml from your compose files",
		Usage:    []string{"docktree init [options]"},
		Options: []HelpOption{
			{Flags: []string{"--dry-run"}, Description: "Print the proposed config to stdout without writing"},
			{Flags: []string{"--force"}, Description: "Overwrite an existing docktree.yml (default writes .proposed)"},
			{Flags: []string{"--apply"}, Description: "Read answers as JSON from stdin and write final docktree.yml"},
			{Flags: []string{"-h", "--help"}, Description: "Show this help text"},
		},
		Examples: []string{
			"docktree init",
			"docktree init --dry-run",
			"docktree init --json",
			"echo '{\"shared.services.db.tenancy\":\"per_database\"}' | docktree init --apply",
		},
	}
}

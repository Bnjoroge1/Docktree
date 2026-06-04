package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bnjoroge/docktree/internal/compose"
	"github.com/bnjoroge/docktree/internal/provision"
	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/docker"
	dockgit "github.com/bnjoroge/docktree/internal/git"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/tui"
)

// PlatformResult is the structured result for platform subcommands.
type PlatformResult struct {
	Action       string   `json:"action"`
	Project      string   `json:"project"`
	Network      string   `json:"network"`
	Services     []string `json:"services,omitempty"`
	ComposeFile  string   `json:"compose_file,omitempty"`
	Running      bool     `json:"running"`
	AlreadyState bool     `json:"already_state,omitempty"`
	Skipped      bool     `json:"skipped,omitempty"`
	Reason       string   `json:"reason,omitempty"`
}

// runPlatform dispatches `docktree platform <sub>` to the matching handler.
func runPlatform(ctx *Context) (any, int, error) {
	args := ctx.Args[1:]
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printPlatformHelp(ctx.Stdout)
		return nil, output.ExitOK, nil
	}
	switch args[0] {
	case "up":
		return runPlatformUp(ctx)
	case "down":
		return runPlatformDown(ctx)
	case "status":
		return runPlatformStatus(ctx)
	default:
		fmt.Fprintf(ctx.Stderr, "unknown platform subcommand %q\n\n", args[0])
		printPlatformHelp(ctx.Stderr)
		return nil, output.ExitUsage, nil
	}
}

func printPlatformHelp(w io.Writer) {
	fmt.Fprintf(w, "Usage: docktree platform <command>\n\n")
	fmt.Fprintln(w, "Commands:")
	printHelpCmd(w, 8, "up", "Start the repo-scoped platform stack")
	printHelpCmd(w, 8, "down", "Stop the repo-scoped platform stack (preserves data)")
	printHelpCmd(w, 8, "status", "Show platform stack state")
	fmt.Fprintln(w)
	fmt.Fprintln(w, tui.MutedS("The platform stack runs services marked in `shared.services` of"))
	fmt.Fprintln(w, tui.MutedS("docktree.yml. Worktrees reach them via Docker DNS on the platform"))
	fmt.Fprintln(w, tui.MutedS("external network."))
}

// runPlatformUp starts the repo-scoped platform stack. Idempotent — calling
// it again when already running re-emits the synthesized compose and runs
// `docker compose up -d`, which no-ops if nothing has drifted.
func runPlatformUp(ctx *Context) (any, int, error) {
	plan, err := buildPlatformPlan()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if plan.Skipped {
		return PlatformResult{Action: "up", Skipped: true, Reason: plan.SkipReason}, output.ExitNoop, nil
	}
	steps := ctx.Steps
	if steps != nil {
		steps.Header("Starting platform…", plan.Project)
	}
	if err := ensurePlatformNetwork(plan.Network, plan.RepoSlug); err != nil {
		return nil, output.ExitDocker, err
	}
	if steps != nil {
		steps.Done("Platform network ready")
	}
	if err := compose.WriteComposeFile(plan.PlatformProject, plan.ComposeFile); err != nil {
		return nil, output.ExitConfig, err
	}
	if steps != nil {
		steps.Done("Generated platform compose")
	}
	dockerStdout := ctx.Stdout
	if ctx.Renderer.JSON {
		dockerStdout = io.Discard
	}
	cmd := docker.ComposeCommand{
		ProjectName: plan.Project,
		Files:       []string{plan.ComposeFile},
		CommandArgs: []string{"up", "-d"},
	}
	var spin *tui.SpinStep
	if steps != nil {
		spin = steps.StartSpin("docker compose up -d (platform)")
	}
	if err := docker.Run(cmd, dockerStdout, ctx.Stderr); err != nil {
		if spin != nil {
			spin.Stop()
		}
		return nil, output.ExitDocker, err
	}
	if spin != nil {
		spin.Stop()
	}
	return PlatformResult{
		Action:      "up",
		Project:     plan.Project,
		Network:     plan.Network,
		Services:    compose.SortedServiceNames(plan.PlatformProject),
		ComposeFile: plan.ComposeFile,
		Running:     true,
	}, output.ExitOK, nil
}

// runPlatformDown stops the platform stack but keeps named volumes and the
// external network — explicit destructive paths (`docktree clean --platform`,
// future flag) own data deletion.
func runPlatformDown(ctx *Context) (any, int, error) {
	plan, err := buildPlatformPlan()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if plan.Skipped {
		return PlatformResult{Action: "down", Skipped: true, Reason: plan.SkipReason}, output.ExitNoop, nil
	}
	if _, err := os.Stat(plan.ComposeFile); errors.Is(err, os.ErrNotExist) {
		return PlatformResult{Action: "down", Project: plan.Project, AlreadyState: true, Reason: "no platform compose file on disk"}, output.ExitNoop, nil
	}
	steps := ctx.Steps
	if steps != nil {
		steps.Header("Stopping platform…", plan.Project)
	}
	dockerStdout := ctx.Stdout
	if ctx.Renderer.JSON {
		dockerStdout = io.Discard
	}
	cmd := docker.ComposeCommand{
		ProjectName: plan.Project,
		Files:       []string{plan.ComposeFile},
		CommandArgs: []string{"down"},
	}
	if err := docker.Run(cmd, dockerStdout, ctx.Stderr); err != nil {
		return nil, output.ExitDocker, err
	}
	return PlatformResult{
		Action:      "down",
		Project:     plan.Project,
		Network:     plan.Network,
		ComposeFile: plan.ComposeFile,
		Running:     false,
	}, output.ExitOK, nil
}

// runPlatformStatus reports project name, network, services, and whether the
// platform stack appears to be up.
func runPlatformStatus(ctx *Context) (any, int, error) {
	plan, err := buildPlatformPlan()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if plan.Skipped {
		return PlatformResult{Action: "status", Skipped: true, Reason: plan.SkipReason}, output.ExitNoop, nil
	}
	running, err := platformIsRunning(plan.Project)
	if err != nil {
		return nil, output.ExitDocker, err
	}
	return PlatformResult{
		Action:      "status",
		Project:     plan.Project,
		Network:     plan.Network,
		Services:    compose.SortedServiceNames(plan.PlatformProject),
		ComposeFile: plan.ComposeFile,
		Running:     running,
	}, output.ExitOK, nil
}

// platformPlan is the resolved description of what `platform up/down/status`
// would act on: project name, network, generated compose file path, and the
// synthesized compose project itself.
type platformPlan struct {
	Project         string
	Network         string
	RepoSlug        string
	ComposeFile     string
	PlatformProject *compose.PlatformComposeProject
	Shared          config.SharedConfig
	Skipped         bool
	SkipReason      string
}

// buildPlatformPlan locates the main repo root, loads its docktree.yml,
// reads the source compose files, and synthesizes the platform project.
// All platform CLI commands route through here so they agree on identity.
func buildPlatformPlan() (*platformPlan, error) {
	mainRoot, err := dockgit.MainRepoRoot()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(mainRoot)
	if err != nil {
		return nil, err
	}
	if len(cfg.Shared.Services) == 0 {
		return &platformPlan{Skipped: true, SkipReason: "no shared.services declared in docktree.yml"}, nil
	}
	files, err := composeFiles(mainRoot, cfg)
	if err != nil {
		return nil, err
	}
	raw, _, err := compose.LoadFull(files)
	if err != nil {
		return nil, err
	}
	repoSlug := dockgit.RepoName(mainRoot)
	platformProj, err := compose.SynthesizePlatform(raw, cfg.Shared, repoSlug)
	if err != nil {
		return nil, err
	}
	generatedDir := filepath.Join(mainRoot, cfg.State.Directory, "generated")
	return &platformPlan{
		Project:         compose.PlatformProjectName(repoSlug),
		Network:         compose.PlatformNetworkName(repoSlug),
		RepoSlug:        repoSlug,
		ComposeFile:     filepath.Join(generatedDir, "platform-compose.yml"),
		PlatformProject: (*compose.PlatformComposeProject)(platformProj),
		Shared:          cfg.Shared,
	}, nil
}

// ensurePlatformNetwork is idempotent: it creates the external docker network
// the platform compose project references, labeled for label-based discovery.
func ensurePlatformNetwork(name, repoSlug string) error {
	if name == "" {
		return fmt.Errorf("empty network name")
	}
	existing, err := dockerCapture("network", "ls", "--filter", "name=^"+name+"$", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	if strings.TrimSpace(existing) != "" {
		return nil
	}
	return dockerSilent("network", "create",
		"--label", "docktree.managed=true",
		"--label", "docktree.tier=platform",
		"--label", "docktree.repo="+repoSlug,
		name)
}

// platformIsRunning probes `docker compose ls` for the platform project name.
func platformIsRunning(project string) (bool, error) {
	out, err := dockerCapture("compose", "ls", "--filter", "name=^"+project+"$", "--format", "json")
	if err != nil {
		return false, err
	}
	out = strings.TrimSpace(out)
	if out == "" || out == "[]" {
		return false, nil
	}
	return strings.Contains(out, `"`+project+`"`), nil
}

func dockerCapture(args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return stdout.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

func dockerSilent(args ...string) error {
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

// ensurePlatformUp is called by runUp when shared services are configured.
// It synthesizes and writes the platform compose file, creates the external
// network if needed, and starts the platform stack. Idempotent.
func ensurePlatformUp(ctx *Context, instanceName, repoSlug string) (string, string, error) {
	plan, err := buildPlatformPlan()
	if err != nil {
		return "", "", err
	}
	if plan.Skipped {
		return "", "", nil
	}
	if err := ensurePlatformNetwork(plan.Network, plan.RepoSlug); err != nil {
		return "", "", err
	}
	if err := compose.WriteComposeFile(plan.PlatformProject, plan.ComposeFile); err != nil {
		return "", "", err
	}
	running, err := platformIsRunning(plan.Project)
	if err != nil {
		return plan.Project, plan.ComposeFile, err
	}
	if !running {
		cmd := docker.ComposeCommand{
			ProjectName: plan.Project,
			Files:       []string{plan.ComposeFile},
			CommandArgs: []string{"up", "-d"},
		}
		if err := docker.Run(cmd, io.Discard, ctx.Stderr); err != nil {
			return plan.Project, plan.ComposeFile, err
		}
	}
	// Run per-tenant provisioning for services that need it (postgres per_database).
	for svcName, svc := range plan.Shared.Services {
		if svc.Kind != "postgres" && svc.Kind != "mysql" {
			continue
		}
		if svc.Tenancy != "per_database" {
			continue
		}
		container := plan.Project + "-" + svcName
		// Wait for the database to be ready — platform up -d returns before
		// Postgres finishes its init sequence.
		if err := provision.WaitForPostgres(container, "postgres", 30); err != nil {
			fmt.Fprintf(ctx.Stderr, "warning: %s not ready, skipping tenant provisioning: %v\n", svcName, err)
			continue
		}
		provCfg := provision.TenantConfig{
			Kind:       svc.Kind,
			Tenancy:    svc.Tenancy,
			Template:   svc.Template,
			TenantName: provision.TenantName(repoSlug, instanceName),
			Host:       container,
			User:       "postgres",
		}
		if err := provision.Provision(provCfg); err != nil {
			fmt.Fprintf(ctx.Stderr, "warning: provision %s (%s): %v\n", svcName, provCfg.TenantName, err)
		}
	}
	return plan.Project, plan.ComposeFile, nil
}

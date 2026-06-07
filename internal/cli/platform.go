package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bnjoroge/docktree/internal/compose"
	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/docker"
	dockgit "github.com/bnjoroge/docktree/internal/git"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/provision"
	"github.com/bnjoroge/docktree/internal/state"
	"github.com/bnjoroge/docktree/internal/tui"
	composetypes "github.com/compose-spec/compose-go/v2/types"
)

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

// TenantEntry describes one per-worktree tenant namespace inside a shared service.
type TenantEntry struct {
	Instance  string `json:"instance"`
	Service   string `json:"service"`
	LogicalDB string `json:"logical_db,omitempty"`
	TenantDB  string `json:"tenant_db"`
	Exists    bool   `json:"exists"`
}

// PlatformTenantsResult is the result of `docktree platform tenants`.
type PlatformTenantsResult struct {
	Project string        `json:"project"`
	Tenants []TenantEntry `json:"tenants"`
}

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
	case "tenants":
		return runPlatformTenants(ctx)
	case "logs":
		return runPlatformLogs(ctx)
	case "clean":
		return runPlatformClean(ctx)
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
	printHelpCmd(w, 8, "tenants", "List tenant databases across all instances")
	printHelpCmd(w, 8, "logs", "Stream platform service logs (pass service name to filter)")
	printHelpCmd(w, 8, "clean", "Stop platform, drop all tenant DBs, remove network (--yes required)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, tui.MutedS("The platform stack runs services marked in `shared.services` of"))
	fmt.Fprintln(w, tui.MutedS("docktree.yml. Worktrees reach them via Docker DNS on the platform"))
	fmt.Fprintln(w, tui.MutedS("external network."))
}

func runPlatformUp(ctx *Context) (any, int, error) {
	current, err := dockgit.DetectRepo()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	mainRoot, err := dockgit.MainRepoRoot()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if current.WorktreeRoot != mainRoot {
		return nil, output.ExitConfig, fmt.Errorf("docktree platform up must be run from the main repo root; use docktree up in linked worktrees")
	}
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
	if err := provisionPlatformTenants(plan, plan.RepoSlug); err != nil {
		return nil, output.ExitDocker, err
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
// external network owns data deletion.
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

func platformRepoMatches(instRepoRoot, repoSlug string) bool {
	instMainRoot, err := dockgit.MainRepoRootForPath(instRepoRoot)
	if err != nil {
		return dockgit.RepoName(instRepoRoot) == repoSlug
	}
	return dockgit.RepoName(instMainRoot) == repoSlug
}

func platformRepoSlugForInstance(instRepoRoot string) string {
	instMainRoot, err := dockgit.MainRepoRootForPath(instRepoRoot)
	if err != nil {
		return dockgit.RepoName(instRepoRoot)
	}
	return dockgit.RepoName(instMainRoot)
}

func postgresCredentialsFromEnv(svc composetypes.ServiceConfig) (string, string) {
	user := "postgres"
	password := ""
	if svc.Environment != nil {
		if v, ok := svc.Environment["POSTGRES_USER"]; ok && v != nil && *v != "" {
			user = *v
		}
		if v, ok := svc.Environment["POSTGRES_PASSWORD"]; ok && v != nil {
			password = *v
		}
	}
	return user, password
}

func mysqlCredentialsFromEnv(svc composetypes.ServiceConfig) (string, string) {
	user := "root"
	password := ""
	if svc.Environment != nil {
		if v, ok := svc.Environment["MYSQL_ROOT_PASSWORD"]; ok && v != nil {
			password = *v
			return user, password
		}
		if v, ok := svc.Environment["MYSQL_USER"]; ok && v != nil && *v != "" {
			user = *v
		}
		if v, ok := svc.Environment["MYSQL_PASSWORD"]; ok && v != nil {
			password = *v
		}
	}
	return user, password
}

func mongoCredentialsFromEnv(svc composetypes.ServiceConfig) (string, string) {
	user := ""
	password := ""
	if svc.Environment != nil {
		if v, ok := svc.Environment["MONGO_INITDB_ROOT_USERNAME"]; ok && v != nil {
			user = *v
		}
		if v, ok := svc.Environment["MONGO_INITDB_ROOT_PASSWORD"]; ok && v != nil {
			password = *v
		}
	}
	return user, password
}

func databaseCredentialsFromEnv(kind string, svc composetypes.ServiceConfig) (string, string) {
	switch kind {
	case "mysql":
		return mysqlCredentialsFromEnv(svc)
	case "mongodb":
		return mongoCredentialsFromEnv(svc)
	default:
		return postgresCredentialsFromEnv(svc)
	}
}

func provisionPlatformTenants(plan *platformPlan, repoSlug string) error {
	instances, err := state.LoadGlobalInstances("")
	if err != nil {
		return fmt.Errorf("load global instances: %w", err)
	}
	for _, inst := range instances {
		if !platformRepoMatches(inst.RepoRoot, repoSlug) {
			continue
		}
		for svcName, svc := range plan.Shared.Services {
			if svc.Kind != "postgres" && svc.Kind != "mysql" && svc.Kind != "mongodb" {
				continue
			}
			if svc.Tenancy != "per_database" {
				continue
			}
			platformSvc, ok := plan.PlatformProject.Services[svcName]
			if !ok {
				continue
			}
			user, password := databaseCredentialsFromEnv(svc.Kind, platformSvc)
			container := plan.Project + "-" + svcName
			readyCfg := provision.TenantConfig{Kind: svc.Kind, Tenancy: svc.Tenancy, Host: container, User: user, Password: password}
			if err := provision.WaitForService(readyCfg, 30); err != nil {
				return fmt.Errorf("service %s not ready: %w", container, err)
			}
			for logicalName, dbTarget := range svc.DatabaseTargets() {
				template := dbTarget.Template
				if template == "" {
					template = svc.Template
				}
				provCfg := provision.TenantConfig{
					Kind:       svc.Kind,
					Tenancy:    svc.Tenancy,
					Template:   template,
					TenantName: provision.TenantNameForDatabase(repoSlug, inst.Name, logicalName),
					Host:       container,
					User:       user,
					Password:   password,
				}
				if err := provision.Provision(provCfg); err != nil {
					return fmt.Errorf("failed to provision tenant database %s: %w", provCfg.TenantName, err)
				}
			}
		}
	}
	return nil
}

// tenantBinding pairs a human-readable tenant DB name with the TenantConfig
// needed to deprovision it.
type tenantBinding struct {
	TenantDB string
	Config   provision.TenantConfig
}

// tenantBindingsForInstance returns one binding per logical database that
// the given instance owns across all per_database shared services.
func tenantBindingsForInstance(plan *platformPlan, inst *state.Instance) []tenantBinding {
	repoSlug := platformRepoSlugForInstance(inst.RepoRoot)
	var bindings []tenantBinding
	for svcName, svc := range plan.Shared.Services {
		if svc.Tenancy != "per_database" {
			continue
		}
		platformSvc, ok := plan.PlatformProject.Services[svcName]
		if !ok {
			continue
		}
		user, password := databaseCredentialsFromEnv(svc.Kind, platformSvc)
		container := plan.Project + "-" + svcName
		for logicalName, dbTarget := range svc.DatabaseTargets() {
			template := dbTarget.Template
			if template == "" {
				template = svc.Template
			}
			tenantDB := provision.TenantNameForDatabase(repoSlug, inst.Name, logicalName)
			bindings = append(bindings, tenantBinding{
				TenantDB: tenantDB,
				Config: provision.TenantConfig{
					Kind:       svc.Kind,
					Tenancy:    svc.Tenancy,
					Template:   template,
					TenantName: tenantDB,
					Host:       container,
					User:       user,
					Password:   password,
				},
			})
		}
	}
	return bindings
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
		PlatformProject: platformProj,
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
	if err := provisionPlatformTenants(plan, plan.RepoSlug); err != nil {
		return plan.Project, plan.ComposeFile, err
	}
	return plan.Project, plan.ComposeFile, nil
}

// runPlatformTenants lists every known per-worktree tenant namespace across
// all global instances, querying the platform Postgres to report whether each
// tenant database actually exists.
func runPlatformTenants(ctx *Context) (any, int, error) {
	plan, err := buildPlatformPlan()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if plan.Skipped {
		return PlatformTenantsResult{}, output.ExitNoop, nil
	}

	instances, err := state.LoadGlobalInstances("")
	if err != nil {
		return nil, output.ExitConfig, err
	}

	var entries []TenantEntry
	for _, inst := range instances {
		if !platformRepoMatches(inst.RepoRoot, plan.RepoSlug) {
			continue
		}
		repoSlug := platformRepoSlugForInstance(inst.RepoRoot)
		for svcName, svc := range plan.Shared.Services {
			if svc.Tenancy != "per_database" {
				continue
			}
			platformSvc, ok := plan.PlatformProject.Services[svcName]
			if !ok {
				continue
			}
			user, password := databaseCredentialsFromEnv(svc.Kind, platformSvc)
			container := plan.Project + "-" + svcName
			for logicalName := range svc.DatabaseTargets() {
				tenantDB := provision.TenantNameForDatabase(repoSlug, inst.Name, logicalName)
				exists, err := provision.DBExists(provision.TenantConfig{
					Kind:       svc.Kind,
					Tenancy:    svc.Tenancy,
					TenantName: tenantDB,
					Host:       container,
					User:       user,
					Password:   password,
				})
				if err != nil {
					return nil, output.ExitDocker, fmt.Errorf("checking tenant %s for %s: %w", tenantDB, inst.Name, err)
				}
				entries = append(entries, TenantEntry{
					Instance:  inst.Name,
					Service:   svcName,
					LogicalDB: logicalName,
					TenantDB:  tenantDB,
					Exists:    exists,
				})
			}
		}
	}

	// Sort for stable output
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Instance != entries[j].Instance {
			return entries[i].Instance < entries[j].Instance
		}
		return entries[i].Service < entries[j].Service
	})

	return PlatformTenantsResult{Project: plan.Project, Tenants: entries}, output.ExitOK, nil
}

// runPlatformLogs streams logs from the platform compose project.
// Passes remaining args directly to docker compose logs so standard flags
// (--follow, --tail, service name) all work.
func runPlatformLogs(ctx *Context) (any, int, error) {
	plan, err := buildPlatformPlan()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if plan.Skipped {
		return PlatformResult{Action: "logs", Skipped: true, Reason: plan.SkipReason}, output.ExitNoop, nil
	}
	if _, err := os.Stat(plan.ComposeFile); errors.Is(err, os.ErrNotExist) {
		return nil, output.ExitConfig, fmt.Errorf("platform compose file not found — run 'docktree platform up' first")
	}
	// ctx.Args is ["platform", "logs", ...rest]
	logsArgs := append([]string{"logs"}, ctx.Args[2:]...)
	cmd := docker.ComposeCommand{
		ProjectName: plan.Project,
		Files:       []string{plan.ComposeFile},
		CommandArgs: logsArgs,
	}
	if err := docker.Run(cmd, ctx.Stdout, ctx.Stderr); err != nil {
		return nil, output.ExitDocker, err
	}
	return nil, output.ExitOK, nil
}

// runPlatformClean stops the platform stack, drops all known tenant databases,
// and removes the external network. Requires --yes; destructive and
// irreversible.
func runPlatformClean(ctx *Context) (any, int, error) {
	args := ctx.Args[2:] // ["platform", "clean", ...rest]
	yes := false
	dryRun := false
	for _, a := range args {
		switch a {
		case "-y", "--yes":
			yes = true
		case "--dry-run":
			dryRun = true
		}
	}
	if !yes && !dryRun {
		if !ctx.Renderer.IsTTY {
			return nil, output.ExitUsage, fmt.Errorf("platform clean requires --yes or --dry-run in non-interactive mode")
		}
		fmt.Fprintf(ctx.Stdout, "%s This will stop the platform stack, drop ALL tenant databases, and remove the platform network.\n",
			tui.WarningS("!"))
		fmt.Fprintf(ctx.Stdout, "Type %s to confirm: ", tui.AccentS("yes"))
		var line string
		fmt.Fscan(os.Stdin, &line)
		if strings.TrimSpace(line) != "yes" {
			fmt.Fprintln(ctx.Stdout, "Aborted.")
			return nil, output.ExitNoop, nil
		}
	}
	plan, err := buildPlatformPlan()
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if plan.Skipped {
		return PlatformResult{Action: "clean", Skipped: true, Reason: plan.SkipReason}, output.ExitNoop, nil
	}

	if dryRun {
		fmt.Fprintf(ctx.Stdout, "Would stop:   %s\n", plan.Project)
		fmt.Fprintf(ctx.Stdout, "Would remove: %s\n", plan.Network)
		instances, _ := state.LoadGlobalInstances("")
		for _, inst := range instances {
			repoSlug := platformRepoSlugForInstance(inst.RepoRoot)
			for svcName, svc := range plan.Shared.Services {
				if svc.Tenancy != "per_database" {
					continue
				}
				for logicalName := range svc.DatabaseTargets() {
					tenantDB := provision.TenantNameForDatabase(repoSlug, inst.Name, logicalName)
					fmt.Fprintf(ctx.Stdout, "Would drop:   %s (instance %s, service %s)\n",
						tenantDB, inst.Name, svcName)
				}
			}
		}
		return nil, output.ExitOK, nil
	}

	// 1. Drop all known tenant databases while platform containers are still up.
	instances, _ := state.LoadGlobalInstances("")
	for _, inst := range instances {
		repoSlug := platformRepoSlugForInstance(inst.RepoRoot)
		for svcName, svc := range plan.Shared.Services {
			if svc.Tenancy != "per_database" {
				continue
			}
			container := plan.Project + "-" + svcName
			platformSvc, ok := plan.PlatformProject.Services[svcName]
			if !ok {
				continue
			}
			user, password := databaseCredentialsFromEnv(svc.Kind, platformSvc)
			for logicalName := range svc.DatabaseTargets() {
				tenantDB := provision.TenantNameForDatabase(repoSlug, inst.Name, logicalName)
				fmt.Fprintf(ctx.Stderr, "Dropping tenant database: %s\n", tenantDB)
				deprovCfg := provision.TenantConfig{
					Kind:       svc.Kind,
					Tenancy:    svc.Tenancy,
					TenantName: tenantDB,
					Host:       container,
					User:       user,
					Password:   password,
				}
				if err := provision.Deprovision(deprovCfg); err != nil {
					fmt.Fprintf(ctx.Stderr, "warning: drop %s: %v\n", tenantDB, err)
				}
			}
		}
	}

	// 2. Stop the platform stack.
	if _, err := os.Stat(plan.ComposeFile); err == nil {
		cmd := docker.ComposeCommand{
			ProjectName: plan.Project,
			Files:       []string{plan.ComposeFile},
			CommandArgs: []string{"down", "-v"},
		}
		_ = docker.Run(cmd, io.Discard, ctx.Stderr)
	}

	// 3. Remove the external network.
	_ = dockerSilent("network", "rm", plan.Network)

	fmt.Fprintf(ctx.Stdout, "%s Platform cleaned: %s\n", tui.OKS("✓"), plan.Project)
	return PlatformResult{Action: "clean", Project: plan.Project, Network: plan.Network}, output.ExitOK, nil
}

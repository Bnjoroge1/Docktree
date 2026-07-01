package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	dockgit "github.com/bnjoroge/docktree/internal/git"
	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/ports"
	"github.com/bnjoroge/docktree/internal/state"
	"github.com/bnjoroge/docktree/internal/tui"
)

// --- Tunnel Provider Interface ---

type TunnelProvider interface {
	Name() string
	Check() error
	// Start launches the tunnel binary with stdout/stderr directed to logPath.
	// The returned *os.File is the log file (caller must close it after Start).
	Start(targetURL, logPath string) (*exec.Cmd, *os.File, error)
	InstallHint() string
}

// --- Cloudflare Provider ---

type cloudflareProvider struct{}

func (p *cloudflareProvider) Name() string { return "cloudflare" }

func (p *cloudflareProvider) Check() error {
	_, err := exec.LookPath("cloudflared")
	return err
}

func (p *cloudflareProvider) Start(targetURL, logPath string) (*exec.Cmd, *os.File, error) {
	cmd := exec.Command("cloudflared", "tunnel", "--url", targetURL, "--no-autoupdate")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, nil, fmt.Errorf("create tunnel log: %w", err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, nil, fmt.Errorf("start cloudflared: %w", err)
	}
	return cmd, logFile, nil
}

func (p *cloudflareProvider) InstallHint() string {
	return "brew install cloudflare/cloudflare/cloudflared"
}

// --- Ngrok Provider ---

type ngrokProvider struct{}

func (p *ngrokProvider) Name() string { return "ngrok" }

func (p *ngrokProvider) Check() error {
	_, err := exec.LookPath("ngrok")
	return err
}

func (p *ngrokProvider) Start(targetURL, logPath string) (*exec.Cmd, *os.File, error) {
	cmd := exec.Command("ngrok", "http", targetURL, "--log", "stdout", "--log-format", "json")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, nil, fmt.Errorf("create tunnel log: %w", err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, nil, fmt.Errorf("start ngrok: %w", err)
	}
	return cmd, logFile, nil
}

func (p *ngrokProvider) InstallHint() string {
	return "brew install ngrok/ngrok/ngrok"
}

// --- Provider Registry ---

var tunnelProviders = map[string]TunnelProvider{
	"cloudflare": &cloudflareProvider{},
	"ngrok":      &ngrokProvider{},
}

func getTunnelProvider(name string) (TunnelProvider, error) {
	if p, ok := tunnelProviders[name]; ok {
		return p, nil
	}
	available := make([]string, 0, len(tunnelProviders))
	for k := range tunnelProviders {
		available = append(available, k)
	}
	return nil, fmt.Errorf("unknown tunnel provider %q (available: %s)", name, strings.Join(available, ", "))
}

// --- Per-Worktree Tunnel State ---

type TunnelState struct {
	PID       int    `json:"pid"`
	URL       string `json:"url"`
	Provider  string `json:"provider"`
	Port      int    `json:"port"`
	StartedAt string `json:"started_at"`
	StartTime string `json:"start_time"`
	LogPath   string `json:"log_path"`
}

func tunnelStatePath(worktreeRoot, stateDir string) string {
	if stateDir == "" {
		stateDir = ".docktree"
	}
	return filepath.Join(worktreeRoot, stateDir, "tunnel.json")
}

func LoadTunnelState(worktreeRoot, stateDir string) (*TunnelState, error) {
	data, err := os.ReadFile(tunnelStatePath(worktreeRoot, stateDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ts TunnelState
	if err := json.Unmarshal(data, &ts); err != nil {
		return nil, err
	}
	return &ts, nil
}

func saveTunnelState(worktreeRoot, stateDir string, ts *TunnelState) error {
	dir := filepath.Dir(tunnelStatePath(worktreeRoot, stateDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tunnelStatePath(worktreeRoot, stateDir), append(data, '\n'), 0o644)
}

func removeTunnelState(worktreeRoot, stateDir string) error {
	return os.Remove(tunnelStatePath(worktreeRoot, stateDir))
}

// --- CLI Types ---

type tunnelOptions struct {
	help     bool
	action   string
	port     int
	provider string
	service  string // compose service name (mutually exclusive with --port)
}

func parseTunnelOptions(args []string, cfg *config.Config) (tunnelOptions, error) {
	providerDefault := "cloudflare"
	portDefault := 0

	if cfg != nil {
		if cfg.Tunnel.Provider != "" {
			providerDefault = cfg.Tunnel.Provider
		}
		if cfg.Tunnel.Port > 0 {
			portDefault = cfg.Tunnel.Port
		}
	}
	options := tunnelOptions{provider: providerDefault, port: portDefault}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		options.action = args[0]
		args = args[1:]
	}
	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "-h", arg == "--help":
			options.help = true
		case arg == "--port" || arg == "-p":
			i++
			if i >= len(args) {
				return tunnelOptions{}, fmt.Errorf("--port requires a value")
			}
			port, err := strconv.Atoi(args[i])
			if err != nil {
				return tunnelOptions{}, fmt.Errorf("invalid port %q: %w", args[i], err)
			}
			options.port = port
		case arg == "--provider":
			i++
			if i >= len(args) {
				return tunnelOptions{}, fmt.Errorf("--provider requires a value")
			}
			options.provider = args[i]

		case strings.HasPrefix(arg, "--provider="):
			options.provider = strings.TrimPrefix(arg, "--provider=")
		case arg == "--service" || arg == "-s":
			i++
			if i >= len(args) {
				return tunnelOptions{}, fmt.Errorf("--service requires a value")
			}
			options.service = args[i]
		case strings.HasPrefix(arg, "--service="):
			options.service = strings.TrimPrefix(arg, "--service=")
		default:
			return tunnelOptions{}, fmt.Errorf("unknown tunnel flag %q", arg)
		}
		i++
	}
	return options, nil
}

// --- Result types ---

type TunnelStartResult struct {
	Instance string `json:"instance"`
	Provider string `json:"provider"`
	Service  string `json:"service,omitempty"`
	URL      string `json:"url"`
	Port     int    `json:"port"`
	PID      int    `json:"pid"`
	LogPath  string `json:"log_path"`
}

type TunnelListEntry struct {
	Instance string `json:"instance"`
	Branch   string `json:"branch"`
	Provider string `json:"provider"`
	URL      string `json:"url"`
	Port     int    `json:"port"`
	PID      int    `json:"pid"`
	Alive    bool   `json:"alive"`
}

type TunnelListResult struct {
	Entries []TunnelListEntry `json:"entries"`
}

// TunnelStatusResult is the structured --json result for tunnel status.
// TunnelStoppedResult is the structured --json result for tunnel stop.
type TunnelStoppedResult struct {
	Instance string `json:"instance"`
	PID      int    `json:"pid"`
	Provider string `json:"provider"`
}

// TunnelStatusResult is the structured --json result for tunnel status.
type TunnelStatusResult struct {
	Instance string `json:"instance"`
	Provider string `json:"provider"`
	PID      int    `json:"pid"`
	URL      string `json:"url,omitempty"`
	Port     int    `json:"port"`
	Alive    bool   `json:"alive"`
	Status   string `json:"status"` // "running", "dead", "unknown"
	Since    string `json:"since"`
	LogPath  string `json:"log_path,omitempty"`
}

// --- Commands ---

func runTunnel(ctx *Context) (any, int, error) {
	// Load docktree.yml from the repo root for tunnel defaults.
	var repoCfg *config.Config
	if repo, err := dockgit.DetectRepo(); err == nil {
		repoCfg, _ = config.Load(repo.RepoRoot)
	}
	options, err := parseTunnelOptions(ctx.Args[1:], repoCfg)
	if err != nil {
		return nil, output.ExitUsage, err
	}
	if options.help || options.action == "" {
		return tunnelHelpDoc(), output.ExitOK, nil
	}

	switch options.action {
	case "start":
		return runTunnelStart(ctx, options)
	case "stop":
		return runTunnelStop(ctx)
	case "status":
		return runTunnelStatus(ctx)
	case "list":
		return runTunnelList(ctx)
	default:
		return nil, output.ExitUsage, fmt.Errorf("unknown tunnel action %q (use start, stop, status, list)", options.action)
	}
}

// detectCurrentWorktree returns worktreeRoot, stateDir, and instance for the cwd.
func detectCurrentWorktree() (string, string, *state.Instance, error) {
	repo, err := dockgit.DetectRepo()
	if err != nil {
		return "", "", nil, err
	}
	cfg, err := loadConfigWithSharedWarnings(repo.RepoRoot, os.Stderr)
	if err != nil {
		return "", "", nil, err
	}
	stateDir := cfg.State.Directory
	inst, err := state.LoadInstance(state.StatePath(repo.WorktreeRoot, stateDir))
	if errors.Is(err, os.ErrNotExist) {
		return "", "", nil, fmt.Errorf("not a docktree worktree (run `docktree up` first)")
	}
	if err != nil {
		return "", "", nil, err
	}
	return repo.WorktreeRoot, stateDir, inst, nil
}

func runTunnelStart(ctx *Context, options tunnelOptions) (any, int, error) {
	worktreeRoot, stateDir, inst, err := detectCurrentWorktree()
	if err != nil {
		return nil, output.ExitConfig, err
	}

	var ts *TunnelState
	ts, err = LoadTunnelState(worktreeRoot, stateDir)
	if err != nil {
		return nil, output.ExitConfig, fmt.Errorf("corrupt tunnel state — remove %s and retry: %w",
			tunnelStatePath(worktreeRoot, stateDir), err)
	}
	if ts != nil && processMatchesStr(ts.PID, ts.StartTime) {
		return nil, output.ExitUsage, fmt.Errorf("tunnel already running for %s (PID %d, %s)", inst.Name, ts.PID, ts.URL)
	}

	provider, err := getTunnelProvider(options.provider)
	if err != nil {
		return nil, output.ExitUsage, err
	}
	if err := provider.Check(); err != nil {
		return nil, output.ExitConfig, fmt.Errorf("%s not installed — install with:\n  %s", provider.Name(), provider.InstallHint())
	}

	// Build tunnel log path under the global docktree config dir.
	cfgDir := state.GlobalConfigDir()
	if err := os.MkdirAll(filepath.Join(cfgDir, "tunnel-logs"), 0o755); err != nil {
		return nil, output.ExitConfig, fmt.Errorf("create tunnel log dir: %w", err)
	}
	logPath := filepath.Join(cfgDir, "tunnel-logs", inst.Name+".log")

	// --service and --port are mutually exclusive.
	if options.service != "" && options.port > 0 {
		return nil, output.ExitUsage,
			fmt.Errorf("--service and --port are mutually exclusive")
	}

	port := options.port
	var selectedService string
	if options.service != "" {
		port, selectedService, err = serviceHostPort(inst.Name, options.service)
		if err != nil {
			return nil, output.ExitConfig, err
		}
	} else if port == 0 {
		port, err = preferredExposedPort(inst.Name)
		if err != nil {
			return nil, output.ExitConfig, err
		}
		selectedService = serviceNameForPort(inst.Name, port)
	}

	targetURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Verify the target port is reachable before starting the tunnel.
	if !portReachable(port) {
		return nil, output.ExitDocker,
			fmt.Errorf("%s port %d is not reachable — worktree appears stopped; run `docktree up` first", selectedService, port)
	}

	steps := ctx.Steps
	var cmd *exec.Cmd
	var logFile *os.File
	var capturedURL string

	if steps != nil {
		spin := steps.StartSpin(fmt.Sprintf("starting %s tunnel for %s…", provider.Name(), inst.Name))
		cmd, logFile, err = provider.Start(targetURL, logPath)
		if err != nil {
			spin.Stop()
			return nil, output.ExitDocker, err
		}
		logFile.Close() // parent doesn't need the handle; child inherited the fd
		capturedURL = captureTunnelURL(cmd, logPath, inst.Name, 8*time.Second)
		spin.Stop()
	} else {
		cmd, logFile, err = provider.Start(targetURL, logPath)
		if err != nil {
			return nil, output.ExitDocker, err
		}
		logFile.Close()
		capturedURL = captureTunnelURL(cmd, logPath, inst.Name, 8*time.Second)
	}

	if capturedURL == "" {
		// URL capture failed — grab the last few log lines for diagnostics.
		tail := tailLogFile(logPath, 8)
		_ = cmd.Process.Signal(syscall.SIGTERM)
		if steps != nil {
			steps.Done("tunnel failed to start")
			if len(tail) > 0 {
				steps.Sub(tui.ErrorS("last log lines:"))
				for _, line := range tail {
					steps.Sub("  " + tui.MutedS(line))
				}
			}
		} else if !ctx.Renderer.JSON {
			fmt.Fprintf(ctx.Stdout, "Tunnel failed to start for %s via %s (no URL captured).\n", inst.Name, provider.Name())
			if len(tail) > 0 {
				fmt.Fprintf(ctx.Stdout, "  Last log lines:\n")
				for _, line := range tail {
					fmt.Fprintf(ctx.Stdout, "    %s\n", line)
				}
			}
			fmt.Fprintf(ctx.Stderr, "  Log: %s\n", logPath)
		}
		return nil, output.ExitDocker, fmt.Errorf("tunnel failed to start (PID %d) — no URL captured; check log: %s", cmd.Process.Pid, logPath)
	}

	// Capture the process start time for PID-reuse-safe liveness checks.
	// This must succeed: without it, stop/status can't reliably identify

	startTime, err := processStartTimeStr(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		if !ctx.Renderer.JSON {
			tail := tailLogFile(logPath, 4)
			if len(tail) > 0 {
				fmt.Fprintf(ctx.Stderr, "  Last log lines:\n")
				for _, line := range tail {
					fmt.Fprintf(ctx.Stderr, "    %s\n", line)
				}
			}
		}
		return nil, output.ExitDocker,
			fmt.Errorf("cannot capture tunnel process start time (PID %d): %w — log: %s", cmd.Process.Pid, err, logPath)
	}

	newState := &TunnelState{
		PID:       cmd.Process.Pid,
		URL:       capturedURL,
		Provider:  provider.Name(),
		Port:      port,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		StartTime: startTime,
		LogPath:   logPath,
	}
	if err := saveTunnelState(worktreeRoot, stateDir, newState); err != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		return nil, output.ExitConfig, fmt.Errorf("save tunnel state: %w", err)
	}

	if steps != nil {
		steps.Done(fmt.Sprintf("tunnel for %s", tui.AccentS(inst.Name)))
		if capturedURL != "" {
			steps.Sub(fmt.Sprintf("%s %s", tui.MutedS("URL:"), tui.URLS(capturedURL)))
		}
		steps.Sub(fmt.Sprintf("%s %s → %s", tui.MutedS("route:"), tui.TextS(targetURL), tui.MutedS(provider.Name())))
	} else if ctx.Renderer.JSON {
		// JSON mode: result struct rendered by Renderer below.
	} else {

	svcLabel := ""
	if selectedService != "" {
		svcLabel = fmt.Sprintf(" (%s)", selectedService)
	}
	fmt.Fprintf(ctx.Stdout, "Tunnel started for %s%s via %s (PID %d)\n", inst.Name, svcLabel, provider.Name(), cmd.Process.Pid)
		if capturedURL != "" {
			fmt.Fprintf(ctx.Stdout, "  URL:    %s\n", capturedURL)
		}
		fmt.Fprintf(ctx.Stdout, "  Target: %s\n", targetURL)
	}

	return TunnelStartResult{
		Instance: inst.Name,
		Provider: provider.Name(),
		Service:  selectedService,
		URL:      capturedURL,
		Port:     port,
		PID:      cmd.Process.Pid,
		LogPath:  logPath,
	}, output.ExitOK, nil
}

func runTunnelStop(ctx *Context) (any, int, error) {
	worktreeRoot, stateDir, inst, err := detectCurrentWorktree()
	if err != nil {
		return nil, output.ExitConfig, err
	}

	ts, err := LoadTunnelState(worktreeRoot, stateDir)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if ts == nil {
		return nil, output.ExitNoop, fmt.Errorf("no tunnel running for %s", inst.Name)
	}


	if ts.StartTime == "" || !processMatchesStr(ts.PID, ts.StartTime) {
		removeTunnelState(worktreeRoot, stateDir)
		return nil, output.ExitNoop, fmt.Errorf("tunnel process (PID %d) already exited", ts.PID)
	}

	p, err := os.FindProcess(ts.PID)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		return nil, output.ExitDocker, err
	}
	removeTunnelState(worktreeRoot, stateDir)

	if ctx.Steps != nil {
		ctx.Steps.Done(fmt.Sprintf("tunnel stopped for %s", tui.AccentS(inst.Name)))
	} else if ctx.Renderer.JSON {
		return TunnelStoppedResult{
			Instance: inst.Name,
			PID:      ts.PID,
			Provider: ts.Provider,
		}, output.ExitOK, nil
	} else {
		fmt.Fprintf(ctx.Stdout, "Tunnel stopped for %s (PID %d)\n", inst.Name, ts.PID)
	}
	return nil, output.ExitOK, nil
}

func runTunnelStatus(ctx *Context) (any, int, error) {
	worktreeRoot, stateDir, inst, err := detectCurrentWorktree()
	if err != nil {
		return nil, output.ExitConfig, err
	}

	ts, err := LoadTunnelState(worktreeRoot, stateDir)
	if err != nil {
		return nil, output.ExitConfig, err
	}
	if ts == nil {
		if ctx.Renderer.JSON {
			return TunnelStatusResult{
				Instance: inst.Name,
				Status:   "none",
			}, output.ExitOK, nil
		}
		fmt.Fprintf(ctx.Stdout, "No tunnel running for %s.\n", inst.Name)
		return nil, output.ExitNoop, nil
	}

	var status string
	var rawStatus string
	var alive bool

	if ts.StartTime == "" {
		status = tui.WarningS("unknown")
		rawStatus = "unknown"
	} else if alive = processMatchesStr(ts.PID, ts.StartTime); alive {
		status = tui.OKS("running")
		rawStatus = "running"
	} else {
		status = tui.ErrorS("dead")
		rawStatus = "dead"
	}

	if ctx.Renderer.JSON {
		return TunnelStatusResult{
			Instance: inst.Name,
			Provider: ts.Provider,
			PID:      ts.PID,
			URL:      ts.URL,
			Port:     ts.Port,
			Alive:    alive,
			Status:   rawStatus,
			Since:    ts.StartedAt,
			LogPath:  ts.LogPath,
		}, output.ExitOK, nil
	}

	fmt.Fprintf(ctx.Stdout, "%s %s\n", tui.BrandS("Tunnel"), status)
	fmt.Fprintf(ctx.Stdout, "  %s %s\n", tui.DimS("Instance:"), tui.MutedS(inst.Name))
	fmt.Fprintf(ctx.Stdout, "  %s %s\n", tui.DimS("Provider:"), tui.MutedS(ts.Provider))
	fmt.Fprintf(ctx.Stdout, "  %s %d\n", tui.DimS("PID:"), ts.PID)
	if ts.URL != "" {
		fmt.Fprintf(ctx.Stdout, "  %s %s\n", tui.DimS("URL:"), tui.URLS(ts.URL))
	}
	fmt.Fprintf(ctx.Stdout, "  %s %s\n", tui.DimS("Port:"), tui.AccentS(fmt.Sprintf("%d", ts.Port)))
	fmt.Fprintf(ctx.Stdout, "  %s %s\n", tui.DimS("Since:"), tui.MutedS(ts.StartedAt))
	if ts.LogPath != "" {
		fmt.Fprintf(ctx.Stdout, "  %s %s\n", tui.DimS("Log:"), tui.MutedS(ts.LogPath))
	}
	return nil, output.ExitOK, nil
}

func runTunnelList(ctx *Context) (any, int, error) {
	instances, err := state.LoadGlobalInstances("")
	if err != nil {
		return nil, output.ExitConfig, err
	}

	var entries []TunnelListEntry
	for _, inst := range instances {
		ts, _ := LoadTunnelState(inst.WorktreeRoot, inst.StateDirectory)
		if ts == nil {
			continue
		}

		alive := ts.StartTime != "" && processMatchesStr(ts.PID, ts.StartTime)
		entries = append(entries, TunnelListEntry{
			Instance: inst.Name,
			Branch:   inst.Branch,
			Provider: ts.Provider,
			URL:      ts.URL,
			Port:     ts.Port,
			PID:      ts.PID,
			Alive:    alive,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Instance < entries[j].Instance
	})

	return TunnelListResult{Entries: entries}, output.ExitOK, nil
}

// preferredExposedPort picks the best allocated HTTP port for an instance.
//
// Prefers container ports 80 or 8080; falls back to the first non-zero
// host port.
func preferredExposedPort(instanceName string) (int, error) {
	registry, err := ports.NewRegistry().Load()
	if err != nil {
		return 0, err
	}
	assignments := registry[instanceName]
	var fallback int
	for _, a := range assignments {
		if a.HostPort == 0 {
			continue
		}
		if fallback == 0 {
			fallback = a.HostPort
		}
		if a.ContainerPort == 80 || a.ContainerPort == 8080 {
			return a.HostPort, nil
		}
	}
	if fallback > 0 {
		return fallback, nil
	}
	return 0, fmt.Errorf("no exposed ports for instance %s (run `docktree up` first)", instanceName)
}

// serviceHostPort returns the host port for a named compose service.
//
// If the service has multiple ports, prefers 80/8080.  Otherwise returns
// an error listing the available ports and suggesting --port.
func serviceHostPort(instanceName, serviceName string) (int, string, error) {
	registry, err := ports.NewRegistry().Load()
	if err != nil {
		return 0, "", err
	}
	var candidates []ports.Assignment
	for _, a := range registry[instanceName] {
		if a.Service == serviceName && a.HostPort > 0 {
			candidates = append(candidates, a)
		}
	}
	if len(candidates) == 0 {
		return 0, "", fmt.Errorf("service %q not found in port registry for %s (run `docktree up` first)", serviceName, instanceName)
	}
	// Single port — use it.
	if len(candidates) == 1 {
		return candidates[0].HostPort, fmt.Sprintf("%s:%d", candidates[0].Service, candidates[0].ContainerPort), nil
	}
	// Multiple ports — prefer HTTP.
	for _, a := range candidates {
		if a.ContainerPort == 80 || a.ContainerPort == 8080 {
			return a.HostPort, fmt.Sprintf("%s:%d", a.Service, a.ContainerPort), nil
		}
	}
	// Multiple non-HTTP ports — fail with a list.
	var parts []string
	for _, a := range candidates {
		parts = append(parts, fmt.Sprintf(":%d → %d", a.ContainerPort, a.HostPort))
	}
	return 0, "", fmt.Errorf("service %q has multiple ports (%s); specify one with --port", serviceName, strings.Join(parts, ", "))
}

// serviceNameForPort returns a human-readable label for a host port.
func serviceNameForPort(instanceName string, hostPort int) string {
	registry, err := ports.NewRegistry().Load()
	if err != nil {
		return ""
	}
	for _, a := range registry[instanceName] {
		if a.HostPort == hostPort {
			return fmt.Sprintf("%s:%d", a.Service, a.ContainerPort)
		}
	}
	return ""
}

// captureTunnelURL polls the log file for a tunnel URL, with a concurrent
// check for early child exit.
//
// Returns the captured URL or an empty string if the child exits first or
// the poll window expires without a match.
func captureTunnelURL(cmd *exec.Cmd, logPath, instanceName string, timeout time.Duration) string {
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	deadline := time.After(timeout)
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			return "" // timed out waiting for URL
		case <-waitCh:
			return "" // child exited early
		case <-tick.C:
			if url := scanLogForURL(logPath); url != "" {
				return url
			}
		}
	}
}

// scanLogForURL reads the log file and returns the first tunnel URL found.
func scanLogForURL(logPath string) string {
	f, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		if url := extractTunnelURL(scanner.Text()); url != "" {
			return url
		}
	}
	return ""
}


func extractTunnelURL(line string) string {
	// Find every https:// URL in the line and return the first one
	// whose host matches a known provider domain.
	for _, domain := range []string{"trycloudflare.com", "ngrok-free.app", "ngrok.io"} {
		start := 0
		for {
			idx := strings.Index(line[start:], "https://")
			if idx < 0 {
					break
			}
			idx += start
			// Find end of URL (delimiter or line end).
			end := idx
			for end < len(line) {
				c := line[end]
				if c == ' ' || c == '\n' || c == '\r' || c == '"' ||
					c == ',' || c == '}' || c == '|' || c == '<' || c == '\'' {
					break
				}
				end++
			}
			url := line[idx:end]
			// Only return if the URL's host matches the provider domain.
			if strings.Contains(url, domain) {
				return url
			}
			start = end
		}
	}
	return ""
}

// tailLogFile returns the last n non-empty lines from a file.
func tailLogFile(path string, n int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// processStartTimeStr returns the raw ps lstart output for pid,
// or an empty string if it cannot be determined.
//
// Storing and comparing the raw string avoids format ambiguity from
// ps's varying date layouts (space-padded days, locale differences).
func processStartTimeStr(pid int) (string, error) {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "lstart=").Output()
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return "", fmt.Errorf("ps returned empty lstart for pid %d", pid)
	}
	return s, nil
}

// processMatchesStr verifies that pid exists AND its ps lstart matches.
//
// String comparison avoids parsing pitfalls from ps's varying date
// formats.  PID reuse yields a different start time → mismatch → false.
func processMatchesStr(pid int, startTime string) bool {
	actual, err := processStartTimeStr(pid)
	if err != nil {
		return false
	}
	return actual == startTime
}

// portReachable checks whether a TCP port is listening on localhost.
func portReachable(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func printTunnelHelp(w interface{ Write([]byte) (int, error) }) {
	providers := make([]string, 0, len(tunnelProviders))
	for k := range tunnelProviders {
		providers = append(providers, k)
	}

	fmt.Fprintf(w, `Usage: docktree tunnel <action> [flags]

Expose a worktree externally via a tunnel provider.

Each worktree gets its own independent tunnel. Run from the
worktree directory to start/stop that worktree's tunnel.

Actions:
  start    Start a tunnel for the current worktree
  stop     Stop the current worktree's tunnel
  status   Show current worktree's tunnel status
  list     Show all running tunnels across worktrees

Flags:
  --provider NAME   Tunnel provider (default: cloudflare)
  --port, -p PORT   Local port to tunnel (default: first allocated port)
 --service, -s SVC Compose service to tunnel (picks HTTP port; use --port for specific ports)
  --help, -h        Show this help

Available providers: %s

Examples:
  cd myapp.worktrees/feature-a && docktree tunnel start                      # auto-detect first HTTP port
  docktree tunnel start --service ui                                         # tunnel a specific service
  docktree tunnel start --port 41006                                         # tunnel an explicit port
  docktree tunnel status
  docktree tunnel stop
  docktree tunnel list
`, strings.Join(providers, ", "))
}

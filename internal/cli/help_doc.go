package cli

import (
	"fmt"
	"io"

	"github.com/bnjoroge/docktree/internal/tui"
)

// HelpDoc is the structured form of a command's help text. It is rendered as
// JSON under `--json` and as the legacy text under the human renderer.
type HelpDoc struct {
	Command     string       `json:"command"`            // "" for root, "platform" for the platform group, etc.
	Synopsis    string       `json:"synopsis,omitempty"` // one-line description
	Usage       []string     `json:"usage,omitempty"`    // each entry is a usage line
	Options     []HelpOption `json:"options,omitempty"`
	Arguments   []HelpArg    `json:"arguments,omitempty"`
	Subcommands []HelpCmd    `json:"subcommands,omitempty"` // root / platform group
	Examples    []string     `json:"examples,omitempty"`
	Passthrough bool         `json:"passthrough,omitempty"` // pass-through to docker compose
	Notes       []string     `json:"notes,omitempty"`
	// GlobalFlags is set on root help only.
	GlobalFlags []HelpOption `json:"global_flags,omitempty"`
}

type HelpOption struct {
	Flags       []string `json:"flags"`           // e.g. ["-a", "--all"]
	Value       string   `json:"value,omitempty"` // e.g. "<path>"
	Description string   `json:"description"`
}

type HelpArg struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Optional    bool   `json:"optional,omitempty"`
	Repeatable  bool   `json:"repeatable,omitempty"`
}

type HelpCmd struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// VersionInfo is the structured form of `docktree version`.
type VersionInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// helpTextPrinters maps a HelpDoc.Command to the legacy human-mode printer.
// Side-by-side with the structured docs so we don't reformat any text.
var helpTextPrinters = map[string]func(io.Writer){
	"":         printHelp,
	"clean":    printCleanHelp,
	"create":   printCreateHelp,
	"down":     printDownHelp,
	"platform": printPlatformHelp,
	"ports":    printPortsHelp,
	"prepare":  printPrepareHelp,
	"proxy":    func(w io.Writer) { printProxyHelp(w) },
	"status":   printStatusHelp,
	"tunnel":   func(w io.Writer) { printTunnelHelp(w) },
	"stop":     printStopHelp,
	"sync":     printSyncHelp,
	"up":       printUpHelp,
	"volumes":  printVolumesHelp,
}

// renderHelpText writes a HelpDoc using the legacy human-mode printer. Falls
// back to a generic renderer for any command not in the table (shouldn't
// happen for native commands).
func renderHelpText(w io.Writer, doc HelpDoc) {
	if printer, ok := helpTextPrinters[doc.Command]; ok {
		printer(w)
		return
	}
	// Generic fallback: render from the structured fields. Used only if a new
	// native command forgets to register a text printer.
	if doc.Synopsis != "" {
		fmt.Fprintln(w, doc.Synopsis)
		fmt.Fprintln(w)
	}
	if len(doc.Usage) > 0 {
		fmt.Fprintln(w, "Usage:")
		for _, line := range doc.Usage {
			fmt.Fprintln(w, "  "+line)
		}
	}
}

// renderVersionText writes the legacy human-mode version string.
func renderVersionText(w io.Writer, v VersionInfo) {
	fmt.Fprintf(w, "%s\n", tui.MutedS(v.Name+" "+v.Version))
}

// ---- structured doc constructors --------------------------------------------

func rootHelpDoc() HelpDoc {
	return HelpDoc{
		Command:  "",
		Synopsis: "docktree coordinates Docker Compose services across git worktrees.",
		Usage:    []string{"docktree [--json] <command>"},
		GlobalFlags: []HelpOption{
			{Flags: []string{"--json"}, Description: "Emit machine-readable JSON instead of formatted text"},
		},
		Subcommands: []HelpCmd{
			{Name: "build", Description: "Pass through to docker compose build"},
			{Name: "clean", Description: "Remove stale Docktree-managed resources"},
			{Name: "config", Description: "Pass through to docker compose config"},
			{Name: "cp", Description: "Pass through to docker compose cp"},
			{Name: "create", Description: "Create a worktree and prepare its local Docker setup"},
			{Name: "docker", Description: "Run any docker compose subcommand with worktree context pre-filled"},
			{Name: "down", Description: "Stop the current worktree's Compose project (or specific services)"},
			{Name: "exec", Description: "Pass through to docker compose exec"},
			{Name: "images", Description: "Pass through to docker compose images"},
			{Name: "init", Description: "Generate a docktree.yml from your compose files"},
			{Name: "kill", Description: "Pass through to docker compose kill"},
			{Name: "logs", Description: "Pass through to docker compose logs"},
			{Name: "pause", Description: "Pass through to docker compose pause"},
			{Name: "platform", Description: "Manage the repo-scoped shared services platform"},
			{Name: "port", Description: "Pass through to docker compose port"},
			{Name: "ports", Description: "Show allocated host ports (use --all for all worktrees)"},
			{Name: "prepare", Description: "Prepare the current worktree's local Docker setup"},
			{Name: "pull", Description: "Pass through to docker compose pull"},
			{Name: "push", Description: "Pass through to docker compose push"},
			{Name: "restart", Description: "Pass through to docker compose restart"},
			{Name: "rm", Description: "Pass through to docker compose rm"},
			{Name: "run", Description: "Pass through to docker compose run --rm"},
			{Name: "start", Description: "Pass through to docker compose start"},
			{Name: "status", Description: "Show managed worktree services"},
			{Name: "stop", Description: "Stop running containers without removing them"},
			{Name: "sync", Description: "Sync setup.copy files to all worktrees"},
			{Name: "top", Description: "Pass through to docker compose top"},
			{Name: "unpause", Description: "Pass through to docker compose unpause"},
			{Name: "up", Description: "Start the current worktree's Compose project (or --create <branch>)"},
			{Name: "volumes", Description: "Show Docktree-managed volumes (use --all for all worktrees)"},
			{Name: "proxy", Description: "Reverse proxy routing by hostname to worktree ports"},
			{Name: "tunnel", Description: "Expose worktrees externally via Cloudflare Tunnel or ngrok"},
			{Name: "wait", Description: "Pass through to docker compose wait"},
			{Name: "watch", Description: "Pass through to docker compose watch"},
			{Name: "help", Description: "Show this help text"},
			{Name: "version", Description: "Print the docktree version"},
		},
	}
}

func cleanHelpDoc() HelpDoc {
	return HelpDoc{
		Command:  "clean",
		Synopsis: "Remove stale Docktree-managed resources for missing or idle worktrees.",
		Usage:    []string{"docktree clean [options]"},
		Options: []HelpOption{
			{Flags: []string{"--dry-run"}, Description: "Show stale resources without removing them"},
			{Flags: []string{"--yes"}, Description: "Skip the interactive confirmation prompt"},
			{Flags: []string{"--volumes"}, Description: "Include Docker volumes when discovering and removing stale resources"},
			{Flags: []string{"-h", "--help"}, Description: "Show this help text"},
		},
	}
}

func createHelpDoc() HelpDoc {
	return HelpDoc{
		Command:  "create",
		Synopsis: "Create a new git worktree for <branch> and prepare its local Docker setup.",
		Usage:    []string{"docktree create <branch>"},
		Options: []HelpOption{
			{Flags: []string{"-h", "--help"}, Description: "Show this help text"},
		},
		Examples: []string{
			"docktree create feature/auth",
			"docktree create bugfix/api-timeout",
		},
	}
}

func downHelpDoc() HelpDoc {
	return HelpDoc{
		Command:  "down",
		Synopsis: "Stop the current worktree's Compose project, or specific services.",
		Usage:    []string{"docktree down [options] [service...]"},
		Options: []HelpOption{
			{Flags: []string{"-v", "--volumes"}, Description: "Drop per-worktree tenant databases and Docker volumes. Data is permanently deleted."},
			{Flags: []string{"-a", "--all"}, Description: "Apply to all worktree instances in this repository. Combine with -v to drop volumes across all worktrees at once."},
			{Flags: []string{"--dry-run"}, Description: "Show what would be stopped without making changes"},
			{Flags: []string{"-h", "--help"}, Description: "Show this help text"},
		},
		Arguments: []HelpArg{
			{Name: "service", Description: "One or more service names to stop (default: all services)", Optional: true, Repeatable: true},
		},
	}
}

func platformHelpDoc() HelpDoc {
	return HelpDoc{
		Command: "platform",
		Usage:   []string{"docktree platform <command>"},
		Subcommands: []HelpCmd{
			{Name: "up", Description: "Start the repo-scoped platform stack"},
			{Name: "down", Description: "Stop the repo-scoped platform stack (preserves data)"},
			{Name: "status", Description: "Show platform stack state"},
			{Name: "tenants", Description: "List tenant databases across all instances"},
			{Name: "logs", Description: "Stream platform service logs (pass service name to filter)"},
			{Name: "clean", Description: "Stop platform, drop all tenant DBs, remove network (--yes required)"},
		},
		Notes: []string{
			"The platform stack runs services marked in `shared.services` of docktree.yml.",
			"Worktrees reach them via Docker DNS on the platform external network.",
		},
	}
}

func portsHelpDoc() HelpDoc {
	return HelpDoc{
		Command:  "ports",
		Synopsis: "Show allocated host ports.",
		Usage:    []string{"docktree ports [options]"},
		Options: []HelpOption{
			{Flags: []string{"-a", "--all"}, Description: "Show ports for all worktree instances"},
			{Flags: []string{"-h", "--help"}, Description: "Show this help text"},
		},
	}
}

func prepareHelpDoc() HelpDoc {
	return HelpDoc{
		Command:  "prepare",
		Synopsis: "Prepare the current worktree's local Docker setup from docktree.yml.",
		Usage:    []string{"docktree prepare [options]"},
		Options: []HelpOption{
			{Flags: []string{"-h", "--help"}, Description: "Show this help text"},
		},
		Examples: []string{"docktree prepare"},
	}
}

func statusHelpDoc() HelpDoc {
	return HelpDoc{
		Command:  "status",
		Synopsis: "Show managed worktree services.",
		Usage:    []string{"docktree status [options]"},
		Options: []HelpOption{
			{Flags: []string{"-a", "--all"}, Description: "Show status for all worktree instances"},
			{Flags: []string{"-h", "--help"}, Description: "Show this help text"},
		},
		Examples: []string{
			"docktree status",
			"docktree status --all",
		},
	}
}

func stopHelpDoc() HelpDoc {
	return HelpDoc{
		Command:  "stop",
		Synopsis: "Stop running containers without removing them (unlike down).",
		Usage:    []string{"docktree stop [options] [service...]"},
		Options: []HelpOption{
			{Flags: []string{"--dry-run"}, Description: "Show what would be stopped without making changes"},
			{Flags: []string{"-h", "--help"}, Description: "Show this help text"},
		},
		Arguments: []HelpArg{
			{Name: "service", Description: "One or more service names to stop (default: all services)", Optional: true, Repeatable: true},
		},
	}
}

func syncHelpDoc() HelpDoc {
	return HelpDoc{
		Command:  "sync",
		Synopsis: "Sync setup.copy files from the main repo to all worktrees. Reports which files are stale (different or missing) and copies them. Skips worktrees that are currently running.",
		Usage:    []string{"docktree sync [options]"},
		Options: []HelpOption{
			{Flags: []string{"--dry-run"}, Description: "Show what would be synced without copying"},
			{Flags: []string{"--force"}, Description: "Sync without confirmation prompt"},
			{Flags: []string{"-h", "--help"}, Description: "Show this help text"},
		},
	}
}

func upHelpDoc() HelpDoc {
	return HelpDoc{
		Command:  "up",
		Synopsis: "Start the current worktree's Compose project. When services are provided, only those services are started; overrides and port allocations are still generated for the full project.",
		Usage:    []string{"docktree up [options] [service...]"},
		Options: []HelpOption{
			{Flags: []string{"-f", "--file"}, Value: "<path>", Description: "Use a specific Compose file"},
			{Flags: []string{"--only"}, Value: "<service>", Description: "Start only the named service (repeatable)"},
			{Flags: []string{"--skip"}, Value: "<service>", Description: "Skip a service and save it for this worktree (repeatable)"},
			{Flags: []string{"--skip-clear"}, Description: "Clear all saved skipped services for this worktree"},
			{Flags: []string{"--profile"}, Value: "<name>", Description: "Activate a Compose profile for this run only (repeatable, not saved)"},
			{Flags: []string{"--build"}, Description: "Force rebuild of images with a build: directive"},
			{Flags: []string{"--create"}, Value: "<branch>", Description: "Create and prepare a new worktree before starting"},
			{Flags: []string{"--sync"}, Description: "Run setup copy/symlink/run steps before starting"},
			{Flags: []string{"--validate"}, Description: "Check config, ports, and compose validity without starting"},
			{Flags: []string{"--dry-run"}, Description: "Show what would happen without making changes"},
			{Flags: []string{"--prune-networks"}, Description: "Prune unused Docker networks before starting"},
			{Flags: []string{"-h", "--help"}, Description: "Show this help text"},
		},
		Examples: []string{
			"docktree up",
			"docktree up db redis",
			"docktree up --only db",
		},
	}
}

func volumesHelpDoc() HelpDoc {
	return HelpDoc{
		Command:  "volumes",
		Synopsis: "Show Docktree-managed volumes.",
		Usage:    []string{"docktree volumes [options]"},
		Options: []HelpOption{
			{Flags: []string{"-a", "--all"}, Description: "Show volumes for all worktree instances"},
			{Flags: []string{"-h", "--help"}, Description: "Show this help text"},
		},
	}
}
func proxyHelpDoc() HelpDoc {
	return HelpDoc{
		Command:  "proxy",
		Synopsis: "Start a reverse proxy that routes by hostname to worktree ports.",
		Usage:    []string{"docktree proxy [--port PORT] [--host HOST]"},
		Options: []HelpOption{
			{Flags: []string{"-p", "--port"}, Value: "PORT", Description: "Proxy listen port (default: 8320)"},
			{Flags: []string{"--host"}, Value: "HOST", Description: "Proxy listen host (default: 127.0.0.1)"},
			{Flags: []string{"-h", "--help"}, Description: "Show this help text"},
		},
		Examples: []string{
			"docktree proxy",
			"docktree proxy --port 9000",
		},
	}
}
func tunnelHelpDoc() HelpDoc {
	return HelpDoc{
		Command:  "tunnel",
		Synopsis: "Expose a worktree externally via a tunnel provider.",
		Usage:    []string{"docktree tunnel <action> [flags]"},
		Subcommands: []HelpCmd{
			{Name: "start", Description: "Start a tunnel for the current worktree"},
			{Name: "stop", Description: "Stop the current worktree's tunnel"},
			{Name: "status", Description: "Show current worktree's tunnel status"},
			{Name: "list", Description: "Show all running tunnels across worktrees"},
		},
		Options: []HelpOption{
			{Flags: []string{"--provider"}, Value: "NAME", Description: "Tunnel provider (default: cloudflare)"},
			{Flags: []string{"-p", "--port"}, Value: "PORT", Description: "Local port to tunnel (default: first allocated port)"},
			{Flags: []string{"-s", "--service"}, Value: "SVC", Description: "Compose service to tunnel"},
			{Flags: []string{"-h", "--help"}, Description: "Show this help text"},
		},
		Examples: []string{
			"docktree tunnel start",
			"docktree tunnel start --service ui",
			"docktree tunnel start --port 41006",
			"docktree tunnel status",
			"docktree tunnel stop",
			"docktree tunnel list",
		},
	}
}

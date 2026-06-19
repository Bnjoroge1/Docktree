package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/bnjoroge/docktree/internal/tui"
)

func printHelp(w io.Writer) {
	maxCmd := 9
	fmt.Fprintf(w, "%s\n\n", tui.MutedS("docktree coordinates Docker Compose services across git worktrees."))
	fmt.Fprintf(w, "%s\n", tui.TextS("Usage:"))
	fmt.Fprintf(w, "  %s\n\n", tui.AccentS("docktree [--json] <command>"))
	fmt.Fprintf(w, "%s\n", tui.TextS("Commands:"))
	printHelpCmd(w, maxCmd, "create", "Create a worktree and prepare its local Docker setup")
	printHelpCmd(w, maxCmd, "up", "Start the current worktree's Compose project (or --create <branch>)")
	printHelpCmd(w, maxCmd, "down", "Stop the current worktree's Compose project (or specific services)")
	printHelpCmd(w, maxCmd, "stop", "Stop running containers without removing them")
	printHelpCmd(w, maxCmd, "restart", "Pass through to docker compose restart")
	printHelpCmd(w, maxCmd, "start", "Pass through to docker compose start")
	printHelpCmd(w, maxCmd, "rm", "Pass through to docker compose rm")
	printHelpCmd(w, maxCmd, "pause", "Pass through to docker compose pause")
	printHelpCmd(w, maxCmd, "unpause", "Pass through to docker compose unpause")
	printHelpCmd(w, maxCmd, "kill", "Pass through to docker compose kill")
	printHelpCmd(w, maxCmd, "docker", "Run any docker compose subcommand with worktree context pre-filled")
	printHelpCmd(w, maxCmd, "logs", "Pass through to docker compose logs")
	printHelpCmd(w, maxCmd, "exec", "Pass through to docker compose exec")
	printHelpCmd(w, maxCmd, "run", "Pass through to docker compose run --rm")
	printHelpCmd(w, maxCmd, "build", "Pass through to docker compose build")
	printHelpCmd(w, maxCmd, "pull", "Pass through to docker compose pull")
	printHelpCmd(w, maxCmd, "push", "Pass through to docker compose push")
	printHelpCmd(w, maxCmd, "status", "Show managed worktree services")
	printHelpCmd(w, maxCmd, "ports", "Show allocated host ports (use --all for all worktrees)")
	printHelpCmd(w, maxCmd, "prepare", "Prepare the current worktree's local Docker setup")
	printHelpCmd(w, maxCmd, "platform", "Manage the repo-scoped shared services platform")
	printHelpCmd(w, maxCmd, "clean", "Remove stale Docktree-managed resources")
	printHelpCmd(w, maxCmd, "config", "Pass through to docker compose config")
	printHelpCmd(w, maxCmd, "images", "Pass through to docker compose images")
	printHelpCmd(w, maxCmd, "top", "Pass through to docker compose top")
	printHelpCmd(w, maxCmd, "ls", "Pass through to docker compose ls (all projects)")
	printHelpCmd(w, maxCmd, "port", "Pass through to docker compose port")
	printHelpCmd(w, maxCmd, "help", "Show this help text")
	printHelpCmd(w, maxCmd, "version", "Print the docktree version")
}

func printHelpCmd(w io.Writer, max int, cmd, desc string) {
	pad := strings.Repeat(" ", max-len(cmd)+2)
	fmt.Fprintf(w, "  %s%s%s\n", tui.OKS(cmd), pad, desc)
}

func printPortsHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree ports [options]

Show allocated host ports.

Options:
  -a, --all    Show ports for all worktree instances
  -h, --help   Show this help text`)
}

func printDownHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree down [options] [service...]

Stop the current worktree's Compose project, or specific services.

Options:
  -v, --volumes  Drop per-worktree tenant databases and Docker volumes.
                 Data is permanently deleted.
  -a, --all      Apply to all worktree instances in this repository.
                 Combine with -v to drop volumes across all worktrees at once.
  --dry-run      Show what would be stopped without making changes
  -h, --help     Show this help text

Arguments:
  service        One or more service names to stop (default: all services)`)
}

func printStopHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree stop [options] [service...]

Stop running containers without removing them (unlike down).

Options:
  --dry-run    Show what would be stopped without making changes
  -h, --help   Show this help text

Arguments:
  service      One or more service names to stop (default: all services)`)
}

func printLogsHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree logs [options] [service...]

Pass through to docker compose logs with the current worktree's project context.

Options and arguments are passed through directly to docker compose logs.
Common options: --follow (-f), --tail N, --since, --timestamps

Examples:
  docktree logs                  # tail all services
  docktree logs api             # tail the api service
  docktree logs api --tail 50   # last 50 lines
  docktree logs -f db            # follow db logs`)
}

func printExecHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree exec <service> -- <command> [args...]

Pass through to docker compose exec with the current worktree's project context.
Options and arguments are passed through directly to docker compose exec.

Examples:
  docktree exec db -- psql -U postgres
  docktree exec api -- sh
  docktree exec --index 1 api -- bash`)
}

func printRunHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree run [options] <service> -- <command> [args...]

Pass through to docker compose run --rm with the current worktree's project context.
Containers are removed after the command exits (--rm is always included).
Options and arguments are passed through directly to docker compose run.

Examples:
  docktree run api -- rake db:migrate
  docktree run db -- psql -U postgres
  docktree run --no-deps api -- rspec`)
}

func printConfigHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree config [options]

Pass through to docker compose config with the current worktree's compose files.
Options and arguments are passed through directly to docker compose config.

Examples:
  docktree config                    # dump resolved compose config
  docktree config --format json      # output as JSON
  docktree config --services         # list services`)
}

func printImagesHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree images [options]

Pass through to docker compose images with the current worktree's project context.
Options and arguments are passed through directly to docker compose images.

Examples:
  docktree images              # list images used by the project
  docktree images api          # filter by service name
  docktree images -q           # quiet mode, show image IDs only`)
}

func printTopHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree top [options] [service...]

Pass through to docker compose top with the current worktree's project context.
Options and arguments are passed through directly to docker compose top.

Examples:
  docktree top                # show running processes for all services
  docktree top api            # show processes for the api service`)
}

func printLsHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree ls [options]

Pass through to docker compose ls to list all Compose projects.
This lists ALL compose projects on the system, not just the current worktree's.

Options and arguments are passed through directly to docker compose ls.

Examples:
  docktree ls                   # list all projects
  docktree ls -a                # include stopped projects
  docktree ls --format json     # output as JSON`)
}

func printPortHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree port [options] <service> <private_port>

Pass through to docker compose port with the current worktree's project context.
Options and arguments are passed through directly to docker compose port.

Examples:
  docktree port api 3000          # show host mapping for port 3000
  docktree port --protocol tcp db 5432`)
}

func printUpHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree up [options]

Start the current worktree's Compose project.

Options:
  -f, --file <path>     Use a specific Compose file
  --build               Force rebuild of images with a build: directive
  --create <branch>     Create and prepare a new worktree before starting
  --sync                Run setup copy/symlink/run steps before starting
  --validate            Check config, ports, and compose validity without starting
  --dry-run             Show what would happen without making changes
  -h, --help            Show this help text`)
}

func printBuildHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree build [options] [service...]

Pass through to docker compose build with the current worktree's project context.
Options and arguments are passed through directly to docker compose build.

Examples:
  docktree build                # build all services
  docktree build api            # build the api service
  docktree build --no-cache api # rebuild without cache`)
}

func printPullHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree pull [options] [service...]

Pass through to docker compose pull with the current worktree's project context.
Options and arguments are passed through directly to docker compose pull.

Examples:
  docktree pull              # pull all services
  docktree pull api          # pull the api service
  docktree pull --ignore-pull-failures`)
}

func printPushHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree push [options] [service...]

Pass through to docker compose push with the current worktree's project context.
Options and arguments are passed through directly to docker compose push.

Examples:
  docktree push              # push all services
  docktree push api          # push the api service
  docktree push --ignore-push-failures`)
}

package cli

import (
	"fmt"
	"io"
)

func runRestart(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "restart", ctx.Args[1:], printRestartHelp)
}

func runStart(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "start", ctx.Args[1:], printStartHelp)
}

func runRm(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "rm", ctx.Args[1:], printRmHelp)
}

func runPause(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "pause", ctx.Args[1:], printPauseHelp)
}

func runUnpause(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "unpause", ctx.Args[1:], printUnpauseHelp)
}

func runKill(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "kill", ctx.Args[1:], printKillHelp)
}

func printRestartHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree restart [service...]

Pass through to docker compose restart with the current worktree's project context.
Options and arguments are passed through directly to docker compose restart.

Examples:
  docktree restart              # restart all services
  docktree restart api          # restart the api service
  docktree restart db api       # restart db and api services`)
}

func printStartHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree start [service...]

Pass through to docker compose start with the current worktree's project context.
Options and arguments are passed through directly to docker compose start.

Examples:
  docktree start               # start all stopped services
  docktree start api           # start the api service`)
}

func printRmHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree rm [service...]

Pass through to docker compose rm with the current worktree's project context.
Options and arguments are passed through directly to docker compose rm.

Examples:
  docktree rm -f               # force remove all stopped services
  docktree rm api              # remove the api service
  docktree rm -f api db        # force remove api and db services`)
}

func printPauseHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree pause [service...]

Pass through to docker compose pause with the current worktree's project context.
Options and arguments are passed through directly to docker compose pause.

Examples:
  docktree pause               # pause all services
  docktree pause api           # pause the api service`)
}

func printUnpauseHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree unpause [service...]

Pass through to docker compose unpause with the current worktree's project context.
Options and arguments are passed through directly to docker compose unpause.

Examples:
  docktree unpause             # unpause all services
  docktree unpause api         # unpause the api service`)
}

func printKillHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  docktree kill [service...]

Pass through to docker compose kill with the current worktree's project context.
Options and arguments are passed through directly to docker compose kill.

Examples:
  docktree kill                # kill all services
  docktree kill api            # kill the api service
  docktree kill -s SIGTERM api # send SIGTERM to the api service`)
}

package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/tui"
)

var version = "0.1.0-dev"

func Run(args []string, stdout, stderr io.Writer) int {
	jsonMode, rest := parseGlobalFlags(args)
	renderer := output.New(stdout, jsonMode)
	ctx := &Context{Args: rest, Renderer: renderer, Stdout: stdout, Stderr: stderr}
	if len(rest) == 0 {
		printHelp(stdout)
		return output.ExitOK
	}
	var result any
	var code int
	var err error

	switch rest[0] {
	case "help", "-h", "--help":
		printHelp(stdout)
		return output.ExitOK
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "%s\n", tui.MutedS("docktree "+version))
		return output.ExitOK
	case "up":
		result, code, err = runWithProgress(ctx, runUp)
	case "down":
		result, code, err = runWithProgress(ctx, runDown)
	default:
		commands := map[string]commandFunc{
			"stop":     runStop,
			"logs":     runLogs,
			"exec":     runExec,
			"run":      runComposeRun,
			"docker":   runDocker,
			"status":   runStatus,
			"ports":    runPorts,
			"clean":    runClean,
			"create":   runCreate,
			"prepare":  runPrepare,
			"platform": runPlatform,
			"cp":       runCp,
			"wait":     runWait,
			"watch":    runWatch,
			"restart":  runRestart,
			"start":    runStart,
			"rm":       runRm,
			"pause":    runPause,
			"unpause":  runUnpause,
			"kill":     runKill,
		}
		fn, ok := commands[rest[0]]
		if !ok {
			fmt.Fprintf(stderr, "unknown command %q\n\n", rest[0])
			printHelp(stderr)
			return output.ExitUsage
		}
		result, code, err = fn(ctx)
	}
	if err != nil {
		target := renderer
		target.Writer = stderr
		target.Error(errorCode(code), err.Error(), nil)
		return code
	}
	if result != nil {
		renderer.Render(result, humanRenderer())
	}
	return code
}

// runWithProgress runs fn with step-by-step progress on stderr.
// Docker stdout/stderr is captured to avoid interleaving.
// Progress steps are cleared before the final result is rendered.
func runWithProgress(ctx *Context, fn commandFunc) (any, int, error) {
	isTTY := ctx.Renderer.IsTTY && !ctx.Renderer.JSON
	if !isTTY {
		return fn(ctx)
	}
	steps := tui.NewStepPrinter(os.Stderr, true)
	ctx.Steps = steps

	var stderrBuf bytes.Buffer
	oldStdout := ctx.Stdout
	oldStderr := ctx.Stderr
	ctx.Stdout = io.Discard
	ctx.Stderr = &stderrBuf
	defer func() {
		ctx.Stdout = oldStdout
		ctx.Stderr = oldStderr
		ctx.Steps = nil
	}()

	result, code, runErr := fn(ctx)

	if runErr != nil {
		steps.Clear()
		_, _ = io.Copy(oldStderr, &stderrBuf)
	} else {
		steps.Clear()
	}
	return result, code, runErr
}

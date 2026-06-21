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

type commandSpec struct {
	run      commandFunc
	progress bool
}

var rootCommands = map[string]commandSpec{
	"up":       {run: runUp, progress: true},
	"down":     {run: runDown, progress: true},
	"stop":     {run: runStop},
	"logs":     {run: runLogs},
	"exec":     {run: runExec},
	"run":      {run: runComposeRun},
	"docker":   {run: runDocker},
	"build":    {run: runBuild},
	"pull":     {run: runPull},
	"push":     {run: runPush},
	"status":   {run: runStatus},
	"ports":    {run: runPorts},
	"clean":    {run: runClean},
	"create":   {run: runCreate},
	"prepare":  {run: runPrepare},
	"platform": {run: runPlatform},
	"cp":       {run: runCp},
	"wait":     {run: runWait},
	"watch":    {run: runWatch},
	"restart":  {run: runRestart},
	"start":    {run: runStart},
	"rm":       {run: runRm},
	"pause":    {run: runPause},
	"unpause":  {run: runUnpause},
	"kill":     {run: runKill},
	"config":   {run: runConfig},
	"images":   {run: runImages},
	"top":      {run: runTop},
	"ls":       {run: runLs},
	"port":     {run: runPort},
}

func Run(args []string, stdout, stderr io.Writer) int {
	jsonMode, rest := parseGlobalFlags(args)
	renderer := output.New(stdout, jsonMode)
	ctx := &Context{Args: rest, Renderer: renderer, Stdout: stdout, Stderr: stderr}
	if len(rest) == 0 {
		printHelp(stdout)
		return output.ExitOK
	}

	result, code, err := runRootCommand(ctx)
	if err != nil {
		renderError(renderer, stderr, code, err)
		return code
	}
	if result != nil {
		renderer.Render(result, humanRenderer())
	}
	return code
}

func runRootCommand(ctx *Context) (any, int, error) {
	cmd := ctx.Args[0]
	switch cmd {
	case "help", "-h", "--help":
		printHelp(ctx.Stdout)
		return nil, output.ExitOK, nil
	case "version", "-v", "--version":
		fmt.Fprintf(ctx.Stdout, "%s\n", tui.MutedS("docktree "+version))
		return nil, output.ExitOK, nil
	}

	spec, ok := rootCommands[cmd]
	if !ok {
		fmt.Fprintf(ctx.Stderr, "unknown command %q\n\n", cmd)
		printHelp(ctx.Stderr)
		return nil, output.ExitUsage, nil
	}
	if spec.progress {
		return runWithProgress(ctx, spec.run)
	}
	return spec.run(ctx)
}

func renderError(renderer *output.Renderer, stderr io.Writer, code int, err error) {
	target := renderer
	target.Writer = stderr
	target.Error(errorCode(code), err.Error(), nil)
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

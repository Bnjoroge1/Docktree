package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/bnjoroge/docktree/internal/config"
	"github.com/bnjoroge/docktree/internal/output"
	"github.com/bnjoroge/docktree/internal/proxy"
	"github.com/bnjoroge/docktree/internal/tui"
)

type proxyOptions struct {
	help bool
	port int
	host string
}

func parseProxyOptions(args []string) (proxyOptions, error) {
	options := proxyOptions{}
	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "-h", arg == "--help":
			options.help = true
		case arg == "--port" || arg == "-p":
			i++
			if i >= len(args) {
				return proxyOptions{}, fmt.Errorf("--port requires a value")
			}
			port, err := strconv.Atoi(args[i])
			if err != nil {
				return proxyOptions{}, fmt.Errorf("invalid port %q: %w", args[i], err)
			}
			options.port = port
		case strings.HasPrefix(arg, "--port="):
			port, err := strconv.Atoi(strings.TrimPrefix(arg, "--port="))
			if err != nil {
				return proxyOptions{}, fmt.Errorf("invalid port: %w", err)
			}
			options.port = port
		case arg == "--host":
			i++
			if i >= len(args) {
				return proxyOptions{}, fmt.Errorf("--host requires a value")
			}
			options.host = args[i]
		case strings.HasPrefix(arg, "--host="):
			options.host = strings.TrimPrefix(arg, "--host=")
		default:
			return proxyOptions{}, fmt.Errorf("unknown proxy flag %q", arg)
		}
		i++
	}
	return options, nil
}

type ProxyResult struct {
	Addr    string            `json:"addr"`
	Routes  map[string]string `json:"routes"`
	Running bool              `json:"running"`
}

func runProxy(ctx *Context) (any, int, error) {
	options, err := parseProxyOptions(ctx.Args[1:])
	if err != nil {
		return nil, output.ExitUsage, err
	}
	if options.help {
		return proxyHelpDoc(), output.ExitOK, nil
	}

	cfg := config.Defaults()
	if repo := loadRepoConfig(); repo != nil {
		cfg = *repo
	}

	port := cfg.Proxy.Port
	host := cfg.Proxy.Host
	if options.port != 0 {
		port = options.port
	}
	if options.host != "" {
		host = options.host
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	router := proxy.NewRouter()

	// Initial refresh to get routes for display
	if err := router.Refresh(); err != nil {
		return nil, output.ExitConfig, fmt.Errorf("load routes: %w", err)
	}
	routes := router.Routes()

	// Print startup banner (stderr in JSON mode)
	steps := ctx.Steps
	if steps != nil {
		steps.Header("Docktree proxy", addr)
		if len(routes) == 0 {
			steps.Sub(tui.MutedS("no running instances — start with docktree up"))
		} else {
			for name, backend := range routes {
				steps.Sub(fmt.Sprintf("%s → %s",
					tui.URLS(fmt.Sprintf("http://%s.localhost", name)),
					tui.MutedS(backend)))
			}
		}
		steps.Blank()
		steps.Active(tui.InfoS("listening… (Ctrl+C to stop)"))
	} else if !ctx.Renderer.JSON {
		fmt.Fprintf(ctx.Stdout, "%s proxy listening on %s\n", tui.BrandS("Docktree"), tui.URLS("http://"+addr))
		if len(routes) == 0 {
			fmt.Fprintf(ctx.Stdout, "  %s\n", tui.MutedS("(no running instances — start with docktree up)"))
		} else {
			for name, backend := range routes {
				fmt.Fprintf(ctx.Stdout, "  %s → %s\n",
					tui.URLS(fmt.Sprintf("http://%s.localhost", name)),
					tui.MutedS(backend))
				}
			}
		}

	if ctx.Renderer.JSON {
		// Emit startup JSON once the listener is bound. The ready callback
		// fires from ListenAndServe after net.Listen succeeds. All subsequent
		// server output goes to stderr. Return nil to avoid a second render.
		proxyCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		proxyCtx = proxy.WithStdout(proxyCtx, ctx.Stderr)
		if err := proxy.ListenAndServe(proxyCtx, addr, router, func() {
			_ = json.NewEncoder(ctx.Stdout).Encode(ProxyResult{Addr: addr, Routes: routes, Running: true})
		}); err != nil {
			return nil, output.ExitDocker, err
		}
		return nil, output.ExitOK, nil
	}

	proxyCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// We already printed a rich banner above. Let ListenAndServe write startup messages to a no-op writer
	// so they don't duplicate on stdout.
	if err := proxy.ListenAndServe(proxyCtx, addr, router, nil); err != nil {
		return nil, output.ExitDocker, err
	}

	if steps != nil {
		steps.Done("proxy stopped")
	}

	return ProxyResult{Addr: addr, Routes: routes, Running: true}, output.ExitOK, nil
}

// loadRepoConfig tries to load docktree.yml from the current directory.
func loadRepoConfig() *config.Config {
	cfg, err := config.Load(".")
	if err != nil {
		return nil
	}
	return cfg
}

func printProxyHelp(w interface{ Write([]byte) (int, error) }) {
	fmt.Fprint(w, `Usage: docktree proxy [--port PORT] [--host HOST]

Start a reverse proxy that routes by hostname to worktree ports.

Access worktrees as http://<instance>.localhost instead of
remembering port numbers.

Flags:
  --port, -p PORT   Proxy listen port (default: 8320)
  --host HOST       Proxy listen host (default: 127.0.0.1)
  --help, -h        Show this help
`)
}

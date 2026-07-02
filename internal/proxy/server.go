package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// ListenAndServe starts the reverse proxy on addr (e.g. "127.0.0.1:8320").
// It refreshes routes on startup and every 10 seconds while running.
// Blocks until ctx is cancelled, then gracefully shuts down.
// ready, if non-nil, is called after the listener is bound successfully.
func ListenAndServe(ctx context.Context, addr string, router *Router, ready func()) error {
	if err := router.Refresh(); err != nil {
		return fmt.Errorf("initial route refresh: %w", err)
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // streaming proxied responses
	}

	// background refresh
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = router.Refresh()
			}
		}
	}()

	// graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	if ready != nil {
		ready()
	}

	routes := router.Routes()
	fmt.Fprintf(addrWriter(ctx), "Docktree proxy listening on http://%s\n", addr)
	if len(routes) == 0 {
		fmt.Fprintf(addrWriter(ctx), "  (no running instances — start with docktree up)\n")
	} else {
		for name, backend := range routes {
			fmt.Fprintf(addrWriter(ctx), "  %s.localhost → %s\n", name, backend)
		}
	}

	if err := srv.Serve(ln); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// addrWriter is a context-carried writer for printing startup messages.
// Falls back to stdout if not set.
type ctxKeyWriter struct{}

func WithStdout(ctx context.Context, w interface{ Write([]byte) (int, error) }) context.Context {
	return context.WithValue(ctx, ctxKeyWriter{}, w)
}

func addrWriter(ctx context.Context) interface{ Write([]byte) (int, error) } {
	if w, ok := ctx.Value(ctxKeyWriter{}).(interface{ Write([]byte) (int, error) }); ok {
		return w
	}
	return writerNop{}
}

type writerNop struct{}

func (writerNop) Write(p []byte) (int, error) { return len(p), nil }

package srvhttp

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// defaultReadHeaderTimeout bounds the time spent reading request headers, so
// a slow client cannot hold connections open indefinitely.
const defaultReadHeaderTimeout = 30 * time.Second

// Run starts the server on addr and blocks until it stops.
// A normal Shutdown is not an error. Use RunGracefully for signal-driven
// graceful shutdown; build your own http.Server for TLS or other timeout tuning.
func (e *Engine) Run(addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           e,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
	}

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// RunGracefully starts the server on addr, blocks until SIGINT/SIGTERM, then
// drains in-flight requests within timeout. ctx is the parent of the shutdown
// context; a non-positive timeout means no timeout.
func (e *Engine) RunGracefully(ctx context.Context, addr string, timeout time.Duration) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           e,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(quit)

	select {
	case err := <-errCh:
		return err // server failed to start
	case <-ctx.Done():
	case <-quit:
	}

	shutdownCtx, cancel := loadContext(context.WithoutCancel(ctx), timeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		// drain timeout: force-close remaining connections
		_ = srv.Close()
		return err
	}
	return <-errCh
}

func loadContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout > 0 {
		return context.WithTimeout(parent, timeout)
	}
	return context.WithCancel(parent)
}

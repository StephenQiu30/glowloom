package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"go.uber.org/zap"
)

const roleHealthShutdownTimeout = 5 * time.Second

// RunWithHealth keeps a role's liveness listener owned by the role process.
// It stops the other side when either the role or the listener exits.
func RunWithHealth(ctx context.Context, role, address string, logger *zap.Logger, run func(context.Context) error) error {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("listen %s health: %w", role, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		name string
		err  error
	}
	results := make(chan result, 2)
	go func() { results <- result{"role", run(runCtx)} }()
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		results <- result{"health", err}
	}()
	logger.Info("role health listening", zap.String("role", role), zap.String("addr", address))
	first := <-results
	parentCancelled := ctx.Err() != nil
	cancel()
	shutdownCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), roleHealthShutdownTimeout)
	shutdownErr := server.Shutdown(shutdownCtx)
	stop()
	second := <-results
	if first.err != nil && !errors.Is(first.err, context.Canceled) {
		return fmt.Errorf("%s %s: %w", role, first.name, first.err)
	}
	if second.err != nil && !errors.Is(second.err, context.Canceled) {
		return fmt.Errorf("%s %s: %w", role, second.name, second.err)
	}
	if shutdownErr != nil {
		return fmt.Errorf("shutdown %s health: %w", role, shutdownErr)
	}
	if parentCancelled {
		return nil
	}
	if first.err != nil {
		return fmt.Errorf("%s %s: %w", role, first.name, first.err)
	}
	return fmt.Errorf("%s %s stopped unexpectedly", role, first.name)
}

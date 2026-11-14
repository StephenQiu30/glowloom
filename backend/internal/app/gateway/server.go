package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"

	proxygateway "github.com/StephenQiu30/glowloom-server/backend/internal/gateway"
)

// Run owns the gateway listener until cancellation or a serving failure.
func Run(ctx context.Context, cfg Config, logger *zap.Logger) error {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen gateway: %w", err)
	}
	logger.Info("gateway listening", zap.String("addr", listener.Addr().String()))
	return Serve(ctx, listener, cfg.BackendOrigin, logger)
}

// Serve owns listener and serves the public proxy without starting business
// runtimes. Cancelling ctx also closes active SSE and upgraded connections via
// their request contexts; HTTP shutdown alone does not close hijacked sockets.
func Serve(ctx context.Context, listener net.Listener, origin string, logger *zap.Logger) error {
	defer func() { _ = listener.Close() }()
	handler, cleanup, err := proxygateway.NewHandler(origin, logger)
	if err != nil {
		return fmt.Errorf("configure gateway: %w", err)
	}
	defer cleanup()
	lifetime, cancel := context.WithCancel(ctx)
	defer cancel()
	var active sync.WaitGroup
	var admission sync.Mutex
	stopping := false
	tracked := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		admission.Lock()
		if stopping {
			admission.Unlock()
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		active.Add(1)
		admission.Unlock()
		defer active.Done()
		handler.ServeHTTP(w, r)
	})
	stopRequests := func() {
		admission.Lock()
		stopping = true
		admission.Unlock()
		cancel()
	}
	server := &http.Server{
		Handler:           tracked,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		BaseContext:       func(net.Listener) context.Context { return lifetime },
		// No overall ReadTimeout/WriteTimeout: uploads and long-lived streams
		// retain the business owner's limits and request cancellation contract.
	}
	done := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		done <- err
	}()
	select {
	case err := <-done:
		stopRequests()
		_ = server.Close()
		active.Wait()
		return err
	case <-ctx.Done():
	}
	stopRequests()
	shutdown, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer stop()
	shutdownErr := server.Shutdown(shutdown)
	if shutdownErr != nil {
		_ = server.Close()
	}
	serveErr := <-done
	// Also join hijacked handlers: net/http.Shutdown does not wait for them.
	active.Wait()
	if shutdownErr != nil {
		return fmt.Errorf("shutdown gateway: %w", shutdownErr)
	}
	return serveErr
}

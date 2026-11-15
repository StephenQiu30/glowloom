package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"

	proxygateway "github.com/StephenQiu30/glowloom-server/backend/gateway/proxy"
)

// Run owns the gateway listener until cancellation or a serving failure.
func Run(ctx context.Context, cfg Config, logger *zap.Logger) error {
	if err := cfg.validate(cfg.HTTPAddr); err != nil {
		return fmt.Errorf("configure gateway: %w", err)
	}
	if logger == nil {
		return errors.New("gateway requires a logger")
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen gateway: %w", err)
	}
	mode := "four-owner"
	if !cfg.Origins.Configured() {
		mode = "pre-migration"
	}
	logger.Info("gateway listening", zap.String("addr", listener.Addr().String()), zap.String("mode", mode))
	return ServeConfig(ctx, listener, cfg, logger)
}

// Serve serves the explicitly selected pre-migration single-backend mode.
func Serve(ctx context.Context, listener net.Listener, origin string, logger *zap.Logger) error {
	return ServeConfig(ctx, listener, Config{BackendOrigin: origin}, logger)
}

// ServeConfig owns listener without starting business runtimes. Cancellation
// reaches active SSE and upgraded connections; shutdown and handler joins share
// a bounded deadline because HTTP shutdown alone does not join hijacked sockets.
func ServeConfig(ctx context.Context, listener net.Listener, cfg Config, logger *zap.Logger) error {
	defer func() { _ = listener.Close() }()
	if err := cfg.validate(listener.Addr().String()); err != nil {
		return fmt.Errorf("configure gateway: %w", err)
	}
	var handler http.Handler
	var cleanup func()
	var err error
	if cfg.Origins.Configured() {
		handler, cleanup, err = proxygateway.NewDomainHandler(cfg.Origins, logger)
	} else {
		handler, cleanup, err = proxygateway.NewHandler(cfg.BackendOrigin, logger)
	}
	if err != nil {
		return fmt.Errorf("configure gateway: %w", err)
	}
	defer cleanup()
	lifetime, cancel := context.WithCancel(ctx)
	defer cancel()
	var admission sync.Mutex
	active := 0
	stopping := false
	joined := make(chan struct{})
	tracked := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		admission.Lock()
		if stopping {
			admission.Unlock()
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		active++
		admission.Unlock()
		defer func() {
			admission.Lock()
			defer admission.Unlock()
			active--
			if stopping && active == 0 {
				close(joined)
			}
		}()
		handler.ServeHTTP(w, r)
	})
	stopRequests := func() {
		admission.Lock()
		stopping = true
		if active == 0 {
			close(joined)
		}
		admission.Unlock()
		cancel()
	}
	server := &http.Server{
		Handler:           tracked,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		BaseContext:       func(net.Listener) context.Context { return lifetime },
		// No overall ReadTimeout/WriteTimeout: retain owner limits for uploads
		// and long-lived streams together with request cancellation.
	}
	done := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		done <- err
	}()
	var serveErr error
	served := false
	select {
	case serveErr = <-done:
		served = true
	case <-ctx.Done():
	}
	stopRequests()
	shutdown, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer stop()
	shutdownErr := server.Shutdown(shutdown)
	if shutdownErr != nil {
		_ = server.Close()
	}
	if !served {
		select {
		case serveErr = <-done:
		case <-shutdown.Done():
			return fmt.Errorf("join gateway listener: %w", shutdown.Err())
		}
	}
	select {
	case <-joined:
	case <-shutdown.Done():
		return fmt.Errorf("join gateway handlers: %w", shutdown.Err())
	}
	if shutdownErr != nil {
		return fmt.Errorf("shutdown gateway: %w", shutdownErr)
	}
	return serveErr
}

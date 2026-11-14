package creationruntime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/adapter/grpcserver"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/adapter/native"
)

type tracedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *tracedStream) Context() context.Context { return s.ctx }

// Run validates the private process dependencies before binding either listener.
// Both serving goroutines and the graceful-stop goroutine are joined on exit.
func Run(ctx context.Context, cfg Config, logger *slog.Logger) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if logger == nil {
		return errors.New("creation logger required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	probe, err := os.CreateTemp("", "glowloom-creation-ready-*")
	if err != nil {
		return fmt.Errorf("creation temporary storage unavailable: %w", err)
	}
	closeErr := probe.Close()
	removeErr := os.Remove(probe.Name())
	if err := errors.Join(closeErr, removeErr); err != nil {
		return fmt.Errorf("creation temporary storage unavailable: %w", err)
	}
	tlsConfig, err := cfg.serverTLS()
	if err != nil {
		return err
	}
	handler, err := grpcserver.New(native.NewExtractor(), cfg.Token, cfg.Concurrent)
	if err != nil {
		return err
	}
	options := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(grpcserver.MaxMessageBytes),
		grpc.MaxSendMsgSize(grpcserver.MaxMessageBytes),
		grpc.MaxHeaderListSize(8192),
		grpc.MaxConcurrentStreams(uint32(cfg.Concurrent)),
		grpc.WaitForHandlers(true),
		grpc.StreamInterceptor(func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, next grpc.StreamHandler) error {
			start := time.Now()
			incoming, _ := metadata.FromIncomingContext(stream.Context())
			carrier := propagation.MapCarrier{}
			for _, key := range []string{"traceparent", "tracestate"} {
				if values := incoming.Get(key); len(values) == 1 {
					carrier[key] = values[0]
				}
			}
			callCtx := propagation.TraceContext{}.Extract(stream.Context(), carrier)
			err := next(srv, &tracedStream{ServerStream: stream, ctx: callCtx})
			fields := []any{
				"service", "creation", "method", info.FullMethod,
				"status", status.Code(err).String(), "duration_ms", time.Since(start).Milliseconds(),
			}
			if correlation := trace.SpanContextFromContext(callCtx); correlation.IsValid() {
				fields = append(fields, "trace_id", correlation.TraceID().String())
			}
			// Deliberately omit request bodies, metadata and error strings: they can
			// contain original document text, private tokens or untrusted trace data.
			logger.InfoContext(callCtx, "creation RPC completed", fields...)
			return err
		}),
	}
	if tlsConfig != nil {
		options = append(options, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}
	server := grpc.NewServer(options...)
	pb.RegisterExtractionServer(server, handler)
	var listenConfig net.ListenConfig
	grpcListener, err := listenConfig.Listen(ctx, "tcp", cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("listen creation RPC: %w", err)
	}
	defer func() { _ = grpcListener.Close() }()
	httpListener, err := listenConfig.Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen creation health: %w", err)
	}
	defer func() { _ = httpListener.Close() }()

	var ready atomic.Bool
	ready.Store(true)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, _ *http.Request) {
		if !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	health := &http.Server{
		Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192,
	}
	errorsCh := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() { defer workers.Done(); errorsCh <- server.Serve(grpcListener) }()
	go func() { defer workers.Done(); errorsCh <- health.Serve(httpListener) }()
	logger.InfoContext(ctx, "creation extractor listening", "service", "creation", "grpc_addr", grpcListener.Addr().String(), "health_addr", httpListener.Addr().String(), "concurrency", cfg.Concurrent)
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-errorsCh:
	}
	ready.Store(false)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stopped := make(chan struct{})
	go func() { server.GracefulStop(); close(stopped) }()
	if err := health.Shutdown(shutdownCtx); err != nil {
		_ = health.Close()
	}
	select {
	case <-stopped:
	case <-shutdownCtx.Done():
		server.Stop()
		<-stopped
	}
	workers.Wait()
	if serveErr != nil && !errors.Is(serveErr, grpc.ErrServerStopped) && !errors.Is(serveErr, http.ErrServerClosed) && !errors.Is(serveErr, net.ErrClosed) {
		return fmt.Errorf("serve creation: %w", serveErr)
	}
	return nil
}

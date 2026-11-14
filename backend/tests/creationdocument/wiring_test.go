package creationdocument_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"google.golang.org/grpc"

	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/adapter/grpcserver"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/adapter/native"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	script "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
)

func TestCreationProductionFactoryUsesRemoteParserWithoutFallback(t *testing.T) {
	var lc net.ListenConfig
	listener, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := grpcserver.New(native.NewExtractor(), token, 1)
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(grpc.MaxRecvMsgSize(grpcserver.MaxMessageBytes))
	pb.RegisterExtractionServer(server, handler)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
		if err := <-done; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Error(err)
		}
	})
	for _, tc := range []struct {
		name, address, identity string
		wantError               bool
	}{
		{"valid", listener.Addr().String(), token, false},
		{"wrong_identity", listener.Addr().String(), "another-service-identity-at-least-32-bytes", true},
		{"disabled", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			extractor, err := scriptwire.ProvideSourceExtractor(config.Config{Env: "local", CreationExtractionAddress: tc.address, CreationServiceToken: tc.identity})
			if err != nil {
				t.Fatal(err)
			}
			original := nativeOriginal(t, []byte("完整脚本文档"), "text/plain")
			result, err := extractor.Extract(t.Context(), &media.Downloaded{File: original.File, Size: original.Size, SHA256: original.SHA256, MIMEType: original.MIMEType})
			if tc.wantError {
				if !errors.Is(err, script.ErrUnavailable) {
					t.Fatalf("expected unavailable, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			plain, err := result.Document.PlainText()
			if err != nil || plain != "完整脚本文档" {
				t.Fatalf("plain=%q err=%v", plain, err)
			}
		})
	}
}

func TestCreationProductionFactoryRejectsPartialOrUnsafeTransport(t *testing.T) {
	for _, cfg := range []config.Config{
		{Env: "local", CreationExtractionAddress: "127.0.0.1:9092"},
		{Env: "local", CreationServiceToken: token},
		{Env: "local", CreationExtractionTLSCAFile: "/unused"},
		{Env: "production", CreationExtractionAddress: "creation:9092", CreationServiceToken: token},
		{Env: "local", CreationExtractionAddress: "creation:9092", CreationServiceToken: token, CreationExtractionTLSCAFile: "/unused"},
	} {
		if _, err := scriptwire.ProvideSourceExtractor(cfg); err == nil {
			t.Errorf("accepted partial Creation configuration: target=%q", cfg.CreationExtractionAddress)
		}
	}
	client, err := scriptwire.ProvideSourceExtractor(config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.Extract(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation mapping: %v", err)
	}
}

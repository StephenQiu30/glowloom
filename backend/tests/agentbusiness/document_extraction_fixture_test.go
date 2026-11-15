package agentbusiness_test

import (
	"errors"
	"net"
	"testing"

	"google.golang.org/grpc"

	"github.com/StephenQiu30/glowloom-server/backend/creation/document/adapter/grpcserver"
	"github.com/StephenQiu30/glowloom-server/backend/creation/document/adapter/native"
	script "github.com/StephenQiu30/glowloom-server/backend/creation/script/application"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func referenceDocumentExtractor(t *testing.T) script.SourceExtractor {
	t.Helper()
	var lc net.ListenConfig
	listener, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	const token = "agent-document-fixture-service-identity-long-enough"
	handler, err := grpcserver.New(native.NewExtractor(), token, 1)
	if err != nil {
		_ = listener.Close()
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
	extractor, err := scriptwire.ProvideSourceExtractor(config.Config{
		Env: "local", CreationExtractionAddress: listener.Addr().String(), CreationServiceToken: token,
	})
	if err != nil {
		t.Fatal(err)
	}
	return extractor
}

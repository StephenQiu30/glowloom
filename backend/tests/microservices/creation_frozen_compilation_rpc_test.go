package microservices_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/grpchandoff"
	prompt "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationhandoff"
)

// This substitute exercises error transport only. Successful source authorization
// and result recovery are tested through the real Identity/PG pipeline separately.
type rejectingFrozenCompiler struct{ err error }

func (r rejectingFrozenCompiler) ReadFrozenInput(context.Context, prompt.FrozenInputKey) (prompt.FrozenInputEnvelope, error) {
	return prompt.FrozenInputEnvelope{}, r.err
}
func (r rejectingFrozenCompiler) Freeze(context.Context, prompt.FreezeInputRequest) (prompt.FrozenInputEnvelope, error) {
	return prompt.FrozenInputEnvelope{}, r.err
}
func (r rejectingFrozenCompiler) Compile(context.Context, prompt.CompileFrozenInputRequest) (prompt.FrozenInputCompilation, error) {
	return prompt.FrozenInputCompilation{}, r.err
}

func TestCreationFrozenCompilationRPCPreservesOriginalQuoteFailureReasons(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		code   codes.Code
		reason pb.FrozenInputErrorReason
	}{
		{"template-invalid", prompt.ErrTemplateInvalid, codes.InvalidArgument, pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_TEMPLATE_INVALID},
		{"template-changed", prompt.ErrTemplateChanged, codes.Aborted, pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_TEMPLATE_CHANGED},
		{"protected-context", prompt.ErrTemplateContextUnavailable, codes.FailedPrecondition, pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_TEMPLATE_CONTEXT_UNAVAILABLE},
		{"unknown-compiler", domain.ErrUnsupportedSchema, codes.FailedPrecondition, pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_UNSUPPORTED_COMPILER},
		{"stale-source", domain.ErrInputStale, codes.Aborted, pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_SOURCE_STALE},
		{"reference-unavailable", domain.ErrReferenceUnavailable, codes.FailedPrecondition, pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_REFERENCE_UNAVAILABLE},
	} {
		t.Run(test.name, func(t *testing.T) {
			local := rejectingFrozenCompiler{test.err}
			token := strings.Repeat("compile-source-private-", 3)
			server, err := grpchandoff.NewCompilingServer(local, local, local, token, insecure.NewCredentials())
			if err != nil {
				t.Fatal(err)
			}
			conn, err := grpc.NewClient(serveSourcePipeline(t, server), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close() }()
			_, request := compilationEnvelope(t)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
			key, catalog := request.Key, request.CatalogContext
			_, err = pb.NewFrozenInputsClient(conn).CompileFrozenInput(ctx, &pb.CompileFrozenInputRequest{Key: &pb.FrozenInputKey{HandoffId: key.HandoffID[:], CommandId: key.CommandID[:], CommandSha256: key.CommandSHA256, SnapshotSha256: key.SnapshotSHA256}, CatalogContext: &pb.FrozenCatalogContext{ModelVersionId: catalog.ModelVersionID[:], ModelKey: catalog.ModelKey, Capability: catalog.Capability, Mode: catalog.Mode}})
			if status.Code(err) != test.code {
				t.Fatal("original quote failure became unavailable", err)
			}
			details := status.Convert(err).Details()
			if len(details) != 1 {
				t.Fatal("missing closed error reason", details)
			}
			failure, ok := details[0].(*pb.FrozenInputFailure)
			if !ok || failure.Reason != test.reason {
				t.Fatal("wrong typed quote failure reason", details)
			}
		})
	}
}

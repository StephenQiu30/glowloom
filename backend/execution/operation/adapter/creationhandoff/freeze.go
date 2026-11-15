package creationhandoff

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	wirehash "github.com/StephenQiu30/glowloom-server/backend/internal/contract/frozeninput"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationhandoff"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// FreezeFrozenInput consumes the unique private protobuf selection, not a copied
// Creation draft or caller Principal. Identity admission for the original target
// Execution command must already exist; Creation revalidates that exact proof.
func (c *SourceClient) FreezeFrozenInput(ctx context.Context, selection *pb.FreezeFrozenInputRequest) (application.FrozenInput, error) {
	var out application.FrozenInput
	if c == nil || !closed(selection) || selection.Source == nil || len(selection.Pins) > 256 || selection.SourceGeneration < 1 || selection.ExpectedScopeRevision < 1 || selection.ExpectedProjectRevision < 1 {
		return out, application.ErrFrozenInputInvalid
	}
	req := proto.Clone(selection).(*pb.FreezeFrozenInputRequest)
	handoff, err := uuid.FromBytes(req.HandoffId)
	if err != nil || handoff == uuid.Nil {
		return out, application.ErrFrozenInputInvalid
	}
	request, err := uuid.FromBytes(req.RequestId)
	if err != nil || request == uuid.Nil {
		return out, application.ErrFrozenInputInvalid
	}
	scope, err := uuid.FromBytes(req.ScopeId)
	if err != nil || scope == uuid.Nil {
		return out, application.ErrFrozenInputInvalid
	}
	project, err := uuid.FromBytes(req.ProjectId)
	if err != nil || project == uuid.Nil {
		return out, application.ErrFrozenInputInvalid
	}
	command, err := decodeCommand(req.Command)
	if err != nil || command.Action != "execution.frozen_input.accept" {
		return out, application.ErrFrozenInputInvalid
	}
	source, err := decodeCommand(req.SourceCommand)
	if err != nil || source.Action != "creation.frozen_input.freeze" || source.ID != uuid.NewSHA1(handoff, []byte("creation.frozen_input.freeze")) || source.ID == command.ID {
		return out, application.ErrFrozenInputInvalid
	}
	digest, _ := command.AuthorityDigest()
	sourceDigest, _ := source.AuthorityDigest()
	if digest != sourceDigest {
		return out, application.ErrFrozenInputConflict
	}
	expectedSourceHash, err := wirehash.SourceCommandSHA256(req)
	if err != nil || source.SHA256 != expectedSourceHash {
		return out, application.ErrFrozenInputConflict
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	md := metadata.Pairs("authorization", "Bearer "+c.target.Token)
	for _, key := range []string{"traceparent", "tracestate"} {
		if value := carrier[key]; value != "" {
			md.Set(key, value)
		}
	}
	ctx = metadata.NewOutgoingContext(ctx, md)
	conn, err := grpc.NewClient(c.target.Address, grpc.WithTransportCredentials(c.target.Transport.Clone()), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMessageBytes), grpc.MaxCallSendMsgSize(maxMessageBytes), grpc.MaxRetryRPCBufferSize(0)))
	if err != nil {
		return out, writefence.ErrUnavailable
	}
	defer func() { _ = conn.Close() }()
	response, err := pb.NewFrozenInputsClient(conn).FreezeFrozenInput(ctx, req)
	if err != nil {
		return out, localError(err)
	}
	out, err = decodeInput(response)
	if err != nil {
		return out, err
	}
	if out.Key.HandoffID != handoff || out.RequestID != request || out.ScopeID != scope || out.ProjectID != project || !application.SameFrozenInputCommand(out.Command, command) || out.SourceGeneration != req.SourceGeneration || !proto.Equal(response.SourceCommand, req.SourceCommand) || !proto.Equal(response.Source, req.Source) || response.ScopeRevision != req.ExpectedScopeRevision || response.ProjectRevision != req.ExpectedProjectRevision || len(response.Pins) != len(req.Pins) {
		return application.FrozenInput{}, application.ErrFrozenInputConflict
	}
	for i, pin := range req.Pins {
		if !proto.Equal(pin, response.Pins[i]) {
			return application.FrozenInput{}, application.ErrFrozenInputConflict
		}
	}
	return out, nil
}

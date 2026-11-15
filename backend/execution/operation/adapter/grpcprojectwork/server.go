// Package grpcprojectwork exposes only Execution's read-only project responsibility.
package grpcprojectwork

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/executionwork"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
)

type server struct {
	pb.UnimplementedProjectWorkServer
	reader  application.ProjectWorkReader
	token   string
	peerDNS string
	slots   chan struct{}
}

// NewServer binds the default Creation peer name in addition to its dedicated token.
func NewServer(reader application.ProjectWorkReader, creationToken string, transport credentials.TransportCredentials) (*grpc.Server, error) {
	return NewPeerServer(reader, creationToken, "creation", transport)
}

// NewPeerServer requires the configured Creation DNS SAN and verified mTLS chain.
func NewPeerServer(reader application.ProjectWorkReader, creationToken, creationDNS string, transport credentials.TransportCredentials) (*grpc.Server, error) {
	if reader == nil || !serviceauth.ValidToken(creationToken) || creationDNS == "" || strings.TrimSpace(creationDNS) != creationDNS || strings.ContainsAny(creationDNS, "*/\\\\ \t\r\n") || transport == nil || transport.Info().SecurityProtocol != "tls" {
		return nil, writefence.ErrInvalid
	}
	handler := &server{reader: reader, token: creationToken, peerDNS: creationDNS, slots: make(chan struct{}, 4)}
	listener := grpc.NewServer(grpc.Creds(transport.Clone()), grpc.MaxRecvMsgSize(512<<10), grpc.MaxSendMsgSize(1<<20), grpc.MaxHeaderListSize(16384), grpc.UnaryInterceptor(handler.authorize))
	pb.RegisterProjectWorkServer(listener, handler)
	return listener, nil
}

func (s *server) authorize(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	auth := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(auth) != 1 || subtle.ConstantTimeCompare([]byte(auth[0]), []byte("Bearer "+s.token)) != 1 {
		return nil, status.Error(codes.Unauthenticated, "Creation project responsibility identity required")
	}
	remote, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "verified project responsibility peer required")
	}
	tlsInfo, ok := remote.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.PeerCertificates) == 0 || tlsInfo.State.PeerCertificates[0].VerifyHostname(s.peerDNS) != nil {
		return nil, status.Error(codes.Unauthenticated, "mutual TLS project responsibility peer required")
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 10*time.Second {
		return nil, status.Error(codes.InvalidArgument, "bounded project responsibility deadline required")
	}
	if info.FullMethod != pb.ProjectWork_ReadProjectWork_FullMethodName {
		return nil, status.Error(codes.Unimplemented, "unknown project responsibility operation")
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return nil, status.Error(codes.ResourceExhausted, "project responsibility capacity reached")
	}
	carrier := propagation.MapCarrier{}
	safe := metadata.MD{}
	for _, key := range []string{"traceparent", "tracestate"} {
		if values := metadata.ValueFromIncomingContext(ctx, key); len(values) == 1 {
			carrier[key] = values[0]
			safe.Set(key, values[0])
		}
	}
	ctx = propagation.TraceContext{}.Extract(ctx, carrier)
	return next(metadata.NewIncomingContext(ctx, safe), request)
}

func closedMessage(message protoreflect.Message) bool {
	if len(message.GetUnknown()) != 0 {
		return false
	}
	valid := true
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.Kind() != protoreflect.MessageKind {
			return true
		}
		if field.IsList() {
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				if !closedMessage(list.Get(i).Message()) {
					valid = false
					return false
				}
			}
			return true
		}
		valid = closedMessage(value.Message())
		return valid
	})
	return valid
}

func decodeRequest(value *pb.ProjectWorkRequest) (application.ProjectWorkRequest, error) {
	var out application.ProjectWorkRequest
	if value == nil || !closedMessage(value.ProtoReflect()) || len(value.Commands) > 128 {
		return out, writefence.ErrInvalid
	}
	ids := []*uuid.UUID{&out.CloseID, &out.ProjectID, &out.OrganizationID, &out.ScopeID}
	raw := [][]byte{value.CloseId, value.ProjectId, value.OrganizationId, value.ScopeId}
	for i, dst := range ids {
		id, err := uuid.FromBytes(raw[i])
		if err != nil {
			return out, writefence.ErrInvalid
		}
		*dst = id
	}
	out.ProjectEpoch, out.ManifestSHA256 = value.ProjectEpoch, value.ManifestSha256
	out.Commands = make([]writefence.Command, len(value.Commands))
	for i, command := range value.Commands {
		var err error
		out.Commands[i], err = grpctransport.DecodeCommand(command)
		if err != nil {
			return out, err
		}
	}
	return out, out.Validate()
}
func encodeRequest(value application.ProjectWorkRequest) *pb.ProjectWorkRequest {
	out := &pb.ProjectWorkRequest{CloseId: value.CloseID[:], ProjectId: value.ProjectID[:], OrganizationId: value.OrganizationID[:], ScopeId: value.ScopeID[:], ProjectEpoch: value.ProjectEpoch, ManifestSha256: value.ManifestSHA256}
	for _, command := range value.Commands {
		out.Commands = append(out.Commands, grpctransport.EncodeCommand(command))
	}
	return out
}

func (s *server) ReadProjectWork(ctx context.Context, value *pb.ProjectWorkRequest) (*pb.ProjectWorkSnapshot, error) {
	request, err := decodeRequest(value)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid project responsibility request")
	}
	snapshot, err := s.reader.ReadProjectWork(ctx, request)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return nil, status.FromContextError(err).Err()
		case errors.Is(err, writefence.ErrInvalid), errors.Is(err, writefence.ErrConflict):
			return nil, status.Error(codes.FailedPrecondition, "project responsibility identity conflict")
		default:
			return nil, status.Error(codes.Unavailable, "project responsibility unresolved")
		}
	}
	if !proto.Equal(encodeRequest(snapshot.Request), value) || len(snapshot.Commands) != len(request.Commands) {
		return nil, status.Error(codes.Unavailable, "project responsibility echo incomplete")
	}
	out := &pb.ProjectWorkSnapshot{Request: proto.Clone(value).(*pb.ProjectWorkRequest)}
	switch snapshot.BusinessState {
	case application.ProjectWorkIdle:
		out.BusinessState = pb.BusinessState_IDLE
	case application.ProjectWorkBusy:
		out.BusinessState = pb.BusinessState_BUSY
	case application.ProjectWorkUnknown:
		out.BusinessState = pb.BusinessState_UNKNOWN
	default:
		return nil, status.Error(codes.Unavailable, "project responsibility state unresolved")
	}
	commands := make(map[uuid.UUID]writefence.Command, len(request.Commands))
	for _, command := range request.Commands {
		commands[command.ID] = command
	}
	for _, observation := range snapshot.Commands {
		original, ok := commands[observation.Command.ID]
		if !ok || !application.SameFrozenInputCommand(original, observation.Command) {
			return nil, status.Error(codes.Unavailable, "project command observation incomplete")
		}
		delete(commands, original.ID)
		encoded := &pb.CommandObservation{Command: grpctransport.EncodeCommand(observation.Command), ReceiptSha256: observation.ReceiptSHA256}
		switch observation.State {
		case writefence.Committed:
			encoded.State = pb.CommandState_COMMITTED
			if len(observation.ReceiptSHA256) != 64 {
				return nil, status.Error(codes.Unavailable, "project command receipt unresolved")
			}
		case writefence.Frozen:
			encoded.State = pb.CommandState_FROZEN
		case writefence.Admitted:
			encoded.State = pb.CommandState_ADMITTED
		case writefence.State("unknown"):
			encoded.State = pb.CommandState_COMMAND_UNKNOWN
		default:
			return nil, status.Error(codes.Unavailable, "project command state unresolved")
		}
		out.Commands = append(out.Commands, encoded)
	}
	return out, nil
}

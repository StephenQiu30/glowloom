package grpctransport

import (
	"context"
	"crypto/subtle"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// WriterStore uses only its own restricted writer database. Install must return
// only after its local final-write barrier has committed.
type WriterStore interface {
	Owner(context.Context) (writefence.Writer, error)
	Install(context.Context, writefence.Control) error
	Load(context.Context, uuid.UUID) (writefence.Record, error)
}

// Server binds each control issuer credential to one concrete target writer.
// A runtime must also configure its private listener with verified TLS peers.
type Server struct {
	pb.UnimplementedWriterFenceServer
	writer        writefence.Writer
	store         WriterStore
	identityToken string
	creationToken string
}

// NewServer rejects missing identities rather than enabling anonymous control.
func NewServer(writer writefence.Writer, store WriterStore, identityToken, creationToken string) (*Server, error) {
	if writer.Validate() != nil || store == nil || !serviceauth.ValidToken(identityToken) || !serviceauth.ValidToken(creationToken) || identityToken == creationToken {
		return nil, writefence.ErrInvalid
	}
	return &Server{writer: writer, store: store, identityToken: identityToken, creationToken: creationToken}, nil
}

func (s *Server) issuer(ctx context.Context) (writefence.Issuer, error) {
	if s == nil || s.store == nil {
		return "", status.Error(codes.Unavailable, "writer control unavailable")
	}
	values := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(values) != 1 {
		return "", status.Error(codes.Unauthenticated, "service identity required")
	}
	identity := subtle.ConstantTimeCompare([]byte(values[0]), []byte("Bearer "+s.identityToken))
	creation := subtle.ConstantTimeCompare([]byte(values[0]), []byte("Bearer "+s.creationToken))
	if identity == 1 {
		return writefence.Identity, nil
	}
	if creation == 1 {
		return writefence.Creation, nil
	}
	return "", status.Error(codes.Unauthenticated, "service identity required")
}

func requireDeadline(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return status.FromContextError(err).Err()
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 30*time.Second {
		return status.Error(codes.InvalidArgument, "bounded deadline required")
	}
	return nil
}

// Install authenticates issuer and target before touching local durable state.
func (s *Server) Install(ctx context.Context, request *pb.InstallRequest) (*pb.InstallResponse, error) {
	issuer, err := s.issuer(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireDeadline(ctx); err != nil {
		return nil, err
	}
	if request == nil || request.GetWriter() != encodeWriter(s.writer) {
		return nil, status.Error(codes.PermissionDenied, "wrong writer audience")
	}
	control, err := decodeControl(request.GetControl())
	if err != nil {
		return nil, rpcError(err)
	}
	if control.Issuer != issuer {
		return nil, status.Error(codes.PermissionDenied, "wrong control issuer")
	}
	if owner, err := s.store.Owner(ctx); err != nil || owner != s.writer {
		return nil, status.Error(codes.Unavailable, "writer control unavailable")
	}
	if err := s.store.Install(ctx, control); err != nil {
		return nil, rpcError(err)
	}
	return &pb.InstallResponse{Writer: encodeWriter(s.writer), Control: encodeControl(control)}, nil
}

// Resolve returns only a fully matching original command to one of its policy
// issuers. A command receipt cannot be used to admit or run additional effects.
func (s *Server) Resolve(ctx context.Context, request *pb.ResolveRequest) (*pb.ResolveResponse, error) {
	issuer, err := s.issuer(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireDeadline(ctx); err != nil {
		return nil, err
	}
	if request == nil || request.GetWriter() != encodeWriter(s.writer) {
		return nil, status.Error(codes.PermissionDenied, "wrong writer audience")
	}
	command, err := decodeCommand(request.GetCommand())
	if err != nil {
		return nil, rpcError(err)
	}
	owned := false
	for _, authority := range command.Authorities {
		owned = owned || authority.Issuer == issuer
	}
	if !owned {
		return nil, status.Error(codes.PermissionDenied, "command belongs to another issuer")
	}
	if owner, err := s.store.Owner(ctx); err != nil || owner != s.writer {
		return nil, status.Error(codes.Unavailable, "writer control unavailable")
	}
	record, err := s.store.Load(ctx, command.ID)
	if err != nil {
		return nil, rpcError(err)
	}
	if !sameCommand(record.Command, command) {
		return nil, rpcError(writefence.ErrConflict)
	}
	var state pb.CommandState
	switch record.State {
	case writefence.Admitted:
		state = pb.CommandState_COMMAND_STATE_ADMITTED
	case writefence.Frozen:
		state = pb.CommandState_COMMAND_STATE_FROZEN
	case writefence.Committed:
		state = pb.CommandState_COMMAND_STATE_COMMITTED
		if record.Receipt, err = writefence.CanonicalReceipt(record.Receipt); err != nil {
			return nil, status.Error(codes.DataLoss, "invalid writer receipt")
		}
	default:
		return nil, status.Error(codes.DataLoss, "invalid writer outcome")
	}
	if state != pb.CommandState_COMMAND_STATE_COMMITTED && len(record.Receipt) != 0 {
		return nil, status.Error(codes.DataLoss, "invalid writer receipt")
	}
	return &pb.ResolveResponse{Command: encodeCommand(record.Command), State: state, ReceiptJson: append([]byte(nil), record.Receipt...)}, nil
}

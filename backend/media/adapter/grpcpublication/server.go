// Package grpcpublication exposes authenticated Media owner publication, not public API.
package grpcpublication

import (
	"context"
	"crypto/subtle"
	"errors"
	"io"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediapublication"
	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/outputpublication"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	"github.com/StephenQiu30/glowloom-server/backend/media/application"
)

// Private publication bounds include binary frames, envelope overhead and total deadline.
const (
	ChunkSize       = 256 << 10
	MaxMessageBytes = ChunkSize + 16384
	MaxDuration     = 3 * time.Minute
)

// Publisher is consumed by the authenticated Media listener.
type Publisher interface {
	Publish(context.Context, application.PublishOperationOutputRequest, io.Reader) (contract.OperationOutputReceipt, error)
	Resolve(context.Context, application.PublishOperationOutputRequest) (contract.OperationOutputReceipt, error)
}

// Server bounds concurrent archive work; its composition root supplies mandatory mTLS.
type Server struct {
	pb.UnimplementedOperationOutputsServer
	publisher Publisher
	token     string
	slots     chan struct{}
}

// New requires a dedicated Execution-to-Media token, independent from control RPCs.
func New(publisher Publisher, token string, concurrent int) (*Server, error) {
	if publisher == nil || !serviceauth.ValidToken(token) || concurrent < 1 || concurrent > 8 {
		return nil, errors.New("invalid Media output publication configuration")
	}
	return &Server{publisher: publisher, token: token, slots: make(chan struct{}, concurrent)}, nil
}

// NewListener constructs the bounded private publication listener with mandatory
// peer transport credentials; full owner roots must provide their verified mTLS.
func NewListener(publisher Publisher, token string, concurrent int, transport credentials.TransportCredentials) (*grpc.Server, error) {
	if transport == nil {
		return nil, errors.New("media publication transport required")
	}
	handler, err := New(publisher, token, concurrent)
	if err != nil {
		return nil, err
	}
	server := grpc.NewServer(grpc.Creds(transport.Clone()), grpc.MaxRecvMsgSize(MaxMessageBytes), grpc.MaxSendMsgSize(MaxMessageBytes), grpc.MaxHeaderListSize(16384))
	pb.RegisterOperationOutputsServer(server, handler)
	return server, nil
}

func (s *Server) authorize(ctx context.Context) error {
	md, _ := metadata.FromIncomingContext(ctx)
	auth := md.Get("authorization")
	if len(auth) != 1 || subtle.ConstantTimeCompare([]byte(auth[0]), []byte("Bearer "+s.token)) != 1 {
		return status.Error(codes.Unauthenticated, "Media output authentication required")
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > MaxDuration {
		return status.Error(codes.InvalidArgument, "bounded Media output deadline required")
	}
	return nil
}

// PublishOperationOutput requires an immutable typed header, bounded frames and EOF.
func (s *Server) PublishOperationOutput(stream grpc.ClientStreamingServer[pb.PublishOperationOutputFrame, pb.OperationOutputReceipt]) error {
	if err := s.authorize(stream.Context()); err != nil {
		return err
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return status.Error(codes.ResourceExhausted, "Media output capacity exhausted")
	}
	first, err := stream.Recv()
	if err != nil {
		return publicationStatus(err)
	}
	if !contract.Closed(first) || first.GetPublication() == nil {
		return publicationStatus(contract.ErrInvalidOutputPublication)
	}
	req, err := contract.DecodeRequest(first.GetPublication())
	if err != nil {
		return publicationStatus(err)
	}
	receipt, err := s.publisher.Publish(stream.Context(), req, &streamReader{stream: stream})
	if err != nil {
		return publicationStatus(err)
	}
	if err := req.Matches(receipt); err != nil {
		return publicationStatus(err)
	}
	return stream.SendAndClose(contract.EncodeReceipt(receipt))
}

// ResolveOperationOutput returns only the exact original final archive/review ACK.
func (s *Server) ResolveOperationOutput(ctx context.Context, value *pb.OutputPublication) (*pb.OperationOutputReceipt, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	req, err := contract.DecodeRequest(value)
	if err != nil {
		return nil, publicationStatus(err)
	}
	receipt, err := s.publisher.Resolve(ctx, req)
	if err != nil {
		return nil, publicationStatus(err)
	}
	if err := req.Matches(receipt); err != nil {
		return nil, publicationStatus(err)
	}
	return contract.EncodeReceipt(receipt), nil
}

type streamReader struct {
	stream  grpc.ClientStreamingServer[pb.PublishOperationOutputFrame, pb.OperationOutputReceipt]
	pending []byte
}

func (r *streamReader) Read(dst []byte) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}
	if len(r.pending) == 0 {
		frame, err := r.stream.Recv()
		if err != nil {
			return 0, err
		}
		if !contract.Closed(frame) || frame.GetPublication() != nil || len(frame.GetChunk()) == 0 || len(frame.GetChunk()) > ChunkSize {
			return 0, contract.ErrInvalidOutputPublication
		}
		r.pending = frame.GetChunk()
	}
	n := copy(dst, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}
func publicationStatus(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.FromContextError(err).Err()
	}
	switch {
	case errors.Is(err, contract.ErrInvalidOutputPublication), errors.Is(err, writefence.ErrInvalid), errors.Is(err, application.ErrObjectMismatch), errors.Is(err, io.EOF):
		return status.Error(codes.InvalidArgument, "invalid Media output publication")
	case errors.Is(err, contract.ErrOutputPublicationConflict), errors.Is(err, writefence.ErrConflict):
		return status.Error(codes.AlreadyExists, "Media output identity conflict")
	case errors.Is(err, writefence.ErrFenced), errors.Is(err, writefence.ErrFrozen), errors.Is(err, writefence.ErrStale):
		return status.Error(codes.FailedPrecondition, "Media output authority closed")
	case errors.Is(err, contract.ErrOutputPublicationUnknown):
		return status.Error(codes.NotFound, "Media output publication unresolved")
	default:
		return status.Error(codes.Unavailable, "Media output publication unavailable")
	}
}

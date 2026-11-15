package mediaoutput

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediapublication"
	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/outputpublication"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
)

// PublicationProof reads only an already persisted and attempted original command.
type PublicationProof interface {
	VerifyOutputPublication(context.Context, contract.Request) error
}

// ProofServer rejects arbitrary IDs and exposes no provider response or object location.
type ProofServer struct {
	pb.UnimplementedOutputDeliveryProofServer
	store PublicationProof
	token string
}

// NewProofServer requires the dedicated Media-to-Execution proof token and mTLS.
func NewProofServer(store PublicationProof, token string, transport credentials.TransportCredentials) (*grpc.Server, error) {
	if store == nil || !serviceauth.ValidToken(token) || transport == nil {
		return nil, errors.New("invalid output proof listener configuration")
	}
	server := grpc.NewServer(grpc.Creds(transport.Clone()), grpc.MaxRecvMsgSize(messageBytes), grpc.MaxSendMsgSize(messageBytes), grpc.MaxHeaderListSize(16384))
	pb.RegisterOutputDeliveryProofServer(server, &ProofServer{store: store, token: token})
	return server, nil
}

// VerifyOutputPublication echoes only the exact stored immutable original header.
func (s *ProofServer) VerifyOutputPublication(ctx context.Context, value *pb.OutputPublication) (*pb.OutputPublication, error) {
	auth := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(auth) != 1 || subtle.ConstantTimeCompare([]byte(auth[0]), []byte("Bearer "+s.token)) != 1 {
		return nil, status.Error(codes.Unauthenticated, "output proof identity required")
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 10*time.Second {
		return nil, status.Error(codes.InvalidArgument, "bounded output proof deadline required")
	}
	req, err := contract.DecodeRequest(value)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid output proof")
	}
	if err := s.store.VerifyOutputPublication(ctx, req); err != nil {
		if errors.Is(err, contract.ErrOutputPublicationConflict) {
			return nil, status.Error(codes.AlreadyExists, "output proof conflict")
		}
		if errors.Is(err, contract.ErrOutputPublicationUnknown) {
			return nil, status.Error(codes.NotFound, "output proof unresolved")
		}
		return nil, status.Error(codes.FailedPrecondition, "output proof unavailable")
	}
	return contract.EncodeRequest(req), nil
}

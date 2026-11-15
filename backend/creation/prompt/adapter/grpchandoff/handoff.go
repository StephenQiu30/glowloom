// Package grpchandoff exposes Creation's authenticated frozen input commands and recovery.
package grpchandoff

import (
	"context"
	"crypto/subtle"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	cpb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationhandoff"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

const maxMessageBytes = 2 << 20

// FrozenInputs is the source handler's only business capability. It cannot save
// drafts, invent Principals, admit commands or reopen authority heads.
type FrozenInputs interface {
	ReadFrozenInput(context.Context, application.FrozenInputKey) (application.FrozenInputEnvelope, error)
}

// Freezer requires the authenticated source-admission use case. A raw persistence
// store deliberately does not implement Freeze and cannot be passed here.
type Freezer interface {
	Freeze(context.Context, application.FreezeInputRequest) (application.FrozenInputEnvelope, error)
}

type server struct {
	pb.UnimplementedFrozenInputsServer
	store    FrozenInputs
	freezer  Freezer
	compiler Compiler
	token    string
}

// NewServer requires the dedicated Execution token and explicit transport. The
// root supplies verified peer mTLS credentials in production, never a public route.
// Supplying the authenticated Freezer enables commands; omission supports only
// the original immutable recovery capability and cannot grant write authority.
func NewServer(store FrozenInputs, executionToken string, transport credentials.TransportCredentials, freezer ...Freezer) (*grpc.Server, error) {
	if store == nil || !serviceauth.ValidToken(executionToken) || transport == nil || len(freezer) > 1 || (len(freezer) == 1 && freezer[0] == nil) {
		return nil, domain.ErrFrozenInputInvalid
	}
	var source Freezer
	if len(freezer) == 1 {
		source = freezer[0]
	}
	return newServer(store, source, nil, executionToken, transport)
}

func newServer(store FrozenInputs, freezer Freezer, compiler Compiler, executionToken string, transport credentials.TransportCredentials) (*grpc.Server, error) {
	if store == nil || !serviceauth.ValidToken(executionToken) || transport == nil {
		return nil, domain.ErrFrozenInputInvalid
	}
	handler := &server{store: store, freezer: freezer, compiler: compiler, token: executionToken}
	s := grpc.NewServer(grpc.Creds(transport.Clone()), grpc.MaxRecvMsgSize(maxMessageBytes), grpc.MaxSendMsgSize(maxMessageBytes), grpc.MaxHeaderListSize(16384), grpc.UnaryInterceptor(handler.intercept))
	pb.RegisterFrozenInputsServer(s, handler)
	return s, nil
}
func (s *server) intercept(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	auth := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(auth) != 1 || subtle.ConstantTimeCompare([]byte(auth[0]), []byte("Bearer "+s.token)) != 1 {
		return nil, status.Error(codes.Unauthenticated, "service identity required")
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 30*time.Second {
		return nil, rpcError(domain.ErrFrozenInputInvalid)
	}
	if info.FullMethod != pb.FrozenInputs_ResolveFrozenInput_FullMethodName && info.FullMethod != pb.FrozenInputs_FreezeFrozenInput_FullMethodName && info.FullMethod != pb.FrozenInputs_CompileFrozenInput_FullMethodName {
		return nil, status.Error(codes.Unimplemented, "unknown input operation")
	}
	message, ok := request.(proto.Message)
	if !ok || !closed(message) {
		return nil, rpcError(domain.ErrFrozenInputInvalid)
	}
	safe := metadata.MD{}
	carrier := propagation.MapCarrier{}
	for _, key := range []string{"traceparent", "tracestate"} {
		values := metadata.ValueFromIncomingContext(ctx, key)
		if len(values) == 1 {
			safe.Set(key, values[0])
			carrier[key] = values[0]
		}
	}
	ctx = propagation.TraceContext{}.Extract(ctx, carrier)
	return next(metadata.NewIncomingContext(ctx, safe), request)
}
func (s *server) ResolveFrozenInput(ctx context.Context, key *pb.FrozenInputKey) (*pb.FrozenInput, error) {
	local, err := decodeKey(key)
	if err != nil {
		return nil, rpcError(err)
	}
	envelope, err := s.store.ReadFrozenInput(ctx, local)
	if err != nil {
		return nil, rpcError(err)
	}
	if envelope.Key() != local {
		return nil, rpcError(domain.ErrFrozenInputConflict)
	}
	out, err := encodeInput(envelope)
	if err != nil {
		return nil, rpcError(err)
	}
	return out, nil
}
func encodeInput(e application.FrozenInputEnvelope) (*pb.FrozenInput, error) {
	raw, hash, err := e.Input.Snapshot()
	if err != nil || hash != e.SnapshotSHA256 || e.AdoptionBlocked != (e.CancelCommand != nil) {
		return nil, domain.ErrFrozenInputConflict
	}
	f := e.Input
	out := &pb.FrozenInput{Key: encodeKey(e.Key()), RequestId: f.RequestID[:], Command: encodeCommand(f.Command), SourceCommand: encodeCommand(f.SourceCommand), OrgId: f.OrgID[:], AspectRatio: f.AspectRatio, ActorId: f.ActorID[:], SessionId: f.SessionID[:], ScopeId: f.ScopeID[:], ProjectId: f.ProjectID[:], Source: &pb.FrozenInputSource{Kind: f.Source.Kind, DraftId: f.Source.DraftID[:], Revision: f.Source.Revision, CanvasId: f.Source.CanvasID[:], NodeId: f.Source.NodeID[:], CanvasRevision: f.Source.CanvasRevision, InputRevision: f.Source.InputRevision}, SourceGeneration: f.SourceGeneration, DraftRevision: f.DraftRevision, ScopeRevision: f.ScopeRevision, ProjectRevision: f.ProjectRevision, TeamEpoch: f.TeamEpoch, MemberEpoch: f.MemberEpoch, DraftHash: f.DraftHash, InputHash: f.InputHash, CanonicalSnapshotJson: raw, AdoptionBlocked: e.AdoptionBlocked}
	for _, p := range f.Pins {
		out.Pins = append(out.Pins, &pb.FrozenInputPin{ReferenceId: p.ReferenceID, PinId: p.PinID[:], AcquireCommandId: p.AcquireCommandID[:], AssetId: p.AssetID[:], AssetRevision: p.AssetRevision, Kind: p.Kind, SnapshotSha256: p.SnapshotSHA256})
	}
	if e.CancelCommand != nil {
		out.CancelCommand = encodeCommand(*e.CancelCommand)
	}
	return out, nil
}
func encodeKey(k application.FrozenInputKey) *pb.FrozenInputKey {
	return &pb.FrozenInputKey{HandoffId: k.HandoffID[:], CommandId: k.CommandID[:], CommandSha256: k.CommandSHA256, SnapshotSha256: k.SnapshotSHA256}
}
func decodeKey(k *pb.FrozenInputKey) (application.FrozenInputKey, error) {
	var out application.FrozenInputKey
	if k == nil {
		return out, domain.ErrFrozenInputInvalid
	}
	var err error
	out.HandoffID, err = wireUUID(k.HandoffId)
	if err != nil {
		return out, err
	}
	out.CommandID, err = wireUUID(k.CommandId)
	if err != nil {
		return out, err
	}
	out.CommandSHA256 = k.CommandSha256
	out.SnapshotSHA256 = k.SnapshotSha256
	if !hashString(out.CommandSHA256) || !hashString(out.SnapshotSHA256) {
		return out, domain.ErrFrozenInputInvalid
	}
	return out, nil
}
func hashString(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, b := range []byte(s) {
		if (b < '0' || b > '9') && (b < 'a' || b > 'f') {
			return false
		}
	}
	return true
}
func wireUUID(raw []byte) (uuid.UUID, error) {
	id, err := uuid.FromBytes(raw)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, domain.ErrFrozenInputInvalid
	}
	return id, nil
}
func encodeCommand(c writefence.Command) *cpb.Command {
	out := &cpb.Command{Id: c.ID[:], Sha256: c.SHA256, Action: c.Action}
	issuers := map[writefence.Issuer]cpb.Issuer{writefence.Identity: cpb.Issuer_ISSUER_IDENTITY, writefence.Creation: cpb.Issuer_ISSUER_CREATION}
	kinds := map[writefence.Kind]cpb.AuthorityKind{writefence.Account: cpb.AuthorityKind_AUTHORITY_KIND_ACCOUNT, writefence.Session: cpb.AuthorityKind_AUTHORITY_KIND_SESSION, writefence.Scope: cpb.AuthorityKind_AUTHORITY_KIND_SCOPE, writefence.Project: cpb.AuthorityKind_AUTHORITY_KIND_PROJECT}
	for _, a := range c.Authorities {
		out.Authorities = append(out.Authorities, &cpb.Authority{Issuer: issuers[a.Issuer], Kind: kinds[a.Kind], Id: a.ID[:], Epoch: a.Epoch})
	}
	return out
}
func decodeCommand(c *cpb.Command) (writefence.Command, error) {
	var out writefence.Command
	if c == nil {
		return out, domain.ErrFrozenInputInvalid
	}
	var err error
	out.ID, err = wireUUID(c.Id)
	if err != nil {
		return out, err
	}
	out.SHA256 = c.Sha256
	out.Action = c.Action
	issuers := map[cpb.Issuer]writefence.Issuer{cpb.Issuer_ISSUER_IDENTITY: writefence.Identity, cpb.Issuer_ISSUER_CREATION: writefence.Creation}
	kinds := map[cpb.AuthorityKind]writefence.Kind{cpb.AuthorityKind_AUTHORITY_KIND_ACCOUNT: writefence.Account, cpb.AuthorityKind_AUTHORITY_KIND_SESSION: writefence.Session, cpb.AuthorityKind_AUTHORITY_KIND_SCOPE: writefence.Scope, cpb.AuthorityKind_AUTHORITY_KIND_PROJECT: writefence.Project}
	for _, a := range c.Authorities {
		if a == nil {
			return out, domain.ErrFrozenInputInvalid
		}
		id, err := wireUUID(a.Id)
		if err != nil {
			return out, err
		}
		out.Authorities = append(out.Authorities, writefence.Authority{Issuer: issuers[a.Issuer], Kind: kinds[a.Kind], ID: id, Epoch: a.Epoch})
	}
	_, err = out.AuthorityDigest()
	return out, err
}
func closed(m proto.Message) bool {
	if m == nil {
		return false
	}
	return closedReflection(m.ProtoReflect())
}
func closedReflection(m protoreflect.Message) bool {
	if !m.IsValid() || len(m.GetUnknown()) != 0 {
		return false
	}
	valid := true
	m.Range(func(f protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if f.IsList() && f.Message() != nil {
			list := v.List()
			for i := 0; i < list.Len(); i++ {
				if !closedReflection(list.Get(i).Message()) {
					valid = false
					return false
				}
			}
		} else if f.Message() != nil {
			valid = closedReflection(v.Message())
		}
		return valid
	})
	return valid
}
func rpcError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.FromContextError(err).Err()
	}
	code, reason := codes.Unavailable, pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_UNAVAILABLE
	switch {
	case errors.Is(err, application.ErrTemplateInvalid):
		code, reason = codes.InvalidArgument, pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_TEMPLATE_INVALID
	case errors.Is(err, application.ErrTemplateChanged):
		code, reason = codes.Aborted, pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_TEMPLATE_CHANGED
	case errors.Is(err, application.ErrTemplateContextUnavailable):
		code, reason = codes.FailedPrecondition, pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_TEMPLATE_CONTEXT_UNAVAILABLE
	case errors.Is(err, domain.ErrUnsupportedSchema):
		code, reason = codes.FailedPrecondition, pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_UNSUPPORTED_COMPILER
	case errors.Is(err, domain.ErrInputStale):
		code, reason = codes.Aborted, pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_SOURCE_STALE
	case errors.Is(err, domain.ErrReferenceUnavailable):
		code, reason = codes.FailedPrecondition, pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_REFERENCE_UNAVAILABLE
	case errors.Is(err, domain.ErrFrozenInputInvalid), errors.Is(err, domain.ErrInvalidDraft), errors.Is(err, writefence.ErrInvalid):
		code = codes.InvalidArgument
		reason = pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_INVALID
	case errors.Is(err, domain.ErrFrozenInputConflict), errors.Is(err, writefence.ErrConflict):
		code = codes.AlreadyExists
		reason = pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_CONFLICT
	case errors.Is(err, domain.ErrFrozenInputBlocked), errors.Is(err, writefence.ErrFrozen):
		code = codes.FailedPrecondition
		reason = pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_CANCELLED
	case errors.Is(err, writefence.ErrFenced):
		code = codes.PermissionDenied
		reason = pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_FENCED
	case errors.Is(err, domain.ErrFrozenInputNotFound), errors.Is(err, writefence.ErrNotFound):
		code = codes.NotFound
		reason = pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_NOT_FOUND
	case errors.Is(err, domain.ErrFrozenInputOwner):
		code = codes.PermissionDenied
		reason = pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_WRONG_OWNER
	}
	s, e := status.New(code, "frozen input unavailable").WithDetails(&pb.FrozenInputFailure{Reason: reason})
	if e != nil {
		return status.Error(codes.Unavailable, "frozen input unavailable")
	}
	return s.Err()
}
func localError(err error) error {
	if errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) || status.Code(err) == codes.DeadlineExceeded {
		return context.DeadlineExceeded
	}
	for _, detail := range status.Convert(err).Details() {
		if f, ok := detail.(*pb.FrozenInputFailure); ok && closed(f) {
			switch f.Reason {
			case pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_INVALID:
				return domain.ErrFrozenInputInvalid
			case pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_CONFLICT:
				return domain.ErrFrozenInputConflict
			case pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_CANCELLED:
				return domain.ErrFrozenInputBlocked
			case pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_FENCED:
				return writefence.ErrFenced
			case pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_NOT_FOUND:
				return domain.ErrFrozenInputNotFound
			case pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_EXECUTION_RESERVED:
				return domain.ErrFrozenInputReserved
			case pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_WRONG_OWNER:
				return domain.ErrFrozenInputOwner
			}
		}
	}
	return writefence.ErrUnavailable
}

// Target is an explicit private address, per-operation caller token and injected
// transport. TLS peer identity validation is the composition root's responsibility.
type Target struct {
	Address, Token string
	Transport      credentials.TransportCredentials
}

// CancellationClient closes the exact original input at Execution without retry.
type CancellationClient struct{ target Target }

// NewCancellationClient requires the dedicated Creation caller capability and explicit transport.
func NewCancellationClient(target Target) (*CancellationClient, error) {
	host, port, err := net.SplitHostPort(target.Address)
	number, e := strconv.Atoi(port)
	if err != nil || e != nil || host == "" || strings.ContainsAny(host, "/@?#% \t\r\n") || number < 1 || number > 65535 || !serviceauth.ValidToken(target.Token) || target.Transport == nil {
		return nil, domain.ErrFrozenInputInvalid
	}
	target.Transport = target.Transport.Clone()
	return &CancellationClient{target}, nil
}

// CancelFrozenInput retains the original command, with no retry or refund inference.
func (c *CancellationClient) CancelFrozenInput(ctx context.Context, key application.FrozenInputKey, command writefence.Command) (application.FrozenInputCancelProof, error) {
	var proof application.FrozenInputCancelProof
	if c == nil || command.Action != "creation.frozen_input.cancel" {
		return proof, domain.ErrFrozenInputInvalid
	}
	if _, err := decodeKey(encodeKey(key)); err != nil {
		return proof, err
	}
	if _, err := command.AuthorityDigest(); err != nil {
		return proof, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	md := metadata.Pairs("authorization", "Bearer "+c.target.Token)
	for _, k := range []string{"traceparent", "tracestate"} {
		if v := carrier[k]; v != "" {
			md.Set(k, v)
		}
	}
	ctx = metadata.NewOutgoingContext(ctx, md)
	conn, err := grpc.NewClient(c.target.Address, grpc.WithTransportCredentials(c.target.Transport.Clone()), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMessageBytes), grpc.MaxCallSendMsgSize(maxMessageBytes), grpc.MaxRetryRPCBufferSize(0)))
	if err != nil {
		return proof, writefence.ErrUnavailable
	}
	defer func() { _ = conn.Close() }()
	response, err := pb.NewFrozenInputConsumerClient(conn).CancelFrozenInput(ctx, &pb.CancelFrozenInputRequest{Key: encodeKey(key), CancelCommand: encodeCommand(command)})
	if err != nil {
		return proof, localError(err)
	}
	if !closed(response) || response.Original == nil {
		return proof, domain.ErrFrozenInputConflict
	}
	proof.Key, err = decodeKey(response.Key)
	if err != nil || proof.Key != key {
		return proof, domain.ErrFrozenInputConflict
	}
	proof.CancelCommandID, err = wireUUID(response.CancelCommandId)
	if err != nil || proof.CancelCommandID != command.ID {
		return proof, domain.ErrFrozenInputConflict
	}
	proof.Original.Command, err = decodeCommand(response.Original.Command)
	if err != nil || proof.Original.Command.ID != key.CommandID || proof.Original.Command.SHA256 != key.CommandSHA256 {
		return proof, domain.ErrFrozenInputConflict
	}
	switch response.Outcome {
	case pb.FrozenInputCancellationOutcome_FROZEN_INPUT_CANCELLATION_OUTCOME_NOT_RECEIVED_FROZEN:
		if response.Original.State != cpb.CommandState_COMMAND_STATE_FROZEN || len(response.Original.ReceiptJson) != 0 {
			return proof, domain.ErrFrozenInputConflict
		}
		proof.Outcome = "not_received_frozen"
		proof.Original.State = writefence.Frozen
	case pb.FrozenInputCancellationOutcome_FROZEN_INPUT_CANCELLATION_OUTCOME_CANCELLED_INPUT:
		if response.Original.State != cpb.CommandState_COMMAND_STATE_COMMITTED {
			return proof, domain.ErrFrozenInputConflict
		}
		proof.Outcome = "cancelled_input"
		proof.Original.State = writefence.Committed
		proof.Original.Receipt, err = writefence.CanonicalReceipt(response.Original.ReceiptJson)
		if err != nil {
			return proof, err
		}
	default:
		return proof, domain.ErrFrozenInputConflict
	}
	return proof, nil
}

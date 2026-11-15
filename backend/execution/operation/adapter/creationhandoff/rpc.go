// Package creationhandoff adapts the closed Creation wire contract to Execution's
// consumer-owned ports. It imports no Creation business/application/ORM package.
package creationhandoff

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
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

	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	cpb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationhandoff"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

const maxMessageBytes = 2 << 20

// Target is supplied only by the private composition root, including verified
// peer transport credentials and a dedicated Execution-to-Creation token.
type Target struct {
	Address, Token string
	Transport      credentials.TransportCredentials
}

// SourceClient reads closed immutable facts from authenticated Creation.
type SourceClient struct{ target Target }

// NewSourceClient requires an explicit private address, token and peer transport.
func NewSourceClient(target Target) (*SourceClient, error) {
	host, port, err := net.SplitHostPort(target.Address)
	number, e := strconv.Atoi(port)
	if err != nil || e != nil || host == "" || strings.ContainsAny(host, "/@?#% \t\r\n") || number < 1 || number > 65535 || !serviceauth.ValidToken(target.Token) || target.Transport == nil {
		return nil, application.ErrFrozenInputInvalid
	}
	target.Transport = target.Transport.Clone()
	return &SourceClient{target}, nil
}

// ReadFrozenInput verifies complete snapshot content against all typed metadata.
func (c *SourceClient) ReadFrozenInput(ctx context.Context, key application.FrozenInputKey) (application.FrozenInput, error) {
	var out application.FrozenInput
	if c == nil {
		return out, application.ErrFrozenInputInvalid
	}
	if _, err := decodeKey(encodeKey(key)); err != nil {
		return out, err
	}
	if err := ctx.Err(); err != nil {
		return out, err
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
		return out, writefence.ErrUnavailable
	}
	defer func() { _ = conn.Close() }()
	response, err := pb.NewFrozenInputsClient(conn).ResolveFrozenInput(ctx, encodeKey(key))
	if err != nil {
		return out, localError(err)
	}
	out, err = decodeInput(response)
	if err != nil || out.Key != key {
		return application.FrozenInput{}, application.ErrFrozenInputConflict
	}
	return out, nil
}

// snapshot is the wire document, not a second editable domain model. Its only
// purpose is full-content/metadata binding; no quote or source policy runs here.
type snapshot struct {
	HandoffID         uuid.UUID                  `json:"handoff_id"`
	RequestID         uuid.UUID                  `json:"request_id"`
	Command           writefence.Command         `json:"command"`
	SourceCommand     writefence.Command         `json:"source_command"`
	ActorID           uuid.UUID                  `json:"actor_id"`
	SessionID         uuid.UUID                  `json:"session_id"`
	ScopeID           uuid.UUID                  `json:"scope_id"`
	ProjectID         uuid.UUID                  `json:"project_id"`
	OrgID             uuid.UUID                  `json:"org_id"`
	AspectRatio       string                     `json:"aspect_ratio"`
	Source            snapshotSource             `json:"source"`
	SourceGeneration  int64                      `json:"source_generation"`
	DraftRevision     int64                      `json:"draft_revision"`
	ScopeRevision     int64                      `json:"scope_revision"`
	ProjectRevision   int64                      `json:"project_revision"`
	TeamEpoch         int64                      `json:"team_epoch"`
	MemberEpoch       int64                      `json:"member_epoch"`
	DraftHash         string                     `json:"draft_hash"`
	InputHash         string                     `json:"input_hash"`
	Draft             json.RawMessage            `json:"draft"`
	Pins              []snapshotPin              `json:"pins"`
	CompilationInputs *snapshotCompilationInputs `json:"compilation_inputs,omitempty"`
}
type snapshotSource struct {
	Kind           string    `json:"kind"`
	DraftID        uuid.UUID `json:"draft_id,omitempty"`
	Revision       int64     `json:"revision,omitempty"`
	CanvasID       uuid.UUID `json:"canvas_id,omitempty"`
	NodeID         uuid.UUID `json:"node_id,omitempty"`
	CanvasRevision int64     `json:"canvas_revision,omitempty"`
	InputRevision  int64     `json:"input_revision,omitempty"`
}
type snapshotPin struct {
	ReferenceID      string    `json:"reference_id"`
	PinID            uuid.UUID `json:"pin_id"`
	AcquireCommandID uuid.UUID `json:"acquire_command_id"`
	AssetID          uuid.UUID `json:"asset_id"`
	AssetRevision    int64     `json:"asset_revision"`
	Kind             string    `json:"kind"`
	SnapshotSHA256   string    `json:"snapshot_sha256"`
}

func decodeInput(wire *pb.FrozenInput) (application.FrozenInput, error) {
	var out application.FrozenInput
	if !closed(wire) || len(wire.CanonicalSnapshotJson) == 0 || len(wire.CanonicalSnapshotJson) > 768<<10 {
		return out, application.ErrFrozenInputInvalid
	}
	var f snapshot
	decoder := json.NewDecoder(bytes.NewReader(wire.CanonicalSnapshotJson))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&f) != nil || f.ScopeRevision < 1 || f.ProjectRevision < 1 || f.TeamEpoch < 0 || f.MemberEpoch < 0 || (f.TeamEpoch == 0) != (f.MemberEpoch == 0) || f.Pins == nil || !hashString(f.DraftHash) || !hashString(f.InputHash) || len(f.Draft) == 0 {
		return out, application.ErrFrozenInputInvalid
	}
	// Marshal checks trailing bytes and the canonical representation of UUIDs,
	// authorities, fields and ordering. No scalar number passes through float64.
	raw, err := json.Marshal(f)
	if err != nil {
		return out, application.ErrFrozenInputConflict
	}
	raw, err = writefence.CanonicalReceipt(raw)
	if err != nil || !bytes.Equal(raw, wire.CanonicalSnapshotJson) {
		return out, application.ErrFrozenInputConflict
	}
	sourceDigest, sourceErr := f.SourceCommand.AuthorityDigest()
	targetDigest, targetErr := f.Command.AuthorityDigest()
	if sourceErr != nil || targetErr != nil || sourceDigest != targetDigest || f.SourceCommand.ID == f.Command.ID || f.SourceCommand.ID != uuid.NewSHA1(f.HandoffID, []byte("creation.frozen_input.freeze")) || f.SourceCommand.Action != "creation.frozen_input.freeze" {
		return out, application.ErrFrozenInputInvalid
	}
	source := f.Source
	switch source.Kind {
	case "prompt_draft":
		if source.DraftID == uuid.Nil || source.Revision < 1 || source.CanvasID != uuid.Nil || source.NodeID != uuid.Nil || source.CanvasRevision != 0 || source.InputRevision != 0 || f.DraftRevision != source.Revision {
			return out, application.ErrFrozenInputInvalid
		}
	case "canvas_input":
		if source.DraftID != uuid.Nil || source.Revision != 0 || source.CanvasID == uuid.Nil || source.NodeID == uuid.Nil || source.CanvasRevision < 1 || source.InputRevision < 1 || f.DraftRevision != source.InputRevision {
			return out, application.ErrFrozenInputInvalid
		}
	default:
		return out, application.ErrFrozenInputInvalid
	}
	key, err := decodeKey(wire.Key)
	if err != nil || key.HandoffID != f.HandoffID || key.CommandID != f.Command.ID || key.CommandSHA256 != f.Command.SHA256 {
		return out, application.ErrFrozenInputConflict
	}
	expected := &pb.FrozenInput{Key: encodeKey(key), RequestId: f.RequestID[:], Command: encodeCommand(f.Command), SourceCommand: encodeCommand(f.SourceCommand), ActorId: f.ActorID[:], SessionId: f.SessionID[:], ScopeId: f.ScopeID[:], ProjectId: f.ProjectID[:], OrgId: f.OrgID[:], AspectRatio: f.AspectRatio, Source: &pb.FrozenInputSource{Kind: source.Kind, DraftId: source.DraftID[:], Revision: source.Revision, CanvasId: source.CanvasID[:], NodeId: source.NodeID[:], CanvasRevision: source.CanvasRevision, InputRevision: source.InputRevision}, SourceGeneration: f.SourceGeneration, DraftRevision: f.DraftRevision, ScopeRevision: f.ScopeRevision, ProjectRevision: f.ProjectRevision, TeamEpoch: f.TeamEpoch, MemberEpoch: f.MemberEpoch, DraftHash: f.DraftHash, InputHash: f.InputHash, CanonicalSnapshotJson: wire.CanonicalSnapshotJson, AdoptionBlocked: wire.AdoptionBlocked, CancelCommand: wire.CancelCommand}
	for _, p := range f.Pins {
		expected.Pins = append(expected.Pins, &pb.FrozenInputPin{ReferenceId: p.ReferenceID, PinId: p.PinID[:], AcquireCommandId: p.AcquireCommandID[:], AssetId: p.AssetID[:], AssetRevision: p.AssetRevision, Kind: p.Kind, SnapshotSha256: p.SnapshotSHA256})
	}
	if !proto.Equal(expected, wire) {
		return out, application.ErrFrozenInputConflict
	}
	out = application.FrozenInput{Key: key, RequestID: f.RequestID, Command: f.Command, ActorID: f.ActorID, SessionID: f.SessionID, ScopeID: f.ScopeID, ProjectID: f.ProjectID, OrgID: f.OrgID, AspectRatio: f.AspectRatio, SourceGeneration: f.SourceGeneration, DraftRevision: f.DraftRevision, Snapshot: append([]byte(nil), raw...), AdoptionBlocked: wire.AdoptionBlocked}
	if wire.CancelCommand != nil {
		command, err := decodeCommand(wire.CancelCommand)
		if err != nil || command.Action != "creation.frozen_input.cancel" || command.ID == key.CommandID || len(command.Authorities) != 4 {
			return out, application.ErrFrozenInputInvalid
		}
		expectedIDs := map[string]uuid.UUID{"identity/account": out.ActorID, "identity/session": out.SessionID, "creation/scope": out.ScopeID, "creation/project": out.ProjectID}
		for _, a := range command.Authorities {
			if expectedIDs[string(a.Issuer)+"/"+string(a.Kind)] != a.ID {
				return out, application.ErrFrozenInputInvalid
			}
		}
		out.CancelCommand = &command
	}
	return out, out.Validate()
}

// NewConsumerServer authenticates Creation before invoking the consumer-owned
// port. No callerPrincipal exists in the protocol or becomes an authority.
func NewConsumerServer(store application.FrozenInputAcceptance, creationToken string, transport credentials.TransportCredentials) (*grpc.Server, error) {
	if store == nil || !serviceauth.ValidToken(creationToken) || transport == nil {
		return nil, application.ErrFrozenInputInvalid
	}
	handler := &server{store: store, token: creationToken}
	s := grpc.NewServer(grpc.Creds(transport.Clone()), grpc.MaxRecvMsgSize(maxMessageBytes), grpc.MaxSendMsgSize(maxMessageBytes), grpc.MaxHeaderListSize(16384), grpc.UnaryInterceptor(handler.intercept))
	pb.RegisterFrozenInputConsumerServer(s, handler)
	return s, nil
}

type server struct {
	pb.UnimplementedFrozenInputConsumerServer
	store application.FrozenInputAcceptance
	token string
}

func (s *server) intercept(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	auth := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(auth) != 1 || subtle.ConstantTimeCompare([]byte(auth[0]), []byte("Bearer "+s.token)) != 1 {
		return nil, status.Error(codes.Unauthenticated, "service identity required")
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 30*time.Second {
		return nil, rpcError(application.ErrFrozenInputInvalid)
	}
	if info.FullMethod != pb.FrozenInputConsumer_AcceptFrozenInput_FullMethodName && info.FullMethod != pb.FrozenInputConsumer_CancelFrozenInput_FullMethodName {
		return nil, status.Error(codes.Unimplemented, "unknown input operation")
	}
	message, ok := request.(proto.Message)
	if !ok || !closed(message) {
		return nil, rpcError(application.ErrFrozenInputInvalid)
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
func (s *server) AcceptFrozenInput(ctx context.Context, key *pb.FrozenInputKey) (*pb.FrozenInputAcceptance, error) {
	local, err := decodeKey(key)
	if err != nil {
		return nil, rpcError(err)
	}
	receipt, err := s.store.AcceptFrozenInput(ctx, local)
	if err != nil {
		return nil, rpcError(err)
	}
	if receipt.HandoffID != local.HandoffID || receipt.CommandID != local.CommandID || receipt.CommandSHA256 != local.CommandSHA256 || receipt.SnapshotSHA256 != local.SnapshotSHA256 {
		return nil, rpcError(application.ErrFrozenInputConflict)
	}
	return &pb.FrozenInputAcceptance{Key: encodeKey(local), RequestId: receipt.RequestID[:], OperationId: receipt.OperationID[:], SubmissionId: receipt.SubmissionID[:], ReceiptId: receipt.ReceiptID[:]}, nil
}
func (s *server) CancelFrozenInput(ctx context.Context, req *pb.CancelFrozenInputRequest) (*pb.FrozenInputCancellation, error) {
	key, err := decodeKey(req.Key)
	if err != nil {
		return nil, rpcError(err)
	}
	command, err := decodeCommand(req.CancelCommand)
	if err != nil {
		return nil, rpcError(err)
	}
	proof, err := s.store.CancelFrozenInput(ctx, key, command)
	if err != nil {
		return nil, rpcError(err)
	}
	if proof.Key != key || proof.CancelCommandID != command.ID {
		return nil, rpcError(application.ErrFrozenInputConflict)
	}
	out := &pb.FrozenInputCancellation{Key: encodeKey(key), CancelCommandId: command.ID[:], Original: &cpb.ResolveResponse{Command: encodeCommand(proof.Original.Command), ReceiptJson: proof.Original.Receipt}}
	switch proof.Outcome {
	case "not_received_frozen":
		if proof.Original.State != writefence.Frozen || len(proof.Original.Receipt) != 0 {
			return nil, rpcError(application.ErrFrozenInputConflict)
		}
		out.Outcome = pb.FrozenInputCancellationOutcome_FROZEN_INPUT_CANCELLATION_OUTCOME_NOT_RECEIVED_FROZEN
		out.Original.State = cpb.CommandState_COMMAND_STATE_FROZEN
	case "cancelled_input":
		if proof.Original.State != writefence.Committed {
			return nil, rpcError(application.ErrFrozenInputConflict)
		}
		out.Outcome = pb.FrozenInputCancellationOutcome_FROZEN_INPUT_CANCELLATION_OUTCOME_CANCELLED_INPUT
		out.Original.State = cpb.CommandState_COMMAND_STATE_COMMITTED
	default:
		return nil, rpcError(application.ErrFrozenInputConflict)
	}
	return out, nil
}
func encodeKey(k application.FrozenInputKey) *pb.FrozenInputKey {
	return &pb.FrozenInputKey{HandoffId: k.HandoffID[:], CommandId: k.CommandID[:], CommandSha256: k.CommandSHA256, SnapshotSha256: k.SnapshotSHA256}
}
func decodeKey(k *pb.FrozenInputKey) (application.FrozenInputKey, error) {
	var out application.FrozenInputKey
	if k == nil {
		return out, application.ErrFrozenInputInvalid
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
		return out, application.ErrFrozenInputInvalid
	}
	return out, nil
}
func wireUUID(raw []byte) (uuid.UUID, error) {
	id, err := uuid.FromBytes(raw)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, application.ErrFrozenInputInvalid
	}
	return id, nil
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
		return out, application.ErrFrozenInputInvalid
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
			return out, application.ErrFrozenInputInvalid
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
	code, reason := codes.Unavailable, pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_UNAVAILABLE
	switch {
	case errors.Is(err, application.ErrFrozenInputInvalid), errors.Is(err, writefence.ErrInvalid):
		code = codes.InvalidArgument
		reason = pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_INVALID
	case errors.Is(err, application.ErrFrozenInputConflict), errors.Is(err, writefence.ErrConflict):
		code = codes.AlreadyExists
		reason = pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_CONFLICT
	case errors.Is(err, application.ErrFrozenInputCancelled), errors.Is(err, writefence.ErrFrozen):
		code = codes.FailedPrecondition
		reason = pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_CANCELLED
	case errors.Is(err, writefence.ErrFenced):
		code = codes.PermissionDenied
		reason = pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_FENCED
	case errors.Is(err, writefence.ErrNotFound):
		code = codes.NotFound
		reason = pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_NOT_FOUND
	case errors.Is(err, application.ErrSubmissionAccepted):
		code = codes.FailedPrecondition
		reason = pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_EXECUTION_RESERVED
	case errors.Is(err, application.ErrFrozenInputOwner):
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
				return application.ErrFrozenInputInvalid
			case pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_CONFLICT:
				return application.ErrFrozenInputConflict
			case pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_CANCELLED:
				return application.ErrFrozenInputCancelled
			case pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_FENCED:
				return writefence.ErrFenced
			case pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_NOT_FOUND:
				return writefence.ErrNotFound
			case pb.FrozenInputErrorReason_FROZEN_INPUT_ERROR_REASON_WRONG_OWNER:
				return application.ErrFrozenInputOwner
			}
		}
	}
	return writefence.ErrUnavailable
}

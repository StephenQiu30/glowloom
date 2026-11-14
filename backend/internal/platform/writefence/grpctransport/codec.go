// Package grpctransport exposes only authenticated writer-local control and
// original-command recovery. It never grants business authorization.
package grpctransport

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// MaxMessageBytes bounds a canonical receipt plus its closed identity envelope.
const MaxMessageBytes = writefence.MaxReceiptBytes + (16 << 10)

func encodeWriter(writer writefence.Writer) pb.Writer {
	switch writer {
	case writefence.WriterIdentity:
		return pb.Writer_WRITER_IDENTITY
	case writefence.WriterCreation:
		return pb.Writer_WRITER_CREATION
	case writefence.WriterMedia:
		return pb.Writer_WRITER_MEDIA
	case writefence.WriterExecution:
		return pb.Writer_WRITER_EXECUTION
	default:
		return pb.Writer_WRITER_UNSPECIFIED
	}
}

func encodeAuthority(authority writefence.Authority) *pb.Authority {
	issuer := pb.Issuer_ISSUER_UNSPECIFIED
	switch authority.Issuer {
	case writefence.Identity:
		issuer = pb.Issuer_ISSUER_IDENTITY
	case writefence.Creation:
		issuer = pb.Issuer_ISSUER_CREATION
	}
	kind := pb.AuthorityKind_AUTHORITY_KIND_UNSPECIFIED
	switch authority.Kind {
	case writefence.Account:
		kind = pb.AuthorityKind_AUTHORITY_KIND_ACCOUNT
	case writefence.Session:
		kind = pb.AuthorityKind_AUTHORITY_KIND_SESSION
	case writefence.Scope:
		kind = pb.AuthorityKind_AUTHORITY_KIND_SCOPE
	case writefence.Project:
		kind = pb.AuthorityKind_AUTHORITY_KIND_PROJECT
	case writefence.AgentGrant:
		kind = pb.AuthorityKind_AUTHORITY_KIND_AGENT_GRANT
	}
	return &pb.Authority{Issuer: issuer, Kind: kind, Id: append([]byte(nil), authority.ID[:]...), Epoch: authority.Epoch}
}

func decodeAuthority(value *pb.Authority) (writefence.Authority, error) {
	if value == nil {
		return writefence.Authority{}, writefence.ErrInvalid
	}
	id, err := uuid.FromBytes(value.GetId())
	if err != nil {
		return writefence.Authority{}, writefence.ErrInvalid
	}
	authority := writefence.Authority{ID: id, Epoch: value.GetEpoch()}
	switch value.GetIssuer() {
	case pb.Issuer_ISSUER_IDENTITY:
		authority.Issuer = writefence.Identity
	case pb.Issuer_ISSUER_CREATION:
		authority.Issuer = writefence.Creation
	default:
		return writefence.Authority{}, writefence.ErrInvalid
	}
	switch value.GetKind() {
	case pb.AuthorityKind_AUTHORITY_KIND_ACCOUNT:
		authority.Kind = writefence.Account
	case pb.AuthorityKind_AUTHORITY_KIND_SESSION:
		authority.Kind = writefence.Session
	case pb.AuthorityKind_AUTHORITY_KIND_SCOPE:
		authority.Kind = writefence.Scope
	case pb.AuthorityKind_AUTHORITY_KIND_PROJECT:
		authority.Kind = writefence.Project
	case pb.AuthorityKind_AUTHORITY_KIND_AGENT_GRANT:
		authority.Kind = writefence.AgentGrant
	default:
		return writefence.Authority{}, writefence.ErrInvalid
	}
	return authority, authority.Validate()
}

func encodeControl(control writefence.Control) *pb.Control {
	return &pb.Control{Authority: encodeAuthority(control.Authority), Blocked: control.Blocked, CoordinationId: append([]byte(nil), control.CoordinationID[:]...)}
}

func decodeControl(value *pb.Control) (writefence.Control, error) {
	if value == nil {
		return writefence.Control{}, writefence.ErrInvalid
	}
	authority, err := decodeAuthority(value.GetAuthority())
	if err != nil {
		return writefence.Control{}, err
	}
	id, err := uuid.FromBytes(value.GetCoordinationId())
	if err != nil {
		return writefence.Control{}, writefence.ErrInvalid
	}
	control := writefence.Control{Authority: authority, Blocked: value.GetBlocked(), CoordinationID: id}
	return control, control.Validate()
}

func encodeCommand(command writefence.Command) *pb.Command {
	authorities := make([]*pb.Authority, len(command.Authorities))
	for i, authority := range command.Authorities {
		authorities[i] = encodeAuthority(authority)
	}
	return &pb.Command{Id: append([]byte(nil), command.ID[:]...), Sha256: command.SHA256, Action: command.Action, Authorities: authorities}
}

func decodeCommand(value *pb.Command) (writefence.Command, error) {
	if value == nil || len(value.GetAuthorities()) > writefence.MaxAuthorities {
		return writefence.Command{}, writefence.ErrInvalid
	}
	id, err := uuid.FromBytes(value.GetId())
	if err != nil {
		return writefence.Command{}, writefence.ErrInvalid
	}
	command := writefence.Command{ID: id, SHA256: value.GetSha256(), Action: value.GetAction(), Authorities: make([]writefence.Authority, len(value.GetAuthorities()))}
	for i, value := range value.GetAuthorities() {
		command.Authorities[i], err = decodeAuthority(value)
		if err != nil {
			return writefence.Command{}, err
		}
	}
	_, err = command.AuthorityDigest()
	return command, err
}

func sameCommand(left, right writefence.Command) bool {
	if left.ID != right.ID || left.SHA256 != right.SHA256 || left.Action != right.Action {
		return false
	}
	a, err := left.AuthorityDigest()
	if err != nil {
		return false
	}
	b, err := right.AuthorityDigest()
	return err == nil && a == b
}

var reasons = []struct {
	err    error
	reason pb.ErrorReason
	code   codes.Code
}{
	{writefence.ErrInvalid, pb.ErrorReason_ERROR_REASON_INVALID, codes.InvalidArgument},
	{writefence.ErrFenced, pb.ErrorReason_ERROR_REASON_FENCED, codes.FailedPrecondition},
	{writefence.ErrConflict, pb.ErrorReason_ERROR_REASON_CONFLICT, codes.AlreadyExists},
	{writefence.ErrStale, pb.ErrorReason_ERROR_REASON_STALE, codes.FailedPrecondition},
	{writefence.ErrNotFound, pb.ErrorReason_ERROR_REASON_NOT_FOUND, codes.NotFound},
	{writefence.ErrCommitted, pb.ErrorReason_ERROR_REASON_COMMITTED, codes.FailedPrecondition},
	{writefence.ErrFrozen, pb.ErrorReason_ERROR_REASON_FROZEN, codes.FailedPrecondition},
}

func rpcError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.FromContextError(err).Err()
	}
	for _, reason := range reasons {
		if errors.Is(err, reason.err) {
			value, detailErr := status.New(reason.code, "writer control rejected").WithDetails(&pb.Failure{Reason: reason.reason})
			if detailErr != nil {
				return status.Error(codes.Internal, "writer control unavailable")
			}
			return value.Err()
		}
	}
	return status.Error(codes.Unavailable, "writer control unavailable")
}

func localError(err error) error {
	value, ok := status.FromError(err)
	if !ok {
		return writefence.ErrUnavailable
	}
	for _, detail := range value.Details() {
		failure, ok := detail.(*pb.Failure)
		if !ok {
			continue
		}
		for _, reason := range reasons {
			if failure.GetReason() == reason.reason && value.Code() == reason.code {
				return reason.err
			}
		}
	}
	return writefence.ErrUnavailable
}

// Package grpcauthority transports private Identity verification and exact command
// admission. It grants no scope, platform, team or effect-owner permission.
package grpcauthority

import (
	"context"
	"encoding/base64"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// MaxMessageBytes bounds the entire private credential and command envelope.
const MaxMessageBytes = 32 << 10

// Identity contains only authenticated account and exact session references.
// It is not an admission receipt or permission to execute a business command.
type Identity struct {
	AccountID       uuid.UUID
	SessionID       uuid.UUID
	CredentialEpoch int64
}

// SessionAuthorities is a current read snapshot for this exact authenticated
// identity. Neither generation is derived from CredentialEpoch or grants a write.
type SessionAuthorities struct {
	Identity         Identity
	Account, Session writefence.Authority
}

func (s SessionAuthorities) valid() bool {
	return s.Identity.AccountID != uuid.Nil && s.Identity.SessionID != uuid.Nil && s.Identity.CredentialEpoch > 0 && s.Account.Validate() == nil && s.Session.Validate() == nil && s.Account.Issuer == writefence.Identity && s.Session.Issuer == writefence.Identity && s.Account.Kind == writefence.Account && s.Session.Kind == writefence.Session && s.Account.ID == s.Identity.AccountID && s.Session.ID == s.Identity.SessionID
}

func encodeSessionAuthority(a writefence.Authority) *coordination.Authority {
	kind := coordination.AuthorityKind_AUTHORITY_KIND_ACCOUNT
	if a.Kind == writefence.Session {
		kind = coordination.AuthorityKind_AUTHORITY_KIND_SESSION
	}
	return &coordination.Authority{Issuer: coordination.Issuer_ISSUER_IDENTITY, Kind: kind, Id: append([]byte(nil), a.ID[:]...), Epoch: a.Epoch}
}

func decodeSessionAuthority(a *coordination.Authority, kind writefence.Kind) (writefence.Authority, error) {
	if a == nil || a.Issuer != coordination.Issuer_ISSUER_IDENTITY {
		return writefence.Authority{}, writefence.ErrUnavailable
	}
	expected := coordination.AuthorityKind_AUTHORITY_KIND_ACCOUNT
	if kind == writefence.Session {
		expected = coordination.AuthorityKind_AUTHORITY_KIND_SESSION
	}
	id, err := uuid.FromBytes(a.Id)
	out := writefence.Authority{Issuer: writefence.Identity, Kind: kind, ID: id, Epoch: a.Epoch}
	if err != nil || a.Kind != expected || out.Validate() != nil {
		return writefence.Authority{}, writefence.ErrUnavailable
	}
	return out, nil
}

// AdmissionProof is evidence of one existing Identity admission for its original
// target. Reading it neither grants resource policy nor proves business commit.
type AdmissionProof struct {
	Identity Identity
	Command  writefence.Command
	Target   writefence.Writer
}

// Credential is closed to legacy and access proofs. It never contains a Principal.
// Its private fields and redacted formatting prevent accidental credential logs.
type Credential struct{ legacy, access, csrf string }

// LegacyCredential supplies an original legacy session token. CSRF may be empty
// for Verify; command admission always requires the original CSRF proof.
func LegacyCredential(token, csrf string) Credential { return Credential{legacy: token, csrf: csrf} }

// AccessCredential supplies an original access token, never refresh. CSRF may be
// empty for Verify; command admission always requires the original CSRF proof.
func AccessCredential(token, csrf string) Credential { return Credential{access: token, csrf: csrf} }

// String redacts credentials in ordinary formatting and structured string logs.
func (Credential) String() string { return "[private identity credential]" }

// GoString also redacts credentials when formatted with the Go-syntax verb.
func (Credential) GoString() string { return "[private identity credential]" }

func validProof(value string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(raw) == 32 && base64.RawURLEncoding.EncodeToString(raw) == value
}
func validCredential(c Credential) bool {
	if c.csrf != "" && !validProof(c.csrf) {
		return false
	}
	if c.legacy != "" {
		return c.access == "" && validProof(c.legacy)
	}
	if len(c.access) == 0 || len(c.access) > 8192 {
		return false
	}
	for i := range len(c.access) {
		if c.access[i] < 0x21 || c.access[i] > 0x7e {
			return false
		}
	}
	return true
}
func encodeCredential(c Credential) *pb.Credential {
	value := &pb.Credential{CsrfToken: c.csrf}
	if c.legacy != "" {
		value.Proof = &pb.Credential_LegacyToken{LegacyToken: c.legacy}
	} else {
		value.Proof = &pb.Credential_AccessToken{AccessToken: c.access}
	}
	return value
}
func decodeCredential(c *pb.Credential) (Credential, error) {
	if c == nil {
		return Credential{}, writefence.ErrInvalid
	}
	var value Credential
	switch p := c.GetProof().(type) {
	case *pb.Credential_LegacyToken:
		value = LegacyCredential(p.LegacyToken, c.CsrfToken)
	case *pb.Credential_AccessToken:
		value = AccessCredential(p.AccessToken, c.CsrfToken)
	default:
		return value, writefence.ErrInvalid
	}
	if !validCredential(value) {
		return Credential{}, writefence.ErrInvalid
	}
	return value, nil
}
func encodeIdentity(value Identity) *pb.Identity {
	return &pb.Identity{AccountId: append([]byte(nil), value.AccountID[:]...), SessionId: append([]byte(nil), value.SessionID[:]...), CredentialEpoch: value.CredentialEpoch}
}
func decodeIdentity(value *pb.Identity) (Identity, error) {
	if value == nil {
		return Identity{}, writefence.ErrUnavailable
	}
	account, e1 := uuid.FromBytes(value.AccountId)
	session, e2 := uuid.FromBytes(value.SessionId)
	if e1 != nil || e2 != nil || account == uuid.Nil || session == uuid.Nil || value.CredentialEpoch <= 0 {
		return Identity{}, writefence.ErrUnavailable
	}
	return Identity{AccountID: account, SessionID: session, CredentialEpoch: value.CredentialEpoch}, nil
}
func encodeWriter(value writefence.Writer) coordination.Writer {
	switch value {
	case writefence.WriterIdentity:
		return coordination.Writer_WRITER_IDENTITY
	case writefence.WriterCreation:
		return coordination.Writer_WRITER_CREATION
	case writefence.WriterMedia:
		return coordination.Writer_WRITER_MEDIA
	case writefence.WriterExecution:
		return coordination.Writer_WRITER_EXECUTION
	default:
		return coordination.Writer_WRITER_UNSPECIFIED
	}
}
func decodeWriter(value coordination.Writer) (writefence.Writer, error) {
	for _, w := range writefence.Writers() {
		if encodeWriter(w) == value {
			return w, nil
		}
	}
	return "", writefence.ErrInvalid
}
func encodeCommand(c writefence.Command) *coordination.Command {
	value := &coordination.Command{Id: append([]byte(nil), c.ID[:]...), Sha256: c.SHA256, Action: c.Action}
	issuers := map[writefence.Issuer]coordination.Issuer{writefence.Identity: coordination.Issuer_ISSUER_IDENTITY, writefence.Creation: coordination.Issuer_ISSUER_CREATION}
	kinds := map[writefence.Kind]coordination.AuthorityKind{writefence.Account: coordination.AuthorityKind_AUTHORITY_KIND_ACCOUNT, writefence.Session: coordination.AuthorityKind_AUTHORITY_KIND_SESSION, writefence.Scope: coordination.AuthorityKind_AUTHORITY_KIND_SCOPE, writefence.Project: coordination.AuthorityKind_AUTHORITY_KIND_PROJECT, writefence.AgentGrant: coordination.AuthorityKind_AUTHORITY_KIND_AGENT_GRANT}
	for _, a := range c.Authorities {
		value.Authorities = append(value.Authorities, &coordination.Authority{Issuer: issuers[a.Issuer], Kind: kinds[a.Kind], Id: append([]byte(nil), a.ID[:]...), Epoch: a.Epoch})
	}
	return value
}
func decodeCommand(value *coordination.Command) (writefence.Command, error) {
	if value == nil || len(value.Authorities) > writefence.MaxAuthorities {
		return writefence.Command{}, writefence.ErrInvalid
	}
	id, err := uuid.FromBytes(value.Id)
	if err != nil {
		return writefence.Command{}, writefence.ErrInvalid
	}
	command := writefence.Command{ID: id, SHA256: value.Sha256, Action: value.Action}
	issuers := map[coordination.Issuer]writefence.Issuer{coordination.Issuer_ISSUER_IDENTITY: writefence.Identity, coordination.Issuer_ISSUER_CREATION: writefence.Creation}
	kinds := map[coordination.AuthorityKind]writefence.Kind{coordination.AuthorityKind_AUTHORITY_KIND_ACCOUNT: writefence.Account, coordination.AuthorityKind_AUTHORITY_KIND_SESSION: writefence.Session, coordination.AuthorityKind_AUTHORITY_KIND_SCOPE: writefence.Scope, coordination.AuthorityKind_AUTHORITY_KIND_PROJECT: writefence.Project, coordination.AuthorityKind_AUTHORITY_KIND_AGENT_GRANT: writefence.AgentGrant}
	for _, a := range value.Authorities {
		if a == nil {
			return writefence.Command{}, writefence.ErrInvalid
		}
		id, err := uuid.FromBytes(a.Id)
		if err != nil {
			return writefence.Command{}, writefence.ErrInvalid
		}
		command.Authorities = append(command.Authorities, writefence.Authority{Issuer: issuers[a.Issuer], Kind: kinds[a.Kind], ID: id, Epoch: a.Epoch})
	}
	_, err = command.AuthorityDigest()
	return command, err
}
func sameCommand(a, b writefence.Command) bool {
	ad, e1 := a.AuthorityDigest()
	bd, e2 := b.AuthorityDigest()
	return e1 == nil && e2 == nil && a.ID == b.ID && a.SHA256 == b.SHA256 && a.Action == b.Action && ad == bd
}
func bindsIdentity(c writefence.Command, id Identity) bool {
	var account, session bool
	for _, a := range c.Authorities {
		if a.Issuer != writefence.Identity {
			continue
		}
		switch {
		case a.Kind == writefence.Account && a.ID == id.AccountID && !account:
			account = true
		case a.Kind == writefence.Session && a.ID == id.SessionID && !session:
			session = true
		default:
			return false
		}
	}
	return account && session
}

// closedMessage rejects unknown fields recursively, including imported command
// messages. Unknown actor/Principal extensions cannot become ignored authority.
func closedMessage(message proto.Message) bool {
	if message == nil {
		return false
	}
	return closedReflect(message.ProtoReflect())
}
func closedReflect(message protoreflect.Message) bool {
	if !message.IsValid() || len(message.GetUnknown()) != 0 {
		return false
	}
	closed := true
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.Kind() != protoreflect.MessageKind {
			return true
		}
		if field.IsList() {
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				if !closedReflect(list.Get(i).Message()) {
					closed = false
					break
				}
			}
		} else {
			closed = closedReflect(value.Message())
		}
		return closed
	})
	return closed
}

var rejectionReasons = []struct {
	err    error
	code   codes.Code
	reason coordination.ErrorReason
}{
	{identity.ErrSessionCSRFInvalid, codes.PermissionDenied, coordination.ErrorReason_ERROR_REASON_CSRF_INVALID},
	{identity.ErrAccessExpired, codes.Unauthenticated, coordination.ErrorReason_ERROR_REASON_ACCESS_EXPIRED},
	{writefence.ErrInvalid, codes.InvalidArgument, coordination.ErrorReason_ERROR_REASON_INVALID},
	{writefence.ErrFenced, codes.FailedPrecondition, coordination.ErrorReason_ERROR_REASON_FENCED},
	{writefence.ErrConflict, codes.AlreadyExists, coordination.ErrorReason_ERROR_REASON_CONFLICT},
	{writefence.ErrStale, codes.FailedPrecondition, coordination.ErrorReason_ERROR_REASON_STALE},
	{writefence.ErrFrozen, codes.FailedPrecondition, coordination.ErrorReason_ERROR_REASON_FROZEN},
	{writefence.ErrCommitted, codes.FailedPrecondition, coordination.ErrorReason_ERROR_REASON_COMMITTED},
	{writefence.ErrNotFound, codes.NotFound, coordination.ErrorReason_ERROR_REASON_NOT_FOUND},
}

func rpcError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.FromContextError(err).Err()
	}
	for _, r := range rejectionReasons {
		if errors.Is(err, r.err) {
			s, e := status.New(r.code, "identity admission rejected").WithDetails(&coordination.Failure{Reason: r.reason})
			if e == nil {
				return s.Err()
			}
		}
	}
	if errors.Is(err, identity.ErrSessionInvalid) || errors.Is(err, identity.ErrAccessInvalid) {
		return status.Error(codes.Unauthenticated, "identity proof rejected")
	}
	if errors.Is(err, identity.ErrForbidden) {
		return status.Error(codes.PermissionDenied, "identity admission denied")
	}
	return status.Error(codes.Unavailable, "identity authority unavailable")
}
func localError(err error) error {
	s, ok := status.FromError(err)
	if !ok {
		return writefence.ErrUnavailable
	}
	switch s.Code() {
	case codes.Canceled:
		return context.Canceled
	case codes.DeadlineExceeded:
		return context.DeadlineExceeded
	}
	for _, d := range s.Details() {
		f, ok := d.(*coordination.Failure)
		if !ok || !closedMessage(f) {
			continue
		}
		for _, r := range rejectionReasons {
			if r.code == s.Code() && r.reason == f.Reason {
				return r.err
			}
		}
	}
	if len(s.Details()) != 0 {
		return writefence.ErrUnavailable
	}
	// Older peers carry only status. Unknown typed details remain unavailable;
	// they must not be mislabeled as a confirmed credential or policy rejection.
	switch s.Code() {
	case codes.Unauthenticated:
		return identity.ErrSessionInvalid
	case codes.PermissionDenied:
		return identity.ErrForbidden
	}
	return writefence.ErrUnavailable
}

// Package identitysource consumes Identity's generated private authority contract.
// It does not import Identity business types, roles, stores or session machinery.
package identitysource

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

const maxMessageBytes = 32 << 10

// Client has the fixed Creation caller and target. It cannot admit commands on
// behalf of Execution or Media, nor use their dedicated authority tokens.
type Client struct {
	address, token string
	transport      credentials.TransportCredentials
}

// New requires an explicit private endpoint, dedicated Creation token and TLS
// policy. It does not read shared configuration or select an implicit fallback.
func New(address, token string, transport credentials.TransportCredentials) (*Client, error) {
	host, port, err := net.SplitHostPort(address)
	number, portErr := strconv.Atoi(port)
	if err != nil || host == "" || strings.ContainsAny(host, "/@?#% ") || strings.IndexFunc(host, unicode.IsSpace) >= 0 || portErr != nil || number < 1 || number > 65535 || !serviceauth.ValidToken(token) || transport == nil {
		return nil, writefence.ErrInvalid
	}
	return &Client{address: address, token: token, transport: transport.Clone()}, nil
}

var _ workspacesource.IdentityAuthority = (*Client)(nil)

func (c *Client) connect(ctx context.Context) (context.Context, context.CancelFunc, *grpc.ClientConn, error) {
	if c == nil {
		return nil, nil, nil, writefence.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(bounded, carrier)
	values := []string{"authorization", "Bearer " + c.token}
	for _, key := range []string{"traceparent", "tracestate"} {
		if value := carrier[key]; value != "" {
			values = append(values, key, value)
		}
	}
	bounded = metadata.NewOutgoingContext(bounded, metadata.Pairs(values...))
	connection, err := grpc.NewClient(c.address, grpc.WithTransportCredentials(c.transport.Clone()), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMessageBytes), grpc.MaxCallSendMsgSize(maxMessageBytes), grpc.MaxRetryRPCBufferSize(0)))
	if err != nil {
		cancel()
		return nil, nil, nil, writefence.ErrUnavailable
	}
	return bounded, cancel, connection, nil
}

// VerifySourceIdentity only reads current account/session identity. It never
// substitutes for the separate persisted command admission method below.
func (c *Client) VerifySourceIdentity(ctx context.Context, credential workspacesource.Credential) (workspacesource.Identity, error) {
	proof, err := encodeCredential(credential, false)
	if err != nil {
		return workspacesource.Identity{}, err
	}
	ctx, cancel, connection, err := c.connect(ctx)
	if err != nil {
		return workspacesource.Identity{}, err
	}
	defer cancel()
	defer func() { _ = connection.Close() }()
	response, err := pb.NewAuthorityClient(connection).Verify(ctx, &pb.VerifyRequest{Caller: coordination.Writer_WRITER_CREATION, Credential: proof})
	if err != nil {
		return workspacesource.Identity{}, localError(err)
	}
	if !closed(response) {
		return workspacesource.Identity{}, writefence.ErrUnavailable
	}
	return decodeIdentity(response.Identity)
}

// AdmitSourceSession submits original credentials and exact command once. A
// timeout remains unknown to its caller; the adapter never retries a write.
func (c *Client) AdmitSourceSession(ctx context.Context, credential workspacesource.Credential, command writefence.Command, target writefence.Writer) (workspacesource.Identity, error) {
	if target != writefence.WriterCreation {
		return workspacesource.Identity{}, writefence.ErrInvalid
	}
	if _, err := command.AuthorityDigest(); err != nil {
		return workspacesource.Identity{}, err
	}
	proof, err := encodeCredential(credential, true)
	if err != nil {
		return workspacesource.Identity{}, err
	}
	encoded := encodeCommand(command)
	ctx, cancel, connection, err := c.connect(ctx)
	if err != nil {
		return workspacesource.Identity{}, err
	}
	defer cancel()
	defer func() { _ = connection.Close() }()
	response, err := pb.NewAuthorityClient(connection).AdmitSessionCommand(ctx, &pb.AdmitSessionCommandRequest{Caller: coordination.Writer_WRITER_CREATION, Target: coordination.Writer_WRITER_CREATION, Credential: proof, Command: encoded})
	if err != nil {
		return workspacesource.Identity{}, localError(err)
	}
	if !closed(response) || response.Target != coordination.Writer_WRITER_CREATION || !proto.Equal(encoded, response.Command) {
		return workspacesource.Identity{}, writefence.ErrConflict
	}
	identity, err := decodeIdentity(response.Identity)
	if err != nil {
		return workspacesource.Identity{}, err
	}
	if !identityMatchesCommand(identity, command) {
		return workspacesource.Identity{}, writefence.ErrConflict
	}
	return identity, nil
}

func encodeCredential(credential workspacesource.Credential, write bool) (*pb.Credential, error) {
	access, legacy, csrf := credential.IdentityProof()
	if (write && !validProof(csrf)) || (csrf != "" && !validProof(csrf)) {
		return nil, writefence.ErrInvalid
	}
	value := &pb.Credential{CsrfToken: csrf}
	if legacy != "" {
		if access != "" || !validProof(legacy) {
			return nil, writefence.ErrInvalid
		}
		value.Proof = &pb.Credential_LegacyToken{LegacyToken: legacy}
	} else {
		if access == "" || len(access) > 8192 {
			return nil, writefence.ErrInvalid
		}
		for i := range len(access) {
			if access[i] < 0x21 || access[i] > 0x7e {
				return nil, writefence.ErrInvalid
			}
		}
		value.Proof = &pb.Credential_AccessToken{AccessToken: access}
	}
	return value, nil
}
func validProof(value string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(raw) == 32 && base64.RawURLEncoding.EncodeToString(raw) == value
}
func decodeIdentity(value *pb.Identity) (workspacesource.Identity, error) {
	if value == nil {
		return workspacesource.Identity{}, writefence.ErrUnavailable
	}
	account, ae := uuid.FromBytes(value.AccountId)
	session, se := uuid.FromBytes(value.SessionId)
	if ae != nil || se != nil || account == uuid.Nil || session == uuid.Nil || value.CredentialEpoch < 1 {
		return workspacesource.Identity{}, writefence.ErrUnavailable
	}
	return workspacesource.Identity{AccountID: account, SessionID: session, CredentialEpoch: value.CredentialEpoch}, nil
}
func encodeCommand(command writefence.Command) *coordination.Command {
	value := &coordination.Command{Id: append([]byte(nil), command.ID[:]...), Sha256: command.SHA256, Action: command.Action}
	issuers := map[writefence.Issuer]coordination.Issuer{writefence.Identity: coordination.Issuer_ISSUER_IDENTITY, writefence.Creation: coordination.Issuer_ISSUER_CREATION}
	kinds := map[writefence.Kind]coordination.AuthorityKind{writefence.Account: coordination.AuthorityKind_AUTHORITY_KIND_ACCOUNT, writefence.Session: coordination.AuthorityKind_AUTHORITY_KIND_SESSION, writefence.Scope: coordination.AuthorityKind_AUTHORITY_KIND_SCOPE, writefence.Project: coordination.AuthorityKind_AUTHORITY_KIND_PROJECT, writefence.AgentGrant: coordination.AuthorityKind_AUTHORITY_KIND_AGENT_GRANT}
	for _, a := range command.Authorities {
		value.Authorities = append(value.Authorities, &coordination.Authority{Issuer: issuers[a.Issuer], Kind: kinds[a.Kind], Id: append([]byte(nil), a.ID[:]...), Epoch: a.Epoch})
	}
	return value
}
func closed(value proto.Message) bool {
	if value == nil {
		return false
	}
	message := value.ProtoReflect()
	if !message.IsValid() || len(message.GetUnknown()) > 0 {
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
				if !closed(list.Get(i).Message().Interface()) {
					valid = false
					break
				}
			}
		} else {
			valid = closed(value.Message().Interface())
		}
		return valid
	})
	return valid
}
func localError(err error) error {
	s := status.Convert(err)
	switch s.Code() {
	case codes.Canceled:
		return context.Canceled
	case codes.DeadlineExceeded:
		return context.DeadlineExceeded
	}
	if len(s.Proto().Details) > 0 && len(s.Proto().Details) != 1 {
		return writefence.ErrUnavailable
	}
	for _, detail := range s.Details() {
		failure, ok := detail.(*coordination.Failure)
		if !ok || !closed(failure) {
			return writefence.ErrUnavailable
		}
		switch failure.Reason {
		case coordination.ErrorReason_ERROR_REASON_CSRF_INVALID:
			if s.Code() == codes.PermissionDenied {
				return errors.Join(workspacesource.ErrSourceCSRFInvalid, workspacesource.ErrScopeNotFound)
			}
		case coordination.ErrorReason_ERROR_REASON_ACCESS_EXPIRED:
			if s.Code() == codes.Unauthenticated {
				return errors.Join(workspacesource.ErrSourceAccessExpired, workspacesource.ErrScopeNotFound)
			}
		case coordination.ErrorReason_ERROR_REASON_INVALID:
			if s.Code() == codes.InvalidArgument {
				return writefence.ErrInvalid
			}
		case coordination.ErrorReason_ERROR_REASON_FENCED:
			if s.Code() == codes.FailedPrecondition {
				return writefence.ErrFenced
			}
		case coordination.ErrorReason_ERROR_REASON_STALE:
			if s.Code() == codes.FailedPrecondition {
				return writefence.ErrStale
			}
		case coordination.ErrorReason_ERROR_REASON_CONFLICT:
			if s.Code() == codes.AlreadyExists {
				return writefence.ErrConflict
			}
		case coordination.ErrorReason_ERROR_REASON_FROZEN:
			if s.Code() == codes.FailedPrecondition {
				return writefence.ErrFrozen
			}
		case coordination.ErrorReason_ERROR_REASON_COMMITTED:
			if s.Code() == codes.FailedPrecondition {
				return writefence.ErrCommitted
			}
		}
	}
	if len(s.Proto().Details) == 0 {
		switch s.Code() {
		case codes.Unauthenticated:
			return errors.Join(workspacesource.ErrSourceSessionInvalid, workspacesource.ErrScopeNotFound)
		case codes.PermissionDenied:
			return errors.Join(workspacesource.ErrSourceForbidden, workspacesource.ErrScopeNotFound)
		}
	}
	return fmt.Errorf("creation identity authority: %w", writefence.ErrUnavailable)
}

// Package executionwork consumes only Execution's generated read-only project
// responsibility protocol. It carries no Execution application or store type.
package executionwork

import (
	"context"
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

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/executionwork"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

const maxMessageBytes = 512 << 10

// Client has one fixed authenticated Execution endpoint, with no retry/fallback.
type Client struct {
	address, token string
	transport      credentials.TransportCredentials
}

var _ source.ProjectWorkReader = (*Client)(nil)

// New requires explicit TLS and the dedicated Creation caller credential.
func New(address, token string, transport credentials.TransportCredentials) (*Client, error) {
	host, port, err := net.SplitHostPort(address)
	number, parseErr := strconv.Atoi(port)
	if err != nil || host == "" || strings.ContainsAny(host, "/@?#% ") || strings.IndexFunc(host, unicode.IsSpace) >= 0 || parseErr != nil || number < 1 || number > 65535 || !serviceauth.ValidToken(token) || transport == nil || transport.Info().SecurityProtocol != "tls" {
		return nil, writefence.ErrInvalid
	}
	return &Client{address: address, token: token, transport: transport.Clone()}, nil
}

// ReadProjectWork makes one bounded read and validates the complete original echo.
// A missing record/endpoint is unresolved responsibility, never an idle result.
func (c *Client) ReadProjectWork(ctx context.Context, key source.ProjectWorkRequest) (source.ProjectWorkSnapshot, error) {
	if c == nil {
		return source.ProjectWorkSnapshot{}, writefence.ErrUnavailable
	}
	if err := key.Validate(); err != nil {
		return source.ProjectWorkSnapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return source.ProjectWorkSnapshot{}, err
	}
	request := &pb.ProjectWorkRequest{CloseId: key.CloseID[:], ProjectId: key.ProjectID[:], OrganizationId: key.OrganizationID[:], ScopeId: key.ScopeID[:], ProjectEpoch: key.ProjectEpoch, ManifestSha256: key.ManifestSHA256}
	for _, command := range key.Commands {
		request.Commands = append(request.Commands, encodeCommand(command))
	}
	if proto.Size(request) > maxMessageBytes {
		return source.ProjectWorkSnapshot{}, writefence.ErrInvalid
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(bounded, carrier)
	values := []string{"authorization", "Bearer " + c.token}
	for _, name := range []string{"traceparent", "tracestate"} {
		if value := carrier[name]; value != "" {
			values = append(values, name, value)
		}
	}
	bounded = metadata.NewOutgoingContext(bounded, metadata.Pairs(values...))
	connection, err := grpc.NewClient(c.address, grpc.WithTransportCredentials(c.transport.Clone()), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMessageBytes), grpc.MaxCallSendMsgSize(maxMessageBytes), grpc.MaxRetryRPCBufferSize(0)))
	if err != nil {
		return source.ProjectWorkSnapshot{}, writefence.ErrUnavailable
	}
	defer func() { _ = connection.Close() }()
	response, err := pb.NewProjectWorkClient(connection).ReadProjectWork(bounded, request)
	if err != nil {
		switch status.Code(err) {
		case codes.Canceled:
			return source.ProjectWorkSnapshot{}, context.Canceled
		case codes.DeadlineExceeded:
			return source.ProjectWorkSnapshot{}, context.DeadlineExceeded
		default:
			return source.ProjectWorkSnapshot{}, writefence.ErrUnavailable
		}
	}
	if !closed(response) || !proto.Equal(request, response.Request) || len(response.Commands) != len(key.Commands) {
		return source.ProjectWorkSnapshot{}, writefence.ErrConflict
	}
	result := source.ProjectWorkSnapshot{Request: key}
	switch response.BusinessState {
	case pb.BusinessState_IDLE:
		result.BusinessState = source.ProjectWorkIdle
	case pb.BusinessState_BUSY:
		result.BusinessState = source.ProjectWorkBusy
	case pb.BusinessState_UNKNOWN:
		result.BusinessState = source.ProjectWorkUnknown
	default:
		return result, writefence.ErrUnavailable
	}
	for _, observation := range response.Commands {
		command, err := decodeCommand(observation.Command)
		if err != nil {
			return result, err
		}
		entry := source.ProjectCommandObservation{Command: command, ReceiptSHA256: observation.ReceiptSha256}
		switch observation.State {
		case pb.CommandState_COMMITTED:
			entry.State = writefence.Committed
		case pb.CommandState_FROZEN:
			entry.State = writefence.Frozen
		case pb.CommandState_ADMITTED:
			entry.State = writefence.Admitted
		case pb.CommandState_COMMAND_UNKNOWN:
			entry.State = writefence.State("unknown")
		default:
			return result, writefence.ErrUnavailable
		}
		result.Commands = append(result.Commands, entry)
	}
	if _, err := result.Inspect(key); err != nil {
		return source.ProjectWorkSnapshot{}, err
	}
	return result, nil
}
func encodeCommand(command writefence.Command) *coordination.Command {
	result := &coordination.Command{Id: command.ID[:], Sha256: command.SHA256, Action: command.Action}
	issuers := map[writefence.Issuer]coordination.Issuer{writefence.Identity: coordination.Issuer_ISSUER_IDENTITY, writefence.Creation: coordination.Issuer_ISSUER_CREATION}
	kinds := map[writefence.Kind]coordination.AuthorityKind{writefence.Account: coordination.AuthorityKind_AUTHORITY_KIND_ACCOUNT, writefence.Session: coordination.AuthorityKind_AUTHORITY_KIND_SESSION, writefence.Scope: coordination.AuthorityKind_AUTHORITY_KIND_SCOPE, writefence.Project: coordination.AuthorityKind_AUTHORITY_KIND_PROJECT, writefence.AgentGrant: coordination.AuthorityKind_AUTHORITY_KIND_AGENT_GRANT}
	for _, a := range command.Authorities {
		result.Authorities = append(result.Authorities, &coordination.Authority{Issuer: issuers[a.Issuer], Kind: kinds[a.Kind], Id: a.ID[:], Epoch: a.Epoch})
	}
	return result
}
func decodeCommand(value *coordination.Command) (writefence.Command, error) {
	if value == nil {
		return writefence.Command{}, writefence.ErrConflict
	}
	id, err := uuid.FromBytes(value.Id)
	if err != nil {
		return writefence.Command{}, writefence.ErrConflict
	}
	command := writefence.Command{ID: id, SHA256: value.Sha256, Action: value.Action}
	issuers := map[coordination.Issuer]writefence.Issuer{coordination.Issuer_ISSUER_IDENTITY: writefence.Identity, coordination.Issuer_ISSUER_CREATION: writefence.Creation}
	kinds := map[coordination.AuthorityKind]writefence.Kind{coordination.AuthorityKind_AUTHORITY_KIND_ACCOUNT: writefence.Account, coordination.AuthorityKind_AUTHORITY_KIND_SESSION: writefence.Session, coordination.AuthorityKind_AUTHORITY_KIND_SCOPE: writefence.Scope, coordination.AuthorityKind_AUTHORITY_KIND_PROJECT: writefence.Project, coordination.AuthorityKind_AUTHORITY_KIND_AGENT_GRANT: writefence.AgentGrant}
	for _, a := range value.Authorities {
		if a == nil {
			return command, writefence.ErrConflict
		}
		id, err := uuid.FromBytes(a.Id)
		if err != nil {
			return command, writefence.ErrConflict
		}
		command.Authorities = append(command.Authorities, writefence.Authority{Issuer: issuers[a.Issuer], Kind: kinds[a.Kind], ID: id, Epoch: a.Epoch})
	}
	_, err = command.AuthorityDigest()
	return command, err
}
func closed(value proto.Message) bool {
	if value == nil {
		return false
	}
	message := value.ProtoReflect()
	if !message.IsValid() || len(message.GetUnknown()) != 0 {
		return false
	}
	valid := true
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.Kind() != protoreflect.MessageKind {
			return true
		}
		if field.IsList() {
			list := value.List()
			for i := range list.Len() {
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

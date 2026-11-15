// Package executionbudget consumes only Execution's generated zero-credit
// project preparation contract. It owns no wallet, model or pricing business.
package executionbudget

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

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/execution"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// Client has one dedicated target and never selects another owner or fallback.
type Client struct {
	address, token string
	transport      credentials.TransportCredentials
}

var _ workspacesource.ProjectBudgetParticipant = (*Client)(nil)

// New requires explicit mTLS transport and the Creation-specific participant token.
func New(address, token string, transport credentials.TransportCredentials) (*Client, error) {
	host, port, err := net.SplitHostPort(address)
	number, portErr := strconv.Atoi(port)
	if err != nil || host == "" || strings.ContainsAny(host, "/@?#% ") || strings.IndexFunc(host, unicode.IsSpace) >= 0 || portErr != nil || number < 1 || number > 65535 || !serviceauth.ValidToken(token) || transport == nil || transport.Info().SecurityProtocol != "tls" {
		return nil, workspacesource.ErrProjectPreparationConflict
	}
	return &Client{address: address, token: token, transport: transport.Clone()}, nil
}

// PrepareProjectBudget sends exactly once. An unknown ACK is recovered later by
// ResolveProjectBudget with the original immutable request, never a new ID.
func (c *Client) PrepareProjectBudget(ctx context.Context, key workspacesource.ProjectBudgetPreparation) (workspacesource.PreparedProjectBudget, error) {
	return c.call(ctx, key, true)
}

// ResolveProjectBudget reads the participant's original durable ACK only.
func (c *Client) ResolveProjectBudget(ctx context.Context, key workspacesource.ProjectBudgetPreparation) (workspacesource.PreparedProjectBudget, error) {
	return c.call(ctx, key, false)
}

func encode(key workspacesource.ProjectBudgetPreparation) *pb.ProjectBudgetPreparation {
	return &pb.ProjectBudgetPreparation{ExecutionCommandId: key.ExecutionCommandID[:], PreparationId: key.PreparationID[:], SourceCommandId: key.SourceCommandID[:], SourceSha256: key.SourceSHA256, InputSha256: key.InputSHA256, ProjectId: key.ProjectID[:], OrganizationId: key.OrganizationID[:], ScopeId: key.ScopeID[:]}
}

func (c *Client) call(ctx context.Context, key workspacesource.ProjectBudgetPreparation, prepare bool) (workspacesource.PreparedProjectBudget, error) {
	if c == nil {
		return workspacesource.PreparedProjectBudget{}, writefence.ErrUnavailable
	}
	if key.Validate() != nil {
		return workspacesource.PreparedProjectBudget{}, workspacesource.ErrProjectPreparationConflict
	}
	if err := ctx.Err(); err != nil {
		return workspacesource.PreparedProjectBudget{}, err
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
	connection, err := grpc.NewClient(c.address, grpc.WithTransportCredentials(c.transport.Clone()), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(4096), grpc.MaxCallSendMsgSize(4096), grpc.MaxRetryRPCBufferSize(0)))
	if err != nil {
		return workspacesource.PreparedProjectBudget{}, writefence.ErrUnavailable
	}
	defer func() { _ = connection.Close() }()
	request := encode(key)
	client := pb.NewProjectBudgetsClient(connection)
	var response *pb.ProjectBudgetReceipt
	if prepare {
		response, err = client.PrepareProjectBudget(bounded, request)
	} else {
		response, err = client.ResolveProjectBudget(bounded, request)
	}
	if err != nil {
		return workspacesource.PreparedProjectBudget{}, localError(err)
	}
	if response == nil || len(response.ProtoReflect().GetUnknown()) != 0 || response.Original == nil || len(response.Original.ProtoReflect().GetUnknown()) != 0 || !proto.Equal(request, response.Original) {
		return workspacesource.PreparedProjectBudget{}, workspacesource.ErrProjectPreparationConflict
	}
	id, err := uuid.FromBytes(response.BudgetId)
	if err != nil {
		return workspacesource.PreparedProjectBudget{}, workspacesource.ErrProjectPreparationConflict
	}
	receipt := workspacesource.PreparedProjectBudget{Request: key, BudgetID: id, BudgetRevision: response.BudgetRevision, LimitMicros: response.LimitMicros, SpentMicros: response.SpentMicros, ReservedMicros: response.ReservedMicros, WalletPrepared: response.WalletPrepared}
	return receipt, receipt.Validate()
}

func localError(err error) error {
	switch status.Code(err) {
	case codes.Canceled:
		return context.Canceled
	case codes.DeadlineExceeded:
		return context.DeadlineExceeded
	case codes.NotFound:
		return workspacesource.ErrProjectPreparationUnknown
	case codes.AlreadyExists, codes.InvalidArgument:
		return workspacesource.ErrProjectPreparationConflict
	default:
		return writefence.ErrUnavailable
	}
}

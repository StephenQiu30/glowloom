package grpctransport

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"

	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// Target is a composition-root supplied private writer, never a request URL.
// Tokens are dedicated to this caller issuer and target audience.
type Target struct {
	Address   string
	Token     string
	Transport credentials.TransportCredentials
}

// Client propagates one exact control without transparent application replay.
// Failed calls remain unknown and must be resumed under the original identity.
type Client struct {
	issuer  writefence.Issuer
	targets map[writefence.Writer]Target
}

// NewClient requires the complete closed writer set and distinct audience tokens.
// Production composition roots must supply verified mutual TLS credentials.
func NewClient(issuer writefence.Issuer, targets map[writefence.Writer]Target) (*Client, error) {
	if issuer != writefence.Identity && issuer != writefence.Creation || len(targets) != len(writefence.Writers()) {
		return nil, writefence.ErrInvalid
	}
	copied := make(map[writefence.Writer]Target, len(targets))
	seenTokens := make(map[string]bool, len(targets))
	for _, writer := range writefence.Writers() {
		target, ok := targets[writer]
		if !ok || !validAddress(target.Address) || !serviceauth.ValidToken(target.Token) || target.Transport == nil || seenTokens[target.Token] {
			return nil, writefence.ErrInvalid
		}
		seenTokens[target.Token] = true
		target.Transport = target.Transport.Clone()
		copied[writer] = target
	}
	return &Client{issuer: issuer, targets: copied}, nil
}

func (c *Client) connect(ctx context.Context, writer writefence.Writer) (context.Context, context.CancelFunc, *grpc.ClientConn, error) {
	if c == nil || writer.Validate() != nil {
		return nil, nil, nil, writefence.ErrInvalid
	}
	target, ok := c.targets[writer]
	if !ok {
		return nil, nil, nil, writefence.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	values := []string{"authorization", "Bearer " + target.Token}
	for _, key := range []string{"traceparent", "tracestate"} {
		if value := carrier[key]; value != "" {
			values = append(values, key, value)
		}
	}
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(values...))
	connection, err := grpc.NewClient(target.Address,
		grpc.WithTransportCredentials(target.Transport.Clone()), grpc.WithDisableRetry(),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(MaxMessageBytes), grpc.MaxCallSendMsgSize(MaxMessageBytes)),
	)
	if err != nil {
		cancel()
		return nil, nil, nil, writefence.ErrUnavailable
	}
	return ctx, cancel, connection, nil
}

// Install succeeds only after the final RPC status and exact barrier receipt.
func (c *Client) Install(ctx context.Context, writer writefence.Writer, control writefence.Control) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c == nil || control.Validate() != nil || control.Issuer != c.issuer {
		return writefence.ErrInvalid
	}
	ctx, cancel, connection, err := c.connect(ctx, writer)
	if err != nil {
		return err
	}
	defer cancel()
	defer func() { _ = connection.Close() }()
	response, err := pb.NewWriterFenceClient(connection).Install(ctx, &pb.InstallRequest{Writer: encodeWriter(writer), Control: encodeControl(control)})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return localError(err)
	}
	if response == nil || response.GetWriter() != encodeWriter(writer) {
		return writefence.ErrUnavailable
	}
	acknowledged, err := decodeControl(response.GetControl())
	if err != nil || acknowledged != control {
		return writefence.ErrUnavailable
	}
	return nil
}

// Resolve verifies the entire original identity and returns no new authorization.
func (c *Client) Resolve(ctx context.Context, writer writefence.Writer, command writefence.Command) (writefence.Record, error) {
	if err := ctx.Err(); err != nil {
		return writefence.Record{}, err
	}
	if c == nil {
		return writefence.Record{}, writefence.ErrInvalid
	}
	if _, err := command.AuthorityDigest(); err != nil {
		return writefence.Record{}, err
	}
	owned := false
	for _, authority := range command.Authorities {
		owned = owned || authority.Issuer == c.issuer
	}
	if !owned {
		return writefence.Record{}, writefence.ErrInvalid
	}
	ctx, cancel, connection, err := c.connect(ctx, writer)
	if err != nil {
		return writefence.Record{}, err
	}
	defer cancel()
	defer func() { _ = connection.Close() }()
	response, err := pb.NewWriterFenceClient(connection).Resolve(ctx, &pb.ResolveRequest{Writer: encodeWriter(writer), Command: encodeCommand(command)})
	if err != nil {
		if ctx.Err() != nil {
			return writefence.Record{}, ctx.Err()
		}
		return writefence.Record{}, localError(err)
	}
	if response == nil {
		return writefence.Record{}, writefence.ErrUnavailable
	}
	original, err := decodeCommand(response.GetCommand())
	if err != nil || !sameCommand(original, command) {
		return writefence.Record{}, writefence.ErrUnavailable
	}
	record := writefence.Record{Command: original}
	switch response.GetState() {
	case pb.CommandState_COMMAND_STATE_ADMITTED:
		record.State = writefence.Admitted
	case pb.CommandState_COMMAND_STATE_FROZEN:
		record.State = writefence.Frozen
	case pb.CommandState_COMMAND_STATE_COMMITTED:
		record.State = writefence.Committed
		record.Receipt, err = writefence.CanonicalReceipt(response.GetReceiptJson())
		if err != nil {
			return writefence.Record{}, writefence.ErrUnavailable
		}
	default:
		return writefence.Record{}, writefence.ErrUnavailable
	}
	if record.State != writefence.Committed && len(response.GetReceiptJson()) != 0 {
		return writefence.Record{}, writefence.ErrUnavailable
	}
	return record, nil
}

func validAddress(address string) bool {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || strings.ContainsAny(host, "/@?#% ") || strings.IndexFunc(host, unicode.IsSpace) >= 0 {
		return false
	}
	number, err := strconv.Atoi(port)
	return err == nil && number > 0 && number <= 65535
}

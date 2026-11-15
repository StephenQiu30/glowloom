// Package grpcregistration maps Identity's registration ports to private owner protocols.
package grpcregistration

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"github.com/StephenQiu30/glowloom-server/backend/identity/application"
	creationpb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creation"
	executionpb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/execution"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
)

// Target belongs to a composition root, never to a public request. TLS validates
// the owner audience and presents Identity's dedicated service certificate.
type Target struct {
	Address, Token string
	Transport      credentials.TransportCredentials
}

// Client delivers one exact registration command to its fixed owner audience.
type Client struct {
	target Target
	owner  string
}

func newClient(target Target, owner string) (*Client, error) {
	host, port, err := net.SplitHostPort(target.Address)
	n, parseErr := strconv.Atoi(port)
	if err != nil || parseErr != nil || n < 1 || n > 65535 || host == "" || strings.ContainsAny(host, "/@?#% \t\r\n") || !serviceauth.ValidToken(target.Token) || target.Transport == nil {
		return nil, application.ErrRegistrationUnavailable
	}
	target.Transport = target.Transport.Clone()
	return &Client{target: target, owner: owner}, nil
}

// NewPersonalScopeClient fixes the Creation preparation audience.
func NewPersonalScopeClient(target Target) (*Client, error) { return newClient(target, "creation") }

// NewWalletClient fixes the Execution wallet preparation audience.
func NewWalletClient(target Target) (*Client, error) { return newClient(target, "execution") }
func (c *Client) connect(ctx context.Context) (context.Context, context.CancelFunc, *grpc.ClientConn, error) {
	if c == nil {
		return nil, nil, nil, application.ErrRegistrationUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	md := metadata.Pairs("authorization", "Bearer "+c.target.Token)
	for _, key := range []string{"traceparent", "tracestate"} {
		if v := carrier[key]; v != "" {
			md.Set(key, v)
		}
	}
	ctx = metadata.NewOutgoingContext(ctx, md)
	conn, err := grpc.NewClient(c.target.Address, grpc.WithTransportCredentials(c.target.Transport.Clone()), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(4096), grpc.MaxCallSendMsgSize(4096), grpc.MaxRetryRPCBufferSize(0)))
	if err != nil {
		cancel()
		return nil, nil, nil, application.ErrRegistrationUnavailable
	}
	return ctx, cancel, conn, nil
}

// PrepareRegistration sends one original idempotent command. On an unknown reply,
// the caller retains the durable source operation; the adapter never retries it.
func (c *Client) PrepareRegistration(ctx context.Context, p application.RegistrationPreparation) (application.RegistrationReady, error) {
	return c.call(ctx, p, false)
}

// ResolveRegistration queries only the original participant command.
func (c *Client) ResolveRegistration(ctx context.Context, p application.RegistrationPreparation) (application.RegistrationReady, error) {
	return c.call(ctx, p, true)
}
func (c *Client) call(ctx context.Context, p application.RegistrationPreparation, resolve bool) (application.RegistrationReady, error) {
	ctx, cancel, conn, err := c.connect(ctx)
	if err != nil {
		return application.RegistrationReady{}, err
	}
	defer cancel()
	defer func() { _ = conn.Close() }()
	var operation, account, organization, scope []byte
	var digest string
	if c.owner == "creation" {
		q := &creationpb.PersonalRegistrationRequest{OperationId: p.OperationID[:], AccountId: p.AccountID[:], OrganizationId: p.OrganizationID[:], ScopeId: p.ScopeID[:], InputSha256: p.InputSHA256}
		var r *creationpb.PersonalRegistrationReceipt
		if resolve {
			r, err = creationpb.NewPersonalRegistrationClient(conn).Resolve(ctx, q)
		} else {
			r, err = creationpb.NewPersonalRegistrationClient(conn).Prepare(ctx, q)
		}
		if err != nil {
			return application.RegistrationReady{}, err
		}
		if r == nil || r.Command == nil || len(r.ProtoReflect().GetUnknown()) != 0 || len(r.Command.ProtoReflect().GetUnknown()) != 0 {
			return application.RegistrationReady{}, application.ErrRegistrationReceipt
		}
		operation, account, organization, scope, digest = r.Command.OperationId, r.Command.AccountId, r.Command.OrganizationId, r.Command.ScopeId, r.Command.InputSha256
	} else {
		if !p.CreationReady {
			return application.RegistrationReady{}, application.ErrRegistrationReceipt
		}
		q := &executionpb.RegistrationWalletRequest{OperationId: p.OperationID[:], AccountId: p.AccountID[:], OrganizationId: p.OrganizationID[:], ScopeId: p.ScopeID[:], InputSha256: p.InputSHA256, CreationReady: &creationpb.PersonalRegistrationReceipt{Command: &creationpb.PersonalRegistrationRequest{OperationId: p.OperationID[:], AccountId: p.AccountID[:], OrganizationId: p.OrganizationID[:], ScopeId: p.ScopeID[:], InputSha256: p.InputSHA256}}}
		var r *executionpb.RegistrationWalletReceipt
		if resolve {
			r, err = executionpb.NewRegistrationWalletClient(conn).Resolve(ctx, q)
		} else {
			r, err = executionpb.NewRegistrationWalletClient(conn).Prepare(ctx, q)
		}
		if err != nil {
			return application.RegistrationReady{}, err
		}
		if r == nil || r.Command == nil || len(r.ProtoReflect().GetUnknown()) != 0 || len(r.Command.ProtoReflect().GetUnknown()) != 0 {
			return application.RegistrationReady{}, application.ErrRegistrationReceipt
		}
		if !proto.Equal(q.CreationReady, r.Command.CreationReady) {
			return application.RegistrationReady{}, application.ErrRegistrationReceipt
		}
		operation, account, organization, scope, digest = r.Command.OperationId, r.Command.AccountId, r.Command.OrganizationId, r.Command.ScopeId, r.Command.InputSha256
	}
	r := application.RegistrationReady{Owner: c.owner, InputSHA256: digest}
	for _, v := range []struct {
		raw []byte
		id  *uuid.UUID
	}{{operation, &r.OperationID}, {account, &r.AccountID}, {organization, &r.OrganizationID}, {scope, &r.ScopeID}} {
		id, err := uuid.FromBytes(v.raw)
		if err != nil {
			return application.RegistrationReady{}, application.ErrRegistrationReceipt
		}
		*v.id = id
	}
	if r != p.ExpectedReady(c.owner) {
		return application.RegistrationReady{}, application.ErrRegistrationReceipt
	}
	return r, nil
}

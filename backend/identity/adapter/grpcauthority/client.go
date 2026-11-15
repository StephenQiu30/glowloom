package grpcauthority

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

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// Target is supplied by a composition root, never by a public request. Its token
// is dedicated to this authority audience and authenticated caller writer.
type Target struct {
	Address, Token string
	Transport      credentials.TransportCredentials
}

// Client sends a single bounded RPC. Unknown outcomes require explicit recovery
// of the original command; this adapter never retries, falls back or reopens heads.
type Client struct {
	caller writefence.Writer
	target Target
}

// NewClient requires an explicit private address, dedicated token and transport.
func NewClient(caller writefence.Writer, target Target) (*Client, error) {
	if caller.Validate() != nil || !validAddress(target.Address) || !serviceauth.ValidToken(target.Token) || target.Transport == nil {
		return nil, writefence.ErrInvalid
	}
	target.Transport = target.Transport.Clone()
	return &Client{caller: caller, target: target}, nil
}
func validAddress(address string) bool {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || strings.ContainsAny(host, "/@?#% ") || strings.IndexFunc(host, unicode.IsSpace) >= 0 {
		return false
	}
	number, err := strconv.Atoi(port)
	return err == nil && number > 0 && number <= 65535
}
func (c *Client) connect(ctx context.Context) (context.Context, context.CancelFunc, *grpc.ClientConn, error) {
	if c == nil {
		return nil, nil, nil, writefence.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	values := []string{"authorization", "Bearer " + c.target.Token}
	for _, key := range []string{"traceparent", "tracestate"} {
		if value := carrier[key]; value != "" {
			values = append(values, key, value)
		}
	}
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(values...))
	conn, err := grpc.NewClient(c.target.Address, grpc.WithTransportCredentials(c.target.Transport.Clone()), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(MaxMessageBytes), grpc.MaxCallSendMsgSize(MaxMessageBytes), grpc.MaxRetryRPCBufferSize(0)))
	if err != nil {
		cancel()
		return nil, nil, nil, writefence.ErrUnavailable
	}
	return ctx, cancel, conn, nil
}

// Verify proves current account and exact session identity with NoActivity. CSRF
// may be omitted for this read; a supplied proof is still checked. It neither
// registers a command nor grants a write permit or extends idle expiry.
func (c *Client) Verify(ctx context.Context, credential Credential) (Identity, error) {
	if !validCredential(credential) {
		return Identity{}, writefence.ErrInvalid
	}
	ctx, cancel, conn, err := c.connect(ctx)
	if err != nil {
		return Identity{}, err
	}
	defer cancel()
	defer func() { _ = conn.Close() }()
	response, err := pb.NewAuthorityClient(conn).Verify(ctx, &pb.VerifyRequest{Caller: encodeWriter(c.caller), Credential: encodeCredential(credential)})
	if err != nil {
		return Identity{}, localError(err)
	}
	if !closedMessage(response) {
		return Identity{}, writefence.ErrUnavailable
	}
	return decodeIdentity(response.Identity)
}

// ReadSessionAuthorities returns current complete/open account and session
// generations. This bounded single read has NoActivity and cannot admit work.
func (c *Client) ReadSessionAuthorities(ctx context.Context, credential Credential) (SessionAuthorities, error) {
	if !validCredential(credential) {
		return SessionAuthorities{}, writefence.ErrInvalid
	}
	ctx, cancel, conn, err := c.connect(ctx)
	if err != nil {
		return SessionAuthorities{}, err
	}
	defer cancel()
	defer func() { _ = conn.Close() }()
	response, err := pb.NewAuthorityClient(conn).ReadSessionAuthorities(ctx, &pb.ReadSessionAuthoritiesRequest{Caller: encodeWriter(c.caller), Credential: encodeCredential(credential)})
	if err != nil {
		return SessionAuthorities{}, localError(err)
	}
	if !closedMessage(response) {
		return SessionAuthorities{}, writefence.ErrUnavailable
	}
	id, err := decodeIdentity(response.Identity)
	if err != nil {
		return SessionAuthorities{}, err
	}
	account, err := decodeSessionAuthority(response.Account, writefence.Account)
	if err != nil {
		return SessionAuthorities{}, err
	}
	session, err := decodeSessionAuthority(response.Session, writefence.Session)
	if err != nil {
		return SessionAuthorities{}, err
	}
	value := SessionAuthorities{Identity: id, Account: account, Session: session}
	if !value.valid() {
		return SessionAuthorities{}, writefence.ErrUnavailable
	}
	return value, nil
}

// AdmitSessionCommand authenticates raw credentials again and registers the exact
// command for this caller writer under original policy locks. The effect owner
// must still authorize resources and check its local final transaction fence.
func (c *Client) AdmitSessionCommand(ctx context.Context, credential Credential, command writefence.Command, target writefence.Writer) (Identity, error) {
	if c == nil || target != c.caller || target.Validate() != nil || !validCredential(credential) || !validProof(credential.csrf) {
		return Identity{}, writefence.ErrInvalid
	}
	if _, err := command.AuthorityDigest(); err != nil {
		return Identity{}, err
	}
	ctx, cancel, conn, err := c.connect(ctx)
	if err != nil {
		return Identity{}, err
	}
	defer cancel()
	defer func() { _ = conn.Close() }()
	response, err := pb.NewAuthorityClient(conn).AdmitSessionCommand(ctx, &pb.AdmitSessionCommandRequest{Caller: encodeWriter(c.caller), Credential: encodeCredential(credential), Command: encodeCommand(command), Target: encodeWriter(target)})
	if err != nil {
		return Identity{}, localError(err)
	}
	if !closedMessage(response) || response.Target != encodeWriter(target) {
		return Identity{}, writefence.ErrUnavailable
	}
	original, err := decodeCommand(response.Command)
	if err != nil || !sameCommand(original, command) {
		return Identity{}, writefence.ErrUnavailable
	}
	id, err := decodeIdentity(response.Identity)
	if err != nil || !bindsIdentity(command, id) {
		return Identity{}, writefence.ErrUnavailable
	}
	return id, nil
}

// ResolveAdmission is available only to the Creation caller. It checks the entire
// original command and target without forwarding credentials or creating work.
// The returned proof cannot replace Creation policy or the final writer fence.
func (c *Client) ResolveAdmission(ctx context.Context, command writefence.Command, target writefence.Writer) (AdmissionProof, error) {
	if c == nil || c.caller != writefence.WriterCreation || target.Validate() != nil {
		return AdmissionProof{}, writefence.ErrInvalid
	}
	if _, err := command.AuthorityDigest(); err != nil {
		return AdmissionProof{}, err
	}
	ctx, cancel, conn, err := c.connect(ctx)
	if err != nil {
		return AdmissionProof{}, err
	}
	defer cancel()
	defer func() { _ = conn.Close() }()
	response, err := pb.NewAuthorityClient(conn).ResolveAdmission(ctx, &pb.ResolveAdmissionRequest{Caller: encodeWriter(c.caller), Command: encodeCommand(command), Target: encodeWriter(target)})
	if err != nil {
		return AdmissionProof{}, localError(err)
	}
	if !closedMessage(response) || response.Proof == nil || response.Proof.Target != encodeWriter(target) {
		return AdmissionProof{}, writefence.ErrUnavailable
	}
	original, err := decodeCommand(response.Proof.Command)
	if err != nil || !sameCommand(original, command) {
		return AdmissionProof{}, writefence.ErrUnavailable
	}
	id, err := decodeIdentity(response.Proof.Identity)
	if err != nil || !bindsIdentity(original, id) {
		return AdmissionProof{}, writefence.ErrUnavailable
	}
	return AdmissionProof{Identity: id, Command: original, Target: target}, nil
}

// Package artifactrpc bounds authenticated transport for opaque original byte facts.
// Owner applications supply all admission, storage, recovery and money decisions.
package artifactrpc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediaartifact"
	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/providerartifact"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

const messageBytes = contract.ChunkSize + 16384
const duration = 3 * time.Minute

// MediaService owns original bytes and exact permanent receipts.
type MediaService interface {
	StageArtifact(context.Context, contract.Request) (contract.Receipt, error)
	ResolveArtifact(context.Context, contract.Request) (contract.Receipt, error)
	OpenArtifact(context.Context, contract.Request) (io.ReadCloser, error)
}

// SourceService can only read a previously persisted attempted source.
type SourceService interface {
	VerifyArtifactSource(context.Context, contract.Request) error
	OpenArtifactSource(context.Context, contract.Request) (io.ReadCloser, error)
}

type gate struct {
	token string
	slots chan struct{}
}

func (g *gate) enter(ctx context.Context) (func(), error) {
	md, _ := metadata.FromIncomingContext(ctx)
	auth := md.Get("authorization")
	if len(auth) != 1 || subtle.ConstantTimeCompare([]byte(auth[0]), []byte("Bearer "+g.token)) != 1 {
		return nil, status.Error(codes.Unauthenticated, "artifact peer authentication required")
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > duration {
		return nil, status.Error(codes.InvalidArgument, "bounded artifact deadline required")
	}
	select {
	case g.slots <- struct{}{}:
		return func() { <-g.slots }, nil
	default:
		return nil, status.Error(codes.ResourceExhausted, "artifact capacity exhausted")
	}
}
func newListener(token string, concurrent int, transport credentials.TransportCredentials) (*grpc.Server, gate, error) {
	if !serviceauth.ValidToken(token) || concurrent < 1 || concurrent > 8 || transport == nil {
		return nil, gate{}, contract.ErrInvalid
	}
	return grpc.NewServer(grpc.Creds(transport.Clone()), grpc.MaxRecvMsgSize(messageBytes), grpc.MaxSendMsgSize(messageBytes), grpc.MaxHeaderListSize(16384)), gate{token: token, slots: make(chan struct{}, concurrent)}, nil
}

// NewMediaListener exposes only Media's Stage, original Resolve and bounded Read.
func NewMediaListener(service MediaService, token string, concurrent int, transport credentials.TransportCredentials) (*grpc.Server, error) {
	if service == nil {
		return nil, contract.ErrInvalid
	}
	server, g, err := newListener(token, concurrent, transport)
	if err != nil {
		return nil, err
	}
	pb.RegisterProviderArtifactsServer(server, &mediaServer{gate: g, service: service})
	return server, nil
}

// NewSourceListener exposes only read-only Execution source verification and bytes.
func NewSourceListener(service SourceService, token string, concurrent int, transport credentials.TransportCredentials) (*grpc.Server, error) {
	if service == nil {
		return nil, contract.ErrInvalid
	}
	server, g, err := newListener(token, concurrent, transport)
	if err != nil {
		return nil, err
	}
	pb.RegisterArtifactSourcesServer(server, &sourceServer{gate: g, service: service})
	return server, nil
}

type mediaServer struct {
	pb.UnimplementedProviderArtifactsServer
	gate    gate
	service MediaService
}

func (s *mediaServer) StageProviderArtifact(ctx context.Context, v *pb.ProviderArtifactRequest) (*pb.ProviderArtifactReceipt, error) {
	return s.call(ctx, v, s.service.StageArtifact)
}
func (s *mediaServer) ResolveProviderArtifact(ctx context.Context, v *pb.ProviderArtifactRequest) (*pb.ProviderArtifactReceipt, error) {
	return s.call(ctx, v, s.service.ResolveArtifact)
}
func (s *mediaServer) call(ctx context.Context, v *pb.ProviderArtifactRequest, fn func(context.Context, contract.Request) (contract.Receipt, error)) (*pb.ProviderArtifactReceipt, error) {
	leave, err := s.gate.enter(ctx)
	if err != nil {
		return nil, err
	}
	defer leave()
	req, err := contract.DecodeRequest(v)
	if err != nil {
		return nil, remoteError(err)
	}
	ack, err := fn(ctx, req)
	if err != nil {
		return nil, remoteError(err)
	}
	if !ack.Matches(req) {
		return nil, remoteError(contract.ErrConflict)
	}
	return contract.EncodeReceipt(ack), nil
}
func (s *mediaServer) ReadProviderArtifact(v *pb.ProviderArtifactRequest, stream grpc.ServerStreamingServer[pb.ProviderArtifactChunk]) error {
	leave, err := s.gate.enter(stream.Context())
	if err != nil {
		return err
	}
	defer leave()
	req, err := contract.DecodeRequest(v)
	if err != nil {
		return remoteError(err)
	}
	ack, err := s.service.ResolveArtifact(stream.Context(), req)
	if err != nil {
		return remoteError(err)
	}
	if !ack.Matches(req) {
		return remoteError(contract.ErrConflict)
	}
	body, err := s.service.OpenArtifact(stream.Context(), req)
	if err != nil {
		return remoteError(err)
	}
	expected := req
	if ack.Canonical != nil {
		expected.ByteSize, expected.SHA256, expected.MIME = ack.Canonical.ByteSize, ack.Canonical.SHA256, ack.Canonical.MIME
	}
	return sendBytes(body, expected, stream.Send)
}

type sourceServer struct {
	pb.UnimplementedArtifactSourcesServer
	gate    gate
	service SourceService
}

func (s *sourceServer) VerifyProviderArtifactSource(ctx context.Context, v *pb.ProviderArtifactRequest) (*pb.ProviderArtifactRequest, error) {
	leave, err := s.gate.enter(ctx)
	if err != nil {
		return nil, err
	}
	defer leave()
	req, err := contract.DecodeRequest(v)
	if err != nil {
		return nil, remoteError(err)
	}
	if err := s.service.VerifyArtifactSource(ctx, req); err != nil {
		return nil, remoteError(err)
	}
	return contract.EncodeRequest(req), nil
}
func (s *sourceServer) ReadProviderArtifactSource(v *pb.ProviderArtifactRequest, stream grpc.ServerStreamingServer[pb.ProviderArtifactChunk]) error {
	leave, err := s.gate.enter(stream.Context())
	if err != nil {
		return err
	}
	defer leave()
	req, err := contract.DecodeRequest(v)
	if err != nil {
		return remoteError(err)
	}
	body, err := s.service.OpenArtifactSource(stream.Context(), req)
	if err != nil {
		return remoteError(err)
	}
	return sendBytes(body, req, stream.Send)
}

func sendBytes(body io.ReadCloser, req contract.Request, send func(*pb.ProviderArtifactChunk) error) error {
	defer func() { _ = body.Close() }()
	hash := sha256.New()
	var total int64
	seq := int32(0)
	for total < req.ByteSize {
		size := int64(contract.ChunkSize)
		if req.ByteSize-total < size {
			size = req.ByteSize - total
		}
		data := make([]byte, int(size))
		n, err := io.ReadFull(body, data)
		if err != nil {
			return remoteError(contract.ErrUnknown)
		}
		hash.Write(data[:n])
		if err := send(&pb.ProviderArtifactChunk{Sequence: seq, Data: data[:n]}); err != nil {
			return err
		}
		total += int64(n)
		seq++
	}
	var extra [1]byte
	n, err := body.Read(extra[:])
	if n != 0 || err != io.EOF || hex.EncodeToString(hash.Sum(nil)) != req.SHA256 {
		return remoteError(contract.ErrConflict)
	}
	if err := body.Close(); err != nil {
		return remoteError(err)
	}
	return nil
}

func remoteError(err error) error {
	switch {
	case errors.Is(err, contract.ErrInvalid):
		return status.Error(codes.InvalidArgument, "invalid artifact request")
	case errors.Is(err, contract.ErrConflict), errors.Is(err, writefence.ErrConflict):
		return status.Error(codes.AlreadyExists, "artifact request conflicts")
	case errors.Is(err, contract.ErrUnknown):
		return status.Error(codes.NotFound, "original artifact unresolved")
	case errors.Is(err, writefence.ErrFenced), errors.Is(err, writefence.ErrFrozen):
		return status.Error(codes.FailedPrecondition, "artifact authority closed")
	default:
		return status.Error(codes.Unavailable, "artifact unavailable")
	}
}
func localError(err error) error {
	switch status.Code(err) {
	case codes.InvalidArgument:
		return contract.ErrInvalid
	case codes.AlreadyExists:
		return contract.ErrConflict
	case codes.NotFound:
		return contract.ErrUnknown
	case codes.FailedPrecondition:
		return writefence.ErrFenced
	default:
		return writefence.ErrUnavailable
	}
}

// Target is one fixed private peer with a separate directional token and mTLS.
type Target struct {
	Address, Token string
	Transport      credentials.TransportCredentials
}

// Client has no automatic retry, fallback or shared database capability.
type Client struct{ target Target }

// NewClient validates a fixed peer configuration before any work can be sent.
func NewClient(target Target) (*Client, error) {
	host, port, err := net.SplitHostPort(target.Address)
	number, e := strconv.Atoi(port)
	if err != nil || e != nil || host == "" || strings.ContainsAny(host, "/@?#% \t\r\n") || number < 1 || number > 65535 || !serviceauth.ValidToken(target.Token) || target.Transport == nil {
		return nil, contract.ErrInvalid
	}
	target.Transport = target.Transport.Clone()
	return &Client{target: target}, nil
}
func (c *Client) connect(ctx context.Context) (context.Context, context.CancelFunc, *grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(ctx, duration)
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	md := metadata.Pairs("authorization", "Bearer "+c.target.Token)
	for _, key := range []string{"traceparent", "tracestate"} {
		if value := carrier[key]; value != "" {
			md.Set(key, value)
		}
	}
	ctx = metadata.NewOutgoingContext(ctx, md)
	conn, err := grpc.NewClient(c.target.Address, grpc.WithTransportCredentials(c.target.Transport.Clone()), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(messageBytes), grpc.MaxCallSendMsgSize(messageBytes), grpc.MaxRetryRPCBufferSize(0)))
	if err != nil {
		cancel()
		return nil, nil, nil, writefence.ErrUnavailable
	}
	return ctx, cancel, conn, nil
}

// StageArtifact sends only the exact original reference. Media pulls durable bytes.
func (c *Client) StageArtifact(ctx context.Context, req contract.Request) (contract.Receipt, error) {
	return c.receipt(ctx, req, false)
}

// ResolveArtifact cannot create a missing original Media responsibility.
func (c *Client) ResolveArtifact(ctx context.Context, req contract.Request) (contract.Receipt, error) {
	return c.receipt(ctx, req, true)
}
func (c *Client) receipt(ctx context.Context, req contract.Request, resolve bool) (contract.Receipt, error) {
	if c == nil || req.Validate() != nil {
		return contract.Receipt{}, contract.ErrInvalid
	}
	ctx, cancel, conn, err := c.connect(ctx)
	if err != nil {
		return contract.Receipt{}, err
	}
	defer cancel()
	defer func() { _ = conn.Close() }()
	var response *pb.ProviderArtifactReceipt
	client := pb.NewProviderArtifactsClient(conn)
	if resolve {
		response, err = client.ResolveProviderArtifact(ctx, contract.EncodeRequest(req))
	} else {
		response, err = client.StageProviderArtifact(ctx, contract.EncodeRequest(req))
	}
	if err != nil {
		return contract.Receipt{}, localError(err)
	}
	ack, err := contract.DecodeReceipt(response)
	if err != nil || !ack.Matches(req) {
		return contract.Receipt{}, contract.ErrConflict
	}
	return ack, nil
}

// VerifyArtifactSource requires the exact original echoed header and final RPC status.
func (c *Client) VerifyArtifactSource(ctx context.Context, req contract.Request) error {
	if c == nil || req.Validate() != nil {
		return contract.ErrInvalid
	}
	ctx, cancel, conn, err := c.connect(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	defer func() { _ = conn.Close() }()
	response, err := pb.NewArtifactSourcesClient(conn).VerifyProviderArtifactSource(ctx, contract.EncodeRequest(req))
	if err != nil {
		return localError(err)
	}
	echo, err := contract.DecodeRequest(response)
	if err != nil || !echo.Equal(req) {
		return contract.ErrConflict
	}
	return nil
}

// OpenArtifact opens Media's original bytes, never a physical location or grant.
func (c *Client) OpenArtifact(ctx context.Context, req contract.Request) (io.ReadCloser, error) {
	return c.open(ctx, req, false)
}

// OpenArtifactSource opens Execution's durable source chunks without mutating them.
func (c *Client) OpenArtifactSource(ctx context.Context, req contract.Request) (io.ReadCloser, error) {
	return c.open(ctx, req, true)
}
func (c *Client) open(ctx context.Context, req contract.Request, source bool) (io.ReadCloser, error) {
	if c == nil || req.Validate() != nil {
		return nil, contract.ErrInvalid
	}
	expected := req
	if !source {
		ack, err := c.ResolveArtifact(ctx, req)
		if err != nil {
			return nil, err
		}
		if !ack.Matches(req) {
			return nil, contract.ErrConflict
		}
		if ack.Canonical != nil {
			expected.ByteSize, expected.SHA256, expected.MIME = ack.Canonical.ByteSize, ack.Canonical.SHA256, ack.Canonical.MIME
		}
	}
	ctx, cancel, conn, err := c.connect(ctx)
	if err != nil {
		return nil, err
	}
	var stream grpc.ServerStreamingClient[pb.ProviderArtifactChunk]
	if source {
		stream, err = pb.NewArtifactSourcesClient(conn).ReadProviderArtifactSource(ctx, contract.EncodeRequest(req))
	} else {
		stream, err = pb.NewProviderArtifactsClient(conn).ReadProviderArtifact(ctx, contract.EncodeRequest(req))
	}
	if err != nil {
		cancel()
		_ = conn.Close()
		return nil, localError(err)
	}
	return &streamReader{recv: stream.Recv, cancel: cancel, conn: conn, req: expected, hash: sha256.New()}, nil
}

type streamReader struct {
	recv          func() (*pb.ProviderArtifactChunk, error)
	cancel        context.CancelFunc
	conn          *grpc.ClientConn
	req           contract.Request
	hash          hash.Hash
	current       *bytes.Reader
	seq           int32
	total         int64
	ended, closed bool
}

func (r *streamReader) Read(p []byte) (int, error) {
	if r.closed {
		return 0, io.ErrClosedPipe
	}
	if len(p) == 0 {
		return 0, nil
	}
	if r.ended {
		return 0, io.EOF
	}
	if r.current == nil || r.current.Len() == 0 {
		frame, err := r.recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if r.total != r.req.ByteSize || hex.EncodeToString(r.hash.Sum(nil)) != r.req.SHA256 {
					return 0, contract.ErrConflict
				}
				r.ended = true
				return 0, io.EOF
			}
			return 0, localError(err)
		}
		expected := int64(contract.ChunkSize)
		if r.req.ByteSize-r.total < expected {
			expected = r.req.ByteSize - r.total
		}
		if frame == nil || len(frame.ProtoReflect().GetUnknown()) != 0 || frame.GetSequence() != r.seq || expected <= 0 || int64(len(frame.GetData())) != expected {
			return 0, contract.ErrConflict
		}
		r.seq++
		r.total += int64(len(frame.GetData()))
		r.hash.Write(frame.GetData())
		r.current = bytes.NewReader(frame.GetData())
	}
	return r.current.Read(p)
}
func (r *streamReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	r.current = nil
	r.cancel()
	return r.conn.Close()
}

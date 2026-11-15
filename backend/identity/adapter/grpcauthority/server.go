package grpcauthority

import (
	"context"
	"crypto/hmac"
	"crypto/subtle"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// Authenticator consumes the existing Auth policy without exposing account
// mutations or minting. Verification, authority reads and admission use NoActivity;
// only the separately typed foreground recorder can update idle activity.
type Authenticator interface {
	CurrentWithActivity(context.Context, string, identity.SessionActivity) (identity.AuthResult, error)
	CurrentAccess(context.Context, string, string, identity.SessionActivity) (identity.AuthResult, error)
	ReadAccessIdentity(context.Context, string) (identity.Principal, error)
}

var _ Authenticator = (*identity.Auth)(nil)

// SessionStore keeps writes and controlled proof reads on the same Identity
// authority. A missing reader cannot silently fall back to authentication alone.
type SessionStore interface {
	identity.SessionAdmissionStore
	identity.SessionAdmissionReader
}

type server struct {
	pb.UnimplementedAuthorityServer
	auth           Authenticator
	store          SessionStore
	tokens         map[writefence.Writer]string
	clock          func() time.Time
	peerNames      map[writefence.Writer]string
	qualifications *identity.TeamQualifications
}

// NewServer creates only the private typed authority listener. Composition roots
// must explicitly supply transport credentials (verified mTLS in production) and
// four distinct dedicated caller tokens. This port is not a complete Identity app.
func NewServer(auth Authenticator, store SessionStore, tokens map[writefence.Writer]string, clock func() time.Time, transport credentials.TransportCredentials) (*grpc.Server, error) {
	return newServer(auth, store, tokens, nil, clock, transport)
}

// NewMTLSServer binds each dedicated bearer audience to its verified service
// certificate. Independent owner processes must use this constructor.
func NewMTLSServer(auth Authenticator, store SessionStore, tokens map[writefence.Writer]string, names map[writefence.Writer]string, clock func() time.Time, transport credentials.TransportCredentials, qualifications ...*identity.TeamQualifications) (*grpc.Server, error) {
	if len(names) != len(writefence.Writers()) || transport == nil || transport.Info().SecurityProtocol != "tls" {
		return nil, writefence.ErrInvalid
	}
	if _, ok := store.(identity.SessionAuthorityReader); !ok {
		return nil, writefence.ErrInvalid
	}
	if _, ok := auth.(identity.ForegroundActivityRecorder); !ok {
		return nil, writefence.ErrInvalid
	}
	copied := map[writefence.Writer]string{}
	seen := map[string]bool{}
	for _, writer := range writefence.Writers() {
		name := names[writer]
		if name == "" || seen[name] {
			return nil, writefence.ErrInvalid
		}
		seen[name] = true
		copied[writer] = name
	}
	if len(qualifications) > 1 || (len(qualifications) == 1 && qualifications[0] == nil) {
		return nil, writefence.ErrInvalid
	}
	return newServer(auth, store, tokens, copied, clock, transport, qualifications...)
}

func newServer(auth Authenticator, store SessionStore, tokens map[writefence.Writer]string, names map[writefence.Writer]string, clock func() time.Time, transport credentials.TransportCredentials, qualifications ...*identity.TeamQualifications) (*grpc.Server, error) {
	if auth == nil || store == nil || clock == nil || transport == nil || len(tokens) != len(writefence.Writers()) {
		return nil, writefence.ErrInvalid
	}
	copied := make(map[writefence.Writer]string, len(tokens))
	seen := map[string]bool{}
	for _, w := range writefence.Writers() {
		token := tokens[w]
		if !serviceauth.ValidToken(token) || seen[token] {
			return nil, writefence.ErrInvalid
		}
		seen[token] = true
		copied[w] = token
	}
	handler := &server{auth: auth, store: store, tokens: copied, clock: clock, peerNames: names}
	if len(qualifications) == 1 {
		handler.qualifications = qualifications[0]
	}
	s := grpc.NewServer(grpc.Creds(transport.Clone()), grpc.MaxRecvMsgSize(MaxMessageBytes), grpc.MaxSendMsgSize(MaxMessageBytes), grpc.MaxHeaderListSize(MaxMessageBytes), grpc.UnaryInterceptor(handler.intercept))
	pb.RegisterAuthorityServer(s, handler)
	return s, nil
}
func (s *server) intercept(ctx context.Context, request any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	values := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(values) != 1 {
		return nil, status.Error(codes.Unauthenticated, "service identity required")
	}
	var caller writefence.Writer
	for _, w := range writefence.Writers() {
		if subtle.ConstantTimeCompare([]byte(values[0]), []byte("Bearer "+s.tokens[w])) == 1 {
			caller = w
		}
	}
	if caller == "" {
		return nil, status.Error(codes.Unauthenticated, "service identity required")
	}
	if s.peerNames != nil {
		remote, ok := peer.FromContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "service identity required")
		}
		tls, ok := remote.AuthInfo.(credentials.TLSInfo)
		if !ok || len(tls.State.VerifiedChains) == 0 || len(tls.State.PeerCertificates) == 0 || tls.State.PeerCertificates[0].VerifyHostname(s.peerNames[caller]) != nil {
			return nil, status.Error(codes.Unauthenticated, "service identity required")
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 30*time.Second {
		return nil, status.Error(codes.InvalidArgument, "bounded deadline required")
	}
	message, ok := request.(proto.Message)
	if !ok || !closedMessage(message) {
		return nil, rpcError(writefence.ErrInvalid)
	}
	switch r := request.(type) {
	case *pb.VerifyRequest:
		if r.Caller != encodeWriter(caller) {
			return nil, rpcError(identity.ErrForbidden)
		}
	case *pb.ReadSessionAuthoritiesRequest:
		if r.Caller != encodeWriter(caller) {
			return nil, rpcError(identity.ErrForbidden)
		}
	case *pb.RecordForegroundActivityRequest:
		purpose, err := decodeForegroundPurpose(r.Purpose)
		if err != nil {
			return nil, rpcError(err)
		}
		owner, err := purpose.Owner()
		if err != nil || r.Caller != encodeWriter(caller) || owner != caller {
			return nil, rpcError(identity.ErrForbidden)
		}
	case *pb.ReadAccountQualificationIntentRequest:
		if caller != writefence.WriterCreation || r.Caller != encodeWriter(caller) {
			return nil, rpcError(identity.ErrForbidden)
		}
	case *pb.IssueTeamAdministratorQualificationRequest:
		if caller != writefence.WriterCreation || r.Caller != encodeWriter(caller) {
			return nil, rpcError(identity.ErrForbidden)
		}
	case *pb.RefreshTeamAdministratorQualificationRequest:
		if caller != writefence.WriterCreation || r.Caller != encodeWriter(caller) {
			return nil, rpcError(identity.ErrForbidden)
		}
	case *pb.ResolveTeamAdministratorQualificationRequest:
		if caller != writefence.WriterCreation || r.Caller != encodeWriter(caller) {
			return nil, rpcError(identity.ErrForbidden)
		}
	case *pb.AdmitSessionCommandRequest:
		if r.Caller != encodeWriter(caller) || r.Target != encodeWriter(caller) {
			return nil, rpcError(identity.ErrForbidden)
		}
	case *pb.ResolveAdmissionRequest:
		if caller != writefence.WriterCreation || r.Caller != encodeWriter(caller) {
			return nil, rpcError(identity.ErrForbidden)
		}
	default:
		return nil, status.Error(codes.Unimplemented, "unknown authority operation")
	}
	// The policy consumer sees only W3C tracing metadata, never user headers or the
	// caller's bearer token. Preserve cancellation and the original bounded deadline.
	carrier := propagation.MapCarrier{}
	safe := metadata.MD{}
	for _, key := range []string{"traceparent", "tracestate"} {
		values := metadata.ValueFromIncomingContext(ctx, key)
		if len(values) == 1 {
			carrier[key] = values[0]
			safe.Set(key, values[0])
		}
	}
	ctx = propagation.TraceContext{}.Extract(ctx, carrier)
	ctx = metadata.NewIncomingContext(ctx, safe)
	return next(ctx, request)
}
func (s *server) authenticate(ctx context.Context, wire *pb.Credential) (Identity, error) {
	c, err := decodeCredential(wire)
	if err != nil {
		return Identity{}, err
	}
	if c.access != "" && c.csrf == "" {
		actor, err := s.auth.ReadAccessIdentity(ctx, c.access)
		if err != nil {
			return Identity{}, err
		}
		if actor.ID == uuid.Nil || actor.SessionID == uuid.Nil || actor.CredentialEpoch <= 0 {
			return Identity{}, identity.ErrSessionInvalid
		}
		return Identity{AccountID: actor.ID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}, nil
	}
	var result identity.AuthResult
	if c.legacy != "" {
		result, err = s.auth.CurrentWithActivity(ctx, c.legacy, identity.NoActivity)
		if err == nil && c.csrf != "" && (!validProof(result.CSRFToken) || !hmac.Equal([]byte(c.csrf), []byte(result.CSRFToken))) {
			err = identity.ErrSessionCSRFInvalid
		}
	} else {
		result, err = s.auth.CurrentAccess(ctx, c.access, c.csrf, identity.NoActivity)
	}
	if err != nil {
		return Identity{}, err
	}
	id := Identity{AccountID: result.User.ID, SessionID: result.Session.ID, CredentialEpoch: result.User.SessionEpoch}
	if id.AccountID == uuid.Nil || id.SessionID == uuid.Nil || id.CredentialEpoch <= 0 || result.Session.AccountID != id.AccountID || result.Session.CredentialRevision != id.CredentialEpoch {
		return Identity{}, identity.ErrSessionInvalid
	}
	return id, nil
}
func (s *server) Verify(ctx context.Context, request *pb.VerifyRequest) (*pb.VerifyResponse, error) {
	id, err := s.authenticate(ctx, request.Credential)
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.VerifyResponse{Identity: encodeIdentity(id)}, nil
}
func (s *server) ReadSessionAuthorities(ctx context.Context, request *pb.ReadSessionAuthoritiesRequest) (*pb.ReadSessionAuthoritiesResponse, error) {
	id, err := s.authenticate(ctx, request.Credential)
	if err != nil {
		return nil, rpcError(err)
	}
	reader, ok := s.store.(identity.SessionAuthorityReader)
	if !ok {
		return nil, rpcError(writefence.ErrUnavailable)
	}
	state, err := reader.ReadSessionAuthorities(ctx, identity.Principal{ID: id.AccountID, SessionID: id.SessionID, CredentialEpoch: id.CredentialEpoch}, s.clock)
	if err != nil {
		return nil, rpcError(err)
	}
	value := SessionAuthorities{Identity: id, Account: state.Account, Session: state.Session}
	if !value.valid() {
		return nil, rpcError(writefence.ErrUnavailable)
	}
	return &pb.ReadSessionAuthoritiesResponse{Identity: encodeIdentity(id), Account: encodeSessionAuthority(state.Account), Session: encodeSessionAuthority(state.Session)}, nil
}

func (s *server) AdmitSessionCommand(ctx context.Context, request *pb.AdmitSessionCommandRequest) (*pb.AdmitSessionCommandResponse, error) {
	if request.Credential == nil || !validProof(request.Credential.CsrfToken) {
		return nil, rpcError(writefence.ErrInvalid)
	}
	command, err := decodeCommand(request.Command)
	if err != nil {
		return nil, rpcError(err)
	}
	target, err := decodeWriter(request.Target)
	if err != nil {
		return nil, rpcError(err)
	}
	id, err := s.authenticate(ctx, request.Credential)
	if err != nil {
		return nil, rpcError(err)
	}
	if !bindsIdentity(command, id) {
		return nil, rpcError(identity.ErrForbidden)
	}
	actor := identity.Principal{ID: id.AccountID, SessionID: id.SessionID, CredentialEpoch: id.CredentialEpoch}
	if err := s.store.RegisterSessionCommand(ctx, actor, command, target, s.clock); err != nil {
		return nil, rpcError(err)
	}
	return &pb.AdmitSessionCommandResponse{Identity: encodeIdentity(id), Command: encodeCommand(command), Target: encodeWriter(target)}, nil
}

func (s *server) ResolveAdmission(ctx context.Context, request *pb.ResolveAdmissionRequest) (*pb.ResolveAdmissionResponse, error) {
	command, err := decodeCommand(request.Command)
	if err != nil {
		return nil, rpcError(err)
	}
	target, err := decodeWriter(request.Target)
	if err != nil {
		return nil, rpcError(err)
	}
	proof, err := s.store.ReadSessionAdmission(ctx, command, target, s.clock)
	if err != nil {
		return nil, rpcError(err)
	}
	id := Identity{AccountID: proof.AccountID, SessionID: proof.SessionID, CredentialEpoch: proof.CredentialEpoch}
	if id.AccountID == uuid.Nil || id.SessionID == uuid.Nil || id.CredentialEpoch <= 0 || proof.Target != target || !sameCommand(proof.Command, command) || !bindsIdentity(proof.Command, id) {
		return nil, rpcError(writefence.ErrUnavailable)
	}
	return &pb.ResolveAdmissionResponse{Proof: &pb.AdmissionProof{Identity: encodeIdentity(id), Command: encodeCommand(proof.Command), Target: encodeWriter(proof.Target)}}, nil
}

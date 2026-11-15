package app

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/identity/adapter/credentialproof"
	"github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcauthority"
	"github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcgovernance"
	"github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcregistration"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/http"
	identitymail "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/mail"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/identity/adapter/refreshredis"
	"github.com/StephenQiu30/glowloom-server/backend/identity/adapter/sessiontoken"
	"github.com/StephenQiu30/glowloom-server/backend/identity/application"
	coordinationpb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/otelconn"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	fencerpc "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// Runtime owns the accepted authentication/session API, its private source RPCs,
// mail delivery, source-coordinated platform qualification and durable recovery.
// Object-backed directory routes remain in their existing composition until their
// source contracts are ready; this program is not a full-Identity cutover claim.
type Runtime struct {
	http                                            *http.Server
	authority, writer                               *grpc.Server
	httpListener, authorityListener, writerListener net.Listener
	database                                        *gorm.DB
	ping                                            func(context.Context) error
	closeDatabase                                   func() error
	traceShutdown                                   func(context.Context) error
	redis                                           *redis.Client
	tasks                                           *taskruntime.Worker
	mail                                            *application.MailDelivery
	cleanup                                         *application.SessionReceiptCleanup
	logger                                          *zap.Logger
	ready, started                                  atomic.Bool
	closeOnce                                       sync.Once
}

// Addresses returns only bound listener addresses, never configuration secrets.
func (r *Runtime) Addresses() (string, string, string) {
	return r.httpListener.Addr().String(), r.authorityListener.Addr().String(), r.writerListener.Addr().String()
}

// New validates the complete source dependency set and actual restricted database
// capability before exposing any listener. It performs no schema or seed writes.
func New(ctx context.Context, cfg Config, logger *zap.Logger) (runtime *Runtime, err error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if logger == nil {
		return nil, errConfiguration
	}
	r := &Runtime{logger: logger}
	defer func() {
		if err != nil {
			r.Close()
		}
	}()
	serverTLS, clientTLS, err := serviceTLS(cfg.TLS)
	if err != nil {
		return nil, err
	}
	provider, shutdown, err := otelconn.Open(ctx, cfg.TraceEndpoint, "identity")
	if err != nil {
		return nil, errors.New("identitytracing initialization failed")
	}
	r.traceShutdown = shutdown
	opened, err := db.Open(ctx, cfg.DatabaseDSN, provider)
	if err != nil {
		return nil, errors.New("identitydatabase connection unavailable")
	}
	r.database = opened.DB
	r.closeDatabase = opened.Close
	sqlDB, err := opened.DB.DB()
	if err != nil {
		return nil, errors.New("identitydatabase pool unavailable")
	}
	r.ping = sqlDB.PingContext
	owner, err := pgfence.New(opened.DB).Owner(ctx)
	if err != nil || owner != writefence.WriterIdentity {
		return nil, errors.New("identityrequires its restricted database capability")
	}
	var elevated bool
	if err := opened.DB.WithContext(ctx).Raw(`SELECT rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls OR has_database_privilege(current_user,current_database(),'CREATE') OR has_schema_privilege(current_user,'identity','CREATE') FROM pg_roles WHERE rolname=current_user`).Scan(&elevated).Error; err != nil || elevated {
		return nil, errors.New("identitydatabase capability is elevated")
	}
	var prepared bool
	if err := opened.DB.WithContext(ctx).Raw(`SELECT to_regclass('identity.registration_preparation') IS NOT NULL AND to_regclass('identity.session_activation') IS NOT NULL AND to_regclass('identity.session_command_admission') IS NOT NULL AND to_regclass('identity.account_qualification_change') IS NOT NULL AND EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='identity.session_revocation_request'::regclass AND contype='c' AND pg_get_constraintdef(oid) LIKE '%refresh_reuse%')`).Scan(&prepared).Error; err != nil || !prepared {
		return nil, errors.New("identitycanonical source schema is required")
	}
	protector, err := credentialproof.LoadValue(cfg.IdentityKey)
	if err != nil {
		return nil, err
	}
	r.writerListener, err = (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.WriterAddress)
	if err != nil {
		return nil, errors.New("identitywriter listener unavailable")
	}
	targets := map[writefence.Writer]fencerpc.Target{}
	for _, writer := range writefence.Writers() {
		p := cfg.Writers[writer]
		address := p.Address
		if writer == writefence.WriterIdentity {
			address = r.writerListener.Addr().String()
			p.ServerName = cfg.TLS.OwnName
		}
		targets[writer] = fencerpc.Target{Address: address, Token: p.Token, Transport: credentials.NewTLS(clientTLS(p.ServerName))}
	}
	controlClient, err := fencerpc.NewClient(writefence.Identity, targets)
	if err != nil {
		return nil, err
	}
	coordinator := writefence.NewCoordinator(pgfence.NewAuthorityStore(opened.DB), controlClient)
	writerHandler, err := fencerpc.NewServer(writefence.WriterIdentity, pgfence.New(opened.DB), cfg.Writers[writefence.WriterIdentity].Token, cfg.CreationControlToken)
	if err != nil {
		return nil, err
	}
	r.writer = grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS.Clone())), grpc.MaxRecvMsgSize(fencerpc.MaxMessageBytes), grpc.MaxSendMsgSize(fencerpc.MaxMessageBytes), grpc.UnaryInterceptor(writerIdentity(cfg)))
	coordinationpb.RegisterWriterFenceServer(r.writer, writerHandler)
	creation, err := grpcregistration.NewPersonalScopeClient(registrationTarget(cfg.CreationRegistration, clientTLS))
	if err != nil {
		return nil, err
	}
	wallet, err := grpcregistration.NewWalletClient(registrationTarget(cfg.ExecutionWallet, clientTLS))
	if err != nil {
		return nil, err
	}
	store := pgidentity.NewRegistrationStore(opened.DB)
	auth := application.NewEmailAuth(store, protector, application.EmailPolicy{TermsVersion: identityhttp.AcceptedLegalVersion(cfg.TermsVersion), SourceHourlyLimit: cfg.MailSourceHourlyLimit, GlobalHourlyLimit: cfg.MailGlobalHourlyLimit}, time.Now)
	if err := auth.WithRegistrationPreparation(store, creation, wallet, coordinator); err != nil {
		return nil, err
	}
	if err := auth.WithSessionActivation(store, coordinator, protector); err != nil {
		return nil, err
	}
	if err := auth.WithPasswordChanges(pgidentity.NewPasswordChangeStore(opened.DB), coordinator, protector); err != nil {
		return nil, err
	}
	if err := auth.WithPasswordResets(pgidentity.NewPasswordResetStore(opened.DB), coordinator, protector); err != nil {
		return nil, err
	}
	emailChangeStore := pgidentity.NewSourceEmailChangeStore(opened.DB)
	emailChange := application.NewEmailChange(auth, emailChangeStore, time.Now)
	if err := emailChange.WithSource(emailChangeStore, coordinator, protector); err != nil {
		return nil, err
	}
	governanceStore := pgidentity.NewSourceGovernanceStore(opened.DB)
	governance := application.NewGovernance(governanceStore, protector, time.Now)
	if err := governance.WithSource(auth, governanceStore, coordinator, protector); err != nil {
		return nil, err
	}
	teamGovernance, err := grpcgovernance.New(cfg.CreationGovernance.Address, cfg.CreationGovernance.Token, credentials.NewTLS(clientTLS(cfg.CreationGovernance.ServerName)))
	if err != nil {
		return nil, err
	}
	if err := governance.WithTeamGovernance(teamGovernance); err != nil {
		return nil, err
	}
	if err := r.configureSession(ctx, auth, cfg.Session); err != nil {
		return nil, err
	}
	r.authority, err = grpcauthority.NewMTLSServer(auth, pgidentity.NewSessionAdmissionStore(opened.DB), cfg.AuthorityTokens, cfg.AuthorityPeerNames, time.Now, credentials.NewTLS(serverTLS.Clone()), application.NewTeamQualifications(pgidentity.NewTeamQualificationStore(opened.DB)))
	if err != nil {
		return nil, err
	}
	r.authorityListener, err = (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.AuthorityAddress)
	if err != nil {
		return nil, errors.New("identityauthority listener unavailable")
	}
	closuresStore := pgidentity.NewSessionRevocationStore(opened.DB)
	closures := application.NewSessionRevocations(closuresStore, coordinator, time.Now)
	durable := application.NewPublicSessionRevocations(auth, closuresStore, closures, time.Now)
	if err := auth.WithSessionRevocationRequests(durable); err != nil {
		return nil, err
	}
	registrations, err := application.NewRegistrationPreparation(store, creation, wallet, coordinator)
	if err != nil {
		return nil, err
	}
	sessionRecovery := application.NewSessionRevocationRecovery(closuresStore, closures, time.Now)
	handler := func(kind string, resume func(context.Context, uuid.UUID) error) taskruntime.Handler {
		return func(ctx context.Context, job taskruntime.Job) (time.Time, error) {
			id, err := uuid.Parse(job.BusinessID)
			if err != nil || id == uuid.Nil || job.Kind != kind {
				return time.Time{}, errors.New("invalid Identity original-request task")
			}
			if err := resume(ctx, id); err != nil {
				return time.Time{}, err
			}
			return time.Time{}, nil
		}
	}
	r.tasks = &taskruntime.Worker{Store: taskruntime.NewStore(opened.DB), Concurrency: 2, Lease: 30 * time.Second, Poll: 250 * time.Millisecond, Handlers: map[string]taskruntime.Handler{
		application.RegistrationRecoveryTask: handler(application.RegistrationRecoveryTask, func(ctx context.Context, id uuid.UUID) error {
			_, err := registrations.ResumeAccepted(ctx, id, store)
			return err
		}),
		application.SessionActivationRecoveryTask: handler(application.SessionActivationRecoveryTask, auth.ResumeSessionActivation),
		application.PasswordChangeRecoveryTask:    handler(application.PasswordChangeRecoveryTask, auth.ResumePasswordChange),
		application.PasswordResetRecoveryTask:     handler(application.PasswordResetRecoveryTask, auth.ResumePasswordReset),
		application.EmailChangeRecoveryTask:       handler(application.EmailChangeRecoveryTask, emailChange.ResumeEmailChange),
		application.GovernanceRecoveryTask:        handler(application.GovernanceRecoveryTask, governance.ResumeGovernance),
		application.SessionRevocationRecoveryTask: handler(application.SessionRevocationRecoveryTask, sessionRecovery.Resume),
	}}
	cfg.Mail.PublicOrigin = cfg.PublicOrigin
	if cfg.MailCAPEM != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(cfg.MailCAPEM)) {
			return nil, errors.New("invalid Identity SMTP trust root")
		}
		cfg.Mail.RootCAs = pool
	}
	sender, err := identitymail.NewSender(cfg.Mail)
	if err != nil {
		return nil, err
	}
	r.mail = application.NewMailDelivery(pgidentity.NewMailStore(opened.DB), protector, sender, time.Now)
	r.cleanup = application.NewSessionReceiptCleanup(store, time.Now)
	engine := gin.New()
	engine.Use(tracing(provider))
	engine.GET("/healthz", func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.GET("/readyz", func(c *gin.Context) {
		probe, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		if !r.ready.Load() || r.ping(probe) != nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		c.Status(http.StatusOK)
	})
	public := engine.Group("/api")
	identityhttp.NewAuthPolicyHandler(application.NewAuthPolicyQuery(pgidentity.NewStore(opened.DB), identityhttp.AcceptedLegalVersion(cfg.TermsVersion), identityhttp.LegalPackageVersion(), true)).Register(public)
	email := identityhttp.NewEmailAuthHandler(auth, strings.TrimRight(cfg.PublicOrigin, "/"), strings.HasPrefix(cfg.PublicOrigin, "https://")).WithPublicSessionRevocations(durable).WithSessionPolicy(identityhttp.SessionHTTPPolicy{V2Enabled: cfg.Session.Enabled, LegacyUpgradeUntil: cfg.Session.LegacyUpgradeUntil})
	email.RegisterPreparedSessions(public)
	email.RegisterPreparedCredentialChanges(public)
	identityhttp.NewEmailChangeHandler(emailChange, strings.HasPrefix(cfg.PublicOrigin, "https://")).RegisterSource(public, email)
	identityhttp.NewGovernanceHandler(governance).RegisterSource(public, email)
	authenticated := public.Group("", email.RequireSession())
	identityhttp.NewSelfProfileHandler(application.NewSelfProfile(pgidentity.NewSourceSelfProfileStore(opened.DB), time.Now)).RegisterReadAndPreferences(authenticated)
	identityhttp.NewSessionDirectoryHandler(application.NewSessionDirectory(pgidentity.NewSessionDirectoryStore(opened.DB), time.Now), strings.HasPrefix(cfg.PublicOrigin, "https://")).RegisterRead(authenticated)
	r.http = &http.Server{Handler: engine, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: time.Minute}
	r.httpListener, err = (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.HTTPAddress)
	if err != nil {
		return nil, errors.New("identityHTTP listener unavailable")
	}
	return r, nil
}

func registrationTarget(p Peer, tlsConfig func(string) *tls.Config) grpcregistration.Target {
	return grpcregistration.Target{Address: p.Address, Token: p.Token, Transport: credentials.NewTLS(tlsConfig(p.ServerName))}
}
func serviceTLS(c ServiceTLS) (*tls.Config, func(string) *tls.Config, error) {
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(c.CAPEM)) {
		return nil, nil, errors.New("invalid Identity service trust root")
	}
	certificate, err := tls.X509KeyPair([]byte(c.CertificatePEM), []byte(c.PrivateKeyPEM))
	if err != nil {
		return nil, nil, errors.New("invalid Identity service certificate")
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil || leaf.VerifyHostname(c.OwnName) != nil {
		return nil, nil, errors.New("identityservice certificate does not match its dedicated identity")
	}
	server := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert}
	client := func(name string) *tls.Config {
		return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, RootCAs: roots, ServerName: name}
	}
	return server, client, nil
}
func writerIdentity(cfg Config) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		values := metadata.ValueFromIncomingContext(ctx, "authorization")
		if len(values) != 1 {
			return nil, status.Error(codes.Unauthenticated, "service identity required")
		}
		expected := ""
		if subtle.ConstantTimeCompare([]byte(values[0]), []byte("Bearer "+cfg.Writers[writefence.WriterIdentity].Token)) == 1 {
			expected = cfg.TLS.OwnName
		}
		if subtle.ConstantTimeCompare([]byte(values[0]), []byte("Bearer "+cfg.CreationControlToken)) == 1 {
			expected = cfg.CreationControlPeerName
		}
		remote, ok := peer.FromContext(ctx)
		if !ok || expected == "" {
			return nil, status.Error(codes.Unauthenticated, "service identity required")
		}
		tlsInfo, ok := remote.AuthInfo.(credentials.TLSInfo)
		if !ok || len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.PeerCertificates) == 0 || tlsInfo.State.PeerCertificates[0].VerifyHostname(expected) != nil {
			return nil, status.Error(codes.Unauthenticated, "service identity required")
		}
		return next(ctx, request)
	}
}
func (r *Runtime) configureSession(ctx context.Context, auth *application.Auth, cfg SessionConfig) error {
	auth.WithLegacyUpgradeUntil(cfg.LegacyUpgradeUntil)
	if cfg.VerifyKeys == "" {
		if cfg.Enabled {
			return errConfiguration
		}
		return nil
	}
	if !validAddress(cfg.RedisAddress, false) {
		return errConfiguration
	}
	tokens, err := sessiontoken.LoadValues(cfg.PrivateKey, cfg.VerifyKeys, cfg.ActiveKID)
	if err != nil {
		return err
	}
	r.redis = redis.NewClient(&redis.Options{Addr: cfg.RedisAddress, Username: cfg.RedisUsername, Password: cfg.RedisPassword, DB: cfg.RedisDB, DialTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second})
	if r.redis.Ping(ctx).Err() != nil {
		return errors.New("identityrefresh coordination unavailable")
	}
	coordinator, err := refreshredis.New(r.redis)
	if err != nil {
		return err
	}
	var receipts application.RefreshReceiptProtector
	if cfg.ReceiptKey != "" {
		receipts, err = sessiontoken.LoadReceiptValue(cfg.ReceiptKey)
		if err != nil {
			return err
		}
	}
	auth.WithSessionSecurity(tokens, receipts, coordinator).WithSessionIssuance(cfg.Enabled)
	return nil
}
func tracing(provider trace.TracerProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := propagation.TraceContext{}.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		ctx, span := provider.Tracer("identity.http").Start(ctx, "identity.http")
		defer span.End()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// Run serves real HTTP/RPC and owns every worker through cancellation and join.
func (r *Runtime) Run(ctx context.Context) error {
	if r == nil || !r.started.CompareAndSwap(false, true) {
		return errors.New("identityruntime already started")
	}
	defer r.Close()
	group, owned := errgroup.WithContext(ctx)
	r.http.BaseContext = func(net.Listener) context.Context { return owned }
	group.Go(func() error {
		err := r.http.Serve(r.httpListener)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	})
	group.Go(func() error {
		err := r.authority.Serve(r.authorityListener)
		if errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}
		return err
	})
	group.Go(func() error {
		err := r.writer.Serve(r.writerListener)
		if errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}
		return err
	})
	group.Go(func() error { return r.tasks.Run(owned) })
	group.Go(func() error { return r.batches(owned) })
	group.Go(func() error {
		<-owned.Done()
		r.ready.Store(false)
		stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = r.http.Shutdown(stop)
		stopRPC(stop, r.authority)
		stopRPC(stop, r.writer)
		return nil
	})
	r.ready.Store(true)
	err := group.Wait()
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("identityruntime stopped: %w", err)
	}
	return nil
}
func (r *Runtime) batches(ctx context.Context) error {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			if _, err := r.mail.RunOnce(ctx, 20); err != nil && ctx.Err() == nil {
				r.logger.Warn("identity mail batch deferred")
			}
			cleanup, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, err := r.cleanup.RunOnce(cleanup, 1000)
			cancel()
			if err != nil && ctx.Err() == nil {
				r.logger.Warn("identity refresh receipt cleanup deferred")
			}
		}
	}
}
func stopRPC(ctx context.Context, server *grpc.Server) {
	done := make(chan struct{})
	go func() { server.GracefulStop(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		server.Stop()
		<-done
	}
}

// Close releases constructed resources once, including partially built runtimes.
func (r *Runtime) Close() {
	if r == nil {
		return
	}
	r.closeOnce.Do(func() {
		r.ready.Store(false)
		if r.http != nil {
			_ = r.http.Close()
		}
		if r.authority != nil {
			r.authority.Stop()
		}
		if r.writer != nil {
			r.writer.Stop()
		}
		for _, listener := range []net.Listener{r.httpListener, r.authorityListener, r.writerListener} {
			if listener != nil {
				_ = listener.Close()
			}
		}
		if r.redis != nil {
			_ = r.redis.Close()
		}
		if r.closeDatabase != nil {
			_ = r.closeDatabase()
		}
		if r.traceShutdown != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = r.traceShutdown(ctx)
		}
	})
}

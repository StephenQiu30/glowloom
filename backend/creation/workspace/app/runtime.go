package app

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/executionbudget"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/executionwork"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/grpcaccountgovernance"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/grpcpreparation"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/grpcregistration"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/httpprojects"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/identitysource"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/mediacover"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/mediawork"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/pgsource"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/kafkaconn"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/otelconn"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/outbox"
	kafkaoutbox "github.com/StephenQiu30/glowloom-server/backend/internal/platform/outbox/kafka"
	pgoutbox "github.com/StephenQiu30/glowloom-server/backend/internal/platform/outbox/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	fencerpc "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// Runtime owns actual project API/registration/source controls and durable task /
// event delivery. This project slice is not a cutover of all Creation modules.
type Runtime struct {
	http           *http.Server
	httpListener   net.Listener
	servers        []*grpc.Server
	listeners      []net.Listener
	database       *db.Connection
	broker         *kafkaconn.Connection
	traceShutdown  func(context.Context) error
	worker         *taskruntime.Worker
	relay          *outbox.Relay
	logger         *zap.Logger
	ready, started atomic.Bool
	closeOnce      sync.Once
}

// Addresses returns only bound HTTP/writer/registration/preparation endpoints.
func (r *Runtime) Addresses() (string, string, string, string) {
	return r.httpListener.Addr().String(), r.listeners[0].Addr().String(), r.listeners[1].Addr().String(), r.listeners[2].Addr().String()
}

// GovernanceAddress returns the independent Identity-audience governance endpoint.
func (r *Runtime) GovernanceAddress() string { return r.listeners[3].Addr().String() }

// New validates real restricted capability and dependencies before exposing work.
// It neither installs schema/data nor changes roles or guesses missing authorities.
func New(ctx context.Context, cfg Config, logger *zap.Logger) (runtime *Runtime, err error) {
	if cfg.Validate() != nil || logger == nil {
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
	provider, shutdown, err := otelconn.Open(ctx, cfg.TraceEndpoint, "creation")
	if err != nil {
		return nil, errors.New("creation tracing initialization failed")
	}
	r.traceShutdown = shutdown
	r.database, err = db.Open(ctx, cfg.DatabaseDSN, provider)
	if err != nil {
		return nil, errors.New("creation database unavailable")
	}
	writer := pgfence.New(r.database.DB)
	owner, err := writer.Owner(ctx)
	if err != nil || owner != writefence.WriterCreation {
		return nil, errors.New("creation requires its restricted database capability")
	}
	var elevated bool
	if err := r.database.DB.WithContext(ctx).Raw(`SELECT rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls OR pg_has_role(session_user,'glowloom_app','MEMBER') OR has_database_privilege(current_user,current_database(),'CREATE') OR has_schema_privilege(current_user,'workspace','CREATE') FROM pg_roles WHERE rolname=current_user`).Scan(&elevated).Error; err != nil || elevated {
		return nil, errors.New("creation database capability is elevated")
	}
	var canonical bool
	if err := r.database.DB.WithContext(ctx).Raw(`SELECT to_regclass('workspace.project_preparation') IS NOT NULL AND to_regclass('workspace.personal_scope_preparation') IS NOT NULL AND to_regclass('infra.authority_head') IS NOT NULL AND to_regclass('infra.writer_command') IS NOT NULL AND to_regclass('infra.task_work') IS NOT NULL AND to_regclass('workspace.account_governance_preparation') IS NOT NULL`).Scan(&canonical).Error; err != nil || !canonical {
		return nil, errors.New("creation canonical source schema required")
	}
	r.broker, err = kafkaconn.Open(cfg.KafkaBrokers)
	if err != nil {
		return nil, errors.New("creation event broker configuration invalid")
	}
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = r.broker.Ping(probe)
	cancel()
	if err != nil {
		return nil, errors.New("creation event broker unavailable")
	}
	for _, address := range []string{cfg.WriterAddress, cfg.RegistrationAddress, cfg.PreparationAddress, cfg.GovernanceAddress} {
		listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", address)
		if err != nil {
			return nil, errors.New("creation private listener unavailable")
		}
		r.listeners = append(r.listeners, listener)
	}
	targets := map[writefence.Writer]fencerpc.Target{}
	for _, targetWriter := range writefence.Writers() {
		p := cfg.Writers[targetWriter]
		if targetWriter == writefence.WriterCreation {
			p.Address = r.listeners[0].Addr().String()
			p.ServerName = cfg.TLS.OwnName
		}
		targets[targetWriter] = fencerpc.Target{Address: p.Address, Token: p.Token, Transport: credentials.NewTLS(clientTLS(p.ServerName))}
	}
	controls, err := fencerpc.NewClient(writefence.Creation, targets)
	if err != nil {
		return nil, err
	}
	coordinator := writefence.NewCoordinator(pgfence.NewAuthorityStore(r.database.DB), controls)
	writerHandler, err := fencerpc.NewServer(writefence.WriterCreation, writer, cfg.IdentityWriterToken, cfg.Writers[writefence.WriterCreation].Token)
	if err != nil {
		return nil, err
	}
	writerServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS.Clone())), grpc.MaxRecvMsgSize(fencerpc.MaxMessageBytes), grpc.MaxSendMsgSize(fencerpc.MaxMessageBytes), grpc.UnaryInterceptor(writerIdentity(cfg)))
	coordination.RegisterWriterFenceServer(writerServer, writerHandler)
	r.servers = append(r.servers, writerServer)
	store := pgsource.New(r.database.DB)
	resources, err := source.NewPersonalRegistrations(pgsource.NewPersonalRegistrationStore(r.database.DB))
	if err != nil {
		return nil, err
	}
	registrations, err := source.NewReadyPersonalRegistrations(resources, store, coordinator)
	if err != nil {
		return nil, err
	}
	registrationServer, err := grpcregistration.NewServer(registrations, cfg.RegistrationToken, cfg.IdentityAuthority.ServerName, credentials.NewTLS(serverTLS.Clone()))
	if err != nil {
		return nil, err
	}
	r.servers = append(r.servers, registrationServer)
	proofServer, err := grpcpreparation.NewServer(store, cfg.PreparationToken, cfg.ExecutionBudget.ServerName, credentials.NewTLS(serverTLS.Clone()))
	if err != nil {
		return nil, err
	}
	r.servers = append(r.servers, proofServer)
	identity, err := identitysource.New(cfg.IdentityAuthority.Address, cfg.IdentityAuthority.Token, credentials.NewTLS(clientTLS(cfg.IdentityAuthority.ServerName)))
	if err != nil {
		return nil, err
	}
	governance, err := source.NewAccountGovernance(pgsource.NewAccountGovernanceStore(r.database.DB), identity, coordinator)
	if err != nil {
		return nil, err
	}
	governanceServer, err := grpcaccountgovernance.NewServer(governance, cfg.GovernanceToken, cfg.IdentityAuthority.ServerName, credentials.NewTLS(serverTLS.Clone()))
	if err != nil {
		return nil, err
	}
	r.servers = append(r.servers, governanceServer)
	budgets, err := executionbudget.New(cfg.ExecutionBudget.Address, cfg.ExecutionBudget.Token, credentials.NewTLS(clientTLS(cfg.ExecutionBudget.ServerName)))
	if err != nil {
		return nil, err
	}
	projects, err := source.NewProjectPreparations(store, budgets, coordinator)
	if err != nil {
		return nil, err
	}
	create, err := source.NewNativeProjectCreation(identity, store, source.NewAdmission(identity, store), writer, projects)
	if err != nil {
		return nil, err
	}
	executionWork, err := executionwork.New(cfg.ExecutionWork.Address, cfg.ExecutionWork.Token, credentials.NewTLS(clientTLS(cfg.ExecutionWork.ServerName)))
	if err != nil {
		return nil, err
	}
	mediaWork, err := mediawork.New(cfg.MediaWork.Address, cfg.MediaWork.Token, credentials.NewTLS(clientTLS(cfg.MediaWork.ServerName)))
	if err != nil {
		return nil, err
	}
	coverReader, err := mediacover.New(cfg.MediaCover.Address, cfg.MediaCover.Token, credentials.NewTLS(clientTLS(cfg.MediaCover.ServerName)))
	if err != nil {
		return nil, err
	}
	closures, err := source.NewProjectClosures(store, map[writefence.Writer]source.ProjectWorkReader{writefence.WriterCreation: store, writefence.WriterExecution: executionWork, writefence.WriterMedia: mediaWork}, coordinator, coverReader)
	if err != nil {
		return nil, err
	}
	closeProject, err := source.NewNativeProjectClosure(identity, store, source.NewAdmission(identity, store), writer, closures, store)
	if err != nil {
		return nil, err
	}
	resolve := httpprojects.CookieResolver(strings.HasPrefix(cfg.PublicOrigin, "https://"))
	handler, err := httpprojects.NewNativeProjectHandler(create, resolve)
	if err != nil {
		return nil, err
	}
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		requestCtx := propagation.TraceContext{}.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		requestCtx, span := provider.Tracer("creation.http").Start(requestCtx, "creation.http")
		defer span.End()
		c.Request = c.Request.WithContext(requestCtx)
		c.Next()
	}, httpapi.Middleware(cfg.PublicOrigin))
	engine.GET("/healthz", func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.GET("/readyz", func(c *gin.Context) {
		probe, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		if !r.ready.Load() || r.database.Ping(probe) != nil || r.broker.Ping(probe) != nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		c.Status(http.StatusOK)
	})
	public := engine.Group("/api")
	public.Use(func(c *gin.Context) {
		credential, _, err := resolve(c)
		if c.IsAborted() {
			return
		}
		if err == nil {
			_, err = identity.ReadSourceSessionAuthorities(c.Request.Context(), credential)
		}
		if err != nil {
			httpprojects.WriteProjectProblem(c, err)
			return
		}
		c.Next()
	})
	handler.Register(public)
	closureHandler, err := httpprojects.NewNativeClosureHandler(closeProject, resolve)
	if err != nil {
		return nil, err
	}
	closureHandler.Register(public)
	r.http = &http.Server{Handler: engine, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: time.Minute}
	r.httpListener, err = (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.HTTPAddress)
	if err != nil {
		return nil, errors.New("creation HTTP listener unavailable")
	}
	r.worker = &taskruntime.Worker{Store: taskruntime.NewStore(r.database.DB), Handlers: map[string]taskruntime.Handler{source.ProjectPreparationTask: projects.Handle, source.ProjectClosureTask: closures.Handle, source.AccountGovernanceRecoveryTask: func(ctx context.Context, job taskruntime.Job) (time.Time, error) {
		id, err := uuid.Parse(job.BusinessID)
		if err != nil || id == uuid.Nil || id.String() != job.BusinessID {
			return time.Time{}, writefence.ErrInvalid
		}
		return time.Time{}, governance.ResumeAccountGovernance(ctx, id)
	}}, Concurrency: 2, Lease: 30 * time.Second, Poll: 250 * time.Millisecond}
	r.relay = outbox.NewRelay(pgoutbox.NewStore(r.database.DB), kafkaoutbox.NewPublisher(r.broker.Client))
	return r, nil
}

func serviceTLS(c ServiceTLS) (*tls.Config, func(string) *tls.Config, error) {
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(c.CAPEM)) {
		return nil, nil, errors.New("invalid Creation private trust root")
	}
	certificate, err := tls.X509KeyPair([]byte(c.CertificatePEM), []byte(c.PrivateKeyPEM))
	if err != nil {
		return nil, nil, errors.New("invalid Creation service certificate")
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil || leaf.VerifyHostname(c.OwnName) != nil {
		return nil, nil, errors.New("creation certificate does not match its service identity")
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
		expected := ""
		if len(values) == 1 {
			if subtle.ConstantTimeCompare([]byte(values[0]), []byte("Bearer "+cfg.IdentityWriterToken)) == 1 {
				expected = cfg.IdentityAuthority.ServerName
			}
			if subtle.ConstantTimeCompare([]byte(values[0]), []byte("Bearer "+cfg.Writers[writefence.WriterCreation].Token)) == 1 {
				expected = cfg.TLS.OwnName
			}
		}
		remote, ok := peer.FromContext(ctx)
		if !ok || expected == "" {
			return nil, status.Error(codes.Unauthenticated, "service identity required")
		}
		identity, ok := remote.AuthInfo.(credentials.TLSInfo)
		if !ok || len(identity.State.VerifiedChains) == 0 || len(identity.State.PeerCertificates) == 0 || identity.State.PeerCertificates[0].VerifyHostname(expected) != nil {
			return nil, status.Error(codes.Unauthenticated, "service identity required")
		}
		return next(ctx, request)
	}
}

// Run owns every listener, task and relay goroutine through cancellation and join.
func (r *Runtime) Run(ctx context.Context) error {
	if r == nil || !r.started.CompareAndSwap(false, true) {
		return errors.New("creation runtime already started")
	}
	defer r.Close()
	group, owned := errgroup.WithContext(ctx)
	r.http.BaseContext = func(net.Listener) context.Context { return owned }
	group.Go(func() error {
		err := r.http.Serve(r.httpListener)
		if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
			return nil
		}
		return err
	})
	for i, server := range r.servers {
		listener := r.listeners[i]
		group.Go(func() error {
			err := server.Serve(listener)
			if errors.Is(err, grpc.ErrServerStopped) || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		})
	}
	group.Go(func() error { return r.worker.Run(owned) })
	group.Go(func() error { return r.relay.Run(owned) })
	group.Go(func() error {
		<-owned.Done()
		r.ready.Store(false)
		bounded, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = r.http.Shutdown(bounded)
		var wait sync.WaitGroup
		for _, server := range r.servers {
			wait.Go(server.GracefulStop)
		}
		stopped := make(chan struct{})
		go func() { wait.Wait(); close(stopped) }()
		select {
		case <-stopped:
		case <-bounded.Done():
			for _, server := range r.servers {
				server.Stop()
			}
			<-stopped
		}
		return nil
	})
	r.ready.Store(true)
	return group.Wait()
}

// Close releases only resources owned by this program; it never mutates business.
func (r *Runtime) Close() {
	if r == nil {
		return
	}
	r.closeOnce.Do(func() {
		r.ready.Store(false)
		if r.http != nil {
			_ = r.http.Close()
		}
		if r.httpListener != nil {
			_ = r.httpListener.Close()
		}
		for _, server := range r.servers {
			server.Stop()
		}
		for _, listener := range r.listeners {
			_ = listener.Close()
		}
		if r.broker != nil {
			r.broker.Close()
		}
		if r.database != nil {
			_ = r.database.Close()
		}
		if r.traceShutdown != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = r.traceShutdown(ctx)
		}
	})
}

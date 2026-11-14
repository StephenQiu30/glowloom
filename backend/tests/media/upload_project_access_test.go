package media_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspacedomain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// Routing fixtures deliberately stub identity, but Creation still owns project
// state and row locks. They do not constitute live-session authorization tests.
type routingUploadScope struct{}

func (routingUploadScope) Resolve(_ context.Context, actor identity.Principal, id uuid.UUID, _ bool) (workspacedomain.Scope, error) {
	return workspacedomain.Scope{ID: id, PhysicalScopeID: actor.OrgID}, nil
}

type routingUploadAccess struct {
	routingScopeStub
	projects *pgworkspace.ProjectContentAccessStore
}

func (a routingUploadAccess) AuthorizeUploadProject(ctx context.Context, actor identity.Principal, id uuid.UUID) (string, error) {
	return a.projects.AuthorizeUploadProject(ctx, actor, id)
}
func (a routingUploadAccess) AwaitUploadProjectCommit(ctx context.Context, id uuid.UUID) error {
	return a.projects.AwaitUploadProjectCommit(ctx, id)
}

// uploadPortTx never opens a socket or executes SQL. Any bypass of the owner
// port reaches an explicit error instead of accidentally consulting a test DB.
type uploadPortTx struct {
	queries, commits, rollbacks int
}

func (p *uploadPortTx) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	p.queries++
	return nil, errors.New("unexpected SQL before upload authority")
}
func (p *uploadPortTx) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	p.queries++
	return nil, errors.New("unexpected SQL before upload authority")
}
func (p *uploadPortTx) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	p.queries++
	return nil, errors.New("unexpected SQL before upload authority")
}
func (p *uploadPortTx) QueryRowContext(context.Context, string, ...any) *sql.Row {
	p.queries++
	return &sql.Row{}
}

type uploadPortPool struct {
	gorm.ConnPool
	tx *uploadPortTx
}

func (p *uploadPortPool) BeginTx(context.Context, *sql.TxOptions) (gorm.ConnPool, error) {
	return p.tx, nil
}
func (p *uploadPortTx) Commit() error   { p.commits++; return nil }
func (p *uploadPortTx) Rollback() error { p.rollbacks++; return nil }

type uploadProjectPort struct {
	routingScopeStub
	authorizations, waits int
	actor                 identity.Principal
	project               uuid.UUID
	rejection             error
}

func (a *uploadProjectPort) AuthorizeUploadProject(_ context.Context, actor identity.Principal, project uuid.UUID) (string, error) {
	a.authorizations++
	a.actor, a.project = actor, project
	return "", a.rejection
}
func (a *uploadProjectPort) AwaitUploadProjectCommit(_ context.Context, project uuid.UUID) error {
	a.waits++
	a.project = project
	return a.rejection
}

func uploadPortDB(t *testing.T) (*gorm.DB, *uploadPortTx) {
	t.Helper()
	pool := &uploadPortTx{}
	database, err := gorm.Open(postgres.New(postgres.Config{Conn: &uploadPortPool{ConnPool: pool, tx: pool}}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	return database, pool
}

func TestUploadProjectPortFailsClosedBeforeMediaSQL(t *testing.T) {
	for _, cleanup := range []bool{false, true} {
		t.Run(map[bool]string{false: "new upload", true: "unknown commit cleanup"}[cleanup], func(t *testing.T) {
			database, pool := uploadPortDB(t)
			rejected := errors.New("creation authority unresolved")
			port := &uploadProjectPort{rejection: rejected}
			factoryCalls := 0
			store := pgmedia.NewScopedStore(database, func(tx *gorm.DB) media.ScopeAccess {
				factoryCalls++
				if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
					t.Error("owner port is not bound to the caller transaction")
				}
				return port
			})
			actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New()}
			project := uuid.New()
			var err error
			if cleanup {
				_, err = store.UploadAssetExists(t.Context(), project, uuid.New())
			} else {
				_, err = store.AuthorizeUpload(t.Context(), actor, project)
			}
			if !errors.Is(err, rejected) || port.project != project || factoryCalls != 1 || pool.queries != 0 || pool.commits != 0 || pool.rollbacks != 1 {
				t.Fatalf("authority bypass: error=%v project=%v factory=%d SQL=%d commits=%d rollbacks=%d", err, port.project, factoryCalls, pool.queries, pool.commits, pool.rollbacks)
			}
			if cleanup {
				if port.authorizations != 0 || port.waits != 1 {
					t.Fatal("unknown commit cleanup was coupled to new upload authorization")
				}
			} else if port.authorizations != 1 || port.waits != 0 || port.actor != actor {
				t.Fatal("upload did not pass the original actor to its authority")
			}
		})
	}
}

func TestUploadProjectPortRejectsMissingCapability(t *testing.T) {
	database, pool := uploadPortDB(t)
	store := pgmedia.NewScopedStore(database, func(*gorm.DB) media.ScopeAccess { return routingScopeStub{} })
	if _, err := store.AuthorizeUpload(t.Context(), identity.Principal{}, uuid.New()); !errors.Is(err, media.ErrUnavailable) {
		t.Fatalf("missing upload authority accepted: %v", err)
	}
	if _, err := store.UploadAssetExists(t.Context(), uuid.New(), uuid.New()); !errors.Is(err, media.ErrUnavailable) {
		t.Fatalf("missing cleanup serialization accepted: %v", err)
	}
	if pool.queries != 0 || pool.commits != 0 || pool.rollbacks != 2 {
		t.Fatalf("missing capability reached SQL: %+v", pool)
	}
}

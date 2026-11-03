package agent_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	agentwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/agent"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"
)

type controlPreflight struct{ calls atomic.Int32 }

func (p *controlPreflight) Freeze(_ context.Context, _ identity.Principal, _ uuid.UUID, in app.StartRunInput) (domain.GoalManifest, error) {
	p.calls.Add(1)
	g := controlRun().Goal
	g.Goal = in.Goal
	g.Inputs = nil
	g.Skills = nil
	g.PlanningBudgetMicros = in.PlanningBudgetMicros
	g.RootBudgetMicros = in.RootBudgetMicros
	return g, nil
}
func controlDB(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_AGENT_CONTROL_DB_DSN")
	if dsn == "" {
		t.Skip("set GLOWLOOM_TEST_AGENT_CONTROL_DB_DSN to an isolated migrated PostgreSQL database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	q.Set("options", "-c role=glowloom_app")
	parsed.RawQuery = q.Encode()
	open := func(dsn string) *gorm.DB {
		conn, err := platformdb.Open(t.Context(), dsn, noop.NewTracerProvider())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		return conn.DB
	}
	return open(parsed.String()), open(dsn)
}
func actorProject(t *testing.T, owner *gorm.DB) (identity.Principal, uuid.UUID) {
	t.Helper()
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: identitydomain.RoleProducer}
	pid := uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace.organization(id,name) VALUES(?,?)`, []any{actor.OrgID, "agent-test-" + actor.OrgID.String()}},
		{`INSERT INTO identity."user"(id,org_id,login_name,display_name,password_hash,role,status,must_change_password) VALUES(?,?,?,?,?,'producer','active',false)`, []any{actor.ID, actor.OrgID, "agent-" + actor.ID.String(), "Harness fixture", "not-a-credential"}},
		{`INSERT INTO workspace.project(id,org_id,owner_id,name,description,aspect_ratio,style_type,resolution,status,revision,default_models) VALUES(?,?,?,'Harness fixture','','16:9','realistic','1080p','active',1,'{}')`, []any{pid, actor.OrgID, actor.ID}},
	} {
		if err := owner.Exec(q.sql, q.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	actor = identityfixture.Session(t, owner, actor)
	if err := owner.Exec(`UPDATE workspace.project SET owner_scope_id=? WHERE id=?`, actor.ScopeID, pid).Error; err != nil {
		t.Fatal(err)
	}
	return actor, pid
}
func prepareControl(t *testing.T) (*app.Controller, *pg.ControlStore, *gorm.DB, identity.Principal, uuid.UUID, domain.Session, app.StartRunInput) {
	t.Helper()
	db, owner := controlDB(t)
	actor, pid := actorProject(t, owner)
	store := pg.NewControlStore(db, agentwire.ProvideControlAccess)
	service := app.NewController(store, &controlPreflight{}, time.Now)
	session, err := service.CreateSession(t.Context(), actor, pid, uuid.New(), "短剧创作")
	if err != nil {
		t.Fatal(err)
	}
	input := app.StartRunInput{RequestID: uuid.New(), ExpectedRevision: 1, Goal: "先解析雨夜故事", Model: domain.ModelChoice{Key: "fixture-dialogue", Mode: "text_to_text", Params: []byte(`{}`)}, PlanningBudgetMicros: 30, RootBudgetMicros: 100}
	return service, store, owner, actor, pid, session, input
}
func TestControlStoreStableRunReplayAndApproval(t *testing.T) {
	service, store, owner, actor, pid, session, input := prepareControl(t)
	first, err := service.StartRun(t.Context(), actor, pid, session.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.StartRun(t.Context(), actor, pid, session.ID, input)
	if err != nil || replay != first {
		t.Fatal("run replay", replay, err)
	}
	changed := input
	changed.Goal = "different"
	if _, err := service.StartRun(t.Context(), actor, pid, session.ID, changed); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("input changed on replay", err)
	}
	run, err := service.ReadRun(t.Context(), actor, pid, first.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.AuthorizeRuntime(t.Context(), run.RuntimeIdentity(), "model.generate"); !errors.Is(err, domain.ErrApprovalRequired) {
		t.Fatal("unapproved model", err)
	}
	var pending int64
	if err := owner.Table("agent.harness_command").Where("run_id=? AND delivery_state='pending'", run.ID).Count(&pending).Error; err != nil || pending != 0 {
		t.Fatal("initial start dispatched", pending, err)
	}
	approve := app.ControlRequest{RequestID: uuid.New(), ExpectedRevision: run.Revision, ControlInput: domain.ControlInput{Action: "approve", CandidateID: run.Candidate.ID, ManifestHash: run.Candidate.Hash, MaxMicros: 30, ExpiresAt: time.Now().Add(time.Hour)}}
	approved, err := service.ApplyCommand(t.Context(), actor, pid, run.ID, approve)
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.ApplyCommand(t.Context(), actor, pid, run.ID, approve)
	if err != nil || again != approved {
		t.Fatal("approval replay", again, err)
	}
	run, err = service.ReadRun(t.Context(), actor, pid, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Authorize(t.Context(), run.RuntimeIdentity(), "model.generate", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApplyCommand(t.Context(), actor, pid, run.ID, approve); err == nil {
		t.Fatal("replay bypassed current actor authority")
	}
	if err := store.Authorize(t.Context(), run.RuntimeIdentity(), "model.generate", time.Now()); err == nil {
		t.Fatal("worker bypassed revoked actor")
	}
}
func TestControlStoreStopLinearizesAgainstConcurrentReservations(t *testing.T) {
	service, _, owner, actor, pid, session, input := prepareControl(t)
	receipt, err := service.StartRun(t.Context(), actor, pid, session.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.ReadRun(t.Context(), actor, pid, receipt.RunID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ApplyCommand(t.Context(), actor, pid, run.ID, app.ControlRequest{RequestID: uuid.New(), ExpectedRevision: run.Revision, ControlInput: domain.ControlInput{Action: "approve", CandidateID: run.Candidate.ID, ManifestHash: run.Candidate.Hash, MaxMicros: 30, ExpiresAt: time.Now().Add(time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	run, err = service.ReadRun(t.Context(), actor, pid, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	id := run.RuntimeIdentity()
	key := uuid.New()
	admitted, err := service.AdmitRuntimeAction(t.Context(), id, key, domain.ActionRequest{Action: "model.generate", MaxMicros: 20})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.AdmitRuntimeAction(t.Context(), id, key, domain.ActionRequest{Action: "model.generate", MaxMicros: 20})
	if err != nil || !replay.Replayed || replay.ID != admitted.ID {
		t.Fatal("admission replay", replay, err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, err := service.AdmitRuntimeAction(t.Context(), id, uuid.New(), domain.ActionRequest{Action: "model.compaction", MaxMicros: 10})
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, domain.ErrBudget) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("concurrent reservations overspent", successes.Load())
	}
	stopped, err := service.ApplyCommand(t.Context(), actor, pid, run.ID, app.ControlRequest{RequestID: uuid.New(), ExpectedRevision: run.Revision, ControlInput: domain.ControlInput{Action: "stop"}})
	if err != nil {
		t.Fatal(err)
	}
	if stopped.ControlRevision <= id.ControlRevision {
		t.Fatal("barrier did not advance")
	}
	if _, err := service.AdmitRuntimeAction(t.Context(), id, uuid.New(), domain.ActionRequest{Action: "model.generate", MaxMicros: 0}); !errors.Is(err, domain.ErrFenced) {
		t.Fatal("Host-disconnected old revision bypassed stop", err)
	}
	var count int64
	if err := owner.Table("agent.harness_admission").Where("run_id=? AND status='in_flight'", run.ID).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("stop erased admitted operations", count, err)
	}
}

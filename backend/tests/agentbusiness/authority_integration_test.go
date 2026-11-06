package agentbusiness_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

var reset sync.Once

type access struct{}

func (access) AuthorizeRequest(context.Context, identity.Principal, uuid.UUID, bool) error {
	return nil
}
func (access) AuthorizeWorker(context.Context, identity.Principal, uuid.UUID) error { return nil }
func fixture(t *testing.T) (*pg.Store, domain.Admission, domain.Grant) {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_AGENT_BUSINESS_DB_DSN")
	if dsn == "" {
		t.Skip("set GLOWLOOM_TEST_AGENT_BUSINESS_DB_DSN to an isolated PostgreSQL database initialized from db/schema.sql")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(parsed.Path, "_test") || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") {
		t.Fatal("Agent business fixture requires an isolated loopback *_test database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	reset.Do(func() {
		if err := db.Exec(`TRUNCATE agent_business.run_authorization CASCADE`).Error; err != nil {
			t.Fatal(err)
		}
	})
	s := pg.NewStore(db, func(*gorm.DB) app.ControlAccess { return access{} }, time.Now)
	a := domain.Admission{RootRunID: uuid.New(), SessionID: uuid.New(), SubmissionID: uuid.New(), RequestID: uuid.New(), ContentHash: domain.Hash([]byte(`{"goal":"test"}`)), Scope: domain.Scope{ActorID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New(), ProjectID: uuid.New()}, BudgetMicros: 100}
	t.Cleanup(func() {
		for _, table := range []string{"quiesced_receipt", "call_receipt", "proposal", "control_decision", "execution_grant", "run_authorization"} {
			if err := db.Exec("DELETE FROM agent_business."+table+" WHERE root_run_id=?", a.RootRunID).Error; err != nil {
				t.Error(err)
			}
		}
	})
	if err = s.AdmitRun(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	g, err := s.RegisterGrant(t.Context(), domain.GrantRequest{RootRunID: a.RootRunID, SubmissionID: a.SubmissionID, OwnerEpoch: 1, LeaseUntil: time.Now().Add(time.Minute), AuthorizationRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	return s, a, g
}
func decide(t *testing.T, s *pg.Store, a domain.Admission, action string, revision int64) domain.Decision {
	t.Helper()
	d, err := s.Decide(t.Context(), a.Scope.Principal(), a.Scope.ProjectID, a.RootRunID, domain.DecisionInput{RequestID: uuid.New(), Action: action, ExpectedAuthorizationRevision: revision})
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func TestGrantPrivateScopeEpochExpiryAndSegment(t *testing.T) {
	s, a, g := fixture(t)
	wrong := a.Scope.Principal()
	wrong.ID = uuid.New()
	if _, err := s.ReadAuthority(t.Context(), wrong, a.Scope.ProjectID, a.RootRunID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross actor: %v", err)
	}
	if _, err := s.ReadAuthority(t.Context(), a.Scope.Principal(), uuid.New(), a.RootRunID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross project: %v", err)
	}
	for _, mutate := range []func(*domain.Grant){func(x *domain.Grant) { x.SubmissionID = uuid.New() }, func(x *domain.Grant) { x.OwnerEpoch++ }, func(x *domain.Grant) { x.AuthorizationRevision++ }, func(x *domain.Grant) { x.GrantID = uuid.New() }} {
		bad := g
		mutate(&bad)
		if _, err := s.ValidateGrant(t.Context(), bad); err == nil {
			t.Fatal("forged grant accepted")
		}
	}
	newer := domain.GrantRequest{RootRunID: a.RootRunID, SubmissionID: a.SubmissionID, OwnerEpoch: 2, LeaseUntil: time.Now().Add(time.Minute), AuthorizationRevision: 1}
	if _, err := s.RegisterGrant(t.Context(), newer); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ValidateGrant(t.Context(), g); err == nil {
		t.Fatal("old epoch admitted")
	}
	newer.OwnerEpoch = 1
	if _, err := s.RegisterGrant(t.Context(), newer); err == nil {
		t.Fatal("epoch decreased")
	}
	newer.OwnerEpoch = 2
	newer.LeaseUntil = time.Now().Add(-time.Second)
	if _, err := s.RegisterGrant(t.Context(), newer); err == nil {
		t.Fatal("expired lease admitted")
	}
}
func TestResumeRequiresQuiescenceUniqueContinuationAndStopDominates(t *testing.T) {
	s, a, g := fixture(t)
	pause := decide(t, s, a, "pause", 1)
	if _, err := s.ValidateGrant(t.Context(), g); err == nil {
		t.Fatal("pause did not fence")
	}
	resume := decide(t, s, a, "resume", pause.AuthorizationRevision)
	req := domain.GrantRequest{RootRunID: a.RootRunID, SubmissionID: a.SubmissionID, OwnerEpoch: 2, LeaseUntil: time.Now().Add(time.Minute), AuthorizationRevision: resume.AuthorizationRevision}
	if _, err := s.RegisterGrant(t.Context(), req); !errors.Is(err, domain.ErrBlocked) {
		t.Fatalf("owner cleared barrier: %v", err)
	}
	q := domain.QuiescedReceipt{ReceiptID: uuid.New(), DecisionID: resume.DecisionID, AuthorizationRevision: resume.AuthorizationRevision, OwnerEpoch: 2, ContinuationID: domain.ContinuationID(a.RootRunID, resume.DecisionID), LeaseUntil: time.Now().Add(time.Minute)}
	next, err := s.Quiesce(t.Context(), a.RootRunID, q)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.Quiesce(t.Context(), a.RootRunID, q)
	if err != nil || next.GrantID != replay.GrantID {
		t.Fatal("grant replay", err)
	}
	q.ContinuationID = uuid.New()
	if _, err := s.Quiesce(t.Context(), a.RootRunID, q); err == nil {
		t.Fatal("second continuation admitted")
	}
	if _, err := s.ValidateGrant(t.Context(), next); err != nil {
		t.Fatal(err)
	}
	stop := decide(t, s, a, "stop", resume.AuthorizationRevision)
	if _, err := s.ValidateGrant(t.Context(), next); err == nil {
		t.Fatal("stop did not fence")
	}
	if _, err := s.Decide(t.Context(), a.Scope.Principal(), a.Scope.ProjectID, a.RootRunID, domain.DecisionInput{RequestID: uuid.New(), Action: "resume", ExpectedAuthorizationRevision: stop.AuthorizationRevision}); !errors.Is(err, domain.ErrTerminal) {
		t.Fatalf("stop reopened %v", err)
	}
	q.ContinuationID = next.SubmissionID
	if _, err := s.Quiesce(t.Context(), a.RootRunID, q); err == nil {
		t.Fatal("late resume overwrote stop")
	}
}
func TestDecisionConcurrentRevisionAndDurableDelivery(t *testing.T) {
	s, a, _ := fixture(t)
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Decide(context.Background(), a.Scope.Principal(), a.Scope.ProjectID, a.RootRunID, domain.DecisionInput{RequestID: uuid.New(), Action: "pause", ExpectedAuthorizationRevision: 1}); err == nil {
				wins.Add(1)
			} else if !errors.Is(err, domain.ErrConflict) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("revision winners %d", wins.Load())
	}
	claim, ok, err := s.ClaimDecision(t.Context(), time.Now())
	if err != nil || !ok {
		t.Fatal("outbox claim", err)
	}
	if err := s.Acknowledge(t.Context(), claim.RootRunID, domain.AppliedReceipt{ReceiptID: uuid.New(), DecisionID: claim.DecisionID, AuthorizationRevision: claim.AuthorizationRevision, Status: "pending", ExecutionRevision: 1}); err == nil {
		t.Fatal("accepted pending as applied")
	}
	ack := domain.AppliedReceipt{ReceiptID: uuid.New(), DecisionID: claim.DecisionID, AuthorizationRevision: claim.AuthorizationRevision, Status: "applied", ExecutionRevision: 1}
	if err := s.Acknowledge(t.Context(), claim.RootRunID, ack); err != nil {
		t.Fatal(err)
	}
	if err := s.Acknowledge(t.Context(), claim.RootRunID, ack); err != nil {
		t.Fatal("lost ack replay", err)
	}
}
func TestCallReceiptUnknownDoesNotReexecuteOrReserveTwice(t *testing.T) {
	s, _, g := fixture(t)
	input := []byte(`{"prompt":"one"}`)
	call := domain.CallInput{Grant: g, Call: domain.CallIdentity{TaskID: "task", CallID: "call", CheckpointID: "checkpoint", Attempt: 1, ContractVersion: "v1", InputHash: domain.Hash(input)}, Input: input, MaxMicros: 10}
	var executions atomic.Int32
	execute := func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		executions.Add(1)
		return domain.CallResult{Status: "unknown", Result: []byte(`{"provider_request_id":"original"}`)}, nil
	}
	first, err := s.ExecuteCall(t.Context(), "model", "text", call, execute)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.ExecuteCall(t.Context(), "model", "text", call, execute)
	if err != nil || again.ReceiptID != first.ReceiptID || executions.Load() != 1 {
		t.Fatal("unknown reexecuted", executions.Load(), err)
	}
	call.Input = []byte(`{"prompt":"changed"}`)
	call.Call.InputHash = domain.Hash(call.Input)
	if _, err := s.ExecuteCall(t.Context(), "model", "text", call, execute); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("changed stable call", err)
	}
}

func TestSkillVersionsPrivateHashPinnedAndImmediatelyRevoked(t *testing.T) {
	s, a, _ := fixture(t)
	input := domain.SkillInput{RequestID: uuid.New(), Name: "storyboard", Version: "1.0.0", Description: "分镜知识", Body: "# Storyboard", Resources: map[string]string{"references/camera.md": "wide / close"}}
	installed, err := s.InstallSkill(t.Context(), a.Scope.Principal(), a.Scope.ProjectID, input)
	if err != nil {
		t.Fatal(err)
	}
	a.RootRunID = uuid.New()
	a.RequestID = uuid.New()
	a.SubmissionID = uuid.New()
	a.SkillPins = []domain.SkillPin{{SkillID: installed.SkillID, Version: installed.Version, ResourceHash: installed.ResourceHash}}
	if err = s.AdmitRun(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	g, err := s.RegisterGrant(t.Context(), domain.GrantRequest{RootRunID: a.RootRunID, SubmissionID: a.SubmissionID, OwnerEpoch: 1, AuthorizationRevision: 1, LeaseUntil: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReadSkill(t.Context(), g, installed.SkillID, installed.Version, installed.ResourceHash, "../secrets"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("resource traversal", err)
	}
	content, err := s.ReadSkill(t.Context(), g, installed.SkillID, installed.Version, installed.ResourceHash, "references/camera.md")
	if err != nil || content.Content != "wide / close" {
		t.Fatal("resource read", err)
	}
	if _, err = s.ReadSkill(t.Context(), g, installed.SkillID, installed.Version, "wrong", ""); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("hash drift", err)
	}
	other := a.Scope.Principal()
	other.ID = uuid.New()
	if _, err = s.SetSkillEnabled(t.Context(), other, a.Scope.ProjectID, installed.SkillID, installed.Version, false); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("cross owner disable", err)
	}
	if _, err = s.SetSkillEnabled(t.Context(), a.Scope.Principal(), a.Scope.ProjectID, installed.SkillID, installed.Version, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReadSkill(t.Context(), g, installed.SkillID, installed.Version, installed.ResourceHash, ""); !errors.Is(err, domain.ErrBlocked) {
		t.Fatal("frozen disabled skill still readable", err)
	}
}

func TestAtomicToolRollbackReplayBudgetAndStopSerialization(t *testing.T) {
	s, a, g := fixture(t)
	body := []byte(`{"write":"fixture"}`)
	input := domain.CallInput{Grant: g, Call: domain.CallIdentity{TaskID: "root", CallID: "atomic-write", CheckpointID: "boundary", Attempt: 1, ContractVersion: "v1", InputHash: domain.Hash(body)}, Input: body, MaxMicros: 3}
	failed := errors.New("formal command failed")
	if _, err := s.ExecuteAtomicTool(t.Context(), "fixture", input, func(tx *gorm.DB, _ domain.Scope, _ uuid.UUID) (domain.CallResult, error) {
		if err := tx.Exec(`UPDATE agent_business.run_authorization SET budget_micros=budget_micros+10 WHERE root_run_id=?`, a.RootRunID).Error; err != nil {
			return domain.CallResult{}, err
		}
		return domain.CallResult{}, failed
	}); !errors.Is(err, failed) {
		t.Fatal("failure not propagated", err)
	}
	authority, err := s.CurrentAuthority(t.Context(), a.RootRunID)
	if err != nil || authority.BudgetMicros != 100 || authority.ReservedMicros != 0 {
		t.Fatal("failed business write leaked", authority, err)
	}
	entered, release, completed := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var executions atomic.Int32
	execute := func(tx *gorm.DB, _ domain.Scope, _ uuid.UUID) (domain.CallResult, error) {
		executions.Add(1)
		close(entered)
		<-release
		if err := tx.Exec(`UPDATE agent_business.run_authorization SET budget_micros=budget_micros+1 WHERE root_run_id=?`, a.RootRunID).Error; err != nil {
			return domain.CallResult{}, err
		}
		return domain.CallResult{Status: "completed", Result: []byte(`{"revision":2}`), CostMicros: 2}, nil
	}
	go func() {
		_, err := s.ExecuteAtomicTool(context.Background(), "fixture", input, execute)
		completed <- err
	}()
	<-entered
	stopped := make(chan error, 1)
	go func() {
		_, err := s.Decide(context.Background(), a.Scope.Principal(), a.Scope.ProjectID, a.RootRunID, domain.DecisionInput{RequestID: uuid.New(), Action: "stop", ExpectedAuthorizationRevision: 1})
		stopped <- err
	}()
	close(release)
	if err := <-completed; err != nil {
		t.Fatal(err)
	}
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	replay, err := s.ExecuteAtomicTool(t.Context(), "fixture", input, execute)
	if err != nil || replay.CostMicros != 2 || executions.Load() != 1 {
		t.Fatal("committed receipt lost after stop", err)
	}
	input.Call.CallID = "late-write"
	if _, err := s.ExecuteAtomicTool(t.Context(), "fixture", input, execute); !errors.Is(err, domain.ErrTerminal) {
		t.Fatal("new write after stop", err)
	}
	authority, err = s.CurrentAuthority(t.Context(), a.RootRunID)
	if err != nil || authority.BudgetMicros != 101 || authority.ReservedMicros != 2 {
		t.Fatal("atomic accounting/replay", authority, err)
	}
}
func TestCanonicalHashCrossLanguageGolden(t *testing.T) {
	if got := domain.Hash([]byte(`{"name":"<角色>&"}`)); got != "49f209bb03e92a15616a0ef206d13f7cc751253fb4315b141b3aaceec55f04c7" {
		t.Fatal(got)
	}
}

type proposalCheck struct{}

func (proposalCheck) ValidateProposal(context.Context, domain.Scope, domain.ProposalInput) error {
	return nil
}
func TestProposalFencesTreeAndEarlyApprovalCannotOpenOldSegment(t *testing.T) {
	s, a, g := fixture(t)
	s.WithProposalValidator(func(*gorm.DB) app.DecisionValidator { return proposalCheck{} })
	candidate := []byte(`{"operation_id":"` + uuid.NewString() + `","quote_micros":10}`)
	input := domain.ProposalInput{Grant: g, RequestID: uuid.New(), CandidateHash: domain.Hash(candidate), CanvasRevision: 3, PriceVersion: "fixture-price", BudgetMicros: 10, Candidate: candidate}
	proposal, err := s.Propose(t.Context(), input)
	if err != nil || !proposal.Control.Terminate {
		t.Fatal("proposal", err)
	}
	input.Grant.ExpiresAt = input.Grant.ExpiresAt.Add(time.Second)
	replay, err := s.Propose(t.Context(), input)
	if err != nil || proposal.ProposalID != replay.ProposalID {
		t.Fatal("proposal replay after grant renewal", err)
	}
	other := input
	other.RequestID = uuid.New()
	if _, err = s.Propose(t.Context(), other); !errors.Is(err, domain.ErrBlocked) {
		t.Fatal("competing proposal replaced first", err)
	}
	approved, err := s.Decide(t.Context(), a.Scope.Principal(), a.Scope.ProjectID, a.RootRunID, domain.DecisionInput{RequestID: uuid.New(), Action: "approve", ProposalID: proposal.ProposalID, CandidateHash: proposal.CandidateHash, ExpectedAuthorizationRevision: proposal.AuthorizationRevision})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ValidateGrant(t.Context(), g); !errors.Is(err, domain.ErrBlocked) {
		t.Fatal("early approve opened old grant", err)
	}
	receipt := domain.AppliedReceipt{ReceiptID: uuid.New(), DecisionID: approved.DecisionID, AuthorizationRevision: approved.AuthorizationRevision, ExecutionRevision: 2, Status: "applied"}
	if err = s.Acknowledge(t.Context(), a.RootRunID, receipt); !errors.Is(err, domain.ErrBlocked) {
		t.Fatal("approved without durable quiescence", err)
	}
	q := domain.QuiescedReceipt{ReceiptID: uuid.New(), DecisionID: approved.DecisionID, AuthorizationRevision: approved.AuthorizationRevision, OwnerEpoch: 1, ContinuationID: domain.ContinuationID(a.RootRunID, approved.DecisionID), LeaseUntil: time.Now().Add(time.Minute)}
	next, err := s.Quiesce(t.Context(), a.RootRunID, q)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Acknowledge(t.Context(), a.RootRunID, receipt); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ValidateGrant(t.Context(), next); err != nil {
		t.Fatal("continuation denied", err)
	}
	if _, err = s.ValidateGrant(t.Context(), g); err == nil {
		t.Fatal("old segment borrowed continuation")
	}
	stop := decide(t, s, a, "stop", approved.AuthorizationRevision)
	if err = s.Acknowledge(t.Context(), a.RootRunID, domain.AppliedReceipt{ReceiptID: uuid.New(), DecisionID: approved.DecisionID, AuthorizationRevision: approved.AuthorizationRevision, ExecutionRevision: 3, Status: "superseded"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("changed already-applied receipt accepted", err)
	}
	if stop.Action != "stop" {
		t.Fatal(stop)
	}
}

func TestDatabaseRolesCannotCrossBusinessAndRuntimeTables(t *testing.T) {
	_, _, _ = fixture(t)
	db, err := gorm.Open(postgres.Open(os.Getenv("GLOWLOOM_TEST_AGENT_BUSINESS_DB_DSN")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error("close database connection:", err)
		}
	})
	for _, tc := range []struct{ role, query string }{{"glowloom_agent", `SELECT root_run_id FROM agent_business.run_authorization LIMIT 1`}, {"glowloom_app", `SELECT session_id FROM agent.runtime_sessions LIMIT 1`}} {
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SET LOCAL ROLE " + tc.role).Error; err != nil {
				return err
			}
			return tx.Exec(tc.query).Error
		})
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("%s crossed ownership boundary: %v", tc.role, err)
		}
	}
}

func TestFreeAtomicToolAcceptsCallerUpperBoundWithoutReservingIt(t *testing.T) {
	s, a, g := fixture(t)
	raw := []byte(`{"read":"current"}`)
	in := domain.CallInput{Grant: g, Call: domain.CallIdentity{TaskID: "task", CallID: "free", CheckpointID: "point", Attempt: 1, ContractVersion: "v1", InputHash: domain.Hash(raw)}, Input: raw, MaxMicros: 1_000_000}
	out, err := s.ExecuteAtomicTool(t.Context(), "free_read", in, func(*gorm.DB, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		return domain.CallResult{Status: "completed", Result: []byte(`{"actual":true}`)}, nil
	})
	if err != nil || out.CostMicros != 0 {
		t.Fatal("free formal command was charged caller upper bound", err)
	}
	current, err := s.CurrentAuthority(t.Context(), a.RootRunID)
	if err != nil || current.ReservedMicros != 0 {
		t.Fatal("free command reserved fees", err)
	}
}

func TestValidateGrantReturnsFreshRemainingBudgetForSuccessiveModelCalls(t *testing.T) {
	s, a, g := fixture(t)
	if g.AvailableMicros != 100 {
		t.Fatal("initial available budget", g.AvailableMicros)
	}
	raw := json.RawMessage(`{"messages":[]}`)
	call := domain.CallInput{Grant: g, Call: domain.CallIdentity{TaskID: "budget-model", CallID: "first", CheckpointID: "model", Attempt: 1, ContractVersion: "v1", InputHash: domain.Hash(raw)}, Input: raw, MaxMicros: 100}
	executes := 0
	model := func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		executes++
		return domain.CallResult{Status: "completed", Result: json.RawMessage(`{"output":"fixture"}`), CostMicros: 1}, nil
	}
	if _, err := s.ExecuteCall(t.Context(), "model", "text", call, model); err != nil {
		t.Fatal(err)
	}
	// The caller's stale available field is informational; validation must return
	// the current Go ledger balance and ExecuteCall must still recheck atomically.
	call.Grant.AvailableMicros = 1000000
	fresh, err := s.ValidateGrant(t.Context(), call.Grant)
	if err != nil || fresh.AvailableMicros != 99 {
		t.Fatal("fresh balance", fresh.AvailableMicros, err)
	}
	call.Grant = fresh
	call.MaxMicros = fresh.AvailableMicros
	call.Call.CallID = "second"
	if _, err = s.ExecuteCall(t.Context(), "model", "text", call, model); err != nil {
		t.Fatal("second model rejected within total budget", err)
	}
	fresh, err = s.ValidateGrant(t.Context(), g)
	if err != nil || fresh.AvailableMicros != 98 || executes != 2 {
		t.Fatal("settled balance", fresh.AvailableMicros, executes, err)
	}
	call.Call.CallID = "overrun"
	call.MaxMicros = 99
	if _, err = s.ExecuteCall(t.Context(), "model", "text", call, model); !errors.Is(err, domain.ErrBudget) || executes != 2 {
		t.Fatal("stale balance bypassed Go fence", err)
	}
	authority, err := s.CurrentAuthority(t.Context(), a.RootRunID)
	if err != nil || authority.ReservedMicros != 2 {
		t.Fatal("total accounting", authority, err)
	}
}

func TestSourceIntentDurableGrantHashUnknownReplayAndFreeAccounting(t *testing.T) {
	s, a, g := fixture(t)
	raw := json.RawMessage(`{"action":"create","body":"original"}`)
	in := domain.CallInput{Grant: g, Call: domain.CallIdentity{TaskID: "source", CallID: "source-original", CheckpointID: "source", Attempt: 1, ContractVersion: "v1", InputHash: domain.Hash(raw)}, Input: raw, MaxMicros: 1000000}
	hash := domain.Hash(raw)
	calls := 0
	admit := func(tx *gorm.DB, _ domain.Scope, id uuid.UUID) error {
		return pg.BindSourceAuthorization(tx, id, g, hash)
	}
	first, err := s.ExecuteToolIntent(t.Context(), "script_source_publish", in, admit, func(ctx context.Context, scope domain.Scope, id uuid.UUID) (domain.CallResult, error) {
		calls++
		if err := s.AuthorizeSourcePublication(ctx, scope.Principal(), scope.ProjectID, id, hash); err != nil {
			t.Fatal("original durable authority absent before I/O", err)
		}
		if err := s.AuthorizeSourcePublication(ctx, scope.Principal(), scope.ProjectID, id, domain.Hash([]byte(`{"other":true}`))); !errors.Is(err, domain.ErrConflict) {
			t.Fatal("source key reused for different body", err)
		}
		return domain.CallResult{}, errors.New("fixture actual I/O outcome unknown")
	})
	if err != nil || first.Status != "unknown" || calls != 1 {
		t.Fatal(first, err, calls)
	}
	replay, err := s.ExecuteToolIntent(t.Context(), "script_source_publish", in, admit, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		t.Fatal("unknown source reexecuted")
		return domain.CallResult{}, nil
	})
	if err != nil || replay.ReceiptID != first.ReceiptID {
		t.Fatal(err)
	}
	in.Call.CallID = "replacement"
	if _, err = s.ExecuteToolIntent(t.Context(), "script_source_publish", in, admit, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		t.Fatal("replacement source executed")
		return domain.CallResult{}, nil
	}); !errors.Is(err, domain.ErrBlocked) {
		t.Fatal("unknown source got replacement call", err)
	}
	current, err := s.CurrentAuthority(t.Context(), a.RootRunID)
	if err != nil || current.ReservedMicros != 0 {
		t.Fatal("free external source reserved manifest cap", current, err)
	}
	decide(t, s, a, "stop", 1)
	if err := s.AuthorizeSourcePublication(t.Context(), a.Scope.Principal(), a.Scope.ProjectID, first.ReceiptID, hash); !errors.Is(err, domain.ErrTerminal) {
		t.Fatal("Stop failed durable source fence", err)
	}
}

func TestLostQuiescedGrantReplyRebindsOriginalContinuationToRegisteredNewOwner(t *testing.T) {
	s, a, _ := fixture(t)
	paused := decide(t, s, a, "pause", 1)
	resumeInput := domain.DecisionInput{RequestID: uuid.New(), Action: "resume", ExpectedAuthorizationRevision: paused.AuthorizationRevision}
	resumed, err := s.Decide(t.Context(), a.Scope.Principal(), a.Scope.ProjectID, a.RootRunID, resumeInput)
	if err != nil {
		t.Fatal(err)
	}
	request := domain.GrantRequest{RootRunID: a.RootRunID, SubmissionID: a.SubmissionID, OwnerEpoch: 1, AuthorizationRevision: resumed.AuthorizationRevision, LeaseUntil: time.Now().Add(time.Minute)}
	if _, err := s.RegisterGrant(t.Context(), request); !errors.Is(err, domain.ErrBlocked) {
		t.Fatal("approval fence cleared early", err)
	}
	proof := domain.QuiescedReceipt{ReceiptID: uuid.New(), DecisionID: resumed.DecisionID, AuthorizationRevision: resumed.AuthorizationRevision, OwnerEpoch: 1, ContinuationID: domain.ContinuationID(a.RootRunID, resumed.DecisionID), LeaseUntil: time.Now().Add(time.Minute)}
	lost, err := s.Quiesce(t.Context(), a.RootRunID, proof)
	if err != nil {
		t.Fatal(err)
	}
	// Go committed the grant but Node lost this response before its atomic Pi
	// input submission. A new owner must recover this same proof and segment.
	nextProof := proof
	nextProof.OwnerEpoch = 2
	if _, err = s.Quiesce(t.Context(), a.RootRunID, nextProof); err == nil {
		t.Fatal("unregistered owner recovered approval")
	}
	request.OwnerEpoch = 2
	request.SubmissionID = proof.ContinuationID
	registered, err := s.RegisterGrant(t.Context(), request)
	if err != nil {
		t.Fatal("registered new owner on current continuation", err)
	}
	next, err := s.Quiesce(t.Context(), a.RootRunID, nextProof)
	if err != nil {
		t.Fatal("lost grant reply permanently stranded approved continuation", err)
	}
	if next.GrantID == lost.GrantID || next.GrantID != registered.GrantID || next.SubmissionID != lost.SubmissionID || next.OwnerEpoch != 2 {
		t.Fatal("not one original continuation under new owner", lost, next)
	}
	again, err := s.Quiesce(t.Context(), a.RootRunID, nextProof)
	if err != nil || again.GrantID != next.GrantID {
		t.Fatal("new-owner same-proof replay signed another grant", again, err)
	}
	if _, err = s.ValidateGrant(t.Context(), lost); err == nil {
		t.Fatal("lost old-owner grant remained usable")
	}
	if _, err = s.ValidateGrant(t.Context(), next); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*domain.QuiescedReceipt){func(p *domain.QuiescedReceipt) { p.ReceiptID = uuid.New() }, func(p *domain.QuiescedReceipt) { p.ContinuationID = uuid.New() }, func(p *domain.QuiescedReceipt) { p.OwnerEpoch = 1 }, func(p *domain.QuiescedReceipt) { p.OwnerEpoch = 3 }, func(p *domain.QuiescedReceipt) { p.AuthorizationRevision-- }, func(p *domain.QuiescedReceipt) {
		p.DecisionID = uuid.New()
		p.ContinuationID = domain.ContinuationID(a.RootRunID, p.DecisionID)
	}} {
		bad := nextProof
		mutate(&bad)
		if _, err := s.Quiesce(t.Context(), a.RootRunID, bad); err == nil {
			t.Fatal("changed continuation proof accepted", bad)
		}
	}
	if _, err := s.Decide(t.Context(), a.Scope.Principal(), a.Scope.ProjectID, a.RootRunID, domain.DecisionInput{RequestID: resumeInput.RequestID, Action: "resume", ExpectedAuthorizationRevision: paused.AuthorizationRevision, CandidateHash: "changed-original-decision-hash"}); err == nil {
		t.Fatal("changed immutable original decision hash accepted")
	}
	current, err := s.CurrentAuthority(t.Context(), a.RootRunID)
	if err != nil || current.SubmissionID != proof.ContinuationID || current.Barrier {
		t.Fatal("recovery changed original submission", current, err)
	}
	decide(t, s, a, "pause", resumed.AuthorizationRevision)
	if _, err = s.Quiesce(t.Context(), a.RootRunID, nextProof); err == nil {
		t.Fatal("old proof reopened a newer decision")
	}
}

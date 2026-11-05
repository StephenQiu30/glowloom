package postgres_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	driver "gorm.io/driver/postgres"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestConversationStopReadSurvivesPendingDeliveryAndRejectsWrongSession(t *testing.T) {
	s, a, _ := fixture(t)
	key := uuid.New()
	decision, err := s.Decide(t.Context(), a.Scope.Principal(), a.Scope.ProjectID, a.RootRunID, domain.DecisionInput{RequestID: key, Action: "stop", ExpectedAuthorizationRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	status, err := s.ReadControlStatus(t.Context(), a.Scope.Principal(), a.Scope.ProjectID, a.SessionID, a.RootRunID, &key)
	if err != nil || status.DeliveryStatus != "pending" || status.ApplicationReceipt != nil || status.CurrentDecision.DecisionID != decision.DecisionID || status.RequestDecision.DecisionID != decision.DecisionID {
		t.Fatalf("pending status=%+v err=%v", status, err)
	}
	receipt := domain.AppliedReceipt{ReceiptID: uuid.New(), DecisionID: decision.DecisionID, AuthorizationRevision: decision.AuthorizationRevision, Status: "applied", ExecutionRevision: 3}
	if err = s.Acknowledge(t.Context(), a.RootRunID, receipt); err != nil {
		t.Fatal(err)
	}
	status, err = s.ReadControlStatus(t.Context(), a.Scope.Principal(), a.Scope.ProjectID, a.SessionID, a.RootRunID, &key)
	if err != nil || status.DeliveryStatus != "applied" || status.ApplicationReceipt == nil || status.ApplicationReceipt.ExecutionRevision != 3 {
		t.Fatalf("applied status=%+v err=%v", status, err)
	}
	if _, err = s.ReadControlStatus(t.Context(), a.Scope.Principal(), a.Scope.ProjectID, uuid.New(), a.RootRunID, nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("wrong session control exposed", err)
	}
}

func TestConversationUnresolvedReceiptBlocksNewRunButAllowsOriginalAdmission(t *testing.T) {
	s, a, g := fixture(t)
	raw := []byte(`{"prompt":"original"}`)
	call := domain.CallInput{Grant: g, Call: domain.CallIdentity{TaskID: "task", CallID: "call", CheckpointID: "checkpoint", Attempt: 1, ContractVersion: "v1", InputHash: domain.Hash(raw)}, Input: raw, MaxMicros: 10}
	_, err := s.ExecuteCall(t.Context(), "model", "text", call, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		return domain.CallResult{Status: "unknown", Result: []byte(`{}`)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ids, readErr := s.UnresolvedConversationRuns(t.Context(), a.Scope.Principal(), a.Scope.ProjectID, a.SessionID)
	if readErr != nil || len(ids) != 1 || ids[0] != a.RootRunID {
		t.Fatalf("unresolved authority missing: %v %v", ids, readErr)
	}
	stranger := a.Scope.Principal()
	stranger.ID = uuid.New()
	ids, readErr = s.UnresolvedConversationRuns(t.Context(), stranger, a.Scope.ProjectID, a.SessionID)
	if readErr != nil || len(ids) != 0 {
		t.Fatalf("another actor read unresolved runs: %v %v", ids, readErr)
	}
	next := a
	next.RootRunID = uuid.New()
	next.RequestID = uuid.New()
	next.SubmissionID = uuid.New()
	if err = s.AdmitRun(t.Context(), next); !errors.Is(err, domain.ErrBlocked) {
		t.Fatalf("new run admitted over unknown original receipt: %v", err)
	}
	if err = s.AdmitRun(t.Context(), a); err != nil {
		t.Fatal("original idempotent admission blocked", err)
	}
}

type access struct{}

func (access) AuthorizeRequest(context.Context, identity.Principal, uuid.UUID, bool) error {
	return nil
}
func (access) AuthorizeWorker(context.Context, identity.Principal, uuid.UUID) error { return nil }
func fixture(t *testing.T) (*pg.Store, domain.Admission, domain.Grant) {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_AGENT_BUSINESS_DB_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(u.Path, "_test") || u.Hostname() != "127.0.0.1" {
		t.Fatal("isolated loopback *_test required")
	}
	db, err := gorm.Open(driver.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	s := pg.NewStore(db, func(*gorm.DB) app.ControlAccess { return access{} }, time.Now)
	a := domain.Admission{RootRunID: uuid.New(), SessionID: uuid.New(), SubmissionID: uuid.New(), RequestID: uuid.New(), ContentHash: domain.Hash([]byte("input")), Scope: domain.Scope{ActorID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New(), ProjectID: uuid.New()}, BudgetMicros: 100}
	if err = s.AdmitRun(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	g, err := s.RegisterGrant(t.Context(), domain.GrantRequest{RootRunID: a.RootRunID, SubmissionID: a.SubmissionID, OwnerEpoch: 1, AuthorizationRevision: 1, LeaseUntil: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	return s, a, g
}

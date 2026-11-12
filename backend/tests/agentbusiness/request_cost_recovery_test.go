package agentbusiness_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
)

func costCall(g domain.Grant, maximum int64) domain.CallInput {
	raw := json.RawMessage(`{"prompt":"one shot"}`)
	return domain.CallInput{Grant: g, Call: domain.CallIdentity{TaskID: "original-task", CallID: "original-call", CheckpointID: "original-checkpoint", Attempt: 1, ContractVersion: "v1", InputHash: domain.Hash(raw)}, Input: raw, MaxMicros: maximum}
}

func TestCallPersistsActualOverrunCostAndBlocksNewPaidAdmission(t *testing.T) {
	s, a, g := fixture(t)
	in := costCall(g, 100)
	sends := 0
	execute := func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		sends++
		return domain.CallResult{Status: "completed", Result: json.RawMessage(`{"provider_receipt":"original"}`), CostMicros: 125}, nil
	}
	out, err := s.ExecuteCall(t.Context(), "model", "model", in, execute)
	if err != nil || out.Status != "completed" || out.CostMicros != 125 {
		t.Fatalf("actual provider cost was lost: %+v %v", out, err)
	}
	replay, err := s.ExecuteCall(t.Context(), "model", "model", in, execute)
	if err != nil || replay.ReceiptID != out.ReceiptID || replay.CostMicros != 125 || sends != 1 {
		t.Fatalf("replay changed original settlement: %+v sends=%d %v", replay, sends, err)
	}
	authority, err := s.ReadAuthority(t.Context(), a.Scope.Principal(), a.Scope.ProjectID, a.RootRunID)
	if err != nil || authority.ReservedMicros != 125 {
		t.Fatalf("actual charge missing from root: %+v %v", authority, err)
	}
	fresh, err := s.ValidateGrant(t.Context(), g)
	if err != nil || fresh.AvailableMicros != 0 {
		t.Fatalf("overrun must expose zero spending capacity: %+v %v", fresh, err)
	}
	next := in
	next.Call.CallID = "another-call"
	next.MaxMicros = 1
	if _, err = s.ExecuteCall(t.Context(), "model", "model", next, execute); !errors.Is(err, domain.ErrBudget) || sends != 1 {
		t.Fatalf("overrun admitted another paid send: %v sends=%d", err, sends)
	}
	next.MaxMicros = 0
	if _, err = s.ExecuteCall(t.Context(), "model", "model", next, execute); !errors.Is(err, domain.ErrBudget) {
		t.Fatalf("zero maximum must not bypass paid admission: %v", err)
	}
	next.Call.CallID = "free-recovery"
	free := func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		return domain.CallResult{Status: "completed", Result: json.RawMessage(`{}`)}, nil
	}
	if _, err = s.ExecuteCall(t.Context(), "tool", "read", next, free); err != nil {
		t.Fatalf("free original recovery blocked by actual overrun: %v", err)
	}
	next.Call.CallID = "free-atomic-read"
	if _, err = s.ExecuteAtomicTool(t.Context(), "read", next, func(*gorm.DB, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		return domain.CallResult{Status: "completed", Result: json.RawMessage(`{}`)}, nil
	}); err != nil {
		t.Fatalf("free atomic recovery blocked: %v", err)
	}
	raw := json.RawMessage(`{"kind":"local-delivery"}`)
	if _, err = s.Propose(t.Context(), domain.ProposalInput{Grant: g, RequestID: uuid.New(), CandidateHash: domain.Hash(raw), Candidate: raw, PriceVersion: "local", BudgetMicros: 0}); err != nil {
		t.Fatalf("free adoption proposal blocked: %v", err)
	}
}

func TestRecoveredOverrunSettlesOnce(t *testing.T) {
	s, a, g := fixture(t)
	in := costCall(g, 50)
	first, err := s.ExecuteCall(t.Context(), "model", "model", in, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		return domain.CallResult{}, errors.New("response lost after acceptance")
	})
	if err != nil || first.Status != "unknown" {
		t.Fatal(first, err)
	}
	db, err := gorm.Open(postgres.Open(os.Getenv("GLOWLOOM_TEST_AGENT_BUSINESS_DB_DSN")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	t.Cleanup(func() { _ = conn.Close() })
	actual := domain.CallResult{Status: "completed", Result: json.RawMessage(`{"provider_receipt":"recovered"}`), CostMicros: 70}
	for range 2 {
		err = db.Transaction(func(tx *gorm.DB) error { _, e := pg.CompleteCall(tx, first.ReceiptID, actual); return e })
		if err != nil {
			t.Fatalf("recover original actual bill: %v", err)
		}
	}
	authority, err := s.ReadAuthority(t.Context(), a.Scope.Principal(), a.Scope.ProjectID, a.RootRunID)
	if err != nil || authority.ReservedMicros != 70 {
		t.Fatalf("recovery settled twice or discarded charge: %+v %v", authority, err)
	}
	saved, err := s.ReadCall(t.Context(), a.RootRunID, g.SubmissionID, in.Call)
	if err != nil || saved.CostMicros != 70 || saved.Status != "completed" {
		t.Fatal(saved, err)
	}
}

func TestReadCallRejectsDifferentInputHash(t *testing.T) {
	s, a, g := fixture(t)
	in := costCall(g, 1)
	if _, err := s.ExecuteCall(t.Context(), "model", "model", in, func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		return domain.CallResult{Status: "completed", Result: json.RawMessage(`{}`)}, nil
	}); err != nil {
		t.Fatal(err)
	}
	changed := in.Call
	changed.InputHash = domain.Hash([]byte(`{"prompt":"different"}`))
	if _, err := s.ReadCall(t.Context(), a.RootRunID, g.SubmissionID, changed); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("original identity with changed input was accepted: %v", err)
	}
}

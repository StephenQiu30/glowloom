package agentbusiness_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	gatewayapp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	agentapp "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
)

func costCall(g domain.Grant, maximum int64) domain.CallInput {
	raw := json.RawMessage(`{"prompt":"one shot"}`)
	return domain.CallInput{Grant: g, Call: domain.CallIdentity{TaskID: "original-task", CallID: "original-call", CheckpointID: "original-checkpoint", Attempt: 1, ContractVersion: "v1", InputHash: domain.Hash(raw)}, Input: raw, MaxMicros: maximum}
}

func TestCallPersistsActualOverrunCostAndBlocksNewPaidAdmission(t *testing.T) {
	_, s, a, g, ledger := fundedCostFixture(t)
	in := costCall(g, 100*creditMicros)
	sends := 0
	execute := func(_ context.Context, _ domain.Scope, id uuid.UUID) (domain.CallResult, error) {
		sends++
		result := ledger.settle(t, ledger.admit(t, id, 100), 125)
		require.EqualValues(t, 125, result.Receipt.PurchaseMicros)
		require.EqualValues(t, 100, result.Charged)
		uncapped, err := ledger.model.Price.Cost(result.Receipt.Usage)
		require.NoError(t, err)
		require.EqualValues(t, 125*creditMicros, uncapped)
		return domain.CallResult{Status: "completed", Result: json.RawMessage(`{"provider_receipt":"original"}`), CostMicros: result.Charged * creditMicros}, nil
	}
	out, err := s.ExecuteCall(t.Context(), "model", "model", in, execute)
	if err != nil || out.Status != "completed" || out.CostMicros != 100*creditMicros {
		t.Fatalf("capped customer charge was lost: %+v %v", out, err)
	}
	replay, err := s.ExecuteCall(t.Context(), "model", "model", in, execute)
	if err != nil || replay.ReceiptID != out.ReceiptID || replay.CostMicros != 100*creditMicros || sends != 1 {
		t.Fatalf("replay changed original settlement: %+v sends=%d %v", replay, sends, err)
	}
	authority, err := s.ReadAuthority(t.Context(), agentapp.Principal(a.Scope), a.Scope.ProjectID, a.RootRunID)
	if err != nil || authority.ReservedMicros != 100*creditMicros {
		t.Fatalf("capped charge missing from root: %+v %v", authority, err)
	}
	fresh, err := s.ValidateGrant(t.Context(), g)
	if err != nil || fresh.AvailableMicros != 0 {
		t.Fatalf("exhausted budget must expose zero spending capacity: %+v %v", fresh, err)
	}
	next := in
	next.Call.CallID = "another-call"
	next.MaxMicros = 1
	if _, err = s.ExecuteCall(t.Context(), "model", "model", next, execute); !errors.Is(err, domain.ErrBudget) || sends != 1 {
		t.Fatalf("overrun admitted another paid send: %v sends=%d", err, sends)
	}
	next.MaxMicros = 0
	if _, err = s.ExecuteCall(t.Context(), "model", "model", next, execute); !errors.Is(err, domain.ErrBudget) || sends != 1 {
		t.Fatalf("zero maximum must not bypass paid admission: %v", err)
	}
	next.Call.CallID = "free-recovery"
	free := func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error) {
		return domain.CallResult{Status: "completed", Result: json.RawMessage(`{}`)}, nil
	}
	if _, err = s.ExecuteCall(t.Context(), "tool", "read", next, free); err != nil {
		t.Fatalf("free original recovery blocked by exhausted budget: %v", err)
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
	db, s, a, g, ledger := fundedCostFixture(t)
	in := costCall(g, 50*creditMicros)
	var dispatch gatewayapp.Dispatch
	first, err := s.ExecuteCall(t.Context(), "model", "model", in, func(_ context.Context, _ domain.Scope, id uuid.UUID) (domain.CallResult, error) {
		dispatch = ledger.admit(t, id, 50)
		require.NoError(t, ledger.store.Unknown(t.Context(), id, dispatch.Attempt))
		return domain.CallResult{}, errors.New("response lost after acceptance")
	})
	if err != nil || first.Status != "unknown" {
		t.Fatal(first, err)
	}
	result := ledger.settle(t, dispatch, 70)
	require.EqualValues(t, 70, result.Receipt.PurchaseMicros)
	actual := domain.CallResult{Status: "completed", Result: json.RawMessage(`{"provider_receipt":"recovered"}`), CostMicros: result.Charged * creditMicros}
	for range 2 {
		settled, e := ledger.store.Settle(t.Context(), dispatch.Call.ID)
		require.NoError(t, e)
		require.Equal(t, result, settled)
		err = db.Transaction(func(tx *gorm.DB) error { _, e := pg.CompleteCall(tx, first.ReceiptID, actual); return e })
		if err != nil {
			t.Fatalf("recover original capped bill: %v", err)
		}
	}
	authority, err := s.ReadAuthority(t.Context(), agentapp.Principal(a.Scope), a.Scope.ProjectID, a.RootRunID)
	if err != nil || authority.ReservedMicros != 50*creditMicros {
		t.Fatalf("recovery settled twice or discarded charge: %+v %v", authority, err)
	}
	var settlements int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM ai.money_event WHERE request_id=? AND kind='settle'`, dispatch.Call.ID).Scan(&settlements).Error)
	require.EqualValues(t, 1, settlements)
	saved, err := s.ReadCall(t.Context(), a.RootRunID, g.SubmissionID, in.Call)
	if err != nil || saved.CostMicros != 50*creditMicros || saved.Status != "completed" {
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

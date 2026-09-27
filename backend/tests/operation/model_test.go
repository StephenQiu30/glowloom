package operation_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func validQuotedOperation() domain.Operation {
	created := time.Date(2026, time.September, 27, 12, 0, 0, 0, time.UTC)
	expires := created.Add(15 * time.Minute)
	quote := int64(1_000_000)
	model := uuid.New()
	price := uuid.New()
	region := "domestic"
	return domain.Operation{
		ID: uuid.New(), ProjectID: uuid.New(), TargetType: "shot_take",
		TargetID: uuidPtr(uuid.New()), TargetVersionNo: int32Ptr(2),
		Capability: "video.generate", Mode: "text_to_video",
		ModelProfileVersionID: &model, PriceRuleVersionID: &price,
		Params: json.RawMessage(`{"duration":10}`), OutputCount: 1,
		InputHash: "sha256:valid", Origin: "canvas", Status: domain.StatusQuoted,
		QuoteMicros: &quote, QuoteDetail: json.RawMessage(`{"unit":"per_second"}`),
		QuoteExpiresAt: &expires, Region: &region, CreateTime: created,
	}
}

func uuidPtr(value uuid.UUID) *uuid.UUID { return &value }
func int32Ptr(value int32) *int32        { return &value }

func TestOperationQuoteRequiresFrozenValidFacts(t *testing.T) {
	valid := validQuotedOperation()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid quote: %v", err)
	}
	tests := []struct {
		name   string
		change func(*domain.Operation)
	}{
		{"no project", func(o *domain.Operation) { o.ProjectID = uuid.Nil }},
		{"no hash", func(o *domain.Operation) { o.InputHash = "" }},
		{"invalid params", func(o *domain.Operation) { o.Params = json.RawMessage(`[]`) }},
		{"negative quote", func(o *domain.Operation) { value := int64(-1); o.QuoteMicros = &value }},
		{"no quote", func(o *domain.Operation) { o.QuoteMicros = nil }},
		{"no expiry", func(o *domain.Operation) { o.QuoteExpiresAt = nil }},
		{"past expiry", func(o *domain.Operation) { value := o.CreateTime.Add(-time.Second); o.QuoteExpiresAt = &value }},
		{"no model snapshot", func(o *domain.Operation) { o.ModelProfileVersionID = nil }},
		{"no price snapshot", func(o *domain.Operation) { o.PriceRuleVersionID = nil }},
		{"too many outputs", func(o *domain.Operation) { o.OutputCount = 9 }},
		{"wrong region", func(o *domain.Operation) { region := "invalid"; o.Region = &region }},
		{"reused and regenerated", func(o *domain.Operation) { o.ReusedFromID = uuidPtr(uuid.New()); o.ForceRegenerate = true }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			operation := valid
			tc.change(&operation)
			if err := operation.Validate(); !errors.Is(err, domain.ErrInvalidOperation) {
				t.Fatalf("Validate = %v, want ErrInvalidOperation", err)
			}
		})
	}
}

func TestOperationQuoteExpiryAndStateGate(t *testing.T) {
	operation := validQuotedOperation()
	if err := operation.CanConfirmAt(operation.QuoteExpiresAt.Add(-time.Nanosecond)); err != nil {
		t.Fatalf("still valid: %v", err)
	}
	if err := operation.CanConfirmAt(*operation.QuoteExpiresAt); !errors.Is(err, domain.ErrQuoteExpired) {
		t.Fatalf("at expiry = %v", err)
	}
	operation.Status = domain.StatusConfirmed
	if err := operation.CanConfirmAt(operation.CreateTime); !errors.Is(err, domain.ErrQuoteNotConfirmable) {
		t.Fatalf("confirmed = %v", err)
	}
	operation = validQuotedOperation()
	operation.Origin = "upload"
	operation.QuoteExpiresAt = nil
	if err := operation.CanConfirmAt(operation.CreateTime); !errors.Is(err, domain.ErrInvalidOperation) {
		t.Fatalf("quoted upload without expiry = %v", err)
	}
	operation.QuoteExpiresAt = validQuotedOperation().QuoteExpiresAt
	if err := operation.CanConfirmAt(operation.CreateTime); !errors.Is(err, domain.ErrQuoteNotConfirmable) {
		t.Fatalf("nonpaid upload confirmation = %v", err)
	}
}

func TestBatchCountsAndTransitions(t *testing.T) {
	batch := domain.Batch{
		ID: uuid.New(), ProjectID: uuid.New(), Kind: "video", Scope: json.RawMessage(`{"episode_id":"example"}`),
		Status: domain.BatchStatusQuoted, TotalCount: 2, QuoteTotalMicros: 100,
	}
	if err := batch.Validate(); err != nil {
		t.Fatalf("valid batch: %v", err)
	}
	if err := batch.Status.CanTransitionTo(domain.BatchStatusConfirmed); err != nil {
		t.Fatalf("quoted -> confirmed: %v", err)
	}
	if err := domain.BatchStatusConfirmed.CanTransitionTo(domain.BatchStatusRunning); err != nil {
		t.Fatalf("confirmed -> running: %v", err)
	}
	if err := domain.BatchStatusRunning.CanTransitionTo(domain.BatchStatusFinished); err != nil {
		t.Fatalf("running -> finished: %v", err)
	}
	if err := domain.BatchStatusExpired.CanTransitionTo(domain.BatchStatusRunning); !errors.Is(err, domain.ErrIllegalBatchTransition) {
		t.Fatalf("expired -> running = %v", err)
	}
	batch.SucceededCount, batch.FailedCount, batch.UnknownCount = 1, 1, 1
	if err := batch.Validate(); !errors.Is(err, domain.ErrInvalidBatch) {
		t.Fatalf("overcount = %v", err)
	}
	batch = domain.Batch{ID: uuid.New(), ProjectID: uuid.New(), Kind: "arbitrary", Scope: json.RawMessage(`{}`), Status: domain.BatchStatusQuoted, TotalCount: 1}
	if err := batch.Validate(); !errors.Is(err, domain.ErrInvalidBatch) {
		t.Fatalf("unsupported kind = %v", err)
	}
}

func TestAgentSessionQuoteHasNoSingleModelSnapshot(t *testing.T) {
	operation := validQuotedOperation()
	operation.TargetType = "agent_session"
	operation.ModelProfileVersionID = nil
	operation.PriceRuleVersionID = nil
	if err := operation.Validate(); err != nil {
		t.Fatalf("agent session quota: %v", err)
	}
	operation.PriceRuleVersionID = uuidPtr(uuid.New())
	if err := operation.Validate(); !errors.Is(err, domain.ErrInvalidOperation) {
		t.Fatalf("agent session single price rule = %v", err)
	}
}

func TestOperationInputFreezesReference(t *testing.T) {
	value := "a prompt"
	input := domain.OperationInput{
		ID: uuid.New(), OperationID: uuid.New(), SeqNo: 1,
		Role: "prompt", RefType: "text", TextValue: &value,
	}
	if err := input.Validate(); err != nil {
		t.Fatalf("valid input: %v", err)
	}
	input.TextValue = nil
	if err := input.Validate(); !errors.Is(err, domain.ErrInvalidOperationInput) {
		t.Fatalf("missing prompt = %v", err)
	}
	input.TextValue = &value
	input.OperationID = uuid.Nil
	if err := input.Validate(); !errors.Is(err, domain.ErrInvalidOperationInput) {
		t.Fatalf("missing parent = %v", err)
	}
}

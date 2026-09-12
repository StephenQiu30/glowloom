package generation_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/google/uuid"
)

func referenceReceiptFixture(t *testing.T) (domain.ReferenceCallState, domain.ReferenceCallReceiptInput) {
	t.Helper()
	hash := strings.Repeat("a", 64)
	_, calls, err := domain.BuildReferenceProviderJob(domain.GenerationRevisionRef{ID: uuid.NewString(), Revision: 1, ContentHash: hash}, []domain.ReferenceProviderCallInput{{SlotKey: "front", CompiledRequestHash: hash}})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := domain.NewReferenceCallState(calls[0].CallKey)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	state, _, err := domain.ClaimReferenceCall(pending, domain.ReferenceCallDispatch{SubmissionToken: uuid.NewString(), DispatchedBy: uuid.NewString(), MembershipTokenVersion: 1, DispatchedAt: now, DeadlineAt: now.Add(180 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	input := domain.ReferenceCallReceiptInput{WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(), Call: calls[0], SubmissionToken: state.Dispatch.SubmissionToken, Slot: referenceOutputSlots([]string{"front"})[0], ObservedAt: now.Add(time.Second), Disposition: "staged", Usage: domain.ProviderUsageObservation{ImageCount: 1}}
	input.Output = &domain.ProviderOutput{OutputKey: "image", StagingObjectKey: "staging/reference/" + input.WorkspaceID + "/" + input.ProjectID + "/" + input.Call.ExecutionRef.ID + "/" + input.Call.CallKey + "/" + input.SubmissionToken + "/image.png", SHA256: hash, MediaType: "image/png", Bytes: 100, Width: input.Slot.MinWidth, Height: input.Slot.MinHeight}
	return state, input
}

func TestReferenceCallReceiptIsAtomicImmutableAndNeverReauthorizes(t *testing.T) {
	for _, disposition := range []string{"staged", "output_rejected", "outcome_unknown", "not_sent"} {
		t.Run(disposition, func(t *testing.T) {
			before, input := referenceReceiptFixture(t)
			input.Disposition = disposition
			expected := domain.ProviderCallSucceeded
			if disposition != "staged" {
				input.Output, input.Usage = nil, domain.ProviderUsageObservation{}
				expected = domain.ProviderCallFailed
				switch disposition {
				case "output_rejected":
					input.ReasonCode = "invalid_png_contents"
				case "outcome_unknown":
					input.ReasonCode = "transport_failed"
					expected = domain.ProviderCallOutcomeUnknown
				case "not_sent":
					input.ReasonCode = "submit_not_attempted"
				}
			}
			receipt, err := domain.BuildReferenceCallReceipt(input)
			if err != nil {
				t.Fatal(err)
			}
			after, changed, err := domain.RecordReferenceCallReceipt(before, receipt)
			if err != nil || !changed || after.Status != expected || after.Revision != 3 || after.Receipt == nil || before.Receipt != nil {
				t.Fatalf("receipt transition: %+v %v", after, err)
			}
			if err := domain.ValidateReferenceCallTransition(before, after); err != nil {
				t.Fatal(err)
			}
			repeated, changed, err := domain.RecordReferenceCallReceipt(after, receipt)
			if err != nil || changed || !reflect.DeepEqual(repeated, after) {
				t.Fatal("receipt replay was not idempotent")
			}
			input.ObservedAt = input.ObservedAt.Add(time.Second)
			different, err := domain.BuildReferenceCallReceipt(input)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := domain.RecordReferenceCallReceipt(after, different); err == nil {
				t.Fatal("receipt was overwritten")
			}
			if _, send, err := domain.ClaimReferenceCall(after, *before.Dispatch); err != nil || send {
				t.Fatal("observed call regained send right")
			}
			if expired, changed, err := domain.ExpireReferenceCall(after, before.Dispatch.DeadlineAt); err != nil || changed || !reflect.DeepEqual(expired, after) {
				t.Fatal("expiry changed observed call")
			}
			raw, _ := json.Marshal(after)
			if decoded, err := domain.DecodeReferenceCallState(raw); err != nil || !reflect.DeepEqual(decoded, after) {
				t.Fatalf("state roundtrip: %v", err)
			}
			bad := strings.Replace(string(raw), receipt.ContentHash, strings.Repeat("f", 64), 1)
			if _, err := domain.DecodeReferenceCallState([]byte(bad)); err == nil {
				t.Fatal("corrupt receipt accepted")
			}
		})
	}
}

func TestReferenceCallReceiptReconcilesExpiryWithoutAnotherSubmit(t *testing.T) {
	before, input := referenceReceiptFixture(t)
	receipt, err := domain.BuildReferenceCallReceipt(input)
	if err != nil {
		t.Fatal(err)
	}
	expired, _, err := domain.ExpireReferenceCall(before, before.Dispatch.DeadlineAt)
	if err != nil {
		t.Fatal(err)
	}
	after, changed, err := domain.RecordReferenceCallReceipt(expired, receipt)
	if err != nil || !changed || after.Revision != 4 || after.Status != domain.ProviderCallSucceeded || after.OutcomeUnknownAt != nil {
		t.Fatalf("expiry race: %+v %v", after, err)
	}
	if err := domain.ValidateReferenceCallTransition(expired, after); err != nil {
		t.Fatal(err)
	}
	input.ObservedAt = before.Dispatch.DeadlineAt.Add(time.Second)
	late, err := domain.BuildReferenceCallReceipt(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := domain.RecordReferenceCallReceipt(expired, late); err == nil {
		t.Fatal("late staging declared timely success")
	}
}

func TestReferenceCallReceiptRejectsForeignUnsafeAndLeakingEvidence(t *testing.T) {
	for _, fault := range []string{"scope", "call", "token", "slot", "reason", "path", "bytes", "dimensions", "digest", "usage", "missing-output", "timestamp"} {
		t.Run(fault, func(t *testing.T) {
			before, input := referenceReceiptFixture(t)
			switch fault {
			case "scope":
				input.WorkspaceID = "invalid"
			case "call":
				input.Call.CallKey = strings.Repeat("f", 64)
			case "token":
				input.SubmissionToken = uuid.NewString()
			case "slot":
				input.Slot.SlotKey = "back"
			case "reason":
				input.ReasonCode = "raw-secret-response"
			case "path":
				input.Output.StagingObjectKey = "https://untrusted.invalid/image.png"
			case "bytes":
				input.Output.Bytes = input.Slot.MaxBytes + 1
			case "dimensions":
				input.Output.Width++
			case "digest":
				input.Output.SHA256 = "invalid"
			case "usage":
				input.Usage.InputTokens = -1
			case "missing-output":
				input.Output = nil
			case "timestamp":
				input.ObservedAt = before.Dispatch.DispatchedAt.Add(-time.Second)
			}
			receipt, err := domain.BuildReferenceCallReceipt(input)
			if err == nil {
				_, _, err = domain.RecordReferenceCallReceipt(before, receipt)
			}
			if err == nil {
				t.Fatalf("unsafe %s receipt accepted", fault)
			}
		})
	}
}

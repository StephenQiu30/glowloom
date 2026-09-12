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

func TestReferenceCallStateHasOneSendBoundaryAndNeverResets(t *testing.T) {
	pending, err := domain.NewReferenceCallState(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 13, 1, 2, 3, 123456789, time.FixedZone("offset", 3600))
	input := domain.ReferenceCallDispatch{SubmissionToken: uuid.NewString(), DispatchedBy: uuid.NewString(), MembershipTokenVersion: 1, DispatchedAt: now, DeadlineAt: now.Add(180 * time.Second)}
	claimed, send, err := domain.ClaimReferenceCall(pending, input)
	if err != nil || !send || claimed.Status != domain.ProviderCallDispatching || claimed.Revision != 2 {
		t.Fatalf("claim: %v", err)
	}
	if !claimed.Dispatch.DispatchedAt.Equal(now.UTC().Truncate(time.Microsecond)) {
		t.Fatal("noncanonical dispatch time")
	}
	if pending.Dispatch != nil {
		t.Fatal("claim mutated its input")
	}
	copyInput := input
	copyInput.SubmissionToken = uuid.NewString()
	if repeated, send, err := domain.ClaimReferenceCall(claimed, copyInput); err != nil || send || !reflect.DeepEqual(repeated, claimed) {
		t.Fatalf("repeated dispatch: %v", err)
	}
	if early, changed, err := domain.ExpireReferenceCall(claimed, claimed.Dispatch.DeadlineAt.Add(-time.Microsecond)); err != nil || changed || !reflect.DeepEqual(early, claimed) {
		t.Fatal("premature expiry")
	}
	unknown, changed, err := domain.ExpireReferenceCall(claimed, claimed.Dispatch.DeadlineAt)
	if err != nil || !changed || unknown.Status != domain.ProviderCallOutcomeUnknown || unknown.Revision != 3 {
		t.Fatalf("expire: %v", err)
	}
	if _, send, err := domain.ClaimReferenceCall(unknown, copyInput); err != nil || send {
		t.Fatal("unknown call reacquired send right")
	}
	if repeated, changed, err := domain.ExpireReferenceCall(unknown, now.Add(time.Hour)); err != nil || changed || !reflect.DeepEqual(repeated, unknown) {
		t.Fatal("expiry was not idempotent")
	}
	if _, _, err := domain.ExpireReferenceCall(pending, now); err == nil {
		t.Fatal("pending call expired as a sent request")
	}
	for _, state := range []domain.ReferenceCallState{pending, claimed, unknown} {
		raw, _ := json.Marshal(state)
		decoded, err := domain.DecodeReferenceCallState(raw)
		if err != nil || !reflect.DeepEqual(state, decoded) {
			t.Fatalf("state roundtrip: %v", err)
		}
		for _, bad := range []string{strings.Replace(string(raw), state.ContentHash, strings.Repeat("f", 64), 1), strings.TrimSuffix(string(raw), "}") + `,"revision":1}`, strings.TrimSuffix(string(raw), "}") + `,"should_dispatch":true}`} {
			if _, err := domain.DecodeReferenceCallState([]byte(bad)); err == nil {
				t.Fatal("corrupt state accepted")
			}
		}
	}
	for _, fault := range []string{"token", "actor", "membership", "deadline", "time"} {
		bad := input
		switch fault {
		case "token":
			bad.SubmissionToken = "invalid"
		case "actor":
			bad.DispatchedBy = "invalid"
		case "membership":
			bad.MembershipTokenVersion = 0
		case "deadline":
			bad.DeadlineAt = bad.DispatchedAt
		case "time":
			bad.DispatchedAt = time.Time{}
		}
		if got, send, err := domain.ClaimReferenceCall(pending, bad); err == nil || send || !reflect.DeepEqual(got, domain.ReferenceCallState{}) {
			t.Fatalf("invalid %s claim accepted", fault)
		}
	}
}

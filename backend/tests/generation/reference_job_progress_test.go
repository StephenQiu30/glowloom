package generation_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/google/uuid"
)

func referenceProgressFixture(t *testing.T, statuses []string) (domain.ReferenceProviderJob, []domain.ReferenceProviderCall, []domain.ReferenceCallState) {
	t.Helper()
	ref := domain.GenerationRevisionRef{ID: uuid.NewString(), Revision: 1, ContentHash: strings.Repeat("a", 64)}
	inputs := []domain.ReferenceProviderCallInput{}
	for _, slot := range []string{"back", "front", "profile"} {
		inputs = append(inputs, domain.ReferenceProviderCallInput{SlotKey: slot, CompiledRequestHash: strings.Repeat("b", 64)})
	}
	job, calls, err := domain.BuildReferenceProviderJob(ref, inputs)
	if err != nil {
		t.Fatal(err)
	}
	states := make([]domain.ReferenceCallState, len(calls))
	workspace, project := uuid.NewString(), uuid.NewString()
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	for i, call := range calls {
		state, err := domain.NewReferenceCallState(call.CallKey)
		if err != nil {
			t.Fatal(err)
		}
		if statuses[i] != domain.ProviderCallPending {
			state, _, err = domain.ClaimReferenceCall(state, domain.ReferenceCallDispatch{SubmissionToken: uuid.NewString(), DispatchedBy: uuid.NewString(), MembershipTokenVersion: 1, DispatchedAt: now, DeadlineAt: now.Add(time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
		}
		if statuses[i] == domain.ProviderCallOutcomeUnknown {
			state, _, err = domain.ExpireReferenceCall(state, now.Add(time.Minute))
		} else if statuses[i] == domain.ProviderCallSucceeded || statuses[i] == domain.ProviderCallFailed {
			_, input := referenceReceiptFixture(t)
			input.WorkspaceID, input.ProjectID, input.Call, input.SubmissionToken, input.ObservedAt = workspace, project, call, state.Dispatch.SubmissionToken, now.Add(time.Second)
			input.Slot.SlotKey, input.Slot.ViewRole = call.SlotKey, call.SlotKey
			input.Usage = domain.ProviderUsageObservation{}
			input.Output = nil
			input.Disposition, input.ReasonCode = "not_sent", "submit_not_attempted"
			if statuses[i] == domain.ProviderCallSucceeded {
				input.Disposition, input.ReasonCode = "staged", ""
				input.Usage.ImageCount = 1
				input.Output = &domain.ProviderOutput{OutputKey: "image", StagingObjectKey: "staging/reference/" + workspace + "/" + project + "/" + ref.ID + "/" + call.CallKey + "/" + input.SubmissionToken + "/image.png", SHA256: strings.Repeat("c", 64), Bytes: 10, MediaType: "image/png", Width: input.Slot.MinWidth, Height: input.Slot.MinHeight}
			}
			receipt, receiptErr := domain.BuildReferenceCallReceipt(input)
			if receiptErr != nil {
				t.Fatal(receiptErr)
			}
			state, _, err = domain.RecordReferenceCallReceipt(state, receipt)
		}
		if err != nil {
			t.Fatal(err)
		}
		states[i] = state
	}
	return job, calls, states
}

func TestReferenceJobProgressRequiresAllCalls(t *testing.T) {
	for _, example := range []struct {
		name     string
		statuses []string
		want     string
		terminal bool
	}{
		{"pending", []string{"PENDING", "PENDING", "PENDING"}, "PENDING", false},
		{"dispatching", []string{"DISPATCHING", "PENDING", "PENDING"}, "RUNNING", false},
		{"single_success", []string{"SUCCEEDED", "PENDING", "PENDING"}, "RUNNING", false},
		{"all_success", []string{"SUCCEEDED", "SUCCEEDED", "SUCCEEDED"}, "SUCCEEDED", true},
		{"partial", []string{"SUCCEEDED", "FAILED", "FAILED"}, "PARTIAL_SUCCEEDED", true},
		{"all_failed", []string{"FAILED", "FAILED", "FAILED"}, "FAILED", true},
		{"unknown_wins", []string{"SUCCEEDED", "OUTCOME_UNKNOWN", "PENDING"}, "OUTCOME_UNKNOWN", false},
	} {
		t.Run(example.name, func(t *testing.T) {
			job, calls, states := referenceProgressFixture(t, example.statuses)
			got, err := domain.BuildReferenceJobProgress(job, calls, states)
			if err != nil || got.Status != example.want || got.Terminal != example.terminal || got.Total != 3 || len(got.Calls) != 3 {
				t.Fatalf("progress=%+v err=%v", got, err)
			}
			if got.Pending+got.Dispatching+got.Succeeded+got.Failed+got.OutcomeUnknown != got.Total {
				t.Fatal("incomplete counts")
			}
			identity := got
			identity.ContentHash = ""
			rawIdentity, _ := json.Marshal(identity)
			hash, err := canonical.Hash(rawIdentity)
			if err != nil || got.ContentHash != hash || got.JobHash != job.ContentHash || got.CallSetRoot != job.CallSetRoot || got.ExecutionRef != job.ExecutionRef {
				t.Fatal("progress hash lost its complete input identity")
			}
			slices.Reverse(calls)
			slices.Reverse(states)
			repeated, err := domain.BuildReferenceJobProgress(job, calls, states)
			if err != nil || !reflect.DeepEqual(got, repeated) {
				t.Fatalf("unstable order: %v", err)
			}
			raw, _ := json.Marshal(got)
			for _, secret := range []string{"submission_token", "object_key", "receipt", "prompt", "dispatched_by"} {
				if strings.Contains(string(raw), secret) {
					t.Fatalf("exposed %s", secret)
				}
			}
		})
	}
}

func TestReferenceJobProgressKeepsBundlesDistinct(t *testing.T) {
	base, calls, _ := referenceProgressFixture(t, []string{"PENDING", "PENDING", "PENDING"})
	inputs := make([]domain.ReferenceProviderCallInput, 0, 6)
	for bundle := 0; bundle < 2; bundle++ {
		for _, call := range calls {
			input := call.ReferenceProviderCallInput
			input.BundleIndex = bundle
			inputs = append(inputs, input)
		}
	}
	job, calls, err := domain.BuildReferenceProviderJob(base.ExecutionRef, inputs)
	if err != nil {
		t.Fatal(err)
	}
	states := make([]domain.ReferenceCallState, len(calls))
	for i, call := range calls {
		states[i], err = domain.NewReferenceCallState(call.CallKey)
		if err != nil {
			t.Fatal(err)
		}
	}
	progress, err := domain.BuildReferenceJobProgress(job, calls, states)
	if err != nil || progress.Total != 6 {
		t.Fatalf("two bundles: %+v %v", progress, err)
	}
	for i, call := range progress.Calls {
		if call.BundleIndex != i/3 || call.SlotKey != calls[i].SlotKey {
			t.Fatal("mixed bundle slots")
		}
	}
	for i := 3; i < 6; i++ {
		states[i] = states[i-3]
	}
	if _, err := domain.BuildReferenceJobProgress(job, calls, states); err == nil {
		t.Fatal("first bundle reused as second")
	}
}

func TestReferenceJobProgressRejectsIncompleteOrDriftingFacts(t *testing.T) {
	for _, fault := range []string{"missing_call", "extra_call", "duplicate_call", "missing_state", "duplicate_state", "state_hash", "job_hash", "job_order", "foreign_call", "foreign_state"} {
		t.Run(fault, func(t *testing.T) {
			job, calls, states := referenceProgressFixture(t, []string{"PENDING", "PENDING", "PENDING"})
			switch fault {
			case "missing_call":
				calls = calls[:2]
			case "extra_call":
				calls = append(calls, calls[0])
			case "duplicate_call":
				calls[1] = calls[0]
			case "missing_state":
				states = states[:2]
			case "duplicate_state":
				states[1] = states[0]
			case "state_hash":
				states[0].ContentHash = strings.Repeat("f", 64)
			case "job_hash":
				job.ContentHash = strings.Repeat("f", 64)
			case "job_order":
				slices.Reverse(job.CallKeys)
			case "foreign_call":
				calls[0].ExecutionRef.ID = uuid.NewString()
			case "foreign_state":
				states[0], _ = domain.NewReferenceCallState(strings.Repeat("f", 64))
			}
			got, err := domain.BuildReferenceJobProgress(job, calls, states)
			if err == nil || !reflect.DeepEqual(got, domain.ReferenceJobProgress{}) {
				t.Fatalf("accepted %s: %+v", fault, got)
			}
		})
	}
}

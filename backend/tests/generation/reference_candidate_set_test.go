package generation_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/google/uuid"
)

func TestReferenceCandidateSetRequiresEveryReviewedGroup(t *testing.T) {
	facts := referenceBundleFacts(t, false)
	inputs, err := domain.BuildReferenceBundleInputs(facts)
	if err != nil {
		t.Fatal(err)
	}
	var bundles []domain.ReferenceCandidateBundle
	for index := range inputs.Bundles {
		value, err := domain.BuildReferenceCandidateBundle(facts.WorkspaceID, facts.ProjectID, inputs, index, domain.GenerationRevisionRef{ID: uuid.NewString(), Revision: 1, ContentHash: strings.Repeat("b", 64)}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		bundles = append(bundles, value)
	}
	value, err := domain.BuildReferenceCandidateSet(facts, bundles)
	if err != nil || value.GenerationCompletionState != "complete" || len(value.BundleRefs) != 2 || len(value.FailedSlots) != 0 {
		t.Fatalf("complete set: %+v %v", value, err)
	}
	slices.Reverse(bundles)
	again, err := domain.BuildReferenceCandidateSet(facts, bundles)
	if err != nil || !reflect.DeepEqual(again, value) {
		t.Fatal("caller order changed set identity")
	}
	raw, _ := json.Marshal(value)
	if _, err := domain.DecodeReferenceCandidateSet(raw); err != nil {
		t.Fatal(err)
	}
	if _, err := domain.BuildReferenceCandidateSet(facts, bundles[:1]); err == nil {
		t.Fatal("missing review marked complete")
	}
	if _, err := domain.BuildReferenceCandidateSet(facts, append(bundles, bundles[0])); err == nil {
		t.Fatal("duplicate review accepted")
	}
	for _, field := range []string{"generation_completion_state", "expected_bundle_count", "failed_or_unknown_slot_refs"} {
		var wire map[string]any
		_ = json.Unmarshal(raw, &wire)
		delete(wire, field)
		bad, _ := json.Marshal(wire)
		if _, err := domain.DecodeReferenceCandidateSet(bad); err == nil {
			t.Fatalf("missing %s accepted", field)
		}
	}
}

func TestReferenceCandidateSetKeepsTechnicalFailuresAndUnknownSeparate(t *testing.T) {
	facts := referenceBundleFacts(t, true)
	value, err := domain.BuildReferenceCandidateSet(facts, []domain.ReferenceCandidateBundle{})
	if err != nil || value.GenerationCompletionState != "partial_explicit_failure" || len(value.BundleRefs) != 0 || len(value.FailedSlots) != 6 {
		t.Fatalf("technical failure set: %+v %v", value, err)
	}
	facts = referenceBundleFacts(t, false)
	state := facts.States[0]
	pending, _ := domain.NewReferenceCallState(state.CallKey)
	claimed, _, err := domain.ClaimReferenceCall(pending, *state.Dispatch)
	if err != nil {
		t.Fatal(err)
	}
	unknown, _, err := domain.ExpireReferenceCall(claimed, state.Dispatch.DeadlineAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	facts.States[0] = unknown
	value, err = domain.BuildReferenceCandidateSet(facts, []domain.ReferenceCandidateBundle{})
	if err != nil || value.GenerationCompletionState != "outcome_unknown" || len(value.BundleRefs) != 0 || len(value.FailedSlots) != 1 {
		t.Fatalf("unknown set: %+v %v", value, err)
	}
	facts.States[0] = claimed
	if _, err := domain.BuildReferenceCandidateSet(facts, []domain.ReferenceCandidateBundle{}); err == nil {
		t.Fatal("running call marked final")
	}
}

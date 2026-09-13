package generation_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/google/uuid"
)

func TestReferenceCandidateBundleKeepsExactWholeGroup(t *testing.T) {
	facts := referenceBundleFacts(t, false)
	inputs, err := domain.BuildReferenceBundleInputs(facts)
	if err != nil {
		t.Fatal(err)
	}
	review := domain.GenerationRevisionRef{ID: uuid.NewString(), Revision: 1, ContentHash: strings.Repeat("e", 64)}
	at := time.Now().UTC().Truncate(time.Microsecond)
	value, err := domain.BuildReferenceCandidateBundle(facts.WorkspaceID, facts.ProjectID, inputs, 0, review, at)
	if err != nil {
		t.Fatal(err)
	}
	if value.BundleInputRef.ID != inputs.Bundles[0].Input.ID || value.VisionReviewRef != review || value.ExecutionRef != inputs.ExecutionRef {
		t.Fatal("Bundle lost its exact inputs")
	}
	again, err := domain.BuildReferenceCandidateBundle(facts.WorkspaceID, facts.ProjectID, inputs, 0, review, at)
	if err != nil || !reflect.DeepEqual(value, again) {
		t.Fatal("Bundle identity is unstable")
	}
	raw, _ := json.Marshal(value)
	if _, err := domain.DecodeReferenceCandidateBundle(raw); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"hash", "input", "review", "missing_index"} {
		var wire map[string]any
		_ = json.Unmarshal(raw, &wire)
		switch mutation {
		case "hash":
			wire["content_hash"] = strings.Repeat("a", 64)
		case "input":
			wire["bundle_input_ref"] = domain.GenerationActionRef{ID: uuid.NewString(), ContentHash: strings.Repeat("b", 64)}
		case "review":
			wire["bundle_vision_review_candidate_revision_ref"] = domain.GenerationRevisionRef{ID: review.ID, Revision: 2, ContentHash: review.ContentHash}
		case "missing_index":
			delete(wire, "candidate_bundle_index")
		}
		bad, _ := json.Marshal(wire)
		if _, err := domain.DecodeReferenceCandidateBundle(bad); err == nil {
			t.Fatalf("accepted %s", mutation)
		}
	}
	for _, index := range []int{-1, 2} {
		if _, err := domain.BuildReferenceCandidateBundle(facts.WorkspaceID, facts.ProjectID, inputs, index, review, at); err == nil {
			t.Fatal("accepted invalid group")
		}
	}
	forged := value
	forged.GenerationRound = 2
	forged.ContentHash = ""
	material, _ := json.Marshal(forged)
	forged.ContentHash, _ = canonical.Hash(material)
	material, _ = json.Marshal(forged)
	if _, err := domain.DecodeReferenceCandidateBundle(material); err == nil {
		t.Fatal("accepted unsupported round with recomputed hash")
	}
	bad, _ := domain.BuildReferenceBundleInputs(referenceBundleFacts(t, true))
	if _, err := domain.BuildReferenceCandidateBundle(facts.WorkspaceID, facts.ProjectID, bad, 0, review, at); err == nil {
		t.Fatal("accepted technically failed group")
	}
}

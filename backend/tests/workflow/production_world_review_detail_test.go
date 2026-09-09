package workflow_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/google/uuid"

	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestProductionWorldReviewDetailHasSixTypedViews(t *testing.T) {
	draft := productionWorldCandidateDraft(t)
	candidate, _, err := worlddomain.NewProductionWorldCandidate(draft)
	if err != nil {
		t.Fatal(err)
	}
	gate, _, err := workflow.NewProductionWorldGateInput(workflow.ProductionWorldGateInputDraft{
		WorkspaceID: draft.WorkspaceID, ProjectID: draft.ProjectID,
		WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(),
		CandidateRevisionID: uuid.NewString(), CandidateRevision: 1,
		CandidateRevisionHash: "e" + candidate.ContentHash[1:], Candidate: candidate,
		AllowedDecisions: []string{"approved", "rejected"},
		ExpectedHeads:    productionWorldExpectedHeads(candidate, 0, 0),
	})
	if err != nil {
		t.Fatal(err)
	}

	detail, encoded, err := workflow.NewProductionWorldReviewDetail(gate, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if detail.SchemaVersion != workflow.ProductionWorldReviewDetailSchemaVersion ||
		detail.GateKey != workflow.ProductionWorldGateKey || detail.InputHash != gate.InputHash ||
		detail.CandidateRevision != gate.Subject.ProductionWorldCandidate ||
		!slices.EqualFunc(detail.RepairTargets, gate.Subject.RepairTargets, func(left, right workflow.ProductionWorldRepairTargetSet) bool {
			return left.Operation == right.Operation && slices.Equal(left.TargetKeys, right.TargetKeys)
		}) {
		t.Fatalf("review detail identity drifted: %#v", detail)
	}
	if len(detail.Views.CharacterAppearances) != 1 ||
		detail.Views.CharacterAppearances[0].IdentityKey != "character:linzhou" ||
		len(detail.Views.CharacterAppearances[0].States) != 1 ||
		detail.Views.CharacterAppearances[0].States[0].StateKind != "character_appearance" {
		t.Fatalf("character/appearance view = %#v", detail.Views.CharacterAppearances)
	}
	if detail.Views.Locations == nil || detail.Views.PropStates == nil || detail.Views.Interactions == nil ||
		len(detail.Views.SceneOccurrences) != 1 || detail.Views.SceneOccurrences[0].StoryTimeKey != "storytime:00000001" ||
		len(detail.Views.SceneOccurrences[0].Occurrences) != 1 ||
		detail.Views.Continuity.Claims == nil || len(detail.Views.Continuity.Ledger) != 1 {
		t.Fatalf("six review views are incomplete: %#v", detail.Views)
	}
	var wire struct {
		Views map[string]json.RawMessage `json:"views"`
	}
	if err = json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.Views) != 6 {
		t.Fatalf("review detail exposes %d views, want 6", len(wire.Views))
	}
	decoded, canonical, err := workflow.DecodeProductionWorldReviewDetail(encoded)
	if err != nil || decoded.InputHash != detail.InputHash || string(canonical) != string(encoded) {
		t.Fatalf("decode review detail: decoded=%#v err=%v", decoded, err)
	}
}

func TestProductionWorldReviewDetailRejectsCandidateOutsideFrozenGate(t *testing.T) {
	draft := productionWorldCandidateDraft(t)
	candidate, _, err := worlddomain.NewProductionWorldCandidate(draft)
	if err != nil {
		t.Fatal(err)
	}
	gate, _, err := workflow.NewProductionWorldGateInput(workflow.ProductionWorldGateInputDraft{
		WorkspaceID: draft.WorkspaceID, ProjectID: draft.ProjectID,
		WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(),
		CandidateRevisionID: uuid.NewString(), CandidateRevision: 1,
		CandidateRevisionHash: "e" + candidate.ContentHash[1:], Candidate: candidate,
		AllowedDecisions: []string{"approved", "rejected"},
		ExpectedHeads:    productionWorldExpectedHeads(candidate, 0, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate.ContentHash = "f" + candidate.ContentHash[1:]
	if _, _, err = workflow.NewProductionWorldReviewDetail(gate, candidate); err == nil {
		t.Fatal("review detail accepted a Candidate outside the frozen Gate")
	}
}

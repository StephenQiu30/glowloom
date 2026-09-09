package workflow_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestProductionWorldRepairClosureStopsAfterDirectContinuityNeighbour(t *testing.T) {
	views := productionWorldRepairViews()

	closure, err := workflow.NewProductionWorldRepairClosure(
		views,
		workflow.ProductionWorldRepairSelection{
			Operation:  "rebind_scene_occurrence",
			TargetKeys: []string{"occurrence_character_scene_0001"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	assertStringsEqual(t, "Scene scope", closure.SceneScopeKeys, []string{"scene:0001", "scene:0002"})
	assertStringsEqual(t, "entity", closure.EntityKeys, []string{"character:linzhou", "prop:key"})
	assertStringsEqual(t, "state", closure.StateKeys, []string{
		"state_character_linzhou_dry", "state_character_linzhou_wet", "state_prop_key_carried", "state_prop_key_ground",
	})
	assertStringsEqual(t, "Occurrence", closure.OccurrenceKeys, []string{
		"occurrence_character_scene_0001", "occurrence_character_scene_0002", "occurrence_prop_scene_0001",
	})
	assertStringsEqual(t, "Interaction", closure.InteractionKeys, []string{"interaction_pick_up_key"})
	assertStringsEqual(t, "Continuity", closure.ContinuityKeys, []string{"continuity_character_scene_0001_scene_0002"})
	assertStringsEqual(t, "ledger", closure.LedgerKeys, []string{
		"ledger_character_scene_0001", "ledger_character_scene_0002", "ledger_prop_scene_0001",
	})

	for _, forbidden := range []string{
		"scene:0003",
		"occurrence_character_scene_0003",
		"continuity_character_scene_0002_scene_0003",
		"ledger_character_scene_0003",
	} {
		if slices.Contains(closure.AllKeys(), forbidden) {
			t.Fatalf("direct closure leaked into the next continuity edge: %s in %#v", forbidden, closure)
		}
	}
}

func TestProductionWorldChangeRequestMatchesFrozenTargetEvidenceAndClosure(t *testing.T) {
	draft := productionWorldCandidateDraft(t)
	candidate, _, err := worlddomain.NewProductionWorldCandidate(draft)
	if err != nil {
		t.Fatal(err)
	}
	gate, _, err := workflow.NewProductionWorldGateInput(workflow.ProductionWorldGateInputDraft{
		WorkspaceID: draft.WorkspaceID, ProjectID: draft.ProjectID,
		WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(),
		CandidateRevisionID: uuid.NewString(), CandidateRevision: 1,
		CandidateRevisionHash: strings.Repeat("a", 64), Candidate: candidate,
		AllowedDecisions: []string{"approved", "rejected"}, ExpectedHeads: productionWorldExpectedHeads(candidate, 0, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, _, err := workflow.NewProductionWorldReviewDetail(gate, candidate)
	if err != nil {
		t.Fatal(err)
	}
	selection := workflow.ProductionWorldRepairSelection{
		Operation:  workflow.ProductionWorldRepairRebindOccurrence,
		TargetKeys: []string{detail.Views.SceneOccurrences[0].Occurrences[0].OccurrenceKey},
	}
	closure, err := workflow.NewProductionWorldRepairClosure(detail.Views, selection)
	if err != nil {
		t.Fatal(err)
	}
	evidence := detail.Views.SceneOccurrences[0].Occurrences[0].Evidence
	request := workflow.ProductionWorldChangeRequest{
		IssueRefs: []string{},
		EvidenceRefs: []workflow.HumanGateEvidenceRef{{
			SourceVersionID: gate.Subject.SourceVersion.VersionID,
			SourceStart:     evidence.SourceStart, SourceEnd: evidence.SourceEnd, TextHash: evidence.TextHash,
		}},
		ChangeSpec: workflow.ProductionWorldRepairChange{
			Operation: selection.Operation, TargetKeys: selection.TargetKeys, AffectedScopeKeys: closure.AllKeys(),
		},
		ReasonCode: "scene_occurrence_incorrect",
	}
	if err = workflow.ValidateProductionWorldChangeRequest(gate, detail, request); err != nil {
		t.Fatal(err)
	}

	mutations := []func(*workflow.ProductionWorldChangeRequest){
		func(value *workflow.ProductionWorldChangeRequest) {
			value.ChangeSpec.AffectedScopeKeys = append(value.ChangeSpec.AffectedScopeKeys, "scene:outside_frozen_closure")
		},
		func(value *workflow.ProductionWorldChangeRequest) { value.ReasonCode = "rewrite_everything" },
		func(value *workflow.ProductionWorldChangeRequest) { value.EvidenceRefs[0].SourceStart++ },
		func(value *workflow.ProductionWorldChangeRequest) {
			value.ChangeSpec.TargetKeys[0] = "occurrence_missing"
		},
		func(value *workflow.ProductionWorldChangeRequest) {
			value.EvidenceRefs, value.IssueRefs, value.UserNote = []workflow.HumanGateEvidenceRef{}, []string{}, nil
		},
	}
	for index, mutate := range mutations {
		changed := request.Clone()
		mutate(&changed)
		if err = workflow.ValidateProductionWorldChangeRequest(gate, detail, changed); err == nil {
			t.Fatalf("accepted Production World change request mutation %d: %#v", index, changed)
		}
	}
}

func TestProductionWorldRepairClosureRejectsUnknownOrMismatchedTargets(t *testing.T) {
	views := productionWorldRepairViews()
	for _, test := range []workflow.ProductionWorldRepairSelection{
		{Operation: "rebind_scene_occurrence", TargetKeys: []string{"occurrence_missing"}},
		{Operation: "revise_interaction", TargetKeys: []string{"continuity_character_scene_0001_scene_0002"}},
		{Operation: "rewrite_production_world", TargetKeys: []string{"scene:0001"}},
		{Operation: "revise_continuity", TargetKeys: []string{"continuity_character_scene_0001_scene_0002", "continuity_character_scene_0001_scene_0002"}},
	} {
		if _, err := workflow.NewProductionWorldRepairClosure(views, test); err == nil {
			t.Fatalf("accepted invalid repair selection: %#v", test)
		}
	}
}

func TestProductionWorldRepairClosureKeepsOperationSpecificBounds(t *testing.T) {
	views := productionWorldRepairViews()
	for _, test := range []struct {
		name       string
		selection  workflow.ProductionWorldRepairSelection
		scenes     []string
		continuity []string
	}{
		{
			name: "Scene shard", selection: workflow.ProductionWorldRepairSelection{
				Operation: workflow.ProductionWorldRepairRebindOccurrence, TargetKeys: []string{"scene:0001"},
			},
			scenes: []string{"scene:0001", "scene:0002"}, continuity: []string{"continuity_character_scene_0001_scene_0002"},
		},
		{
			name: "Interaction shard", selection: workflow.ProductionWorldRepairSelection{
				Operation: workflow.ProductionWorldRepairReviseInteraction, TargetKeys: []string{"interaction_pick_up_key"},
			},
			scenes: []string{"scene:0001", "scene:0002"}, continuity: []string{"continuity_character_scene_0001_scene_0002"},
		},
		{
			name: "entity shard", selection: workflow.ProductionWorldRepairSelection{
				Operation: workflow.ProductionWorldRepairReviseEntity, TargetKeys: []string{"character:linzhou"},
			},
			scenes:     []string{"scene:0001", "scene:0002", "scene:0003"},
			continuity: []string{"continuity_character_scene_0001_scene_0002", "continuity_character_scene_0002_scene_0003"},
		},
		{
			name: "Continuity shard", selection: workflow.ProductionWorldRepairSelection{
				Operation: workflow.ProductionWorldRepairReviseContinuity, TargetKeys: []string{"continuity_character_scene_0001_scene_0002"},
			},
			scenes: []string{"scene:0001", "scene:0002"}, continuity: []string{"continuity_character_scene_0001_scene_0002"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			closure, err := workflow.NewProductionWorldRepairClosure(views, test.selection)
			if err != nil {
				t.Fatal(err)
			}
			assertStringsEqual(t, "Scene scope", closure.SceneScopeKeys, test.scenes)
			assertStringsEqual(t, "Continuity", closure.ContinuityKeys, test.continuity)
		})
	}
}

func TestProductionWorldRepairClosureIsDeterministicAcrossReviewOrdering(t *testing.T) {
	selection := workflow.ProductionWorldRepairSelection{
		Operation: workflow.ProductionWorldRepairRebindOccurrence, TargetKeys: []string{"occurrence_character_scene_0001"},
	}
	leftViews := productionWorldRepairViews()
	rightViews := productionWorldRepairViews()
	slices.Reverse(rightViews.CharacterAppearances)
	slices.Reverse(rightViews.PropStates)
	slices.Reverse(rightViews.SceneOccurrences)
	slices.Reverse(rightViews.Interactions)
	slices.Reverse(rightViews.Continuity.Claims)
	slices.Reverse(rightViews.Continuity.Ledger)

	left, err := workflow.NewProductionWorldRepairClosure(leftViews, selection)
	if err != nil {
		t.Fatal(err)
	}
	right, err := workflow.NewProductionWorldRepairClosure(rightViews, selection)
	if err != nil {
		t.Fatal(err)
	}
	assertStringsEqual(t, "all", left.AllKeys(), right.AllKeys())
}

func assertStringsEqual(t *testing.T, label string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s keys = %#v, want %#v", label, got, want)
	}
}

func productionWorldRepairViews() workflow.ProductionWorldReviewViews {
	characterDry := agentcontract.ProductionStateFragment{StateKey: "state_character_linzhou_dry"}
	characterWet := agentcontract.ProductionStateFragment{StateKey: "state_character_linzhou_wet"}
	propGround := agentcontract.ProductionStateFragment{StateKey: "state_prop_key_ground"}
	propCarried := agentcontract.ProductionStateFragment{StateKey: "state_prop_key_carried"}
	return workflow.ProductionWorldReviewViews{
		CharacterAppearances: []workflow.ProductionWorldEntityReviewItem{{
			IdentityKey: "character:linzhou", Kind: "character", States: []agentcontract.ProductionStateFragment{characterDry, characterWet},
		}},
		Locations: []workflow.ProductionWorldEntityReviewItem{},
		PropStates: []workflow.ProductionWorldEntityReviewItem{{
			IdentityKey: "prop:key", Kind: "prop", States: []agentcontract.ProductionStateFragment{propGround, propCarried},
		}},
		SceneOccurrences: []workflow.ProductionWorldSceneOccurrenceReviewItem{
			{
				SceneScopeKey: "scene:0001",
				Occurrences: []agentcontract.SceneOccurrenceFragment{
					{OccurrenceKey: "occurrence_character_scene_0001", SubjectKind: "character", IdentityKey: "character:linzhou", StateKey: characterDry.StateKey},
					{OccurrenceKey: "occurrence_prop_scene_0001", SubjectKind: "prop", IdentityKey: "prop:key", StateKey: propGround.StateKey},
				},
			},
			{
				SceneScopeKey: "scene:0002",
				Occurrences: []agentcontract.SceneOccurrenceFragment{
					{OccurrenceKey: "occurrence_character_scene_0002", SubjectKind: "character", IdentityKey: "character:linzhou", StateKey: characterWet.StateKey},
				},
			},
			{
				SceneScopeKey: "scene:0003",
				Occurrences: []agentcontract.SceneOccurrenceFragment{
					{OccurrenceKey: "occurrence_character_scene_0003", SubjectKind: "character", IdentityKey: "character:linzhou", StateKey: characterWet.StateKey},
				},
			},
		},
		Interactions: []agentcontract.InteractionFragment{{
			InteractionKey: "interaction_pick_up_key", SceneScopeKey: "scene:0001",
			ActorOccurrenceKey: "occurrence_character_scene_0001", PropOccurrenceKey: "occurrence_prop_scene_0001",
			PropStateBeforeKey: propGround.StateKey, PropStateAfterKey: propCarried.StateKey,
		}},
		Continuity: workflow.ProductionWorldContinuityReviewView{
			Claims: []agentcontract.ContinuityFragment{
				{
					ContinuityKey: "continuity_character_scene_0001_scene_0002", IdentityKey: "character:linzhou",
					FromSceneScopeKey: "scene:0001", ToSceneScopeKey: "scene:0002",
					BeforeStateKey: characterDry.StateKey, AfterStateKey: characterWet.StateKey,
				},
				{
					ContinuityKey: "continuity_character_scene_0002_scene_0003", IdentityKey: "character:linzhou",
					FromSceneScopeKey: "scene:0002", ToSceneScopeKey: "scene:0003",
					BeforeStateKey: characterWet.StateKey, AfterStateKey: characterWet.StateKey,
				},
			},
			Ledger: []agentcontract.ContinuityLedgerEntry{
				{LedgerKey: "ledger_character_scene_0001", IdentityKey: "character:linzhou", SceneScopeKey: "scene:0001", StateKey: characterDry.StateKey},
				{LedgerKey: "ledger_prop_scene_0001", IdentityKey: "prop:key", SceneScopeKey: "scene:0001", StateKey: propCarried.StateKey, TransitionInteractionKey: repairStringRef("interaction_pick_up_key")},
				{LedgerKey: "ledger_character_scene_0002", IdentityKey: "character:linzhou", SceneScopeKey: "scene:0002", StateKey: characterWet.StateKey},
				{LedgerKey: "ledger_character_scene_0003", IdentityKey: "character:linzhou", SceneScopeKey: "scene:0003", StateKey: characterWet.StateKey},
			},
		},
	}
}

func repairStringRef(value string) *string { return &value }

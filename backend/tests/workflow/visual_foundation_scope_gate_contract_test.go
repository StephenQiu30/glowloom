package workflow_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	storygraphdomain "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestVisualFoundationScopeSubjectBindsExactCandidatesPresetAndExpectedTargets(t *testing.T) {
	draft := visualFoundationScopeSubjectDraft(t)
	subject, canonical, err := workflow.NewVisualFoundationScopeSubject(draft)
	if err != nil {
		t.Fatal(err)
	}
	if subject.SchemaVersion != workflow.VisualFoundationScopeSubjectSchemaVersion ||
		subject.ConfirmedProductionWorld.StoryGraphVersionID != draft.ConfirmedProductionWorld.StoryGraphVersionID ||
		subject.ConfirmedProductionWorld.OwnerSetHash != draft.ConfirmedProductionWorld.Inventory.OwnerSetHash ||
		subject.ProjectPresetSelection.SelectionID != draft.ProjectPresetSelection.ID ||
		subject.PresetRelease.ContentHash != draft.ProjectPresetSelection.PresetRelease.ContentHash ||
		subject.VisualFoundationCandidate.RevisionID != draft.VisualFoundationCandidate.RevisionID ||
		subject.ReferencePlanCandidate.RevisionID != draft.ReferencePlanCandidate.RevisionID ||
		subject.ReferenceTargetSeedRoot != draft.ReferencePlanInput.ReferenceTargetSeedRoot ||
		subject.ExpectedReferenceTargetSet.ExpectedTargetKeyRoot != draft.ExpectedReferenceTargetSet.ExpectedTargetKeyRoot ||
		len(subject.ConfirmedProductionWorld.ReadSetRoot) != 64 || len(subject.ReadSetRoot) != 64 {
		t.Fatalf("Gate 3 Subject is incomplete: %#v", subject)
	}
	decoded, decodedCanonical, err := workflow.DecodeVisualFoundationScopeSubject(canonical)
	if err != nil || !reflect.DeepEqual(decoded, subject) || !bytes.Equal(decodedCanonical, canonical) {
		t.Fatalf("decode Gate 3 Subject: decoded=%#v err=%v", decoded, err)
	}
}

func TestVisualFoundationScopeSubjectRejectsCandidateAndExpectedSetDrift(t *testing.T) {
	draft := visualFoundationScopeSubjectDraft(t)
	draft.ReferencePlanCandidate.ContentHash = strings.Repeat("f", 64)
	if _, _, err := workflow.NewVisualFoundationScopeSubject(draft); err == nil {
		t.Fatal("Gate 3 Subject accepted a drifted Reference Plan Candidate content hash")
	}

	draft = visualFoundationScopeSubjectDraft(t)
	draft.ExpectedReferenceTargetSet.ExpectedTargetBusinessKeys = append(
		draft.ExpectedReferenceTargetSet.ExpectedTargetBusinessKeys,
		`["scene_composition",["production/planning","planning_scene_set","outside",""]]`,
	)
	if _, _, err := workflow.NewVisualFoundationScopeSubject(draft); err == nil {
		t.Fatal("Gate 3 Subject accepted a Target outside the Backend expected set")
	}

	draft = visualFoundationScopeSubjectDraft(t)
	draft.ProjectPresetSelection.PresetRelease.ContentHash = strings.Repeat("e", 64)
	if _, _, err := workflow.NewVisualFoundationScopeSubject(draft); err == nil {
		t.Fatal("Gate 3 Subject accepted a drifted Project Preset Selection")
	}

	draft = visualFoundationScopeSubjectDraft(t)
	draft.ConfirmedProductionWorld.Inventory.CharacterSeeds[0].AnchorBusinessKey = "substituted-anchor"
	if _, _, err := workflow.NewVisualFoundationScopeSubject(draft); err == nil {
		t.Fatal("Gate 3 Subject accepted Reference Plan seeds outside the confirmed Production World")
	}
}

func TestVisualFoundationScopeSubjectRejectsUnknownOrPrePublishedSnapshotFields(t *testing.T) {
	draft := visualFoundationScopeSubjectDraft(t)
	_, canonical, err := workflow.NewVisualFoundationScopeSubject(draft)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(canonical, &value); err != nil {
		t.Fatal(err)
	}
	value["effective_style_snapshot_ref"] = map[string]any{
		"owner_version_id": uuid.NewString(), "revision": 1, "content_hash": strings.Repeat("a", 64),
	}
	drifted, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = workflow.DecodeVisualFoundationScopeSubject(drifted); err == nil {
		t.Fatal("Gate 3 Subject accepted a pre-published Effective Style Snapshot")
	}
}

func visualFoundationScopeSubjectDraft(t *testing.T) workflow.VisualFoundationScopeSubjectDraft {
	t.Helper()
	release := curatedFaithfulRelease(t)
	inventory := referencePlanInventoryFixture()
	visualRevision := referencePlanVisualFoundationRevision(t, inventory, release.ContentHash)
	input, _, err := workflowapp.CompileReferencePlanInput(workflowapp.ReferencePlanInputCommand{
		Inventory: inventory, VisualFoundationRevision: visualRevision, PresetRelease: release,
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate := referencePlanGateCandidate(t, input)
	candidateBytes, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := workflowapp.BuildReferencePlanCandidateProjection(input, candidateBytes)
	if err != nil {
		t.Fatal(err)
	}
	candidateContentHash, err := agentcontract.ProductionCanonicalHash(candidateBytes)
	if err != nil {
		t.Fatal(err)
	}
	workspaceID := inventory.CharacterSeeds[0].IdentityRef.WorkspaceID
	projectID := inventory.CharacterSeeds[0].IdentityRef.ProjectID
	return workflow.VisualFoundationScopeSubjectDraft{
		ConfirmedProductionWorld: storygraphdomain.ReferencePlanWorldReadSet{
			WorkspaceID: workspaceID, ProjectID: projectID,
			StoryGraphVersionID: uuid.NewString(), StoryGraphContentHash: referencePlanCompilerHash("gate-3-storygraph"),
			Inventory: inventory,
		},
		ProjectPresetSelection: frozenProjectSelection(t, workspaceID, projectID, release),
		VisualFoundationCandidate: workflow.VisualFoundationScopeCandidateRevisionMaterial{
			RevisionID: visualRevision.ID, Revision: visualRevision.Revision,
			RevisionHash: visualRevision.RevisionHash, ContentHash: visualRevision.CandidateContentHash,
			Candidate: visualRevision.Candidate,
		},
		ReferencePlanInput: input,
		ReferencePlanCandidate: workflow.VisualFoundationScopeCandidateRevisionMaterial{
			RevisionID: uuid.NewString(), Revision: 1,
			RevisionHash: referencePlanCompilerHash("gate-3-reference-plan-revision"),
			ContentHash:  candidateContentHash, Candidate: candidateBytes,
		},
		ExpectedReferenceTargetSet: projection.ExpectedTargetSet,
	}
}

func referencePlanGateCandidate(
	t *testing.T,
	input agentcontract.ReferencePlanInput,
) agentcontract.ReferencePlanCandidate {
	t.Helper()
	profiles := make(map[string]agentcontract.ReferencePlanPurposeProfile, len(input.PurposeProfiles))
	for _, profile := range input.PurposeProfiles {
		profiles[profile.TargetKind] = profile
	}
	anchor := input.CharacterSeeds[0]
	fixed := input.FixedTargetSeeds[0]
	targets := []agentcontract.ReferencePlanTargetSpecification{
		{
			TargetBusinessKey: anchor.AnchorBusinessKey, TargetKind: "character_identity_anchor",
			Fulfillment: "required", DesignFocus: profiles["character_identity_anchor"].DesignFocus[:1],
			ForbiddenChanges:            profiles["character_identity_anchor"].ForbiddenChanges,
			DependsOnTargetBusinessKeys: []string{},
		},
		{
			TargetBusinessKey: fixed.TargetBusinessKey, TargetKind: fixed.TargetKind,
			Fulfillment: "required", DesignFocus: profiles[fixed.TargetKind].DesignFocus[:1],
			ForbiddenChanges:            profiles[fixed.TargetKind].ForbiddenChanges,
			DependsOnTargetBusinessKeys: []string{anchor.AnchorBusinessKey},
		},
	}
	if targets[0].TargetBusinessKey > targets[1].TargetBusinessKey {
		targets[0], targets[1] = targets[1], targets[0]
	}
	return agentcontract.ReferencePlanCandidate{
		WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID,
		ProductionWorldOwnerSetHash:           input.ProductionWorldOwnerSetHash,
		P1ScopeKeys:                           append([]string(nil), input.P1ScopeKeys...),
		VisualFoundationCandidateRevisionID:   input.VisualFoundationCandidateRevisionID,
		VisualFoundationCandidateRevisionHash: input.VisualFoundationCandidateRevisionHash,
		ReferenceTargetSeedRoot:               input.ReferenceTargetSeedRoot,
		AnchorSelections: []agentcontract.ReferencePlanAnchorSelection{{
			AnchorBusinessKey: anchor.AnchorBusinessKey, SelectedStateRef: anchor.StateOptions[0].StateRef,
		}},
		TargetSpecifications: targets,
	}
}

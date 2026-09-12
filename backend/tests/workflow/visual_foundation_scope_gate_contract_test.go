package workflow_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	presetdomain "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
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

func TestVisualFoundationScopeGateInputDerivesReadyDecisionsAndAtomicEffectPlan(t *testing.T) {
	draft := visualFoundationScopeSubjectDraft(t)
	subject, _, err := workflow.NewVisualFoundationScopeSubject(draft)
	if err != nil {
		t.Fatal(err)
	}
	value, canonical, err := workflow.NewVisualFoundationScopeGateInput(
		workflow.VisualFoundationScopeGateInputDraft{
			WorkspaceID:   draft.ConfirmedProductionWorld.WorkspaceID,
			ProjectID:     draft.ConfirmedProductionWorld.ProjectID,
			WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(),
			Subject: subject, PresetRelease: draft.PresetRelease,
			ImageGenerationCapability: workflow.VisualFoundationScopeImageGenerationCapability{
				Available: true, ReadSetHash: strings.Repeat("c", 64),
			},
			ExpectedPresetHead: workflow.HumanGateExpectedHead{
				OwnerKind: "preset", LogicalID: draft.ConfirmedProductionWorld.ProjectID,
				Revision: 0,
			},
			ExpectedReferenceHead: workflow.HumanGateExpectedHead{
				OwnerKind: "production/reference", LogicalID: uuid.NewString(), Revision: 0,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if value.GateKey != workflow.VisualFoundationScopeGateKey || len(value.InputHash) != 64 ||
		len(value.SubjectHash) != 64 || len(value.PresetCapabilityManifestRoot) != 64 ||
		!value.ImageGenerationCapability.Available || len(value.SemanticBlockers) != 0 ||
		!slices.Equal(value.AllowedDecisions, []string{"approved", "changes_requested", "rejected"}) {
		t.Fatalf("ready Gate 3 input is incomplete: %#v", value)
	}
	step := value.EffectPlan.AtomicStep
	if value.EffectPlan.PlanKey != "visual_foundation_scope" ||
		step.StepKey != "confirm_visual_foundation_and_reference_plan" ||
		step.OwnerCommand != "confirm_visual_foundation_and_reference_plan" ||
		!slices.Equal(step.OwnerKinds, []string{"preset", "production/reference"}) ||
		len(step.ExpectedHeads) != 2 || len(value.ReadSetRoot) != 64 || step.ReadSetRoot != value.ReadSetRoot ||
		step.ExpectedReferenceTargetKeyRoot != subject.ExpectedReferenceTargetSet.ExpectedTargetKeyRoot {
		t.Fatalf("Gate 3 atomic effect plan drifted: %#v", value.EffectPlan)
	}
	decoded, decodedCanonical, err := workflow.DecodeVisualFoundationScopeGateInput(canonical)
	if err != nil || !reflect.DeepEqual(decoded, value) || !bytes.Equal(decodedCanonical, canonical) {
		t.Fatalf("decode Gate 3 input: decoded=%#v err=%v", decoded, err)
	}
}

func TestVisualFoundationScopeGateInputKeepsDraftButRemovesApprovalWithoutImageCapability(t *testing.T) {
	draft := visualFoundationScopeSubjectDraft(t)
	subject, _, err := workflow.NewVisualFoundationScopeSubject(draft)
	if err != nil {
		t.Fatal(err)
	}
	value, canonical, err := workflow.NewVisualFoundationScopeGateInput(
		workflow.VisualFoundationScopeGateInputDraft{
			WorkspaceID:   draft.ConfirmedProductionWorld.WorkspaceID,
			ProjectID:     draft.ConfirmedProductionWorld.ProjectID,
			WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(),
			Subject: subject, PresetRelease: draft.PresetRelease,
			ImageGenerationCapability: workflow.VisualFoundationScopeImageGenerationCapability{
				Available: false, ReadSetHash: strings.Repeat("d", 64),
			},
			ExpectedPresetHead: workflow.HumanGateExpectedHead{
				OwnerKind: "preset", LogicalID: draft.ConfirmedProductionWorld.ProjectID,
				Revision: 0,
			},
			ExpectedReferenceHead: workflow.HumanGateExpectedHead{
				OwnerKind: "production/reference", LogicalID: uuid.NewString(), Revision: 0,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(value.AllowedDecisions, []string{"changes_requested", "rejected"}) ||
		len(value.SemanticBlockers) != 1 ||
		value.SemanticBlockers[0].Code != "image_generation_capability_unavailable" {
		t.Fatalf("unavailable capability did not close approval: %#v", value)
	}
	for _, forbidden := range []string{"provider_call", "provider_job", "artifact", "media_bytes"} {
		if bytes.Contains(canonical, []byte(forbidden)) {
			t.Fatalf("Gate 3 input contains forbidden execution field %q", forbidden)
		}
	}
	var raw map[string]any
	if err = json.Unmarshal(canonical, &raw); err != nil {
		t.Fatal(err)
	}
	raw["provider_call_ref"] = uuid.NewString()
	drifted, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = workflow.DecodeVisualFoundationScopeGateInput(drifted); err == nil {
		t.Fatal("Gate 3 input accepted a Provider call reference")
	}
}

func TestVisualFoundationScopeGateInputRejectsCapabilityAndHashDrift(t *testing.T) {
	draft := visualFoundationScopeSubjectDraft(t)
	subject, _, err := workflow.NewVisualFoundationScopeSubject(draft)
	if err != nil {
		t.Fatal(err)
	}
	base := workflow.VisualFoundationScopeGateInputDraft{
		WorkspaceID:   draft.ConfirmedProductionWorld.WorkspaceID,
		ProjectID:     draft.ConfirmedProductionWorld.ProjectID,
		WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(),
		Subject: subject, PresetRelease: draft.PresetRelease,
		ImageGenerationCapability: workflow.VisualFoundationScopeImageGenerationCapability{
			Available: true, ReadSetHash: strings.Repeat("e", 64),
		},
		ExpectedPresetHead: workflow.HumanGateExpectedHead{
			OwnerKind: "preset", LogicalID: draft.ConfirmedProductionWorld.ProjectID, Revision: 0,
		},
		ExpectedReferenceHead: workflow.HumanGateExpectedHead{
			OwnerKind: "production/reference", LogicalID: uuid.NewString(), Revision: 0,
		},
	}
	_, canonical, err := workflow.NewVisualFoundationScopeGateInput(base)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err = json.Unmarshal(canonical, &raw); err != nil {
		t.Fatal(err)
	}
	raw["allowed_decisions"] = []string{"approved", "changes_requested", "rejected"}
	capability := raw["image_generation_capability"].(map[string]any)
	capability["available"] = false
	drifted, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = workflow.DecodeVisualFoundationScopeGateInput(drifted); err == nil {
		t.Fatal("Gate 3 input accepted capability readiness and allowed decision drift")
	}

	invalidRelease := draft.PresetRelease
	invalidRelease.CapabilityManifest = append([]presetdomain.Capability(nil), invalidRelease.CapabilityManifest[:5]...)
	base.PresetRelease = invalidRelease
	if _, _, err = workflow.NewVisualFoundationScopeGateInput(base); err == nil {
		t.Fatal("Gate 3 input accepted a Preset capability projection outside the frozen Release")
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
		PresetRelease:          release,
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

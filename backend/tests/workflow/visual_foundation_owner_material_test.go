package workflow_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestVisualFoundationOwnerMaterialFreezesBothCandidatesAndRejectsDrift(t *testing.T) {
	draft := visualFoundationScopeSubjectDraft(t)
	subject, _, err := workflow.NewVisualFoundationScopeSubject(draft)
	if err != nil {
		t.Fatal(err)
	}
	gate, _, err := workflow.NewVisualFoundationScopeGateInput(workflow.VisualFoundationScopeGateInputDraft{
		WorkspaceID: draft.ConfirmedProductionWorld.WorkspaceID, ProjectID: draft.ConfirmedProductionWorld.ProjectID,
		WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), Subject: subject, PresetRelease: draft.PresetRelease,
		ImageGenerationCapability: workflow.VisualFoundationScopeImageGenerationCapability{
			Available: true, ReadSetHash: strings.Repeat("c", 64),
		},
		ExpectedPresetHead: workflow.HumanGateExpectedHead{
			OwnerKind: "preset", LogicalID: draft.ConfirmedProductionWorld.ProjectID,
		},
		ExpectedReferenceHead: workflow.HumanGateExpectedHead{
			OwnerKind: "production/reference", LogicalID: uuid.NewString(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	material := workflow.VisualFoundationOwnerMaterial{
		SchemaVersion: workflow.VisualFoundationOwnerMaterialSchema, GateInputID: uuid.NewString(), GateInput: gate,
		ConfirmedProductionWorld: draft.ConfirmedProductionWorld, Selection: draft.ProjectPresetSelection,
		Release: draft.PresetRelease, VisualCandidate: draft.VisualFoundationCandidate,
		ReferencePlanInput: draft.ReferencePlanInput, ReferenceCandidate: draft.ReferencePlanCandidate,
	}
	raw, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = workflow.DecodeVisualFoundationOwnerMaterial(raw); err != nil {
		t.Fatalf("decode Visual Foundation owner material: %v", err)
	}
	material.ReferencePlanInput.ReferenceTargetSeedRoot = strings.Repeat("f", 64)
	raw, _ = json.Marshal(material)
	if _, err = workflow.DecodeVisualFoundationOwnerMaterial(raw); err == nil {
		t.Fatal("Visual Foundation owner material accepted Reference Plan lineage drift")
	}
}

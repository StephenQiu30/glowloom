package storygraph_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestReferenceGenerationSourceResolvesExactIdentityBinding(t *testing.T) {
	world := visualFoundationProductionVersion(t)
	input, brief := referenceGenerationAnchorBrief(t, world)
	compiled, err := generationapp.CompileBaseReferenceGenerationSource(input, brief, world)
	if err != nil {
		t.Fatal(err)
	}
	var source generationapp.CharacterIdentityAnchorSource
	if err = json.Unmarshal(compiled.Payload, &source); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(source.IdentityRef, input.SourceRefs.Identity[0]) || !reflect.DeepEqual(source.IdentityAnchorAssetStateRef, input.SourceRefs.State[0]) || !reflect.DeepEqual(source.OccurrenceRefs, input.SourceRefs.Occurrence) {
		t.Fatal("source lost exact identity, state, or occurrences")
	}
	var expectedBinding contract.ReferencePlanOwnerRef
	for _, node := range world.Nodes {
		if node.NodeType == storygraph.NodeTypeProductionBinding {
			expectedBinding = generationTestOwnerRef(t, node.OwnerRef)
		}
	}
	if !reflect.DeepEqual(source.ProductionBindingRef, expectedBinding) {
		t.Fatal("source resolved a different Production Binding")
	}
	for _, field := range []string{"identity", "specification", "state", "occurrence"} {
		t.Run(field+" exact hash", func(t *testing.T) {
			changedInput, changedBrief := referenceGenerationAnchorBrief(t, world)
			refs := map[string][]contract.ReferencePlanOwnerRef{"identity": changedInput.SourceRefs.Identity, "specification": changedInput.SourceRefs.Specification, "state": changedInput.SourceRefs.State, "occurrence": changedInput.SourceRefs.Occurrence}
			refs[field][0].OwnerContentHash = strings.Repeat("f", 64)
			changedBrief.SourceRefs = changedInput.SourceRefs
			if err := changedBrief.ValidateFor(changedInput); err != nil {
				t.Fatalf("invalid negative fixture: %v", err)
			}
			if _, err := generationapp.CompileBaseReferenceGenerationSource(changedInput, changedBrief, world); err == nil {
				t.Fatal("source accepted an inexact Owner ref")
			}
		})
	}
}

func referenceGenerationAnchorBrief(t *testing.T, world storygraph.Version) (contract.ReferenceBriefInput, contract.ReferenceBriefCandidate) {
	t.Helper()
	rootRef := func(kind, family, logical string) contract.ReferencePlanOwnerRef {
		return contract.ReferencePlanOwnerRef{WorkspaceID: world.WorkspaceID, ProjectID: world.ProjectID, OwnerKind: kind, VersionFamily: family, OwnerLogicalID: logical, OwnerVersionID: uuid.NewString(), OwnerRevision: 1, OwnerContentHash: strings.Repeat("a", 64)}
	}
	refs := contract.ReferencePlanTargetOwnerRefs{Interaction: []contract.ReferencePlanOwnerRef{}}
	for _, node := range world.Nodes {
		switch node.NodeType {
		case storygraph.NodeTypeAssetIdentity:
			refs.Identity = append(refs.Identity, generationTestOwnerRef(t, node.OwnerRef))
		case storygraph.NodeTypeCharacterSpecification:
			refs.Specification = append(refs.Specification, generationTestOwnerRef(t, node.OwnerRef))
		case storygraph.NodeTypeAssetState:
			refs.State = append(refs.State, generationTestOwnerRef(t, node.OwnerRef))
		case storygraph.NodeTypeOccurrence:
			refs.Occurrence = append(refs.Occurrence, generationTestOwnerRef(t, node.OwnerRef))
		case storygraph.NodeTypeScene:
			refs.Scene = append(refs.Scene, generationTestOwnerRef(t, node.OwnerRef))
		}
	}
	identity := refs.Identity[0]
	keyBytes, err := json.Marshal([]any{"character_identity_anchor", []string{identity.OwnerKind, identity.VersionFamily, identity.OwnerLogicalID, ""}})
	if err != nil {
		t.Fatal(err)
	}
	targetKey := string(keyBytes)
	input := contract.ReferenceBriefInput{
		WorkspaceID: world.WorkspaceID, ProjectID: world.ProjectID, TargetKind: "character_identity_anchor", TargetBusinessKey: targetKey, TargetFulfillment: "required",
		ApprovedReferencePlanVersionRef: rootRef("production/reference", "reference_plan_set", uuid.NewString()), ReferencePlanTargetRef: rootRef("production/reference", "reference_plan_set", targetKey),
		VisualFoundationVersionRef: rootRef("preset", "preset_effective_set", world.ProjectID), EffectiveStyleSnapshotRef: rootRef("preset", "preset_effective_set", world.ProjectID+":style"), EffectivePolicySnapshotRef: rootRef("preset", "preset_effective_set", world.ProjectID+":policy"),
		DependencySelections: []contract.ReferenceBriefDependencySelection{}, StageRelease: contract.ReferenceBriefStageRelease{StageKey: "compile_reference_brief", StageReleaseHash: strings.Repeat("b", 64)}, TypedReadSetRoot: strings.Repeat("c", 64),
		SourceRefs: refs, DesignFocus: []string{"preserve approved identity"}, ForbiddenChanges: []string{"no invented identity"}, RequiredViewRoles: contract.ReferenceBriefRequiredViewRoles("character_identity_anchor"),
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var brief contract.ReferenceBriefCandidate
	if err = json.Unmarshal(raw, &brief); err != nil {
		t.Fatal(err)
	}
	brief.PositiveInstructions, brief.NegativeInstructions = input.DesignFocus, input.ForbiddenChanges
	brief.SourceDesignSlots = []contract.ReferenceBriefSourceDesignSlot{{SlotKey: "face", SourceRequirement: "preserve identity", DesignRequirement: "approved style"}}
	brief.LayoutRequirements, brief.ScaleRequirements = []string{"independent views"}, []string{"approved scale"}
	brief.RightsRequirements, brief.ProvenanceRequirements = []string{"authorized material"}, []string{"preserve source lineage"}
	brief.QCRubricRefs = []contract.ReferenceBriefQCRubricRef{{ContractID: "reference-visual-qc-production", ContentHash: strings.Repeat("d", 64)}}
	brief.Brief = json.RawMessage(`{"target_kind":"character_identity_anchor","identity_invariant_slots":["body_shape","facial_structure","hair","permanent_marks","proportions"]}`)
	if err = brief.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	return input, brief
}

func generationTestOwnerRef(t *testing.T, ref storygraph.OwnerRef) contract.ReferencePlanOwnerRef {
	t.Helper()
	raw, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	var value contract.ReferencePlanOwnerRef
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

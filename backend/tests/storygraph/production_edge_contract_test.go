package storygraph_test

import (
	"testing"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestProductionEdgeContractRepresentsEveryManifestRelationship(t *testing.T) {
	tests := []struct {
		name      string
		edgeType  storygraph.EdgeType
		from, to  storygraph.NodeType
		qualifier storygraph.EdgeQualifier
	}{
		{"contains", storygraph.EdgeTypeContains, storygraph.NodeTypeEpisode, storygraph.NodeTypeScene, storygraph.EdgeQualifier{SequenceKey: "scene:0001"}},
		{"derived from", storygraph.EdgeTypeDerivedFrom, storygraph.NodeTypeSourceRevision, storygraph.NodeTypeSourceEvidence, storygraph.EdgeQualifier{}},
		{"describes identity", storygraph.EdgeTypeDescribesIdentity, storygraph.NodeTypeAssetIdentity, storygraph.NodeTypeCharacterSpecification, storygraph.EdgeQualifier{}},
		{"has state", storygraph.EdgeTypeHasState, storygraph.NodeTypeAssetIdentity, storygraph.NodeTypeAssetState, storygraph.EdgeQualifier{}},
		{"precedes", storygraph.EdgeTypePrecedes, storygraph.NodeTypeEpisode, storygraph.NodeTypeEpisode, storygraph.EdgeQualifier{SequenceKey: "episode:0002"}},
		{"anchors occurrence", storygraph.EdgeTypeAnchorsOccurrence, storygraph.NodeTypeScene, storygraph.NodeTypeOccurrence, storygraph.EdgeQualifier{AnchorRole: "scene"}},
		{"instantiates occurrence", storygraph.EdgeTypeInstantiatesOccurrence, storygraph.NodeTypeAssetState, storygraph.NodeTypeOccurrence, storygraph.EdgeQualifier{}},
		{"supports", storygraph.EdgeTypeSupports, storygraph.NodeTypeSourceEvidence, storygraph.NodeTypeRelationshipClaim, storygraph.EdgeQualifier{}},
		{"claim participant", storygraph.EdgeTypeClaimParticipant, storygraph.NodeTypeAssetIdentity, storygraph.NodeTypeRelationshipClaim, storygraph.EdgeQualifier{ParticipantRole: "subject"}},
		{"claim anchor", storygraph.EdgeTypeClaimAnchor, storygraph.NodeTypeScene, storygraph.NodeTypeRelationshipClaim, storygraph.EdgeQualifier{AnchorRole: "scene"}},
		{"claim state", storygraph.EdgeTypeClaimState, storygraph.NodeTypeAssetState, storygraph.NodeTypeContinuityClaim, storygraph.EdgeQualifier{StateRole: "before"}},
		{"supersedes", storygraph.EdgeTypeSupersedes, storygraph.NodeTypeRelationshipClaim, storygraph.NodeTypeRelationshipClaim, storygraph.EdgeQualifier{}},
		{"constrains", storygraph.EdgeTypeConstrains, storygraph.NodeTypeEffectiveStyleSnapshot, storygraph.NodeTypeAssetVersion, storygraph.EdgeQualifier{ConstraintRole: "style"}},
		{"materializes", storygraph.EdgeTypeMaterializes, storygraph.NodeTypeAssetVersion, storygraph.NodeTypeAssetVersion, storygraph.EdgeQualifier{BindingRole: "identity_anchor"}},
		{"contains reference target", storygraph.EdgeTypeContainsReferenceTarget, storygraph.NodeTypeApprovedReferencePlanVersion, storygraph.NodeTypeReferencePlanTarget, storygraph.EdgeQualifier{SequenceKey: "target:0001"}},
		{"depends on reference target", storygraph.EdgeTypeDependsOnReferenceTarget, storygraph.NodeTypeReferencePlanTarget, storygraph.NodeTypeReferencePlanTarget, storygraph.EdgeQualifier{}},
		{"plans reference", storygraph.EdgeTypePlansReference, storygraph.NodeTypeAssetIdentity, storygraph.NodeTypeReferencePlanTarget, storygraph.EdgeQualifier{ReferenceRole: "identity"}},
		{"fulfills reference target", storygraph.EdgeTypeFulfillsReferenceTarget, storygraph.NodeTypeReferencePlanTarget, storygraph.NodeTypeAssetVersion, storygraph.EdgeQualifier{}},
		{"binds reference input", storygraph.EdgeTypeBindsReferenceInput, storygraph.NodeTypeAssetVersion, storygraph.NodeTypeSceneReferenceBindingVersion, storygraph.EdgeQualifier{ReferenceRole: "character_asset"}},
		{"binds reference output", storygraph.EdgeTypeBindsReferenceOutput, storygraph.NodeTypeArtifact, storygraph.NodeTypeSceneReferenceBindingVersion, storygraph.EdgeQualifier{}},
		{"realizes", storygraph.EdgeTypeRealizes, storygraph.NodeTypeNarrativeBeat, storygraph.NodeTypeShot, storygraph.EdgeQualifier{}},
		{"informs", storygraph.EdgeTypeInforms, storygraph.NodeTypeSceneReferenceBindingVersion, storygraph.NodeTypeShotProductionBindingVersion, storygraph.EdgeQualifier{InformsRole: "scene_reference"}},
		{"binds input", storygraph.EdgeTypeBindsInput, storygraph.NodeTypeSceneReferenceBindingVersion, storygraph.NodeTypeShotProductionBindingVersion, storygraph.EdgeQualifier{BindingRole: "scene_reference"}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if err := storygraph.ValidateEdgeEndpoint(testCase.edgeType, testCase.from, testCase.to, testCase.qualifier); err != nil {
				t.Fatalf("manifest edge cannot be represented by the graph contract: %v", err)
			}
			if err := storygraph.ValidateProductionEdgeEndpoint(testCase.edgeType, testCase.from, testCase.to, testCase.qualifier); err != nil {
				t.Fatalf("manifest edge was rejected: %v", err)
			}
		})
	}
}

func TestProductionEdgeContractRejectsExcludedOrMismatchedRelationships(t *testing.T) {
	tests := []struct {
		name      string
		edgeType  storygraph.EdgeType
		from, to  storygraph.NodeType
		qualifier storygraph.EdgeQualifier
	}{
		{"legacy generation edge", storygraph.EdgeTypeFeedsGeneration, storygraph.NodeTypeGenerationTarget, storygraph.NodeTypeArtifact, storygraph.EdgeQualifier{}},
		{"legacy output edge", storygraph.EdgeTypeBindsOutput, storygraph.NodeTypeArtifact, storygraph.NodeTypeShotImageBindingVersion, storygraph.EdgeQualifier{}},
		{"constraint role drift", storygraph.EdgeTypeConstrains, storygraph.NodeTypeEffectiveStyleSnapshot, storygraph.NodeTypeAssetVersion, storygraph.EdgeQualifier{ConstraintRole: "world"}},
		{"materialization role drift", storygraph.EdgeTypeMaterializes, storygraph.NodeTypeAssetVersion, storygraph.NodeTypeAssetVersion, storygraph.EdgeQualifier{BindingRole: "artifact"}},
		{"reference role drift", storygraph.EdgeTypePlansReference, storygraph.NodeTypeAssetIdentity, storygraph.NodeTypeReferencePlanTarget, storygraph.EdgeQualifier{ReferenceRole: "state"}},
		{"informs role drift", storygraph.EdgeTypeInforms, storygraph.NodeTypeSceneReferenceBindingVersion, storygraph.NodeTypeShotProductionBindingVersion, storygraph.EdgeQualifier{InformsRole: "interaction_reference"}},
		{"binding role drift", storygraph.EdgeTypeBindsInput, storygraph.NodeTypeSceneReferenceBindingVersion, storygraph.NodeTypeShotProductionBindingVersion, storygraph.EdgeQualifier{BindingRole: "interaction_reference"}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if err := storygraph.ValidateProductionEdgeEndpoint(testCase.edgeType, testCase.from, testCase.to, testCase.qualifier); err == nil {
				t.Fatal("relationship excluded by the production manifest was accepted")
			}
		})
	}
}

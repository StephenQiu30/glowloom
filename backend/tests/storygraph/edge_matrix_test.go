package storygraph_test

import (
	"testing"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestStoryGraphEdgeMatrixCoversEveryCanonicalRelationship(t *testing.T) {
	cases := []struct {
		name      string
		edgeType  storygraph.EdgeType
		from, to  storygraph.NodeType
		qualifier storygraph.EdgeQualifier
		badFrom   storygraph.NodeType
		badTo     storygraph.NodeType
	}{
		{"contains", storygraph.EdgeTypeContains, storygraph.NodeTypeEpisode, storygraph.NodeTypeScene, storygraph.EdgeQualifier{SequenceKey: "episode/1"}, storygraph.NodeTypeAssetIdentity, storygraph.NodeTypeScene},
		{"derived_from", storygraph.EdgeTypeDerivedFrom, storygraph.NodeTypeSourceRevision, storygraph.NodeTypeEpisode, storygraph.EdgeQualifier{}, storygraph.NodeTypeEpisode, storygraph.NodeTypeSourceRevision},
		{"describes_identity", storygraph.EdgeTypeDescribesIdentity, storygraph.NodeTypeAssetIdentity, storygraph.NodeTypeCharacterSpecification, storygraph.EdgeQualifier{}, storygraph.NodeTypeAssetState, storygraph.NodeTypeCharacterSpecification},
		{"has_state", storygraph.EdgeTypeHasState, storygraph.NodeTypeAssetIdentity, storygraph.NodeTypeAssetState, storygraph.EdgeQualifier{}, storygraph.NodeTypeCharacterSpecification, storygraph.NodeTypeAssetState},
		{"precedes", storygraph.EdgeTypePrecedes, storygraph.NodeTypeScene, storygraph.NodeTypeScene, storygraph.EdgeQualifier{SequenceKey: "episode/1/scenes"}, storygraph.NodeTypeEpisode, storygraph.NodeTypeEpisode},
		{"anchors_occurrence", storygraph.EdgeTypeAnchorsOccurrence, storygraph.NodeTypeNarrativeBeat, storygraph.NodeTypeOccurrence, storygraph.EdgeQualifier{}, storygraph.NodeTypeAssetState, storygraph.NodeTypeOccurrence},
		{"instantiates_occurrence", storygraph.EdgeTypeInstantiatesOccurrence, storygraph.NodeTypeAssetState, storygraph.NodeTypeOccurrence, storygraph.EdgeQualifier{}, storygraph.NodeTypeAssetIdentity, storygraph.NodeTypeOccurrence},
		{"realizes", storygraph.EdgeTypeRealizes, storygraph.NodeTypeNarrativeBeat, storygraph.NodeTypeShot, storygraph.EdgeQualifier{}, storygraph.NodeTypeScene, storygraph.NodeTypeShot},
		{"informs", storygraph.EdgeTypeInforms, storygraph.NodeTypeOccurrence, storygraph.NodeTypeShot, storygraph.EdgeQualifier{}, storygraph.NodeTypeScene, storygraph.NodeTypeShot},
		{"constrains", storygraph.EdgeTypeConstrains, storygraph.NodeTypeWorldRule, storygraph.NodeTypeGenerationTarget, storygraph.EdgeQualifier{}, storygraph.NodeTypeSourceEvidence, storygraph.NodeTypeGenerationTarget},
		{"materializes", storygraph.EdgeTypeMaterializes, storygraph.NodeTypeCharacterSpecification, storygraph.NodeTypeProductionBinding, storygraph.EdgeQualifier{BindingRole: "specification"}, storygraph.NodeTypeAssetState, storygraph.NodeTypeProductionBinding},
		{"binds_input", storygraph.EdgeTypeBindsInput, storygraph.NodeTypeAssetVersion, storygraph.NodeTypeShotProductionBindingVersion, storygraph.EdgeQualifier{}, storygraph.NodeTypeArtifact, storygraph.NodeTypeShotProductionBindingVersion},
		{"feeds_generation", storygraph.EdgeTypeFeedsGeneration, storygraph.NodeTypeGenerationTarget, storygraph.NodeTypeArtifact, storygraph.EdgeQualifier{}, storygraph.NodeTypeArtifact, storygraph.NodeTypeGenerationTarget},
		{"binds_output", storygraph.EdgeTypeBindsOutput, storygraph.NodeTypeArtifact, storygraph.NodeTypeShotImageBindingVersion, storygraph.EdgeQualifier{}, storygraph.NodeTypeAssetVersion, storygraph.NodeTypeShotImageBindingVersion},
		{"supports", storygraph.EdgeTypeSupports, storygraph.NodeTypeSourceEvidence, storygraph.NodeTypeRelationshipClaim, storygraph.EdgeQualifier{}, storygraph.NodeTypeAssetIdentity, storygraph.NodeTypeRelationshipClaim},
		{"claim_participant", storygraph.EdgeTypeClaimParticipant, storygraph.NodeTypeAssetIdentity, storygraph.NodeTypeRelationshipClaim, storygraph.EdgeQualifier{ParticipantRole: "subject"}, storygraph.NodeTypeScene, storygraph.NodeTypeRelationshipClaim},
		{"claim_anchor", storygraph.EdgeTypeClaimAnchor, storygraph.NodeTypeScene, storygraph.NodeTypeRelationshipClaim, storygraph.EdgeQualifier{AnchorRole: "scene"}, storygraph.NodeTypeAssetIdentity, storygraph.NodeTypeRelationshipClaim},
		{"supersedes", storygraph.EdgeTypeSupersedes, storygraph.NodeTypeRelationshipClaim, storygraph.NodeTypePayoffClaim, storygraph.EdgeQualifier{}, storygraph.NodeTypeRelationshipClaim, storygraph.NodeTypeAssetIdentity},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := storygraph.ValidateEdgeEndpoint(testCase.edgeType, testCase.from, testCase.to, testCase.qualifier); err != nil {
				t.Fatalf("valid endpoint rejected: %v", err)
			}
			if err := storygraph.ValidateEdgeEndpoint(testCase.edgeType, testCase.badFrom, testCase.badTo, testCase.qualifier); err == nil {
				t.Fatal("invalid endpoint combination was accepted")
			}
		})
	}

	if err := storygraph.ValidateEdgeEndpoint(
		storygraph.EdgeTypeMaterializes,
		storygraph.NodeTypeAssetState,
		storygraph.NodeTypeProductionBinding,
		storygraph.EdgeQualifier{BindingRole: "specification"},
	); err == nil {
		t.Fatal("materializes accepted a binding_role that does not match its source type")
	}
}

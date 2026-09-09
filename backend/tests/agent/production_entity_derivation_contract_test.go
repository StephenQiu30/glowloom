package agent_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestProductionEntityDerivationRequiresFormalIdentityAndTypedState(t *testing.T) {
	input, candidate := productionEntityContractFixture(t)
	if err := input.Validate(); err != nil {
		t.Fatalf("validate Production Entity input: %v", err)
	}
	raw, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err = contract.ValidateProductionEntityFragmentCandidate(raw, input); err != nil {
		t.Fatalf("validate Production Entity Candidate: %v", err)
	}

	invalid := candidate
	invalid.Entities = append([]contract.ProductionEntityFragment(nil), candidate.Entities...)
	invalid.Entities[0].Basis.CreatorDecisionProposal = &contract.CreatorDecisionProposal{
		DecisionKey: "decision_character_linzhou", Rationale: "补充人物设计",
	}
	raw, err = json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if err = contract.ValidateProductionEntityFragmentCandidate(raw, input); err == nil {
		t.Fatal("Production Entity Candidate accepted Evidence and CreatorDecision together")
	}
}

func TestProductionEntityDerivationStageRejectsFormalIdentityDrift(t *testing.T) {
	input, _ := productionEntityContractFixture(t)
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	payload := contract.SceneAnalysisPayload{
		Variant: contract.SceneAnalysisStageVariant{
			StageKey: "derive_production_entities", ProfileKey: "default", LaneKey: "primary",
			OutputSchemaVersion: contract.ProductionEntityFragmentCandidateSchemaVersion,
		},
		Scope: contract.SceneAnalysisScope{
			WorkspaceID: input.StructureIdentitySet.WorkspaceID,
			ProjectID:   input.StructureIdentitySet.ProjectID,
		},
		SourceRefs: []contract.ScriptSourceVersionIdentity{{
			OwnerKind: "production/script", LogicalID: uuid.NewString(),
			VersionID: input.SourceVersionID, Revision: 1, ContentHash: input.SourceHash,
			CreatedAt: time.Date(2026, time.September, 9, 8, 0, 0, 0, time.UTC),
		}},
		UpstreamCandidates: []contract.SceneAnalysisCandidateRevisionIdentity{{
			StageKey: "extract_scene_facts", ShardKey: "script:full",
			CandidateRevisionID:   input.SceneFactCandidateRevisionID,
			CandidateRevisionHash: input.SceneFactCandidateRevisionHash,
			SourceInvocationID:    uuid.NewString(), SourceResultHash: "b" + input.SourceHash[1:],
		}},
		Shard: contract.SceneAnalysisShard{
			ManifestID: uuid.NewString(), ManifestHash: input.SourceHash, ShardKey: "script:full",
			CodepointStart: 0, CodepointEnd: 2,
		},
		StageInput: encoded,
	}
	if err = payload.Validate(); err != nil {
		t.Fatalf("validate Production Entity payload: %v", err)
	}

	drifted := input
	drifted.StructureIdentitySetVersionHash = "f" + input.StructureIdentitySetVersionHash[1:]
	payload.StageInput, err = json.Marshal(drifted)
	if err != nil {
		t.Fatal(err)
	}
	if err = payload.Validate(); err == nil {
		t.Fatal("Production Entity payload accepted a drifted formal identity hash")
	}
}

func productionEntityContractFixture(
	t *testing.T,
) (contract.ProductionEntityDerivationInput, contract.ProductionEntityFragmentCandidate) {
	t.Helper()
	text := "林舟"
	sourceHash := hashSceneAnalysisText(text)
	evidence := contract.SourceEvidenceSpan{
		SourceStart: 0, SourceEnd: 2, TextHash: sourceHash, ExactAnchor: text,
	}
	workspaceID, projectID := uuid.NewString(), uuid.NewString()
	sourceVersionID, identityVersionID := uuid.NewString(), uuid.NewString()
	sceneOwnerID := uuid.NewString()
	episodeID := uuid.NewString()
	sceneFactRevisionID := uuid.NewString()
	sceneFact := contract.SceneFactCandidate{
		SourceVersionID: sourceVersionID, SourceHash: sourceHash,
		SpanCandidateRevisionID: uuid.NewString(), SpanCandidateRevisionHash: "2" + sourceHash[1:],
		Scenes: []contract.SceneFact{{
			TemporarySceneID: "scene_0001", SpanID: "span_0001", SourceStart: 0, SourceEnd: 2,
			Actions: []contract.GroundedAction{}, Dialogues: []contract.GroundedDialogue{},
			RawCharacterMentions: []contract.RawEntityMention{{
				Text: text, OccurrenceRole: "actual", Evidence: evidence,
			}},
			RawPropMentions: []contract.RawEntityMention{},
		}},
		ReviewIssues: []contract.CandidateReviewIssue{},
	}
	sceneFactRaw, err := json.Marshal(sceneFact)
	if err != nil {
		t.Fatal(err)
	}
	identityHash := "1" + sourceHash[1:]
	input := contract.ProductionEntityDerivationInput{
		SourceVersionID: sourceVersionID, SourceHash: sourceHash, NormalizedText: text,
		StructureIdentitySetVersionID:   identityVersionID,
		StructureIdentitySetVersionHash: identityHash,
		StructureIdentitySet: contract.FrozenStructureIdentitySet{
			SchemaVersion: "structure-identity-set-production", ID: identityVersionID,
			WorkspaceID: workspaceID, ProjectID: projectID, Version: 1,
			GateInputID: uuid.NewString(), GateInputHash: "3" + sourceHash[1:],
			ReviewDecisionID: uuid.NewString(), ProjectEpisodeReceiptID: uuid.NewString(),
			DocumentRevisionID: sourceVersionID, SpanIndexID: uuid.NewString(),
			CandidateRefs: []contract.FrozenStructureIdentityCandidateRef{},
			EpisodeRefs: []contract.FrozenEpisodeRef{{
				TemporaryEpisodeID: "episode_0001", EpisodeID: episodeID, EpisodeRevision: 1,
				Position: 1, ScriptVersionID: uuid.NewString(), ScriptVersion: 1,
				SourceStart: 0, SourceEnd: 2, ContentHash: "4" + sourceHash[1:],
			}},
			SceneRefs: []contract.FrozenStructureIdentitySceneRef{{
				TemporaryEpisodeID: "episode_0001", EpisodeID: episodeID,
				TemporarySpanID: "span_0001", TemporarySceneID: "scene_0001",
				SceneOwnerLogicalID: sceneOwnerID, ScopeKey: "scene:" + sceneOwnerID,
				SourceStart: 0, SourceEnd: 2, EvidenceHash: "5" + sourceHash[1:],
			}},
			Identities: []contract.FrozenStructureIdentity{{
				TemporaryIdentityKey: "identity_character_linzhou", IdentityKey: "character:linzhou",
				Kind: "character", Resolution: "new", CanonicalName: text, Aliases: []string{text},
			}},
			MentionMappings: []contract.FrozenStructureIdentityMentionMapping{{
				Kind: "character", OccurrenceRole: "actual",
				TemporarySceneID: "scene_0001", SourceStart: 0, SourceEnd: 2,
				TextHash: sourceHash, ExactAnchor: text, Resolution: "resolved",
				IdentityKey: stringPointer("character:linzhou"),
			}},
			Coverage: contract.FrozenStructureIdentityCoverage{
				SceneCount: 1, IdentityCount: 1, MentionCount: 1, ResolvedCount: 1,
				MentionUniverseHash: "6" + sourceHash[1:], ScopeSetHash: "7" + sourceHash[1:],
			},
			ContentHash: identityHash, CreatedBy: uuid.NewString(),
			CreatedAt: time.Date(2026, time.September, 9, 8, 0, 0, 0, time.UTC),
		},
		SceneFactCandidateRevisionID:   sceneFactRevisionID,
		SceneFactCandidateRevisionHash: "a" + sourceHash[1:], SceneFactCandidate: sceneFactRaw,
	}
	basis := contract.ProductionSourceBasis{
		Provenance: "source_explicit", Evidence: []contract.SourceEvidenceSpan{evidence},
	}
	candidate := contract.ProductionEntityFragmentCandidate{
		SourceVersionID: sourceVersionID, SourceHash: sourceHash,
		StructureIdentitySetVersionID:   identityVersionID,
		StructureIdentitySetVersionHash: identityHash,
		SceneFactCandidateRevisionID:    sceneFactRevisionID,
		SceneFactCandidateRevisionHash:  input.SceneFactCandidateRevisionHash,
		Entities: []contract.ProductionEntityFragment{{
			IdentityKey: "character:linzhou", Kind: "character",
			SpecificationKey: "specification_character_linzhou",
			SpecificationSlots: []contract.ProductionSemanticSlot{{
				SlotKey: "canonical_name", Resolution: "known", Value: stringPointer(text),
			}},
			Basis: basis,
			States: []contract.ProductionStateFragment{{
				StateKey: "state_character_linzhou_initial", StateKind: "character_appearance",
				CompleteSlots: []contract.ProductionSemanticSlot{{
					SlotKey: "baseline", Resolution: "known", Value: stringPointer(text),
				}},
				ApplicableSceneScopeKeys: []string{"scene:" + sceneOwnerID},
				EntryReason:              "剧本首次出现", ExitReason: "剧本范围结束", Basis: basis,
			}},
		}},
		WorldClaims: []contract.ProductionWorldClaimFragment{},
		DesignGaps:  []contract.ProductionDesignGap{}, ReviewIssues: []contract.CandidateReviewIssue{},
	}
	return input, candidate
}

func stringPointer(value string) *string { return &value }

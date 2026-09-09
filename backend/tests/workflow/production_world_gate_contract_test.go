package workflow_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestProductionWorldCandidatePartitionsThreeFrozenFragments(t *testing.T) {
	draft := productionWorldCandidateDraft(t)
	value, encoded, err := worlddomain.NewProductionWorldCandidate(draft)
	if err != nil {
		t.Fatal(err)
	}
	if value.SchemaVersion != worlddomain.ProductionWorldCandidateSchemaVersion || len(value.ContentHash) != 64 {
		t.Fatalf("invalid Production World Candidate identity: %#v", value)
	}
	if len(value.SharedProof.PlanningEpisodeScopes) != 1 ||
		value.SharedProof.PlanningEpisodeScopes[0].EpisodeID != draft.FrozenInput.StructureIdentitySet.EpisodeRefs[0].EpisodeID ||
		!slices.Equal(value.SharedProof.PlanningEpisodeScopes[0].SceneScopeKeys, value.SharedProof.ScopeKeys) {
		t.Fatalf("Production World Candidate lost Episode to Scene closure: %#v", value.SharedProof.PlanningEpisodeScopes)
	}
	if len(value.Bible.Specifications) != 1 || len(value.Asset.Identities) != 1 ||
		len(value.Planning.Scenes) != 1 || len(value.Planning.SceneStoryTimes) != 1 {
		t.Fatalf("Production World partitions are incomplete: %#v", value)
	}
	if value.Bible.Specifications[0].IdentityKey != value.Asset.Identities[0].IdentityKey ||
		value.Asset.Identities[0].States[0].StateKey != value.Planning.Scenes[0].Occurrences[0].StateKey {
		t.Fatalf("cross-partition references drifted: %#v", value.SharedProof.CrossPartitionRefs)
	}
	if len(value.SharedProof.ExpectedBusinessKeyRoots) != 3 ||
		len(value.SharedProof.ScopeClosureRoot) != 64 || len(value.SharedProof.CrossPartitionRefRoot) != 64 ||
		len(value.InteractionProjectionHash) != 64 || len(value.ContinuityProjectionHash) != 64 {
		t.Fatalf("Production World proof is incomplete: %#v", value.SharedProof)
	}
	decoded, canonical, err := worlddomain.DecodeProductionWorldCandidate(encoded)
	if err != nil || decoded.ContentHash != value.ContentHash || string(canonical) != string(encoded) {
		t.Fatalf("decode Production World Candidate: decoded=%#v err=%v", decoded, err)
	}
}

func TestProductionWorldGateInputBindsAggregateAndSeparatePlanningProjections(t *testing.T) {
	candidateDraft := productionWorldCandidateDraft(t)
	candidate, _, err := worlddomain.NewProductionWorldCandidate(candidateDraft)
	if err != nil {
		t.Fatal(err)
	}
	draft := workflow.ProductionWorldGateInputDraft{
		WorkspaceID: candidateDraft.WorkspaceID, ProjectID: candidateDraft.ProjectID,
		WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(),
		CandidateRevisionID: uuid.NewString(), CandidateRevision: 1,
		CandidateRevisionHash: strings.Repeat("f", 64), Candidate: candidate,
		AllowedDecisions: []string{"rejected", "approved"},
		ExpectedHeads:    productionWorldExpectedHeads(candidate, 1, 2),
	}
	value, encoded, err := workflow.NewProductionWorldGateInput(draft)
	if err != nil {
		t.Fatal(err)
	}
	if value.GateKey != workflow.ProductionWorldGateKey || len(value.InputHash) != 64 ||
		value.Subject.ProductionWorldCandidate.CandidateContentHash != candidate.ContentHash ||
		value.Subject.SceneOccurrenceCandidate != candidate.UpstreamCandidates.SceneOccurrence ||
		value.Subject.InteractionCandidate.ProjectionHash != candidate.InteractionProjectionHash ||
		value.Subject.ContinuityCandidate.ProjectionHash != candidate.ContinuityProjectionHash ||
		value.Subject.InteractionCandidate.ProjectionKind != "interaction" ||
		value.Subject.ContinuityCandidate.ProjectionKind != "continuity" ||
		value.Subject.InteractionCandidate.Candidate != candidate.UpstreamCandidates.InteractionContinuity ||
		value.Subject.ContinuityCandidate.Candidate != candidate.UpstreamCandidates.InteractionContinuity ||
		value.Subject.StructureIdentitySetVersion.VersionID != candidate.StructureIdentitySetVersion.VersionID ||
		len(value.Subject.ExpectedHeads) != 4 || len(value.Subject.RepairTargets) != 4 {
		t.Fatalf("Gate 2 Subject is incomplete: %#v", value.Subject)
	}
	if !slices.Equal(value.Subject.RepairTargets[0].TargetKeys, []string{
		"character:linzhou", "state_character_linzhou_initial",
	}) || !slices.Equal(value.Subject.RepairTargets[1].TargetKeys, []string{
		"occurrence_scene_0001_0001", candidate.Planning.Scenes[0].SceneScopeKey,
	}) || value.Subject.RepairTargets[2].TargetKeys == nil || value.Subject.RepairTargets[3].TargetKeys == nil {
		t.Fatalf("Gate 2 repair target inventory is incomplete: %#v", value.Subject.RepairTargets)
	}
	if !slices.Equal(value.EffectPlan.AtomicStep.OwnerKinds, []string{"asset", "production/bible", "production/planning"}) ||
		value.EffectPlan.AtomicStep.OwnerCommand != "confirm_production_world" ||
		value.EffectPlan.AtomicStep.ReadSetRoot != value.Subject.ReadSetRoot {
		t.Fatalf("Gate 2 effect plan drifted: %#v", value.EffectPlan)
	}
	decoded, canonical, err := workflow.DecodeProductionWorldGateInput(encoded)
	if err != nil || decoded.InputHash != value.InputHash || string(canonical) != string(encoded) {
		t.Fatalf("decode Gate 2 input: decoded=%#v err=%v", decoded, err)
	}
}

func TestProductionWorldContractsRejectUpstreamAndHashDrift(t *testing.T) {
	draft := productionWorldCandidateDraft(t)
	draft.SceneOccurrenceCandidate.CandidateRevisionHash = strings.Repeat("0", 64)
	if _, _, err := worlddomain.NewProductionWorldCandidate(draft); err == nil {
		t.Fatal("Production World Candidate accepted a SceneOccurrence revision hash outside its frozen input")
	}

	draft = productionWorldCandidateDraft(t)
	candidate, _, err := worlddomain.NewProductionWorldCandidate(draft)
	if err != nil {
		t.Fatal(err)
	}
	gateDraft := workflow.ProductionWorldGateInputDraft{
		WorkspaceID: draft.WorkspaceID, ProjectID: draft.ProjectID,
		WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(),
		CandidateRevisionID: uuid.NewString(), CandidateRevision: 1,
		CandidateRevisionHash: strings.Repeat("e", 64), Candidate: candidate,
		AllowedDecisions: []string{"approved", "rejected"},
		ExpectedHeads:    productionWorldExpectedHeads(candidate, 0, 0),
	}
	_, encoded, err := workflow.NewProductionWorldGateInput(gateDraft)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err = json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	subject := raw["subject"].(map[string]any)
	interaction := subject["interaction_candidate"].(map[string]any)
	interaction["projection_hash"] = strings.Repeat("1", 64)
	drifted, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = workflow.DecodeProductionWorldGateInput(drifted); err == nil {
		t.Fatal("Gate 2 accepted a drifted Interaction projection")
	}

	_, encoded, err = workflow.NewProductionWorldGateInput(gateDraft)
	if err != nil || json.Unmarshal(encoded, &raw) != nil {
		t.Fatal(err)
	}
	subject = raw["subject"].(map[string]any)
	repairTargets := subject["repair_targets"].([]any)
	firstTarget := repairTargets[0].(map[string]any)
	firstTarget["target_keys"] = append(firstTarget["target_keys"].([]any), "character:outside_frozen_candidate")
	drifted, err = json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = workflow.DecodeProductionWorldGateInput(drifted); err == nil {
		t.Fatal("Gate 2 accepted a repair target outside the frozen Candidate")
	}
}

func productionWorldCandidateDraft(t *testing.T) worlddomain.ProductionWorldCandidateDraft {
	t.Helper()
	text := "林舟"
	sourceHash := sha256Text(text)
	evidence := agentcontract.SourceEvidenceSpan{SourceStart: 0, SourceEnd: 2, TextHash: sourceHash, ExactAnchor: text}
	workspaceID, projectID := uuid.NewString(), uuid.NewString()
	sourceVersionID, identityVersionID := uuid.NewString(), uuid.NewString()
	sceneOwnerID, episodeID := uuid.NewString(), uuid.NewString()
	sceneFactRevisionID := uuid.NewString()
	sceneFact := agentcontract.SceneFactCandidate{
		SourceVersionID: sourceVersionID, SourceHash: sourceHash,
		SpanCandidateRevisionID: uuid.NewString(), SpanCandidateRevisionHash: "2" + sourceHash[1:],
		Scenes: []agentcontract.SceneFact{{
			TemporarySceneID: "scene_0001", SpanID: "span_0001", SourceStart: 0, SourceEnd: 2,
			Actions: []agentcontract.GroundedAction{}, Dialogues: []agentcontract.GroundedDialogue{},
			RawCharacterMentions: []agentcontract.RawEntityMention{{Text: text, OccurrenceRole: "actual", Evidence: evidence}},
			RawPropMentions:      []agentcontract.RawEntityMention{},
		}},
		ReviewIssues: []agentcontract.CandidateReviewIssue{},
	}
	sceneFactRaw := mustJSON(t, sceneFact)
	identityHash := "1" + sourceHash[1:]
	identitySet := agentcontract.FrozenStructureIdentitySet{
		SchemaVersion: "structure-identity-set-production", ID: identityVersionID,
		WorkspaceID: workspaceID, ProjectID: projectID, Version: 1,
		GateInputID: uuid.NewString(), GateInputHash: "3" + sourceHash[1:],
		ReviewDecisionID: uuid.NewString(), ProjectEpisodeReceiptID: uuid.NewString(),
		DocumentRevisionID: sourceVersionID, SpanIndexID: uuid.NewString(),
		CandidateRefs: []agentcontract.FrozenStructureIdentityCandidateRef{},
		EpisodeRefs: []agentcontract.FrozenEpisodeRef{{
			TemporaryEpisodeID: "episode_0001", EpisodeID: episodeID, EpisodeRevision: 1, Position: 1,
			ScriptVersionID: uuid.NewString(), ScriptVersion: 1, SourceStart: 0, SourceEnd: 2,
			ContentHash: "4" + sourceHash[1:],
		}},
		SceneRefs: []agentcontract.FrozenStructureIdentitySceneRef{{
			TemporaryEpisodeID: "episode_0001", EpisodeID: episodeID, TemporarySpanID: "span_0001",
			TemporarySceneID: "scene_0001", SceneOwnerLogicalID: sceneOwnerID,
			ScopeKey: "scene:" + sceneOwnerID, SourceStart: 0, SourceEnd: 2,
			EvidenceHash: "5" + sourceHash[1:],
		}},
		Identities: []agentcontract.FrozenStructureIdentity{{
			TemporaryIdentityKey: "identity_character_linzhou", IdentityKey: "character:linzhou",
			Kind: "character", Resolution: "new", CanonicalName: text, Aliases: []string{text},
		}},
		MentionMappings: []agentcontract.FrozenStructureIdentityMentionMapping{{
			Kind: "character", OccurrenceRole: "actual", TemporarySceneID: "scene_0001",
			SourceStart: 0, SourceEnd: 2, TextHash: sourceHash, ExactAnchor: text,
			Resolution: "resolved", IdentityKey: stringRef("character:linzhou"),
		}},
		Coverage: agentcontract.FrozenStructureIdentityCoverage{
			SceneCount: 1, IdentityCount: 1, MentionCount: 1, ResolvedCount: 1,
			MentionUniverseHash: "6" + sourceHash[1:], ScopeSetHash: "7" + sourceHash[1:],
		},
		ContentHash: identityHash, CreatedBy: uuid.NewString(),
		CreatedAt: time.Date(2026, time.September, 9, 8, 0, 0, 0, time.UTC),
	}
	productionInput := agentcontract.ProductionEntityDerivationInput{
		SourceVersionID: sourceVersionID, SourceHash: sourceHash, NormalizedText: text,
		StructureIdentitySetVersionID: identityVersionID, StructureIdentitySetVersionHash: identityHash,
		StructureIdentitySet: identitySet, SceneFactCandidateRevisionID: sceneFactRevisionID,
		SceneFactCandidateRevisionHash: "a" + sourceHash[1:], SceneFactCandidate: sceneFactRaw,
	}
	basis := agentcontract.ProductionSourceBasis{Provenance: "source_explicit", Evidence: []agentcontract.SourceEvidenceSpan{evidence}}
	productionCandidate := agentcontract.ProductionEntityFragmentCandidate{
		SourceVersionID: sourceVersionID, SourceHash: sourceHash,
		StructureIdentitySetVersionID: identityVersionID, StructureIdentitySetVersionHash: identityHash,
		SceneFactCandidateRevisionID:   sceneFactRevisionID,
		SceneFactCandidateRevisionHash: productionInput.SceneFactCandidateRevisionHash,
		Entities: []agentcontract.ProductionEntityFragment{{
			IdentityKey: "character:linzhou", Kind: "character", SpecificationKey: "specification_character_linzhou",
			SpecificationSlots: []agentcontract.ProductionSemanticSlot{{SlotKey: "canonical_name", Resolution: "known", Value: stringRef(text)}},
			Basis:              basis,
			States: []agentcontract.ProductionStateFragment{{
				StateKey: "state_character_linzhou_initial", StateKind: "character_appearance",
				CompleteSlots:            []agentcontract.ProductionSemanticSlot{{SlotKey: "baseline", Resolution: "known", Value: stringRef(text)}},
				ApplicableSceneScopeKeys: []string{"scene:" + sceneOwnerID}, EntryReason: "剧本首次出现",
				ExitReason: "剧本范围结束", Basis: basis,
			}},
		}},
		WorldClaims: []agentcontract.ProductionWorldClaimFragment{},
		DesignGaps:  []agentcontract.ProductionDesignGap{}, ReviewIssues: []agentcontract.CandidateReviewIssue{},
	}
	productionRaw := mustJSON(t, productionCandidate)
	productionRef := candidateRef("derive_production_entities", "b", sourceHash)
	bindingInput := agentcontract.SceneOccurrenceBindingInput{
		ProductionEntityDerivationInput:       productionInput,
		ProductionEntityCandidateRevisionID:   productionRef.CandidateRevisionID,
		ProductionEntityCandidateRevisionHash: productionRef.CandidateRevisionHash,
		ProductionEntityCandidate:             productionRaw,
	}
	bindingCandidate := agentcontract.SceneBindingFragmentCandidate{
		SourceVersionID: sourceVersionID, SourceHash: sourceHash,
		StructureIdentitySetVersionID: identityVersionID, StructureIdentitySetVersionHash: identityHash,
		SceneFactCandidateRevisionID:          sceneFactRevisionID,
		SceneFactCandidateRevisionHash:        productionInput.SceneFactCandidateRevisionHash,
		ProductionEntityCandidateRevisionID:   productionRef.CandidateRevisionID,
		ProductionEntityCandidateRevisionHash: productionRef.CandidateRevisionHash,
		Scenes: []agentcontract.SceneBindingFragment{{
			SceneScopeKey: "scene:" + sceneOwnerID, SceneOwnerLogicalID: sceneOwnerID,
			TemporarySceneID: "scene_0001", SourceStart: 0, SourceEnd: 2,
			Dialogues: []agentcontract.SceneDialogueFragment{}, Beats: []agentcontract.SceneBeatFragment{},
			Occurrences: []agentcontract.SceneOccurrenceFragment{{
				OccurrenceKey: "occurrence_scene_0001_0001", Order: 1, SubjectKind: "character",
				IdentityKey: "character:linzhou", StateKey: "state_character_linzhou_initial",
				OccurrenceRole: "actual", Evidence: evidence,
			}},
		}},
		ReviewIssues: []agentcontract.CandidateReviewIssue{},
	}
	bindingRaw := mustJSON(t, bindingCandidate)
	bindingRef := candidateRef("bind_scene_occurrences", "c", sourceHash)
	continuityInput := agentcontract.InteractionContinuityInput{
		SceneOccurrenceBindingInput:       bindingInput,
		SceneBindingCandidateRevisionID:   bindingRef.CandidateRevisionID,
		SceneBindingCandidateRevisionHash: bindingRef.CandidateRevisionHash,
		SceneBindingCandidate:             bindingRaw,
	}
	continuityCandidate := agentcontract.InteractionContinuityCandidate{
		SourceVersionID: sourceVersionID, SourceHash: sourceHash,
		StructureIdentitySetVersionID: identityVersionID, StructureIdentitySetVersionHash: identityHash,
		SceneFactCandidateRevisionID:          sceneFactRevisionID,
		SceneFactCandidateRevisionHash:        productionInput.SceneFactCandidateRevisionHash,
		ProductionEntityCandidateRevisionID:   productionRef.CandidateRevisionID,
		ProductionEntityCandidateRevisionHash: productionRef.CandidateRevisionHash,
		SceneBindingCandidateRevisionID:       bindingRef.CandidateRevisionID,
		SceneBindingCandidateRevisionHash:     bindingRef.CandidateRevisionHash,
		SceneStoryTimes:                       []agentcontract.SceneStoryTimeFragment{{SceneScopeKey: "scene:" + sceneOwnerID, StoryTimeKey: "storytime:00000001"}},
		Interactions:                          []agentcontract.InteractionFragment{},
		ContinuityLedger: []agentcontract.ContinuityLedgerEntry{{
			LedgerKey: "ledger_character_linzhou_scene_0001", SubjectKind: "character",
			IdentityKey: "character:linzhou", SceneScopeKey: "scene:" + sceneOwnerID,
			StoryTimeKey: "storytime:00000001", StateKey: "state_character_linzhou_initial",
			Evidence: []agentcontract.SourceEvidenceSpan{evidence},
		}},
		Continuity: []agentcontract.ContinuityFragment{}, ReviewIssues: []agentcontract.CandidateReviewIssue{},
	}
	continuityRef := candidateRef("reconcile_interaction_continuity", "d", sourceHash)
	return worlddomain.ProductionWorldCandidateDraft{
		WorkspaceID: workspaceID, ProjectID: projectID,
		SourceVersion: agentcontract.ScriptSourceVersionIdentity{
			OwnerKind: "production/script", LogicalID: uuid.NewString(), VersionID: sourceVersionID,
			Revision: 1, ContentHash: sourceHash,
			CreatedAt: time.Date(2026, time.September, 9, 7, 0, 0, 0, time.UTC),
		},
		FrozenInput:               continuityInput,
		ProductionEntityCandidate: productionRef, SceneOccurrenceCandidate: bindingRef,
		InteractionContinuityCandidate:     continuityRef,
		InteractionContinuityCandidateBody: mustJSON(t, continuityCandidate),
	}
}

func candidateRef(stage, prefix, sourceHash string) agentcontract.SceneAnalysisCandidateRevisionIdentity {
	return agentcontract.SceneAnalysisCandidateRevisionIdentity{
		StageKey: stage, ShardKey: "script:full", CandidateRevisionID: uuid.NewString(),
		CandidateRevisionHash: prefix + sourceHash[1:], SourceInvocationID: uuid.NewString(),
		SourceResultHash: prefix + sourceHash[1:],
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func sha256Text(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func stringRef(value string) *string { return &value }

func productionWorldExpectedHeads(
	candidate worlddomain.ProductionWorldCandidate,
	bibleRevision, episodeRevision int64,
) []workflow.ProductionWorldExpectedHead {
	bibleHash, episodeHash := "", ""
	if bibleRevision > 0 {
		bibleHash = strings.Repeat("c", 64)
	}
	if episodeRevision > 0 {
		episodeHash = strings.Repeat("d", 64)
	}
	episode := candidate.SharedProof.PlanningEpisodeScopes[0]
	return []workflow.ProductionWorldExpectedHead{
		{OwnerKind: "asset", VersionFamily: "asset_identity_state_set", ScopeKind: "project", ScopeKey: "project:" + candidate.ProjectID},
		{OwnerKind: "production/bible", VersionFamily: "bible_production_world_set", ScopeKind: "project", ScopeKey: "project:" + candidate.ProjectID, Revision: bibleRevision, ContentHash: bibleHash},
		{OwnerKind: "production/planning", VersionFamily: "planning_scene_set", ScopeKind: "episode", ScopeKey: episode.ScopeKey, Revision: episodeRevision, ContentHash: episodeHash},
		{OwnerKind: "production/planning", VersionFamily: "planning_structure_rebase_set", ScopeKind: "project", ScopeKey: "project:" + candidate.ProjectID},
	}
}

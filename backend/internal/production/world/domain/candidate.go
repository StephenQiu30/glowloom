package domain

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

const ProductionWorldCandidateSchemaVersion = "production-world-candidate-production"

var productionWorldContentHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ProductionWorldOwnerVersionRef struct {
	OwnerKind   string `json:"owner_kind"`
	LogicalID   string `json:"logical_id"`
	VersionID   string `json:"version_id"`
	Revision    int64  `json:"revision"`
	ContentHash string `json:"content_hash"`
}

type ProductionWorldUpstreamCandidates struct {
	ProductionEntity      agentcontract.SceneAnalysisCandidateRevisionIdentity `json:"production_entity"`
	SceneOccurrence       agentcontract.SceneAnalysisCandidateRevisionIdentity `json:"scene_occurrence"`
	InteractionContinuity agentcontract.SceneAnalysisCandidateRevisionIdentity `json:"interaction_continuity"`
}

type ProductionSpecificationCandidate struct {
	IdentityKey        string                                 `json:"identity_key"`
	Kind               string                                 `json:"kind"`
	SpecificationKey   string                                 `json:"specification_key"`
	SpecificationSlots []agentcontract.ProductionSemanticSlot `json:"specification_slots"`
	Basis              agentcontract.ProductionSourceBasis    `json:"basis"`
}

type ProductionAssetIdentityCandidate struct {
	IdentityKey string                                  `json:"identity_key"`
	Kind        string                                  `json:"kind"`
	States      []agentcontract.ProductionStateFragment `json:"states"`
}

type ProductionWorldBiblePartition struct {
	Specifications []ProductionSpecificationCandidate           `json:"specifications"`
	WorldClaims    []agentcontract.ProductionWorldClaimFragment `json:"world_claims"`
}

type ProductionWorldPlanningPartition struct {
	Scenes          []agentcontract.SceneBindingFragment   `json:"scenes"`
	SceneStoryTimes []agentcontract.SceneStoryTimeFragment `json:"scene_story_times"`
	Interactions    []agentcontract.InteractionFragment    `json:"interactions"`
	Continuity      []agentcontract.ContinuityFragment     `json:"continuity"`
}

type ProductionWorldAssetPartition struct {
	Identities []ProductionAssetIdentityCandidate `json:"identities"`
}

type ProductionWorldExpectedBusinessKeySet struct {
	Partition string   `json:"partition"`
	Keys      []string `json:"keys"`
	Root      string   `json:"root"`
}

type ProductionWorldCrossPartitionRef struct {
	SourceKey string `json:"source_key"`
	Relation  string `json:"relation"`
	TargetKey string `json:"target_key"`
}

type ProductionWorldReviewIssue struct {
	SourceStage string                             `json:"source_stage"`
	Issue       agentcontract.CandidateReviewIssue `json:"issue"`
}

type ProductionWorldPlanningEpisodeScope struct {
	EpisodeID      string   `json:"episode_id"`
	ScopeKey       string   `json:"scope_key"`
	SceneScopeKeys []string `json:"scene_scope_keys"`
}

type ProductionWorldSharedProof struct {
	ExpectedBusinessKeyRoots []ProductionWorldExpectedBusinessKeySet `json:"expected_business_key_roots"`
	PlanningEpisodeScopes    []ProductionWorldPlanningEpisodeScope   `json:"planning_episode_scopes"`
	ScopeKeys                []string                                `json:"scope_keys"`
	ScopeClosureRoot         string                                  `json:"scope_closure_root"`
	CrossPartitionRefs       []ProductionWorldCrossPartitionRef      `json:"cross_partition_refs"`
	CrossPartitionRefRoot    string                                  `json:"cross_partition_ref_root"`
	DesignGaps               []agentcontract.ProductionDesignGap     `json:"design_gaps"`
	ContinuityLedger         []agentcontract.ContinuityLedgerEntry   `json:"continuity_ledger"`
	ReviewIssues             []ProductionWorldReviewIssue            `json:"review_issues"`
}

type ProductionWorldPartitionRoots struct {
	Bible    string `json:"bible"`
	Planning string `json:"planning"`
	Asset    string `json:"asset"`
	Proof    string `json:"proof"`
}

type ProductionWorldCandidate struct {
	SchemaVersion               string                                    `json:"schema_version"`
	WorkspaceID                 string                                    `json:"workspace_id"`
	ProjectID                   string                                    `json:"project_id"`
	SourceVersion               agentcontract.ScriptSourceVersionIdentity `json:"source_version"`
	StructureIdentitySetVersion ProductionWorldOwnerVersionRef            `json:"structure_identity_set_version"`
	UpstreamCandidates          ProductionWorldUpstreamCandidates         `json:"upstream_candidates"`
	Bible                       ProductionWorldBiblePartition             `json:"bible_partition"`
	Planning                    ProductionWorldPlanningPartition          `json:"planning_partition"`
	Asset                       ProductionWorldAssetPartition             `json:"asset_partition"`
	SharedProof                 ProductionWorldSharedProof                `json:"shared_proof"`
	PartitionRoots              ProductionWorldPartitionRoots             `json:"partition_roots"`
	InteractionProjectionHash   string                                    `json:"interaction_projection_hash"`
	ContinuityProjectionHash    string                                    `json:"continuity_projection_hash"`
	ContentHash                 string                                    `json:"content_hash"`
}

type ProductionWorldCandidateDraft struct {
	WorkspaceID, ProjectID             string
	SourceVersion                      agentcontract.ScriptSourceVersionIdentity
	FrozenInput                        agentcontract.InteractionContinuityInput
	ProductionEntityCandidate          agentcontract.SceneAnalysisCandidateRevisionIdentity
	SceneOccurrenceCandidate           agentcontract.SceneAnalysisCandidateRevisionIdentity
	InteractionContinuityCandidate     agentcontract.SceneAnalysisCandidateRevisionIdentity
	InteractionContinuityCandidateBody json.RawMessage
}

func NewProductionWorldCandidate(
	draft ProductionWorldCandidateDraft,
) (ProductionWorldCandidate, json.RawMessage, error) {
	if err := validateProductionWorldCandidateDraft(draft); err != nil {
		return ProductionWorldCandidate{}, nil, err
	}
	var production agentcontract.ProductionEntityFragmentCandidate
	var scenes agentcontract.SceneBindingFragmentCandidate
	var continuity agentcontract.InteractionContinuityCandidate
	if decodeProductionWorldJSON(draft.FrozenInput.ProductionEntityCandidate, &production) != nil ||
		decodeProductionWorldJSON(draft.FrozenInput.SceneBindingCandidate, &scenes) != nil ||
		decodeProductionWorldJSON(draft.InteractionContinuityCandidateBody, &continuity) != nil {
		return ProductionWorldCandidate{}, nil, errors.New("invalid Production World fragment encoding")
	}

	specifications := make([]ProductionSpecificationCandidate, 0, len(production.Entities))
	identities := make([]ProductionAssetIdentityCandidate, 0, len(production.Entities))
	for _, entity := range production.Entities {
		specifications = append(specifications, ProductionSpecificationCandidate{
			IdentityKey: entity.IdentityKey, Kind: entity.Kind, SpecificationKey: entity.SpecificationKey,
			SpecificationSlots: append([]agentcontract.ProductionSemanticSlot(nil), entity.SpecificationSlots...),
			Basis:              entity.Basis,
		})
		identities = append(identities, ProductionAssetIdentityCandidate{
			IdentityKey: entity.IdentityKey, Kind: entity.Kind,
			States: append([]agentcontract.ProductionStateFragment(nil), entity.States...),
		})
	}
	value := ProductionWorldCandidate{
		SchemaVersion: ProductionWorldCandidateSchemaVersion,
		WorkspaceID:   strings.TrimSpace(draft.WorkspaceID), ProjectID: strings.TrimSpace(draft.ProjectID),
		SourceVersion: draft.SourceVersion,
		StructureIdentitySetVersion: ProductionWorldOwnerVersionRef{
			OwnerKind: "production/bible", LogicalID: draft.ProjectID,
			VersionID:   draft.FrozenInput.StructureIdentitySetVersionID,
			Revision:    int64(draft.FrozenInput.StructureIdentitySet.Version),
			ContentHash: draft.FrozenInput.StructureIdentitySetVersionHash,
		},
		UpstreamCandidates: ProductionWorldUpstreamCandidates{
			ProductionEntity:      draft.ProductionEntityCandidate,
			SceneOccurrence:       draft.SceneOccurrenceCandidate,
			InteractionContinuity: draft.InteractionContinuityCandidate,
		},
		Bible: ProductionWorldBiblePartition{
			Specifications: specifications,
			WorldClaims:    append([]agentcontract.ProductionWorldClaimFragment{}, production.WorldClaims...),
		},
		Planning: ProductionWorldPlanningPartition{
			Scenes:          append([]agentcontract.SceneBindingFragment{}, scenes.Scenes...),
			SceneStoryTimes: append([]agentcontract.SceneStoryTimeFragment{}, continuity.SceneStoryTimes...),
			Interactions:    append([]agentcontract.InteractionFragment{}, continuity.Interactions...),
			Continuity:      append([]agentcontract.ContinuityFragment{}, continuity.Continuity...),
		},
		Asset: ProductionWorldAssetPartition{Identities: identities},
		SharedProof: ProductionWorldSharedProof{
			DesignGaps:       append([]agentcontract.ProductionDesignGap{}, production.DesignGaps...),
			ContinuityLedger: append([]agentcontract.ContinuityLedgerEntry{}, continuity.ContinuityLedger...),
			ReviewIssues:     productionWorldReviewIssues(production, scenes, continuity),
		},
	}
	episodeScopes, err := productionWorldEpisodeScopes(draft.FrozenInput.StructureIdentitySet, value.Planning.Scenes)
	if err != nil {
		return ProductionWorldCandidate{}, nil, err
	}
	value.SharedProof.PlanningEpisodeScopes = episodeScopes
	if err := completeProductionWorldCandidate(&value, false); err != nil {
		return ProductionWorldCandidate{}, nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ProductionWorldCandidate{}, nil, err
	}
	return value, encoded, nil
}

func DecodeProductionWorldCandidate(
	raw json.RawMessage,
) (ProductionWorldCandidate, json.RawMessage, error) {
	var value ProductionWorldCandidate
	if err := decodeProductionWorldJSON(raw, &value); err != nil {
		return ProductionWorldCandidate{}, nil, errors.New("invalid Production World Candidate")
	}
	original := value
	if err := completeProductionWorldCandidate(&value, true); err != nil || !reflect.DeepEqual(original, value) {
		return ProductionWorldCandidate{}, nil, errors.New("Production World Candidate proof mismatch")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ProductionWorldCandidate{}, nil, err
	}
	return value, encoded, nil
}

func validateProductionWorldCandidateDraft(value ProductionWorldCandidateDraft) error {
	for _, identifier := range []string{value.WorkspaceID, value.ProjectID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Production World aggregate identity")
		}
	}
	if value.SourceVersion.Validate() != nil || value.FrozenInput.Validate() != nil ||
		agentcontract.ValidateInteractionContinuityCandidate(
			value.InteractionContinuityCandidateBody,
			value.FrozenInput,
		) != nil || value.FrozenInput.StructureIdentitySet.WorkspaceID != value.WorkspaceID ||
		value.FrozenInput.StructureIdentitySet.ProjectID != value.ProjectID ||
		value.SourceVersion.VersionID != value.FrozenInput.SourceVersionID ||
		value.SourceVersion.ContentHash != value.FrozenInput.SourceHash {
		return errors.New("invalid Production World frozen input")
	}
	expected := []struct {
		stage string
		id    string
		hash  string
		ref   agentcontract.SceneAnalysisCandidateRevisionIdentity
	}{
		{"derive_production_entities", value.FrozenInput.ProductionEntityCandidateRevisionID,
			value.FrozenInput.ProductionEntityCandidateRevisionHash, value.ProductionEntityCandidate},
		{"bind_scene_occurrences", value.FrozenInput.SceneBindingCandidateRevisionID,
			value.FrozenInput.SceneBindingCandidateRevisionHash, value.SceneOccurrenceCandidate},
		{"reconcile_interaction_continuity", value.InteractionContinuityCandidate.CandidateRevisionID,
			value.InteractionContinuityCandidate.CandidateRevisionHash, value.InteractionContinuityCandidate},
	}
	seen := make(map[string]struct{}, len(expected))
	for _, item := range expected {
		if item.ref.Validate() != nil || item.ref.StageKey != item.stage || item.ref.ShardKey != "script:full" ||
			item.ref.CandidateRevisionID != item.id || item.ref.CandidateRevisionHash != item.hash {
			return errors.New("Production World Candidate revision set drifted")
		}
		if _, duplicate := seen[item.ref.CandidateRevisionID]; duplicate {
			return errors.New("Production World Candidate revision set is duplicated")
		}
		seen[item.ref.CandidateRevisionID] = struct{}{}
	}
	return nil
}

func completeProductionWorldCandidate(value *ProductionWorldCandidate, verify bool) error {
	if value.SchemaVersion != ProductionWorldCandidateSchemaVersion || value.SourceVersion.Validate() != nil {
		return errors.New("invalid Production World Candidate identity")
	}
	for _, identifier := range []string{value.WorkspaceID, value.ProjectID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Production World Candidate owner")
		}
	}
	if value.StructureIdentitySetVersion.OwnerKind != "production/bible" ||
		value.StructureIdentitySetVersion.LogicalID != value.ProjectID ||
		value.StructureIdentitySetVersion.Revision < 1 ||
		!productionWorldContentHashPattern.MatchString(value.StructureIdentitySetVersion.ContentHash) {
		return errors.New("invalid Production World StructureIdentitySet version")
	}
	if _, err := uuid.Parse(value.StructureIdentitySetVersion.VersionID); err != nil {
		return errors.New("invalid Production World StructureIdentitySet version")
	}
	upstreams := []struct {
		stage string
		ref   agentcontract.SceneAnalysisCandidateRevisionIdentity
	}{
		{"derive_production_entities", value.UpstreamCandidates.ProductionEntity},
		{"bind_scene_occurrences", value.UpstreamCandidates.SceneOccurrence},
		{"reconcile_interaction_continuity", value.UpstreamCandidates.InteractionContinuity},
	}
	for _, upstream := range upstreams {
		if upstream.ref.Validate() != nil || upstream.ref.StageKey != upstream.stage || upstream.ref.ShardKey != "script:full" {
			return errors.New("invalid Production World upstream Candidate")
		}
	}
	if value.Bible.Specifications == nil || value.Bible.WorldClaims == nil || value.Planning.Scenes == nil ||
		value.Planning.SceneStoryTimes == nil || value.Planning.Interactions == nil || value.Planning.Continuity == nil ||
		value.Asset.Identities == nil || value.SharedProof.PlanningEpisodeScopes == nil || value.SharedProof.DesignGaps == nil ||
		value.SharedProof.ContinuityLedger == nil || value.SharedProof.ReviewIssues == nil {
		return errors.New("Production World partitions are incomplete")
	}
	if err := validateProductionWorldGraph(value); err != nil {
		return err
	}

	expectedKeys, err := productionWorldExpectedKeys(*value)
	if err != nil {
		return err
	}
	scopeKeys := make([]string, 0, len(value.Planning.Scenes))
	for _, scene := range value.Planning.Scenes {
		scopeKeys = append(scopeKeys, scene.SceneScopeKey)
	}
	slices.Sort(scopeKeys)
	if !uniqueNonempty(scopeKeys) {
		return errors.New("Production World scope closure is invalid")
	}
	refs := productionWorldCrossRefs(*value)
	roots := ProductionWorldPartitionRoots{
		Bible:    hashProductionWorldMaterial(value.Bible),
		Planning: hashProductionWorldMaterial(value.Planning),
		Asset:    hashProductionWorldMaterial(value.Asset),
	}
	proof := value.SharedProof
	proof.ExpectedBusinessKeyRoots = expectedKeys
	proof.ScopeKeys = scopeKeys
	proof.ScopeClosureRoot = hashProductionWorldMaterial(scopeKeys)
	proof.CrossPartitionRefs = refs
	proof.CrossPartitionRefRoot = hashProductionWorldMaterial(refs)
	roots.Proof = hashProductionWorldMaterial(proof)
	interactionHash := hashProductionWorldMaterial(struct {
		SceneStoryTimes []agentcontract.SceneStoryTimeFragment `json:"scene_story_times"`
		Interactions    []agentcontract.InteractionFragment    `json:"interactions"`
	}{value.Planning.SceneStoryTimes, value.Planning.Interactions})
	continuityHash := hashProductionWorldMaterial(struct {
		ContinuityLedger []agentcontract.ContinuityLedgerEntry `json:"continuity_ledger"`
		Continuity       []agentcontract.ContinuityFragment    `json:"continuity"`
	}{value.SharedProof.ContinuityLedger, value.Planning.Continuity})
	if verify && (!reflect.DeepEqual(value.SharedProof, proof) || value.PartitionRoots != roots ||
		value.InteractionProjectionHash != interactionHash || value.ContinuityProjectionHash != continuityHash) {
		return errors.New("Production World derived roots drifted")
	}
	value.SharedProof, value.PartitionRoots = proof, roots
	value.InteractionProjectionHash, value.ContinuityProjectionHash = interactionHash, continuityHash
	originalHash := value.ContentHash
	value.ContentHash = ""
	contentHash := hashProductionWorldMaterial(productionWorldCandidateHashMaterial(*value))
	if verify && originalHash != contentHash {
		return errors.New("Production World Candidate content hash drifted")
	}
	value.ContentHash = contentHash
	return nil
}

func validateProductionWorldGraph(value *ProductionWorldCandidate) error {
	identities := make(map[string]string, len(value.Asset.Identities))
	states := make(map[string]string)
	specifications := make(map[string]string, len(value.Bible.Specifications))
	previous := ""
	for _, identity := range value.Asset.Identities {
		if identity.IdentityKey <= previous || !validProductionWorldKind(identity.Kind) || len(identity.States) == 0 {
			return errors.New("Production World Asset identities are not canonical")
		}
		identities[identity.IdentityKey] = identity.Kind
		previousState := ""
		for _, state := range identity.States {
			if state.StateKey <= previousState || state.StateKind != productionWorldStateKind(identity.Kind) {
				return errors.New("Production World State is not canonical")
			}
			if _, duplicate := states[state.StateKey]; duplicate {
				return errors.New("Production World State key is duplicated")
			}
			states[state.StateKey] = identity.IdentityKey
			previousState = state.StateKey
		}
		previous = identity.IdentityKey
	}
	previous = ""
	for _, specification := range value.Bible.Specifications {
		if specification.IdentityKey <= previous || identities[specification.IdentityKey] != specification.Kind ||
			!strings.HasPrefix(specification.SpecificationKey, "specification_") {
			return errors.New("Production World Specification identity is invalid")
		}
		if _, duplicate := specifications[specification.SpecificationKey]; duplicate {
			return errors.New("Production World Specification key is duplicated")
		}
		specifications[specification.SpecificationKey] = specification.IdentityKey
		previous = specification.IdentityKey
	}
	if len(value.Bible.Specifications) != len(identities) {
		return errors.New("Production World Specification coverage is incomplete")
	}
	scenes := make(map[string]struct{}, len(value.Planning.Scenes))
	occurrences := make(map[string]agentcontract.SceneOccurrenceFragment)
	for _, scene := range value.Planning.Scenes {
		if _, duplicate := scenes[scene.SceneScopeKey]; duplicate {
			return errors.New("Production World Scene scope is duplicated")
		}
		scenes[scene.SceneScopeKey] = struct{}{}
		for _, occurrence := range scene.Occurrences {
			if _, duplicate := occurrences[occurrence.OccurrenceKey]; duplicate ||
				identities[occurrence.IdentityKey] != occurrence.SubjectKind ||
				states[occurrence.StateKey] != occurrence.IdentityKey {
				return errors.New("Production World Occurrence reference is invalid")
			}
			occurrences[occurrence.OccurrenceKey] = occurrence
		}
	}
	coveredScenes := make(map[string]struct{}, len(scenes))
	previousEpisodeScope := ""
	for _, episode := range value.SharedProof.PlanningEpisodeScopes {
		if episode.ScopeKey <= previousEpisodeScope || episode.ScopeKey != "episode:"+episode.EpisodeID ||
			len(episode.SceneScopeKeys) == 0 || !slices.IsSorted(episode.SceneScopeKeys) {
			return errors.New("Production World Planning Episode scope is not canonical")
		}
		if _, err := uuid.Parse(episode.EpisodeID); err != nil {
			return errors.New("Production World Planning Episode identity is invalid")
		}
		for index, sceneScopeKey := range episode.SceneScopeKeys {
			if _, exists := scenes[sceneScopeKey]; !exists ||
				(index > 0 && episode.SceneScopeKeys[index-1] == sceneScopeKey) {
				return errors.New("Production World Planning Episode Scene reference is invalid")
			}
			if _, duplicate := coveredScenes[sceneScopeKey]; duplicate {
				return errors.New("Production World Planning Scene belongs to multiple Episodes")
			}
			coveredScenes[sceneScopeKey] = struct{}{}
		}
		previousEpisodeScope = episode.ScopeKey
	}
	if len(coveredScenes) != len(scenes) {
		return errors.New("Production World Planning Episode coverage is incomplete")
	}
	if len(value.Planning.SceneStoryTimes) != len(scenes) {
		return errors.New("Production World story-time coverage is incomplete")
	}
	storyTimes := make(map[string]struct{}, len(value.Planning.SceneStoryTimes))
	for _, anchor := range value.Planning.SceneStoryTimes {
		if _, exists := scenes[anchor.SceneScopeKey]; !exists {
			return errors.New("Production World story-time reference is invalid")
		}
		if _, duplicate := storyTimes[anchor.SceneScopeKey]; duplicate {
			return errors.New("Production World story-time Scene is duplicated")
		}
		storyTimes[anchor.SceneScopeKey] = struct{}{}
	}
	for _, interaction := range value.Planning.Interactions {
		if _, exists := scenes[interaction.SceneScopeKey]; !exists ||
			occurrences[interaction.ActorOccurrenceKey].SubjectKind != "character" ||
			occurrences[interaction.PropOccurrenceKey].SubjectKind != "prop" ||
			states[interaction.PropStateBeforeKey] == "" || states[interaction.PropStateAfterKey] == "" {
			return errors.New("Production World Interaction reference is invalid")
		}
		if interaction.CounterpartyOccurrenceKey != nil && occurrences[*interaction.CounterpartyOccurrenceKey].SubjectKind != "character" {
			return errors.New("Production World Interaction counterparty is invalid")
		}
	}
	for _, continuity := range value.Planning.Continuity {
		if identities[continuity.IdentityKey] != continuity.SubjectKind ||
			states[continuity.BeforeStateKey] != continuity.IdentityKey ||
			states[continuity.AfterStateKey] != continuity.IdentityKey {
			return errors.New("Production World Continuity reference is invalid")
		}
		if _, exists := scenes[continuity.FromSceneScopeKey]; !exists {
			return errors.New("Production World Continuity Scene is invalid")
		}
		if _, exists := scenes[continuity.ToSceneScopeKey]; !exists {
			return errors.New("Production World Continuity Scene is invalid")
		}
	}
	subjects := make(map[string]struct{}, len(identities)+len(states)+len(specifications))
	for key := range identities {
		subjects[key] = struct{}{}
	}
	for key := range states {
		subjects[key] = struct{}{}
	}
	for key := range specifications {
		subjects[key] = struct{}{}
	}
	previous = ""
	for _, gap := range value.SharedProof.DesignGaps {
		if gap.GapKey <= previous {
			return errors.New("Production World DesignGap set is not canonical")
		}
		if _, exists := subjects[gap.SubjectKey]; !exists {
			return errors.New("Production World DesignGap subject is invalid")
		}
		previous = gap.GapKey
	}
	for _, entry := range value.SharedProof.ContinuityLedger {
		if identities[entry.IdentityKey] != entry.SubjectKind || states[entry.StateKey] != entry.IdentityKey {
			return errors.New("Production World Continuity ledger subject is invalid")
		}
		if _, exists := scenes[entry.SceneScopeKey]; !exists {
			return errors.New("Production World Continuity ledger Scene is invalid")
		}
	}
	if !slices.IsSortedFunc(value.SharedProof.ReviewIssues, compareProductionWorldReviewIssue) {
		return errors.New("Production World ReviewIssue set is not canonical")
	}
	for index := 1; index < len(value.SharedProof.ReviewIssues); index++ {
		if compareProductionWorldReviewIssue(value.SharedProof.ReviewIssues[index-1], value.SharedProof.ReviewIssues[index]) == 0 {
			return errors.New("Production World ReviewIssue is duplicated")
		}
	}
	return nil
}

func productionWorldEpisodeScopes(
	identitySet agentcontract.FrozenStructureIdentitySet,
	planningScenes []agentcontract.SceneBindingFragment,
) ([]ProductionWorldPlanningEpisodeScope, error) {
	episodes := make(map[string]*ProductionWorldPlanningEpisodeScope, len(identitySet.EpisodeRefs))
	result := make([]ProductionWorldPlanningEpisodeScope, len(identitySet.EpisodeRefs))
	for index, episode := range identitySet.EpisodeRefs {
		result[index] = ProductionWorldPlanningEpisodeScope{
			EpisodeID:      episode.EpisodeID,
			ScopeKey:       "episode:" + episode.EpisodeID,
			SceneScopeKeys: []string{},
		}
		episodes[episode.EpisodeID] = &result[index]
	}
	knownScenes := make(map[string]struct{}, len(planningScenes))
	for _, scene := range planningScenes {
		knownScenes[scene.SceneScopeKey] = struct{}{}
	}
	for _, scene := range identitySet.SceneRefs {
		episode := episodes[scene.EpisodeID]
		if episode == nil {
			return nil, errors.New("Production World Planning Scene has no Episode")
		}
		if _, exists := knownScenes[scene.ScopeKey]; !exists {
			return nil, errors.New("Production World Planning Scene coverage drifted")
		}
		episode.SceneScopeKeys = append(episode.SceneScopeKeys, scene.ScopeKey)
	}
	for index := range result {
		slices.Sort(result[index].SceneScopeKeys)
	}
	slices.SortFunc(result, func(left, right ProductionWorldPlanningEpisodeScope) int {
		return cmp.Compare(left.ScopeKey, right.ScopeKey)
	})
	return result, nil
}

func productionWorldExpectedKeys(value ProductionWorldCandidate) ([]ProductionWorldExpectedBusinessKeySet, error) {
	bible, planning, asset := []string{}, []string{}, []string{}
	for _, item := range value.Bible.Specifications {
		bible = append(bible, "specification:"+item.SpecificationKey)
	}
	for _, item := range value.Bible.WorldClaims {
		bible = append(bible, "world_claim:"+item.ClaimKey)
	}
	for _, scene := range value.Planning.Scenes {
		planning = append(planning, "scene:"+scene.SceneOwnerLogicalID)
		for _, dialogue := range scene.Dialogues {
			planning = append(planning, "dialogue:"+scene.SceneOwnerLogicalID+":"+dialogue.DialogueKey)
		}
		for _, beat := range scene.Beats {
			planning = append(planning, "beat:"+scene.SceneOwnerLogicalID+":"+beat.BeatKey)
		}
		for _, occurrence := range scene.Occurrences {
			planning = append(planning, "occurrence:"+occurrence.OccurrenceKey)
		}
	}
	for _, item := range value.Planning.Interactions {
		planning = append(planning, "interaction:"+item.InteractionKey)
	}
	for _, item := range value.Planning.Continuity {
		planning = append(planning, "continuity:"+item.ContinuityKey)
	}
	for _, identity := range value.Asset.Identities {
		asset = append(asset, "asset_identity:"+identity.IdentityKey)
		for _, state := range identity.States {
			asset = append(asset, "asset_state:"+state.StateKey)
		}
	}
	sets := []ProductionWorldExpectedBusinessKeySet{
		{Partition: "asset", Keys: asset},
		{Partition: "bible", Keys: bible},
		{Partition: "planning", Keys: planning},
	}
	for index := range sets {
		slices.Sort(sets[index].Keys)
		if !uniqueNonempty(sets[index].Keys) {
			return nil, errors.New("Production World expected business key is duplicated")
		}
		sets[index].Root = hashProductionWorldMaterial(sets[index].Keys)
	}
	return sets, nil
}

func productionWorldCrossRefs(value ProductionWorldCandidate) []ProductionWorldCrossPartitionRef {
	refs := make([]ProductionWorldCrossPartitionRef, 0)
	add := func(source, relation, target string) {
		refs = append(refs, ProductionWorldCrossPartitionRef{SourceKey: source, Relation: relation, TargetKey: target})
	}
	for _, specification := range value.Bible.Specifications {
		add("specification:"+specification.SpecificationKey, "specifies", "asset_identity:"+specification.IdentityKey)
		for _, slot := range specification.SpecificationSlots {
			if slot.DesignGapKey != nil {
				add("specification:"+specification.SpecificationKey, "has_design_gap", "design_gap:"+*slot.DesignGapKey)
			}
		}
	}
	for _, claim := range value.Bible.WorldClaims {
		for _, participant := range claim.Participants {
			add("world_claim:"+claim.ClaimKey, "claim_"+participant.Role, "asset_identity:"+participant.IdentityKey)
		}
		if claim.Narrative != nil {
			for _, anchor := range claim.Narrative.Anchors {
				add("world_claim:"+claim.ClaimKey, "claim_anchor", anchor.TargetKey)
			}
		}
	}
	for _, identity := range value.Asset.Identities {
		for _, state := range identity.States {
			add("asset_state:"+state.StateKey, "belongs_to", "asset_identity:"+identity.IdentityKey)
			for _, slot := range state.CompleteSlots {
				if slot.DesignGapKey != nil {
					add("asset_state:"+state.StateKey, "has_design_gap", "design_gap:"+*slot.DesignGapKey)
				}
			}
		}
	}
	for _, scene := range value.Planning.Scenes {
		for _, occurrence := range scene.Occurrences {
			source := "occurrence:" + occurrence.OccurrenceKey
			add(source, "in_scene", "scene:"+scene.SceneOwnerLogicalID)
			add(source, "uses_identity", "asset_identity:"+occurrence.IdentityKey)
			add(source, "uses_state", "asset_state:"+occurrence.StateKey)
		}
	}
	for _, interaction := range value.Planning.Interactions {
		source := "interaction:" + interaction.InteractionKey
		add(source, "actor", "occurrence:"+interaction.ActorOccurrenceKey)
		add(source, "prop", "occurrence:"+interaction.PropOccurrenceKey)
		if interaction.CounterpartyOccurrenceKey != nil {
			add(source, "counterparty", "occurrence:"+*interaction.CounterpartyOccurrenceKey)
		}
		add(source, "state_before", "asset_state:"+interaction.PropStateBeforeKey)
		add(source, "state_after", "asset_state:"+interaction.PropStateAfterKey)
	}
	for _, continuity := range value.Planning.Continuity {
		source := "continuity:" + continuity.ContinuityKey
		add(source, "subject", "asset_identity:"+continuity.IdentityKey)
		add(source, "state_before", "asset_state:"+continuity.BeforeStateKey)
		add(source, "state_after", "asset_state:"+continuity.AfterStateKey)
	}
	for _, gap := range value.SharedProof.DesignGaps {
		add("design_gap:"+gap.GapKey, "constrains", productionWorldSubjectRef(value, gap.SubjectKey))
	}
	for _, entry := range value.SharedProof.ContinuityLedger {
		source := "continuity_ledger:" + entry.LedgerKey
		add(source, "subject", "asset_identity:"+entry.IdentityKey)
		add(source, "state", "asset_state:"+entry.StateKey)
		if entry.HolderIdentityKey != nil {
			add(source, "holder", "asset_identity:"+*entry.HolderIdentityKey)
		}
		if entry.LocationIdentityKey != nil {
			add(source, "location", "asset_identity:"+*entry.LocationIdentityKey)
		}
		if entry.TransitionInteractionKey != nil {
			add(source, "transition", "interaction:"+*entry.TransitionInteractionKey)
		}
	}
	slices.SortFunc(refs, func(left, right ProductionWorldCrossPartitionRef) int {
		if result := cmp.Compare(left.SourceKey, right.SourceKey); result != 0 {
			return result
		}
		if result := cmp.Compare(left.Relation, right.Relation); result != 0 {
			return result
		}
		return cmp.Compare(left.TargetKey, right.TargetKey)
	})
	return refs
}

func productionWorldSubjectRef(value ProductionWorldCandidate, key string) string {
	for _, specification := range value.Bible.Specifications {
		if specification.SpecificationKey == key {
			return "specification:" + key
		}
	}
	for _, identity := range value.Asset.Identities {
		if identity.IdentityKey == key {
			return "asset_identity:" + key
		}
		for _, state := range identity.States {
			if state.StateKey == key {
				return "asset_state:" + key
			}
		}
	}
	return "unknown:" + key
}

func validProductionWorldKind(value string) bool {
	return value == "character" || value == "location" || value == "prop"
}

func productionWorldStateKind(kind string) string {
	return map[string]string{
		"character": "character_appearance",
		"location":  "location_state",
		"prop":      "prop_state",
	}[kind]
}

func productionWorldReviewIssues(
	production agentcontract.ProductionEntityFragmentCandidate,
	scenes agentcontract.SceneBindingFragmentCandidate,
	continuity agentcontract.InteractionContinuityCandidate,
) []ProductionWorldReviewIssue {
	result := make([]ProductionWorldReviewIssue, 0,
		len(production.ReviewIssues)+len(scenes.ReviewIssues)+len(continuity.ReviewIssues))
	appendIssues := func(stage string, issues []agentcontract.CandidateReviewIssue) {
		for _, issue := range issues {
			result = append(result, ProductionWorldReviewIssue{SourceStage: stage, Issue: issue})
		}
	}
	appendIssues("derive_production_entities", production.ReviewIssues)
	appendIssues("bind_scene_occurrences", scenes.ReviewIssues)
	appendIssues("reconcile_interaction_continuity", continuity.ReviewIssues)
	slices.SortFunc(result, compareProductionWorldReviewIssue)
	return result
}

func compareProductionWorldReviewIssue(left, right ProductionWorldReviewIssue) int {
	if result := cmp.Compare(left.SourceStage, right.SourceStage); result != 0 {
		return result
	}
	return cmp.Compare(left.Issue.IssueKey, right.Issue.IssueKey)
}

func productionWorldCandidateHashMaterial(value ProductionWorldCandidate) any {
	return struct {
		SchemaVersion               string                                    `json:"schema_version"`
		WorkspaceID                 string                                    `json:"workspace_id"`
		ProjectID                   string                                    `json:"project_id"`
		SourceVersion               agentcontract.ScriptSourceVersionIdentity `json:"source_version"`
		StructureIdentitySetVersion ProductionWorldOwnerVersionRef            `json:"structure_identity_set_version"`
		UpstreamCandidates          ProductionWorldUpstreamCandidates         `json:"upstream_candidates"`
		PartitionRoots              ProductionWorldPartitionRoots             `json:"partition_roots"`
		InteractionProjectionHash   string                                    `json:"interaction_projection_hash"`
		ContinuityProjectionHash    string                                    `json:"continuity_projection_hash"`
	}{
		value.SchemaVersion, value.WorkspaceID, value.ProjectID, value.SourceVersion,
		value.StructureIdentitySetVersion, value.UpstreamCandidates, value.PartitionRoots,
		value.InteractionProjectionHash, value.ContinuityProjectionHash,
	}
}

func decodeProductionWorldJSON(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	return nil
}

func uniqueNonempty(values []string) bool {
	if len(values) == 0 || !slices.IsSorted(values) {
		return false
	}
	for index, value := range values {
		if strings.TrimSpace(value) == "" || (index > 0 && values[index-1] == value) {
			return false
		}
	}
	return true
}

func hashProductionWorldMaterial(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

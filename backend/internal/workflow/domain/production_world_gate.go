package domain

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

const (
	ProductionWorldCandidateSchemaVersion = "production-world-candidate-production"
	ProductionWorldGateInputSchemaVersion = "production-world-human-gate-input-production"
	ProductionWorldGateKey                = "bible_continuity"
	productionWorldSubjectType            = "production_world"
	productionWorldEffectPlanKey          = "bible_continuity"
	confirmProductionWorldStep            = "confirm_production_world"
)

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

type ProductionWorldBusinessKeyRoot struct {
	Partition string `json:"partition"`
	Root      string `json:"root"`
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

type ProductionWorldSharedProof struct {
	ExpectedBusinessKeyRoots []ProductionWorldExpectedBusinessKeySet `json:"expected_business_key_roots"`
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
		!nodeOutputContentHashPattern.MatchString(value.StructureIdentitySetVersion.ContentHash) {
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
		value.Asset.Identities == nil || value.SharedProof.DesignGaps == nil ||
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
		for _, identity := range claim.SubjectIdentityKeys {
			add("world_claim:"+claim.ClaimKey, "subjects", "asset_identity:"+identity)
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

type ProductionWorldCandidateRevisionRef struct {
	CandidateRevisionID   string `json:"candidate_revision_id"`
	CandidateRevision     int64  `json:"candidate_revision"`
	CandidateRevisionHash string `json:"candidate_revision_hash"`
	CandidateContentHash  string `json:"candidate_content_hash"`
}

type ProductionWorldCandidateProjectionRef struct {
	Candidate      agentcontract.SceneAnalysisCandidateRevisionIdentity `json:"candidate"`
	ProjectionKind string                                               `json:"projection_kind"`
	ProjectionHash string                                               `json:"projection_hash"`
}

type ProductionWorldGateSubject struct {
	SourceVersion               agentcontract.ScriptSourceVersionIdentity            `json:"source_version"`
	StructureIdentitySetVersion ProductionWorldOwnerVersionRef                       `json:"structure_identity_set_version"`
	ProductionWorldCandidate    ProductionWorldCandidateRevisionRef                  `json:"production_world_candidate"`
	SceneOccurrenceCandidate    agentcontract.SceneAnalysisCandidateRevisionIdentity `json:"scene_occurrence_candidate"`
	InteractionCandidate        ProductionWorldCandidateProjectionRef                `json:"interaction_candidate"`
	ContinuityCandidate         ProductionWorldCandidateProjectionRef                `json:"continuity_candidate"`
	ExpectedBusinessKeyRoots    []ProductionWorldBusinessKeyRoot                     `json:"expected_business_key_roots"`
	ScopeClosureRoot            string                                               `json:"scope_closure_root"`
	ExpectedHeads               []HumanGateExpectedHead                              `json:"expected_heads"`
	ReadSetRoot                 string                                               `json:"read_set_root"`
}

type ProductionWorldAtomicEffectStep struct {
	StepKey       string                  `json:"step_key"`
	OwnerKinds    []string                `json:"owner_kinds"`
	OwnerCommand  string                  `json:"owner_command"`
	ExpectedHeads []HumanGateExpectedHead `json:"expected_heads"`
	ReadSetRoot   string                  `json:"read_set_root"`
}

type ProductionWorldEffectPlan struct {
	PlanKey    string                          `json:"plan_key"`
	AtomicStep ProductionWorldAtomicEffectStep `json:"atomic_step"`
}

type ProductionWorldGateInputDraft struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	CandidateRevisionID                              string
	CandidateRevision                                int64
	CandidateRevisionHash                            string
	Candidate                                        ProductionWorldCandidate
	AllowedDecisions                                 []string
	ExpectedHeads                                    []HumanGateExpectedHead
}

type ProductionWorldGateInput struct {
	SchemaVersion    string                     `json:"schema_version"`
	GateKey          string                     `json:"gate_key"`
	GateInstanceKey  string                     `json:"gate_instance_key"`
	WorkspaceID      string                     `json:"workspace_id"`
	ProjectID        string                     `json:"project_id"`
	WorkflowRunID    string                     `json:"workflow_run_id"`
	NodeRunID        string                     `json:"node_run_id"`
	SubjectType      string                     `json:"subject_type"`
	Subject          ProductionWorldGateSubject `json:"subject"`
	SubjectHash      string                     `json:"subject_hash"`
	AllowedDecisions []string                   `json:"allowed_decisions"`
	EffectPlan       ProductionWorldEffectPlan  `json:"effect_plan"`
	EffectPlanHash   string                     `json:"effect_plan_hash"`
	InputHash        string                     `json:"input_hash"`
}

func NewProductionWorldGateInput(
	draft ProductionWorldGateInputDraft,
) (ProductionWorldGateInput, json.RawMessage, error) {
	candidateJSON, err := json.Marshal(draft.Candidate)
	if err != nil {
		return ProductionWorldGateInput{}, nil, err
	}
	candidate, _, err := DecodeProductionWorldCandidate(candidateJSON)
	if err != nil || candidate.WorkspaceID != draft.WorkspaceID || candidate.ProjectID != draft.ProjectID {
		return ProductionWorldGateInput{}, nil, errors.New("invalid Gate 2 Production World Candidate")
	}
	candidateRef := ProductionWorldCandidateRevisionRef{
		CandidateRevisionID: draft.CandidateRevisionID, CandidateRevision: draft.CandidateRevision,
		CandidateRevisionHash: draft.CandidateRevisionHash, CandidateContentHash: candidate.ContentHash,
	}
	value := ProductionWorldGateInput{
		SchemaVersion: ProductionWorldGateInputSchemaVersion, GateKey: ProductionWorldGateKey,
		WorkspaceID: draft.WorkspaceID, ProjectID: draft.ProjectID,
		WorkflowRunID: draft.WorkflowRunID, NodeRunID: draft.NodeRunID,
		SubjectType: productionWorldSubjectType,
		Subject: ProductionWorldGateSubject{
			SourceVersion:               candidate.SourceVersion,
			StructureIdentitySetVersion: candidate.StructureIdentitySetVersion,
			ProductionWorldCandidate:    candidateRef,
			SceneOccurrenceCandidate:    candidate.UpstreamCandidates.SceneOccurrence,
			InteractionCandidate: ProductionWorldCandidateProjectionRef{
				Candidate:      candidate.UpstreamCandidates.InteractionContinuity,
				ProjectionKind: "interaction", ProjectionHash: candidate.InteractionProjectionHash,
			},
			ContinuityCandidate: ProductionWorldCandidateProjectionRef{
				Candidate:      candidate.UpstreamCandidates.InteractionContinuity,
				ProjectionKind: "continuity", ProjectionHash: candidate.ContinuityProjectionHash,
			},
			ExpectedBusinessKeyRoots: productionWorldBusinessKeyRoots(candidate.SharedProof.ExpectedBusinessKeyRoots),
			ScopeClosureRoot:         candidate.SharedProof.ScopeClosureRoot,
			ExpectedHeads:            append([]HumanGateExpectedHead(nil), draft.ExpectedHeads...),
		},
		AllowedDecisions: append([]string(nil), draft.AllowedDecisions...),
	}
	if err = completeProductionWorldGateInput(&value, false); err != nil {
		return ProductionWorldGateInput{}, nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ProductionWorldGateInput{}, nil, err
	}
	return value, encoded, nil
}

func DecodeProductionWorldGateInput(
	raw json.RawMessage,
) (ProductionWorldGateInput, json.RawMessage, error) {
	var value ProductionWorldGateInput
	if err := decodeProductionWorldJSON(raw, &value); err != nil {
		return ProductionWorldGateInput{}, nil, errors.New("invalid Gate 2 input")
	}
	original := value
	if err := completeProductionWorldGateInput(&value, true); err != nil || !reflect.DeepEqual(original, value) {
		return ProductionWorldGateInput{}, nil, errors.New("Gate 2 input hash mismatch")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ProductionWorldGateInput{}, nil, err
	}
	return value, encoded, nil
}

func completeProductionWorldGateInput(value *ProductionWorldGateInput, verify bool) error {
	if value.SchemaVersion != ProductionWorldGateInputSchemaVersion || value.GateKey != ProductionWorldGateKey ||
		value.SubjectType != productionWorldSubjectType {
		return errors.New("invalid Gate 2 identity")
	}
	for _, identifier := range []string{value.WorkspaceID, value.ProjectID, value.WorkflowRunID, value.NodeRunID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Gate 2 workflow identity")
		}
	}
	if value.Subject.SourceVersion.Validate() != nil ||
		value.Subject.StructureIdentitySetVersion.OwnerKind != "production/bible" ||
		value.Subject.StructureIdentitySetVersion.LogicalID != value.ProjectID ||
		value.Subject.StructureIdentitySetVersion.Revision < 1 ||
		!nodeOutputContentHashPattern.MatchString(value.Subject.StructureIdentitySetVersion.ContentHash) ||
		validateProductionWorldCandidateRevisionRef(value.Subject.ProductionWorldCandidate) != nil ||
		value.Subject.SceneOccurrenceCandidate.Validate() != nil ||
		value.Subject.SceneOccurrenceCandidate.StageKey != "bind_scene_occurrences" ||
		validateProductionWorldProjection(value.Subject.InteractionCandidate, "interaction") != nil ||
		validateProductionWorldProjection(value.Subject.ContinuityCandidate, "continuity") != nil ||
		value.Subject.InteractionCandidate.Candidate != value.Subject.ContinuityCandidate.Candidate ||
		!nodeOutputContentHashPattern.MatchString(value.Subject.ScopeClosureRoot) ||
		len(value.Subject.ExpectedBusinessKeyRoots) != 3 {
		return errors.New("invalid Gate 2 Subject")
	}
	if _, err := uuid.Parse(value.Subject.StructureIdentitySetVersion.VersionID); err != nil {
		return errors.New("invalid Gate 2 formal version")
	}
	for index, expected := range []string{"asset", "bible", "planning"} {
		root := value.Subject.ExpectedBusinessKeyRoots[index]
		if root.Partition != expected || !nodeOutputContentHashPattern.MatchString(root.Root) {
			return errors.New("invalid Gate 2 expected business key roots")
		}
	}
	for index := range value.AllowedDecisions {
		value.AllowedDecisions[index] = strings.ToLower(strings.TrimSpace(value.AllowedDecisions[index]))
	}
	slices.Sort(value.AllowedDecisions)
	if !slices.Equal(value.AllowedDecisions, []string{"approved", "rejected"}) {
		return errors.New("Gate 2 requires exact typed decisions")
	}
	slices.SortFunc(value.Subject.ExpectedHeads, compareProductionWorldExpectedHead)
	if err := validateProductionWorldExpectedHeads(value.Subject.ExpectedHeads, value.ProjectID); err != nil {
		return err
	}
	readSetRoot := hashProductionWorldMaterial(productionWorldReadSetMaterial(value.Subject))
	if verify && value.Subject.ReadSetRoot != readSetRoot {
		return errors.New("Gate 2 read set drifted")
	}
	value.Subject.ReadSetRoot = readSetRoot
	instanceKey := ProductionWorldGateKey + ":" + value.ProjectID + ":" + value.Subject.ProductionWorldCandidate.CandidateRevisionHash
	subjectHash := hashProductionWorldMaterial(value.Subject)
	plan := ProductionWorldEffectPlan{
		PlanKey: productionWorldEffectPlanKey,
		AtomicStep: ProductionWorldAtomicEffectStep{
			StepKey:       confirmProductionWorldStep,
			OwnerKinds:    []string{"asset", "production/bible", "production/planning"},
			OwnerCommand:  "confirm_production_world",
			ExpectedHeads: append([]HumanGateExpectedHead(nil), value.Subject.ExpectedHeads...),
			ReadSetRoot:   readSetRoot,
		},
	}
	planHash := hashProductionWorldMaterial(plan)
	if verify && (value.GateInstanceKey != instanceKey || value.SubjectHash != subjectHash ||
		!reflect.DeepEqual(value.EffectPlan, plan) || value.EffectPlanHash != planHash) {
		return errors.New("Gate 2 effect plan drifted")
	}
	value.GateInstanceKey, value.SubjectHash = instanceKey, subjectHash
	value.EffectPlan, value.EffectPlanHash = plan, planHash
	originalInputHash := value.InputHash
	value.InputHash = ""
	inputHash := hashProductionWorldMaterial(productionWorldGateInputHashMaterial(*value))
	if verify && originalInputHash != inputHash {
		return errors.New("Gate 2 input content drifted")
	}
	value.InputHash = inputHash
	return nil
}

func validateProductionWorldCandidateRevisionRef(value ProductionWorldCandidateRevisionRef) error {
	if value.CandidateRevision < 1 || !nodeOutputContentHashPattern.MatchString(value.CandidateRevisionHash) ||
		!nodeOutputContentHashPattern.MatchString(value.CandidateContentHash) {
		return errors.New("invalid Production World Candidate revision")
	}
	if _, err := uuid.Parse(value.CandidateRevisionID); err != nil {
		return errors.New("invalid Production World Candidate revision")
	}
	return nil
}

func validateProductionWorldProjection(value ProductionWorldCandidateProjectionRef, kind string) error {
	if value.Candidate.Validate() != nil || value.Candidate.StageKey != "reconcile_interaction_continuity" ||
		value.Candidate.ShardKey != "script:full" || value.ProjectionKind != kind ||
		!nodeOutputContentHashPattern.MatchString(value.ProjectionHash) {
		return errors.New("invalid Production World Candidate projection")
	}
	return nil
}

func compareProductionWorldExpectedHead(left, right HumanGateExpectedHead) int {
	return cmp.Compare(left.OwnerKind, right.OwnerKind)
}

func validateProductionWorldExpectedHeads(values []HumanGateExpectedHead, projectID string) error {
	expected := []string{"asset", "production/bible", "production/planning"}
	if len(values) != len(expected) {
		return errors.New("Gate 2 expected Head set is incomplete")
	}
	for index, value := range values {
		if value.OwnerKind != expected[index] || value.LogicalID != projectID || value.Revision < 0 ||
			(value.Revision == 0 && value.ContentHash != "") ||
			(value.Revision > 0 && !nodeOutputContentHashPattern.MatchString(value.ContentHash)) {
			return errors.New("invalid Gate 2 expected Head")
		}
	}
	return nil
}

func productionWorldReadSetMaterial(value ProductionWorldGateSubject) any {
	return struct {
		SourceVersion               agentcontract.ScriptSourceVersionIdentity            `json:"source_version"`
		StructureIdentitySetVersion ProductionWorldOwnerVersionRef                       `json:"structure_identity_set_version"`
		ProductionWorldCandidate    ProductionWorldCandidateRevisionRef                  `json:"production_world_candidate"`
		SceneOccurrenceCandidate    agentcontract.SceneAnalysisCandidateRevisionIdentity `json:"scene_occurrence_candidate"`
		InteractionCandidate        ProductionWorldCandidateProjectionRef                `json:"interaction_candidate"`
		ContinuityCandidate         ProductionWorldCandidateProjectionRef                `json:"continuity_candidate"`
		ExpectedBusinessKeyRoots    []ProductionWorldBusinessKeyRoot                     `json:"expected_business_key_roots"`
		ScopeClosureRoot            string                                               `json:"scope_closure_root"`
		ExpectedHeads               []HumanGateExpectedHead                              `json:"expected_heads"`
	}{
		value.SourceVersion, value.StructureIdentitySetVersion, value.ProductionWorldCandidate,
		value.SceneOccurrenceCandidate, value.InteractionCandidate, value.ContinuityCandidate,
		value.ExpectedBusinessKeyRoots, value.ScopeClosureRoot, value.ExpectedHeads,
	}
}

func productionWorldBusinessKeyRoots(
	sets []ProductionWorldExpectedBusinessKeySet,
) []ProductionWorldBusinessKeyRoot {
	result := make([]ProductionWorldBusinessKeyRoot, len(sets))
	for index, set := range sets {
		result[index] = ProductionWorldBusinessKeyRoot{Partition: set.Partition, Root: set.Root}
	}
	return result
}

func productionWorldGateInputHashMaterial(value ProductionWorldGateInput) any {
	return struct {
		SchemaVersion    string                     `json:"schema_version"`
		GateKey          string                     `json:"gate_key"`
		GateInstanceKey  string                     `json:"gate_instance_key"`
		WorkspaceID      string                     `json:"workspace_id"`
		ProjectID        string                     `json:"project_id"`
		WorkflowRunID    string                     `json:"workflow_run_id"`
		NodeRunID        string                     `json:"node_run_id"`
		SubjectType      string                     `json:"subject_type"`
		Subject          ProductionWorldGateSubject `json:"subject"`
		SubjectHash      string                     `json:"subject_hash"`
		AllowedDecisions []string                   `json:"allowed_decisions"`
		EffectPlan       ProductionWorldEffectPlan  `json:"effect_plan"`
		EffectPlanHash   string                     `json:"effect_plan_hash"`
	}{
		value.SchemaVersion, value.GateKey, value.GateInstanceKey, value.WorkspaceID, value.ProjectID,
		value.WorkflowRunID, value.NodeRunID, value.SubjectType, value.Subject, value.SubjectHash,
		value.AllowedDecisions, value.EffectPlan, value.EffectPlanHash,
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
	return sha256Hex(encoded)
}

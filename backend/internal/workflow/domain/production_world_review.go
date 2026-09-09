package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
)

const ProductionWorldReviewDetailSchemaVersion = "production-world-review-detail-production"

type ProductionWorldEntityReviewItem struct {
	IdentityKey        string                                  `json:"identity_key"`
	Kind               string                                  `json:"kind"`
	SpecificationKey   string                                  `json:"specification_key"`
	SpecificationSlots []agentcontract.ProductionSemanticSlot  `json:"specification_slots"`
	States             []agentcontract.ProductionStateFragment `json:"states"`
	Basis              agentcontract.ProductionSourceBasis     `json:"basis"`
}

type ProductionWorldSceneOccurrenceReviewItem struct {
	SceneScopeKey       string                                  `json:"scene_scope_key"`
	SceneOwnerLogicalID string                                  `json:"scene_owner_logical_id"`
	TemporarySceneID    string                                  `json:"temporary_scene_id"`
	SourceStart         int                                     `json:"source_start"`
	SourceEnd           int                                     `json:"source_end"`
	StoryTimeKey        string                                  `json:"story_time_key"`
	Occurrences         []agentcontract.SceneOccurrenceFragment `json:"occurrences"`
}

type ProductionWorldContinuityReviewView struct {
	Claims []agentcontract.ContinuityFragment    `json:"claims"`
	Ledger []agentcontract.ContinuityLedgerEntry `json:"ledger"`
}

type ProductionWorldReviewViews struct {
	CharacterAppearances []ProductionWorldEntityReviewItem          `json:"character_appearances"`
	Locations            []ProductionWorldEntityReviewItem          `json:"locations"`
	PropStates           []ProductionWorldEntityReviewItem          `json:"prop_states"`
	SceneOccurrences     []ProductionWorldSceneOccurrenceReviewItem `json:"scene_occurrences"`
	Interactions         []agentcontract.InteractionFragment        `json:"interactions"`
	Continuity           ProductionWorldContinuityReviewView        `json:"continuity"`
}

type ProductionWorldReviewDetail struct {
	SchemaVersion     string                                       `json:"schema_version"`
	GateKey           string                                       `json:"gate_key"`
	InputHash         string                                       `json:"input_hash"`
	CandidateRevision ProductionWorldCandidateRevisionRef          `json:"candidate_revision"`
	PartitionRoots    worlddomain.ProductionWorldPartitionRoots    `json:"partition_roots"`
	AllowedDecisions  []string                                     `json:"allowed_decisions"`
	RepairTargets     []ProductionWorldRepairTargetSet             `json:"repair_targets"`
	Views             ProductionWorldReviewViews                   `json:"views"`
	WorldClaims       []agentcontract.ProductionWorldClaimFragment `json:"world_claims"`
	DesignGaps        []agentcontract.ProductionDesignGap          `json:"design_gaps"`
	ReviewIssues      []worlddomain.ProductionWorldReviewIssue     `json:"review_issues"`
}

func NewProductionWorldReviewDetail(
	gate ProductionWorldGateInput,
	candidate worlddomain.ProductionWorldCandidate,
) (ProductionWorldReviewDetail, json.RawMessage, error) {
	canonicalGate, err := canonicalProductionWorldGate(gate)
	if err != nil {
		return ProductionWorldReviewDetail{}, nil, err
	}
	canonicalCandidate, err := canonicalProductionWorldCandidate(candidate)
	if err != nil || !productionWorldReviewCandidateMatchesGate(canonicalGate, canonicalCandidate) {
		return ProductionWorldReviewDetail{}, nil, errors.New("Production World review Candidate is outside the frozen Gate")
	}
	views, err := productionWorldReviewViews(canonicalCandidate)
	if err != nil {
		return ProductionWorldReviewDetail{}, nil, err
	}
	detail := ProductionWorldReviewDetail{
		SchemaVersion: ProductionWorldReviewDetailSchemaVersion,
		GateKey:       canonicalGate.GateKey, InputHash: canonicalGate.InputHash,
		CandidateRevision: canonicalGate.Subject.ProductionWorldCandidate,
		PartitionRoots:    canonicalCandidate.PartitionRoots,
		AllowedDecisions:  append([]string{}, canonicalGate.AllowedDecisions...),
		RepairTargets:     cloneProductionWorldRepairTargetSets(canonicalGate.Subject.RepairTargets),
		Views:             views,
		WorldClaims:       append([]agentcontract.ProductionWorldClaimFragment{}, canonicalCandidate.Bible.WorldClaims...),
		DesignGaps:        append([]agentcontract.ProductionDesignGap{}, canonicalCandidate.SharedProof.DesignGaps...),
		ReviewIssues:      append([]worlddomain.ProductionWorldReviewIssue{}, canonicalCandidate.SharedProof.ReviewIssues...),
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		return ProductionWorldReviewDetail{}, nil, err
	}
	return detail, encoded, nil
}

func productionWorldReviewViews(
	candidate worlddomain.ProductionWorldCandidate,
) (ProductionWorldReviewViews, error) {
	entities, err := productionWorldReviewEntities(candidate)
	if err != nil {
		return ProductionWorldReviewViews{}, err
	}
	scenes, err := productionWorldReviewScenes(candidate)
	if err != nil {
		return ProductionWorldReviewViews{}, err
	}
	return ProductionWorldReviewViews{
		CharacterAppearances: entities["character"], Locations: entities["location"],
		PropStates: entities["prop"], SceneOccurrences: scenes,
		Interactions: append([]agentcontract.InteractionFragment{}, candidate.Planning.Interactions...),
		Continuity: ProductionWorldContinuityReviewView{
			Claims: append([]agentcontract.ContinuityFragment{}, candidate.Planning.Continuity...),
			Ledger: append([]agentcontract.ContinuityLedgerEntry{}, candidate.SharedProof.ContinuityLedger...),
		},
	}, nil
}

func DecodeProductionWorldReviewDetail(
	raw json.RawMessage,
) (ProductionWorldReviewDetail, json.RawMessage, error) {
	var value ProductionWorldReviewDetail
	if decodeProductionWorldJSON(raw, &value) != nil || validateProductionWorldReviewDetail(value) != nil {
		return ProductionWorldReviewDetail{}, nil, errors.New("invalid Production World review detail")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ProductionWorldReviewDetail{}, nil, err
	}
	return value, encoded, nil
}

func canonicalProductionWorldGate(value ProductionWorldGateInput) (ProductionWorldGateInput, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ProductionWorldGateInput{}, err
	}
	decoded, _, err := DecodeProductionWorldGateInput(encoded)
	if err != nil || !reflect.DeepEqual(value, decoded) {
		return ProductionWorldGateInput{}, errors.New("invalid frozen Production World Gate")
	}
	return decoded, nil
}

func canonicalProductionWorldCandidate(
	value worlddomain.ProductionWorldCandidate,
) (worlddomain.ProductionWorldCandidate, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return worlddomain.ProductionWorldCandidate{}, err
	}
	decoded, _, err := worlddomain.DecodeProductionWorldCandidate(encoded)
	if err != nil || !reflect.DeepEqual(value, decoded) {
		return worlddomain.ProductionWorldCandidate{}, errors.New("invalid Production World review Candidate")
	}
	return decoded, nil
}

func productionWorldReviewCandidateMatchesGate(
	gate ProductionWorldGateInput,
	candidate worlddomain.ProductionWorldCandidate,
) bool {
	frozen := gate.Subject.ProductionWorldCandidate
	return candidate.WorkspaceID == gate.WorkspaceID && candidate.ProjectID == gate.ProjectID &&
		candidate.ContentHash == frozen.CandidateContentHash &&
		candidate.SourceVersion == gate.Subject.SourceVersion &&
		candidate.StructureIdentitySetVersion == gate.Subject.StructureIdentitySetVersion &&
		candidate.UpstreamCandidates.SceneOccurrence == gate.Subject.SceneOccurrenceCandidate &&
		candidate.UpstreamCandidates.InteractionContinuity == gate.Subject.InteractionCandidate.Candidate &&
		candidate.UpstreamCandidates.InteractionContinuity == gate.Subject.ContinuityCandidate.Candidate &&
		candidate.InteractionProjectionHash == gate.Subject.InteractionCandidate.ProjectionHash &&
		candidate.ContinuityProjectionHash == gate.Subject.ContinuityCandidate.ProjectionHash
}

func productionWorldReviewEntities(
	candidate worlddomain.ProductionWorldCandidate,
) (map[string][]ProductionWorldEntityReviewItem, error) {
	identities := make(map[string]worlddomain.ProductionAssetIdentityCandidate, len(candidate.Asset.Identities))
	for _, identity := range candidate.Asset.Identities {
		identities[identity.IdentityKey] = identity
	}
	result := map[string][]ProductionWorldEntityReviewItem{
		"character": {}, "location": {}, "prop": {},
	}
	for _, specification := range candidate.Bible.Specifications {
		identity, ok := identities[specification.IdentityKey]
		if !ok || identity.Kind != specification.Kind {
			return nil, errors.New("Production World review entity partitions have drifted")
		}
		if _, ok = result[specification.Kind]; !ok {
			return nil, errors.New("Production World review entity kind is invalid")
		}
		result[specification.Kind] = append(result[specification.Kind], ProductionWorldEntityReviewItem{
			IdentityKey: specification.IdentityKey, Kind: specification.Kind,
			SpecificationKey:   specification.SpecificationKey,
			SpecificationSlots: append([]agentcontract.ProductionSemanticSlot{}, specification.SpecificationSlots...),
			States:             append([]agentcontract.ProductionStateFragment{}, identity.States...), Basis: specification.Basis,
		})
		delete(identities, specification.IdentityKey)
	}
	if len(identities) != 0 {
		return nil, errors.New("Production World review entity partitions are incomplete")
	}
	return result, nil
}

func productionWorldReviewScenes(
	candidate worlddomain.ProductionWorldCandidate,
) ([]ProductionWorldSceneOccurrenceReviewItem, error) {
	storyTimes := make(map[string]string, len(candidate.Planning.SceneStoryTimes))
	for _, value := range candidate.Planning.SceneStoryTimes {
		storyTimes[value.SceneScopeKey] = value.StoryTimeKey
	}
	result := make([]ProductionWorldSceneOccurrenceReviewItem, 0, len(candidate.Planning.Scenes))
	for _, scene := range candidate.Planning.Scenes {
		storyTime, ok := storyTimes[scene.SceneScopeKey]
		if !ok {
			return nil, errors.New("Production World review Scene story time is incomplete")
		}
		result = append(result, ProductionWorldSceneOccurrenceReviewItem{
			SceneScopeKey: scene.SceneScopeKey, SceneOwnerLogicalID: scene.SceneOwnerLogicalID,
			TemporarySceneID: scene.TemporarySceneID, SourceStart: scene.SourceStart, SourceEnd: scene.SourceEnd,
			StoryTimeKey: storyTime, Occurrences: append([]agentcontract.SceneOccurrenceFragment{}, scene.Occurrences...),
		})
		delete(storyTimes, scene.SceneScopeKey)
	}
	if len(storyTimes) != 0 {
		return nil, errors.New("Production World review Scene story time has drifted")
	}
	return result, nil
}

func validateProductionWorldReviewDetail(value ProductionWorldReviewDetail) error {
	if value.SchemaVersion != ProductionWorldReviewDetailSchemaVersion || value.GateKey != ProductionWorldGateKey ||
		!nodeOutputContentHashPattern.MatchString(value.InputHash) ||
		validateProductionWorldCandidateRevisionRef(value.CandidateRevision) != nil ||
		!nodeOutputContentHashPattern.MatchString(value.PartitionRoots.Bible) ||
		!nodeOutputContentHashPattern.MatchString(value.PartitionRoots.Planning) ||
		!nodeOutputContentHashPattern.MatchString(value.PartitionRoots.Asset) ||
		!nodeOutputContentHashPattern.MatchString(value.PartitionRoots.Proof) ||
		!slices.Equal(value.AllowedDecisions, []string{"approved", "changes_requested", "rejected"}) ||
		validateProductionWorldRepairTargetSets(value.RepairTargets) != nil ||
		value.Views.CharacterAppearances == nil || value.Views.Locations == nil || value.Views.PropStates == nil ||
		value.Views.SceneOccurrences == nil || value.Views.Interactions == nil || value.Views.Continuity.Claims == nil ||
		value.Views.Continuity.Ledger == nil || value.WorldClaims == nil || value.DesignGaps == nil || value.ReviewIssues == nil {
		return errors.New("Production World review detail is incomplete")
	}
	for kind, values := range map[string][]ProductionWorldEntityReviewItem{
		"character": value.Views.CharacterAppearances, "location": value.Views.Locations, "prop": value.Views.PropStates,
	} {
		for _, item := range values {
			if item.Kind != kind || strings.TrimSpace(item.IdentityKey) == "" || strings.TrimSpace(item.SpecificationKey) == "" ||
				item.SpecificationSlots == nil || item.States == nil {
				return errors.New("Production World review entity view is invalid")
			}
		}
	}
	for _, scene := range value.Views.SceneOccurrences {
		if strings.TrimSpace(scene.SceneScopeKey) == "" || strings.TrimSpace(scene.SceneOwnerLogicalID) == "" ||
			strings.TrimSpace(scene.StoryTimeKey) == "" || scene.SourceStart < 0 || scene.SourceEnd <= scene.SourceStart ||
			scene.Occurrences == nil {
			return errors.New("Production World review Scene view is invalid")
		}
	}
	if _, err := uuid.Parse(value.CandidateRevision.CandidateRevisionID); err != nil {
		return errors.New("Production World review Candidate identity is invalid")
	}
	return nil
}

package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

const ProductionEntityFragmentCandidateSchemaVersion = "production-entity-fragment-candidate-production"

var (
	productionSemanticKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,120}$`)
	productionScopePattern       = regexp.MustCompile(`^scene:[0-9a-f-]{36}$`)
	productionPredicatePattern   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	productionBeatAnchorPattern  = regexp.MustCompile(`^beat:[0-9a-f-]{36}:beat_[a-z0-9_]{1,120}$`)
)

type FrozenStructureIdentityCandidateRef struct {
	StageKey              string `json:"stage_key"`
	ShardKey              string `json:"shard_key"`
	CandidateRevisionID   string `json:"candidate_revision_id"`
	CandidateRevisionHash string `json:"candidate_revision_hash"`
	SourceInvocationID    string `json:"source_invocation_id"`
	SourceResultHash      string `json:"source_result_hash"`
	SkillReleaseID        string `json:"skill_release_id"`
	SkillReleaseHash      string `json:"skill_release_hash"`
	StageReleaseHash      string `json:"stage_release_hash"`
	BundleContentHash     string `json:"bundle_content_hash"`
	AgentImageDigest      string `json:"agent_image_digest"`
}

type FrozenEpisodeRef struct {
	TemporaryEpisodeID string `json:"temporary_episode_id"`
	EpisodeID          string `json:"episode_id"`
	EpisodeRevision    int    `json:"episode_revision"`
	Position           int    `json:"position"`
	ScriptVersionID    string `json:"script_version_id"`
	ScriptVersion      int    `json:"script_version"`
	SourceStart        int    `json:"source_start"`
	SourceEnd          int    `json:"source_end"`
	ContentHash        string `json:"content_hash"`
}

type FrozenStructureIdentitySceneRef struct {
	TemporaryEpisodeID  string `json:"temporary_episode_id"`
	EpisodeID           string `json:"episode_id"`
	TemporarySpanID     string `json:"temporary_span_id"`
	TemporarySceneID    string `json:"temporary_scene_id"`
	SceneOwnerLogicalID string `json:"scene_owner_logical_id"`
	ScopeKey            string `json:"scope_key"`
	SourceStart         int    `json:"source_start"`
	SourceEnd           int    `json:"source_end"`
	EvidenceHash        string `json:"evidence_hash"`
}

type FrozenStructureIdentity struct {
	TemporaryIdentityKey string   `json:"temporary_identity_key"`
	IdentityKey          string   `json:"identity_key"`
	Kind                 string   `json:"kind"`
	Resolution           string   `json:"resolution"`
	ReuseIdentityKey     *string  `json:"reuse_identity_key"`
	CanonicalName        string   `json:"canonical_name"`
	Aliases              []string `json:"aliases"`
}

type FrozenStructureIdentityMentionMapping struct {
	Kind             string  `json:"kind"`
	OccurrenceRole   string  `json:"occurrence_role"`
	TemporarySceneID string  `json:"temporary_scene_id"`
	SourceStart      int     `json:"source_start"`
	SourceEnd        int     `json:"source_end"`
	TextHash         string  `json:"text_hash"`
	ExactAnchor      string  `json:"exact_anchor"`
	Resolution       string  `json:"resolution"`
	IdentityKey      *string `json:"identity_key"`
}

type FrozenStructureIdentityCoverage struct {
	SceneCount          int    `json:"scene_count"`
	IdentityCount       int    `json:"identity_count"`
	MentionCount        int    `json:"mention_count"`
	ResolvedCount       int    `json:"resolved_count"`
	UnresolvedCount     int    `json:"unresolved_count"`
	MentionUniverseHash string `json:"mention_universe_hash"`
	ScopeSetHash        string `json:"scope_set_hash"`
}

type FrozenStructureIdentitySet struct {
	SchemaVersion           string                                  `json:"schema_version"`
	ID                      string                                  `json:"id"`
	WorkspaceID             string                                  `json:"workspace_id"`
	ProjectID               string                                  `json:"project_id"`
	Version                 int                                     `json:"version"`
	ParentVersionID         *string                                 `json:"parent_version_id"`
	GateInputID             string                                  `json:"gate_input_id"`
	GateInputHash           string                                  `json:"gate_input_hash"`
	ReviewDecisionID        string                                  `json:"review_decision_id"`
	ProjectEpisodeReceiptID string                                  `json:"project_episode_receipt_id"`
	DocumentRevisionID      string                                  `json:"document_revision_id"`
	SpanIndexID             string                                  `json:"span_index_id"`
	CandidateRefs           []FrozenStructureIdentityCandidateRef   `json:"candidate_refs"`
	EpisodeRefs             []FrozenEpisodeRef                      `json:"episode_refs"`
	SceneRefs               []FrozenStructureIdentitySceneRef       `json:"scene_refs"`
	Identities              []FrozenStructureIdentity               `json:"identities"`
	MentionMappings         []FrozenStructureIdentityMentionMapping `json:"mention_mappings"`
	Coverage                FrozenStructureIdentityCoverage         `json:"coverage"`
	ContentHash             string                                  `json:"content_hash"`
	CreatedBy               string                                  `json:"created_by"`
	CreatedAt               time.Time                               `json:"created_at"`
}

type ProductionEntityDerivationInput struct {
	SourceVersionID                 string                     `json:"source_version_id"`
	SourceHash                      string                     `json:"source_hash"`
	NormalizedText                  string                     `json:"normalized_text"`
	StructureIdentitySetVersionID   string                     `json:"structure_identity_set_version_id"`
	StructureIdentitySetVersionHash string                     `json:"structure_identity_set_version_hash"`
	StructureIdentitySet            FrozenStructureIdentitySet `json:"structure_identity_set"`
	SceneFactCandidateRevisionID    string                     `json:"scene_fact_candidate_revision_id"`
	SceneFactCandidateRevisionHash  string                     `json:"scene_fact_candidate_revision_hash"`
	SceneFactCandidate              json.RawMessage            `json:"scene_fact_candidate"`
}

func DecodeFrozenStructureIdentitySet(raw json.RawMessage) (FrozenStructureIdentitySet, error) {
	var value FrozenStructureIdentitySet
	if err := decodeStrict(raw, &value); err != nil {
		return FrozenStructureIdentitySet{}, err
	}
	return value, nil
}

func (value ProductionEntityDerivationInput) Validate() error {
	for _, identifier := range []string{
		value.SourceVersionID, value.StructureIdentitySetVersionID,
		value.SceneFactCandidateRevisionID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Production Entity input identity")
		}
	}
	if value.NormalizedText == "" || hashUTF8(value.NormalizedText) != value.SourceHash ||
		!hashPattern.MatchString(value.StructureIdentitySetVersionHash) ||
		!hashPattern.MatchString(value.SceneFactCandidateRevisionHash) ||
		value.StructureIdentitySet.Validate() != nil || value.StructureIdentitySet.ID != value.StructureIdentitySetVersionID ||
		value.StructureIdentitySet.ContentHash != value.StructureIdentitySetVersionHash ||
		value.StructureIdentitySet.DocumentRevisionID != value.SourceVersionID {
		return errors.New("Production Entity frozen input lineage drifted")
	}
	var facts SceneFactCandidate
	if decodeStrict(value.SceneFactCandidate, &facts) != nil || facts.SourceVersionID != value.SourceVersionID ||
		facts.SourceHash != value.SourceHash || facts.ReviewIssues == nil || len(facts.Scenes) == 0 {
		return errors.New("invalid Production Entity SceneFact input")
	}
	expectedScenes := make(map[string][2]int, len(facts.Scenes))
	runes := []rune(value.NormalizedText)
	for _, scene := range facts.Scenes {
		if _, exists := expectedScenes[scene.TemporarySceneID]; exists || scene.SourceStart < 0 ||
			scene.SourceEnd <= scene.SourceStart || scene.SourceEnd > len(runes) {
			return errors.New("invalid Production Entity SceneFact scope")
		}
		expectedScenes[scene.TemporarySceneID] = [2]int{scene.SourceStart, scene.SourceEnd}
		for _, evidence := range sceneFactEvidence(scene) {
			if evidence.SourceStart < scene.SourceStart || evidence.SourceEnd > scene.SourceEnd || evidence.Validate(runes) != nil {
				return errors.New("invalid Production Entity SceneFact Evidence")
			}
		}
	}
	for _, scene := range value.StructureIdentitySet.SceneRefs {
		if expectedScenes[scene.TemporarySceneID] != [2]int{scene.SourceStart, scene.SourceEnd} {
			return errors.New("formal StructureIdentitySet does not cover frozen SceneFacts")
		}
		delete(expectedScenes, scene.TemporarySceneID)
	}
	if len(expectedScenes) != 0 {
		return errors.New("formal StructureIdentitySet Scene coverage is incomplete")
	}
	return nil
}

func (value FrozenStructureIdentitySet) Validate() error {
	if value.SchemaVersion != "structure-identity-set-production" || value.Version < 1 || value.CreatedAt.IsZero() ||
		len(value.EpisodeRefs) == 0 || len(value.SceneRefs) == 0 || len(value.Identities) == 0 ||
		value.CandidateRefs == nil || value.MentionMappings == nil || !hashPattern.MatchString(value.ContentHash) ||
		!hashPattern.MatchString(value.GateInputHash) || !hashPattern.MatchString(value.Coverage.MentionUniverseHash) ||
		!hashPattern.MatchString(value.Coverage.ScopeSetHash) {
		return errors.New("invalid frozen StructureIdentitySet")
	}
	for _, identifier := range []string{
		value.ID, value.WorkspaceID, value.ProjectID, value.GateInputID, value.ReviewDecisionID,
		value.ProjectEpisodeReceiptID, value.DocumentRevisionID, value.SpanIndexID, value.CreatedBy,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid frozen StructureIdentitySet identity")
		}
	}
	if value.ParentVersionID != nil {
		if _, err := uuid.Parse(*value.ParentVersionID); err != nil {
			return errors.New("invalid frozen StructureIdentitySet parent")
		}
	}
	identities := make(map[string]string, len(value.Identities))
	for _, identity := range value.Identities {
		if _, exists := identities[identity.IdentityKey]; exists || !validProductionEntityKind(identity.Kind) ||
			!temporaryIdentityKeyPattern.MatchString(identity.TemporaryIdentityKey) || strings.TrimSpace(identity.CanonicalName) == "" ||
			len(identity.Aliases) == 0 || (identity.Resolution != "new" && identity.Resolution != "reuse") ||
			(identity.Resolution == "new") != (identity.ReuseIdentityKey == nil) {
			return errors.New("invalid frozen StructureIdentity")
		}
		identities[identity.IdentityKey] = identity.Kind
	}
	scenes := make(map[string]struct{}, len(value.SceneRefs))
	for _, scene := range value.SceneRefs {
		if _, exists := scenes[scene.TemporarySceneID]; exists || !productionScopePattern.MatchString(scene.ScopeKey) ||
			"scene:"+scene.SceneOwnerLogicalID != scene.ScopeKey || scene.SourceStart < 0 || scene.SourceEnd <= scene.SourceStart ||
			!hashPattern.MatchString(scene.EvidenceHash) {
			return errors.New("invalid frozen StructureIdentity Scene")
		}
		for _, identifier := range []string{scene.EpisodeID, scene.SceneOwnerLogicalID} {
			if _, err := uuid.Parse(identifier); err != nil {
				return errors.New("invalid frozen StructureIdentity Scene identity")
			}
		}
		scenes[scene.TemporarySceneID] = struct{}{}
	}
	for _, mapping := range value.MentionMappings {
		_, sceneFound := scenes[mapping.TemporarySceneID]
		resolved := mapping.Resolution == "resolved"
		if !sceneFound || (mapping.Kind != "character" && mapping.Kind != "location" && mapping.Kind != "prop") ||
			!validOccurrenceRole(mapping.OccurrenceRole) || (mapping.Kind == "location" && mapping.OccurrenceRole != "actual") ||
			mapping.SourceStart < 0 || mapping.SourceEnd <= mapping.SourceStart || !hashPattern.MatchString(mapping.TextHash) ||
			strings.TrimSpace(mapping.ExactAnchor) == "" || resolved != (mapping.IdentityKey != nil) ||
			(!resolved && mapping.Resolution != "unresolved") {
			return errors.New("invalid frozen StructureIdentity mention mapping")
		}
		if resolved && identities[*mapping.IdentityKey] != mapping.Kind {
			return errors.New("frozen StructureIdentity mention kind drifted")
		}
	}
	if value.Coverage.SceneCount != len(value.SceneRefs) || value.Coverage.IdentityCount != len(value.Identities) ||
		value.Coverage.MentionCount != len(value.MentionMappings) ||
		value.Coverage.ResolvedCount+value.Coverage.UnresolvedCount != value.Coverage.MentionCount {
		return errors.New("invalid frozen StructureIdentity coverage")
	}
	return nil
}

type CreatorDecisionProposal struct {
	DecisionKey string `json:"decision_key"`
	Rationale   string `json:"rationale"`
}

type ProductionSourceBasis struct {
	Provenance              string                   `json:"provenance"`
	Evidence                []SourceEvidenceSpan     `json:"evidence"`
	CreatorDecisionProposal *CreatorDecisionProposal `json:"creator_decision_proposal"`
}

type ProductionSemanticSlot struct {
	SlotKey      string  `json:"slot_key"`
	Resolution   string  `json:"resolution"`
	Value        *string `json:"value"`
	DesignGapKey *string `json:"design_gap_key"`
}

type ProductionStateFragment struct {
	StateKey                 string                   `json:"state_key"`
	StateKind                string                   `json:"state_kind"`
	CompleteSlots            []ProductionSemanticSlot `json:"complete_slots"`
	ApplicableSceneScopeKeys []string                 `json:"applicable_scene_scope_keys"`
	EntryReason              string                   `json:"entry_reason"`
	ExitReason               string                   `json:"exit_reason"`
	PreviousStateKey         *string                  `json:"previous_state_key"`
	NextStateKey             *string                  `json:"next_state_key"`
	Basis                    ProductionSourceBasis    `json:"basis"`
}

type ProductionEntityFragment struct {
	IdentityKey        string                    `json:"identity_key"`
	Kind               string                    `json:"kind"`
	SpecificationKey   string                    `json:"specification_key"`
	SpecificationSlots []ProductionSemanticSlot  `json:"specification_slots"`
	Basis              ProductionSourceBasis     `json:"basis"`
	States             []ProductionStateFragment `json:"states"`
}

type ProductionWorldClaimParticipant struct {
	Role        string `json:"role"`
	IdentityKey string `json:"identity_key"`
}

type ProductionWorldClaimAnchor struct {
	Role      string `json:"role"`
	TargetKey string `json:"target_key"`
}

type ProductionWorldClaimScope struct {
	Kind           string `json:"kind"`
	OwnerLogicalID string `json:"owner_logical_id"`
}

type ProductionWorldStoryTimeRange struct {
	StartKey string `json:"start_key"`
	EndKey   string `json:"end_key"`
}

type ProductionWorldNarrativeClaim struct {
	ClaimSeriesKey string                         `json:"claim_series_key"`
	Predicate      string                         `json:"predicate"`
	Anchors        []ProductionWorldClaimAnchor   `json:"anchors"`
	ValidScope     ProductionWorldClaimScope      `json:"valid_scope"`
	StoryTimeRange *ProductionWorldStoryTimeRange `json:"story_time_range"`
	Polarity       string                         `json:"polarity"`
	Status         string                         `json:"status"`
}

type ProductionWorldClaimFragment struct {
	ClaimKey     string                            `json:"claim_key"`
	ClaimType    string                            `json:"claim_type"`
	Participants []ProductionWorldClaimParticipant `json:"participants"`
	Statement    string                            `json:"statement"`
	Narrative    *ProductionWorldNarrativeClaim    `json:"narrative"`
	Basis        ProductionSourceBasis             `json:"basis"`
}

type ProductionDesignGap struct {
	GapKey                   string               `json:"gap_key"`
	SubjectKey               string               `json:"subject_key"`
	FieldKey                 string               `json:"field_key"`
	MissingReason            string               `json:"missing_reason"`
	SourceConstraints        []SourceEvidenceSpan `json:"source_constraints"`
	MutuallyExclusiveOptions []string             `json:"mutually_exclusive_options"`
	ImpactedSceneScopeKeys   []string             `json:"impacted_scene_scope_keys"`
	AllowedResolutionSources []string             `json:"allowed_resolution_sources"`
}

type ProductionEntityFragmentCandidate struct {
	SourceVersionID                 string                         `json:"source_version_id"`
	SourceHash                      string                         `json:"source_hash"`
	StructureIdentitySetVersionID   string                         `json:"structure_identity_set_version_id"`
	StructureIdentitySetVersionHash string                         `json:"structure_identity_set_version_hash"`
	SceneFactCandidateRevisionID    string                         `json:"scene_fact_candidate_revision_id"`
	SceneFactCandidateRevisionHash  string                         `json:"scene_fact_candidate_revision_hash"`
	Entities                        []ProductionEntityFragment     `json:"entities"`
	WorldClaims                     []ProductionWorldClaimFragment `json:"world_claims"`
	DesignGaps                      []ProductionDesignGap          `json:"design_gaps"`
	ReviewIssues                    []CandidateReviewIssue         `json:"review_issues"`
}

func ValidateProductionEntityFragmentCandidate(raw json.RawMessage, input ProductionEntityDerivationInput) error {
	if err := input.Validate(); err != nil {
		return err
	}
	var value ProductionEntityFragmentCandidate
	if decodeStrict(raw, &value) != nil || value.Entities == nil || value.WorldClaims == nil ||
		value.DesignGaps == nil || value.ReviewIssues == nil || value.SourceVersionID != input.SourceVersionID ||
		value.SourceHash != input.SourceHash || value.StructureIdentitySetVersionID != input.StructureIdentitySetVersionID ||
		value.StructureIdentitySetVersionHash != input.StructureIdentitySetVersionHash ||
		value.SceneFactCandidateRevisionID != input.SceneFactCandidateRevisionID ||
		value.SceneFactCandidateRevisionHash != input.SceneFactCandidateRevisionHash {
		return errors.New("Production Entity Candidate lineage drifted")
	}
	expected := make(map[string]string, len(input.StructureIdentitySet.Identities))
	formalIdentities := make(map[string]struct{}, len(input.StructureIdentitySet.Identities))
	for _, identity := range input.StructureIdentitySet.Identities {
		expected[identity.IdentityKey] = identity.Kind
		formalIdentities[identity.IdentityKey] = struct{}{}
	}
	allowedScopes := make(map[string]struct{}, len(input.StructureIdentitySet.SceneRefs))
	for _, scene := range input.StructureIdentitySet.SceneRefs {
		allowedScopes[scene.ScopeKey] = struct{}{}
	}
	evidenceUniverse := productionEntityEvidenceUniverse(input.SceneFactCandidate)
	gaps := make(map[string]ProductionDesignGap, len(value.DesignGaps))
	previous := ""
	for _, gap := range value.DesignGaps {
		if gap.GapKey <= previous || !strings.HasPrefix(gap.GapKey, "gap_") || strings.TrimSpace(gap.SubjectKey) == "" ||
			!productionSemanticKeyPattern.MatchString(gap.FieldKey) || strings.TrimSpace(gap.MissingReason) == "" ||
			gap.SourceConstraints == nil || gap.MutuallyExclusiveOptions == nil || len(gap.ImpactedSceneScopeKeys) == 0 ||
			len(gap.AllowedResolutionSources) == 0 || !sortedUnique(gap.MutuallyExclusiveOptions) ||
			!sortedUnique(gap.ImpactedSceneScopeKeys) || !sortedUnique(gap.AllowedResolutionSources) ||
			!allAllowedScopes(gap.ImpactedSceneScopeKeys, allowedScopes) {
			return errors.New("invalid Production Entity DesignGap")
		}
		for _, source := range gap.AllowedResolutionSources {
			if source != "creator_decision" && source != "visual_foundation" {
				return errors.New("invalid Production Entity DesignGap resolution source")
			}
		}
		if validateProductionEvidence(gap.SourceConstraints, input.NormalizedText, evidenceUniverse) != nil {
			return errors.New("invalid Production Entity DesignGap Evidence")
		}
		gaps[gap.GapKey] = gap
		previous = gap.GapKey
	}
	subjects := make(map[string]struct{}, len(value.Entities)*3)
	previous = ""
	for _, entity := range value.Entities {
		kind, exists := expected[entity.IdentityKey]
		if entity.IdentityKey <= previous || !exists || kind != entity.Kind || !validProductionEntityKind(entity.Kind) ||
			!strings.HasPrefix(entity.SpecificationKey, "specification_") || len(entity.SpecificationSlots) == 0 ||
			len(entity.States) == 0 || validateProductionBasis(entity.Basis, input.NormalizedText, evidenceUniverse) != nil ||
			validateProductionSlots(entity.SpecificationSlots, gaps) != nil {
			return errors.New("invalid Production Entity fragment")
		}
		delete(expected, entity.IdentityKey)
		subjects[entity.IdentityKey] = struct{}{}
		subjects[entity.SpecificationKey] = struct{}{}
		expectedStateKind := map[string]string{
			"character": "character_appearance", "location": "location_state", "prop": "prop_state",
		}[entity.Kind]
		previousState := ""
		for index, state := range entity.States {
			if state.StateKey <= previousState || !strings.HasPrefix(state.StateKey, "state_") ||
				state.StateKind != expectedStateKind || len(state.CompleteSlots) == 0 ||
				len(state.ApplicableSceneScopeKeys) == 0 || !sortedUnique(state.ApplicableSceneScopeKeys) ||
				!allAllowedScopes(state.ApplicableSceneScopeKeys, allowedScopes) || strings.TrimSpace(state.EntryReason) == "" ||
				strings.TrimSpace(state.ExitReason) == "" || validateProductionSlots(state.CompleteSlots, gaps) != nil ||
				validateProductionBasis(state.Basis, input.NormalizedText, evidenceUniverse) != nil {
				return errors.New("invalid Production Entity State")
			}
			expectedPrevious := pointerValue(index > 0, entity.States[max(index-1, 0)].StateKey)
			expectedNext := pointerValue(index < len(entity.States)-1, entity.States[min(index+1, len(entity.States)-1)].StateKey)
			if !equalStringPointer(state.PreviousStateKey, expectedPrevious) || !equalStringPointer(state.NextStateKey, expectedNext) {
				return errors.New("Production Entity State lineage is incomplete")
			}
			subjects[state.StateKey] = struct{}{}
			previousState = state.StateKey
		}
		previous = entity.IdentityKey
	}
	if len(expected) != 0 {
		return errors.New("Production Entity Candidate does not cover every formal identity")
	}
	previous = ""
	for _, claim := range value.WorldClaims {
		if claim.ClaimKey <= previous || !strings.HasPrefix(claim.ClaimKey, "claim_") ||
			!slices.Contains([]string{"world_rule", "relationship", "foreshadowing", "payoff", "story_arc", "plot_thread"}, claim.ClaimType) ||
			strings.TrimSpace(claim.Statement) == "" ||
			validateProductionBasis(claim.Basis, input.NormalizedText, evidenceUniverse) != nil {
			return errors.New("invalid Production Entity world claim")
		}
		if err := validateProductionWorldClaimParticipants(claim.Participants, formalIdentities); err != nil {
			return err
		}
		if err := validateProductionWorldNarrativeClaim(claim, input); err != nil {
			return err
		}
		for _, participant := range claim.Participants {
			if _, exists := subjects[participant.IdentityKey]; !exists {
				return errors.New("Production Entity world claim references an unknown identity")
			}
		}
		previous = claim.ClaimKey
	}
	for _, gap := range gaps {
		if _, exists := subjects[gap.SubjectKey]; !exists {
			return errors.New("Production Entity DesignGap references an unknown subject")
		}
	}
	previous = ""
	for _, issue := range value.ReviewIssues {
		if issue.IssueKey <= previous || validateCandidateReviewIssue(issue, []rune(input.NormalizedText), true) != nil {
			return errors.New("invalid Production Entity review issue")
		}
		for _, evidence := range issue.Evidence {
			if _, exists := evidenceUniverse[productionEvidenceKey(evidence)]; !exists {
				return errors.New("Production Entity review Evidence is outside frozen SceneFacts")
			}
		}
		previous = issue.IssueKey
	}
	return nil
}

func validateProductionWorldClaimParticipants(values []ProductionWorldClaimParticipant, identities map[string]struct{}) error {
	if len(values) == 0 {
		return errors.New("Production Entity world claim has no participants")
	}
	previous, subjects, objects := "", 0, 0
	seenIdentities := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := value.IdentityKey + "\x00" + value.Role
		if key <= previous || !slices.Contains([]string{"subject", "object", "participant"}, value.Role) {
			return errors.New("Production Entity world claim participants are not canonical")
		}
		if _, exists := identities[value.IdentityKey]; !exists {
			return errors.New("Production Entity world claim references an unknown identity")
		}
		if _, exists := seenIdentities[value.IdentityKey]; exists {
			return errors.New("Production Entity world claim repeats a participant identity")
		}
		seenIdentities[value.IdentityKey] = struct{}{}
		if value.Role == "subject" {
			subjects++
		}
		if value.Role == "object" {
			objects++
		}
		previous = key
	}
	if subjects != 1 || objects > 1 {
		return errors.New("Production Entity world claim participant roles are invalid")
	}
	return nil
}

func validateProductionWorldNarrativeClaim(claim ProductionWorldClaimFragment, input ProductionEntityDerivationInput) error {
	narrativeType := slices.Contains([]string{"relationship", "foreshadowing", "payoff"}, claim.ClaimType)
	if narrativeType != (claim.Narrative != nil) {
		return errors.New("Production Entity Narrative Claim facts are incomplete")
	}
	if !narrativeType {
		return nil
	}
	narrative := claim.Narrative
	if narrative.ClaimSeriesKey != claim.ClaimKey || !productionPredicatePattern.MatchString(narrative.Predicate) ||
		len(narrative.Anchors) == 0 || !slices.Contains([]string{"positive", "negative", "neutral"}, narrative.Polarity) ||
		!slices.Contains([]string{"asserted", "negated"}, narrative.Status) {
		return errors.New("invalid Production Entity Narrative Claim")
	}
	episodes := make(map[string]struct{}, len(input.StructureIdentitySet.EpisodeRefs))
	scenes := make(map[string]struct{}, len(input.StructureIdentitySet.SceneRefs))
	for _, episode := range input.StructureIdentitySet.EpisodeRefs {
		episodes[episode.EpisodeID] = struct{}{}
	}
	for _, scene := range input.StructureIdentitySet.SceneRefs {
		scenes[scene.SceneOwnerLogicalID] = struct{}{}
	}
	previous := ""
	for _, anchor := range narrative.Anchors {
		key := anchor.TargetKey + "\x00" + anchor.Role
		if key <= previous || !validProductionClaimAnchor(anchor, episodes, scenes) {
			return errors.New("invalid Production Entity Narrative Claim anchor")
		}
		previous = key
	}
	if !validProductionClaimScope(narrative.ValidScope, input.StructureIdentitySet.ProjectID, episodes, scenes, narrative.Anchors) {
		return errors.New("invalid Production Entity Narrative Claim scope")
	}
	if narrative.StoryTimeRange != nil && (!validProductionStableKey(narrative.StoryTimeRange.StartKey) ||
		!validProductionStableKey(narrative.StoryTimeRange.EndKey) || narrative.StoryTimeRange.StartKey > narrative.StoryTimeRange.EndKey) {
		return errors.New("invalid Production Entity Narrative Claim story time")
	}
	return nil
}

func validProductionClaimAnchor(value ProductionWorldClaimAnchor, episodes, scenes map[string]struct{}) bool {
	switch value.Role {
	case "episode":
		if !strings.HasPrefix(value.TargetKey, "episode:") {
			return false
		}
		_, exists := episodes[strings.TrimPrefix(value.TargetKey, "episode:")]
		return exists
	case "scene":
		if !strings.HasPrefix(value.TargetKey, "scene:") {
			return false
		}
		_, exists := scenes[strings.TrimPrefix(value.TargetKey, "scene:")]
		return exists
	case "beat":
		if !productionBeatAnchorPattern.MatchString(value.TargetKey) {
			return false
		}
		parts := strings.SplitN(strings.TrimPrefix(value.TargetKey, "beat:"), ":", 2)
		_, exists := scenes[parts[0]]
		return exists
	default:
		return false
	}
}

func validProductionClaimScope(value ProductionWorldClaimScope, projectID string, episodes, scenes map[string]struct{}, anchors []ProductionWorldClaimAnchor) bool {
	switch value.Kind {
	case "project":
		return value.OwnerLogicalID == projectID
	case "episode":
		_, exists := episodes[value.OwnerLogicalID]
		return exists
	case "scene":
		if !strings.HasPrefix(value.OwnerLogicalID, "scene:") {
			return false
		}
		_, exists := scenes[strings.TrimPrefix(value.OwnerLogicalID, "scene:")]
		return exists
	case "beat":
		for _, anchor := range anchors {
			if anchor.Role == "beat" && anchor.TargetKey == value.OwnerLogicalID {
				return true
			}
		}
	}
	return false
}

func validProductionStableKey(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	for _, character := range value {
		if character <= 0x1f || character == 0x7f {
			return false
		}
	}
	return true
}

func validateProductionBasis(value ProductionSourceBasis, text string, universe map[string]struct{}) error {
	hasEvidence := len(value.Evidence) > 0
	hasDecision := value.CreatorDecisionProposal != nil
	if value.Evidence == nil || hasEvidence == hasDecision ||
		(value.Provenance != "source_explicit" && value.Provenance != "inferred" && value.Provenance != "user_supplied") ||
		(value.Provenance == "user_supplied") != hasDecision {
		return errors.New("Production Entity source must be Evidence XOR CreatorDecision")
	}
	if hasDecision && (!strings.HasPrefix(value.CreatorDecisionProposal.DecisionKey, "decision_") ||
		strings.TrimSpace(value.CreatorDecisionProposal.Rationale) == "") {
		return errors.New("invalid Production Entity CreatorDecision proposal")
	}
	return validateProductionEvidence(value.Evidence, text, universe)
}

func validateProductionSlots(values []ProductionSemanticSlot, gaps map[string]ProductionDesignGap) error {
	previous := ""
	for _, value := range values {
		if value.SlotKey <= previous || !productionSemanticKeyPattern.MatchString(value.SlotKey) {
			return errors.New("Production Entity slots must be sorted and unique")
		}
		switch value.Resolution {
		case "known":
			if value.Value == nil || strings.TrimSpace(*value.Value) == "" || value.DesignGapKey != nil {
				return errors.New("known Production Entity slot is incomplete")
			}
		case "unspecified_design_gap":
			if value.Value != nil || value.DesignGapKey == nil {
				return errors.New("Production Entity slot does not bind one DesignGap")
			}
			if _, exists := gaps[*value.DesignGapKey]; !exists {
				return errors.New("Production Entity slot references an unknown DesignGap")
			}
		case "not_applicable":
			if value.Value != nil || value.DesignGapKey != nil {
				return errors.New("not-applicable Production Entity slot carries a value")
			}
		default:
			return errors.New("invalid Production Entity slot resolution")
		}
		previous = value.SlotKey
	}
	return nil
}

func productionEntityEvidenceUniverse(raw json.RawMessage) map[string]struct{} {
	var facts SceneFactCandidate
	_ = decodeStrict(raw, &facts)
	result := make(map[string]struct{})
	for _, scene := range facts.Scenes {
		for _, evidence := range sceneFactEvidence(scene) {
			result[productionEvidenceKey(evidence)] = struct{}{}
		}
	}
	return result
}

func sceneFactEvidence(scene SceneFact) []SourceEvidenceSpan {
	result := make([]SourceEvidenceSpan, 0, len(scene.Actions)+len(scene.Dialogues)+len(scene.RawCharacterMentions)+len(scene.RawPropMentions)+2)
	if scene.Location != nil {
		result = append(result, scene.Location.Evidence)
	}
	if scene.Time != nil {
		result = append(result, scene.Time.Evidence)
	}
	for _, item := range scene.Actions {
		result = append(result, item.Evidence)
	}
	for _, item := range scene.Dialogues {
		result = append(result, item.Evidence)
	}
	for _, item := range scene.RawCharacterMentions {
		result = append(result, item.Evidence)
	}
	for _, item := range scene.RawPropMentions {
		result = append(result, item.Evidence)
	}
	return result
}

func validateProductionEvidence(values []SourceEvidenceSpan, text string, universe map[string]struct{}) error {
	for _, value := range values {
		if value.Validate([]rune(text)) != nil {
			return errors.New("invalid Production Entity Evidence")
		}
		if _, exists := universe[productionEvidenceKey(value)]; !exists {
			return errors.New("Production Entity Evidence is outside frozen SceneFacts")
		}
	}
	return nil
}

func productionEvidenceKey(value SourceEvidenceSpan) string {
	return fmt.Sprintf("%012d:%012d:%s:%s", value.SourceStart, value.SourceEnd, value.TextHash, value.ExactAnchor)
}

func allAllowedScopes(values []string, allowed map[string]struct{}) bool {
	for _, value := range values {
		if _, exists := allowed[value]; !exists {
			return false
		}
	}
	return true
}

func sortedUnique(values []string) bool {
	return slices.IsSorted(values) && len(values) == len(slices.Compact(append([]string(nil), values...)))
}

func validProductionEntityKind(value string) bool {
	return value == "character" || value == "location" || value == "prop"
}

func pointerValue(present bool, value string) *string {
	if !present {
		return nil
	}
	return &value
}

func equalStringPointer(left, right *string) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

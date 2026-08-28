package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/google/uuid"

	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type EpisodeCandidateIdentity struct {
	EntityKey string
	StateKeys []string
}

type EpisodeCandidateScope struct {
	EpisodeID, ScriptVersionID string
	EpisodePosition            int
	SourceStart, SourceEnd     int
	ContextStart               int
	ContextText                string
	KnownIdentities            []EpisodeCandidateIdentity
}

type EpisodeStructureAttributes struct {
	SceneKey            *string  `json:"scene_key"`
	SpeakerKey          *string  `json:"speaker_key"`
	ParticipantKeys     []string `json:"participant_keys"`
	LocationKey         *string  `json:"location_key"`
	TimeHint            *string  `json:"time_hint"`
	DialogueText        *string  `json:"dialogue_text"`
	Action              *string  `json:"action"`
	OccurrenceEntityKey *string  `json:"occurrence_entity_key"`
	StateKey            *string  `json:"state_key"`
	ContinuityNotes     []string `json:"continuity_notes"`
}

type EpisodeStructureFragment struct {
	TemporaryKey string                     `json:"temporary_key"`
	Kind         string                     `json:"kind"`
	SourceKeys   []string                   `json:"source_keys"`
	SourceStart  int                        `json:"source_start"`
	SourceEnd    int                        `json:"source_end"`
	Summary      string                     `json:"summary"`
	Evidence     []bibledomain.Evidence     `json:"evidence"`
	Attributes   EpisodeStructureAttributes `json:"attributes"`
}

type EpisodeClaimCandidate struct {
	ClaimKey        string                 `json:"claim_key"`
	ClaimType       string                 `json:"claim_type"`
	ParticipantKeys []string               `json:"participant_keys"`
	AnchorKeys      []string               `json:"anchor_keys"`
	Scope           string                 `json:"scope"`
	Polarity        string                 `json:"polarity"`
	Status          string                 `json:"status"`
	Evidence        []bibledomain.Evidence `json:"evidence"`
}

type EpisodeAnalysisCandidate struct {
	EpisodeID       string                     `json:"episode_id"`
	ScriptVersionID string                     `json:"script_version_id"`
	LogicalStart    int                        `json:"logical_start"`
	LogicalEnd      int                        `json:"logical_end"`
	Fragments       []EpisodeStructureFragment `json:"fragments"`
	Claims          []EpisodeClaimCandidate    `json:"claims"`
	ReviewIssues    []bibledomain.ReviewIssue  `json:"review_issues"`
}

type EpisodeReconciliationCandidate struct {
	EpisodeID        string                     `json:"episode_id"`
	ScriptVersionID  string                     `json:"script_version_id"`
	SourceStart      int                        `json:"source_start"`
	SourceEnd        int                        `json:"source_end"`
	OrderedFragments []EpisodeStructureFragment `json:"ordered_fragments"`
	Claims           []EpisodeClaimCandidate    `json:"claims"`
	Conflicts        []bibledomain.ReviewIssue  `json:"conflicts"`
	ReviewIssues     []bibledomain.ReviewIssue  `json:"review_issues"`
}

type EpisodePlanningRootInput struct {
	EpisodeID, ScriptVersionID                 string
	EpisodePosition                            int
	ShardKey, StageInstanceKey                 string
	CandidateRevisionID, CandidateRevisionHash string
	Candidate                                  EpisodeReconciliationCandidate
}

type EpisodePlanningCandidateRoot struct {
	EpisodeID             string                         `json:"episode_id"`
	EpisodePosition       int                            `json:"episode_position"`
	ScriptVersionID       string                         `json:"script_version_id"`
	ShardKey              string                         `json:"shard_key"`
	StageInstanceKey      string                         `json:"stage_instance_key"`
	CandidateRevisionID   string                         `json:"candidate_revision_id"`
	CandidateRevisionHash string                         `json:"candidate_revision_hash"`
	Candidate             EpisodeReconciliationCandidate `json:"candidate"`
}

type EpisodePlanningCandidateSet struct {
	SchemaVersion       string                         `json:"schema_version"`
	BibleVersionID      string                         `json:"bible_version_id"`
	BibleVersion        int                            `json:"bible_version"`
	BibleContentHash    string                         `json:"bible_content_hash"`
	MaterializationHash string                         `json:"materialization_hash"`
	Episodes            []EpisodePlanningCandidateRoot `json:"episodes"`
}

func BuildEpisodePlanningCandidateSet(
	manifest EpisodeReconcileManifest,
	bibleVersionID string,
	bibleVersion int,
	bibleContentHash string,
	materializationHash string,
	inputs []EpisodePlanningRootInput,
) (EpisodePlanningCandidateSet, json.RawMessage, string, string, error) {
	if _, err := uuid.Parse(bibleVersionID); err != nil || bibleVersion < 1 ||
		!episodeAnalysisHashPattern.MatchString(bibleContentHash) ||
		!episodeAnalysisHashPattern.MatchString(materializationHash) || len(inputs) != len(manifest.Roots) {
		return EpisodePlanningCandidateSet{}, nil, "", "", errors.New("invalid Episode planning candidate set input")
	}
	inputs = append([]EpisodePlanningRootInput(nil), inputs...)
	slices.SortFunc(inputs, func(left, right EpisodePlanningRootInput) int {
		return left.EpisodePosition - right.EpisodePosition
	})
	rootsByEpisode := make(map[string]EpisodeReconcileRoot, len(manifest.Roots))
	for _, root := range manifest.Roots {
		rootsByEpisode[root.EpisodeID] = root
	}
	episodes := make([]EpisodePlanningCandidateRoot, len(inputs))
	for index, input := range inputs {
		root, exists := rootsByEpisode[input.EpisodeID]
		if !exists || input.EpisodePosition != index+1 || root.EpisodePosition != input.EpisodePosition ||
			root.ShardKey != input.ShardKey || input.Candidate.EpisodeID != input.EpisodeID ||
			input.Candidate.ScriptVersionID != input.ScriptVersionID ||
			!episodeAnalysisHashPattern.MatchString(input.StageInstanceKey) ||
			!episodeAnalysisHashPattern.MatchString(input.CandidateRevisionHash) {
			return EpisodePlanningCandidateSet{}, nil, "", "", errors.New("invalid Episode planning root Candidate")
		}
		for _, identifier := range []string{input.EpisodeID, input.ScriptVersionID, input.CandidateRevisionID} {
			if _, err := uuid.Parse(identifier); err != nil {
				return EpisodePlanningCandidateSet{}, nil, "", "", errors.New("invalid Episode planning root identity")
			}
		}
		episodes[index] = EpisodePlanningCandidateRoot{
			EpisodeID: input.EpisodeID, EpisodePosition: input.EpisodePosition,
			ScriptVersionID: input.ScriptVersionID, ShardKey: input.ShardKey,
			StageInstanceKey:      input.StageInstanceKey,
			CandidateRevisionID:   input.CandidateRevisionID,
			CandidateRevisionHash: input.CandidateRevisionHash, Candidate: input.Candidate,
		}
	}
	value := EpisodePlanningCandidateSet{
		SchemaVersion:  "episode-planning-candidate-set-v1",
		BibleVersionID: bibleVersionID, BibleVersion: bibleVersion,
		BibleContentHash: bibleContentHash, MaterializationHash: materializationHash,
		Episodes: episodes,
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return EpisodePlanningCandidateSet{}, nil, "", "", err
	}
	contentHash, err := EpisodePlanningCandidateSetContentHash(value)
	if err != nil {
		return EpisodePlanningCandidateSet{}, nil, "", "", err
	}
	stageKey, err := EpisodePlanningCandidateSetStageInstanceKey(manifest)
	if err != nil {
		return EpisodePlanningCandidateSet{}, nil, "", "", err
	}
	return value, encoded, contentHash, stageKey, nil
}

func EpisodePlanningCandidateSetContentHash(value EpisodePlanningCandidateSet) (string, error) {
	return episodeAnalysisCanonicalHash(value)
}

func EpisodePlanningCandidateSetStageInstanceKey(manifest EpisodeReconcileManifest) (string, error) {
	return episodeAnalysisCanonicalHash(struct {
		Schema, NodeRunID, ManifestID, ManifestHash string
		ManifestVersion                             int64
	}{
		"episode-planning-candidate-set-stage-v1", manifest.NodeRunID,
		manifest.ManifestID, manifest.ManifestHash, manifest.Version,
	})
}

func DecodeEpisodeAnalysisCandidate(
	raw json.RawMessage,
	scope EpisodeCandidateScope,
) (EpisodeAnalysisCandidate, error) {
	var value EpisodeAnalysisCandidate
	if err := decodeStrictEpisodeCandidate(raw, &value); err != nil {
		return EpisodeAnalysisCandidate{}, errors.New("candidate does not match Episode analysis schema")
	}
	if value.EpisodeID != scope.EpisodeID || value.ScriptVersionID != scope.ScriptVersionID ||
		value.LogicalStart != scope.SourceStart || value.LogicalEnd != scope.SourceEnd {
		return EpisodeAnalysisCandidate{}, errors.New("Episode analysis candidate escaped its frozen slice")
	}
	if err := validateEpisodeCandidateContent(
		value.Fragments, value.Claims, value.ReviewIssues, scope,
	); err != nil {
		return EpisodeAnalysisCandidate{}, err
	}
	return value, nil
}

func DecodeEpisodeReconciliationCandidate(
	raw json.RawMessage,
	scope EpisodeCandidateScope,
	analysisChildren []EpisodeAnalysisCandidate,
	reconciliationChildren []EpisodeReconciliationCandidate,
) (EpisodeReconciliationCandidate, error) {
	var value EpisodeReconciliationCandidate
	if err := decodeStrictEpisodeCandidate(raw, &value); err != nil {
		return EpisodeReconciliationCandidate{}, errors.New("candidate does not match Episode reconciliation schema")
	}
	if value.EpisodeID != scope.EpisodeID || value.ScriptVersionID != scope.ScriptVersionID ||
		value.SourceStart != scope.SourceStart || value.SourceEnd != scope.SourceEnd {
		return EpisodeReconciliationCandidate{}, errors.New("Episode reconciliation candidate escaped its frozen Episode")
	}
	issues := append(append([]bibledomain.ReviewIssue(nil), value.Conflicts...), value.ReviewIssues...)
	if err := validateEpisodeCandidateContent(value.OrderedFragments, value.Claims, issues, scope); err != nil {
		return EpisodeReconciliationCandidate{}, err
	}
	expectedFragments := make(map[string]struct{})
	expectedClaims := make(map[string]struct{})
	allowedIssueEvidence := make(map[string]struct{})
	addChildren := func(
		fragments []EpisodeStructureFragment,
		claims []EpisodeClaimCandidate,
		childIssues []bibledomain.ReviewIssue,
	) error {
		for _, fragment := range fragments {
			if _, exists := expectedFragments[fragment.TemporaryKey]; exists {
				return errors.New("Episode reconciliation children contain duplicate fragments")
			}
			expectedFragments[fragment.TemporaryKey] = struct{}{}
			for _, evidence := range fragment.Evidence {
				allowedIssueEvidence[episodeEvidenceKey(evidence)] = struct{}{}
			}
		}
		for _, claim := range claims {
			if _, exists := expectedClaims[claim.ClaimKey]; exists {
				return errors.New("Episode reconciliation children contain duplicate claims")
			}
			expectedClaims[claim.ClaimKey] = struct{}{}
			for _, evidence := range claim.Evidence {
				allowedIssueEvidence[episodeEvidenceKey(evidence)] = struct{}{}
			}
		}
		for _, issue := range childIssues {
			for _, evidence := range issue.Evidence {
				allowedIssueEvidence[episodeEvidenceKey(evidence)] = struct{}{}
			}
		}
		return nil
	}
	for _, child := range analysisChildren {
		if err := addChildren(child.Fragments, child.Claims, child.ReviewIssues); err != nil {
			return EpisodeReconciliationCandidate{}, err
		}
	}
	for _, child := range reconciliationChildren {
		childIssues := append(append([]bibledomain.ReviewIssue(nil), child.Conflicts...), child.ReviewIssues...)
		if err := addChildren(child.OrderedFragments, child.Claims, childIssues); err != nil {
			return EpisodeReconciliationCandidate{}, err
		}
	}
	if len(value.OrderedFragments) != len(expectedFragments) || slices.ContainsFunc(value.OrderedFragments, func(fragment EpisodeStructureFragment) bool {
		_, exists := expectedFragments[fragment.TemporaryKey]
		return !exists
	}) {
		return EpisodeReconciliationCandidate{}, errors.New("Episode reconciliation candidate did not preserve its exact child set")
	}
	if len(value.Claims) != len(expectedClaims) || slices.ContainsFunc(value.Claims, func(claim EpisodeClaimCandidate) bool {
		_, exists := expectedClaims[claim.ClaimKey]
		return !exists
	}) {
		return EpisodeReconciliationCandidate{}, errors.New("Episode reconciliation candidate did not preserve its exact claims")
	}
	for _, issue := range issues {
		for _, evidence := range issue.Evidence {
			if _, exists := allowedIssueEvidence[episodeEvidenceKey(evidence)]; !exists {
				return EpisodeReconciliationCandidate{}, errors.New("Episode reconciliation issue Evidence is outside its exact children")
			}
		}
	}
	return value, nil
}

// ValidateEpisodeReconciliationCandidate validates a frozen Episode candidate
// without requiring its intermediate map/reduce children. Owner application
// uses it after the repository has verified the aggregate revision and leaves.
func ValidateEpisodeReconciliationCandidate(
	value EpisodeReconciliationCandidate,
	scope EpisodeCandidateScope,
) error {
	if value.EpisodeID != scope.EpisodeID || value.ScriptVersionID != scope.ScriptVersionID ||
		value.SourceStart != scope.SourceStart || value.SourceEnd != scope.SourceEnd {
		return errors.New("Episode reconciliation candidate escaped its frozen Episode")
	}
	issues := append(append([]bibledomain.ReviewIssue(nil), value.Conflicts...), value.ReviewIssues...)
	return validateEpisodeCandidateContent(value.OrderedFragments, value.Claims, issues, scope)
}

func validateEpisodeCandidateContent(
	fragments []EpisodeStructureFragment,
	claims []EpisodeClaimCandidate,
	issues []bibledomain.ReviewIssue,
	scope EpisodeCandidateScope,
) error {
	if err := validateEpisodeCandidateScope(scope); err != nil {
		return err
	}
	known, states := make(map[string]struct{}, len(scope.KnownIdentities)), make(map[string]struct{})
	for _, identity := range scope.KnownIdentities {
		known[identity.EntityKey] = struct{}{}
		for _, state := range identity.StateKeys {
			states[state] = struct{}{}
		}
	}
	fragmentKeys, sceneKeys := make(map[string]struct{}, len(fragments)), make(map[string]struct{})
	previousStart, previousEnd, previousKey := -1, -1, ""
	for _, fragment := range fragments {
		if strings.TrimSpace(fragment.TemporaryKey) == "" || strings.TrimSpace(fragment.Summary) == "" ||
			len(fragment.SourceKeys) == 0 || len(fragment.Evidence) == 0 ||
			fragment.SourceStart < scope.SourceStart || fragment.SourceEnd <= fragment.SourceStart ||
			fragment.SourceEnd > scope.SourceEnd || !oneOfEpisode(fragment.Kind, "scene", "dialogue", "beat", "occurrence") {
			return errors.New("Episode candidate contains an invalid structure fragment")
		}
		if fragment.SourceStart < previousStart ||
			fragment.SourceStart == previousStart && (fragment.SourceEnd < previousEnd || fragment.SourceEnd == previousEnd && fragment.TemporaryKey <= previousKey) {
			return errors.New("Episode candidate fragments must be source ordered")
		}
		if _, exists := fragmentKeys[fragment.TemporaryKey]; exists {
			return errors.New("Episode candidate fragment keys must be unique")
		}
		fragmentKeys[fragment.TemporaryKey] = struct{}{}
		if fragment.Kind == "scene" {
			sceneKeys[fragment.TemporaryKey] = struct{}{}
		}
		for _, key := range append(
			[]*string{fragment.Attributes.SpeakerKey, fragment.Attributes.LocationKey, fragment.Attributes.OccurrenceEntityKey},
			stringPointers(fragment.Attributes.ParticipantKeys)...,
		) {
			if key != nil {
				if _, exists := known[*key]; !exists {
					return errors.New("Episode candidate references an unknown known identity")
				}
			}
		}
		if fragment.Attributes.StateKey != nil {
			if _, exists := states[*fragment.Attributes.StateKey]; !exists {
				return errors.New("Episode candidate references an unknown known identity state")
			}
		}
		if err := validateEpisodeEvidence(fragment.Evidence, scope); err != nil {
			return err
		}
		previousStart, previousEnd, previousKey = fragment.SourceStart, fragment.SourceEnd, fragment.TemporaryKey
	}
	for _, fragment := range fragments {
		if fragment.Attributes.SceneKey != nil {
			if _, exists := sceneKeys[*fragment.Attributes.SceneKey]; !exists {
				return errors.New("Episode candidate references an unknown scene fragment")
			}
		}
	}
	claimKeys := make(map[string]struct{}, len(claims))
	for _, claim := range claims {
		if strings.TrimSpace(claim.ClaimKey) == "" || strings.TrimSpace(claim.Scope) == "" ||
			len(claim.ParticipantKeys) == 0 || len(claim.AnchorKeys) == 0 || len(claim.Evidence) == 0 ||
			!oneOfEpisode(claim.ClaimType, "relationship", "causal", "continuity", "foreshadowing") ||
			!oneOfEpisode(claim.Polarity, "positive", "negative", "mixed", "unknown") ||
			!oneOfEpisode(claim.Status, "proposed", "ambiguous", "conflicted") {
			return errors.New("Episode candidate contains an invalid Claim")
		}
		if _, exists := claimKeys[claim.ClaimKey]; exists {
			return errors.New("Episode candidate Claim keys must be unique")
		}
		claimKeys[claim.ClaimKey] = struct{}{}
		for _, key := range claim.ParticipantKeys {
			if _, exists := known[key]; !exists {
				return errors.New("Episode candidate Claim references an unknown known identity")
			}
		}
		for _, key := range claim.AnchorKeys {
			_, fragmentExists := fragmentKeys[key]
			_, identityExists := known[key]
			if !fragmentExists && !identityExists {
				return errors.New("Episode candidate Claim references an unknown anchor")
			}
		}
		if err := validateEpisodeEvidence(claim.Evidence, scope); err != nil {
			return err
		}
	}
	issueKeys := make(map[string]struct{}, len(issues))
	for _, issue := range issues {
		if strings.TrimSpace(issue.IssueKey) == "" || strings.TrimSpace(issue.Code) == "" ||
			strings.TrimSpace(issue.Scope) == "" || strings.TrimSpace(issue.Summary) == "" ||
			!oneOfEpisode(issue.Severity, "warning", "blocking") {
			return errors.New("Episode candidate contains an invalid Review Issue")
		}
		if _, exists := issueKeys[issue.IssueKey]; exists {
			return errors.New("Episode candidate Review Issue keys must be unique")
		}
		issueKeys[issue.IssueKey] = struct{}{}
		if err := validateEpisodeEvidence(issue.Evidence, scope); err != nil {
			return err
		}
	}
	return nil
}

func validateEpisodeCandidateScope(scope EpisodeCandidateScope) error {
	if _, err := uuid.Parse(scope.EpisodeID); err != nil {
		return errors.New("invalid Episode candidate scope")
	}
	if _, err := uuid.Parse(scope.ScriptVersionID); err != nil || scope.EpisodePosition < 1 ||
		scope.SourceStart < 0 || scope.SourceEnd <= scope.SourceStart || scope.ContextStart > scope.SourceStart ||
		scope.ContextStart+len([]rune(scope.ContextText)) < scope.SourceEnd {
		return errors.New("invalid Episode candidate scope")
	}
	previousIdentity := ""
	for _, identity := range scope.KnownIdentities {
		if strings.TrimSpace(identity.EntityKey) == "" || previousIdentity >= identity.EntityKey {
			return errors.New("Episode candidate identities must be unique and sorted")
		}
		if !slices.IsSorted(identity.StateKeys) || slices.Contains(identity.StateKeys, "") {
			return errors.New("Episode candidate states must be sorted")
		}
		for index := 1; index < len(identity.StateKeys); index++ {
			if identity.StateKeys[index-1] == identity.StateKeys[index] {
				return errors.New("Episode candidate states must be unique")
			}
		}
		previousIdentity = identity.EntityKey
	}
	return nil
}

func validateEpisodeEvidence(values []bibledomain.Evidence, scope EpisodeCandidateScope) error {
	context := []rune(scope.ContextText)
	for _, evidence := range values {
		if evidence.SourceStart < scope.SourceStart || evidence.SourceEnd <= evidence.SourceStart ||
			evidence.SourceEnd > scope.SourceEnd || evidence.EpisodeNumber == nil ||
			*evidence.EpisodeNumber != scope.EpisodePosition || strings.TrimSpace(evidence.ExactAnchor) == "" ||
			episodeAnalysisTextHash(evidence.ExactAnchor) != evidence.TextHash {
			return errors.New("Episode candidate Evidence escaped or drifted from its source")
		}
		relativeStart, relativeEnd := evidence.SourceStart-scope.ContextStart, evidence.SourceEnd-scope.ContextStart
		if relativeStart < 0 || relativeEnd > len(context) || string(context[relativeStart:relativeEnd]) != evidence.ExactAnchor {
			return errors.New("Episode candidate Evidence does not match its frozen context")
		}
	}
	return nil
}

func episodeEvidenceKey(value bibledomain.Evidence) string {
	episode := 0
	if value.EpisodeNumber != nil {
		episode = *value.EpisodeNumber
	}
	return strings.Join([]string{
		value.ExactAnchor, value.TextHash,
		fmt.Sprint(value.SourceStart), fmt.Sprint(value.SourceEnd), fmt.Sprint(episode),
	}, "\x00")
}

func stringPointers(values []string) []*string {
	result := make([]*string, len(values))
	for index := range values {
		result[index] = &values[index]
	}
	return result
}

func oneOfEpisode(value string, allowed ...string) bool {
	return slices.Contains(allowed, value)
}

func decodeStrictEpisodeCandidate(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("candidate contains multiple JSON values")
	}
	return nil
}

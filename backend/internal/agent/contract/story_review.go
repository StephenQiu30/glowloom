package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const BibleDeterministicGateVersion = "bible-deterministic-gate-v1"

var repairFieldPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,79}$`)

var bibleRepairFields = map[string]string{
	"aliases": "strings", "ambiguities": "strings", "anchor_keys": "strings",
	"canonical_name": "text", "category": "text", "claim_type": "text",
	"entity_keys": "strings", "facts": "strings", "label": "text",
	"normalized_name": "text", "participant_keys": "strings", "polarity": "text",
	"rules": "strings", "scope": "text", "status": "text", "summary": "text", "title": "text",
}

var bibleDeterministicGateCodes = map[string]struct{}{
	"world_unknown_entity": {}, "world_duplicate_entity": {},
	"claim_unknown_participant": {}, "claim_duplicate_participant": {},
	"claim_unknown_anchor": {}, "claim_duplicate_anchor": {},
}

type StoryGraphEvidence struct {
	SourceStart   int    `json:"source_start"`
	SourceEnd     int    `json:"source_end"`
	TextHash      string `json:"text_hash"`
	ExactAnchor   string `json:"exact_anchor"`
	EpisodeNumber *int   `json:"episode_number"`
}

type StoryGraphReviewIssue struct {
	IssueKey   string               `json:"issue_key"`
	Code       string               `json:"code"`
	Severity   string               `json:"severity"`
	Scope      string               `json:"scope"`
	SubjectKey *string              `json:"subject_key"`
	Summary    string               `json:"summary"`
	RepairHint *string              `json:"repair_hint"`
	Evidence   []StoryGraphEvidence `json:"evidence"`
}

type StoryGraphGateBlocker struct {
	Code       string `json:"code"`
	SubjectKey string `json:"subject_key"`
	RelatedKey string `json:"related_key,omitempty"`
	Summary    string `json:"summary"`
}

type StoryGraphDeterministicGateResult struct {
	GateVersion                 string                  `json:"gate_version"`
	TargetCandidateRevisionID   string                  `json:"target_candidate_revision_id"`
	TargetCandidateRevisionHash string                  `json:"target_candidate_revision_hash"`
	Blockers                    []StoryGraphGateBlocker `json:"blockers"`
}

func (value StoryGraphDeterministicGateResult) Validate(targetID, targetHash string) error {
	if value.GateVersion != BibleDeterministicGateVersion ||
		value.TargetCandidateRevisionID != targetID || value.TargetCandidateRevisionHash != targetHash ||
		!hashPattern.MatchString(value.TargetCandidateRevisionHash) || value.Blockers == nil || len(value.Blockers) > 1_000 {
		return errors.New("invalid StoryGraph deterministic gate result")
	}
	if _, err := uuid.Parse(value.TargetCandidateRevisionID); err != nil {
		return errors.New("invalid StoryGraph deterministic gate target")
	}
	previous := ""
	for _, blocker := range value.Blockers {
		if strings.TrimSpace(blocker.Code) == "" || strings.TrimSpace(blocker.SubjectKey) == "" ||
			strings.TrimSpace(blocker.Summary) == "" {
			return errors.New("invalid StoryGraph deterministic gate blocker")
		}
		identity := blocker.Code + "\x00" + blocker.SubjectKey + "\x00" + blocker.RelatedKey
		if previous != "" && identity <= previous {
			return errors.New("StoryGraph deterministic gate blockers must be unique and sorted")
		}
		previous = identity
	}
	return nil
}

type StoryGraphReviewStageInput struct {
	ReviewedStage               string                            `json:"reviewed_stage"`
	TargetCandidateRevisionID   string                            `json:"target_candidate_revision_id"`
	TargetCandidateRevisionHash string                            `json:"target_candidate_revision_hash"`
	CandidateItemStart          int                               `json:"candidate_item_start"`
	CandidateItemEnd            int                               `json:"candidate_item_end"`
	TargetCandidate             json.RawMessage                   `json:"target_candidate"`
	DeterministicGate           StoryGraphDeterministicGateResult `json:"deterministic_gate"`
}

func (value StoryGraphReviewStageInput) Validate() error {
	if value.ReviewedStage != "reconcile_story" || !hashPattern.MatchString(value.TargetCandidateRevisionHash) ||
		value.CandidateItemStart < 0 || value.CandidateItemEnd <= value.CandidateItemStart || !jsonObject(value.TargetCandidate) {
		return errors.New("invalid StoryGraph review stage input")
	}
	if _, err := uuid.Parse(value.TargetCandidateRevisionID); err != nil {
		return errors.New("invalid StoryGraph review target revision")
	}
	itemCount, err := storyReconciliationCandidateItemCount(value.TargetCandidate)
	if err != nil || value.CandidateItemEnd-value.CandidateItemStart != itemCount {
		return errors.New("StoryGraph review range does not match its exact candidate partition")
	}
	return value.DeterministicGate.Validate(value.TargetCandidateRevisionID, value.TargetCandidateRevisionHash)
}

type StoryGraphReviewCandidate struct {
	ReviewedStage               string                  `json:"reviewed_stage"`
	TargetCandidateRevisionID   string                  `json:"target_candidate_revision_id"`
	TargetCandidateRevisionHash string                  `json:"target_candidate_revision_hash"`
	ReviewIssues                []StoryGraphReviewIssue `json:"review_issues"`
}

func DecodeStoryGraphReviewCandidate(raw []byte) (StoryGraphReviewCandidate, error) {
	var value StoryGraphReviewCandidate
	if err := decodeStrict(raw, &value); err != nil {
		return StoryGraphReviewCandidate{}, err
	}
	return value, nil
}

func ValidateStoryGraphReviewCandidate(input StoryGraphReviewStageInput, value StoryGraphReviewCandidate) error {
	if err := input.Validate(); err != nil {
		return err
	}
	if value.ReviewedStage != input.ReviewedStage || value.TargetCandidateRevisionID != input.TargetCandidateRevisionID ||
		value.TargetCandidateRevisionHash != input.TargetCandidateRevisionHash || value.ReviewIssues == nil ||
		len(value.ReviewIssues) > 1_000 {
		return errors.New("StoryGraph review candidate does not match its frozen target")
	}
	allowedEvidence, err := collectStoryGraphEvidence(input.TargetCandidate)
	if err != nil {
		return err
	}
	return validateStoryGraphReviewIssues(value.ReviewIssues, allowedEvidence, true)
}

type StoryGraphRepairAllowedTarget struct {
	CandidateKey     string          `json:"candidate_key"`
	AllowedFields    []string        `json:"allowed_fields"`
	BaseFragmentHash string          `json:"base_fragment_hash"`
	Fragment         json.RawMessage `json:"fragment"`
}

type StoryGraphRepairReadOnlyFragment struct {
	CandidateKey string          `json:"candidate_key"`
	FragmentHash string          `json:"fragment_hash"`
	Fragment     json.RawMessage `json:"fragment"`
}

type StoryGraphRepairStageInput struct {
	TargetCandidateRevisionID   string                             `json:"target_candidate_revision_id"`
	TargetCandidateRevisionHash string                             `json:"target_candidate_revision_hash"`
	ReviewCandidateRevisionID   string                             `json:"review_candidate_revision_id"`
	ReviewCandidateRevisionHash string                             `json:"review_candidate_revision_hash"`
	TargetIssue                 StoryGraphReviewIssue              `json:"target_issue"`
	AllowedTargets              []StoryGraphRepairAllowedTarget    `json:"allowed_targets"`
	ReadOnlyAdjacency           []StoryGraphRepairReadOnlyFragment `json:"read_only_adjacency"`
	RepairRound                 int                                `json:"repair_round"`
	MaxRepairRounds             int                                `json:"max_repair_rounds"`
}

func (value StoryGraphRepairStageInput) Validate() error {
	for _, identifier := range []string{value.TargetCandidateRevisionID, value.ReviewCandidateRevisionID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid StoryGraph repair revision identity")
		}
	}
	if !hashPattern.MatchString(value.TargetCandidateRevisionHash) ||
		!hashPattern.MatchString(value.ReviewCandidateRevisionHash) || value.AllowedTargets == nil ||
		len(value.AllowedTargets) < 1 || len(value.AllowedTargets) > 32 || value.ReadOnlyAdjacency == nil ||
		len(value.ReadOnlyAdjacency) > 64 || value.RepairRound < 1 || value.MaxRepairRounds < value.RepairRound ||
		value.MaxRepairRounds > 3 {
		return errors.New("invalid StoryGraph repair stage input")
	}
	if err := validateStoryGraphReviewIssue(value.TargetIssue, true); err != nil || value.TargetIssue.Severity != "blocking" {
		return errors.New("StoryGraph repair target must be one blocking review issue")
	}
	if _, reserved := bibleDeterministicGateCodes[value.TargetIssue.Code]; reserved {
		return errors.New("StoryGraph repair target cannot impersonate a deterministic Gate blocker")
	}
	keys := make(map[string]struct{}, len(value.AllowedTargets)+len(value.ReadOnlyAdjacency))
	subjectAllowed := value.TargetIssue.SubjectKey == nil
	for _, target := range value.AllowedTargets {
		if strings.TrimSpace(target.CandidateKey) == "" || len(target.AllowedFields) == 0 ||
			len(target.AllowedFields) > 32 || !slices.IsSorted(target.AllowedFields) || !jsonObject(target.Fragment) ||
			!hashPattern.MatchString(target.BaseFragmentHash) {
			return errors.New("invalid StoryGraph repair allowed target")
		}
		if _, exists := keys[target.CandidateKey]; exists {
			return errors.New("duplicate StoryGraph repair boundary key")
		}
		keys[target.CandidateKey] = struct{}{}
		if value.TargetIssue.SubjectKey != nil && target.CandidateKey == *value.TargetIssue.SubjectKey {
			subjectAllowed = true
		}
		previous := ""
		for _, field := range target.AllowedFields {
			_, supported := bibleRepairFields[field]
			if !repairFieldPattern.MatchString(field) || !supported || field == previous {
				return errors.New("invalid StoryGraph repair allowed field")
			}
			previous = field
		}
		computed, err := StoryGraphCandidateFragmentHash(target.Fragment)
		if err != nil || computed != target.BaseFragmentHash {
			return errors.New("StoryGraph repair allowed fragment hash mismatch")
		}
	}
	if !subjectAllowed {
		return errors.New("StoryGraph repair target issue is outside the allowed boundary")
	}
	for _, adjacent := range value.ReadOnlyAdjacency {
		if strings.TrimSpace(adjacent.CandidateKey) == "" || !jsonObject(adjacent.Fragment) ||
			!hashPattern.MatchString(adjacent.FragmentHash) {
			return errors.New("invalid StoryGraph repair read-only adjacency")
		}
		if _, exists := keys[adjacent.CandidateKey]; exists {
			return errors.New("duplicate StoryGraph repair boundary key")
		}
		keys[adjacent.CandidateKey] = struct{}{}
		computed, err := StoryGraphCandidateFragmentHash(adjacent.Fragment)
		if err != nil || computed != adjacent.FragmentHash {
			return errors.New("StoryGraph repair read-only fragment hash mismatch")
		}
	}
	return nil
}

type StoryGraphRepairReplacement struct {
	Text    *string   `json:"text"`
	Integer *int      `json:"integer"`
	Flag    *bool     `json:"flag"`
	Strings *[]string `json:"strings"`
}

type StoryGraphRepairOperation struct {
	TargetCandidateKey string                      `json:"target_candidate_key"`
	BaseFragmentHash   string                      `json:"base_fragment_hash"`
	FieldName          string                      `json:"field_name"`
	Replacement        StoryGraphRepairReplacement `json:"replacement"`
}

type CandidateRepairPatch struct {
	TargetCandidateRevisionID   string                      `json:"target_candidate_revision_id"`
	TargetCandidateRevisionHash string                      `json:"target_candidate_revision_hash"`
	Operations                  []StoryGraphRepairOperation `json:"operations"`
	ReviewIssues                []StoryGraphReviewIssue     `json:"review_issues"`
}

func DecodeCandidateRepairPatch(raw []byte) (CandidateRepairPatch, error) {
	var value CandidateRepairPatch
	if err := decodeStrict(raw, &value); err != nil {
		return CandidateRepairPatch{}, err
	}
	return value, nil
}

func ValidateCandidateRepairPatch(input StoryGraphRepairStageInput, value CandidateRepairPatch) error {
	if err := input.Validate(); err != nil {
		return err
	}
	if value.TargetCandidateRevisionID != input.TargetCandidateRevisionID ||
		value.TargetCandidateRevisionHash != input.TargetCandidateRevisionHash || value.Operations == nil ||
		len(value.Operations) < 1 || len(value.Operations) > 64 || value.ReviewIssues == nil || len(value.ReviewIssues) > 1_000 {
		return errors.New("Candidate repair Patch does not match its frozen target")
	}
	allowed := make(map[string]StoryGraphRepairAllowedTarget, len(input.AllowedTargets))
	for _, target := range input.AllowedTargets {
		allowed[target.CandidateKey] = target
	}
	seen := make(map[string]struct{}, len(value.Operations))
	for _, operation := range value.Operations {
		target, exists := allowed[operation.TargetCandidateKey]
		if !exists || operation.BaseFragmentHash != target.BaseFragmentHash ||
			!slices.Contains(target.AllowedFields, operation.FieldName) {
			return errors.New("Candidate repair Patch escaped its frozen allowlist or fragment")
		}
		identity := operation.TargetCandidateKey + "\x00" + operation.FieldName
		if _, exists = seen[identity]; exists {
			return errors.New("Candidate repair Patch contains a duplicate operation")
		}
		seen[identity] = struct{}{}
		if replacementCount(operation.Replacement) != 1 ||
			replacementKind(operation.Replacement) != bibleRepairFields[operation.FieldName] {
			return errors.New("Candidate repair Patch replacement must contain one typed value")
		}
	}
	allowedEvidence := make(map[string]struct{}, len(input.TargetIssue.Evidence))
	for _, evidence := range input.TargetIssue.Evidence {
		allowedEvidence[storyGraphEvidenceKey(evidence)] = struct{}{}
	}
	return validateStoryGraphReviewIssues(value.ReviewIssues, allowedEvidence, false)
}

func StoryGraphCandidateFragmentHash(fragment json.RawMessage) (string, error) {
	if !jsonObject(fragment) {
		return "", errors.New("StoryGraph candidate fragment must be an object")
	}
	return CanonicalHash(fragment)
}

func storyReconciliationCandidateItemCount(raw json.RawMessage) (int, error) {
	var value struct {
		CanonicalEntities     []json.RawMessage `json:"canonical_entities"`
		CanonicalWorldEntries []json.RawMessage `json:"canonical_world_entries"`
		MergedClaims          []json.RawMessage `json:"merged_claims"`
		MergedArcs            []json.RawMessage `json:"merged_arcs"`
		Conflicts             []json.RawMessage `json:"conflicts"`
		ReviewIssues          []json.RawMessage `json:"review_issues"`
	}
	if err := decodeStrict(raw, &value); err != nil || value.CanonicalEntities == nil ||
		value.CanonicalWorldEntries == nil || value.MergedClaims == nil || value.MergedArcs == nil ||
		value.Conflicts == nil || value.ReviewIssues == nil {
		return 0, errors.New("invalid Story reconciliation review candidate")
	}
	return len(value.CanonicalEntities) + len(value.CanonicalWorldEntries) + len(value.MergedClaims) +
		len(value.MergedArcs) + len(value.Conflicts) + len(value.ReviewIssues), nil
}

func collectStoryGraphEvidence(raw json.RawMessage) (map[string]struct{}, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, errors.New("invalid frozen StoryGraph candidate")
	}
	result := map[string]struct{}{}
	var visit func(any) error
	visit = func(current any) error {
		switch typed := current.(type) {
		case []any:
			for _, item := range typed {
				if err := visit(item); err != nil {
					return err
				}
			}
		case map[string]any:
			if _, ok := typed["source_start"]; ok {
				encoded, err := json.Marshal(typed)
				if err != nil {
					return err
				}
				var evidence StoryGraphEvidence
				if err = decodeStrict(encoded, &evidence); err != nil || validateStoryGraphEvidence(evidence) != nil {
					return errors.New("invalid Evidence in frozen StoryGraph candidate")
				}
				result[storyGraphEvidenceKey(evidence)] = struct{}{}
				return nil
			}
			for _, item := range typed {
				if err := visit(item); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(value); err != nil {
		return nil, err
	}
	return result, nil
}

func validateStoryGraphReviewIssues(values []StoryGraphReviewIssue, allowedEvidence map[string]struct{}, requireEvidence bool) error {
	seen := make(map[string]struct{}, len(values))
	for _, issue := range values {
		if err := validateStoryGraphReviewIssue(issue, requireEvidence); err != nil {
			return err
		}
		if _, reserved := bibleDeterministicGateCodes[issue.Code]; reserved {
			return errors.New("model Review Issue cannot use a deterministic Gate code")
		}
		if _, exists := seen[issue.IssueKey]; exists {
			return errors.New("StoryGraph review issue keys must be unique")
		}
		seen[issue.IssueKey] = struct{}{}
		for _, evidence := range issue.Evidence {
			if _, exists := allowedEvidence[storyGraphEvidenceKey(evidence)]; !exists {
				return errors.New("StoryGraph Review Issue Evidence is outside the frozen Candidate Revision")
			}
		}
	}
	return nil
}

func validateStoryGraphReviewIssue(value StoryGraphReviewIssue, requireEvidence bool) error {
	if strings.TrimSpace(value.IssueKey) == "" || strings.TrimSpace(value.Code) == "" ||
		(value.Severity != "warning" && value.Severity != "blocking") || strings.TrimSpace(value.Scope) == "" ||
		strings.TrimSpace(value.Summary) == "" || value.Evidence == nil || requireEvidence && len(value.Evidence) == 0 {
		return errors.New("invalid StoryGraph review issue")
	}
	for _, evidence := range value.Evidence {
		if err := validateStoryGraphEvidence(evidence); err != nil {
			return err
		}
	}
	return nil
}

func validateStoryGraphEvidence(value StoryGraphEvidence) error {
	if value.SourceStart < 0 || value.SourceEnd <= value.SourceStart ||
		utf8.RuneCountInString(value.ExactAnchor) != value.SourceEnd-value.SourceStart ||
		!hashPattern.MatchString(value.TextHash) || value.EpisodeNumber != nil && *value.EpisodeNumber < 1 {
		return errors.New("invalid StoryGraph review Evidence")
	}
	return nil
}

func storyGraphEvidenceKey(value StoryGraphEvidence) string {
	episode := ""
	if value.EpisodeNumber != nil {
		episode = fmt.Sprint(*value.EpisodeNumber)
	}
	return fmt.Sprintf("%d:%d:%s:%s:%s", value.SourceStart, value.SourceEnd, value.TextHash, value.ExactAnchor, episode)
}

func replacementCount(value StoryGraphRepairReplacement) int {
	count := 0
	for _, present := range []bool{value.Text != nil, value.Integer != nil, value.Flag != nil, value.Strings != nil} {
		if present {
			count++
		}
	}
	return count
}

func replacementKind(value StoryGraphRepairReplacement) string {
	switch {
	case value.Text != nil:
		return "text"
	case value.Integer != nil:
		return "integer"
	case value.Flag != nil:
		return "flag"
	case value.Strings != nil:
		return "strings"
	default:
		return ""
	}
}

package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const SourceEvidenceStage = "extract_source_evidence"

var (
	arabicEpisodeMarker  = regexp.MustCompile(`(?i)^episode\s*([0-9]+)\b`)
	chineseEpisodeMarker = regexp.MustCompile(`^第([0-9〇零一二两三四五六七八九十百千]+)集`)
)

type EpisodeMarkerHint struct {
	EpisodeNumber int    `json:"episode_number"`
	Label         string `json:"label"`
	AbsoluteStart int    `json:"absolute_start"`
	AbsoluteEnd   int    `json:"absolute_end"`
}

type SourceEvidenceShard struct {
	Key                string              `json:"shard_key"`
	TreePath           string              `json:"tree_path"`
	ParentKey          string              `json:"parent_shard_key,omitempty"`
	Kind               string              `json:"kind"`
	LogicalStart       int                 `json:"logical_start"`
	LogicalEnd         int                 `json:"logical_end"`
	ContextStart       int                 `json:"context_start"`
	ContextEnd         int                 `json:"context_end"`
	SourceHashes       []string            `json:"source_hashes"`
	EpisodeMarkerHints []EpisodeMarkerHint `json:"episode_marker_hints"`
	Status             string              `json:"status"`
}

type SourceEvidenceManifest struct {
	ManifestID         string                `json:"manifest_id"`
	Version            int64                 `json:"version"`
	ParentManifestHash *string               `json:"parent_manifest_hash"`
	WorkspaceID        string                `json:"workspace_id"`
	WorkflowRunID      string                `json:"workflow_run_id"`
	NodeRunID          string                `json:"node_run_id"`
	Stage              string                `json:"stage"`
	RootInputHash      string                `json:"root_input_hash"`
	Shards             []SourceEvidenceShard `json:"shards"`
	CoverageHash       string                `json:"coverage_hash"`
	ManifestHash       string                `json:"manifest_hash"`
}

type SourceEvidenceManifestInput struct {
	ManifestID, WorkspaceID, WorkflowRunID, NodeRunID, RootInputHash string
	NormalizedText                                                   string
	MaxShardCodePoints, OverlapCodePoints                            int
}

func BuildSourceEvidenceManifest(input SourceEvidenceManifestInput) (SourceEvidenceManifest, error) {
	if err := validateManifestInput(input); err != nil {
		return SourceEvidenceManifest{}, err
	}
	ranges := sourceRanges(input.NormalizedText, 0, len([]rune(input.NormalizedText)), input.MaxShardCodePoints)
	shards := make([]SourceEvidenceShard, len(ranges))
	for index, value := range ranges {
		shards[index] = buildSourceShard(
			input.NormalizedText,
			fmt.Sprintf("source:%08d:%08d", value.start, value.end),
			fmt.Sprintf("%04d", index),
			"",
			value.start,
			value.end,
			input.OverlapCodePoints,
		)
	}
	manifest := SourceEvidenceManifest{
		ManifestID: input.ManifestID, Version: 1, WorkspaceID: input.WorkspaceID,
		WorkflowRunID: input.WorkflowRunID, NodeRunID: input.NodeRunID,
		Stage: SourceEvidenceStage, RootInputHash: input.RootInputHash, Shards: shards,
	}
	if err := sealSourceEvidenceManifest(&manifest, input.NormalizedText); err != nil {
		return SourceEvidenceManifest{}, err
	}
	return manifest, nil
}

func ReshardSourceEvidenceManifest(
	current SourceEvidenceManifest,
	parentKey string,
	normalizedText string,
	maxChildCodePoints int,
	overlapCodePoints int,
) (SourceEvidenceManifest, error) {
	if err := ValidateSourceEvidenceManifest(current, normalizedText); err != nil {
		return SourceEvidenceManifest{}, err
	}
	if maxChildCodePoints < 1 || overlapCodePoints < 0 {
		return SourceEvidenceManifest{}, errors.New("invalid source Evidence reshard budget")
	}
	parentIndex := -1
	for index, shard := range current.Shards {
		if shard.Key == parentKey && shard.Status == "active" {
			parentIndex = index
			break
		}
	}
	if parentIndex < 0 {
		return SourceEvidenceManifest{}, errors.New("active source Evidence parent shard not found")
	}
	parent := current.Shards[parentIndex]
	if parent.LogicalEnd-parent.LogicalStart <= 1 || maxChildCodePoints >= parent.LogicalEnd-parent.LogicalStart {
		return SourceEvidenceManifest{}, errors.New("source Evidence shard cannot be reduced by the requested budget")
	}
	ranges := sourceRanges(normalizedText, parent.LogicalStart, parent.LogicalEnd, maxChildCodePoints)
	if len(ranges) < 2 {
		return SourceEvidenceManifest{}, errors.New("source Evidence reshard did not produce child shards")
	}
	shards := append([]SourceEvidenceShard(nil), current.Shards...)
	shards[parentIndex].Status = "superseded"
	for index, value := range ranges {
		shards = append(shards, buildSourceShard(
			normalizedText,
			fmt.Sprintf("%s.%04d", parent.Key, index),
			fmt.Sprintf("%s.%04d", parent.TreePath, index),
			parent.Key,
			value.start,
			value.end,
			overlapCodePoints,
		))
	}
	parentHash := current.ManifestHash
	next := SourceEvidenceManifest{
		ManifestID: current.ManifestID, Version: current.Version + 1,
		ParentManifestHash: &parentHash, WorkspaceID: current.WorkspaceID,
		WorkflowRunID: current.WorkflowRunID, NodeRunID: current.NodeRunID,
		Stage: current.Stage, RootInputHash: current.RootInputHash, Shards: shards,
	}
	if err := sealSourceEvidenceManifest(&next, normalizedText); err != nil {
		return SourceEvidenceManifest{}, err
	}
	return next, nil
}

func ValidateSourceEvidenceManifest(value SourceEvidenceManifest, normalizedText string) error {
	if _, err := uuid.Parse(value.ManifestID); err != nil || value.Version < 1 ||
		value.Stage != SourceEvidenceStage || !hashPattern.MatchString(value.RootInputHash) ||
		!hashPattern.MatchString(value.CoverageHash) || !hashPattern.MatchString(value.ManifestHash) {
		return errors.New("invalid source Evidence manifest identity")
	}
	for _, identifier := range []string{value.WorkspaceID, value.WorkflowRunID, value.NodeRunID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid source Evidence manifest owner")
		}
	}
	if value.Version == 1 && value.ParentManifestHash != nil || value.Version > 1 &&
		(value.ParentManifestHash == nil || !hashPattern.MatchString(*value.ParentManifestHash)) {
		return errors.New("invalid source Evidence manifest lineage")
	}
	text := []rune(normalizedText)
	active := make([]SourceEvidenceShard, 0, len(value.Shards))
	keys := make(map[string]struct{}, len(value.Shards))
	for _, shard := range value.Shards {
		if strings.TrimSpace(shard.Key) == "" || strings.TrimSpace(shard.TreePath) == "" ||
			shard.Kind != "source_slice" || (shard.Status != "active" && shard.Status != "superseded") ||
			shard.LogicalStart < 0 || shard.LogicalEnd <= shard.LogicalStart || shard.LogicalEnd > len(text) ||
			shard.ContextStart < 0 || shard.ContextStart > shard.LogicalStart ||
			shard.ContextEnd < shard.LogicalEnd || shard.ContextEnd > len(text) || len(shard.SourceHashes) != 1 ||
			shard.SourceHashes[0] != SourceTextHash(string(text[shard.LogicalStart:shard.LogicalEnd])) {
			return errors.New("invalid source Evidence shard")
		}
		if _, exists := keys[shard.Key]; exists {
			return errors.New("source Evidence shard keys must be unique")
		}
		keys[shard.Key] = struct{}{}
		for _, marker := range shard.EpisodeMarkerHints {
			if marker.EpisodeNumber < 1 || strings.TrimSpace(marker.Label) == "" ||
				marker.AbsoluteStart < shard.ContextStart || marker.AbsoluteEnd <= marker.AbsoluteStart ||
				marker.AbsoluteEnd > shard.ContextEnd {
				return errors.New("invalid source Evidence episode marker hint")
			}
		}
		if shard.Status == "active" {
			active = append(active, shard)
		}
	}
	if len(active) == 0 {
		return errors.New("source Evidence manifest has no active shards")
	}
	slices.SortFunc(active, func(left, right SourceEvidenceShard) int {
		return left.LogicalStart - right.LogicalStart
	})
	position := 0
	for _, shard := range active {
		if shard.LogicalStart != position {
			return errors.New("source Evidence active coverage contains a gap or overlap")
		}
		position = shard.LogicalEnd
	}
	if position != len(text) {
		return errors.New("source Evidence active coverage is incomplete")
	}
	coverageHash, err := sourceCoverageHash(value.RootInputHash, len(text))
	if err != nil || coverageHash != value.CoverageHash {
		return errors.New("source Evidence coverage hash mismatch")
	}
	manifestHash, err := sourceManifestHash(value)
	if err != nil || manifestHash != value.ManifestHash {
		return errors.New("source Evidence manifest hash mismatch")
	}
	return nil
}

type SourceObservation struct {
	ObservationKey string     `json:"observation_key"`
	Kind           string     `json:"kind"`
	ProposedKey    string     `json:"proposed_key"`
	Label          string     `json:"label"`
	Facts          []string   `json:"facts"`
	Evidence       []Evidence `json:"evidence"`
	Ambiguities    []string   `json:"ambiguities"`
}

type SourceEvidenceIssue struct {
	IssueKey   string     `json:"issue_key"`
	Code       string     `json:"code"`
	Severity   string     `json:"severity"`
	Scope      string     `json:"scope"`
	SubjectKey *string    `json:"subject_key"`
	Summary    string     `json:"summary"`
	RepairHint *string    `json:"repair_hint"`
	Evidence   []Evidence `json:"evidence"`
}

type SourceEvidenceCandidate struct {
	Observations []SourceObservation   `json:"observations"`
	ReviewIssues []SourceEvidenceIssue `json:"review_issues"`
}

type SourceEvidenceFragment struct {
	ShardKey              string                  `json:"shard_key"`
	LogicalStart          int                     `json:"logical_start"`
	LogicalEnd            int                     `json:"logical_end"`
	CandidateRevisionID   string                  `json:"candidate_revision_id"`
	CandidateRevisionHash string                  `json:"candidate_revision_hash"`
	Candidate             SourceEvidenceCandidate `json:"candidate"`
}

type SourceEvidenceAggregateCandidate struct {
	ManifestID      string                   `json:"manifest_id"`
	ManifestVersion int64                    `json:"manifest_version"`
	ManifestHash    string                   `json:"manifest_hash"`
	CoverageHash    string                   `json:"coverage_hash"`
	Fragments       []SourceEvidenceFragment `json:"fragments"`
}

func BuildSourceEvidenceAggregate(
	manifest SourceEvidenceManifest,
	fragments []SourceEvidenceFragment,
) (SourceEvidenceAggregateCandidate, json.RawMessage, string, error) {
	active := make([]SourceEvidenceShard, 0, len(manifest.Shards))
	for _, shard := range manifest.Shards {
		if shard.Status == "active" {
			active = append(active, shard)
		}
	}
	slices.SortFunc(active, func(left, right SourceEvidenceShard) int {
		return left.LogicalStart - right.LogicalStart
	})
	fragments = append([]SourceEvidenceFragment(nil), fragments...)
	slices.SortFunc(fragments, func(left, right SourceEvidenceFragment) int {
		if left.LogicalStart != right.LogicalStart {
			return left.LogicalStart - right.LogicalStart
		}
		return strings.Compare(left.ShardKey, right.ShardKey)
	})
	if len(active) != len(fragments) {
		return SourceEvidenceAggregateCandidate{}, nil, "", errors.New("source Evidence aggregate is missing an active shard")
	}
	for index, shard := range active {
		fragment := fragments[index]
		if fragment.ShardKey != shard.Key || fragment.LogicalStart != shard.LogicalStart ||
			fragment.LogicalEnd != shard.LogicalEnd || !hashPattern.MatchString(fragment.CandidateRevisionHash) {
			return SourceEvidenceAggregateCandidate{}, nil, "", errors.New("source Evidence aggregate fragment does not match the manifest")
		}
		if _, err := uuid.Parse(fragment.CandidateRevisionID); err != nil {
			return SourceEvidenceAggregateCandidate{}, nil, "", errors.New("invalid source Evidence aggregate candidate reference")
		}
	}
	value := SourceEvidenceAggregateCandidate{
		ManifestID: manifest.ManifestID, ManifestVersion: manifest.Version,
		ManifestHash: manifest.ManifestHash, CoverageHash: manifest.CoverageHash,
		Fragments: fragments,
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return SourceEvidenceAggregateCandidate{}, nil, "", err
	}
	return value, encoded, SourceTextHash(string(encoded)), nil
}

func DecodeSourceEvidenceAggregate(raw json.RawMessage) (SourceEvidenceAggregateCandidate, error) {
	var value SourceEvidenceAggregateCandidate
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return SourceEvidenceAggregateCandidate{}, errors.New("candidate does not match source Evidence aggregate schema")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return SourceEvidenceAggregateCandidate{}, errors.New("source Evidence aggregate contains multiple JSON values")
	}
	if _, err := uuid.Parse(value.ManifestID); err != nil || value.ManifestVersion < 1 ||
		!hashPattern.MatchString(value.ManifestHash) || !hashPattern.MatchString(value.CoverageHash) || len(value.Fragments) == 0 {
		return SourceEvidenceAggregateCandidate{}, errors.New("invalid source Evidence aggregate identity")
	}
	for _, fragment := range value.Fragments {
		if strings.TrimSpace(fragment.ShardKey) == "" || fragment.LogicalStart < 0 ||
			fragment.LogicalEnd <= fragment.LogicalStart || !hashPattern.MatchString(fragment.CandidateRevisionHash) {
			return SourceEvidenceAggregateCandidate{}, errors.New("invalid source Evidence aggregate fragment")
		}
		if _, err := uuid.Parse(fragment.CandidateRevisionID); err != nil {
			return SourceEvidenceAggregateCandidate{}, errors.New("invalid source Evidence aggregate fragment revision")
		}
	}
	return value, nil
}

func SourceEvidenceCandidateEvidence(value SourceEvidenceCandidate) []Evidence {
	result := []Evidence{}
	for _, observation := range value.Observations {
		result = append(result, observation.Evidence...)
	}
	for _, issue := range value.ReviewIssues {
		result = append(result, issue.Evidence...)
	}
	return uniqueStoryEvidence(result)
}

func SourceEvidenceAggregateStageInstanceKey(manifest SourceEvidenceManifest) string {
	material := "storygraph-stage-aggregate" + manifest.Stage + manifest.ManifestHash + manifest.RootInputHash
	return SourceTextHash(material)
}

func DecodeAndNormalizeSourceEvidenceCandidate(
	raw json.RawMessage,
	normalizedText string,
	shard SourceEvidenceShard,
) (SourceEvidenceCandidate, error) {
	var candidate SourceEvidenceCandidate
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&candidate); err != nil {
		return SourceEvidenceCandidate{}, errors.New("candidate does not match source Evidence schema")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return SourceEvidenceCandidate{}, errors.New("candidate contains multiple JSON values")
	}
	if len(candidate.Observations) > 5000 || len(candidate.ReviewIssues) > 5000 {
		return SourceEvidenceCandidate{}, errors.New("source Evidence candidate exceeds limits")
	}
	observationKeys := map[string]struct{}{}
	for index := range candidate.Observations {
		observation := &candidate.Observations[index]
		if !keyPattern.MatchString(observation.ObservationKey) ||
			!oneOf(observation.Kind, "entity", "entity_state", "world_entry", "event", "marker") ||
			strings.TrimSpace(observation.ProposedKey) == "" || strings.TrimSpace(observation.Label) == "" ||
			len(observation.Evidence) == 0 {
			return SourceEvidenceCandidate{}, errors.New("candidate contains an invalid source observation")
		}
		if _, exists := observationKeys[observation.ObservationKey]; exists {
			return SourceEvidenceCandidate{}, errors.New("source observation keys must be unique")
		}
		observationKeys[observation.ObservationKey] = struct{}{}
		normalized, err := normalizeSourceEvidence(observation.Evidence, normalizedText, shard)
		if err != nil {
			return SourceEvidenceCandidate{}, err
		}
		observation.Evidence = normalized
	}
	issueKeys := map[string]struct{}{}
	for index := range candidate.ReviewIssues {
		issue := &candidate.ReviewIssues[index]
		if !keyPattern.MatchString(issue.IssueKey) || strings.TrimSpace(issue.Code) == "" ||
			!oneOf(issue.Severity, "warning", "blocking") || strings.TrimSpace(issue.Scope) == "" ||
			strings.TrimSpace(issue.Summary) == "" {
			return SourceEvidenceCandidate{}, errors.New("candidate contains an invalid source Evidence issue")
		}
		if _, exists := issueKeys[issue.IssueKey]; exists {
			return SourceEvidenceCandidate{}, errors.New("source Evidence issue keys must be unique")
		}
		issueKeys[issue.IssueKey] = struct{}{}
		normalized, err := normalizeSourceEvidence(issue.Evidence, normalizedText, shard)
		if err != nil {
			return SourceEvidenceCandidate{}, err
		}
		issue.Evidence = normalized
	}
	return candidate, nil
}

func SourceTextHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func normalizeSourceEvidence(values []Evidence, normalizedText string, shard SourceEvidenceShard) ([]Evidence, error) {
	text := []rune(normalizedText)
	seen := map[string]struct{}{}
	result := make([]Evidence, 0, len(values))
	for _, value := range values {
		anchor := []rune(value.ExactAnchor)
		if len(anchor) == 0 || !utf8.ValidString(value.ExactAnchor) || value.TextHash != SourceTextHash(value.ExactAnchor) ||
			value.SourceStart < 0 || value.SourceEnd <= value.SourceStart || value.SourceEnd-value.SourceStart != len(anchor) {
			return nil, errors.New("candidate source Evidence hash or range is invalid")
		}
		start, end := value.SourceStart, value.SourceEnd
		absoluteMatches := start >= shard.ContextStart && end <= shard.ContextEnd && end <= len(text) &&
			string(text[start:end]) == value.ExactAnchor
		if !absoluteMatches {
			start, end = shard.ContextStart+value.SourceStart, shard.ContextStart+value.SourceEnd
			if start < shard.ContextStart || end > shard.ContextEnd || end > len(text) ||
				string(text[start:end]) != value.ExactAnchor {
				return nil, errors.New("candidate source Evidence does not match the immutable source slice")
			}
		}
		value.SourceStart, value.SourceEnd = start, end
		key := strconv.Itoa(start) + ":" + strconv.Itoa(end) + ":" + value.TextHash
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	slices.SortFunc(result, func(left, right Evidence) int {
		if left.SourceStart != right.SourceStart {
			return left.SourceStart - right.SourceStart
		}
		if left.SourceEnd != right.SourceEnd {
			return left.SourceEnd - right.SourceEnd
		}
		return strings.Compare(left.TextHash, right.TextHash)
	})
	return result, nil
}

type sourceRange struct{ start, end int }

func validateManifestInput(value SourceEvidenceManifestInput) error {
	for _, identifier := range []string{value.ManifestID, value.WorkspaceID, value.WorkflowRunID, value.NodeRunID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid source Evidence manifest identifier")
		}
	}
	if !hashPattern.MatchString(value.RootInputHash) || value.MaxShardCodePoints < 1 ||
		value.OverlapCodePoints < 0 || len([]rune(value.NormalizedText)) == 0 || !utf8.ValidString(value.NormalizedText) {
		return errors.New("invalid source Evidence manifest input")
	}
	return nil
}

func sourceRanges(normalizedText string, start, end, maximum int) []sourceRange {
	boundaries := semanticBoundaries(normalizedText)
	result := make([]sourceRange, 0, (end-start+maximum-1)/maximum)
	for position := start; position < end; {
		limit := min(position+maximum, end)
		selected := limit
		for _, boundary := range boundaries {
			if boundary > position && boundary <= limit {
				selected = boundary
			}
		}
		if selected <= position {
			selected = limit
		}
		result = append(result, sourceRange{position, selected})
		position = selected
	}
	return result
}

func semanticBoundaries(normalizedText string) []int {
	text := []rune(normalizedText)
	boundaries := []int{len(text)}
	lineStart := 0
	for index := 0; index <= len(text); index++ {
		if index < len(text) && text[index] != '\n' {
			continue
		}
		line := strings.TrimSpace(string(text[lineStart:index]))
		if line != "" && episodeNumber(line) > 0 && lineStart > 0 {
			boundaries = append(boundaries, lineStart)
		}
		if index < len(text) {
			boundaries = append(boundaries, index+1)
		}
		lineStart = index + 1
	}
	slices.Sort(boundaries)
	return slices.Compact(boundaries)
}

func buildSourceShard(normalizedText, key, treePath, parentKey string, start, end, overlap int) SourceEvidenceShard {
	text := []rune(normalizedText)
	contextStart, contextEnd := max(0, start-overlap), min(len(text), end+overlap)
	return SourceEvidenceShard{
		Key: key, TreePath: treePath, ParentKey: parentKey, Kind: "source_slice",
		LogicalStart: start, LogicalEnd: end, ContextStart: contextStart, ContextEnd: contextEnd,
		SourceHashes:       []string{SourceTextHash(string(text[start:end]))},
		EpisodeMarkerHints: markerHints(normalizedText, contextStart, contextEnd), Status: "active",
	}
}

func markerHints(normalizedText string, contextStart, contextEnd int) []EpisodeMarkerHint {
	text := []rune(normalizedText)
	result := []EpisodeMarkerHint{}
	lineStart := 0
	for index := 0; index <= len(text); index++ {
		if index < len(text) && text[index] != '\n' {
			continue
		}
		label := strings.TrimSpace(string(text[lineStart:index]))
		number := episodeNumber(label)
		if number > 0 && lineStart >= contextStart && index <= contextEnd {
			result = append(result, EpisodeMarkerHint{
				EpisodeNumber: number, Label: label, AbsoluteStart: lineStart, AbsoluteEnd: index,
			})
		}
		lineStart = index + 1
	}
	return result
}

func episodeNumber(label string) int {
	if matches := arabicEpisodeMarker.FindStringSubmatch(label); len(matches) == 2 {
		value, _ := strconv.Atoi(matches[1])
		return value
	}
	matches := chineseEpisodeMarker.FindStringSubmatch(label)
	if len(matches) != 2 {
		return 0
	}
	if value, err := strconv.Atoi(matches[1]); err == nil {
		return value
	}
	return parseChineseNumber(matches[1])
}

func parseChineseNumber(value string) int {
	digits := map[rune]int{'〇': 0, '零': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	units := map[rune]int{'十': 10, '百': 100, '千': 1000}
	total, current := 0, 0
	for _, character := range value {
		if digit, ok := digits[character]; ok {
			current = digit
			continue
		}
		unit, ok := units[character]
		if !ok {
			return 0
		}
		if current == 0 {
			current = 1
		}
		total += current * unit
		current = 0
	}
	return total + current
}

func sealSourceEvidenceManifest(value *SourceEvidenceManifest, normalizedText string) error {
	coverageHash, err := sourceCoverageHash(value.RootInputHash, len([]rune(normalizedText)))
	if err != nil {
		return err
	}
	value.CoverageHash = coverageHash
	manifestHash, err := sourceManifestHash(*value)
	if err != nil {
		return err
	}
	value.ManifestHash = manifestHash
	return ValidateSourceEvidenceManifest(*value, normalizedText)
}

func sourceCoverageHash(rootHash string, codePointCount int) (string, error) {
	encoded, err := json.Marshal(struct {
		Schema         string `json:"schema"`
		RootInputHash  string `json:"root_input_hash"`
		CodePointCount int    `json:"code_point_count"`
	}{"source-evidence-coverage", rootHash, codePointCount})
	if err != nil {
		return "", err
	}
	return SourceTextHash(string(encoded)), nil
}

func sourceManifestHash(value SourceEvidenceManifest) (string, error) {
	shards := append([]SourceEvidenceShard(nil), value.Shards...)
	slices.SortFunc(shards, func(left, right SourceEvidenceShard) int {
		return strings.Compare(left.Key, right.Key)
	})
	encoded, err := json.Marshal(struct {
		Schema             string                `json:"schema"`
		Version            int64                 `json:"version"`
		ParentManifestHash *string               `json:"parent_manifest_hash"`
		WorkspaceID        string                `json:"workspace_id"`
		WorkflowRunID      string                `json:"workflow_run_id"`
		NodeRunID          string                `json:"node_run_id"`
		Stage              string                `json:"stage"`
		RootInputHash      string                `json:"root_input_hash"`
		Shards             []SourceEvidenceShard `json:"shards"`
		CoverageHash       string                `json:"coverage_hash"`
	}{
		"source-evidence-shard-manifest", value.Version, value.ParentManifestHash,
		value.WorkspaceID, value.WorkflowRunID, value.NodeRunID, value.Stage,
		value.RootInputHash, shards, value.CoverageHash,
	})
	if err != nil {
		return "", err
	}
	return SourceTextHash(string(encoded)), nil
}

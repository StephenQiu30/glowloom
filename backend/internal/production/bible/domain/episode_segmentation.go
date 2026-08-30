package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const EpisodeSegmentationStage = "segment_episodes"

type EpisodeSegmentationShard struct {
	Key           string `json:"shard_key"`
	TreePath      string `json:"tree_path"`
	Kind          string `json:"kind"`
	AbsoluteStart int    `json:"absolute_start"`
	AbsoluteEnd   int    `json:"absolute_end"`
}

type EpisodeSegmentationManifest struct {
	ManifestID    string                   `json:"manifest_id"`
	WorkspaceID   string                   `json:"workspace_id"`
	WorkflowRunID string                   `json:"workflow_run_id"`
	NodeRunID     string                   `json:"node_run_id"`
	Stage         string                   `json:"stage"`
	Version       int64                    `json:"version"`
	RootInputHash string                   `json:"root_input_hash"`
	CoverageHash  string                   `json:"coverage_hash"`
	ManifestHash  string                   `json:"manifest_hash"`
	Shard         EpisodeSegmentationShard `json:"shard"`
}

type EpisodeSegmentationManifestInput struct {
	ManifestID, WorkspaceID, WorkflowRunID, NodeRunID, RootInputHash string
	SourceCodePoints                                                 int
}

func BuildEpisodeSegmentationManifest(input EpisodeSegmentationManifestInput) (EpisodeSegmentationManifest, error) {
	value := EpisodeSegmentationManifest{
		ManifestID: input.ManifestID, WorkspaceID: input.WorkspaceID,
		WorkflowRunID: input.WorkflowRunID, NodeRunID: input.NodeRunID,
		Stage: EpisodeSegmentationStage, Version: 1, RootInputHash: input.RootInputHash,
		CoverageHash: SourceTextHash(strings.Join([]string{"episode-segmentation-coverage", input.RootInputHash, strconv.Itoa(input.SourceCodePoints)}, "\x00")),
		Shard: EpisodeSegmentationShard{
			Key: "episode-segmentation:global", TreePath: "global", Kind: "episode_segmentation",
			AbsoluteStart: 0, AbsoluteEnd: input.SourceCodePoints,
		},
	}
	hashValue := value
	hashValue.ManifestID = ""
	hashValue.ManifestHash = ""
	encoded, err := json.Marshal(hashValue)
	if err != nil {
		return EpisodeSegmentationManifest{}, err
	}
	value.ManifestHash = SourceTextHash(string(encoded))
	if err = ValidateEpisodeSegmentationManifest(value); err != nil {
		return EpisodeSegmentationManifest{}, err
	}
	return value, nil
}

func ValidateEpisodeSegmentationManifest(value EpisodeSegmentationManifest) error {
	for _, identifier := range []string{value.ManifestID, value.WorkspaceID, value.WorkflowRunID, value.NodeRunID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Episode segmentation manifest identity")
		}
	}
	if value.Stage != EpisodeSegmentationStage || value.Version != 1 ||
		!hashPattern.MatchString(value.RootInputHash) || !hashPattern.MatchString(value.CoverageHash) ||
		!hashPattern.MatchString(value.ManifestHash) || value.Shard.Key != "episode-segmentation:global" ||
		value.Shard.TreePath != "global" || value.Shard.Kind != "episode_segmentation" ||
		value.Shard.AbsoluteStart != 0 || value.Shard.AbsoluteEnd < 1 {
		return errors.New("invalid Episode segmentation manifest")
	}
	hashValue := value
	hashValue.ManifestID = ""
	hashValue.ManifestHash = ""
	encoded, err := json.Marshal(hashValue)
	if err != nil || SourceTextHash(string(encoded)) != value.ManifestHash {
		return errors.New("Episode segmentation manifest hash mismatch")
	}
	return nil
}

type EpisodeBoundary struct {
	BoundaryKey   string     `json:"boundary_key"`
	EpisodeOrder  int        `json:"episode_order"`
	Title         string     `json:"title"`
	AbsoluteStart int        `json:"absolute_start"`
	AbsoluteEnd   int        `json:"absolute_end"`
	Evidence      []Evidence `json:"evidence"`
}

type EpisodeSegmentationCandidate struct {
	Boundaries   []EpisodeBoundary `json:"boundaries"`
	ReviewIssues []ReviewIssue     `json:"review_issues"`
}

type EpisodeSegmentationMarker struct {
	EpisodeNumber int      `json:"episode_number"`
	Label         string   `json:"label"`
	Evidence      Evidence `json:"evidence"`
}

func DecodeEpisodeSegmentationCandidate(
	raw json.RawMessage,
	normalizedText string,
	allowedEvidence []Evidence,
	markers []EpisodeSegmentationMarker,
) (EpisodeSegmentationCandidate, error) {
	var value EpisodeSegmentationCandidate
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return EpisodeSegmentationCandidate{}, errors.New("candidate does not match Episode segmentation schema")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return EpisodeSegmentationCandidate{}, errors.New("Episode segmentation candidate contains multiple JSON values")
	}
	if err := ValidateEpisodeSegmentationCandidate(value, normalizedText, allowedEvidence, markers); err != nil {
		return EpisodeSegmentationCandidate{}, err
	}
	return value, nil
}

func ValidateEpisodeSegmentationEvidence(values []Evidence, normalizedText string) error {
	if len(values) == 0 {
		return errors.New("Episode segmentation requires bounded Evidence")
	}
	return validateEvidence(values, normalizedText)
}

func ValidateEpisodeSegmentationCandidate(
	value EpisodeSegmentationCandidate,
	normalizedText string,
	allowedEvidence []Evidence,
	markers []EpisodeSegmentationMarker,
) error {
	sourceLength := len([]rune(normalizedText))
	if sourceLength == 0 || len(value.Boundaries) == 0 || len(value.Boundaries) > 1000 || value.ReviewIssues == nil {
		return errors.New("Episode segmentation candidate is incomplete or exceeds limits")
	}
	if err := ValidateEpisodeSegmentationEvidence(allowedEvidence, normalizedText); err != nil {
		return err
	}
	allowed := make(map[string]struct{}, len(allowedEvidence)+len(markers))
	for _, evidence := range allowedEvidence {
		allowed[storyEvidenceKey(evidence)] = struct{}{}
	}
	markerByStart := make(map[int]EpisodeSegmentationMarker, len(markers))
	for _, marker := range markers {
		if marker.EpisodeNumber < 1 || strings.TrimSpace(marker.Label) == "" ||
			marker.Evidence.SourceStart < 0 || marker.Evidence.SourceEnd <= marker.Evidence.SourceStart ||
			marker.Evidence.EpisodeNumber == nil || *marker.Evidence.EpisodeNumber != marker.EpisodeNumber {
			return errors.New("invalid explicit Episode marker")
		}
		if _, exists := markerByStart[marker.Evidence.SourceStart]; exists {
			return errors.New("explicit Episode marker starts must be unique")
		}
		if err := validateEvidence([]Evidence{marker.Evidence}, normalizedText); err != nil {
			return err
		}
		markerByStart[marker.Evidence.SourceStart] = marker
		allowed[storyEvidenceKey(marker.Evidence)] = struct{}{}
	}
	boundaryKeys := make(map[string]struct{}, len(value.Boundaries))
	boundaryByStart := make(map[int]EpisodeBoundary, len(value.Boundaries))
	for index, boundary := range value.Boundaries {
		if !keyPattern.MatchString(boundary.BoundaryKey) || strings.TrimSpace(boundary.Title) == "" ||
			boundary.EpisodeOrder != index+1 || boundary.AbsoluteStart < 0 || boundary.AbsoluteEnd <= boundary.AbsoluteStart ||
			boundary.AbsoluteEnd > sourceLength || len(boundary.Evidence) == 0 {
			return errors.New("Episode segmentation candidate contains an invalid boundary")
		}
		if index == 0 && boundary.AbsoluteStart != 0 || index > 0 && value.Boundaries[index-1].AbsoluteEnd != boundary.AbsoluteStart {
			return errors.New("Episode segmentation boundaries must cover the source without gaps or overlaps")
		}
		if _, exists := boundaryKeys[boundary.BoundaryKey]; exists {
			return errors.New("Episode segmentation boundary keys must be unique")
		}
		boundaryKeys[boundary.BoundaryKey] = struct{}{}
		boundaryByStart[boundary.AbsoluteStart] = boundary
		for _, evidence := range boundary.Evidence {
			if evidence.SourceStart < boundary.AbsoluteStart || evidence.SourceEnd > boundary.AbsoluteEnd {
				return errors.New("Episode boundary Evidence is outside its source range")
			}
			if _, exists := allowed[storyEvidenceKey(evidence)]; !exists {
				return errors.New("Episode boundary Evidence is absent from exact upstream revisions")
			}
		}
	}
	if value.Boundaries[len(value.Boundaries)-1].AbsoluteEnd != sourceLength {
		return errors.New("Episode segmentation boundaries do not cover the immutable source")
	}
	for start, marker := range markerByStart {
		boundary, exists := boundaryByStart[start]
		if !exists || !slices.ContainsFunc(boundary.Evidence, func(value Evidence) bool {
			return storyEvidenceKey(value) == storyEvidenceKey(marker.Evidence)
		}) {
			return errors.New("Episode segmentation candidate overrode an explicit Episode marker")
		}
	}
	issueKeys := make(map[string]struct{}, len(value.ReviewIssues))
	for _, issue := range value.ReviewIssues {
		if !keyPattern.MatchString(issue.IssueKey) || strings.TrimSpace(issue.Code) == "" ||
			!oneOf(issue.Severity, "warning", "blocking") || strings.TrimSpace(issue.Scope) == "" ||
			strings.TrimSpace(issue.Summary) == "" {
			return errors.New("Episode segmentation candidate contains an invalid review issue")
		}
		if _, exists := issueKeys[issue.IssueKey]; exists {
			return errors.New("Episode segmentation review issue keys must be unique")
		}
		issueKeys[issue.IssueKey] = struct{}{}
		for _, evidence := range issue.Evidence {
			if _, exists := allowed[storyEvidenceKey(evidence)]; !exists {
				return errors.New("Episode segmentation review Evidence is absent from exact upstream revisions")
			}
		}
	}
	return nil
}

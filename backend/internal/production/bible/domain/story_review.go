package domain

import (
	"errors"

	"github.com/google/uuid"
)

const ReviewStoryGraphStage = "review_storygraph"

type StoryReviewShard struct {
	Key      string `json:"shard_key"`
	TreePath string `json:"tree_path"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
}

type StoryReviewManifest struct {
	ManifestID         string             `json:"manifest_id"`
	Version            int64              `json:"version"`
	ParentManifestHash *string            `json:"parent_manifest_hash"`
	WorkspaceID        string             `json:"workspace_id"`
	WorkflowRunID      string             `json:"workflow_run_id"`
	NodeRunID          string             `json:"node_run_id"`
	Stage              string             `json:"stage"`
	RootInputHash      string             `json:"root_input_hash"`
	Shards             []StoryReviewShard `json:"shards"`
	CoverageHash       string             `json:"coverage_hash"`
	ManifestHash       string             `json:"manifest_hash"`
}

type StoryReviewManifestInput struct {
	ManifestID, WorkspaceID, WorkflowRunID, NodeRunID string
	TargetCandidateRevisionID                         string
	TargetCandidateRevisionHash                       string
	Version                                           int64
	ParentManifestHash                                *string
}

func BuildStoryReviewManifest(input StoryReviewManifestInput) (StoryReviewManifest, error) {
	for _, identifier := range []string{
		input.ManifestID, input.WorkspaceID, input.WorkflowRunID, input.NodeRunID,
		input.TargetCandidateRevisionID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return StoryReviewManifest{}, errors.New("invalid Story review manifest identity")
		}
	}
	if input.Version < 1 || !hashPattern.MatchString(input.TargetCandidateRevisionHash) ||
		input.Version == 1 && input.ParentManifestHash != nil ||
		input.Version > 1 && (input.ParentManifestHash == nil || !hashPattern.MatchString(*input.ParentManifestHash)) {
		return StoryReviewManifest{}, errors.New("invalid Story review manifest input")
	}
	coverageHash, err := CanonicalStoryHash(struct {
		Schema, CandidateRevisionID, CandidateRevisionHash string
	}{"story-review-coverage-v1", input.TargetCandidateRevisionID, input.TargetCandidateRevisionHash})
	if err != nil {
		return StoryReviewManifest{}, err
	}
	value := StoryReviewManifest{
		ManifestID: input.ManifestID, Version: input.Version, ParentManifestHash: input.ParentManifestHash,
		WorkspaceID: input.WorkspaceID, WorkflowRunID: input.WorkflowRunID, NodeRunID: input.NodeRunID,
		Stage: ReviewStoryGraphStage, RootInputHash: input.TargetCandidateRevisionHash,
		Shards: []StoryReviewShard{
			{Key: "story-review", TreePath: "review", Kind: "story_review", Status: "active"},
			{Key: "story-repair", TreePath: "repair", Kind: "candidate_repair", Status: "active"},
		},
		CoverageHash: coverageHash,
	}
	value.ManifestHash, err = storyReviewManifestHash(value)
	if err != nil {
		return StoryReviewManifest{}, err
	}
	if err = ValidateStoryReviewManifest(value); err != nil {
		return StoryReviewManifest{}, err
	}
	return value, nil
}

func ValidateStoryReviewManifest(value StoryReviewManifest) error {
	if value.Stage != ReviewStoryGraphStage || value.Version < 1 ||
		!hashPattern.MatchString(value.RootInputHash) || !hashPattern.MatchString(value.CoverageHash) ||
		!hashPattern.MatchString(value.ManifestHash) || len(value.Shards) != 2 ||
		value.Shards[0] != (StoryReviewShard{Key: "story-review", TreePath: "review", Kind: "story_review", Status: "active"}) ||
		value.Shards[1] != (StoryReviewShard{Key: "story-repair", TreePath: "repair", Kind: "candidate_repair", Status: "active"}) {
		return errors.New("invalid Story review manifest")
	}
	for _, identifier := range []string{value.ManifestID, value.WorkspaceID, value.WorkflowRunID, value.NodeRunID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Story review manifest identity")
		}
	}
	if value.Version == 1 && value.ParentManifestHash != nil ||
		value.Version > 1 && (value.ParentManifestHash == nil || !hashPattern.MatchString(*value.ParentManifestHash)) {
		return errors.New("invalid Story review manifest lineage")
	}
	expected, err := storyReviewManifestHash(value)
	if err != nil || expected != value.ManifestHash {
		return errors.New("Story review manifest hash has drifted")
	}
	return nil
}

func storyReviewManifestHash(value StoryReviewManifest) (string, error) {
	return CanonicalStoryHash(struct {
		Schema             string             `json:"schema"`
		Version            int64              `json:"version"`
		ParentManifestHash *string            `json:"parent_manifest_hash"`
		WorkspaceID        string             `json:"workspace_id"`
		WorkflowRunID      string             `json:"workflow_run_id"`
		NodeRunID          string             `json:"node_run_id"`
		Stage              string             `json:"stage"`
		RootInputHash      string             `json:"root_input_hash"`
		Shards             []StoryReviewShard `json:"shards"`
		CoverageHash       string             `json:"coverage_hash"`
	}{
		"story-review-shard-manifest-v1", value.Version, value.ParentManifestHash, value.WorkspaceID,
		value.WorkflowRunID, value.NodeRunID, value.Stage, value.RootInputHash, value.Shards, value.CoverageHash,
	})
}

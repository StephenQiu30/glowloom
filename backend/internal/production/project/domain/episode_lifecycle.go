package domain

import "time"

const EpisodeLifecycleSetSchemaVersion = "project-episode-lifecycle-set-production"

type EpisodeLifecycleSource struct {
	WorkspaceID, ProjectID, DocumentID, VersionID string
	Revision, HeadRevision                        int64
	ContentHash, HeadHash, NormalizedText         string
	CreatedAt                                     time.Time
}

type EpisodeLifecycleSpan struct {
	TemporaryEpisodeID string `json:"temporary_episode_id"`
	Position           int    `json:"position"`
	SourceStart        int    `json:"source_start"`
	SourceEnd          int    `json:"source_end"`
	Heading            string `json:"heading"`
}

type EpisodeLifecycleEpisode struct {
	ID, WorkspaceID, ProjectID string
	Name                       string
	Position, TargetDurationMS int
	Status                     string
	Revision                   int
	CurrentScriptVersionID     *string
	CreatedAt, UpdatedAt       time.Time
}

type EpisodeLifecycleScriptVersion struct {
	ID, WorkspaceID, ProjectID, EpisodeID, DocumentRevisionID string
	VersionNo, SourceStart, SourceEnd                         int
	Content, ContentHash, Status, CreatedBy                   string
	CreatedAt, UpdatedAt                                      time.Time
}

type EpisodeLifecycleEpisodeRef struct {
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

type EpisodeLifecycleSet struct {
	SchemaVersion      string                       `json:"schema_version"`
	ID                 string                       `json:"id"`
	WorkspaceID        string                       `json:"workspace_id"`
	ProjectID          string                       `json:"project_id"`
	GateInputID        string                       `json:"gate_input_id"`
	GateInputHash      string                       `json:"gate_input_hash"`
	ReviewDecisionID   string                       `json:"review_decision_id"`
	SourceVersionID    string                       `json:"source_version_id"`
	SourceHash         string                       `json:"source_hash"`
	ProjectRevision    int                          `json:"project_revision"`
	ActiveOrderHash    string                       `json:"active_order_hash"`
	Episodes           []EpisodeLifecycleEpisodeRef `json:"episodes"`
	CollectionRootHash string                       `json:"collection_root_hash"`
	CreatedAt          time.Time                    `json:"created_at"`
}

package domain

import (
	"encoding/json"
	"time"
)

type Evidence struct {
	SourceStart   int    `json:"source_start"`
	SourceEnd     int    `json:"source_end"`
	TextHash      string `json:"text_hash"`
	ExactAnchor   string `json:"exact_anchor"`
	EpisodeNumber *int   `json:"episode_number"`
}

type ReviewIssue struct {
	IssueKey   string     `json:"issue_key"`
	Code       string     `json:"code"`
	Severity   string     `json:"severity"`
	Scope      string     `json:"scope"`
	SubjectKey *string    `json:"subject_key"`
	Summary    string     `json:"summary"`
	RepairHint *string    `json:"repair_hint"`
	Evidence   []Evidence `json:"evidence"`
}

type EntityState struct {
	StateKey       string         `json:"state_key"`
	Label          string         `json:"label"`
	StateSpec      map[string]any `json:"state_spec"`
	EpisodeNumbers []int          `json:"episode_numbers"`
	Evidence       []Evidence     `json:"evidence"`
	Ambiguities    []string       `json:"ambiguities"`
}

type Entity struct {
	EntityKey      string         `json:"entity_key"`
	Kind           string         `json:"kind"`
	CanonicalName  string         `json:"canonical_name"`
	NormalizedName string         `json:"normalized_name"`
	Aliases        []string       `json:"aliases"`
	StableSpec     map[string]any `json:"stable_spec"`
	EpisodeNumbers []int          `json:"episode_numbers"`
	Evidence       []Evidence     `json:"evidence"`
	States         []EntityState  `json:"states"`
	Ambiguities    []string       `json:"ambiguities"`
}

type WorldEntry struct {
	EntryKey       string     `json:"entry_key"`
	Category       string     `json:"category"`
	Title          string     `json:"title"`
	Facts          []string   `json:"facts"`
	Rules          []string   `json:"rules"`
	EntityKeys     []string   `json:"entity_keys"`
	EpisodeNumbers []int      `json:"episode_numbers"`
	Evidence       []Evidence `json:"evidence"`
	Ambiguities    []string   `json:"ambiguities"`
}

type Candidate struct {
	Entities     []Entity         `json:"entities"`
	WorldEntries []WorldEntry     `json:"world_entries"`
	Claims       []map[string]any `json:"claims"`
	Arcs         []map[string]any `json:"arcs"`
	ReviewIssues []ReviewIssue    `json:"review_issues"`
}

type Bible struct {
	ID, WorkspaceID, ProjectID, DocumentRevisionID, TaskID string
	Status, InputHash                                      string
	ResultHash                                             *string
	EngineVersion, ModelName, PromptVersion                string
	SchemaVersion, HarnessVersion                          string
	CheckpointStage                                        *string
	CheckpointRevision                                     int
	CheckpointUpdatedAt                                    *time.Time
	Candidate                                              Candidate
	ReviewDecisions                                        map[string]string
	Error                                                  json.RawMessage
	Revision                                               int
	ConfirmedAt                                            *time.Time
	ConfirmedBy                                            *string
	CreatedBy                                              string
	CreatedAt, UpdatedAt                                   time.Time
}

type Invocation struct {
	ID, WorkspaceID, RequestType, RequestID, Kind, Stage, ShardKey string
	WorkflowRunID, NodeRunID, ManifestID                           string
	ManifestVersion                                                int64
	InputHash, StageInstanceKey, ManifestHash, Status              string
	ExecutionPolicy, Payload                                       json.RawMessage
	Attempts, ClaimVersion                                         int
	LeaseExpiresAt                                                 *time.Time
	CreatedAt                                                      time.Time
}

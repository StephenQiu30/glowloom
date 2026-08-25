package domain

import "time"

type Block struct {
	ID, DocumentRevisionID, Kind, TextHash string
	Position, SourceStart, SourceEnd       int
	Metadata                               map[string]any
}

type Source struct {
	DocumentRevisionID, NormalizedText, NormalizedHash string
	CodepointCount                                     int
	Blocks                                             []Block
}

type Proposal struct {
	ID, PlanID, Title, StartBlockID, EndBlockID, ContentHash, Reason string
	Position, StartBlockPosition, EndBlockPosition                   int
	SourceStart, SourceEnd, EstimatedDurationMS                      int
	Confidence                                                       float64
	BoundaryEvidence                                                 map[string]any
	IsLocked                                                         bool
}

type Plan struct {
	ID, WorkspaceID, ProjectID, DocumentRevisionID, Strategy, Status string
	TargetDurationMS, TotalEstimatedDurationMS, Revision             int
	RequestedEpisodeCount                                            *int
	InputHash, EngineVersion, SchemaVersion, CreatedBy               string
	ModelName, PromptVersion, PlanningErrorCode                      *string
	ConfirmedBy                                                      *string
	ConfirmedAt                                                      *time.Time
	CreatedAt, UpdatedAt                                             time.Time
	Proposals                                                        []Proposal
	Source                                                           Source
}

type Impact struct {
	ProjectRevision, ActiveEpisodeCount, ProjectedEpisodeCount int
	ActiveOrderHash                                            string
	Allowed                                                    bool
	Blockers                                                   []Blocker
}

type Blocker struct{ Code, Summary, NextAction string }

type Episode struct {
	ID, WorkspaceID, ProjectID, Name, Status string
	Position, TargetDurationMS, Revision     int
	CurrentScriptVersionID                   *string
	CurrentTimelineVersionID                 *string
	CreatedAt, UpdatedAt                     time.Time
}

type Segment struct {
	ID, ImportCommitID, ProposalID, DocumentRevisionID string
	EpisodeID, SourceID, DraftVersionID                string
	PublishedVersionID                                 *string
	Position, SourceStart, SourceEnd                   int
	SourceHash, Content                                string
}

type ImportCommit struct {
	ID, WorkspaceID, ProjectID, PlanID, Mode, Status, InputHash string
	ExpectedProjectRevision, Revision                           int
	ExpectedActiveOrderHash                                     string
	ErrorCode                                                   *string
	CreatedBy                                                   string
	CreatedAt, UpdatedAt                                        time.Time
	Segments                                                    []Segment
}

type Dialogue struct {
	ID          string `json:"id"`
	Speaker     string `json:"speaker"`
	Text        string `json:"text"`
	SourceStart int    `json:"source_start"`
	SourceEnd   int    `json:"source_end"`
}

type NarrativeUnit struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Text        string `json:"text"`
	SourceStart int    `json:"source_start"`
	SourceEnd   int    `json:"source_end"`
}

type ProductionTask struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	Required bool   `json:"required"`
}

type Scene struct {
	ID             string           `json:"id"`
	Heading        string           `json:"heading"`
	Position       int              `json:"position"`
	SourceStart    int              `json:"source_start"`
	SourceEnd      int              `json:"source_end"`
	Dialogues      []Dialogue       `json:"dialogues"`
	NarrativeUnits []NarrativeUnit  `json:"narrative_units"`
	Tasks          []ProductionTask `json:"tasks"`
}

type Structure struct {
	ID, WorkspaceID, ProjectID, EpisodeID, ScriptVersionID string
	Status, ResultHash, CreatedBy                          string
	Revision                                               int
	ConfirmedBy                                            *string
	ConfirmedAt                                            *time.Time
	CreatedAt, UpdatedAt                                   time.Time
	Scenes                                                 []Scene
}

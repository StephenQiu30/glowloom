package domain

import (
	"encoding/json"
	"time"

	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

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
	ID              string                     `json:"id"`
	TemporaryKey    string                     `json:"temporary_key,omitempty"`
	Speaker         string                     `json:"speaker"`
	SpeakerIdentity *PlanningIdentityReference `json:"speaker_identity,omitempty"`
	Text            string                     `json:"text"`
	SourceStart     int                        `json:"source_start"`
	SourceEnd       int                        `json:"source_end"`
	Evidence        []bibledomain.Evidence     `json:"evidence,omitempty"`
}

type NarrativeUnit struct {
	ID           string                      `json:"id"`
	TemporaryKey string                      `json:"temporary_key,omitempty"`
	Kind         string                      `json:"kind"`
	Text         string                      `json:"text"`
	SourceStart  int                         `json:"source_start"`
	SourceEnd    int                         `json:"source_end"`
	Participants []PlanningIdentityReference `json:"participants,omitempty"`
	Evidence     []bibledomain.Evidence      `json:"evidence,omitempty"`
}

type PlanningIdentityReference struct {
	EntityKey            string `json:"entity_key"`
	Kind                 string `json:"kind"`
	AssetID              string `json:"asset_id"`
	AssetRevision        int    `json:"asset_revision"`
	AssetContentHash     string `json:"asset_content_hash"`
	SpecificationID      string `json:"specification_id"`
	SpecificationVersion int    `json:"specification_version"`
	SpecificationHash    string `json:"specification_hash"`
}

type PlanningStateReference struct {
	ID          string `json:"id"`
	StateKey    string `json:"state_key"`
	Revision    int    `json:"revision"`
	ContentHash string `json:"content_hash"`
}

type Occurrence struct {
	ID           string                    `json:"id"`
	TemporaryKey string                    `json:"temporary_key"`
	SceneID      string                    `json:"scene_id"`
	Summary      string                    `json:"summary"`
	SourceStart  int                       `json:"source_start"`
	SourceEnd    int                       `json:"source_end"`
	Identity     PlanningIdentityReference `json:"identity"`
	State        PlanningStateReference    `json:"state"`
	Evidence     []bibledomain.Evidence    `json:"evidence"`
}

type PlanningClaimParticipant struct {
	Role     string                    `json:"role"`
	Identity PlanningIdentityReference `json:"identity"`
}

type PlanningClaimAnchor struct {
	Role         string `json:"role"`
	Kind         string `json:"kind"`
	FragmentID   string `json:"fragment_id"`
	TemporaryKey string `json:"temporary_key"`
}

type PlanningClaim struct {
	ID           string                     `json:"id"`
	TemporaryKey string                     `json:"temporary_key"`
	ClaimType    string                     `json:"claim_type"`
	Scope        string                     `json:"scope"`
	Polarity     string                     `json:"polarity"`
	Status       string                     `json:"status"`
	Participants []PlanningClaimParticipant `json:"participants"`
	Anchors      []PlanningClaimAnchor      `json:"anchors"`
	Evidence     []bibledomain.Evidence     `json:"evidence"`
}

type ProductionTask struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	Required bool   `json:"required"`
}

type Scene struct {
	ID               string                     `json:"id"`
	TemporaryKey     string                     `json:"temporary_key,omitempty"`
	Heading          string                     `json:"heading"`
	Position         int                        `json:"position"`
	SourceStart      int                        `json:"source_start"`
	SourceEnd        int                        `json:"source_end"`
	LocationIdentity *PlanningIdentityReference `json:"location_identity,omitempty"`
	LocationState    *PlanningStateReference    `json:"location_state,omitempty"`
	Evidence         []bibledomain.Evidence     `json:"evidence,omitempty"`
	Dialogues        []Dialogue                 `json:"dialogues"`
	NarrativeUnits   []NarrativeUnit            `json:"narrative_units"`
	Occurrences      []Occurrence               `json:"occurrences,omitempty"`
	Claims           []PlanningClaim            `json:"claims,omitempty"`
	Tasks            []ProductionTask           `json:"tasks"`
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

type OutboxEvent struct {
	ID, EventType, WorkspaceID, ProjectID, AggregateKind, AggregateID string
	EventVersion, Attempts                                            int
	AggregateRevision                                                 int64
	SourceReceiptID, PayloadHash, Status                              string
	Payload                                                           json.RawMessage
	OccurredAt, CreatedAt                                             time.Time
}

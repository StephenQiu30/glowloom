package domain

import "time"

type TextIntentAudio struct {
	DialogueID       string `json:"dialogue_id"`
	Channel          string `json:"channel"`
	Text             string `json:"text"`
	SpeakerMentionID string `json:"speaker_mention_id,omitempty"`
}
type TextIntentVisual struct {
	MentionID      string `json:"mention_id"`
	EntityID       string `json:"entity_id,omitempty"`
	AssetReadiness string `json:"asset_readiness"`
}
type TextIntentBlocking struct {
	MentionID string `json:"mention_id"`
	Position  string `json:"position"`
	Facing    string `json:"facing"`
	Action    string `json:"action"`
}
type TextIntentShot struct {
	ID                 string             `json:"id"`
	Key                string             `json:"key"`
	Position           int                `json:"position"`
	Purpose            string             `json:"purpose"`
	Framing            string             `json:"framing"`
	CameraMovement     string             `json:"camera_movement"`
	Action             string             `json:"action"`
	NarrativeUnitIDs   []string           `json:"narrative_unit_ids"`
	Audio              []TextIntentAudio  `json:"audio"`
	VisualRequirements []TextIntentVisual `json:"visual_requirements"`
	DetailEvidence     []EvidenceRef      `json:"detail_evidence"`
	DurationMinMS      int                `json:"duration_min_ms"`
	DurationMaxMS      int                `json:"duration_max_ms"`
	TimingBasis        string             `json:"timing_basis"`
	ScreenDirection    string             `json:"screen_direction"`
	EntryState         string             `json:"entry_state"`
	ExitState          string             `json:"exit_state"`
	PanelCaption       string             `json:"panel_caption"`
	AssetReadiness     string             `json:"asset_readiness"`
}
type TextIntentRiskResolution struct {
	Code   string `json:"code"`
	Scope  string `json:"scope"`
	Reason string `json:"reason"`
}
type TextIntentIssue struct {
	Code     string `json:"code"`
	Scope    string `json:"scope"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
}
type TextIntentVersion struct {
	ID               string                     `json:"id"`
	RunID            string                     `json:"run_id"`
	ProposalID       string                     `json:"proposal_id"`
	DecisionID       string                     `json:"decision_id"`
	WorkspaceID      string                     `json:"workspace_id"`
	ProjectID        string                     `json:"project_id"`
	SourceRevisionID string                     `json:"source_revision_id"`
	SourceHash       string                     `json:"source_hash"`
	Revision         int                        `json:"revision"`
	ContentHash      string                     `json:"content_hash"`
	CreatedBy        string                     `json:"created_by"`
	CreatedAt        time.Time                  `json:"created_at"`
	EpisodeID        string                     `json:"episode_id"`
	StructureID      string                     `json:"structure_id"`
	SceneID          string                     `json:"scene_id"`
	WorldVersionID   string                     `json:"world_version_id"`
	DramaticIntent   string                     `json:"dramatic_intent"`
	AudienceKnows    []string                   `json:"audience_knows"`
	Withhold         []string                   `json:"withhold"`
	Blocking         []TextIntentBlocking       `json:"blocking"`
	Shots            []TextIntentShot           `json:"shots"`
	AssetReadiness   string                     `json:"asset_readiness"`
	Issues           []TextIntentIssue          `json:"issues"`
	RiskResolutions  []TextIntentRiskResolution `json:"risk_resolutions"`
	IDMapping        map[string]string          `json:"id_mapping"`
}

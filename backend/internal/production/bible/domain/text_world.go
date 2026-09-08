package domain

import "time"

type TextWorldEntity struct {
	ID            string     `json:"id"`
	Key           string     `json:"key"`
	Kind          string     `json:"kind"`
	Label         string     `json:"label"`
	MentionIDs    []string   `json:"mention_ids"`
	IdentityBasis string     `json:"identity_basis"`
	Evidence      []Evidence `json:"evidence"`
	Uncertainty   *string    `json:"uncertainty"`
}
type TextWorldRelation struct {
	SubjectID string     `json:"subject_id"`
	Predicate string     `json:"predicate"`
	TargetID  string     `json:"target_id"`
	Origin    string     `json:"origin"`
	Basis     string     `json:"basis"`
	Evidence  []Evidence `json:"evidence"`
}
type TextWorldState struct {
	EntityID   string     `json:"entity_id"`
	EpisodeID  string     `json:"episode_id"`
	SceneID    string     `json:"scene_id"`
	TimeBranch string     `json:"time_branch"`
	StoryTime  string     `json:"story_time"`
	Property   string     `json:"property"`
	Before     *string    `json:"before"`
	After      *string    `json:"after"`
	Knowledge  string     `json:"knowledge"`
	Basis      string     `json:"basis"`
	Evidence   []Evidence `json:"evidence"`
}
type TextWorldAssetNeed struct {
	EntityID    string     `json:"entity_id"`
	Description string     `json:"description"`
	Evidence    []Evidence `json:"evidence"`
}
type TextWorldRiskResolution struct {
	Code   string `json:"code"`
	Scope  string `json:"scope"`
	Reason string `json:"reason"`
}
type TextWorldIssue struct {
	Code     string `json:"code"`
	Scope    string `json:"scope"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
}
type TextWorldVersion struct {
	ID                   string                    `json:"id"`
	RunID                string                    `json:"run_id"`
	ProposalID           string                    `json:"proposal_id"`
	DecisionID           string                    `json:"decision_id"`
	WorkspaceID          string                    `json:"workspace_id"`
	ProjectID            string                    `json:"project_id"`
	SourceRevisionID     string                    `json:"source_revision_id"`
	SourceHash           string                    `json:"source_hash"`
	Revision             int                       `json:"revision"`
	ContentHash          string                    `json:"content_hash"`
	CreatedBy            string                    `json:"created_by"`
	CreatedAt            time.Time                 `json:"created_at"`
	Entities             []TextWorldEntity         `json:"entities"`
	UnresolvedMentionIDs []string                  `json:"unresolved_mention_ids"`
	Relations            []TextWorldRelation       `json:"relations"`
	StateEvents          []TextWorldState          `json:"state_events"`
	AssetNeeds           []TextWorldAssetNeed      `json:"asset_needs"`
	Issues               []TextWorldIssue          `json:"issues"`
	RiskResolutions      []TextWorldRiskResolution `json:"risk_resolutions"`
	IDMapping            map[string]string         `json:"id_mapping"`
}

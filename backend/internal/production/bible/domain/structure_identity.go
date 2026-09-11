package domain

import "time"

const (
	StructureIdentitySetSchemaVersion = "structure-identity-set-production"
	StructureIdentityCheckpointKey    = "gate_1_structure_identity"
	StructureIdentityCollectionFamily = "bible_structure_identity_set"
	StructureIdentityCommandOperation = "production_bible.confirm_structure_identity_set"
)

type StructureIdentityCandidateRef struct {
	StageKey              string `json:"stage_key"`
	ShardKey              string `json:"shard_key"`
	CandidateRevisionID   string `json:"candidate_revision_id"`
	CandidateRevisionHash string `json:"candidate_revision_hash"`
	SourceInvocationID    string `json:"source_invocation_id"`
	SourceResultHash      string `json:"source_result_hash"`
	SkillReleaseID        string `json:"skill_release_id"`
	SkillReleaseHash      string `json:"skill_release_hash"`
	StageReleaseHash      string `json:"stage_release_hash"`
	BundleContentHash     string `json:"bundle_content_hash"`
	AgentImageDigest      string `json:"agent_image_digest"`
}

type StructureIdentitySceneRef struct {
	TemporaryEpisodeID  string `json:"temporary_episode_id"`
	EpisodeID           string `json:"episode_id"`
	TemporarySpanID     string `json:"temporary_span_id"`
	TemporarySceneID    string `json:"temporary_scene_id"`
	SceneOwnerLogicalID string `json:"scene_owner_logical_id"`
	ScopeKey            string `json:"scope_key"`
	SourceStart         int    `json:"source_start"`
	SourceEnd           int    `json:"source_end"`
	EvidenceHash        string `json:"evidence_hash"`
}

type StructureIdentity struct {
	TemporaryIdentityKey string   `json:"temporary_identity_key"`
	IdentityKey          string   `json:"identity_key"`
	Kind                 string   `json:"kind"`
	Resolution           string   `json:"resolution"`
	ReuseIdentityKey     *string  `json:"reuse_identity_key,omitempty"`
	CanonicalName        string   `json:"canonical_name"`
	Aliases              []string `json:"aliases"`
}

type StructureIdentityMentionMapping struct {
	Kind             string  `json:"kind"`
	OccurrenceRole   string  `json:"occurrence_role"`
	TemporarySceneID string  `json:"temporary_scene_id"`
	SourceStart      int     `json:"source_start"`
	SourceEnd        int     `json:"source_end"`
	TextHash         string  `json:"text_hash"`
	ExactAnchor      string  `json:"exact_anchor"`
	Resolution       string  `json:"resolution"`
	IdentityKey      *string `json:"identity_key,omitempty"`
}

type StructureIdentityCoverage struct {
	SceneCount          int    `json:"scene_count"`
	IdentityCount       int    `json:"identity_count"`
	MentionCount        int    `json:"mention_count"`
	ResolvedCount       int    `json:"resolved_count"`
	UnresolvedCount     int    `json:"unresolved_count"`
	MentionUniverseHash string `json:"mention_universe_hash"`
	ScopeSetHash        string `json:"scope_set_hash"`
}

type StructureIdentitySetVersion struct {
	SchemaVersion           string                            `json:"schema_version"`
	ID                      string                            `json:"id"`
	WorkspaceID             string                            `json:"workspace_id"`
	ProjectID               string                            `json:"project_id"`
	Version                 int                               `json:"version"`
	ParentVersionID         *string                           `json:"parent_version_id,omitempty"`
	GateInputID             string                            `json:"gate_input_id"`
	GateInputHash           string                            `json:"gate_input_hash"`
	ReviewDecisionID        string                            `json:"review_decision_id"`
	ProjectEpisodeReceiptID string                            `json:"project_episode_receipt_id"`
	DocumentRevisionID      string                            `json:"document_revision_id"`
	SpanIndexID             string                            `json:"span_index_id"`
	CandidateRefs           []StructureIdentityCandidateRef   `json:"candidate_refs"`
	EpisodeRefs             []EpisodeLifecycleRef             `json:"episode_refs"`
	SceneRefs               []StructureIdentitySceneRef       `json:"scene_refs"`
	Identities              []StructureIdentity               `json:"identities"`
	MentionMappings         []StructureIdentityMentionMapping `json:"mention_mappings"`
	Coverage                StructureIdentityCoverage         `json:"coverage"`
	ContentHash             string                            `json:"content_hash"`
	CreatedBy               string                            `json:"created_by"`
	CreatedAt               time.Time                         `json:"created_at"`
}

type EpisodeLifecycleRef struct {
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

type ConfirmStructureIdentitySetResult struct {
	Version          StructureIdentitySetVersion        `json:"version"`
	Receipt          StructureIdentityCollectionReceipt `json:"receipt"`
	CommandReceiptID string                             `json:"command_receipt_id"`
	CommandOperation string                             `json:"command_operation"`
}

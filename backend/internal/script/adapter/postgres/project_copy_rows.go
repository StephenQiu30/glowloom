package postgres

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

type scriptEpisodeRow struct {
	ID                   uuid.UUID                   `gorm:"column:id"`
	OrgID                uuid.UUID                   `gorm:"column:org_id"`
	ProjectID            uuid.UUID                   `gorm:"column:project_id"`
	VersionID            uuid.UUID                   `gorm:"column:script_version_id"`
	SplitSetID           uuid.UUID                   `gorm:"column:split_set_id"`
	SeqNo                int                         `gorm:"column:seq_no"`
	Title                string                      `gorm:"column:title"`
	Start                int                         `gorm:"column:span_start"`
	End                  int                         `gorm:"column:span_end"`
	Revision             int64                       `gorm:"column:revision"`
	CurrentStructureID   *uuid.UUID                  `gorm:"column:current_structure_id"`
	ConfirmedStructureID *uuid.UUID                  `gorm:"column:confirmed_structure_id"`
	PreviousEpisodeID    *uuid.UUID                  `gorm:"column:previous_episode_id"`
	InheritStatus        domain.EpisodeInheritStatus `gorm:"column:inherit_status"`
	IsDelete             bool                        `gorm:"column:is_delete"`
}

func (scriptEpisodeRow) TableName() string { return "script.episode" }

func (r scriptEpisodeRow) episode() domain.Episode {
	return domain.Episode{
		ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, VersionID: r.VersionID,
		SplitSetID: r.SplitSetID, SeqNo: r.SeqNo, Title: r.Title, Start: r.Start,
		End: r.End, Revision: r.Revision, CurrentStructureID: r.CurrentStructureID,
		ConfirmedStructureID: r.ConfirmedStructureID, PreviousEpisodeID: r.PreviousEpisodeID,
		InheritStatus: r.InheritStatus, IsDelete: r.IsDelete,
	}
}

type scriptSceneRow struct {
	ID          uuid.UUID `gorm:"column:id"`
	OrgID       uuid.UUID `gorm:"column:org_id"`
	ProjectID   uuid.UUID `gorm:"column:project_id"`
	StructureID uuid.UUID `gorm:"column:episode_structure_id"`
	SceneKey    uuid.UUID `gorm:"column:scene_key"`
	SeqNo       int       `gorm:"column:seq_no"`
	Heading     string    `gorm:"column:heading"`
	Location    string    `gorm:"column:location_text"`
	TimeOfDay   string    `gorm:"column:time_of_day"`
	Start       int       `gorm:"column:span_start"`
	End         int       `gorm:"column:span_end"`
}

func (scriptSceneRow) TableName() string { return "script.scene" }

func (r scriptSceneRow) projectCopyScene() application.ProjectCopyScene {
	return application.ProjectCopyScene{
		ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, StructureID: r.StructureID,
		SceneKey: r.SceneKey, SeqNo: r.SeqNo, Heading: r.Heading,
		LocationText: r.Location, TimeOfDay: r.TimeOfDay, Start: r.Start, End: r.End,
	}
}

type scriptDialogueLineRow struct {
	ID                 uuid.UUID           `gorm:"column:id"`
	OrgID              uuid.UUID           `gorm:"column:org_id"`
	ProjectID          uuid.UUID           `gorm:"column:project_id"`
	SceneID            uuid.UUID           `gorm:"column:scene_id"`
	LineKey            uuid.UUID           `gorm:"column:line_key"`
	SeqNo              int                 `gorm:"column:seq_no"`
	Kind               domain.DialogueKind `gorm:"column:kind"`
	Speaker            string              `gorm:"column:speaker_text"`
	Content            string              `gorm:"column:content"`
	Emotion            string              `gorm:"column:emotion"`
	ContentHash        string              `gorm:"column:content_hash"`
	CharacterID        *uuid.UUID          `gorm:"column:character_id"`
	CharacterVersionID *uuid.UUID          `gorm:"column:character_version_id"`
	Start              int                 `gorm:"column:span_start"`
	End                int                 `gorm:"column:span_end"`
}

func (scriptDialogueLineRow) TableName() string { return "script.dialogue_line" }

func (r scriptDialogueLineRow) projectCopyDialogue() application.ProjectCopyDialogue {
	return application.ProjectCopyDialogue{
		ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, SceneID: r.SceneID,
		LineKey: r.LineKey, SeqNo: r.SeqNo, Kind: r.Kind, Speaker: r.Speaker,
		Content: r.Content, Emotion: r.Emotion, ContentHash: r.ContentHash,
		CharacterID: r.CharacterID, CharacterVersionID: r.CharacterVersionID,
		Start: r.Start, End: r.End,
	}
}

type scriptActionLineRow struct {
	ID        uuid.UUID `gorm:"column:id"`
	OrgID     uuid.UUID `gorm:"column:org_id"`
	ProjectID uuid.UUID `gorm:"column:project_id"`
	SceneID   uuid.UUID `gorm:"column:scene_id"`
	LineKey   uuid.UUID `gorm:"column:line_key"`
	SeqNo     int       `gorm:"column:seq_no"`
	Content   string    `gorm:"column:content"`
	Start     int       `gorm:"column:span_start"`
	End       int       `gorm:"column:span_end"`
}

func (scriptActionLineRow) TableName() string { return "script.action_line" }

func (r scriptActionLineRow) projectCopyAction() application.ProjectCopyAction {
	return application.ProjectCopyAction{
		ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, SceneID: r.SceneID,
		LineKey: r.LineKey, SeqNo: r.SeqNo, Content: r.Content, Start: r.Start, End: r.End,
	}
}

type scriptEpisodeStructureRow struct {
	ID         uuid.UUID  `gorm:"column:id"`
	OrgID      uuid.UUID  `gorm:"column:org_id"`
	ProjectID  uuid.UUID  `gorm:"column:project_id"`
	EpisodeID  uuid.UUID  `gorm:"column:episode_id"`
	VersionNo  int64      `gorm:"column:version_no"`
	SourceHash string     `gorm:"column:source_hash"`
	Document   scriptJSON `gorm:"column:document"`
	ActorID    uuid.UUID  `gorm:"column:actor_id"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
}

func (scriptEpisodeStructureRow) TableName() string { return "script.episode_structure" }

type scriptStructureSummaryRow struct {
	ID         uuid.UUID `gorm:"column:id"`
	EpisodeID  uuid.UUID `gorm:"column:episode_id"`
	VersionNo  int64     `gorm:"column:version_no"`
	SourceHash string    `gorm:"column:source_hash"`
	CreatedAt  time.Time `gorm:"column:created_at"`
}

func (scriptStructureSummaryRow) TableName() string { return "script.episode_structure" }

type scriptVersionSummaryRow struct {
	ID                   uuid.UUID `gorm:"column:id"`
	VersionNo            int64     `gorm:"column:version_no"`
	ContentHash          string    `gorm:"column:content_hash"`
	DocumentSHA256       string    `gorm:"column:document_sha256"`
	SourceManifestSHA256 string    `gorm:"column:source_manifest_sha256"`
	CharCount            int       `gorm:"column:char_count"`
	SourceIDs            uuidArray `gorm:"column:source_ids"`
	CreatedAt            time.Time `gorm:"column:created_at"`
}

func (scriptVersionSummaryRow) TableName() string { return "script.script_version" }

type scriptSourceSummaryRow struct {
	ID           uuid.UUID           `gorm:"column:id"`
	LineageID    uuid.UUID           `gorm:"column:source_lineage_id"`
	PreviousID   *uuid.UUID          `gorm:"column:previous_source_id"`
	Revision     int64               `gorm:"column:source_revision"`
	Kind         domain.SourceKind   `gorm:"column:source_kind"`
	Title        string              `gorm:"column:title"`
	Status       domain.SourceStatus `gorm:"column:status"`
	Origin       domain.SourceOrigin `gorm:"column:origin"`
	CharCount    int                 `gorm:"column:char_count"`
	ContentHash  string              `gorm:"column:content_hash"`
	RichSHA256   string              `gorm:"column:rich_sha256"`
	MediaAssetID *uuid.UUID          `gorm:"column:media_asset_id"`
}

func (scriptSourceSummaryRow) TableName() string { return "script.script_source" }

type scriptConfirmationSummaryRow struct {
	ID             uuid.UUID  `gorm:"column:id"`
	Revision       int64      `gorm:"column:revision"`
	CandidateSetID uuid.UUID  `gorm:"column:candidate_set_id"`
	FormalSetID    uuid.UUID  `gorm:"column:formal_set_id"`
	Episodes       scriptJSON `gorm:"column:episodes"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
}

func (scriptConfirmationSummaryRow) TableName() string { return "script.split_confirmation" }

func (r scriptEpisodeStructureRow) structure() (domain.EpisodeStructure, error) {
	var document domain.StructureDocument
	if err := json.Unmarshal(r.Document, &document); err != nil {
		return domain.EpisodeStructure{}, application.ErrUnavailable
	}
	return domain.EpisodeStructure{
		ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, EpisodeID: r.EpisodeID,
		VersionNo: r.VersionNo, SourceHash: r.SourceHash, Document: document,
		ActorID: r.ActorID, CreatedAt: r.CreatedAt.UTC(),
	}, nil
}

type scriptSplitConfirmationRow struct {
	ID             uuid.UUID  `gorm:"column:id"`
	OrgID          uuid.UUID  `gorm:"column:org_id"`
	ProjectID      uuid.UUID  `gorm:"column:project_id"`
	VersionID      uuid.UUID  `gorm:"column:version_id"`
	CandidateSetID uuid.UUID  `gorm:"column:candidate_set_id"`
	FormalSetID    uuid.UUID  `gorm:"column:formal_set_id"`
	ActorID        uuid.UUID  `gorm:"column:actor_id"`
	Revision       int64      `gorm:"column:revision"`
	Preface        scriptJSON `gorm:"column:preface"`
	Episodes       scriptJSON `gorm:"column:episodes"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
}

func (scriptSplitConfirmationRow) TableName() string { return "script.split_confirmation" }

func (r scriptSplitConfirmationRow) confirmation() (domain.SplitConfirmation, error) {
	result := domain.SplitConfirmation{
		ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, VersionID: r.VersionID,
		CandidateSetID: r.CandidateSetID, FormalSetID: r.FormalSetID,
		ActorID: r.ActorID, Revision: r.Revision, CreatedAt: r.CreatedAt.UTC(),
	}
	if len(r.Preface) > 0 && string(r.Preface) != "null" {
		if err := json.Unmarshal(r.Preface, &result.Preface); err != nil {
			return domain.SplitConfirmation{}, application.ErrUnavailable
		}
	}
	if err := json.Unmarshal(r.Episodes, &result.Episodes); err != nil {
		return domain.SplitConfirmation{}, application.ErrUnavailable
	}
	return result, nil
}

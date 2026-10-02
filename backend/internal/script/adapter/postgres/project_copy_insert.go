package postgres

import (
	"encoding/json"

	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
)

func copyInsert(tx *gorm.DB, table string, row map[string]any) error {
	write := tx.Table(table).Create(row)
	if write.Error != nil {
		return write.Error
	}
	if write.RowsAffected != 1 {
		return application.ErrConflict
	}
	return nil
}
func copyJSONValue(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return gorm.Expr("?::jsonb", string(data)), nil
}
func insertCopyHistory(tx *gorm.DB, h application.ProjectCopyHistory) error {
	for _, s := range h.Sources {
		provenance, err := copyJSONValue(s.Provenance)
		if err != nil {
			return err
		}
		if err := copyInsert(tx, "script.script_source", map[string]any{"id": s.ID, "org_id": s.OrgID, "project_id": s.ProjectID, "source_lineage_id": s.LineageID, "previous_source_id": s.PreviousID, "source_revision": s.Revision, "origin": s.Origin, "source_kind": s.Kind, "title": s.Title, "status": s.Status, "rights_actor_id": s.RightsActorID, "rights_confirmed_at": s.RightsConfirmedAt, "media_asset_id": s.MediaAssetID, "media_revision": s.MediaRevision, "media_sha256": s.MediaSHA256, "original_key": s.Original.Key, "original_sha256": s.Original.SHA256, "original_bytes": s.Original.ByteSize, "original_mime": s.Original.MIME, "rich_key": s.Rich.Key, "rich_sha256": s.Rich.SHA256, "rich_bytes": s.Rich.ByteSize, "content_hash": s.ContentHash, "char_count": s.CharCount, "provenance": provenance, "created_at": s.CreatedAt}); err != nil {
			return err
		}
	}
	for _, v := range h.Versions {
		ids, err := json.Marshal(v.SourceIDs)
		if err != nil {
			return err
		}
		spans, err := copyJSONValue(v.Spans)
		if err != nil {
			return err
		}
		if err := copyInsert(tx, "script.script_version", map[string]any{"id": v.ID, "org_id": v.OrgID, "project_id": v.ProjectID, "version_no": v.VersionNo, "source_ids": gorm.Expr("ARRAY(SELECT jsonb_array_elements_text(?::jsonb)::uuid)", string(ids)), "source_spans": spans, "text_key": v.Text.Key, "text_sha256": v.Text.SHA256, "text_bytes": v.Text.ByteSize, "rich_key": v.Rich.Key, "rich_sha256": v.Rich.SHA256, "rich_bytes": v.Rich.ByteSize, "content_hash": v.ContentHash, "document_sha256": v.DocumentSHA256, "source_manifest_sha256": v.SourceManifestSHA256, "char_count": v.CharCount, "created_at": v.CreatedAt}); err != nil {
			return err
		}
	}
	for _, v := range h.VersionSources {
		if err := copyInsert(tx, "script.version_source", map[string]any{"org_id": v.OrgID, "project_id": v.ProjectID, "version_id": v.VersionID, "source_id": v.SourceID, "position": v.Position}); err != nil {
			return err
		}
	}
	for _, s := range h.SplitSets {
		preface, err := copyJSONValue(s.Preface)
		if err != nil {
			return err
		}
		boundaries, err := copyJSONValue(s.Boundaries)
		if err != nil {
			return err
		}
		warningItems := s.Warnings
		if warningItems == nil {
			warningItems = []string{}
		}
		warnings, err := copyJSONValue(warningItems)
		if err != nil {
			return err
		}
		if err := copyInsert(tx, "script.split_set", map[string]any{"id": s.ID, "org_id": s.OrgID, "project_id": s.ProjectID, "version_id": s.VersionID, "kind": s.Kind, "origin": s.Origin, "preface": preface, "boundaries": boundaries, "warnings": warnings, "created_at": s.CreatedAt}); err != nil {
			return err
		}
	}
	for _, v := range h.VersionHeads {
		if err := copyInsert(tx, "script.version_head", map[string]any{"org_id": h.State.OrgID, "project_id": h.State.ProjectID, "version_id": v.VersionID, "split_revision": v.SplitRevision, "candidate_split_set_id": v.CandidateSetID, "confirmed_split_set_id": v.ConfirmedSetID}); err != nil {
			return err
		}
	}
	for _, e := range h.Episodes {
		if err := copyInsert(tx, "script.episode", map[string]any{"id": e.ID, "org_id": e.OrgID, "project_id": e.ProjectID, "script_version_id": e.VersionID, "split_set_id": e.SplitSetID, "seq_no": e.SeqNo, "title": e.Title, "span_start": e.Start, "span_end": e.End, "revision": e.Revision, "current_structure_id": e.CurrentStructureID, "confirmed_structure_id": e.ConfirmedStructureID, "previous_episode_id": e.PreviousEpisodeID, "inherit_status": e.InheritStatus, "is_delete": e.IsDelete}); err != nil {
			return err
		}
	}
	for _, s := range h.Structures {
		doc, err := copyJSONValue(s.Document)
		if err != nil {
			return err
		}
		if err := copyInsert(tx, "script.episode_structure", map[string]any{"id": s.ID, "org_id": s.OrgID, "project_id": s.ProjectID, "episode_id": s.EpisodeID, "version_no": s.VersionNo, "source_hash": s.SourceHash, "document": doc, "actor_id": s.ActorID, "created_at": s.CreatedAt}); err != nil {
			return err
		}
	}
	for _, s := range h.Scenes {
		if err := copyInsert(tx, "script.scene", map[string]any{"id": s.ID, "org_id": s.OrgID, "project_id": s.ProjectID, "episode_structure_id": s.StructureID, "scene_key": s.SceneKey, "seq_no": s.SeqNo, "heading": s.Heading, "location_text": s.LocationText, "time_of_day": s.TimeOfDay, "span_start": s.Start, "span_end": s.End}); err != nil {
			return err
		}
	}
	for _, s := range h.Dialogue {
		if err := copyInsert(tx, "script.dialogue_line", map[string]any{"id": s.ID, "org_id": s.OrgID, "project_id": s.ProjectID, "scene_id": s.SceneID, "line_key": s.LineKey, "seq_no": s.SeqNo, "kind": s.Kind, "speaker_text": s.Speaker, "content": s.Content, "emotion": s.Emotion, "content_hash": s.ContentHash, "character_id": s.CharacterID, "span_start": s.Start, "span_end": s.End}); err != nil {
			return err
		}
	}
	for _, s := range h.Actions {
		if err := copyInsert(tx, "script.action_line", map[string]any{"id": s.ID, "org_id": s.OrgID, "project_id": s.ProjectID, "scene_id": s.SceneID, "line_key": s.LineKey, "seq_no": s.SeqNo, "content": s.Content, "span_start": s.Start, "span_end": s.End}); err != nil {
			return err
		}
	}
	for _, c := range h.SplitConfirmations {
		preface, err := copyJSONValue(c.Preface)
		if err != nil {
			return err
		}
		episodes, err := copyJSONValue(c.Episodes)
		if err != nil {
			return err
		}
		if err := copyInsert(tx, "script.split_confirmation", map[string]any{"id": c.ID, "org_id": c.OrgID, "project_id": c.ProjectID, "version_id": c.VersionID, "candidate_set_id": c.CandidateSetID, "formal_set_id": c.FormalSetID, "actor_id": c.ActorID, "revision": c.Revision, "preface": preface, "episodes": episodes, "created_at": c.CreatedAt}); err != nil {
			return err
		}
	}
	if h.State != nil {
		s := h.State
		if err := copyInsert(tx, "script.project_state", map[string]any{"project_id": s.ProjectID, "org_id": s.OrgID, "revision": s.Revision, "draft_version_id": s.DraftVersionID, "adopted_version_id": s.AdoptedVersionID, "updated_at": s.UpdatedAt}); err != nil {
			return err
		}
	}
	return nil
}

// Package postgres persists authorized script facts without reading another owner's tables.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

const sourceColumns = `id,org_id,project_id,source_lineage_id AS lineage_id,previous_source_id AS previous_id,source_revision AS revision,origin,source_kind AS kind,title,status,rights_actor_id,rights_confirmed_at,media_asset_id,media_revision,media_sha256,original_key,original_sha256,original_bytes,original_mime,rich_key,rich_sha256,rich_bytes,content_hash,char_count,provenance::text AS provenance,created_at`

type sourceRow struct {
	ID, OrgID, ProjectID, LineageID, RightsActorID                             uuid.UUID
	PreviousID, MediaAssetID                                                   *uuid.UUID
	Revision                                                                   int64
	MediaRevision                                                              *int64
	MediaSHA256                                                                *string
	Origin, Kind, Title, Status, ContentHash                                   string
	CharCount                                                                  int
	RightsConfirmedAt, CreatedAt                                               time.Time
	OriginalKey, OriginalSHA256, OriginalMime, RichKey, RichSHA256, Provenance string
	OriginalBytes, RichBytes                                                   int64
}

func (r sourceRow) record() (domain.SourceRecord, error) {
	result := domain.SourceRecord{ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, LineageID: r.LineageID, RightsActorID: r.RightsActorID, PreviousID: r.PreviousID, MediaAssetID: r.MediaAssetID, Revision: r.Revision, MediaRevision: r.MediaRevision, MediaSHA256: r.MediaSHA256, Origin: r.Origin, Kind: r.Kind, Title: r.Title, Status: r.Status, ContentHash: r.ContentHash, CharCount: r.CharCount, RightsConfirmedAt: r.RightsConfirmedAt, CreatedAt: r.CreatedAt}
	result.Original = domain.ObjectFact{Key: r.OriginalKey, SHA256: r.OriginalSHA256, ByteSize: r.OriginalBytes, MIME: r.OriginalMime}
	result.Rich = domain.ObjectFact{Key: r.RichKey, SHA256: r.RichSHA256, ByteSize: r.RichBytes, MIME: "application/json"}
	if err := json.Unmarshal([]byte(r.Provenance), &result.Provenance); err != nil {
		return domain.SourceRecord{}, application.ErrUnavailable
	}
	result.CreatedAt = result.CreatedAt.UTC()
	result.RightsConfirmedAt = result.RightsConfirmedAt.UTC()
	return result, nil
}

const versionColumns = `id,org_id,project_id,version_no,to_json(source_ids)::text AS source_ids,source_spans::text AS spans,text_key,text_sha256,text_bytes,rich_key,rich_sha256,rich_bytes,content_hash,document_sha256,source_manifest_sha256,char_count,created_at`

type versionRow struct {
	ID, OrgID, ProjectID                                       uuid.UUID
	VersionNo                                                  int64
	ContentHash, DocumentSHA256, SourceManifestSHA256          string
	CharCount                                                  int
	CreatedAt                                                  time.Time
	SourceIDs, Spans, TextKey, TextSHA256, RichKey, RichSHA256 string
	TextBytes, RichBytes                                       int64
}

func (r versionRow) record() (domain.ScriptVersion, error) {
	result := domain.ScriptVersion{ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, VersionNo: r.VersionNo, ContentHash: r.ContentHash, DocumentSHA256: r.DocumentSHA256, SourceManifestSHA256: r.SourceManifestSHA256, CharCount: r.CharCount, CreatedAt: r.CreatedAt}
	if err := json.Unmarshal([]byte(r.SourceIDs), &result.SourceIDs); err != nil {
		return domain.ScriptVersion{}, application.ErrUnavailable
	}
	if err := json.Unmarshal([]byte(r.Spans), &result.Spans); err != nil {
		return domain.ScriptVersion{}, application.ErrUnavailable
	}
	result.Text = domain.ObjectFact{Key: r.TextKey, SHA256: r.TextSHA256, ByteSize: r.TextBytes, MIME: "text/plain; charset=utf-8"}
	result.Rich = domain.ObjectFact{Key: r.RichKey, SHA256: r.RichSHA256, ByteSize: r.RichBytes, MIME: "application/json"}
	result.CreatedAt = result.CreatedAt.UTC()
	return result, nil
}

func readState(tx *gorm.DB, org, project uuid.UUID, lock bool) (domain.ProjectState, error) {
	result := domain.ProjectState{ProjectID: project, OrgID: org}
	query := `SELECT project_id,org_id,revision,draft_version_id,adopted_version_id,updated_at FROM script.project_state WHERE project_id=? AND org_id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	read := tx.Raw(query, project, org).Scan(&result)
	if read.Error != nil {
		return domain.ProjectState{}, fmt.Errorf("read script head: %w", read.Error)
	}
	return result, nil
}

func readVersion(tx *gorm.DB, org, project, id uuid.UUID) (domain.ScriptVersion, error) {
	var row versionRow
	read := tx.Raw(`SELECT `+versionColumns+` FROM script.script_version WHERE org_id=? AND project_id=? AND id=?`, org, project, id).Scan(&row)
	if read.Error != nil {
		return domain.ScriptVersion{}, fmt.Errorf("read immutable script version: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return domain.ScriptVersion{}, application.ErrNotFound
	}
	return row.record()
}

func readSources(tx *gorm.DB, org, project uuid.UUID, ids []uuid.UUID) ([]domain.SourceRecord, error) {
	if len(ids) == 0 {
		return []domain.SourceRecord{}, nil
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	var rows []sourceRow
	if err := tx.Raw(`SELECT `+sourceColumns+` FROM script.script_source WHERE org_id=? AND project_id=? AND id IN(SELECT jsonb_array_elements_text(?::jsonb)::uuid)`, org, project, string(data)).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read ordered script sources: %w", err)
	}
	indexed := make(map[uuid.UUID]domain.SourceRecord, len(rows))
	for _, row := range rows {
		record, err := row.record()
		if err != nil {
			return nil, err
		}
		indexed[record.ID] = record
	}
	result := make([]domain.SourceRecord, 0, len(ids))
	for _, id := range ids {
		r, ok := indexed[id]
		if !ok {
			return nil, application.ErrUnavailable
		}
		result = append(result, r)
	}
	return result, nil
}

func insertSource(ctx context.Context, tx *gorm.DB, r domain.SourceRecord) error {
	provenance, err := json.Marshal(r.Provenance)
	if err != nil {
		return err
	}
	return exactlyOne(tx.WithContext(ctx).Exec(`INSERT INTO script.script_source(id,org_id,project_id,source_lineage_id,previous_source_id,source_revision,origin,source_kind,title,status,rights_actor_id,rights_confirmed_at,media_asset_id,media_revision,media_sha256,original_key,original_sha256,original_bytes,original_mime,rich_key,rich_sha256,rich_bytes,content_hash,char_count,provenance,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?::jsonb,?)`, r.ID, r.OrgID, r.ProjectID, r.LineageID, r.PreviousID, r.Revision, r.Origin, r.Kind, r.Title, r.Status, r.RightsActorID, r.RightsConfirmedAt, r.MediaAssetID, r.MediaRevision, r.MediaSHA256, r.Original.Key, r.Original.SHA256, r.Original.ByteSize, r.Original.MIME, r.Rich.Key, r.Rich.SHA256, r.Rich.ByteSize, r.ContentHash, r.CharCount, string(provenance), r.CreatedAt))
}

func insertVersion(tx *gorm.DB, v domain.ScriptVersion, c domain.SplitSet) error {
	ids, err := json.Marshal(v.SourceIDs)
	if err != nil {
		return err
	}
	spans, err := json.Marshal(v.Spans)
	if err != nil {
		return err
	}
	if err := exactlyOne(tx.Exec(`INSERT INTO script.script_version(id,org_id,project_id,version_no,source_ids,source_spans,text_key,text_sha256,text_bytes,rich_key,rich_sha256,rich_bytes,content_hash,document_sha256,source_manifest_sha256,char_count,created_at) VALUES(?,?,?,?,ARRAY(SELECT jsonb_array_elements_text(?::jsonb)::uuid),?::jsonb,?,?,?,?,?,?,?,?,?,?,?)`, v.ID, v.OrgID, v.ProjectID, v.VersionNo, string(ids), string(spans), v.Text.Key, v.Text.SHA256, v.Text.ByteSize, v.Rich.Key, v.Rich.SHA256, v.Rich.ByteSize, v.ContentHash, v.DocumentSHA256, v.SourceManifestSHA256, v.CharCount, v.CreatedAt)); err != nil {
		return err
	}
	for i, id := range v.SourceIDs {
		if err := exactlyOne(tx.Exec(`INSERT INTO script.version_source(org_id,project_id,version_id,source_id,position) VALUES(?,?,?,?,?)`, v.OrgID, v.ProjectID, v.ID, id, i)); err != nil {
			return err
		}
	}
	if err := insertSplitSet(tx, c); err != nil {
		return err
	}
	return exactlyOne(tx.Exec(`INSERT INTO script.version_head(org_id,project_id,version_id,candidate_split_set_id) VALUES(?,?,?,?)`, v.OrgID, v.ProjectID, v.ID, c.ID))
}

func insertSplitSet(tx *gorm.DB, s domain.SplitSet) error {
	preface, err := json.Marshal(s.Preface)
	if err != nil {
		return err
	}
	boundaries, err := json.Marshal(s.Boundaries)
	if err != nil {
		return err
	}
	warnings := s.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	warningData, err := json.Marshal(warnings)
	if err != nil {
		return err
	}
	return exactlyOne(tx.Exec(`INSERT INTO script.split_set(id,org_id,project_id,version_id,kind,origin,preface,boundaries,warnings,created_at) VALUES(?,?,?,?,?,?,?::jsonb,?::jsonb,?::jsonb,?)`, s.ID, s.OrgID, s.ProjectID, s.VersionID, s.Kind, s.Origin, string(preface), string(boundaries), string(warningData), s.CreatedAt))
}

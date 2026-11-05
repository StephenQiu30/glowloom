// Package postgres persists authorized script facts without reading another owner's tables.
package postgres

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

type uuidArray []uuid.UUID

func (a uuidArray) Value() (driver.Value, error) {
	values := make([]string, len(a))
	for i, id := range a {
		if id == uuid.Nil {
			return nil, errors.New("invalid UUID array element")
		}
		values[i] = id.String()
	}
	return "{" + strings.Join(values, ",") + "}", nil
}

func (a *uuidArray) Scan(value any) error {
	var raw string
	switch value := value.(type) {
	case []byte:
		raw = string(value)
	case string:
		raw = value
	case nil:
		*a = nil
		return nil
	default:
		return fmt.Errorf("scan UUID array from %T", value)
	}
	if len(raw) < 2 || raw[0] != '{' || raw[len(raw)-1] != '}' {
		return errors.New("invalid UUID array")
	}
	contents := raw[1 : len(raw)-1]
	if contents == "" {
		*a = uuidArray{}
		return nil
	}
	parts := strings.Split(contents, ",")
	result := make(uuidArray, len(parts))
	for i, part := range parts {
		part = strings.Trim(part, `"`)
		id, err := uuid.Parse(part)
		if err != nil {
			return errors.New("invalid UUID array")
		}
		result[i] = id
	}
	*a = result
	return nil
}

func (uuidArray) GormDataType() string { return "uuid[]" }

type sourceRow struct {
	ID                 uuid.UUID           `gorm:"column:id"`
	OrgID              uuid.UUID           `gorm:"column:org_id"`
	ProjectID          uuid.UUID           `gorm:"column:project_id"`
	LineageID          uuid.UUID           `gorm:"column:source_lineage_id"`
	RightsActorID      uuid.UUID           `gorm:"column:rights_actor_id"`
	OriginalLocationID uuid.UUID           `gorm:"column:original_location_id;not null"`
	RichLocationID     uuid.UUID           `gorm:"column:rich_location_id;not null"`
	PreviousID         *uuid.UUID          `gorm:"column:previous_source_id"`
	MediaAssetID       *uuid.UUID          `gorm:"column:media_asset_id"`
	Revision           int64               `gorm:"column:source_revision"`
	MediaRevision      *int64              `gorm:"column:media_revision"`
	Origin             domain.SourceOrigin `gorm:"column:origin"`
	Kind               domain.SourceKind   `gorm:"column:source_kind"`
	Title              string              `gorm:"column:title"`
	Status             domain.SourceStatus `gorm:"column:status"`
	ContentHash        string              `gorm:"column:content_hash"`
	CharCount          int                 `gorm:"column:char_count"`
	RightsConfirmedAt  time.Time           `gorm:"column:rights_confirmed_at"`
	CreatedAt          time.Time           `gorm:"column:created_at"`
	MediaSHA256        *string             `gorm:"column:media_sha256"`
	OriginalKey        string              `gorm:"column:original_key"`
	OriginalSHA256     string              `gorm:"column:original_sha256"`
	OriginalMIME       string              `gorm:"column:original_mime"`
	RichKey            string              `gorm:"column:rich_key"`
	RichSHA256         string              `gorm:"column:rich_sha256"`
	Provenance         scriptJSON          `gorm:"column:provenance"`
	OriginalBytes      int64               `gorm:"column:original_bytes"`
	RichBytes          int64               `gorm:"column:rich_bytes"`
}

func (sourceRow) TableName() string { return "script.script_source" }

func (r sourceRow) record() (domain.SourceRecord, error) {
	if r.OriginalLocationID == uuid.Nil || r.RichLocationID == uuid.Nil {
		return domain.SourceRecord{}, application.ErrObjectMismatch
	}
	result := domain.SourceRecord{ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, LineageID: r.LineageID, RightsActorID: r.RightsActorID, PreviousID: r.PreviousID, MediaAssetID: r.MediaAssetID, Revision: r.Revision, MediaRevision: r.MediaRevision, MediaSHA256: r.MediaSHA256, Origin: r.Origin, Kind: r.Kind, Title: r.Title, Status: r.Status, ContentHash: r.ContentHash, CharCount: r.CharCount, RightsConfirmedAt: r.RightsConfirmedAt, CreatedAt: r.CreatedAt}
	result.Original = domain.ObjectFact{LocationID: r.OriginalLocationID, Key: r.OriginalKey, SHA256: r.OriginalSHA256, ByteSize: r.OriginalBytes, MIME: r.OriginalMIME}
	result.Rich = domain.ObjectFact{LocationID: r.RichLocationID, Key: r.RichKey, SHA256: r.RichSHA256, ByteSize: r.RichBytes, MIME: "application/json"}
	if err := json.Unmarshal(r.Provenance, &result.Provenance); err != nil {
		return domain.SourceRecord{}, application.ErrUnavailable
	}
	result.CreatedAt = result.CreatedAt.UTC()
	result.RightsConfirmedAt = result.RightsConfirmedAt.UTC()
	return result, nil
}

type versionRow struct {
	ID                   uuid.UUID  `gorm:"column:id"`
	OrgID                uuid.UUID  `gorm:"column:org_id"`
	ProjectID            uuid.UUID  `gorm:"column:project_id"`
	VersionNo            int64      `gorm:"column:version_no"`
	SourceIDs            uuidArray  `gorm:"column:source_ids"`
	Spans                scriptJSON `gorm:"column:source_spans"`
	TextLocationID       uuid.UUID  `gorm:"column:text_location_id;not null"`
	RichLocationID       uuid.UUID  `gorm:"column:rich_location_id;not null"`
	TextKey              string     `gorm:"column:text_key"`
	TextSHA256           string     `gorm:"column:text_sha256"`
	TextBytes            int64      `gorm:"column:text_bytes"`
	RichKey              string     `gorm:"column:rich_key"`
	RichSHA256           string     `gorm:"column:rich_sha256"`
	RichBytes            int64      `gorm:"column:rich_bytes"`
	ContentHash          string     `gorm:"column:content_hash"`
	DocumentSHA256       string     `gorm:"column:document_sha256"`
	SourceManifestSHA256 string     `gorm:"column:source_manifest_sha256"`
	CharCount            int        `gorm:"column:char_count"`
	CreatedAt            time.Time  `gorm:"column:created_at"`
}

func (versionRow) TableName() string { return "script.script_version" }

func (r versionRow) record() (domain.ScriptVersion, error) {
	if r.TextLocationID == uuid.Nil || r.RichLocationID == uuid.Nil {
		return domain.ScriptVersion{}, application.ErrObjectMismatch
	}
	result := domain.ScriptVersion{ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, VersionNo: r.VersionNo, SourceIDs: append([]uuid.UUID(nil), r.SourceIDs...), ContentHash: r.ContentHash, DocumentSHA256: r.DocumentSHA256, SourceManifestSHA256: r.SourceManifestSHA256, CharCount: r.CharCount, CreatedAt: r.CreatedAt}
	if err := json.Unmarshal(r.Spans, &result.Spans); err != nil {
		return domain.ScriptVersion{}, application.ErrUnavailable
	}
	result.Text = domain.ObjectFact{LocationID: r.TextLocationID, Key: r.TextKey, SHA256: r.TextSHA256, ByteSize: r.TextBytes, MIME: "text/plain; charset=utf-8"}
	result.Rich = domain.ObjectFact{LocationID: r.RichLocationID, Key: r.RichKey, SHA256: r.RichSHA256, ByteSize: r.RichBytes, MIME: "application/json"}
	result.CreatedAt = result.CreatedAt.UTC()
	return result, nil
}

type scriptProjectStateRow struct {
	ProjectID        uuid.UUID  `gorm:"column:project_id;primaryKey"`
	OrgID            uuid.UUID  `gorm:"column:org_id"`
	Revision         int64      `gorm:"column:revision"`
	DraftVersionID   *uuid.UUID `gorm:"column:draft_version_id"`
	AdoptedVersionID *uuid.UUID `gorm:"column:adopted_version_id"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
}

func (scriptProjectStateRow) TableName() string { return "script.project_state" }

type scriptVersionSourceRow struct {
	OrgID     uuid.UUID `gorm:"column:org_id"`
	ProjectID uuid.UUID `gorm:"column:project_id"`
	VersionID uuid.UUID `gorm:"column:version_id"`
	SourceID  uuid.UUID `gorm:"column:source_id"`
	Position  int       `gorm:"column:position"`
}

func (scriptVersionSourceRow) TableName() string { return "script.version_source" }

type scriptVersionHeadRow struct {
	OrgID               uuid.UUID  `gorm:"column:org_id"`
	ProjectID           uuid.UUID  `gorm:"column:project_id"`
	VersionID           uuid.UUID  `gorm:"column:version_id"`
	SplitRevision       int64      `gorm:"column:split_revision"`
	CandidateSplitSetID uuid.UUID  `gorm:"column:candidate_split_set_id"`
	ConfirmedSplitSetID *uuid.UUID `gorm:"column:confirmed_split_set_id"`
}

func (scriptVersionHeadRow) TableName() string { return "script.version_head" }

type scriptSplitSetRow struct {
	ID         uuid.UUID             `gorm:"column:id;primaryKey"`
	OrgID      uuid.UUID             `gorm:"column:org_id"`
	ProjectID  uuid.UUID             `gorm:"column:project_id"`
	VersionID  uuid.UUID             `gorm:"column:version_id"`
	Kind       domain.SplitSetKind   `gorm:"column:kind"`
	Origin     domain.SplitSetOrigin `gorm:"column:origin"`
	Preface    scriptJSON            `gorm:"column:preface"`
	Boundaries scriptJSON            `gorm:"column:boundaries"`
	Warnings   scriptJSON            `gorm:"column:warnings"`
	CreatedAt  time.Time             `gorm:"column:created_at"`
}

func (scriptSplitSetRow) TableName() string { return "script.split_set" }

func (r scriptSplitSetRow) splitSet() (domain.SplitSet, error) {
	result := domain.SplitSet{
		ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, VersionID: r.VersionID,
		Kind: r.Kind, Origin: r.Origin, CreatedAt: r.CreatedAt.UTC(),
	}
	if len(r.Preface) > 0 && string(r.Preface) != "null" {
		if err := json.Unmarshal(r.Preface, &result.Preface); err != nil {
			return domain.SplitSet{}, application.ErrUnavailable
		}
	}
	if err := json.Unmarshal(r.Boundaries, &result.Boundaries); err != nil {
		return domain.SplitSet{}, application.ErrUnavailable
	}
	if err := json.Unmarshal(r.Warnings, &result.Warnings); err != nil {
		return domain.SplitSet{}, application.ErrUnavailable
	}
	return result, nil
}

func readState(tx *gorm.DB, org, project uuid.UUID, lock bool) (domain.ProjectState, error) {
	result := domain.ProjectState{ProjectID: project, OrgID: org}
	query := tx.Where("project_id = ? AND org_id = ?", project, org)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row scriptProjectStateRow
	if err := query.Take(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return result, nil
	} else if err != nil {
		return domain.ProjectState{}, fmt.Errorf("read script head: %w", err)
	}
	result.Revision, result.DraftVersionID, result.AdoptedVersionID, result.UpdatedAt = row.Revision, row.DraftVersionID, row.AdoptedVersionID, row.UpdatedAt.UTC()
	return result, nil
}

func readVersion(tx *gorm.DB, org, project, id uuid.UUID) (domain.ScriptVersion, error) {
	var row versionRow
	if err := tx.Where("org_id = ? AND project_id = ? AND id = ?", org, project, id).Take(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ScriptVersion{}, application.ErrNotFound
	} else if err != nil {
		return domain.ScriptVersion{}, fmt.Errorf("read immutable script version: %w", err)
	}
	return row.record()
}

func readSources(tx *gorm.DB, org, project uuid.UUID, ids []uuid.UUID) ([]domain.SourceRecord, error) {
	if len(ids) == 0 {
		return []domain.SourceRecord{}, nil
	}
	var rows []sourceRow
	if err := tx.Where("org_id = ? AND project_id = ? AND id IN ?", org, project, ids).Find(&rows).Error; err != nil {
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
		record, ok := indexed[id]
		if !ok {
			return nil, application.ErrUnavailable
		}
		result = append(result, record)
	}
	return result, nil
}

func insertSource(ctx context.Context, tx *gorm.DB, r domain.SourceRecord) error {
	if r.Original.LocationID == uuid.Nil || r.Rich.LocationID == uuid.Nil {
		return application.ErrObjectMismatch
	}
	provenance, err := json.Marshal(r.Provenance)
	if err != nil {
		return err
	}
	row := sourceRow{ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, LineageID: r.LineageID, RightsActorID: r.RightsActorID, OriginalLocationID: r.Original.LocationID, RichLocationID: r.Rich.LocationID, PreviousID: r.PreviousID, MediaAssetID: r.MediaAssetID, Revision: r.Revision, MediaRevision: r.MediaRevision, Origin: r.Origin, Kind: r.Kind, Title: r.Title, Status: r.Status, ContentHash: r.ContentHash, CharCount: r.CharCount, RightsConfirmedAt: r.RightsConfirmedAt, CreatedAt: r.CreatedAt, MediaSHA256: r.MediaSHA256, OriginalKey: r.Original.Key, OriginalSHA256: r.Original.SHA256, OriginalMIME: r.Original.MIME, RichKey: r.Rich.Key, RichSHA256: r.Rich.SHA256, Provenance: scriptJSON(provenance), OriginalBytes: r.Original.ByteSize, RichBytes: r.Rich.ByteSize}
	return exactlyOne(tx.WithContext(ctx).Create(&row))
}

func insertVersion(tx *gorm.DB, v domain.ScriptVersion, candidate domain.SplitSet) error {
	if v.Text.LocationID == uuid.Nil || v.Rich.LocationID == uuid.Nil {
		return application.ErrObjectMismatch
	}
	spans, err := json.Marshal(v.Spans)
	if err != nil {
		return err
	}
	row := versionRow{ID: v.ID, OrgID: v.OrgID, ProjectID: v.ProjectID, VersionNo: v.VersionNo, SourceIDs: uuidArray(v.SourceIDs), Spans: scriptJSON(spans), TextLocationID: v.Text.LocationID, RichLocationID: v.Rich.LocationID, TextKey: v.Text.Key, TextSHA256: v.Text.SHA256, TextBytes: v.Text.ByteSize, RichKey: v.Rich.Key, RichSHA256: v.Rich.SHA256, RichBytes: v.Rich.ByteSize, ContentHash: v.ContentHash, DocumentSHA256: v.DocumentSHA256, SourceManifestSHA256: v.SourceManifestSHA256, CharCount: v.CharCount, CreatedAt: v.CreatedAt}
	if err := exactlyOne(tx.Create(&row)); err != nil {
		return err
	}
	for position, sourceID := range v.SourceIDs {
		association := scriptVersionSourceRow{OrgID: v.OrgID, ProjectID: v.ProjectID, VersionID: v.ID, SourceID: sourceID, Position: position}
		if err := exactlyOne(tx.Create(&association)); err != nil {
			return err
		}
	}
	if err := insertSplitSet(tx, candidate); err != nil {
		return err
	}
	head := scriptVersionHeadRow{OrgID: v.OrgID, ProjectID: v.ProjectID, VersionID: v.ID, CandidateSplitSetID: candidate.ID}
	return exactlyOne(tx.Create(&head))
}

func insertSplitSet(tx *gorm.DB, splitSet domain.SplitSet) error {
	preface, err := json.Marshal(splitSet.Preface)
	if err != nil {
		return err
	}
	boundaries, err := json.Marshal(splitSet.Boundaries)
	if err != nil {
		return err
	}
	warnings := splitSet.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	warningData, err := json.Marshal(warnings)
	if err != nil {
		return err
	}
	row := scriptSplitSetRow{ID: splitSet.ID, OrgID: splitSet.OrgID, ProjectID: splitSet.ProjectID, VersionID: splitSet.VersionID, Kind: splitSet.Kind, Origin: splitSet.Origin, Preface: scriptJSON(preface), Boundaries: scriptJSON(boundaries), Warnings: scriptJSON(warningData), CreatedAt: splitSet.CreatedAt}
	return exactlyOne(tx.Create(&row))
}

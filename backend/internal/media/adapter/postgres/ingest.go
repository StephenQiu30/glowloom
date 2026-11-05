package postgres

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

var (
	// ErrOperationNotIngesting means the source is absent or cannot accept media.
	ErrOperationNotIngesting = errors.New("operation cannot ingest media")
	// ErrOutputConflict means a replay attempted to rewrite an existing output.
	ErrOutputConflict = errors.New("operation media output conflict")
)

// IngestRepository writes one generated media asset and its operation output
// in the same database transaction. It owns no other operation lifecycle state.
type IngestRepository struct {
	db       *gorm.DB
	original func(*gorm.DB, uuid.UUID) error
	scopeAuthority
}

// NewIngestRepository binds generated-media writes to the caller's database.
func NewIngestRepository(db *gorm.DB) *IngestRepository { return &IngestRepository{db: db} }

// FindOutput returns an existing candidate so a replay can skip re-downloading.
func (r *IngestRepository) FindOutput(ctx context.Context, operationID uuid.UUID, seq int) (application.IngestOutput, bool, error) {
	if r == nil || r.db == nil {
		return application.IngestOutput{}, false, ErrUnavailable
	}
	var out application.IngestOutput
	var found bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.authorizeOperation(tx, operationID, false); err != nil {
			return err
		}
		var err error
		out, found, err = findOutput(tx, operationID, seq)
		return err
	})
	return out, found, err
}

func findOutput(tx *gorm.DB, operationID uuid.UUID, seq int) (application.IngestOutput, bool, error) {
	var row struct {
		OutputID         uuid.UUID
		ProjectID        uuid.UUID
		ObjectLocationID *uuid.UUID
		MediaAssetID     uuid.UUID
		ObjectKey        string
		Kind             string
		MimeType         string
		ByteSize         int64
		SHA256           *string
	}
	read := tx.Table("operation.operation_output AS o").
		Select("o.id AS output_id, a.id AS media_asset_id, a.project_id, a.object_location_id, a.object_key, a.kind, a.mime_type, a.byte_size, a.sha256").
		Joins("JOIN media.media_asset AS a ON a.id = o.media_asset_id AND a.project_id = o.project_id").
		Where("o.operation_id = ? AND o.seq_no = ? AND o.kind = ? AND NOT o.is_delete AND NOT a.is_delete", operationID, seq, operationdomain.OutputKindMedia).
		Take(&row)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return application.IngestOutput{}, false, nil
	}
	if read.Error != nil {
		return application.IngestOutput{}, false, fmt.Errorf("read operation media output: %w", read.Error)
	}
	if row.SHA256 == nil {
		return application.IngestOutput{}, false, ErrOutputConflict
	}
	location, err := readRowLocation(tx, row.ObjectLocationID, row.ObjectKey)
	if err != nil {
		return application.IngestOutput{}, false, err
	}
	return application.IngestOutput{
		ProjectID: row.ProjectID, ObjectLocation: location,
		OutputID: row.OutputID.String(), MediaAssetID: row.MediaAssetID.String(),
		ObjectKey: row.ObjectKey, Kind: row.Kind, MIMEType: row.MimeType,
		ByteSize: row.ByteSize, SHA256: *row.SHA256,
	}, true, nil
}

// LoadOperation reads the persisted source and rejects non-ingesting operations.
func (r *IngestRepository) loadOperation(ctx context.Context, operationID uuid.UUID) (application.SourceOperation, error) {
	if r == nil || r.db == nil {
		return application.SourceOperation{}, ErrUnavailable
	}
	var row application.SourceOperation
	read := r.db.WithContext(ctx).Table("operation.operation AS o").
		Select("o.id, o.project_id, provider.key AS provider_key, profile.model_key, o.region, project.aspect_ratio, capability.output_type AS output_kind, o.create_time").
		Joins("JOIN workspace.project AS project ON project.id = o.project_id").
		Joins("JOIN catalog.model_profile_version AS version ON version.id = o.model_profile_version_id").
		Joins("JOIN catalog.model_profile AS profile ON profile.id = version.model_profile_id").
		Joins("JOIN catalog.capability AS capability ON capability.key = profile.capability").
		Joins("JOIN catalog.provider AS provider ON provider.id = profile.provider_id").
		Where("o.id = ? AND (o.status = ? OR ?) AND NOT o.is_delete AND NOT project.is_delete AND NOT version.is_delete AND NOT profile.is_delete AND NOT provider.is_delete", operationID, operationdomain.StatusIngesting, r.original != nil).
		Take(&row)
	if read.Error != nil && !errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return application.SourceOperation{}, fmt.Errorf("load media source operation: %w", read.Error)
	}
	if errors.Is(read.Error, gorm.ErrRecordNotFound) || row.ID == uuid.Nil || row.ProjectID == uuid.Nil ||
		row.ProviderKey == "" || row.ModelKey == "" || row.Region == "" ||
		(row.AspectRatio != "9:16" && row.AspectRatio != "16:9") ||
		(row.OutputKind != domain.KindImage && row.OutputKind != domain.KindVideo && row.OutputKind != domain.KindAudio) ||
		row.CreateTime.IsZero() {
		return application.SourceOperation{}, ErrOperationNotIngesting
	}
	return row, nil
}

// SaveOutput creates an asset and candidate in one transaction, or returns the prior candidate.
func (r *IngestRepository) SaveOutput(ctx context.Context, asset domain.MediaAsset, renditions []domain.Rendition, seq int, outputID uuid.UUID) (application.IngestOutput, error) {
	if r == nil || r.db == nil {
		return application.IngestOutput{}, ErrUnavailable
	}
	if err := asset.Validate(); err != nil || seq < 1 || seq > 8 || outputID == uuid.Nil || asset.SourceOperationID == nil {
		return application.IngestOutput{}, domain.ErrInvalidMediaAsset
	}
	for _, rendition := range renditions {
		if err := rendition.Validate(); err != nil || rendition.MediaAssetID != asset.ID || rendition.IsDelete {
			return application.IngestOutput{}, domain.ErrInvalidRendition
		}
		prefix := strings.TrimSuffix(asset.ObjectKey, path.Ext(asset.ObjectKey)) + "/"
		name := strings.TrimPrefix(rendition.ObjectKey, prefix)
		if !strings.HasPrefix(rendition.ObjectKey, prefix) || strings.Contains(name, "/") ||
			path.Ext(name) == "" || strings.TrimSuffix(name, path.Ext(name)) != string(rendition.Kind) {
			return application.IngestOutput{}, domain.ErrInvalidRendition
		}
	}
	var saved application.IngestOutput
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.authorizeOperation(tx, *asset.SourceOperationID, true); err != nil {
			return err
		}
		var row ingestOperationRecord
		read := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(map[string]any{"id": *asset.SourceOperationID, "project_id": asset.ProjectID, "is_delete": false}).Take(&row)
		if errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return ErrOperationNotIngesting
		}
		if read.Error != nil {
			return fmt.Errorf("lock media source operation: %w", read.Error)
		}
		if existing, found, err := findOutput(tx, *asset.SourceOperationID, seq); err != nil {
			return err
		} else if found {
			if existing.MediaAssetID != asset.ID.String() || existing.OutputID != outputID.String() {
				return ErrOutputConflict
			}
			saved = existing
			return nil
		}
		if err := r.requireIngestState(row.Status); err != nil {
			return err
		}
		if err := acceptIngestLocations(tx, asset, renditions); err != nil {
			return err
		}
		if err := insertIngestMedia(tx, asset, renditions); err != nil {
			return err
		}
		mediaAssetID := asset.ID
		output := operationOutputRecord{ID: outputID, ProjectID: asset.ProjectID, OperationID: *asset.SourceOperationID, SeqNo: seq, Kind: operationdomain.OutputKindMedia, MediaAssetID: &mediaAssetID, ModerationStatus: string(domain.ModerationPending)}
		if err := tx.Create(&output).Error; err != nil {
			return fmt.Errorf("insert operation media output: %w", err)
		}
		saved = application.IngestOutput{
			ProjectID: asset.ProjectID, ObjectLocation: asset.ObjectLocation,
			OutputID: outputID.String(), MediaAssetID: asset.ID.String(),
			ObjectKey: asset.ObjectKey, Kind: string(asset.Kind), MIMEType: asset.MimeType,
			ByteSize: asset.ByteSize, SHA256: *asset.SHA256,
		}
		return nil
	})
	if err != nil {
		return application.IngestOutput{}, fmt.Errorf("save generated media output: %w", err)
	}
	return saved, nil
}

// FindRenditions returns the durable preview rows attached to one media asset.
func (r *IngestRepository) findRenditions(ctx context.Context, assetID uuid.UUID) ([]domain.Rendition, error) {
	if r == nil || r.db == nil {
		return nil, ErrUnavailable
	}
	var rows []renditionRow
	if err := r.db.WithContext(ctx).Where("media_asset_id = ? AND NOT is_delete", assetID).Order("kind").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("read generated media renditions: %w", err)
	}
	result := make([]domain.Rendition, 0, len(rows))
	for i := range rows {
		if err := rows[i].loadLocation(r.db.WithContext(ctx)); err != nil {
			return nil, err
		}
		rendition := rows[i].domain()
		if err := rendition.Validate(); err != nil {
			return nil, fmt.Errorf("validate stored media rendition: %w", err)
		}
		result = append(result, rendition)
	}
	return result, nil
}

// RecordModeration updates candidate and media states atomically. Repeating the
// same decision is safe; a conflicting decision cannot rewrite a reviewed result.
func (r *IngestRepository) RecordModeration(ctx context.Context, operationID, outputID uuid.UUID, status domain.ModerationStatus, reason string) error {
	if r == nil || r.db == nil {
		return ErrUnavailable
	}
	if operationID == uuid.Nil || outputID == uuid.Nil ||
		(status != domain.ModerationPassed && status != domain.ModerationRejected) {
		return ErrOutputConflict
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.authorizeOperation(tx, operationID, true); err != nil {
			return err
		}
		var op ingestOperationRecord
		read := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND NOT is_delete", operationID).Take(&op)
		if errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return ErrOutputConflict
		}
		if read.Error != nil {
			return fmt.Errorf("lock moderation source operation: %w", read.Error)
		}
		var row struct {
			MediaAssetID     uuid.UUID
			ModerationStatus string
			ModerationReason *string
			AssetStatus      string
			AssetModeration  string
			Revision         int32
		}
		read = tx.Table("operation.operation_output AS o").
			Select("o.media_asset_id, o.moderation_status, o.moderation_reason, a.status AS asset_status, a.moderation_status AS asset_moderation, a.revision").
			Joins("JOIN media.media_asset AS a ON a.id = o.media_asset_id AND a.project_id = o.project_id").
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("o.id = ? AND o.operation_id = ? AND o.kind = ? AND NOT o.is_delete AND NOT a.is_delete", outputID, operationID, operationdomain.OutputKindMedia).
			Take(&row)
		if errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return ErrOutputConflict
		}
		if read.Error != nil {
			return fmt.Errorf("lock media moderation output: %w", read.Error)
		}
		if row.ModerationStatus == string(status) && row.AssetModeration == string(status) &&
			((row.ModerationReason == nil && reason == "") || (row.ModerationReason != nil && *row.ModerationReason == reason)) {
			return nil
		}
		if err := r.requireIngestState(op.Status); err != nil {
			return err
		}
		if row.ModerationStatus != string(domain.ModerationPending) ||
			row.AssetStatus != string(domain.StatusProcessing) || row.AssetModeration != string(domain.ModerationPending) {
			return ErrOutputConflict
		}
		next := domain.StatusReady
		if status == domain.ModerationRejected {
			next = domain.StatusRejected
		}
		now := time.Now().UTC()
		if err := updateOne(tx, "media.media_asset", map[string]any{"id": row.MediaAssetID, "status": domain.StatusProcessing, "moderation_status": domain.ModerationPending, "revision": row.Revision, "is_delete": false}, map[string]any{"status": next, "moderation_status": status, "revision": row.Revision + 1, "update_time": now}, ErrOutputConflict); err != nil {
			return fmt.Errorf("update reviewed media asset: %w", err)
		}
		var reasonValue any
		if reason != "" {
			reasonValue = reason
		}
		if err := updateOne(tx, "operation.operation_output", map[string]any{"id": outputID, "operation_id": operationID, "moderation_status": domain.ModerationPending, "is_delete": false}, map[string]any{"moderation_status": status, "moderation_reason": reasonValue, "update_time": now}, ErrOutputConflict); err != nil {
			return fmt.Errorf("update reviewed operation output: %w", err)
		}
		return nil
	})
}

// NewScopedIngestRepository binds provider-result ingestion to independent live worker authority.
func NewScopedIngestRepository(db *gorm.DB, scopes ScopeAccessFactory) *IngestRepository {
	return &IngestRepository{db: db, scopeAuthority: scopeAuthority{scopes: scopes}}
}
func (r *IngestRepository) authorizeOperation(tx *gorm.DB, id uuid.UUID, write bool) error {
	if r.original != nil {
		return r.original(tx, id)
	}
	var row struct {
		ProjectID, OrgID uuid.UUID
		ConfirmedBy      *uuid.UUID
	}
	read := tx.Table("operation.operation AS o").Select("o.project_id, o.confirmed_by, p.org_id").Joins("JOIN workspace.project AS p ON p.id = o.project_id").Where("o.id = ? AND NOT o.is_delete", id).Take(&row)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) || row.ConfirmedBy == nil {
		return ErrNotFound
	}
	if read.Error != nil {
		return read.Error
	}
	return r.requireProject(tx, identityapp.Principal{ID: *row.ConfirmedBy, OrgID: row.OrgID}, row.ProjectID, write)
}

// LoadOperation authorizes the durable confirming actor before any object I/O.
func (r *IngestRepository) LoadOperation(ctx context.Context, id uuid.UUID) (application.SourceOperation, error) {
	var out application.SourceOperation
	if r == nil || r.db == nil {
		return out, ErrUnavailable
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.authorizeOperation(tx, id, true); err != nil {
			return err
		}
		var err error
		bound := *r
		bound.db = tx
		out, err = bound.loadOperation(ctx, id)
		return err
	})
	return out, err
}

// FindRenditions authorizes the generated asset's actual operation before returning private keys.
func (r *IngestRepository) FindRenditions(ctx context.Context, id uuid.UUID) ([]domain.Rendition, error) {
	var out []domain.Rendition
	if r == nil || r.db == nil {
		return nil, ErrUnavailable
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row assetRow
		q := tx.Select("source_operation_id").Where("id = ? AND NOT is_delete", id).Take(&row)
		if errors.Is(q.Error, gorm.ErrRecordNotFound) || row.SourceOperationID == nil {
			return ErrNotFound
		}
		if q.Error != nil {
			return q.Error
		}
		if err := r.authorizeOperation(tx, *row.SourceOperationID, false); err != nil {
			return err
		}
		var err error
		out, err = NewScopedIngestRepository(tx, r.scopes).findRenditions(ctx, id)
		return err
	})
	return out, err
}

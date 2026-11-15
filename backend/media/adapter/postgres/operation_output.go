package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// OutputPublicationStore owns Media-only durable archive and review receipts.
// Source verification runs before a transaction; Guard and publication share one transaction.
type OutputPublicationStore struct {
	db     *gorm.DB
	source application.OutputPublicationVerifier
}

// NewOutputPublicationStore requires a proof reader for the Execution original command.
func NewOutputPublicationStore(db *gorm.DB, source application.OutputPublicationVerifier) *OutputPublicationStore {
	return &OutputPublicationStore{db: db, source: source}
}

var _ application.OutputPublicationRepository = (*OutputPublicationStore)(nil)

type outputPublicationRow struct {
	CommandID     uuid.UUID `gorm:"primaryKey"`
	OperationID   uuid.UUID
	SeqNo         int
	RequestSHA256 string          `gorm:"column:request_sha256"`
	PayloadSHA256 string          `gorm:"column:payload_sha256"`
	Payload       json.RawMessage `gorm:"type:jsonb"`
	AssetID       uuid.UUID
	State         string
	Receipt       json.RawMessage `gorm:"type:jsonb"`
	CreateTime    time.Time       `gorm:"->"`
}

func (outputPublicationRow) TableName() string { return "media.operation_output_receipt" }

func (s *OutputPublicationStore) ready(ctx context.Context) error {
	if s == nil || s.db == nil || s.db.Statement == nil {
		return application.ErrUnavailable
	}
	if _, nested := s.db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return writefence.ErrTransaction
	}
	owner, err := pgfence.New(s.db).Owner(ctx)
	if err != nil {
		return err
	}
	if owner != writefence.WriterMedia {
		return domain.ErrOutputPublicationOwner
	}
	return nil
}

func matchOutputPublication(row outputPublicationRow, req application.PublishOperationOutputRequest) error {
	if row.CommandID != req.Command.ID || row.OperationID != req.Output.OperationID || row.SeqNo != req.Output.Sequence || row.RequestSHA256 != req.Command.SHA256 || row.PayloadSHA256 != req.Output.Digest() || row.AssetID == uuid.Nil {
		return domain.ErrOutputPublicationConflict
	}
	var original application.PublishOperationOutputRequest
	if json.Unmarshal(row.Payload, &original) != nil {
		return domain.ErrOutputPublicationConflict
	}
	originalRaw, err := writefence.CanonicalReceipt(mustOutputJSON(original))
	if err != nil {
		return err
	}
	incoming, err := writefence.CanonicalReceipt(mustOutputJSON(req))
	if err != nil {
		return err
	}
	if !bytes.Equal(originalRaw, incoming) {
		return domain.ErrOutputPublicationConflict
	}
	return nil
}
func mustOutputJSON(value any) []byte { raw, _ := json.Marshal(value); return raw }
func outputReceipt(row outputPublicationRow) (domain.OperationOutputReceipt, error) {
	var receipt domain.OperationOutputReceipt
	if row.State == "prepared" {
		return receipt, domain.ErrOutputPublicationUnknown
	}
	if json.Unmarshal(row.Receipt, &receipt) != nil || domain.ValidateOperationOutputReceipt(receipt) != nil || receipt.CommandID != row.CommandID || receipt.RequestSHA256 != row.RequestSHA256 || receipt.PayloadSHA256 != row.PayloadSHA256 || receipt.AssetID != row.AssetID || receipt.State != row.State {
		return receipt, domain.ErrOutputPublicationConflict
	}
	return receipt, nil
}

// ResolveOutput never consults the current worker or current policy. It recovers
// the exact original ACK only, with no new business effects or deletion permit.
func (s *OutputPublicationStore) ResolveOutput(ctx context.Context, req application.PublishOperationOutputRequest) (domain.OperationOutputReceipt, error) {
	if err := application.ValidateOutputPublication(req); err != nil {
		return domain.OperationOutputReceipt{}, err
	}
	if err := s.ready(ctx); err != nil {
		return domain.OperationOutputReceipt{}, err
	}
	var row outputPublicationRow
	err := s.db.WithContext(ctx).Where("command_id = ?", req.Command.ID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.OperationOutputReceipt{}, domain.ErrOutputPublicationUnknown
	}
	if err != nil {
		return domain.OperationOutputReceipt{}, err
	}
	if err := matchOutputPublication(row, req); err != nil {
		return domain.OperationOutputReceipt{}, err
	}
	return outputReceipt(row)
}

// PrepareOutput persists responsibility before any object transfer and freezes the
// first accepted sequence identity. A changed command cannot commandeer an asset.
func (s *OutputPublicationStore) PrepareOutput(ctx context.Context, req application.PublishOperationOutputRequest) (application.OperationOutputWork, error) {
	work := application.OperationOutputWork{Request: req}
	if err := application.ValidateOutputPublication(req); err != nil {
		return work, err
	}
	if err := s.ready(ctx); err != nil {
		return work, err
	}
	if receipt, err := s.ResolveOutput(ctx, req); err == nil {
		work.AssetID = receipt.AssetID
		work.Receipt = &receipt
		return work, nil
	} else if !errors.Is(err, domain.ErrOutputPublicationUnknown) {
		return work, err
	}
	if s.source == nil {
		return work, application.ErrUnavailable
	}
	if err := s.source.VerifyOutputPublication(ctx, req); err != nil {
		return work, err
	}
	admitted, err := pgfence.New(s.db).Admit(ctx, req.Command)
	if err != nil {
		return work, err
	}
	if admitted.State == writefence.Frozen {
		return work, writefence.ErrFrozen
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		fence := pgfence.New(s.db)
		guard := fence.Guard(ctx, tx, req.Command)
		if guard != nil && !errors.Is(guard, writefence.ErrCommitted) {
			return guard
		}
		row := outputPublicationRow{CommandID: req.Command.ID, OperationID: req.Output.OperationID, SeqNo: req.Output.Sequence, RequestSHA256: req.Command.SHA256, PayloadSHA256: req.Output.Digest(), Payload: mustOutputJSON(req), AssetID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("media-asset/"+req.Output.OperationID.String()+"/"+fmt.Sprint(req.Output.Sequence))), State: "prepared"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		var saved outputPublicationRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("command_id = ?", req.Command.ID).Take(&saved).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrOutputPublicationConflict
			}
			return err
		}
		if err := matchOutputPublication(saved, req); err != nil {
			return err
		}
		work.AssetID = saved.AssetID
		work.SourceVerified = true
		work.PreparedAt = saved.CreateTime
		if saved.State != "prepared" {
			receipt, err := outputReceipt(saved)
			if err != nil {
				return err
			}
			work.Receipt = &receipt
		}
		return nil
	})
	return work, err
}

// CommitOutput performs only Media-local writes after physical archive and review.
// No operation row, paid usage or foreign owner table is queried or modified.
func (s *OutputPublicationStore) CommitOutput(ctx context.Context, work application.OperationOutputWork, asset domain.MediaAsset, renditions []domain.Rendition, review application.OutputReview) (domain.OperationOutputReceipt, error) {
	var receipt domain.OperationOutputReceipt
	req := work.Request
	if err := application.ValidateOutputPublication(req); err != nil {
		return receipt, err
	}
	if err := s.ready(ctx); err != nil {
		return receipt, err
	}
	if asset.Validate() != nil || asset.ID != work.AssetID || asset.ProjectID != req.Output.ProjectID || asset.SourceOperationID == nil || *asset.SourceOperationID != req.Output.OperationID || asset.Origin != domain.OriginGenerated || asset.Status != domain.StatusProcessing || asset.ModerationStatus != domain.ModerationPending || asset.Revision != 1 || asset.ObjectLocation == nil || asset.ByteSize != req.Output.ByteSize || asset.MimeType != req.Output.MIMEType || asset.SHA256 == nil || *asset.SHA256 != req.Output.SHA256 || string(asset.Kind) != req.Output.Kind || asset.ProviderKey == nil || *asset.ProviderKey != req.Output.ProviderKey || asset.ModelKey == nil || *asset.ModelKey != req.Output.ModelKey || asset.Region == nil || *asset.Region != req.Output.Region || !application.CompleteOutputRenditions(asset.Kind, renditions) {
		return receipt, domain.ErrOutputPublicationConflict
	}
	for _, r := range renditions {
		if r.Validate() != nil || r.MediaAssetID != asset.ID || r.ObjectLocation == nil {
			return receipt, domain.ErrOutputPublicationConflict
		}
	}
	if (review.Status != "passed" && review.Status != "skipped" && review.Status != "rejected") || len(review.Detail) == 0 || len(review.Detail) > 16384 || !json.Valid(review.Detail) {
		return receipt, domain.ErrOutputPublicationConflict
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		fences := pgfence.New(s.db)
		guard := fences.Guard(ctx, tx, req.Command)
		if guard != nil && !errors.Is(guard, writefence.ErrCommitted) {
			return guard
		}
		var row outputPublicationRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("command_id = ?", req.Command.ID).Take(&row).Error; err != nil {
			return err
		}
		if err := matchOutputPublication(row, req); err != nil {
			return err
		}
		if row.State != "prepared" {
			var err error
			receipt, err = outputReceipt(row)
			return err
		}
		if errors.Is(guard, writefence.ErrCommitted) {
			return domain.ErrOutputPublicationConflict
		}
		if err := acceptIngestLocations(tx, asset, renditions); err != nil {
			return err
		}
		if err := insertIngestMedia(tx, asset, renditions); err != nil {
			return err
		}
		state, assetStatus := "published", "ready"
		if review.Status == "rejected" {
			state, assetStatus = "rejected", "rejected"
		}
		updated := tx.Table("media.media_asset").Where("id = ? AND revision = 1 AND status = 'processing' AND moderation_status = 'pending' AND NOT is_delete", asset.ID).Updates(map[string]any{"status": assetStatus, "moderation_status": review.Status, "moderation_detail": string(review.Detail), "revision": 2, "update_time": gorm.Expr("CURRENT_TIMESTAMP")})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return domain.ErrOutputPublicationConflict
		}
		receipt = domain.OperationOutputReceipt{CommandID: req.Command.ID, RequestSHA256: req.Command.SHA256, PayloadSHA256: req.Output.Digest(), Output: req.Output, AssetID: asset.ID, AssetRevision: 2, ModerationStatus: review.Status, ModerationDetail: review.Detail, State: state}
		if err := domain.ValidateOperationOutputReceipt(receipt); err != nil {
			return err
		}
		raw, err := writefence.CanonicalReceipt(mustOutputJSON(receipt))
		if err != nil {
			return err
		}
		updated = tx.Model(&outputPublicationRow{}).Where("command_id = ? AND state = 'prepared'", req.Command.ID).Updates(map[string]any{"state": state, "receipt": string(raw), "update_time": gorm.Expr("CURRENT_TIMESTAMP")})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return domain.ErrOutputPublicationConflict
		}
		if err := fences.Complete(ctx, tx, req.Command, raw); err != nil {
			return err
		}
		return json.Unmarshal(raw, &receipt)
	})
	return receipt, err
}

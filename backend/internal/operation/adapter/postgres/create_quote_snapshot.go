package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
	workspacedomain "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ErrInvalidQuoteSnapshot means the prepared quote and frozen inputs disagree.
var ErrInvalidQuoteSnapshot = errors.New("invalid quote snapshot")

// QuoteItem is one prepared quote and its ordered, frozen inputs. The caller
// must validate current targets, model availability, price, and consent before
// persistence; this store does not confirm or reserve the quote.
type QuoteItem struct {
	Operation domain.Operation
	Inputs    []domain.OperationInput
}

// CreateQuoteSnapshot atomically persists one quoted operation or a quoted
// batch, with every input snapshot. No budget or provider side effect occurs.
func (s *Store) CreateQuoteSnapshot(ctx context.Context, actor identityapp.Principal, batch *domain.Batch, items []QuoteItem) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	projectID, err := validateQuoteSnapshot(batch, items)
	if err != nil {
		return err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		var project struct {
			Status   string
			IsDelete bool
		}
		result := tx.Raw(`
			SELECT status, is_delete FROM workspace.project
			WHERE id = ?::uuid AND org_id = ?::uuid FOR SHARE
		`, projectID.String(), actor.OrgID.String()).Scan(&project)
		if result.Error != nil {
			return fmt.Errorf("lock quote project: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		if project.IsDelete || project.Status != "active" {
			return workspacedomain.ErrProjectStateConflict
		}
		if batch != nil {
			result = tx.Exec(`
				INSERT INTO operation.batch
				  (id, project_id, kind, scope, status, total_count, quote_total_micros)
				VALUES (?::uuid, ?::uuid, ?, ?::jsonb, ?, ?, ?)
			`, batch.ID.String(), projectID.String(), batch.Kind, string(batch.Scope),
				string(batch.Status), batch.TotalCount, batch.QuoteTotalMicros)
			if result.Error != nil {
				return fmt.Errorf("insert quoted batch: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("insert quoted batch: wrote %d rows", result.RowsAffected)
			}
		}
		for _, item := range items {
			if err := insertQuotedOperation(tx, item.Operation); err != nil {
				return err
			}
			for _, input := range item.Inputs {
				result = tx.Exec(`
					INSERT INTO operation.operation_input
					  (id, operation_id, seq_no, role, ref_type, ref_id,
					   ref_version, text_value, media_asset_id, mask_asset_id)
					VALUES (?::uuid, ?::uuid, ?, ?, ?, ?::uuid, ?, ?, ?::uuid, ?::uuid)
				`, input.ID.String(), input.OperationID.String(), input.SeqNo,
					input.Role, input.RefType, input.RefID, input.RefVersion,
					input.TextValue, input.MediaAssetID, input.MaskAssetID)
				if result.Error != nil {
					return fmt.Errorf("insert frozen operation input: %w", result.Error)
				}
				if result.RowsAffected != 1 {
					return fmt.Errorf("insert frozen operation input: wrote %d rows", result.RowsAffected)
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("create quote snapshot transaction: %w", err)
	}
	return nil
}

func validateQuoteSnapshot(batch *domain.Batch, items []QuoteItem) (uuid.UUID, error) {
	if len(items) == 0 || len(items) > 150 || (batch == nil && len(items) != 1) {
		return uuid.Nil, ErrInvalidQuoteSnapshot
	}
	projectID := items[0].Operation.ProjectID
	var total int64
	for _, item := range items {
		op := item.Operation
		if op.Validate() != nil || op.ProjectID != projectID ||
			op.Status != domain.StatusQuoted || op.Origin == "upload" ||
			op.ConfirmedAt != nil || op.ReservationID != nil || op.SettledMicros != nil ||
			op.QuoteMicros == nil ||
			(len(item.Inputs) == 0 && op.TargetType != "agent_session") {
			return uuid.Nil, ErrInvalidQuoteSnapshot
		}
		if batch == nil && op.BatchID != nil ||
			batch != nil && (op.BatchID == nil || *op.BatchID != batch.ID) {
			return uuid.Nil, ErrInvalidQuoteSnapshot
		}
		if *op.QuoteMicros > math.MaxInt64-total {
			return uuid.Nil, ErrInvalidQuoteSnapshot
		}
		total += *op.QuoteMicros
		for _, input := range item.Inputs {
			if input.OperationID != op.ID || input.Validate() != nil {
				return uuid.Nil, ErrInvalidQuoteSnapshot
			}
		}
	}
	if batch != nil && (batch.Validate() != nil || batch.ProjectID != projectID ||
		batch.Status != domain.BatchStatusQuoted || batch.TotalCount != int32(len(items)) ||
		batch.SucceededCount != 0 || batch.FailedCount != 0 || batch.UnknownCount != 0 ||
		batch.QuoteTotalMicros != total) {
		return uuid.Nil, ErrInvalidQuoteSnapshot
	}
	return projectID, nil
}

func insertQuotedOperation(tx *gorm.DB, op domain.Operation) error {
	var source any
	if op.Source != nil {
		body, err := json.Marshal(op.Source)
		if err != nil {
			return fmt.Errorf("encode canvas quote source: %w", err)
		}
		source = string(body)
	}
	result := tx.Exec(`
		INSERT INTO operation.operation
		  (id, project_id, batch_id, target_type, target_id, target_key,
		   target_version_no, capability, mode, model_profile_version_id,
		   price_rule_version_id, params, output_count, input_hash, origin,
		   status, quote_micros, quote_detail, quote_expires_at, reused_from_id,
		   force_regenerate, region, create_time,source_context)
		VALUES (?::uuid, ?::uuid, ?::uuid, NULLIF(?, ''), ?::uuid, ?,
		        ?, ?, ?, ?::uuid, ?::uuid, ?::jsonb, ?, ?, ?,
		        ?, ?, ?::jsonb, ?, ?::uuid, ?, ?, ?,?::jsonb)
	`, op.ID.String(), op.ProjectID.String(), op.BatchID, op.TargetType,
		op.TargetID, op.TargetKey, op.TargetVersionNo, op.Capability, op.Mode,
		op.ModelProfileVersionID, op.PriceRuleVersionID, string(op.Params),
		op.OutputCount, op.InputHash, op.Origin, string(op.Status),
		op.QuoteMicros, nullableJSON(op.QuoteDetail), op.QuoteExpiresAt,
		op.ReusedFromID, op.ForceRegenerate, op.Region, op.CreateTime, source)
	if result.Error != nil {
		return fmt.Errorf("insert quoted operation: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("insert quoted operation: wrote %d rows", result.RowsAffected)
	}
	return nil
}

func nullableJSON(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}

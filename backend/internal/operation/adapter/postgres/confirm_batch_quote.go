package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/lanverse/backend/internal/billing/application"
	billingdomain "github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

// ConfirmBatchQuote confirms only the still valid, selected children. The first
// transaction commits removals independently so a failed reservation does not
// make an excluded or stale quote confirmable again.
func (s *Store) ConfirmBatchQuote(ctx context.Context, actor identityapp.Principal, input application.ConfirmBatchQuoteInput) (application.ConfirmBatchQuoteResult, error) {
	if s == nil || s.db == nil {
		return application.ConfirmBatchQuoteResult{}, ErrUnavailable
	}
	if err := input.Validate(); err != nil {
		return application.ConfirmBatchQuoteResult{}, err
	}
	key := batchConfirmationKey(actor, input)
	outcome, err := s.withConfirmationKey(ctx, actor, key, func(store *Store, lockedKey confirmationKey) (confirmationOutcome, error) {
		result, confirmErr := store.confirmBatchQuote(ctx, actor, input, &lockedKey)
		encoded, encodeErr := makeConfirmationOutcome(result, confirmErr)
		if encodeErr != nil {
			return confirmationOutcome{}, encodeErr
		}
		return encoded, confirmErr
	})
	var result application.ConfirmBatchQuoteResult
	if len(outcome.Result) > 0 {
		if decodeErr := json.Unmarshal(outcome.Result, &result); decodeErr != nil {
			return application.ConfirmBatchQuoteResult{}, fmt.Errorf("decode batch confirmation result: %w", decodeErr)
		}
	}
	return result, err
}

func (s *Store) confirmBatchQuote(ctx context.Context, actor identityapp.Principal, input application.ConfirmBatchQuoteInput, key *confirmationKey) (application.ConfirmBatchQuoteResult, error) {
	excluded := make(map[uuid.UUID]bool, len(input.ExcludeOperationIDs))
	for _, id := range input.ExcludeOperationIDs {
		excluded[id] = true
	}
	var result application.ConfirmBatchQuoteResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rows, err := lockBatchQuotes(tx, actor, input)
		if err != nil {
			return err
		}
		present := make(map[uuid.UUID]bool, len(rows))
		for _, row := range rows {
			present[row.ID] = true
		}
		for id := range excluded {
			if !present[id] {
				return application.ErrInvalidConfirmBatchQuote
			}
		}
		at := time.Now().UTC()
		for _, row := range rows {
			if row.Status == string(domain.StatusExpired) {
				reason := "quote_expired"
				if row.FailureCode != nil && *row.FailureCode != "" {
					reason = *row.FailureCode
				}
				result.Items = append(result.Items, application.BatchConfirmationItem{
					OperationID: row.ID, Status: domain.StatusExpired, Reasons: []string{reason},
				})
				continue
			}
			quoted := row.domain()
			if err := quoted.Validate(); err != nil {
				return fmt.Errorf("validate batch quote: %w", err)
			}
			if quoted.Status != domain.StatusQuoted || quoted.ReservationID != nil ||
				quoted.ConfirmedAt != nil || quoted.Origin == "upload" {
				return domain.ErrQuoteNotConfirmable
			}
			reasons := []string(nil)
			if excluded[row.ID] {
				reasons = []string{"excluded_by_user"}
			} else {
				reasons, err = batchQuoteFreshness(ctx, tx, actor, quoted, at)
				if err != nil {
					return err
				}
			}
			if len(reasons) == 0 {
				continue
			}
			if err := expireBatchQuote(tx, quoted, reasons[0], at); err != nil {
				return err
			}
			result.Items = append(result.Items, application.BatchConfirmationItem{
				OperationID: row.ID, Status: domain.StatusExpired, Reasons: reasons,
			})
		}
		return nil
	})
	if err != nil {
		return application.ConfirmBatchQuoteResult{}, fmt.Errorf("prepare batch confirmation: %w", err)
	}

	preparedItems := len(result.Items)
	var discovered []application.BatchConfirmationItem
	postBudgetStale := make(map[uuid.UUID]bool)
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rows, err := lockBatchQuotes(tx, actor, input)
		if err != nil {
			return err
		}
		at := time.Now().UTC()
		selected := make([]domain.Operation, 0, len(rows))
		for _, row := range rows {
			if row.Status != string(domain.StatusQuoted) {
				continue
			}
			quoted := row.domain()
			reasons, err := batchQuoteFreshness(ctx, tx, actor, quoted, at)
			if err != nil {
				return err
			}
			if len(reasons) > 0 {
				if err := expireBatchQuote(tx, quoted, reasons[0], at); err != nil {
					return err
				}
				discovered = append(discovered, application.BatchConfirmationItem{
					OperationID: row.ID, Status: domain.StatusExpired, Reasons: reasons,
				})
				continue
			}
			if err := checkWorkflowInputsForSubmission(tx, quoted.ID, quoted.ProjectID); err != nil {
				return err
			}
			selected = append(selected, quoted)
		}
		if len(selected) == 0 {
			write := tx.Exec(`UPDATE operation.batch SET status = 'expired', update_time = ?
				WHERE id = ?::uuid AND project_id = ?::uuid AND status = 'quoted'`,
				at, input.BatchID.String(), input.ProjectID.String())
			if write.Error != nil {
				return fmt.Errorf("expire empty batch: %w", write.Error)
			}
			if write.RowsAffected != 1 {
				return domain.ErrQuoteNotConfirmable
			}
			result.Items = append(result.Items, discovered...)
			return nil
		}
		var total int64
		var lastReserve billingapp.ReserveResult
		for _, quoted := range selected {
			if quoted.QuoteMicros == nil || *quoted.QuoteMicros < 0 ||
				total > math.MaxInt64-*quoted.QuoteMicros {
				return application.ErrInvalidConfirmBatchQuote
			}
			var model struct{ ModelKey string }
			read := tx.Raw(`SELECT m.model_key FROM catalog.model_profile_version AS v
				JOIN catalog.model_profile AS m ON m.id = v.model_profile_id
				WHERE v.id = ?::uuid AND NOT v.is_delete AND NOT m.is_delete`,
				quoted.ModelProfileVersionID.String()).Scan(&model)
			if read.Error != nil {
				return fmt.Errorf("read batch model key: %w", read.Error)
			}
			if read.RowsAffected != 1 || model.ModelKey == "" {
				return application.ErrQuoteStale
			}
			reserve, err := pgbilling.NewStore(tx).ReserveInTransaction(ctx, tx, billingapp.ReserveInput{
				ProjectID: input.ProjectID, OperationID: quoted.ID, ActorID: actor.ID,
				AmountMicros: *quoted.QuoteMicros, ModelKey: &model.ModelKey,
				Region: quoted.Region, OccurredAt: at,
			})
			if err != nil {
				return err
			}
			lastReserve = reserve
			total += *quoted.QuoteMicros
			write := tx.Exec(`UPDATE operation.operation
				SET status = 'confirmed', reservation_id = ?::uuid,
				    confirmed_at = ?, confirmed_by = ?::uuid,
				    provider_request_key = ?, workflow_id = ?, update_time = ?
				WHERE id = ?::uuid AND project_id = ?::uuid AND status = 'quoted'`,
				reserve.ReservationID.String(), at, actor.ID.String(),
				"lv-"+quoted.ID.String(), "operation/"+quoted.ID.String(), at,
				quoted.ID.String(), input.ProjectID.String())
			if write.Error != nil {
				return fmt.Errorf("confirm batch operation: %w", write.Error)
			}
			if write.RowsAffected != 1 {
				return domain.ErrQuoteNotConfirmable
			}
			result.Items = append(result.Items, application.BatchConfirmationItem{
				OperationID: quoted.ID, Status: domain.StatusConfirmed,
			})
		}
		// The budget lock may have waited past a quote or price boundary.
		at = time.Now().UTC()
		for _, quoted := range selected {
			reasons, err := batchQuoteFreshness(ctx, tx, actor, quoted, at)
			if err != nil {
				return err
			}
			if len(reasons) > 0 {
				postBudgetStale[quoted.ID] = true
				return &QuoteStaleError{Reasons: reasons}
			}
		}
		write := tx.Exec(`UPDATE operation.batch
			SET status = 'confirmed', total_count = ?, quote_total_micros = ?,
			    workflow_id = ?, update_time = ?
			WHERE id = ?::uuid AND project_id = ?::uuid AND status = 'quoted'`,
			len(selected), total, "batch/"+input.BatchID.String(), at,
			input.BatchID.String(), input.ProjectID.String())
		if write.Error != nil {
			return fmt.Errorf("confirm batch: %w", write.Error)
		}
		if write.RowsAffected != 1 {
			return domain.ErrQuoteNotConfirmable
		}
		events, err := application.BatchConfirmationEvents(actor, input.BatchID, input.ProjectID,
			int32(len(selected)), total, len(result.Items)+len(discovered), input.RequestID, at)
		if err != nil {
			return err
		}
		for _, event := range events {
			if event.Topic == "lanverse.audit.recorded.v1" {
				if _, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
					Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload,
				}); err != nil {
					return fmt.Errorf("validate batch audit: %w", err)
				}
			}
			write = tx.Exec(`INSERT INTO infra.outbox (id, topic, partition_key, payload)
				VALUES (?::uuid, ?, ?, ?::jsonb)`,
				event.ID.String(), event.Topic, event.PartitionKey, string(event.Payload))
			if write.Error != nil {
				return fmt.Errorf("insert batch event: %w", write.Error)
			}
			if write.RowsAffected != 1 {
				return errors.New("batch event was not inserted")
			}
		}
		result.Items = append(result.Items, discovered...)
		result.ConfirmedCount = int32(len(selected))
		result.QuoteTotalMicros = total
		result.AvailableMicros = lastReserve.AvailableMicros
		result.BudgetRevision = lastReserve.BudgetRevision
		if key != nil && key.requestID != uuid.Nil {
			outcome, err := makeConfirmationOutcome(result, nil)
			if err != nil {
				return err
			}
			if err := insertConfirmationOutcome(tx, *key, outcome); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		result.Items = result.Items[:preparedItems]
		if errors.Is(err, billingdomain.ErrBudgetInsufficient) || errors.Is(err, application.ErrQuoteStale) {
			for _, item := range discovered {
				postBudgetStale[item.OperationID] = true
			}
			if len(postBudgetStale) > 0 {
				persisted, persistErr := s.persistStaleBatchQuotes(ctx, actor, input, postBudgetStale)
				if persistErr != nil {
					return application.ConfirmBatchQuoteResult{}, errors.Join(
						fmt.Errorf("confirm batch quote transaction: %w", err),
						fmt.Errorf("persist newly stale batch items: %w", persistErr))
				}
				result.Items = append(result.Items, persisted...)
			}
		}
		return result, fmt.Errorf("confirm batch quote transaction: %w", err)
	}
	if result.ConfirmedCount == 0 {
		return result, application.ErrQuoteStale
	}
	return result, nil
}

// persistStaleBatchQuotes is transaction C after a failed reservation. It
// rechecks the facts so a concurrent recovery cannot be expired from an old
// observation made in transaction B.
func (s *Store) persistStaleBatchQuotes(ctx context.Context, actor identityapp.Principal,
	input application.ConfirmBatchQuoteInput, candidates map[uuid.UUID]bool,
) ([]application.BatchConfirmationItem, error) {
	var persisted []application.BatchConfirmationItem
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rows, err := lockBatchQuotes(tx, actor, input)
		if err != nil {
			return err
		}
		at := time.Now().UTC()
		remaining := 0
		for _, row := range rows {
			if row.Status != string(domain.StatusQuoted) {
				continue
			}
			if !candidates[row.ID] {
				remaining++
				continue
			}
			quoted := row.domain()
			reasons, err := batchQuoteFreshness(ctx, tx, actor, quoted, at)
			if err != nil {
				return err
			}
			if len(reasons) == 0 {
				remaining++
				continue
			}
			if err := expireBatchQuote(tx, quoted, reasons[0], at); err != nil {
				return err
			}
			persisted = append(persisted, application.BatchConfirmationItem{
				OperationID: row.ID, Status: domain.StatusExpired, Reasons: reasons,
			})
		}
		if remaining == 0 {
			write := tx.Exec(`UPDATE operation.batch SET status = 'expired', update_time = ?
				WHERE id = ?::uuid AND project_id = ?::uuid AND status = 'quoted'`,
				at, input.BatchID.String(), input.ProjectID.String())
			if write.Error != nil {
				return fmt.Errorf("expire batch after rollback: %w", write.Error)
			}
		}
		return nil
	})
	return persisted, err
}

func lockBatchQuotes(tx *gorm.DB, actor identityapp.Principal, input application.ConfirmBatchQuoteInput) ([]operationRow, error) {
	if err := requireCurrentActor(tx, actor); err != nil {
		return nil, err
	}
	var batch batchRow
	read := tx.Raw(`SELECT b.id, b.project_id, b.kind, b.scope, b.status,
		b.total_count, b.succeeded_count, b.failed_count, b.unknown_count, b.quote_total_micros
		FROM operation.batch AS b JOIN workspace.project AS p ON p.id = b.project_id
		WHERE b.id = ?::uuid AND b.project_id = ?::uuid AND p.org_id = ?::uuid
		  AND p.status = 'active' AND NOT p.is_delete AND NOT b.is_delete
		FOR UPDATE OF b FOR SHARE OF p`,
		input.BatchID.String(), input.ProjectID.String(), actor.OrgID.String()).Scan(&batch)
	if read.Error != nil {
		return nil, fmt.Errorf("lock quoted batch: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return nil, ErrNotFound
	}
	if batch.Status != string(domain.BatchStatusQuoted) {
		return nil, domain.ErrQuoteNotConfirmable
	}
	var rows []operationRow
	read = tx.Raw(`SELECT id, project_id, batch_id, target_type, target_id,
		target_key, target_version_no, capability, mode, model_profile_version_id,
		price_rule_version_id, params, output_count, input_hash, origin, status,
		quote_micros, quote_detail, quote_expires_at, failure_code, reused_from_id,
		force_regenerate, reservation_id, confirmed_at, settled_micros, region, create_time
		FROM operation.operation
		WHERE project_id = ?::uuid AND batch_id = ?::uuid AND NOT is_delete
		ORDER BY id FOR UPDATE`, input.ProjectID.String(), input.BatchID.String()).Scan(&rows)
	if read.Error != nil {
		return nil, fmt.Errorf("lock batch operations: %w", read.Error)
	}
	if len(rows) < 1 || len(rows) > 300 {
		return nil, domain.ErrQuoteNotConfirmable
	}
	return rows, nil
}

func batchQuoteFreshness(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, quoted domain.Operation, at time.Time) ([]string, error) {
	if quoted.TargetType != "free" || quoted.TargetVersionNo != nil ||
		quoted.ModelProfileVersionID == nil {
		return nil, application.ErrConfirmationTargetUnavailable
	}
	currentModel, currentPrice, err := NewStore(tx).ReadCurrentQuoteCatalog(ctx, actor, quoted, at)
	if err != nil {
		return nil, err
	}
	reuseAvailable, err := NewStore(tx).ReadReuseSourceAvailable(ctx, actor, quoted)
	if err != nil {
		return nil, err
	}
	return application.QuoteFreshnessReasons(quoted, application.QuoteCurrentFacts{
		Now: at, ModelProfileVersionID: currentModel,
		PriceRuleVersionID: currentPrice, ReuseSourceUnavailable: !reuseAvailable,
	})
}

func expireBatchQuote(tx *gorm.DB, quoted domain.Operation, reason string, at time.Time) error {
	write := tx.Exec(`UPDATE operation.operation
		SET status = 'expired', failure_code = ?, update_time = ?
		WHERE id = ?::uuid AND project_id = ?::uuid AND status = 'quoted'`,
		reason, at, quoted.ID.String(), quoted.ProjectID.String())
	if write.Error != nil {
		return fmt.Errorf("expire batch quote: %w", write.Error)
	}
	if write.RowsAffected != 1 {
		return domain.ErrQuoteNotConfirmable
	}
	return nil
}

var _ application.BatchQuoteConfirmer = (*Store)(nil)

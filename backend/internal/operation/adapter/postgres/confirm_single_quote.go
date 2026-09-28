package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/lanverse/backend/internal/billing/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

// QuoteStaleError carries the ordered reasons observed while the quote is locked.
type QuoteStaleError struct{ Reasons []string }

func (e *QuoteStaleError) Error() string { return application.ErrQuoteStale.Error() }
func (e *QuoteStaleError) Unwrap() error { return application.ErrQuoteStale }

// ConfirmSingleQuote replays the first committed outcome for one request key.
// Other target types require their version tables and are rejected until their
// owner modules provide a lockable target reader.
func (s *Store) ConfirmSingleQuote(ctx context.Context, actor identityapp.Principal, input application.ConfirmSingleQuoteInput) (application.ConfirmSingleQuoteResult, error) {
	if s == nil || s.db == nil {
		return application.ConfirmSingleQuoteResult{}, ErrUnavailable
	}
	if err := input.Validate(); err != nil {
		return application.ConfirmSingleQuoteResult{}, err
	}
	key := singleConfirmationKey(actor, input)
	outcome, err := s.withConfirmationKey(ctx, actor, key, func(store *Store, lockedKey confirmationKey) (confirmationOutcome, error) {
		result, confirmErr := store.confirmSingleQuote(ctx, actor, input, &lockedKey)
		encoded, encodeErr := makeConfirmationOutcome(result, confirmErr)
		if encodeErr != nil {
			return confirmationOutcome{}, encodeErr
		}
		return encoded, confirmErr
	})
	var result application.ConfirmSingleQuoteResult
	if len(outcome.Result) > 0 {
		if decodeErr := json.Unmarshal(outcome.Result, &result); decodeErr != nil {
			return application.ConfirmSingleQuoteResult{}, fmt.Errorf("decode single confirmation result: %w", decodeErr)
		}
	}
	return result, err
}

func (s *Store) confirmSingleQuote(ctx context.Context, actor identityapp.Principal, input application.ConfirmSingleQuoteInput, key *confirmationKey) (application.ConfirmSingleQuoteResult, error) {
	var result application.ConfirmSingleQuoteResult
	var stale error
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		var row operationRow
		read := tx.Raw(`
			SELECT o.id, o.project_id, o.batch_id, o.target_type, o.target_id,
			       o.target_key, o.target_version_no, o.capability, o.mode,
			       o.model_profile_version_id, o.price_rule_version_id, o.params,
			       o.output_count, o.input_hash, o.origin, o.status, o.quote_micros,
			       o.quote_detail, o.quote_expires_at, o.reused_from_id,
			       o.force_regenerate, o.reservation_id, o.confirmed_at,
			       o.settled_micros, o.region, o.create_time
			FROM operation.operation AS o
			JOIN workspace.project AS p ON p.id = o.project_id
			WHERE o.id = ?::uuid AND o.project_id = ?::uuid AND p.org_id = ?::uuid
			  AND p.status = 'active' AND NOT p.is_delete AND NOT o.is_delete
			FOR UPDATE OF o FOR SHARE OF p
		`, input.OperationID.String(), input.ProjectID.String(), actor.OrgID.String()).Scan(&row)
		if read.Error != nil {
			return fmt.Errorf("lock quote to confirm: %w", read.Error)
		}
		if read.RowsAffected != 1 {
			return ErrNotFound
		}
		quoted := row.domain()
		if err := quoted.Validate(); err != nil {
			return fmt.Errorf("validate locked quote: %w", err)
		}
		if quoted.Status != domain.StatusQuoted || quoted.BatchID != nil ||
			quoted.ReservationID != nil || quoted.ConfirmedAt != nil || quoted.Origin == "upload" {
			return domain.ErrQuoteNotConfirmable
		}
		if quoted.TargetType != "free" || quoted.TargetVersionNo != nil ||
			quoted.ModelProfileVersionID == nil {
			return application.ErrConfirmationTargetUnavailable
		}
		confirmedAt := time.Now().UTC()
		currentModel, currentPrice, err := NewStore(tx).ReadCurrentQuoteCatalog(ctx, actor, quoted, confirmedAt)
		if err != nil {
			return err
		}
		reuseAvailable, err := NewStore(tx).ReadReuseSourceAvailable(ctx, actor, quoted)
		if err != nil {
			return err
		}
		reasons, err := application.QuoteFreshnessReasons(quoted, application.QuoteCurrentFacts{
			Now: confirmedAt, ModelProfileVersionID: currentModel,
			PriceRuleVersionID:     currentPrice,
			ReuseSourceUnavailable: !reuseAvailable,
		})
		if err != nil {
			return err
		}
		if len(reasons) > 0 {
			write := tx.Exec(`
				UPDATE operation.operation
				SET status = 'expired', failure_code = ?, update_time = ?
				WHERE id = ?::uuid AND project_id = ?::uuid AND status = 'quoted'
			`, reasons[0], confirmedAt, quoted.ID.String(), quoted.ProjectID.String())
			if write.Error != nil {
				return fmt.Errorf("expire stale quote: %w", write.Error)
			}
			if write.RowsAffected != 1 {
				return domain.ErrQuoteNotConfirmable
			}
			stale = &QuoteStaleError{Reasons: reasons}
			return nil
		}
		if err := checkWorkflowInputsForSubmission(tx, quoted.ID, quoted.ProjectID); err != nil {
			return err
		}
		var model struct{ ModelKey string }
		read = tx.Raw(`
			SELECT m.model_key FROM catalog.model_profile_version AS v
			JOIN catalog.model_profile AS m ON m.id = v.model_profile_id
			WHERE v.id = ?::uuid AND NOT v.is_delete AND NOT m.is_delete
		`, quoted.ModelProfileVersionID.String()).Scan(&model)
		if read.Error != nil {
			return fmt.Errorf("read confirmation model key: %w", read.Error)
		}
		if read.RowsAffected != 1 || model.ModelKey == "" {
			return application.ErrQuoteStale
		}
		reserve, err := pgbilling.NewStore(tx).ReserveInTransaction(ctx, tx, billingapp.ReserveInput{
			ProjectID: quoted.ProjectID, OperationID: quoted.ID, ActorID: actor.ID,
			AmountMicros: *quoted.QuoteMicros, ModelKey: &model.ModelKey,
			Region: quoted.Region, OccurredAt: confirmedAt,
		})
		if err != nil {
			return err
		}
		// Budget contention can cross quote expiry or a future price's
		// effective instant. Recheck time-based facts after acquiring its lock.
		confirmedAt = time.Now().UTC()
		currentModel, currentPrice, err = NewStore(tx).ReadCurrentQuoteCatalog(ctx, actor, quoted, confirmedAt)
		if err != nil {
			return err
		}
		reasons, err = application.QuoteFreshnessReasons(quoted, application.QuoteCurrentFacts{
			Now: confirmedAt, ModelProfileVersionID: currentModel,
			PriceRuleVersionID:     currentPrice,
			ReuseSourceUnavailable: !reuseAvailable,
		})
		if err != nil {
			return err
		}
		if len(reasons) > 0 {
			return &QuoteStaleError{Reasons: reasons}
		}
		write := tx.Exec(`
			UPDATE operation.operation
			SET status = 'confirmed', reservation_id = ?::uuid,
			    confirmed_at = ?, confirmed_by = ?::uuid,
			    provider_request_key = ?, workflow_id = ?, update_time = ?
			WHERE id = ?::uuid AND project_id = ?::uuid AND status = 'quoted'
		`, reserve.ReservationID.String(), confirmedAt, actor.ID.String(),
			"lv-"+quoted.ID.String(), "operation/"+quoted.ID.String(), confirmedAt,
			quoted.ID.String(), quoted.ProjectID.String())
		if write.Error != nil {
			return fmt.Errorf("confirm quoted operation: %w", write.Error)
		}
		if write.RowsAffected != 1 {
			return domain.ErrQuoteNotConfirmable
		}
		events, err := application.SingleConfirmationEvents(actor, quoted, reserve.ReservationID,
			model.ModelKey, input.RequestID, confirmedAt)
		if err != nil {
			return err
		}
		for _, event := range events {
			if event.Topic == "lanverse.audit.recorded.v1" {
				if _, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
					Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload,
				}); err != nil {
					return fmt.Errorf("validate confirmation audit: %w", err)
				}
			}
			write = tx.Exec(`
				INSERT INTO infra.outbox (id, topic, partition_key, payload)
				VALUES (?::uuid, ?, ?, ?::jsonb)
			`, event.ID.String(), event.Topic, event.PartitionKey, string(event.Payload))
			if write.Error != nil {
				return fmt.Errorf("insert confirmation event: %w", write.Error)
			}
			if write.RowsAffected != 1 {
				return errors.New("confirmation event was not inserted")
			}
		}
		result = application.ConfirmSingleQuoteResult{
			ReservationID:   reserve.ReservationID,
			AvailableMicros: reserve.AvailableMicros,
			BudgetRevision:  reserve.BudgetRevision,
		}
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
		return application.ConfirmSingleQuoteResult{}, fmt.Errorf("confirm single quote transaction: %w", err)
	}
	if stale != nil {
		return application.ConfirmSingleQuoteResult{}, stale
	}
	return result, nil
}

var _ application.SingleQuoteConfirmer = (*Store)(nil)

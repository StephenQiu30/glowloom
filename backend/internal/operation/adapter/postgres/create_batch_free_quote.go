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

	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

// CreateBatchFreeQuote preserves request order, excludes only invalid items,
// and commits valid snapshots and the idempotency result in one transaction.
func (s *Store) CreateBatchFreeQuote(ctx context.Context, actor identityapp.Principal, input application.CreateBatchFreeQuoteInput) (application.CreateBatchFreeQuoteResult, error) {
	if s == nil || s.db == nil {
		return application.CreateBatchFreeQuoteResult{}, ErrUnavailable
	}
	if err := input.Validate(); err != nil {
		return application.CreateBatchFreeQuoteResult{}, err
	}
	fingerprint, err := batchQuoteRequestFingerprint(input)
	if err != nil {
		return application.CreateBatchFreeQuoteResult{}, err
	}
	requestID := uuid.MustParse(input.RequestID)
	var result application.CreateBatchFreeQuoteResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := lockQuoteRequest(tx, actor, requestID); err != nil {
			return err
		}
		project, err := lockFreeQuoteProject(tx, actor, input.ProjectID)
		if err != nil {
			return err
		}
		stored, found, err := readQuoteRequest(tx, actor, requestID, fingerprint)
		if err != nil {
			return err
		}
		if found {
			if err := json.Unmarshal(stored, &result); err != nil {
				return fmt.Errorf("decode stored batch quote: %w", err)
			}
			if len(result.Items) != len(input.Items) || result.ExpiresAt.IsZero() {
				return fmt.Errorf("decode stored batch quote: invalid result shape")
			}
			return nil
		}
		now := time.Now().UTC()
		result = application.CreateBatchFreeQuoteResult{
			ExpiresAt: now.Add(15 * time.Minute),
			Items:     make([]application.BatchFreeQuoteItemResult, len(input.Items)),
		}
		valid := make([]QuoteItem, 0, len(input.Items))
		for index, requested := range input.Items {
			result.Items[index] = application.BatchFreeQuoteItemResult{
				ModelKey: requested.ModelKey, Mode: requested.Mode,
			}
			item, modelKey, itemErr := prepareFreeQuoteItem(ctx, tx, actor, application.CreateFreeQuoteInput{
				ProjectID: input.ProjectID, RequestID: input.RequestID,
				FreeQuoteItemInput: requested,
			}, project, now)
			if itemErr != nil {
				code, recoverable := batchQuoteItemError(itemErr)
				if !recoverable {
					return fmt.Errorf("prepare batch quote item %d: %w", index, itemErr)
				}
				result.Items[index].ErrorCode = code
				continue
			}
			amount := *item.Operation.QuoteMicros
			result.Items[index].ModelKey = modelKey
			if amount > math.MaxInt64-result.TotalMicros {
				return application.ErrQuoteAmountOverflow
			}
			result.TotalMicros += amount
			operationID := item.Operation.ID
			result.Items[index].OperationID = &operationID
			result.Items[index].QuoteMicros = &amount
			result.Items[index].QuoteDetail = item.Operation.QuoteDetail
			result.Items[index].ReusedFromID = item.Operation.ReusedFromID
			result.Items[index].Region = *item.Operation.Region
			valid = append(valid, item)
		}
		budget, err := pgbilling.NewStore(tx).FindBudget(ctx, actor, input.ProjectID)
		if err != nil {
			return err
		}
		result.AvailableMicros, err = budget.AvailableMicros()
		if err != nil {
			return err
		}
		if len(valid) > 0 {
			batchID := uuid.New()
			for index := range valid {
				valid[index].Operation.BatchID = &batchID
			}
			batch := domain.Batch{
				ID: batchID, ProjectID: input.ProjectID, Kind: "mixed",
				Scope: json.RawMessage(`{"origin":"canvas"}`), Status: domain.BatchStatusQuoted,
				TotalCount: int32(len(valid)), QuoteTotalMicros: result.TotalMicros,
			}
			if err := NewStore(tx).CreateQuoteSnapshot(ctx, actor, &batch, valid); err != nil {
				return err
			}
			result.BatchID = &batchID
			result.Confirmable = result.AvailableMicros >= result.TotalMicros
		}
		return insertQuoteRequest(tx, actor, requestID, fingerprint, result)
	})
	if err != nil {
		return application.CreateBatchFreeQuoteResult{}, fmt.Errorf("create batch free quote transaction: %w", err)
	}
	return result, nil
}

func batchQuoteItemError(err error) (string, bool) {
	switch {
	case errors.Is(err, application.ErrFreeQuoteModelMissing):
		return "model_unavailable", true
	case errors.Is(err, application.ErrInvalidFreeQuote),
		errors.Is(err, application.ErrFreeQuoteInputNotReady),
		errors.Is(err, application.ErrInvalidFingerprint),
		errors.Is(err, application.ErrInvalidPricingInput):
		return "input_not_ready", true
	case errors.Is(err, application.ErrInvalidPricingRule),
		errors.Is(err, application.ErrUnsupportedPriceCoefficient),
		errors.Is(err, application.ErrQuoteAmountOverflow):
		return "price_unavailable", true
	default:
		return "", false
	}
}

var _ application.BatchFreeQuoteCreator = (*Store)(nil)

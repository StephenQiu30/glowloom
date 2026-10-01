package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

// ClaimProviderDispatch gives exactly one caller permission to send a request.
// A claimed call without a receipt stays uncertain forever; timeouts do not
// revoke the durable claim or permit a second provider request.
func (s *Store) ClaimProviderDispatch(ctx context.Context, identity application.ProviderDispatchIdentity) (application.ProviderDispatchResult, error) {
	if s == nil || s.db == nil {
		return application.ProviderDispatchResult{}, ErrUnavailable
	}
	if err := identity.Validate(); err != nil {
		return application.ProviderDispatchResult{}, err
	}
	var dispatch application.ProviderDispatchResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var op struct {
			ProjectID             uuid.UUID
			Status                string
			ProviderRequestKey    *string
			ModelProfileVersionID *uuid.UUID
			PriceRuleVersionID    *uuid.UUID
		}
		result := tx.Raw(`
			SELECT project_id, status, provider_request_key,
			       model_profile_version_id, price_rule_version_id
			FROM operation.operation WHERE id = ?::uuid AND NOT is_delete FOR UPDATE
		`, identity.OperationID.String()).Scan(&op)
		if result.Error != nil {
			return fmt.Errorf("lock provider dispatch operation: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		if op.ProjectID != identity.ProjectID || op.ProviderRequestKey == nil ||
			*op.ProviderRequestKey != identity.RequestKey || op.ModelProfileVersionID == nil ||
			*op.ModelProfileVersionID != identity.ModelProfileVersionID || op.PriceRuleVersionID == nil ||
			*op.PriceRuleVersionID != identity.PriceRuleVersionID {
			return application.ErrProviderCallConflict
		}
		var calls []providerCallRow
		if err := tx.Raw(`
			SELECT id, create_time::text AS create_time, request_summary, request_key,
			       model_profile_version_id, price_rule_version_id, response_summary,
			       dispatch_started_at, receipt
			FROM operation.provider_call
			WHERE project_id = ?::uuid AND operation_id = ?::uuid
			  AND action = ? AND attempt = ? AND NOT is_delete FOR UPDATE
		`, identity.ProjectID.String(), identity.OperationID.String(), identity.Action,
			identity.Attempt).Scan(&calls).Error; err != nil {
			return fmt.Errorf("lock provider dispatch call: %w", err)
		}
		if len(calls) != 1 || !callHasDispatchIdentity(calls[0], identity) {
			return application.ErrProviderCallConflict
		}
		call := calls[0]
		dispatch.DispatchStartedAt = call.DispatchStartedAt
		if len(call.Receipt) > 0 {
			var receipt application.ProviderReceipt
			if err := json.Unmarshal(call.Receipt, &receipt); err != nil || receipt.Identity != identity || call.DispatchStartedAt == nil {
				return application.ErrProviderCallConflict
			}
			dispatch.Receipt = &receipt
		}
		if call.DispatchStartedAt != nil {
			return nil
		}
		if op.Status != "submitting" || len(call.ResponseSummary) != 0 {
			return application.ErrProviderCallConflict
		}
		var cancelled bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM operation.operation_event
			WHERE operation_id=?::uuid AND reason='user_cancel_requested' AND NOT is_delete)`,
			identity.OperationID).Scan(&cancelled).Error; err != nil {
			return fmt.Errorf("check dispatch cancellation: %w", err)
		}
		if cancelled {
			return application.ErrProviderDispatchCancelled
		}
		var claimed struct{ DispatchStartedAt *time.Time }
		result = tx.Raw(`
			UPDATE operation.provider_call
			SET dispatch_started_at = now(), update_time = now()
			WHERE project_id = ?::uuid AND id = ?::uuid AND create_time = ?::timestamptz
			  AND dispatch_started_at IS NULL AND NOT is_delete
			RETURNING dispatch_started_at
		`, identity.ProjectID.String(), call.ID.String(), call.CreateTime).Scan(&claimed)
		if result.Error != nil {
			return fmt.Errorf("claim provider dispatch: %w", result.Error)
		}
		if result.RowsAffected != 1 || claimed.DispatchStartedAt == nil {
			return application.ErrProviderCallConflict
		}
		dispatch.DispatchStartedAt = claimed.DispatchStartedAt
		dispatch.Claimed = true
		return nil
	})
	if err != nil {
		return application.ProviderDispatchResult{}, err
	}
	return dispatch, nil
}

func callHasDispatchIdentity(call providerCallRow, identity application.ProviderDispatchIdentity) bool {
	return callRequiresDispatch(call.RequestSummary) && call.RequestKey != nil &&
		*call.RequestKey == identity.RequestKey && call.ModelProfileVersionID != nil &&
		*call.ModelProfileVersionID == identity.ModelProfileVersionID && call.PriceRuleVersionID != nil &&
		*call.PriceRuleVersionID == identity.PriceRuleVersionID
}

func callRequiresDispatch(summary []byte) bool {
	var fields struct {
		DispatchContract string `json:"dispatch_contract"`
	}
	return json.Unmarshal(summary, &fields) == nil && fields.DispatchContract == "v1"
}

func validateNewDispatchAttempt(tx *gorm.DB, identity application.ProviderDispatchIdentity) error {
	if err := identity.Validate(); err != nil {
		return err
	}
	var previous []providerCallRow
	if err := tx.Raw(`
		SELECT attempt, request_summary, request_key, model_profile_version_id,
		       price_rule_version_id, outcome, response_summary
		FROM operation.provider_call
		WHERE project_id = ?::uuid AND operation_id = ?::uuid
		  AND action = 'submit' AND NOT is_delete ORDER BY attempt
	`, identity.ProjectID.String(), identity.OperationID.String()).Scan(&previous).Error; err != nil {
		return fmt.Errorf("read earlier provider dispatch attempts: %w", err)
	}
	if len(previous) != int(identity.Attempt)-1 {
		return application.ErrProviderCallConflict
	}
	for index, call := range previous {
		var state struct{ State string }
		if call.Attempt != int32(index)+1 || !callHasDispatchIdentity(call, identity) ||
			call.Outcome != "error" || json.Unmarshal(call.ResponseSummary, &state) != nil || state.State != "not_submitted" {
			return application.ErrProviderCallConflict
		}
	}
	return nil
}

func rejectManualExecutedCall(tx *gorm.DB, projectID, operationID uuid.UUID) error {
	var executed bool
	if err := tx.Raw(`
		SELECT EXISTS (
		  SELECT 1 FROM operation.provider_call
		  WHERE project_id = ?::uuid AND operation_id = ?::uuid AND NOT is_delete
		    AND (cost_micros > 0
		      OR action = 'submit' AND response_summary ->> 'state' = 'completed'
		      OR action = 'query' AND response_summary ->> 'state'
		         IN ('succeeded', 'failed', 'confirmed_not_exist')
		      OR action = 'cancel' AND response_summary ->> 'state' = 'cancelled')
		)
	`, projectID.String(), operationID.String()).Scan(&executed).Error; err != nil {
		return fmt.Errorf("check manual provider execution evidence: %w", err)
	}
	if executed {
		return application.ErrProviderCallConflict
	}
	return nil
}

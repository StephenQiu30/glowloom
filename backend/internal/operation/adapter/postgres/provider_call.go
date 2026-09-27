package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

type providerCallRow struct {
	ID              uuid.UUID
	CreateTime      string
	Outcome         string
	ProviderTaskID  *string
	ResponseSummary []byte
	Usage           []byte
	CostMicros      *int64
}

const manualNotExecutedReason = "manual_not_executed"

// RecordManualNotExecuted stores an administrator's private decision before a
// resolve_manual signal is sent. The caller must be an authenticated command;
// this method checks current administrator membership again under the row lock.
func (s *Store) RecordManualNotExecuted(ctx context.Context, input application.ManualNotExecutedInput) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if err := input.Validate(); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var op struct {
			ProjectID uuid.UUID
			Status    string
		}
		result := tx.Raw(`
			SELECT project_id, status FROM operation.operation
			WHERE id = ?::uuid AND NOT is_delete FOR UPDATE
		`, input.OperationID.String()).Scan(&op)
		if result.Error != nil {
			return fmt.Errorf("lock manual provider operation: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		var previous []struct{ Detail []byte }
		if err := tx.Raw(`
			SELECT detail FROM operation.operation_event
			WHERE operation_id = ?::uuid AND to_status = 'manual'
			  AND reason = ? AND NOT is_delete
		`, input.OperationID.String(), manualNotExecutedReason).Scan(&previous).Error; err != nil {
			return fmt.Errorf("read manual resolution replay: %w", err)
		}
		if len(previous) > 0 {
			want, err := json.Marshal(map[string]string{
				"admin_id": input.AdminID.String(), "evidence": input.Evidence,
			})
			if err != nil {
				return err
			}
			if len(previous) == 1 && jsonEqual(previous[0].Detail, want) {
				return nil
			}
			return application.ErrProviderCallConflict
		}
		if op.Status != "manual" {
			return application.ErrInvalidProviderCall
		}
		var authorized int
		result = tx.Raw(`
			SELECT 1 FROM identity."user" AS u
			JOIN workspace.project AS p ON p.org_id = u.org_id
			WHERE u.id = ?::uuid AND p.id = ?::uuid
			  AND u.role = 'admin' AND u.status = 'active'
			  AND NOT u.must_change_password AND NOT u.is_delete AND NOT p.is_delete
			FOR SHARE OF u, p
		`, input.AdminID.String(), op.ProjectID.String()).Scan(&authorized)
		if result.Error != nil {
			return fmt.Errorf("verify manual provider administrator: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return application.ErrManualResolutionUnverified
		}
		detail, err := json.Marshal(map[string]string{
			"admin_id": input.AdminID.String(), "evidence": input.Evidence,
		})
		if err != nil {
			return fmt.Errorf("encode manual resolution evidence: %w", err)
		}
		result = tx.Exec(`
			INSERT INTO operation.operation_event
			  (id, operation_id, from_status, to_status, reason, detail)
			VALUES (?::uuid, ?::uuid, 'manual', 'manual', ?, ?::jsonb)
		`, uuid.NewString(), input.OperationID.String(), manualNotExecutedReason, string(detail))
		if result.Error != nil {
			return fmt.Errorf("record manual provider resolution: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return application.ErrProviderCallConflict
		}
		return nil
	})
}

// CheckManualNotExecuted is a read-only gate for a signal that arrived before
// the administrator command committed its decision.
func (s *Store) CheckManualNotExecuted(ctx context.Context, operationID uuid.UUID) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if operationID == uuid.Nil {
		return application.ErrInvalidProviderCall
	}
	var present int
	result := s.db.WithContext(ctx).Raw(`
		SELECT 1 FROM operation.operation_event AS e
		JOIN operation.operation AS o ON o.id = e.operation_id
		WHERE e.operation_id = ?::uuid AND o.status = 'manual'
		  AND e.to_status = 'manual' AND e.reason = ?
		  AND NOT e.is_delete AND NOT o.is_delete
		LIMIT 1
	`, operationID.String(), manualNotExecutedReason).Scan(&present)
	if result.Error != nil {
		return fmt.Errorf("check manual provider resolution: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return application.ErrManualResolutionUnverified
	}
	return nil
}

// BeginProviderCall persists an unknown attempt before the provider Activity is
// dispatched. The operation row lock serializes attempts across Worker instances.
func (s *Store) BeginProviderCall(ctx context.Context, input application.BeginProviderCallInput) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if err := input.Validate(); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var op struct {
			ProjectID             uuid.UUID
			Status                string
			ProviderRequestKey    *string
			ModelProfileVersionID *uuid.UUID
			PriceRuleVersionID    *uuid.UUID
			Region                *string
		}
		result := tx.Raw(`
			SELECT project_id, status, provider_request_key, model_profile_version_id,
			       price_rule_version_id, region
			FROM operation.operation
			WHERE id = ?::uuid AND NOT is_delete FOR UPDATE
		`, input.OperationID.String()).Scan(&op)
		if result.Error != nil {
			return fmt.Errorf("lock operation before provider call: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		var previous []providerCallRow
		if err := tx.Raw(`
			SELECT id, create_time::text AS create_time, outcome, provider_task_id,
			       response_summary, usage, cost_micros
			FROM operation.provider_call
			WHERE project_id = ?::uuid AND operation_id = ?::uuid
			  AND action = ? AND attempt = ? AND NOT is_delete
		`, op.ProjectID.String(), input.OperationID.String(), input.Action, input.Attempt).Scan(&previous).Error; err != nil {
			return fmt.Errorf("read provider call replay: %w", err)
		}
		if len(previous) != 0 {
			if len(previous) != 1 || (input.ProviderTaskID != nil &&
				!sameOptionalString(previous[0].ProviderTaskID, input.ProviderTaskID)) {
				return application.ErrProviderCallConflict
			}
			return nil
		}
		if op.Status == "completed" || op.Status == "failed" || op.Status == "cancelled" ||
			op.Status == "expired" || op.ModelProfileVersionID == nil || op.Region == nil ||
			op.PriceRuleVersionID == nil || op.ProviderRequestKey == nil || strings.TrimSpace(*op.ProviderRequestKey) == "" {
			return application.ErrInvalidProviderCall
		}
		var provider struct {
			ProviderKey string
			AdapterKey  string
		}
		result = tx.Raw(`
			SELECT p.key AS provider_key, p.adapter_key
			FROM catalog.model_profile_version AS v
			JOIN catalog.model_profile AS m ON m.id = v.model_profile_id
			JOIN catalog.provider AS p ON p.id = m.provider_id
			WHERE v.id = ?::uuid AND NOT v.is_delete AND NOT m.is_delete AND NOT p.is_delete
		`, op.ModelProfileVersionID.String()).Scan(&provider)
		if result.Error != nil {
			return fmt.Errorf("read provider call model: %w", result.Error)
		}
		if result.RowsAffected != 1 || provider.ProviderKey == "" || provider.AdapterKey == "" {
			return application.ErrInvalidProviderCall
		}
		summary, err := json.Marshal(map[string]any{
			"operation_id": input.OperationID.String(), "action": input.Action,
		})
		if err != nil {
			return fmt.Errorf("encode provider call summary: %w", err)
		}
		result = tx.Exec(`
			INSERT INTO operation.provider_call
			  (id, project_id, operation_id, attempt, action, provider_key,
			   provider_task_id, request_summary, outcome, region, request_key,
			   model_profile_version_id, price_rule_version_id)
			VALUES (?::uuid, ?::uuid, ?::uuid, ?, ?, ?, ?, ?::jsonb, 'unknown', ?, ?, ?::uuid, ?::uuid)
		`, uuid.NewString(), op.ProjectID.String(), input.OperationID.String(),
			input.Attempt, input.Action, provider.ProviderKey, input.ProviderTaskID,
			string(summary), *op.Region, *op.ProviderRequestKey,
			op.ModelProfileVersionID.String(), op.PriceRuleVersionID)
		if result.Error != nil {
			return fmt.Errorf("begin provider call: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return application.ErrProviderCallConflict
		}
		return nil
	})
}

// CompleteProviderCall stores only redacted state, usage and cost. Mock calls
// are explicitly zero cost; real adapter usage remains unpriced and cannot be
// settled until a trusted pricing implementation is installed.
func (s *Store) CompleteProviderCall(ctx context.Context, input application.CompleteProviderCallInput) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if err := input.Validate(); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var op struct {
			ProjectID             uuid.UUID
			ModelProfileVersionID *uuid.UUID
		}
		result := tx.Raw(`
			SELECT project_id, model_profile_version_id FROM operation.operation
			WHERE id = ?::uuid AND NOT is_delete FOR UPDATE
		`, input.OperationID.String()).Scan(&op)
		if result.Error != nil {
			return fmt.Errorf("lock operation for provider result: %w", result.Error)
		}
		if result.RowsAffected != 1 || op.ModelProfileVersionID == nil {
			return ErrNotFound
		}
		var calls []providerCallRow
		if err := tx.Raw(`
			SELECT id, create_time::text AS create_time, outcome, provider_task_id,
			       response_summary, usage, cost_micros
			FROM operation.provider_call
			WHERE project_id = ?::uuid AND operation_id = ?::uuid
			  AND action = ? AND attempt = ? AND NOT is_delete
			FOR UPDATE
		`, op.ProjectID.String(), input.OperationID.String(), input.Action, input.Attempt).Scan(&calls).Error; err != nil {
			return fmt.Errorf("lock provider call result: %w", err)
		}
		if len(calls) != 1 {
			return application.ErrProviderCallConflict
		}
		call := calls[0]
		if call.ProviderTaskID != nil && input.ProviderTaskID != nil && *call.ProviderTaskID != *input.ProviderTaskID {
			return application.ErrProviderCallConflict
		}
		if input.ProviderTaskID == nil {
			input.ProviderTaskID = call.ProviderTaskID
		}
		state, err := json.Marshal(map[string]string{"state": input.State})
		if err != nil {
			return fmt.Errorf("encode provider result state: %w", err)
		}
		if len(call.ResponseSummary) != 0 {
			if call.Outcome == input.Outcome && jsonEqual(call.ResponseSummary, state) &&
				sameOptionalString(call.ProviderTaskID, input.ProviderTaskID) &&
				jsonEqual(call.Usage, input.Usage) {
				return nil
			}
			if call.Outcome != "unknown" || input.Outcome == "unknown" {
				return application.ErrProviderCallConflict
			}
		}
		var adapter struct{ AdapterKey string }
		result = tx.Raw(`
			SELECT p.adapter_key
			FROM catalog.model_profile_version AS v
			JOIN catalog.model_profile AS m ON m.id = v.model_profile_id
			JOIN catalog.provider AS p ON p.id = m.provider_id
			WHERE v.id = ?::uuid
		`, op.ModelProfileVersionID.String()).Scan(&adapter)
		if result.Error != nil {
			return fmt.Errorf("read provider adapter for result: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return application.ErrInvalidProviderCall
		}
		var cost *int64
		if adapter.AdapterKey == "mock" {
			zero := int64(0)
			cost = &zero
		}
		result = tx.Exec(`
			UPDATE operation.provider_call
			SET outcome = ?, provider_task_id = ?, response_summary = ?::jsonb,
			    usage = ?::jsonb, cost_micros = ?, update_time = now()
			WHERE project_id = ?::uuid AND id = ?::uuid
			  AND create_time = ?::timestamptz AND NOT is_delete
		`, input.Outcome, input.ProviderTaskID, string(state), nullableProviderUsage(input.Usage),
			cost, op.ProjectID.String(), call.ID.String(), call.CreateTime)
		if result.Error != nil {
			return fmt.Errorf("complete provider call: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return application.ErrProviderCallConflict
		}
		return nil
	})
}

// LoadProviderCost returns the trusted charge after all relevant calls have a
// conclusive result. The finalizer repeats this read under its terminal lock.
func (s *Store) LoadProviderCost(ctx context.Context, operationID uuid.UUID) (application.ProviderCost, error) {
	if s == nil || s.db == nil {
		return application.ProviderCost{}, ErrUnavailable
	}
	var cost application.ProviderCost
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		cost, err = s.ProviderCostInTransaction(ctx, tx, operationID)
		return err
	})
	return cost, err
}

// ProviderCostInTransaction is called under the finalizer's operation row lock.
func (s *Store) ProviderCostInTransaction(ctx context.Context, tx *gorm.DB, operationID uuid.UUID) (application.ProviderCost, error) {
	if s == nil || s.db == nil || tx == nil || operationID == uuid.Nil {
		return application.ProviderCost{}, application.ErrInvalidProviderCall
	}
	var op struct {
		ProjectID             uuid.UUID
		Status                string
		ReusedFromID          *uuid.UUID
		ModelProfileVersionID *uuid.UUID
	}
	result := tx.WithContext(ctx).Raw(`
		SELECT project_id, status, reused_from_id, model_profile_version_id FROM operation.operation
		WHERE id = ?::uuid AND NOT is_delete FOR UPDATE
	`, operationID.String()).Scan(&op)
	if result.Error != nil {
		return application.ProviderCost{}, fmt.Errorf("lock provider cost operation: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return application.ProviderCost{}, ErrNotFound
	}
	var calls []struct {
		Action         string
		Outcome        string
		State          string
		CostMicros     *int64
		ProviderTaskID *string
	}
	if err := tx.WithContext(ctx).Raw(`
		SELECT action, outcome, COALESCE(response_summary ->> 'state', '') AS state,
		       cost_micros, provider_task_id
		FROM operation.provider_call
		WHERE project_id = ?::uuid AND operation_id = ?::uuid AND NOT is_delete
		ORDER BY create_time, attempt
	`, op.ProjectID.String(), operationID.String()).Scan(&calls).Error; err != nil {
		return application.ProviderCost{}, fmt.Errorf("read provider charge evidence: %w", err)
	}
	cost := application.ProviderCost{}
	var unknownCost bool
	var unknownSubmission, acceptedSubmission, conclusiveResolution bool
	for _, call := range calls {
		if call.Action == "submit" {
			cost.HasSubmission = true
			unknownSubmission = unknownSubmission || call.Outcome == "unknown" || call.Outcome == "timeout"
			acceptedSubmission = acceptedSubmission || call.State == "accepted"
		}
		if (call.Action == "query" || call.Action == "cancel") &&
			providerCallEstablishesCharge(call.Action, call.State) {
			conclusiveResolution = true
		}
		if call.CostMicros == nil {
			unknownCost = true
			continue
		}
		if *call.CostMicros < 0 || cost.ActualCostMicros > math.MaxInt64-*call.CostMicros {
			return application.ProviderCost{}, application.ErrProviderCallConflict
		}
		cost.ActualCostMicros += *call.CostMicros
	}
	if !cost.HasSubmission {
		if len(calls) == 0 && (op.Status == "confirmed" || op.Status == "submitting" || op.ReusedFromID != nil) {
			return cost, nil
		}
		return application.ProviderCost{}, application.ErrProviderCostUnknown
	}
	if unknownCost || (unknownSubmission || acceptedSubmission) && !conclusiveResolution {
		if op.Status == "manual" && op.ModelProfileVersionID != nil {
			var adapter struct{ AdapterKey string }
			result = tx.WithContext(ctx).Raw(`
				SELECT p.adapter_key FROM catalog.model_profile_version AS v
				JOIN catalog.model_profile AS m ON m.id = v.model_profile_id
				JOIN catalog.provider AS p ON p.id = m.provider_id
				WHERE v.id = ?::uuid
			`, op.ModelProfileVersionID.String()).Scan(&adapter)
			if result.Error != nil {
				return application.ProviderCost{}, fmt.Errorf("read manual provider adapter: %w", result.Error)
			}
			if result.RowsAffected == 1 && adapter.AdapterKey == "mock" {
				var present int
				result = tx.WithContext(ctx).Raw(`
					SELECT 1 FROM operation.operation_event
					WHERE operation_id = ?::uuid AND to_status = 'manual'
					  AND reason = ? AND NOT is_delete LIMIT 1
				`, operationID.String(), manualNotExecutedReason).Scan(&present)
				if result.Error != nil {
					return application.ProviderCost{}, fmt.Errorf("read manual charge evidence: %w", result.Error)
				}
				if result.RowsAffected == 1 && cost.ActualCostMicros == 0 {
					cost.ManualNotExecuted = true
					return cost, nil
				}
				return application.ProviderCost{}, application.ErrManualResolutionUnverified
			}
		}
		return application.ProviderCost{}, application.ErrProviderCostUnknown
	}
	return cost, nil
}

func providerCallEstablishesCharge(action, state string) bool {
	switch action {
	case "submit":
		return state == "rejected" || state == "not_submitted"
	case "query":
		return state == "succeeded" || state == "failed" || state == "confirmed_not_exist"
	case "cancel":
		return state == "cancelled"
	default:
		return false
	}
}

func sameOptionalString(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func jsonEqual(a, b []byte) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	var left, right any
	return json.Unmarshal(a, &left) == nil && json.Unmarshal(b, &right) == nil &&
		reflect.DeepEqual(left, right)
}

func nullableProviderUsage(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	return string(raw)
}

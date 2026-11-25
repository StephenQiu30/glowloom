package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
)

type supplierObservationRecord struct {
	Original             application.SupplierObservation `json:"original"`
	ModelVersionID       uuid.UUID                       `json:"model_version_id"`
	PriceVersionID       uuid.UUID                       `json:"price_version_id"`
	ReleaseID            *uuid.UUID                      `json:"release_id,omitempty"`
	ProcurementVersionID *uuid.UUID                      `json:"procurement_version_id,omitempty"`
	CredentialID         *uuid.UUID                      `json:"credential_id,omitempty"`
	DispatchStartedAt    time.Time                       `json:"dispatch_started_at"`
	Mode                 string                          `json:"mode"`
	Params               json.RawMessage                 `json:"params"`
	Capability           string                          `json:"capability"`
}

// RecordSupplierObservation seals the small original response before any download,
// FFmpeg, artifact hash or Media transfer. This is historical liability evidence,
// not a new business effect: it never authorizes a send, output or reservation.
// Exact original dispatch/call identity is mandatory; current account revocation
// cannot erase an already issued supplier response or known cost.
func (s *Store) RecordSupplierObservation(ctx context.Context, o application.SupplierObservation) error {
	if s == nil || s.db == nil || s.db.Statement == nil {
		return ErrUnavailable
	}
	if o.Validate() != nil {
		return application.ErrInvalidProviderCall
	}
	if _, nested := s.db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return application.ErrInvalidProviderCall
	}
	var owner string
	if err := s.db.WithContext(ctx).Raw(`SELECT infra.runtime_owner()`).Scan(&owner).Error; err != nil {
		return err
	}
	if owner != "execution" && owner != "legacy" {
		return application.ErrInvalidProviderCall
	}
	var callID uuid.UUID
	var callCreated time.Time
	var record supplierObservationRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct {
			ID                                            uuid.UUID
			CreateTime                                    time.Time
			ResponseSummary                               json.RawMessage
			ProviderTaskID                                *string
			ModelVersionID, PriceVersionID                uuid.UUID
			Mode, Capability                              string
			Params                                        json.RawMessage
			DispatchStartedAt                             *time.Time
			ReleaseID, ProcurementVersionID, CredentialID *uuid.UUID
		}
		r := tx.Raw(`SELECT c.id,c.create_time,c.response_summary,c.provider_task_id,op.model_profile_version_id AS model_version_id,op.price_rule_version_id AS price_version_id,op.mode,op.capability,op.params,original.dispatch_started_at,COALESCE(retry.release_id,routed.release_id,binding.release_id) AS release_id,COALESCE(retry.procurement_price_id,routed.procurement_price_id) AS procurement_version_id,COALESCE(retry.credential_id,credentials.credential_id) AS credential_id FROM operation.operation op JOIN operation.provider_call original ON original.operation_id=op.id AND original.project_id=op.project_id AND original.action='submit' AND original.attempt=? AND original.request_key=? AND original.dispatch_started_at IS NOT NULL AND NOT original.is_delete JOIN operation.provider_call c ON c.operation_id=op.id AND c.project_id=op.project_id AND c.action=? AND c.attempt=? AND NOT c.is_delete JOIN operation.provider_binding binding ON binding.operation_id=op.id JOIN operation.provider_credential_binding credentials ON credentials.operation_id=op.id LEFT JOIN operation.route_delivery routed ON routed.operation_id=op.id LEFT JOIN operation.route_retry retry ON retry.operation_id=op.id WHERE op.id=? AND op.project_id=? AND op.provider_request_key=? AND op.model_profile_version_id=? AND op.price_rule_version_id=? AND c.request_key=? AND c.model_profile_version_id=? AND c.price_rule_version_id=? AND NOT op.is_delete FOR UPDATE OF op,c`, o.Identity.Attempt, o.Identity.RequestKey, o.CallAction, o.CallAttempt, o.Identity.OperationID, o.Identity.ProjectID, o.Identity.RequestKey, o.Identity.ModelProfileVersionID, o.Identity.PriceRuleVersionID, o.Identity.RequestKey, o.Identity.ModelProfileVersionID, o.Identity.PriceRuleVersionID).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 || row.DispatchStartedAt == nil {
			return application.ErrProviderCallConflict
		}
		if o.CallAction == "query" && (row.ProviderTaskID == nil || *row.ProviderTaskID != o.TaskID) {
			return application.ErrProviderCallConflict
		}
		if row.ProviderTaskID != nil && *row.ProviderTaskID != o.TaskID {
			return application.ErrProviderCallConflict
		}
		summary := map[string]json.RawMessage{}
		if len(row.ResponseSummary) > 0 && string(row.ResponseSummary) != "null" {
			if json.Unmarshal(row.ResponseSummary, &summary) != nil {
				return application.ErrProviderCallConflict
			}
		}
		if raw := summary["supplier_observation"]; len(raw) > 0 {
			if json.Unmarshal(raw, &record) != nil {
				return application.ErrProviderCallConflict
			}
			previous, _ := json.Marshal(record.Original)
			incoming, _ := json.Marshal(o)
			if !bytes.Equal(previous, incoming) {
				return application.ErrProviderCallConflict
			}
			callID, callCreated = row.ID, row.CreateTime
			return nil
		}
		record = supplierObservationRecord{Original: o, ModelVersionID: row.ModelVersionID, PriceVersionID: row.PriceVersionID, ReleaseID: row.ReleaseID, ProcurementVersionID: row.ProcurementVersionID, CredentialID: row.CredentialID, DispatchStartedAt: *row.DispatchStartedAt, Mode: row.Mode, Params: row.Params, Capability: row.Capability}
		raw, err := json.Marshal(record)
		if err != nil {
			return err
		}
		summary["supplier_observation"] = raw
		encoded, err := json.Marshal(summary)
		if err != nil {
			return err
		}
		updated := tx.Exec(`UPDATE operation.provider_call SET response_summary=?::jsonb,update_time=CURRENT_TIMESTAMP WHERE id=? AND create_time=?`, string(encoded), row.ID, row.CreateTime)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return application.ErrProviderCallConflict
		}
		callID, callCreated = row.ID, row.CreateTime
		return nil
	})
	if err != nil {
		return err
	}
	// The small receipt has already committed. A pricing/connection failure below
	// cannot roll it back; recovery can repeat frozen pricing without any supplier IO.
	usage, err := record.Original.ObservedUsage(record.Capability)
	if err != nil {
		return nil
	}
	return s.priceObservedSupplierCall(ctx, callID, callCreated, record, usage)
}

func (s *Store) priceObservedSupplierCall(ctx context.Context, id uuid.UUID, created time.Time, record supplierObservationRecord, usage json.RawMessage) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked struct{ ID uuid.UUID }
		lock := tx.Raw(`SELECT id FROM operation.operation WHERE id=? AND project_id=? FOR UPDATE`, record.Original.Identity.OperationID, record.Original.Identity.ProjectID).Scan(&locked)
		if lock.Error != nil {
			return lock.Error
		}
		if lock.RowsAffected != 1 {
			return application.ErrProviderCallConflict
		}
		var existing struct {
			CostMicros            *int64
			ProcurementCostMicros *int64
			ResponseSummary       json.RawMessage
		}
		r := tx.Raw(`SELECT cost_micros,procurement_cost_micros,response_summary FROM operation.provider_call WHERE id=? AND create_time=? FOR UPDATE`, id, created).Scan(&existing)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrProviderCallConflict
		}
		route, err := observedSupplierRoute(tx, record)
		if err != nil {
			return err
		}
		normalized, err := managedBillableUsage(tx, record.Original.Identity.OperationID, record.PriceVersionID, usage)
		if err != nil {
			return nil
		}
		usage = normalized
		price, procurement, err := priceProviderUsageAtRoute(tx, record.Original.Identity.OperationID, &record.ModelVersionID, &record.PriceVersionID, record.Mode, record.Params, usage, &record.DispatchStartedAt, record.Original.ReceivedAt, &route)
		if err != nil {
			// Missing or conflicting normalized facts stay unknown. Never write an
			// invented zero and never erase any previously established charge.
			return nil
		}
		var previous []struct {
			ProviderTaskID *string
			Usage          json.RawMessage
			CostMicros     *int64
		}
		if record.Original.CallAction == "query" {
			if err := tx.Raw(`SELECT provider_task_id,usage,cost_micros FROM operation.provider_call WHERE operation_id=? AND (create_time,id)<(?,?) AND action IN ('query','cancel') AND cost_micros IS NOT NULL AND NOT is_delete AND (response_summary->>'state' IN ('succeeded','failed','cancelled') OR response_summary#>>'{supplier_observation,original,state}'='succeeded')`, record.Original.Identity.OperationID, created, id).Scan(&previous).Error; err != nil {
				return err
			}
			if len(previous) > 0 {
				for _, p := range previous {
					if p.ProviderTaskID == nil || *p.ProviderTaskID != record.Original.TaskID || !jsonEqual(p.Usage, usage) {
						return nil
					}
				}
				price = 0
				if procurement != nil {
					zero := int64(0)
					procurement = &zero
				}
			}
		}
		if existing.CostMicros != nil && *existing.CostMicros != price {
			return application.ErrProviderCallConflict
		}
		if existing.ProcurementCostMicros != nil && procurement != nil && *existing.ProcurementCostMicros != *procurement {
			return application.ErrProviderCallConflict
		}
		if existing.CostMicros != nil {
			price = *existing.CostMicros
		}
		if existing.ProcurementCostMicros != nil {
			procurement = existing.ProcurementCostMicros
		}
		result := tx.Exec(`UPDATE operation.provider_call SET cost_micros=?,procurement_cost_micros=?,usage=?::jsonb,provider_task_id=COALESCE(provider_task_id,NULLIF(?,'')),update_time=CURRENT_TIMESTAMP WHERE id=? AND create_time=?`, price, procurement, string(usage), record.Original.TaskID, id, created)
		if result.Error != nil {
			return fmt.Errorf("persist original observed cost: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return application.ErrProviderCallConflict
		}
		return nil
	})
}

func observedSupplierRoute(tx *gorm.DB, record supplierObservationRecord) (providerUsageRoute, error) {
	route := providerUsageRoute{VersionID: record.ModelVersionID}
	if record.ProcurementVersionID != nil {
		route.ProcurementID = *record.ProcurementVersionID
	}
	if record.ReleaseID != nil {
		read := tx.Raw(`SELECT model_version_id AS version_id FROM catalog.model_release WHERE id=?`, *record.ReleaseID).Scan(&route)
		if read.Error != nil {
			return route, read.Error
		}
		if read.RowsAffected != 1 {
			return route, application.ErrProviderCallConflict
		}
	}
	return route, nil
}

// ReadSupplierObservation reads original durable task evidence inside Execution.
func (s *Store) ReadSupplierObservation(ctx context.Context, i application.ProviderDispatchIdentity) (*application.SupplierObservation, error) {
	if s == nil || s.db == nil || i.Validate() != nil {
		return nil, application.ErrInvalidProviderCall
	}
	var row struct{ Observation json.RawMessage }
	read := s.db.WithContext(ctx).Raw(`SELECT c.response_summary->'supplier_observation' AS observation FROM operation.provider_call c JOIN operation.operation o ON o.id=c.operation_id AND o.project_id=c.project_id WHERE o.id=? AND o.project_id=? AND o.provider_request_key=? AND o.model_profile_version_id=? AND o.price_rule_version_id=? AND c.request_key=o.provider_request_key AND c.model_profile_version_id=o.model_profile_version_id AND c.price_rule_version_id=o.price_rule_version_id AND c.response_summary ? 'supplier_observation' AND NOT c.is_delete ORDER BY c.create_time DESC,c.attempt DESC LIMIT 1`, i.OperationID, i.ProjectID, i.RequestKey, i.ModelProfileVersionID, i.PriceRuleVersionID).Scan(&row)
	if read.Error != nil {
		return nil, read.Error
	}
	if read.RowsAffected == 0 {
		return nil, nil
	}
	var record supplierObservationRecord
	if json.Unmarshal(row.Observation, &record) != nil || record.Original.Validate() != nil || record.Original.Identity != i {
		return nil, application.ErrProviderCallConflict
	}
	return &record.Original, nil
}

var _ application.SupplierObservations = (*Store)(nil)
var _ application.SupplierObservationReader = (*Store)(nil)

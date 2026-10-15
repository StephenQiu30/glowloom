package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// SaveManagedResult validates the original dispatch before persisting evidence.
func (s *Store) SaveManagedResult(ctx context.Context, input application.ManagedResult) error {
	if input.Validate() != nil {
		return application.ErrInvalidProviderCall
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var present int
		i := input.Identity
		r := tx.Raw(`SELECT 1 FROM operation.operation op JOIN operation.provider_call c ON c.operation_id=op.id AND c.project_id=op.project_id
   WHERE op.id=? AND op.project_id=? AND op.model_profile_version_id=? AND op.price_rule_version_id=? AND op.provider_request_key=?
   AND c.action='submit' AND c.attempt=1 AND c.dispatch_started_at IS NOT NULL AND NOT c.is_delete AND NOT op.is_delete
   AND op.status IN ('submitting','submitted','unknown','reconciling','manual','succeeded','ingesting') FOR UPDATE OF op`, i.OperationID, i.ProjectID, i.ModelProfileVersionID, i.PriceRuleVersionID, i.RequestKey).Scan(&present)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrProviderCallConflict
		}
		var previous struct{ Result []byte }
		r = tx.Raw(`SELECT result FROM operation.provider_result WHERE operation_id=? FOR UPDATE`, i.OperationID).Scan(&previous)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 1 {
			var saved application.ManagedResult
			if json.Unmarshal(previous.Result, &saved) != nil || saved.Identity != i {
				return application.ErrProviderCallConflict
			}
			if jsonEqual(previous.Result, raw) {
				return nil
			}
			if saved.State != "accepted" || input.State != "succeeded" || saved.TaskID != input.TaskID {
				return application.ErrProviderCallConflict
			}
			return tx.Exec(`UPDATE operation.provider_result SET result=?::jsonb,update_time=now() WHERE operation_id=?`, string(raw), i.OperationID).Error
		}
		return tx.Exec(`INSERT INTO operation.provider_result(operation_id,result) VALUES(?,?::jsonb)`, i.OperationID, string(raw)).Error
	})
}

// ReadManagedResult returns private evidence only when its full identity matches.
func (s *Store) ReadManagedResult(ctx context.Context, i application.ProviderDispatchIdentity) (*application.ManagedResult, error) {
	if i.Validate() != nil {
		return nil, application.ErrInvalidProviderCall
	}
	var row struct{ Result []byte }
	r := s.db.WithContext(ctx).Raw(`SELECT result FROM operation.provider_result WHERE operation_id=?`, i.OperationID).Scan(&row)
	if r.Error != nil {
		return nil, fmt.Errorf("read managed result: %w", r.Error)
	}
	if r.RowsAffected == 0 {
		return nil, nil
	}
	var result application.ManagedResult
	if json.Unmarshal(row.Result, &result) != nil || result.Validate() != nil || result.Identity != i {
		return nil, application.ErrProviderCallConflict
	}
	return &result, nil
}

// IngestManagedText copies durable provider output to the project candidate table.
func (s *Store) IngestManagedText(ctx context.Context, i application.ProviderDispatchIdentity) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var present int
		r := tx.Raw(`SELECT 1 FROM operation.operation WHERE id=? AND project_id=? AND status='ingesting' AND capability='text.generate' AND NOT is_delete FOR UPDATE`, i.OperationID, i.ProjectID).Scan(&present)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrProviderCallConflict
		}
		result, err := NewStore(tx).ReadManagedResult(ctx, i)
		if err != nil {
			return err
		}
		if result == nil || result.State != "succeeded" || result.Text == "" {
			return application.ErrProviderCallConflict
		}
		payload, err := json.Marshal(map[string]string{"text": result.Text})
		if err != nil {
			return err
		}
		var previous struct{ Payload []byte }
		r = tx.Raw(`SELECT json_payload AS payload FROM operation.operation_output WHERE operation_id=? AND seq_no=1 AND NOT is_delete`, i.OperationID).Scan(&previous)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 1 {
			if !jsonEqual(previous.Payload, payload) {
				return application.ErrProviderCallConflict
			}
			return nil
		}
		return tx.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,json_payload,moderation_status) VALUES(?,?,?,1,'json',?::jsonb,'passed')`, uuid.New(), i.ProjectID, i.OperationID, string(payload)).Error
	})
}

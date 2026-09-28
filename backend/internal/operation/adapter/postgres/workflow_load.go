package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

// ErrWorkflowModelUnavailable means the frozen model or price version is missing.
var ErrWorkflowModelUnavailable = application.ErrWorkflowModelUnavailable

// LoadWorkflowOperation obtains one consistent workflow snapshot without
// putting a provider credential or signed result URL in Workflow history.
func (s *Store) LoadWorkflowOperation(ctx context.Context, operationID uuid.UUID) (application.WorkflowOperation, error) {
	if s == nil || s.db == nil {
		return application.WorkflowOperation{}, ErrUnavailable
	}
	if operationID == uuid.Nil {
		return application.WorkflowOperation{}, ErrNotFound
	}
	var loaded application.WorkflowOperation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row operationRow
		result := tx.Raw(`
			SELECT id, project_id, batch_id, target_type, target_id, target_key,
			       target_version_no, capability, mode, model_profile_version_id,
			       price_rule_version_id, params, output_count, input_hash, origin,
			       status, quote_micros, quote_detail, quote_expires_at, reused_from_id,
			       force_regenerate, reservation_id, confirmed_at, settled_micros,
			       region, create_time
			FROM operation.operation
			WHERE id = ?::uuid AND NOT is_delete
			FOR SHARE
		`, operationID.String()).Scan(&row)
		if result.Error != nil {
			return fmt.Errorf("read workflow operation: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		loaded.Operation = row.domain()
		if err := loaded.Operation.Validate(); err != nil {
			return fmt.Errorf("validate workflow operation: %w", err)
		}
		var metadata struct{ ProviderRequestKey *string }
		result = tx.Raw(`SELECT provider_request_key FROM operation.operation WHERE id = ?::uuid`, operationID.String()).Scan(&metadata)
		if result.Error != nil {
			return fmt.Errorf("read workflow request key: %w", result.Error)
		}
		loaded.ProviderRequestKey = "operation/" + operationID.String()
		if metadata.ProviderRequestKey != nil {
			loaded.ProviderRequestKey = *metadata.ProviderRequestKey
		}
		if loaded.Operation.ReusedFromID == nil {
			if err := loadWorkflowModel(tx, &loaded); err != nil {
				return err
			}
		}
		var inputs []operationInputRow
		result = tx.Raw(`
			SELECT id, operation_id, seq_no, role, ref_type, ref_id, ref_version,
			       text_value, media_asset_id, mask_asset_id
			FROM operation.operation_input
			WHERE operation_id = ?::uuid AND NOT is_delete
			ORDER BY seq_no
		`, operationID.String()).Scan(&inputs)
		if result.Error != nil {
			return fmt.Errorf("read frozen workflow inputs: %w", result.Error)
		}
		loaded.Inputs = make([]domain.OperationInput, 0, len(inputs))
		for _, input := range inputs {
			value := input.domain()
			if err := value.Validate(); err != nil {
				return fmt.Errorf("validate frozen workflow input: %w", err)
			}
			loaded.Inputs = append(loaded.Inputs, value)
		}
		var outputs []struct {
			ID               uuid.UUID
			SeqNo            int32
			Kind             string
			MediaAssetID     *uuid.UUID
			JSONPayload      []byte
			ModerationStatus string
			ModerationReason *string
		}
		result = tx.Raw(`
			SELECT id, seq_no, kind, media_asset_id, json_payload,
			       moderation_status, moderation_reason
			FROM operation.operation_output
			WHERE operation_id = ?::uuid AND project_id = ?::uuid AND NOT is_delete
			ORDER BY seq_no
		`, operationID.String(), loaded.Operation.ProjectID.String()).Scan(&outputs)
		if result.Error != nil {
			return fmt.Errorf("read workflow outputs: %w", result.Error)
		}
		loaded.Outputs = make([]application.WorkflowOutput, 0, len(outputs))
		for _, output := range outputs {
			loaded.Outputs = append(loaded.Outputs, application.WorkflowOutput{
				ID: output.ID, SeqNo: output.SeqNo, Kind: output.Kind,
				MediaAssetID: output.MediaAssetID, JSONPayload: json.RawMessage(output.JSONPayload),
				ModerationStatus: output.ModerationStatus, ModerationReason: output.ModerationReason,
			})
		}
		var latest struct{ ProviderTaskID *string }
		result = tx.Raw(`
			SELECT detail ->> 'provider_task_id' AS provider_task_id
			FROM operation.operation_event
			WHERE operation_id = ?::uuid
			  AND detail ? 'provider_task_id' AND NOT is_delete
			ORDER BY create_time DESC, id DESC LIMIT 1
		`, operationID.String()).Scan(&latest)
		if result.Error != nil {
			return fmt.Errorf("read workflow provider task: %w", result.Error)
		}
		loaded.ProviderTaskID = latest.ProviderTaskID
		return nil
	})
	if err != nil {
		return application.WorkflowOperation{}, fmt.Errorf("load workflow operation transaction: %w", err)
	}
	return loaded, nil
}

func loadWorkflowModel(tx *gorm.DB, loaded *application.WorkflowOperation) error {
	versionID := loaded.Operation.ModelProfileVersionID
	if versionID == nil {
		if loaded.Operation.Origin == "upload" || loaded.Operation.TargetType == "agent_session" {
			return nil
		}
		return ErrWorkflowModelUnavailable
	}
	var provider application.WorkflowProvider
	result := tx.Raw(`
		SELECT p.key AS provider_key, p.adapter_key, m.model_key,
		       v.provider_model_id,
		       v.queue, v.supports_query, v.supports_cancel,
		       v.expected_max_ms, v.moderation
		FROM catalog.model_profile_version AS v
		JOIN catalog.model_profile AS m ON m.id = v.model_profile_id
		JOIN catalog.provider AS p ON p.id = m.provider_id
		WHERE v.id = ?::uuid AND NOT v.is_delete AND NOT m.is_delete AND NOT p.is_delete
	`, versionID.String()).Scan(&provider)
	if result.Error != nil {
		return fmt.Errorf("read frozen workflow provider: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrWorkflowModelUnavailable
	}
	loaded.Provider = provider
	if loaded.Operation.PriceRuleVersionID != nil {
		var price struct{ Unit string }
		result = tx.Raw(`
			SELECT pr.unit
			FROM catalog.price_rule_version AS pr
			JOIN catalog.model_profile_version AS v ON v.model_profile_id = pr.model_profile_id
			WHERE pr.id = ?::uuid AND v.id = ?::uuid AND NOT pr.is_delete
		`, loaded.Operation.PriceRuleVersionID.String(), versionID.String()).Scan(&price)
		if result.Error != nil {
			return fmt.Errorf("read frozen workflow price unit: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrWorkflowModelUnavailable
		}
		loaded.PriceUnit = price.Unit
	}
	return nil
}

package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

// ProviderImageModel reads the immutable provider model for this exact frozen
// identity. A caller-supplied model name cannot replace the published version.
func (s *Store) ProviderImageModel(ctx context.Context, identity application.ProviderDispatchIdentity) (string, error) {
	if s == nil || s.db == nil || identity.Validate() != nil {
		return "", application.ErrInvalidProviderCall
	}
	var row struct{ Model string }
	result := s.db.WithContext(ctx).Raw(`
		SELECT v.provider_model_id AS model
		FROM operation.operation o
		JOIN catalog.model_profile_version v ON v.id=o.model_profile_version_id
		WHERE o.id=?::uuid AND o.project_id=?::uuid AND NOT o.is_delete
		  AND o.provider_request_key=? AND o.model_profile_version_id=?::uuid
		  AND o.price_rule_version_id=?::uuid
	`, identity.OperationID.String(), identity.ProjectID.String(), identity.RequestKey, identity.ModelProfileVersionID.String(), identity.PriceRuleVersionID.String()).Scan(&row)
	if result.Error != nil {
		return "", fmt.Errorf("read frozen provider image model: %w", result.Error)
	}
	if result.RowsAffected != 1 || row.Model == "" || strings.TrimSpace(row.Model) != row.Model || len(row.Model) > 200 {
		return "", application.ErrProviderCallConflict
	}
	return row.Model, nil
}

// LoadProviderImageIdentity finds only the latest explicitly gated submit call.
// Legacy NULL dispatch fields never establish a right to send or recover a turn.
func (s *Store) LoadProviderImageIdentity(ctx context.Context, operationID uuid.UUID) (application.ProviderDispatchIdentity, bool, error) {
	if s == nil || s.db == nil || operationID == uuid.Nil {
		return application.ProviderDispatchIdentity{}, false, application.ErrInvalidProviderCall
	}
	var row struct {
		ProjectID, OperationID                    uuid.UUID
		Attempt                                   int32
		RequestKey                                *string
		ModelProfileVersionID, PriceRuleVersionID *uuid.UUID
	}
	result := s.db.WithContext(ctx).Raw(`
		SELECT c.project_id,c.operation_id,c.attempt,c.request_key,
		       c.model_profile_version_id,c.price_rule_version_id
		FROM operation.provider_call c
		JOIN operation.operation o ON o.id=c.operation_id AND o.project_id=c.project_id
		WHERE o.id=?::uuid AND NOT o.is_delete AND NOT c.is_delete
		  AND c.action='submit' AND c.request_summary->>'dispatch_contract'='v1'
		  AND c.request_key=o.provider_request_key
		  AND c.model_profile_version_id=o.model_profile_version_id
		  AND c.price_rule_version_id=o.price_rule_version_id
		ORDER BY c.attempt DESC LIMIT 1
	`, operationID.String()).Scan(&row)
	if result.Error != nil {
		return application.ProviderDispatchIdentity{}, false, fmt.Errorf("read provider image call identity: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return application.ProviderDispatchIdentity{}, false, nil
	}
	if row.RequestKey == nil || row.ModelProfileVersionID == nil || row.PriceRuleVersionID == nil {
		return application.ProviderDispatchIdentity{}, false, application.ErrProviderCallConflict
	}
	identity := application.ProviderDispatchIdentity{ProjectID: row.ProjectID, OperationID: row.OperationID, Action: "submit", Attempt: row.Attempt, RequestKey: *row.RequestKey, ModelProfileVersionID: *row.ModelProfileVersionID, PriceRuleVersionID: *row.PriceRuleVersionID}
	if identity.Validate() != nil {
		return application.ProviderDispatchIdentity{}, false, application.ErrProviderCallConflict
	}
	return identity, true, nil
}

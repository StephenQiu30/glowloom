package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

// ProviderReceipt reads the immutable evidence of exactly one scoped submit.
// Staging readers cannot choose a bucket key or borrow another call's receipt.
func (s *Store) ProviderReceipt(ctx context.Context, identity application.ProviderDispatchIdentity) (application.ProviderReceipt, bool, error) {
	if s == nil || s.db == nil {
		return application.ProviderReceipt{}, false, ErrUnavailable
	}
	if identity.Validate() != nil {
		return application.ProviderReceipt{}, false, application.ErrInvalidProviderCall
	}
	var calls []providerCallRow
	if err := s.db.WithContext(ctx).Raw(`
  SELECT c.id, c.create_time::text AS create_time, c.request_summary,
         c.request_key, c.model_profile_version_id, c.price_rule_version_id,
         c.dispatch_started_at, c.receipt, c.outcome, c.response_summary
  FROM operation.provider_call AS c
  JOIN operation.operation AS o ON o.id=c.operation_id
  WHERE c.project_id=?::uuid AND o.project_id=c.project_id
    AND c.operation_id=?::uuid AND c.action=? AND c.attempt=?
    AND o.provider_request_key=c.request_key
    AND o.model_profile_version_id=c.model_profile_version_id
    AND o.price_rule_version_id=c.price_rule_version_id
    AND NOT c.is_delete AND NOT o.is_delete
 `, identity.ProjectID.String(), identity.OperationID.String(), identity.Action, identity.Attempt).Scan(&calls).Error; err != nil {
		return application.ProviderReceipt{}, false, fmt.Errorf("read scoped provider receipt: %w", err)
	}
	if len(calls) == 0 {
		return application.ProviderReceipt{}, false, nil
	}
	if len(calls) != 1 || !callHasDispatchIdentity(calls[0], identity) {
		return application.ProviderReceipt{}, false, application.ErrProviderCallConflict
	}
	call := calls[0]
	if len(call.Receipt) == 0 {
		return application.ProviderReceipt{}, false, nil
	}
	var state struct {
		State string `json:"state"`
	}
	var receipt application.ProviderReceipt
	if call.DispatchStartedAt == nil || call.Outcome != "ok" || json.Unmarshal(call.ResponseSummary, &state) != nil || state.State != "completed" || json.Unmarshal(call.Receipt, &receipt) != nil || receipt.Identity != identity {
		return application.ProviderReceipt{}, false, application.ErrProviderCallConflict
	}
	return receipt, true, nil
}

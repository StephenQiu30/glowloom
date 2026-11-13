package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

type deliveryRoute struct{ ReleaseID, ModelID, ProviderID, ConfigVersionID, ProcurementPriceID uuid.UUID }

// selectPoolMember runs inside the send-claim transaction. Freeze the actual
// purchase identity before the first byte can leave; normal recovery reads it.
func (s *Store) selectPoolMember(tx *gorm.DB, id application.ProviderDispatchIdentity) (*deliveryRoute, error) {
	var pool struct {
		VersionID uuid.UUID
		Mode      string
	}
	r := tx.Raw("SELECT b.version_id,o.mode FROM operation.route_pool_binding b JOIN operation.operation o ON o.id=b.operation_id WHERE b.operation_id=?", id.OperationID).Scan(&pool)
	if r.Error != nil || r.RowsAffected == 0 {
		return nil, r.Error
	}
	var rows []deliveryRoute
	// Lock all candidate connections in UUID order to serialize capacity checks
	// even when another pool orders the same two connections differently.
	if err := tx.Exec(`SELECT p.id FROM catalog.provider p WHERE p.id IN
 (SELECT m.provider_id FROM catalog.route_pool_member member JOIN catalog.model_release r ON r.id=member.release_id JOIN catalog.model_profile m ON m.id=r.model_id WHERE member.version_id=?) ORDER BY p.id FOR UPDATE`, pool.VersionID).Error; err != nil {
		return nil, err
	}
	r = tx.Raw(`SELECT release_id,model_id,provider_id,config_version_id,procurement_price_id
 FROM catalog.route_pool_ready WHERE version_id=? AND mode=? ORDER BY priority`, pool.VersionID, pool.Mode).Scan(&rows)
	if r.Error != nil {
		return nil, r.Error
	}
	for _, route := range rows {
		var limits struct{ ConcurrencyLimit, RateLimitPerMin int64 }
		if err := tx.Raw("SELECT concurrency_limit,rate_limit_per_min FROM catalog.effective_provider_limit WHERE id=?", route.ProviderID).Scan(&limits).Error; err != nil {
			return nil, err
		}
		usage, err := readProviderUsage(tx, route.ProviderID)
		if err != nil {
			return nil, err
		}
		if usage.Active >= limits.ConcurrencyLimit || usage.Recent >= limits.RateLimitPerMin {
			continue
		}
		if err := tx.Exec("INSERT INTO operation.route_delivery(operation_id,release_id,procurement_price_id) VALUES(?,?,?)", id.OperationID, route.ReleaseID, route.ProcurementPriceID).Error; err != nil {
			return nil, err
		}
		return &route, nil
	}
	if len(rows) > 0 {
		return nil, application.ErrManagedCapacity
	}
	return nil, application.ErrWorkflowModelUnavailable
}

// ClaimUnsentBackup reserves the only permitted replacement after the worker
// observed a failed dial. Replays and post-send timeouts cannot acquire it.
func (s *Store) ClaimUnsentBackup(ctx context.Context, id application.ProviderDispatchIdentity, failedKey uuid.UUID) (bool, error) {
	claimed := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireWorker(tx, id.OperationID, true); err != nil {
			return err
		}
		var pool struct {
			VersionID, ReleaseID uuid.UUID
			Mode                 string
		}
		r := tx.Raw(`SELECT b.version_id,d.release_id,o.mode FROM operation.operation o
 JOIN operation.route_pool_binding b ON b.operation_id=o.id JOIN operation.route_delivery d ON d.operation_id=o.id
 JOIN operation.provider_credential_binding k ON k.operation_id=o.id AND k.credential_id=?
 JOIN operation.provider_call c ON c.operation_id=o.id AND c.action='submit' AND c.attempt=1 AND c.dispatch_started_at IS NOT NULL AND c.receipt IS NULL AND NOT c.is_delete
 WHERE o.id=? AND o.project_id=? AND o.provider_request_key=? AND o.status='submitting' AND NOT o.is_delete
 AND NOT EXISTS(SELECT 1 FROM operation.operation_event WHERE operation_id=o.id AND reason='user_cancel_requested' AND NOT is_delete)
 AND NOT EXISTS(SELECT 1 FROM operation.route_retry WHERE operation_id=o.id) AND NOT EXISTS(SELECT 1 FROM operation.provider_result WHERE operation_id=o.id) FOR UPDATE OF o`, failedKey, id.OperationID, id.ProjectID, id.RequestKey).Scan(&pool)
		if r.Error != nil || r.RowsAffected != 1 {
			return r.Error
		}
		if err := checkStoredBillingBarrier(tx, id.ProjectID, id.OperationID); err != nil {
			return err
		}
		if err := s.checkWorkflowInputsForSubmission(tx, id.OperationID, id.ProjectID); err != nil {
			return err
		}
		if err := tx.Exec(`SELECT p.id FROM catalog.provider p WHERE p.id IN (SELECT c.provider_id FROM catalog.route_pool_ready c WHERE version_id=?) ORDER BY p.id FOR UPDATE`, pool.VersionID).Error; err != nil {
			return err
		}
		// A failed pre-connect dial applies to this connection, not its other
		// keys individually or unrelated connections sharing billing capacity.
		if err := tx.Exec(`INSERT INTO catalog.route_connection_cooldown(provider_id,until_at)
 SELECT provider_id,now()+interval '15 seconds' FROM catalog.provider_credential WHERE id=?
 ON CONFLICT(provider_id) DO UPDATE SET until_at=greatest(catalog.route_connection_cooldown.until_at,excluded.until_at)`, failedKey).Error; err != nil {
			return err
		}
		var rows []struct {
			ReleaseID, ProviderID, ProcurementPriceID uuid.UUID
			ConcurrencyLimit, RateLimitPerMin         int64
		}
		if err := tx.Raw(`SELECT c.release_id,c.provider_id,c.procurement_price_id,p.concurrency_limit,p.rate_limit_per_min FROM catalog.route_pool_ready c JOIN catalog.effective_provider_limit p ON p.id=c.provider_id WHERE version_id=? AND mode=? AND release_id<>? ORDER BY priority`, pool.VersionID, pool.Mode, pool.ReleaseID).Scan(&rows).Error; err != nil {
			return err
		}
		for _, route := range rows {
			u, err := readProviderUsage(tx, route.ProviderID)
			if err != nil {
				return err
			}
			if u.Active >= route.ConcurrencyLimit || u.Recent >= route.RateLimitPerMin {
				continue
			}
			r := tx.Exec(`INSERT INTO operation.route_retry(operation_id,release_id,credential_id,procurement_price_id,reason)
 SELECT ?,?,candidate.id,?,? FROM (SELECT k.id FROM catalog.verified_route_key v JOIN catalog.provider_credential k ON k.id=v.credential_id
 JOIN catalog.model_release r ON r.id=v.release_id JOIN catalog.model_profile m ON m.id=r.model_id
 JOIN catalog.provider_config_version cfg ON cfg.id=r.config_version_id JOIN catalog.provider_preset_version pv ON pv.id=cfg.preset_version_id
 JOIN catalog.provider_preset preset ON preset.id=pv.preset_id
 WHERE r.id=? AND v.mode=? AND NOT preset.emergency_disabled AND NOT preset.archived AND m.active_release_id=r.id AND m.status='active' AND r.status='active'
 AND (k.status='active' OR (k.status='draining' AND k.pool_enabled)) AND NOT k.is_delete
 ORDER BY k.id LIMIT 1 FOR SHARE OF k,m,r,preset) candidate
 ON CONFLICT DO NOTHING`, id.OperationID, route.ReleaseID, route.ProcurementPriceID, application.DialNotSent, route.ReleaseID, pool.Mode)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected == 1 {
				claimed = true
				return nil
			}
		}
		return nil
	})
	return claimed, err
}

package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type providerUsage struct{ Active, Recent int64 }

// ProviderUsage shares dispatch accounting with explicit catalog checks. Callers
// hold the provider row lock so admission is serialized with managed generation.
func (s *Store) ProviderUsage(ctx context.Context, id uuid.UUID) (int64, int64, error) {
	usage, err := readProviderUsage(s.db.WithContext(ctx), id)
	return usage.Active, usage.Recent, err
}
func readProviderUsage(tx *gorm.DB, id uuid.UUID) (providerUsage, error) {
	// All connections with this administrator-defined account label share one
	// capacity budget. Callers hold the connection lock before the group lock.
	var membership struct{ QuotaGroup string }
	if err := tx.Raw("SELECT quota_group FROM catalog.provider WHERE id=?", id).Scan(&membership).Error; err != nil {
		return providerUsage{}, err
	}
	ids := []uuid.UUID{id}
	if membership.QuotaGroup != "" {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "provider-quota/"+membership.QuotaGroup).Error; err != nil {
			return providerUsage{}, err
		}
		var peers []struct{ ID uuid.UUID }
		if err := tx.Raw("SELECT id FROM catalog.provider WHERE quota_group=?", membership.QuotaGroup).Scan(&peers).Error; err != nil {
			return providerUsage{}, err
		}
		ids = ids[:0]
		for _, peer := range peers {
			ids = append(ids, peer.ID)
		}
	}
	// A synchronous endpoint that returned a classified request rejection has
	// stopped executing. Its unresolved charge still belongs to the original
	// operation; an ambiguous transport failure or asynchronous job keeps its slot.
	var counts providerUsage
	err := tx.Raw(`SELECT count(DISTINCT op.id) FILTER(WHERE op.status NOT IN ('completed','failed','cancelled','expired')
 AND NOT (op.status='manual' AND NOT v.supports_query AND op.capability IN ('image.generate','text.generate')
 AND call.provider_task_id IS NULL AND call.response_summary->>'state' IS NOT DISTINCT FROM 'unknown'
 AND COALESCE(call.response_summary->>'failure_code','') IN ('provider_invalid_parameters','provider_not_found'))) AS active,
 count(DISTINCT op.id) FILTER(WHERE COALESCE(retry.created_at,call.dispatch_started_at)>now()-interval '1 minute') AS recent
 FROM operation.provider_credential_binding cb LEFT JOIN operation.route_retry retry ON retry.operation_id=cb.operation_id JOIN catalog.provider_credential key ON key.id=COALESCE(retry.credential_id,cb.credential_id)
 JOIN operation.operation op ON op.id=cb.operation_id JOIN catalog.model_profile_version v ON v.id=op.model_profile_version_id JOIN operation.provider_call call ON call.operation_id=op.id AND call.action='submit' AND call.dispatch_started_at IS NOT NULL AND NOT call.is_delete
 WHERE key.provider_id IN ?`, ids).Scan(&counts).Error
	if err != nil {
		return counts, err
	}
	var voice, checks providerUsage
	if err := tx.Raw(`SELECT count(*) FILTER(WHERE status IN ('dispatching','unknown')) AS active,count(*) FILTER(WHERE dispatch_started_at>now()-interval '1 minute') AS recent FROM subject.voice_job WHERE provider_id IN ?`, ids).Scan(&voice).Error; err != nil {
		return counts, err
	}
	if err := tx.Raw(`SELECT count(*) FILTER(WHERE status IN ('pending','running') OR (kind='model_access' AND model_result IN ('unknown','accepted') AND tracking_ended_at IS NULL)) AS active,count(*) FILTER(WHERE dispatch_started_at>now()-interval '1 minute') AS recent FROM catalog.provider_check WHERE provider_id IN ? AND kind<>'model_query'`, ids).Scan(&checks).Error; err != nil {
		return counts, err
	}
	counts.Active += voice.Active + checks.Active
	counts.Recent += voice.Recent + checks.Recent
	return counts, nil
}

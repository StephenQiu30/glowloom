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
	var counts providerUsage
	err := tx.Raw(`SELECT count(DISTINCT op.id) FILTER(WHERE op.status NOT IN ('completed','failed','cancelled','expired')) AS active,
 count(DISTINCT op.id) FILTER(WHERE call.dispatch_started_at>now()-interval '1 minute') AS recent
 FROM operation.provider_credential_binding cb JOIN catalog.provider_credential key ON key.id=cb.credential_id
 JOIN operation.operation op ON op.id=cb.operation_id JOIN operation.provider_call call ON call.operation_id=op.id AND call.action='submit' AND call.dispatch_started_at IS NOT NULL AND NOT call.is_delete
 WHERE key.provider_id=?`, id).Scan(&counts).Error
	if err != nil {
		return counts, err
	}
	var voice, checks providerUsage
	if err := tx.Raw(`SELECT count(*) FILTER(WHERE status IN ('dispatching','unknown')) AS active,count(*) FILTER(WHERE dispatch_started_at>now()-interval '1 minute') AS recent FROM subject.voice_job WHERE provider_id=?`, id).Scan(&voice).Error; err != nil {
		return counts, err
	}
	if err := tx.Raw(`SELECT count(*) FILTER(WHERE status IN ('pending','running') OR (kind='model_access' AND model_result IN ('unknown','accepted') AND tracking_ended_at IS NULL)) AS active,count(*) FILTER(WHERE dispatch_started_at>now()-interval '1 minute') AS recent FROM catalog.provider_check WHERE provider_id=? AND kind<>'model_query'`, id).Scan(&checks).Error; err != nil {
		return counts, err
	}
	counts.Active += voice.Active + checks.Active
	counts.Recent += voice.Recent + checks.Recent
	return counts, nil
}

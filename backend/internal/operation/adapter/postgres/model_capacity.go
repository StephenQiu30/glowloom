package postgres

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// modelHasCapacity runs with the provider row locked. Seedream's native
// quota counts requested images in a rolling minute across account keys.
func modelHasCapacity(tx *gorm.DB, provider, release, operation uuid.UUID, concurrency, rpm int64) (bool, error) {
	var model struct {
		ID, Group       string
		Images          int64
		ImagesPerMinute int64
	}
	r := tx.Raw(`SELECT v.provider_model_id AS id,coalesce(nullif(p.quota_group,''),p.id::text) AS "group",o.output_count AS images,
 coalesce(l.images_per_minute,0) AS images_per_minute
 FROM catalog.provider p JOIN catalog.model_release r ON r.id=? JOIN catalog.model_profile_version v ON v.id=r.model_version_id
 JOIN operation.operation o ON o.id=? LEFT JOIN catalog.model_image_limit l ON l.adapter_key=p.adapter_key AND l.provider_model_id=v.provider_model_id WHERE p.id=?`, release, operation, provider).Scan(&model)
	if r.Error != nil || r.RowsAffected != 1 {
		return false, r.Error
	}
	if model.ImagesPerMinute == 0 {
		u, err := readProviderUsage(tx, provider)
		return err == nil && u.Active < concurrency && u.Recent < rpm, err
	}
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "image-quota/"+model.Group+"/"+model.ID).Error; err != nil {
		return false, err
	}
	var used int64
	err := tx.Raw(`WITH peers AS (SELECT peer.id FROM catalog.provider peer JOIN catalog.provider own ON own.id=? WHERE peer.adapter_key=own.adapter_key AND (peer.id=own.id OR (own.quota_group<>'' AND peer.quota_group=own.quota_group))),
 sends AS (
 SELECT c.operation_id,c.dispatch_started_at AS sent,coalesce(d.release_id,b.release_id) AS release_id,k.provider_id
 FROM operation.provider_call c JOIN operation.provider_binding b ON b.operation_id=c.operation_id
 JOIN operation.provider_credential_binding cb ON cb.operation_id=c.operation_id JOIN catalog.provider_credential k ON k.id=cb.credential_id
 LEFT JOIN operation.route_delivery d ON d.operation_id=c.operation_id
 WHERE c.dispatch_started_at>clock_timestamp()-interval '1 minute' AND c.action='submit' AND NOT c.is_delete
 AND NOT EXISTS(SELECT 1 FROM operation.route_retry retry WHERE retry.operation_id=c.operation_id)
 UNION ALL SELECT retry.operation_id,retry.created_at,retry.release_id,k.provider_id FROM operation.route_retry retry JOIN catalog.provider_credential k ON k.id=retry.credential_id WHERE retry.created_at>clock_timestamp()-interval '1 minute')
 SELECT coalesce(sum(o.output_count),0) FROM sends JOIN peers ON peers.id=sends.provider_id
 JOIN operation.operation o ON o.id=sends.operation_id JOIN catalog.model_release r ON r.id=sends.release_id
 JOIN catalog.model_profile_version v ON v.id=r.model_version_id WHERE v.provider_model_id=?`, provider, model.ID).Row().Scan(&used)
	return err == nil && used+model.Images <= model.ImagesPerMinute, err
}

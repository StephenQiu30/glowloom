package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ReadRoutePool returns the current configuration and compatible, verified alternatives.
func (s *Store) ReadRoutePool(ctx context.Context, actor identity.Principal, id uuid.UUID) (out application.RoutePool, err error) {
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.requirePlatformAdmin(tx, actor); e != nil {
			return e
		}
		var e error
		out, e = routePool(tx, id)
		return e
	})
	return
}

func routePool(tx *gorm.DB, id uuid.UUID) (application.RoutePool, error) {
	out := application.RoutePool{Backups: []uuid.UUID{}}
	var version struct {
		ID       uuid.UUID
		Revision int
	}
	if err := tx.Raw("SELECT id,revision FROM catalog.route_pool_version WHERE primary_release_id=? ORDER BY revision DESC LIMIT 1", id).Scan(&version).Error; err != nil {
		return out, err
	}
	out.Revision = version.Revision
	if version.ID != uuid.Nil {
		var members []struct{ ReleaseID uuid.UUID }
		if err := tx.Raw("SELECT release_id FROM catalog.route_pool_member WHERE version_id=? AND priority>0 ORDER BY priority", version.ID).Scan(&members).Error; err != nil {
			return out, err
		}
		for _, m := range members {
			out.Backups = append(out.Backups, m.ReleaseID)
		}
	}
	if err := tx.Raw(`WITH last_jobs AS (
 SELECT d.operation_id,d.release_id,d.created_at FROM operation.route_delivery d
 JOIN operation.provider_binding original ON original.operation_id=d.operation_id AND original.release_id=?
 ORDER BY d.created_at DESC LIMIT 10
 ), attempts AS (
 SELECT j.operation_id,j.created_at,1 AS attempt,j.release_id,k.credential_id,
 CASE WHEN retry.operation_id IS NOT NULL THEN 'dial_not_sent' ELSE o.status END AS reason
 FROM last_jobs j JOIN operation.provider_credential_binding k ON k.operation_id=j.operation_id
 JOIN operation.operation o ON o.id=j.operation_id LEFT JOIN operation.route_retry retry ON retry.operation_id=j.operation_id
 UNION ALL SELECT j.operation_id,j.created_at,2,retry.release_id,retry.credential_id,o.status FROM last_jobs j
 JOIN operation.route_retry retry ON retry.operation_id=j.operation_id JOIN operation.operation o ON o.id=j.operation_id)
 SELECT a.operation_id,a.attempt,a.reason,p.name AS provider_name,k.last4 AS key_suffix
 FROM attempts a JOIN catalog.model_release r ON r.id=a.release_id JOIN catalog.model_profile m ON m.id=r.model_id
 JOIN catalog.provider p ON p.id=m.provider_id JOIN catalog.provider_credential k ON k.id=a.credential_id
 ORDER BY a.created_at DESC,a.operation_id,a.attempt`, id).Scan(&out.Recent).Error; err != nil {
		return out, err
	}
	var rows []struct {
		application.RouteOption `gorm:"embedded"`
		ModelID                 uuid.UUID
	}
	if err := tx.Raw("SELECT c.release_id,c.model_id,c.provider_name,c.provider_model_id,c.display_name FROM catalog.route_pool_candidate c JOIN catalog.model_profile candidate ON candidate.id=c.model_id AND candidate.active_release_id=c.release_id JOIN catalog.model_profile primary_model ON primary_model.active_release_id=c.primary_release_id WHERE c.primary_release_id=? AND c.provider_id<>primary_model.provider_id AND EXISTS(SELECT 1 FROM catalog.model_release r JOIN catalog.model_profile_version mv ON mv.id=r.model_version_id WHERE r.id=c.primary_release_id AND COALESCE(mv.limits->>'provider_endpoint','')='') ORDER BY c.provider_name,c.release_id", id).Scan(&rows).Error; err != nil {
		return out, err
	}
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ModelID)
	}
	available, err := readOwnModelAvailability(tx, ids)
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		if available[r.ModelID].Available {
			out.Options = append(out.Options, r.RouteOption)
		}
	}
	return out, nil
}

// PublishRoutePool appends a complete immutable version with a single audit receipt.
func (s *Store) PublishRoutePool(ctx context.Context, actor identity.Principal, id uuid.UUID, in application.PoolPublication) (application.RoutePool, error) {
	if id == uuid.Nil || in.RequestID == uuid.Nil || in.ExpectedRevision < 0 || len(in.Backups) > 15 || len(in.Backups) > 0 && !in.Equivalent {
		return application.RoutePool{}, domain.ErrInvalidPreset
	}
	return catalogMutation(ctx, s.db, s.actors, actor, "route_pool.publish", id, in.RequestID, in, func(tx *gorm.DB) (application.RoutePool, error) {
		var primary struct{ ID uuid.UUID }
		r := tx.Raw("SELECT r.id FROM catalog.model_release r JOIN catalog.model_profile m ON m.active_release_id=r.id WHERE r.id=? AND r.status='active' AND m.status='active' AND NOT m.is_delete FOR UPDATE OF r", id).Scan(&primary)
		if r.Error != nil {
			return application.RoutePool{}, r.Error
		}
		if primary.ID != id {
			return application.RoutePool{}, application.ErrReleaseUnverified
		}
		out, err := routePool(tx, id)
		if err != nil {
			return out, err
		}
		if out.Revision != in.ExpectedRevision {
			return out, application.ErrPresetConflict
		}
		allowed := map[uuid.UUID]bool{}
		for _, o := range out.Options {
			allowed[o.ReleaseID] = true
		}
		for _, b := range in.Backups {
			if !allowed[b] {
				return out, application.ErrReleaseUnverified
			}
			delete(allowed, b)
		}
		version := uuid.New()
		out.Revision++
		out.Backups = append([]uuid.UUID{}, in.Backups...)
		if err := tx.Exec("INSERT INTO catalog.route_pool_version(id,primary_release_id,revision,created_by) VALUES(?,?,?,?)", version, id, out.Revision, actor.ID).Error; err != nil {
			return out, err
		}
		for priority, member := range append([]uuid.UUID{id}, in.Backups...) {
			if err := tx.Exec("INSERT INTO catalog.route_pool_member(version_id,release_id,priority) VALUES(?,?,?)", version, member, priority).Error; err != nil {
				return out, err
			}
		}
		return out, nil
	})
}

// SetPoolKey admits or drains a credential on its original connection. New
// dispatches must still prove release- and mode-specific execution evidence.
func (s *Store) SetPoolKey(ctx context.Context, actor identity.Principal, providerID, keyID uuid.UUID, enabled bool, request uuid.UUID) (application.ProviderCredentialSummary, error) {
	if providerID == uuid.Nil || keyID == uuid.Nil || request == uuid.Nil {
		return application.ProviderCredentialSummary{}, domain.ErrInvalidPreset
	}
	input := struct {
		Provider uuid.UUID
		Enabled  bool
	}{providerID, enabled}
	return catalogMutation(ctx, s.db, s.actors, actor, "route_pool.key", keyID, request, input, func(tx *gorm.DB) (application.ProviderCredentialSummary, error) {
		var row credentialDisplayRow
		r := tx.Raw(`UPDATE catalog.provider_credential SET pool_enabled=?, update_time=now()
 WHERE provider_id=? AND id=? AND status='draining' AND NOT is_delete
 AND (NOT ? OR EXISTS(SELECT 1 FROM catalog.model_validation x WHERE x.credential_id=? AND x.status='succeeded' AND x.evidence->>'kind'='real_provider'))
 RETURNING id,provider_id,label,last4,status,pool_enabled,last_tested_at,last_test_result,create_time,update_time`, enabled, providerID, keyID, enabled, keyID).Scan(&row)
		if r.Error != nil {
			return application.ProviderCredentialSummary{}, r.Error
		}
		if r.RowsAffected != 1 {
			return application.ProviderCredentialSummary{}, application.ErrReleaseUnverified
		}
		return *row.summary(), nil
	})
}

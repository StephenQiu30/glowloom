package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func (s *Store) PreviewProviderDeletion(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (application.ProviderDeletionPreview, error) {
	var preview application.ProviderDeletionPreview
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requirePlatformAdmin(tx, actor); err != nil {
			return err
		}
		var err error
		preview, err = providerDeletionPreview(tx, id, false)
		return err
	})
	return preview, err
}

func (s *Store) DeleteProvider(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input application.DeleteProviderInput) (application.DeletedProvider, error) {
	return catalogMutation(ctx, s.db, s.actors, actor, "provider.delete", id, input.RequestID, input, func(tx *gorm.DB) (application.DeletedProvider, error) {
		preview, err := providerDeletionPreview(tx, id, true)
		if err != nil {
			return application.DeletedProvider{}, err
		}
		if preview.BlockedReason != "" || preview.Token != input.Token {
			return application.DeletedProvider{}, domain.ErrProviderRevisionConflict
		}
		if err := tx.Exec(`UPDATE catalog.model_profile SET is_delete=true, revision=revision+1, update_time=now()
WHERE provider_id=? AND NOT is_delete`, id).Error; err != nil {
			return application.DeletedProvider{}, err
		}
		if err := tx.Exec(`UPDATE catalog.provider_credential SET status='disabled', update_time=now()
WHERE provider_id=? AND NOT is_delete AND status<>'disabled'`, id).Error; err != nil {
			return application.DeletedProvider{}, err
		}
		if err := tx.Exec(`UPDATE catalog.provider SET is_delete=true, revision=revision+1, update_time=now() WHERE id=?`, id).Error; err != nil {
			return application.DeletedProvider{}, err
		}
		return application.DeletedProvider{ID: id}, nil
	})
}

func providerDeletionPreview(tx *gorm.DB, id uuid.UUID, write bool) (application.ProviderDeletionPreview, error) {
	preview := application.ProviderDeletionPreview{Drafts: []application.ProviderDeletionDraft{}}
	var provider struct {
		Status   domain.ProviderStatus
		Revision int64
	}
	providerQuery := `SELECT status, revision FROM catalog.provider WHERE id=? AND NOT is_delete`
	draftsQuery := `SELECT id, display_name AS name, revision FROM catalog.model_profile
WHERE provider_id=? AND NOT is_delete ORDER BY id`
	if write {
		// Provider locks serialize model creation and credential/test writes.
		// Model locks serialize version, price and release writes.
		providerQuery += " FOR UPDATE"
		draftsQuery += " FOR UPDATE"
	}
	r := tx.Raw(providerQuery, id).Scan(&provider)
	if r.Error != nil {
		return preview, r.Error
	}
	if r.RowsAffected != 1 {
		return preview, application.ErrProviderNotFound
	}
	preview.Revision = provider.Revision
	if provider.Status != domain.ProviderDisabled && provider.Status != domain.ProviderArchived {
		preview.BlockedReason = "active"
		return preview, nil
	}
	var drafts []application.ProviderDeletionDraft
	if err := tx.Raw(draftsQuery, id).Scan(&drafts).Error; err != nil {
		return preview, err
	}

	var references struct {
		Tasks, Checks, Models bool
	}
	if err := tx.Raw(`
WITH models AS (SELECT * FROM catalog.model_profile WHERE provider_id=@id),
     releases AS (SELECT r.* FROM catalog.model_release r JOIN models m ON m.id=r.model_id),
     versions AS (SELECT v.id FROM catalog.model_profile_version v JOIN models m ON m.id=v.model_profile_id)
SELECT
  (EXISTS (SELECT 1 FROM operation.operation o JOIN versions v ON v.id=o.model_profile_version_id)
   OR EXISTS (SELECT 1 FROM operation.provider_binding b JOIN releases r ON r.id=b.release_id)
   OR EXISTS (SELECT 1 FROM operation.provider_credential_binding b
              JOIN catalog.provider_credential c ON c.id=b.credential_id WHERE c.provider_id=@id)
   OR EXISTS (SELECT 1 FROM operation.batch_launch WHERE provider_id=@id)
   OR EXISTS (SELECT 1 FROM subject.voice_job WHERE provider_id=@id)) AS tasks,
  EXISTS (SELECT 1 FROM catalog.provider_check WHERE provider_id=@id
          AND (status IN ('pending','running') OR
               (kind='model_access' AND (dispatch_started_at IS NOT NULL OR task_id<>'' OR model_result IN ('unknown','accepted'))))) AS checks,
  (EXISTS (SELECT 1 FROM models WHERE status='active' OR active_release_id IS NOT NULL
           OR (current_version_id IS NOT NULL AND candidate_snapshot IS NULL))
   OR EXISTS (SELECT 1 FROM releases WHERE status<>'draft' OR activated_at IS NOT NULL OR upstream_cost<>'{}'::jsonb)
   OR EXISTS (SELECT 1 FROM workspace.project p JOIN models m ON p.default_models->>m.capability=m.model_key)
   OR EXISTS (SELECT 1 FROM catalog.capability_default d JOIN releases r ON r.id=d.release_id)
   OR EXISTS (SELECT 1 FROM catalog.model_validation v JOIN releases r ON r.id=v.release_id)) AS models
`, map[string]any{"id": id}).Scan(&references).Error; err != nil {
		return preview, err
	}
	switch {
	case references.Tasks:
		preview.BlockedReason = "task_history"
	case references.Checks:
		preview.BlockedReason = "check_history"
	case references.Models:
		preview.BlockedReason = "model_reference"
	default:
		if len(drafts) > 0 {
			preview.Drafts = drafts
		}
		data, err := json.Marshal(preview)
		if err != nil {
			return preview, err
		}
		hash := sha256.New()
		hash.Write(id[:])
		hash.Write(data)
		preview.Token = hex.EncodeToString(hash.Sum(nil))
	}
	return preview, nil
}

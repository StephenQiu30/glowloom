package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

// EstimateArkVideo reads the reviewed public price without opening a route or touching any wallet.
func (s *Store) EstimateArkVideo(ctx context.Context, in domain.ArkVideoEstimateInput) (domain.ArkVideoEstimate, error) {
	var out domain.ArkVideoEstimate
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, s.actor.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		var row struct{ Profile []byte }
		result := tx.Raw(`SELECT video_estimate AS profile FROM catalog.model_price_preset
WHERE adapter_key = 'volcengine_ark' AND provider_model_id = ? AND video_estimate IS NOT NULL`, in.Model).Scan(&row)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return domain.ErrInvalid
		}
		var profile domain.ArkVideoEstimateProfile
		if err := json.Unmarshal(row.Profile, &profile); err != nil {
			return err
		}
		estimate, err := profile.Estimate(in)
		if err != nil {
			return err
		}
		out = estimate
		return nil
	})
	return out, err
}

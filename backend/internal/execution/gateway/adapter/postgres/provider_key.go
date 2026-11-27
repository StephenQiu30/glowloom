package postgres

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

type keyVerification struct {
	route       domain.Route
	fingerprint string
}

func keyOnlyPlan() application.OnboardingPlan {
	return application.OnboardingPlan{Method: application.ReadOnlyKeyVerification, Probes: []application.Probe{}}
}

func requireConfirmedKey(tx *gorm.DB, id uuid.UUID, fingerprint string) error {
	var same bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM ai.onboarding WHERE id=? AND state='active' AND key_check_fingerprint=?)`, id, fingerprint).Row().Scan(&same); err != nil {
		return err
	}
	if !same {
		return domain.ErrConflict
	}
	return nil
}

// ConfirmProviderKey verifies the key once per provider and address, then
// rechecks the scoped onboarding contract before enabling this route.
func (s *Store) ConfirmProviderKey(ctx context.Context, id uuid.UUID, credits, purchase int64, macKey []byte, key string, verify func(context.Context, domain.Route, string) error, seal func(uuid.UUID, uuid.UUID) ([]byte, string, error)) (uuid.UUID, error) {
	if credits != 0 || purchase != 0 {
		return uuid.Nil, domain.ErrBudget
	}
	if len(macKey) != sha256.Size || verify == nil {
		return uuid.Nil, domain.ErrUnavailable
	}
	var checked keyVerification
	var endpoint application.KeyEndpoint
	var active bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, s.actor.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		row, work, err := loadOnboarding(tx, id, false)
		if err != nil {
			return err
		}
		if err := s.check(ctx, tx, work.Input.Scope, work.Input.Project, false); err != nil {
			return err
		}
		if !work.Plan.ReadOnlyKey() {
			return domain.ErrInvalid
		}
		checked.route = work.Input.Route
		var ok bool
		endpoint, ok = application.KeyEndpointFor(checked.route)
		if !ok {
			return domain.ErrProviderKeyCheckUnsupported
		}
		mac := hmac.New(sha256.New, macKey)
		_, _ = mac.Write([]byte("glowloom/ai-provider-key/v1\x00" + endpoint.Provider + "\x00" + endpoint.BaseURL + "\x00" + key))
		checked.fingerprint = hex.EncodeToString(mac.Sum(nil))
		if row.State == "active" {
			active = true
			return requireConfirmedKey(tx, id, checked.fingerprint)
		}
		if row.State != "prepared" {
			return domain.ErrConflict
		}
		return checkPrevious(tx, row, work.Model)
	})
	if err != nil || active {
		return id, err
	}
	if err := s.verifyKeyOnce(ctx, endpoint, checked.fingerprint, func(ctx context.Context) error {
		return verify(ctx, checked.route, key)
	}); err != nil {
		return uuid.Nil, err
	}
	return s.confirmOnboarding(ctx, id, credits, purchase, seal, &checked)
}

func (s *Store) verifyKeyOnce(ctx context.Context, endpoint application.KeyEndpoint, fingerprint string, verify func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Hold only the per-key advisory lock during the bounded read.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "ai/provider-key/"+fingerprint).Error; err != nil {
			return err
		}
		var exists bool
		if err := tx.Raw("SELECT EXISTS(SELECT 1 FROM ai.provider_key_verification WHERE fingerprint=?)", fingerprint).Row().Scan(&exists); err != nil || exists {
			return err
		}
		if err := verify(ctx); err != nil {
			return err
		}
		return tx.Exec("INSERT INTO ai.provider_key_verification(fingerprint,provider,base_url) VALUES(?,?,?)", fingerprint, endpoint.Provider, endpoint.BaseURL).Error
	})
}

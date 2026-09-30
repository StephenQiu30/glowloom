package postgres

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// EnsureWorkspace provisions a single durable producer for the host-only
// workspace. Existing identity or organization state is never repaired or reset.
// The composition root exposes this workspace only in the local environment.
func (s *Store) EnsureWorkspace(ctx context.Context) (domain.User, error) {
	if s == nil || s.db == nil {
		return domain.User{}, ErrInvalidUser
	}
	userID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://lanverse.local/workspace/user"))
	var user domain.User
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked int
		// Share the organization bootstrap lock with administrator initialization.
		if err := tx.Raw("SELECT 1 FROM pg_advisory_xact_lock(69210, 1)").Scan(&locked).Error; err != nil {
			return fmt.Errorf("lock local workspace: %w", err)
		}
		store := NewStore(tx)
		orgID, err := store.FindBootstrapOrganization(ctx)
		if err != nil {
			return fmt.Errorf("resolve local organization: %w", err)
		}
		var existing struct{ OrgID uuid.UUID }
		row := tx.Raw(`SELECT org_id FROM identity."user" WHERE id = ?::uuid`, userID.String()).Scan(&existing)
		if row.Error != nil {
			return fmt.Errorf("read local workspace identity: %w", row.Error)
		}
		if row.RowsAffected != 0 {
			if orgID == uuid.Nil || existing.OrgID != orgID {
				return application.ErrForbidden
			}
			user, err = store.FindByID(ctx, orgID, userID)
			if errors.Is(err, domain.ErrUserNotFound) {
				return application.ErrForbidden
			}
			if err != nil {
				return err
			}
			if user.LoginName != "local-workspace" || user.Role != domain.RoleProducer ||
				user.Status != domain.StatusActive || user.MustChangePassword || !domain.ValidPasswordHash(user.PasswordHash) {
				return application.ErrForbidden
			}
			return nil
		}
		if orgID == uuid.Nil {
			orgID = uuid.New()
			if err := tx.Exec(`INSERT INTO workspace.organization (id, name) VALUES (?::uuid, 'Lanverse')`, orgID.String()).Error; err != nil {
				return fmt.Errorf("create local workspace organization: %w", err)
			}
		}
		// Only a one-way hash is stored. The random value is never returned,
		// configured or logged, and there is no known initial login password.
		var entropy [32]byte
		if _, err := rand.Read(entropy[:]); err != nil {
			return fmt.Errorf("generate local workspace identity: %w", err)
		}
		hash, err := domain.HashPassword("Local0"+base64.RawURLEncoding.EncodeToString(entropy[:]), "")
		if err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO identity."user"
			  (id, org_id, login_name, display_name, role, password_hash, must_change_password)
			VALUES (?::uuid, ?::uuid, 'local-workspace', '本机工作区', 'producer', ?, false)
		`, userID.String(), orgID.String(), hash).Error; err != nil {
			return fmt.Errorf("create local workspace identity: %w", err)
		}
		user, err = store.FindByID(ctx, orgID, userID)
		return err
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("resolve local workspace transaction: %w", err)
	}
	return user, nil
}

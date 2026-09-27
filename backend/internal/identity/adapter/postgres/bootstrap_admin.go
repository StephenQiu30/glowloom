package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

type bootstrapOrganization struct {
	ID     uuid.UUID
	Status string
}

// FindBootstrapOrganization returns the single active MVP organization or a
// nil UUID when it has not yet been created. The transaction rechecks it.
func (s *Store) FindBootstrapOrganization(ctx context.Context) (uuid.UUID, error) {
	if s == nil || s.db == nil {
		return uuid.Nil, ErrInvalidUser
	}
	var organizations []bootstrapOrganization
	if err := s.db.WithContext(ctx).Raw(`
		SELECT id, status FROM workspace.organization
		WHERE NOT is_delete ORDER BY id LIMIT 2
	`).Scan(&organizations).Error; err != nil {
		return uuid.Nil, fmt.Errorf("find bootstrap organization: %w", err)
	}
	if len(organizations) > 1 ||
		(len(organizations) == 1 && organizations[0].Status != "active") {
		return uuid.Nil, application.ErrBootstrapOrganizationConflict
	}
	if len(organizations) == 0 {
		return uuid.Nil, nil
	}
	return organizations[0].ID, nil
}

// BootstrapAdminWithEvents serializes first-account attempts and commits the
// organization, administrator, and two events as one PostgreSQL transaction.
func (s *Store) BootstrapAdminWithEvents(ctx context.Context, orgName string, user domain.User, events []application.OutboxEvent) (domain.User, error) {
	if s == nil || s.db == nil || user.ID == uuid.Nil || user.OrgID == uuid.Nil ||
		strings.TrimSpace(orgName) == "" || user.Role != domain.RoleAdmin ||
		user.Status != domain.StatusActive || !user.MustChangePassword ||
		!domain.ValidPasswordHash(user.PasswordHash) || !validCreateEvents(user.OrgID, events) {
		return domain.User{}, ErrInvalidUser
	}
	var created domain.User
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked int
		if err := tx.Raw("SELECT 1 FROM pg_advisory_xact_lock(69210, 1)").Scan(&locked).Error; err != nil {
			return fmt.Errorf("lock administrator bootstrap: %w", err)
		}
		var existing bool
		if err := tx.Raw(`SELECT EXISTS (SELECT 1 FROM identity."user" WHERE NOT is_delete)`).Scan(&existing).Error; err != nil {
			return fmt.Errorf("check existing accounts: %w", err)
		}
		if existing {
			return application.ErrAlreadyBootstrapped
		}
		var organizations []bootstrapOrganization
		if err := tx.Raw(`
			SELECT id, status FROM workspace.organization
			WHERE NOT is_delete ORDER BY id LIMIT 2 FOR UPDATE
		`).Scan(&organizations).Error; err != nil {
			return fmt.Errorf("lock bootstrap organization: %w", err)
		}
		if len(organizations) > 1 ||
			(len(organizations) == 1 && (organizations[0].ID != user.OrgID || organizations[0].Status != "active")) {
			return application.ErrBootstrapOrganizationConflict
		}
		if len(organizations) == 0 {
			inserted := tx.Exec(`
				INSERT INTO workspace.organization (id, name) VALUES (?::uuid, ?)
			`, user.OrgID.String(), orgName)
			if inserted.Error != nil {
				return fmt.Errorf("create bootstrap organization: %w", inserted.Error)
			}
			if inserted.RowsAffected != 1 {
				return fmt.Errorf("create bootstrap organization: inserted %d rows", inserted.RowsAffected)
			}
		}
		transactionStore := NewStore(tx)
		if err := transactionStore.Create(ctx, user); err != nil {
			return err
		}
		if err := insertOutboxEvents(tx, events); err != nil {
			return err
		}
		var err error
		created, err = transactionStore.FindByID(ctx, user.OrgID, user.ID)
		if err != nil {
			return fmt.Errorf("read bootstrap administrator: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("bootstrap administrator transaction: %w", err)
	}
	return created, nil
}

var _ application.BootstrapAdminStore = (*Store)(nil)

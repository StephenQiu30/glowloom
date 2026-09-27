// Package postgres persists organization-scoped identity accounts.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

var (
	// ErrInvalidUser means a new account is missing a required identity or hash.
	ErrInvalidUser = errors.New("invalid new user")
	// ErrLoginExists means the organization already has this case-insensitive login.
	ErrLoginExists = errors.New("login name already exists")
	// ErrNotFound means no active account is visible in the organization.
	ErrNotFound = domain.ErrUserNotFound
	// ErrRevisionConflict means an account changed after the caller read it.
	ErrRevisionConflict = domain.ErrRevisionConflict
)

// Store keeps identity queries scoped to one organization.
type Store struct {
	db *gorm.DB
}

// NewStore injects the database handle for identity accounts.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// Create inserts an account with the migration's active, first-login defaults.
// The caller must provide a bcrypt hash, never a plaintext password.
func (s *Store) Create(ctx context.Context, user domain.User) error {
	if user.ID == uuid.Nil || user.OrgID == uuid.Nil ||
		strings.TrimSpace(user.LoginName) == "" || strings.TrimSpace(user.DisplayName) == "" ||
		!domain.ValidPasswordHash(user.PasswordHash) ||
		(user.Role != domain.RoleAdmin && user.Role != domain.RoleProducer) {
		return ErrInvalidUser
	}
	result := s.db.WithContext(ctx).Exec(`
		INSERT INTO identity."user" (id, org_id, login_name, display_name, role, password_hash)
		VALUES (?::uuid, ?::uuid, ?, ?, ?, ?)
	`, user.ID.String(), user.OrgID.String(), user.LoginName, user.DisplayName, string(user.Role), user.PasswordHash)
	if result.Error != nil {
		var pgErr *pgconn.PgError
		if errors.As(result.Error, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_user_login_name" {
			return fmt.Errorf("create user: %w", ErrLoginExists)
		}
		return fmt.Errorf("create user: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("create user: inserted %d rows", result.RowsAffected)
	}
	return nil
}

// FindByLogin returns a non-deleted record within orgID, using citext comparison.
func (s *Store) FindByLogin(ctx context.Context, orgID uuid.UUID, loginName string) (domain.User, error) {
	return s.find(ctx, orgID, false, loginName)
}

// FindByID returns a non-deleted record only in the supplied organization.
func (s *Store) FindByID(ctx context.Context, orgID, userID uuid.UUID) (domain.User, error) {
	return s.find(ctx, orgID, true, userID.String())
}

// SaveLoginState persists a verified login attempt with optimistic concurrency.
// A stale attempt must be retried from a fresh account read.
func (s *Store) SaveLoginState(ctx context.Context, user domain.User, expectedRevision int64) error {
	if user.ID == uuid.Nil || user.OrgID == uuid.Nil || expectedRevision < 1 ||
		user.Revision != expectedRevision+1 || user.FailedLoginCount < 0 {
		return ErrInvalidUser
	}
	var lockedUntil, lastLoginAt any
	if !user.LockedUntil.IsZero() {
		lockedUntil = user.LockedUntil.UTC()
	}
	if !user.LastLoginAt.IsZero() {
		lastLoginAt = user.LastLoginAt.UTC()
	}
	result := s.db.WithContext(ctx).Exec(`
		UPDATE identity."user"
		SET failed_login_count = ?, locked_until = ?, last_login_at = ?,
		    revision = revision + 1, update_time = now()
		WHERE org_id = ?::uuid AND id = ?::uuid AND revision = ?
		  AND status = 'active' AND NOT is_delete
	`, user.FailedLoginCount, lockedUntil, lastLoginAt, user.OrgID.String(), user.ID.String(), expectedRevision)
	if result.Error != nil {
		return fmt.Errorf("save login state: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	return nil
}

// Disable deactivates an account and invalidates its sessions. An organization
// lock serializes concurrent admin removals so one active admin always remains.
func (s *Store) Disable(ctx context.Context, orgID, userID uuid.UUID, expectedRevision int64) error {
	if orgID == uuid.Nil || userID == uuid.Nil {
		return ErrNotFound
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAccountChangeOrg(tx, orgID); err != nil {
			return err
		}
		_, err := disableUserInTx(tx, orgID, userID, expectedRevision)
		return err
	})
}

func lockAccountChangeOrg(tx *gorm.DB, orgID uuid.UUID) error {
	var locked int
	if err := tx.Raw("SELECT 1 FROM pg_advisory_xact_lock(69209, hashtext(?))", orgID.String()).Scan(&locked).Error; err != nil {
		return fmt.Errorf("lock organization admin changes: %w", err)
	}
	return nil
}

func disableUserInTx(tx *gorm.DB, orgID, userID uuid.UUID, expectedRevision int64) (domain.User, error) {
	var state struct {
		Role         string
		Status       string
		SessionEpoch int64
		Revision     int64
	}
	result := tx.Raw(`
		SELECT role, status, session_epoch, revision
		FROM identity."user"
		WHERE org_id = ?::uuid AND id = ?::uuid AND NOT is_delete
		FOR UPDATE
	`, orgID.String(), userID.String()).Scan(&state)
	if result.Error != nil {
		return domain.User{}, fmt.Errorf("read account for disable: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.User{}, ErrNotFound
	}
	if state.Revision != expectedRevision {
		return domain.User{}, ErrRevisionConflict
	}
	var activeAdmins int64
	if err := tx.Raw(`
		SELECT count(*) FROM identity."user"
		WHERE org_id = ?::uuid AND role = 'admin' AND status = 'active' AND NOT is_delete
	`, orgID.String()).Scan(&activeAdmins).Error; err != nil {
		return domain.User{}, fmt.Errorf("count active administrators: %w", err)
	}
	user := domain.User{
		ID: userID, OrgID: orgID, Role: domain.Role(state.Role), Status: domain.Status(state.Status),
		SessionEpoch: state.SessionEpoch, Revision: state.Revision,
	}
	if err := user.Disable(int(activeAdmins)); err != nil {
		return domain.User{}, err
	}
	updated := tx.Exec(`
		UPDATE identity."user"
		SET status = ?, session_epoch = ?, revision = revision + 1, update_time = now()
		WHERE org_id = ?::uuid AND id = ?::uuid AND revision = ? AND NOT is_delete
	`, string(user.Status), user.SessionEpoch, orgID.String(), userID.String(), expectedRevision)
	if updated.Error != nil {
		return domain.User{}, fmt.Errorf("disable account: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return domain.User{}, ErrRevisionConflict
	}
	return user, nil
}

func (s *Store) find(ctx context.Context, orgID uuid.UUID, byID bool, value string) (domain.User, error) {
	if orgID == uuid.Nil {
		return domain.User{}, ErrNotFound
	}
	const selectUser = `
		SELECT id, org_id, login_name, display_name, role, status, password_hash,
		       must_change_password, password_changed_at, failed_login_count,
		       locked_until, session_epoch, last_login_at, revision, create_time, update_time
		FROM identity."user"
		WHERE org_id = ?::uuid AND NOT is_delete
	`
	query := selectUser + " AND login_name = ?"
	if byID {
		query = selectUser + " AND id = ?::uuid"
	}
	var row userRow
	result := s.db.WithContext(ctx).Raw(query, orgID.String(), value).Scan(&row)
	if result.Error != nil {
		return domain.User{}, fmt.Errorf("find user: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.User{}, ErrNotFound
	}
	return row.domainUser(), nil
}

type userRow struct {
	ID                 uuid.UUID
	OrgID              uuid.UUID
	LoginName          string
	DisplayName        string
	Role               string
	Status             string
	PasswordHash       string
	MustChangePassword bool
	PasswordChangedAt  *time.Time
	FailedLoginCount   int
	LockedUntil        *time.Time
	SessionEpoch       int64
	LastLoginAt        *time.Time
	Revision           int64
	CreateTime         time.Time
	UpdateTime         time.Time
}

func (r userRow) domainUser() domain.User {
	user := domain.User{
		ID: r.ID, OrgID: r.OrgID, LoginName: r.LoginName, DisplayName: r.DisplayName,
		Role: domain.Role(r.Role), Status: domain.Status(r.Status), PasswordHash: r.PasswordHash,
		MustChangePassword: r.MustChangePassword, FailedLoginCount: r.FailedLoginCount,
		SessionEpoch: r.SessionEpoch, Revision: r.Revision, CreateTime: r.CreateTime, UpdateTime: r.UpdateTime,
	}
	if r.PasswordChangedAt != nil {
		user.PasswordChangedAt = *r.PasswordChangedAt
	}
	if r.LockedUntil != nil {
		user.LockedUntil = *r.LockedUntil
	}
	if r.LastLoginAt != nil {
		user.LastLoginAt = *r.LastLoginAt
	}
	return user
}

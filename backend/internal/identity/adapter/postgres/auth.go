package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// Transaction commits authentication state and safe events together.
func (s *Store) Transaction(ctx context.Context, execute func(application.AuthTx) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return execute(&authTx{db: tx}) })
}

// LoginAvailable checks the global, case-insensitive account namespace.
func (s *Store) LoginAvailable(ctx context.Context, key string) (bool, error) {
	var exists bool
	err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM identity."user" WHERE lower(btrim(login_name::text)) = ? AND NOT is_delete)`, key).Scan(&exists).Error
	if err != nil {
		return false, fmt.Errorf("check login availability: %w", err)
	}
	return !exists, nil
}

type authTx struct{ db *gorm.DB }

func (t *authTx) RegistrationOrganization() (uuid.UUID, error) {
	if err := t.db.Exec(`SELECT pg_advisory_xact_lock(69210, 1)`).Error; err != nil {
		return uuid.Nil, fmt.Errorf("lock registration scope: %w", err)
	}
	id, err := NewStore(t.db).FindBootstrapOrganization(t.db.Statement.Context)
	if err != nil {
		return uuid.Nil, err
	}
	if id != uuid.Nil {
		return id, nil
	}
	id = uuid.New()
	if err := t.db.Exec(`INSERT INTO workspace.organization(id,name) VALUES(?::uuid,'本机工作区')`, id.String()).Error; err != nil {
		return uuid.Nil, fmt.Errorf("create registration scope: %w", err)
	}
	return id, nil
}

func (t *authTx) CreateAccount(u domain.User) error {
	err := t.db.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,status,password_hash,must_change_password,password_changed_at,last_login_at)
 VALUES(?::uuid,?::uuid,?,?,?,'active',?,false,?,?)`, u.ID.String(), u.OrgID.String(), u.LoginName, u.DisplayName, string(u.Role), u.PasswordHash, u.PasswordChangedAt, u.PasswordChangedAt).Error
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && (pgErr.ConstraintName == "uq_user_login_global" || pgErr.ConstraintName == "uq_user_login_name") {
		return application.ErrLoginTaken
	}
	if err != nil {
		return fmt.Errorf("insert registered account: %w", err)
	}
	return nil
}

func (t *authTx) AccountByLogin(key string) (domain.User, error) {
	return t.account(`lower(btrim(login_name::text)) = ?`, key)
}
func (t *authTx) AccountByID(id uuid.UUID) (domain.User, error) {
	return t.account(`id = ?::uuid`, id.String())
}
func (t *authTx) account(where string, value any) (domain.User, error) {
	var row userRow
	r := t.db.Raw(`SELECT * FROM identity."user" WHERE NOT is_delete AND `+where+` FOR UPDATE`, value).Scan(&row)
	if r.Error != nil {
		return domain.User{}, fmt.Errorf("lock authentication account: %w", r.Error)
	}
	if r.RowsAffected == 0 {
		return domain.User{}, domain.ErrUserNotFound
	}
	return row.domainUser(), nil
}
func (t *authTx) UpdateAccount(u domain.User) error {
	r := t.db.Exec(`UPDATE identity."user" SET password_hash=?,must_change_password=?,password_changed_at=?,session_epoch=?,revision=?,failed_login_count=?,locked_until=?,last_login_at=?,update_time=now() WHERE id=?::uuid AND NOT is_delete`, u.PasswordHash, u.MustChangePassword, nullableTime(u.PasswordChangedAt), u.SessionEpoch, u.Revision, u.FailedLoginCount, nullableTime(u.LockedUntil), nullableTime(u.LastLoginAt), u.ID.String())
	if r.Error != nil {
		return fmt.Errorf("update authentication account: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return domain.ErrUserNotFound
	}
	return nil
}
func nullableTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (t *authTx) Guard(key string) (domain.LoginGuard, error) {
	if err := t.db.Exec(`INSERT INTO identity.login_guard(login_key) VALUES(?) ON CONFLICT DO NOTHING`, key).Error; err != nil {
		return domain.LoginGuard{}, fmt.Errorf("ensure login guard: %w", err)
	}
	rows, err := t.db.Raw(`SELECT failure_times,locked_until FROM identity.login_guard WHERE login_key=? FOR UPDATE`, key).Rows()
	if err != nil {
		return domain.LoginGuard{}, fmt.Errorf("lock login guard: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return domain.LoginGuard{}, fmt.Errorf("missing login guard")
	}
	var failures []byte
	var locked *time.Time
	if err := rows.Scan(&failures, &locked); err != nil {
		return domain.LoginGuard{}, fmt.Errorf("read login guard: %w", err)
	}
	var timestamps []pgtype.Timestamptz
	if err := pgtype.NewMap().Scan(pgtype.TimestamptzArrayOID, pgtype.TextFormatCode, failures, &timestamps); err != nil {
		return domain.LoginGuard{}, fmt.Errorf("decode login window: %w", err)
	}
	guard := domain.LoginGuard{}
	for _, stamp := range timestamps {
		guard.Failures = append(guard.Failures, stamp.Time)
	}
	if locked != nil {
		guard.LockedUntil = *locked
	}
	return guard, rows.Err()
}
func (t *authTx) SaveGuard(key string, g domain.LoginGuard) error {
	stamps := make([]pgtype.Timestamptz, len(g.Failures))
	for i, value := range g.Failures {
		stamps[i] = pgtype.Timestamptz{Time: value, Valid: true}
	}
	encoded, err := pgtype.NewMap().Encode(pgtype.TimestamptzArrayOID, pgtype.TextFormatCode, stamps, nil)
	if err != nil {
		return fmt.Errorf("encode login window: %w", err)
	}
	if err := t.db.Exec(`UPDATE identity.login_guard SET failure_times=?::timestamptz[],locked_until=? WHERE login_key=?`, string(encoded), nullableTime(g.LockedUntil), key).Error; err != nil {
		return fmt.Errorf("save login guard: %w", err)
	}
	return nil
}
func (t *authTx) SessionByHash(hash []byte, lock bool) (domain.AuthSession, error) {
	query := `SELECT id,account_id,token_hash,credential_revision,created_at,last_active_at,absolute_expires_at,persistent,revoked_at FROM identity.user_session WHERE token_hash=?`
	if lock {
		query += ` FOR UPDATE`
	}
	var session domain.AuthSession
	r := t.db.Raw(query, hash).Scan(&session)
	if r.Error != nil {
		return domain.AuthSession{}, fmt.Errorf("read authentication session: %w", r.Error)
	}
	if r.RowsAffected == 0 {
		return domain.AuthSession{}, application.ErrSessionInvalid
	}
	return session, nil
}
func (t *authTx) InsertSession(s domain.AuthSession) error {
	if err := t.db.Exec(`INSERT INTO identity.user_session(id,account_id,token_hash,credential_revision,created_at,last_active_at,absolute_expires_at,persistent) VALUES(?::uuid,?::uuid,?,?,?,?,?,?)`, s.ID.String(), s.AccountID.String(), s.TokenHash, s.CredentialRevision, s.CreatedAt, s.LastActiveAt, s.AbsoluteExpiresAt, s.Persistent).Error; err != nil {
		return fmt.Errorf("insert authentication session: %w", err)
	}
	return nil
}
func (t *authTx) TouchSession(id uuid.UUID, now time.Time) error {
	return t.updateSession(`UPDATE identity.user_session SET last_active_at=? WHERE id=?::uuid`, now, id.String())
}
func (t *authTx) RevokeSession(account, id uuid.UUID, now time.Time) error {
	return t.updateSession(`UPDATE identity.user_session SET revoked_at=? WHERE account_id=?::uuid AND id=?::uuid AND revoked_at IS NULL`, now, account.String(), id.String())
}
func (t *authTx) RevokeAccountSessions(account uuid.UUID, now time.Time) error {
	return t.updateSession(`UPDATE identity.user_session SET revoked_at=? WHERE account_id=?::uuid AND revoked_at IS NULL`, now, account.String())
}
func (t *authTx) updateSession(query string, args ...any) error {
	if err := t.db.Exec(query, args...).Error; err != nil {
		return fmt.Errorf("update authentication session: %w", err)
	}
	return nil
}
func (t *authTx) AppendAuthEvent(account uuid.UUID, action string, now time.Time) error {
	var actor *uuid.UUID
	if account != uuid.Nil {
		actor = &account
	}
	if err := t.db.Exec(`INSERT INTO identity.auth_event(id,account_id,action,created_at) VALUES(?::uuid,?::uuid,?,?)`, uuid.NewString(), actor, action, now).Error; err != nil {
		return fmt.Errorf("append authentication event: %w", err)
	}
	return nil
}

var _ application.AuthStore = (*Store)(nil)
var _ application.AuthTx = (*authTx)(nil)

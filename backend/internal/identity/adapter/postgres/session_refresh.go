package postgres

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// SessionByID is an unlocked candidate lookup or a session lock. Callers lock
// settings, account, session, then refresh records in that order and recheck binding.
func (t *authTx) SessionByID(id uuid.UUID, lock bool) (domain.AuthSession, error) {
	query := `SELECT * FROM identity.user_session WHERE id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	var session domain.AuthSession
	r := t.db.Raw(query, id).Scan(&session)
	if r.Error != nil {
		return session, fmt.Errorf("read session by id: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return session, application.ErrSessionInvalid
	}
	return session, nil
}

func (t *authTx) FindRefresh(digest []byte, lock bool) (domain.RefreshToken, error) {
	if len(digest) != 32 {
		return domain.RefreshToken{}, application.ErrSessionInvalid
	}
	query := `SELECT r.id,r.session_id,s.account_id,r.generation,r.token_digest AS digest,r.issued_at,r.consumed_at,r.revoked_at,r.rotation_request_id,r.replay_until,r.receipt_ciphertext FROM identity.session_refresh_token r JOIN identity.user_session s ON s.id=r.session_id WHERE r.token_digest=?`
	if lock {
		query += ` FOR UPDATE OF r`
	}
	var token domain.RefreshToken
	r := t.db.Raw(query, digest).Scan(&token)
	if r.Error != nil {
		return token, fmt.Errorf("read refresh digest: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return token, application.ErrSessionInvalid
	}
	return token, nil
}

func freshRefresh(token domain.RefreshToken) bool {
	return token.ID != uuid.Nil && token.SessionID != uuid.Nil && token.AccountID != uuid.Nil && token.Generation >= 0 && len(token.Digest) == 32 && !token.IssuedAt.IsZero() && token.ConsumedAt == nil && token.RevokedAt == nil && token.RotationRequestID == uuid.Nil && token.ReplayUntil == nil && len(token.ReceiptCiphertext) == 0
}

func (t *authTx) InsertRefresh(token domain.RefreshToken) error {
	if !freshRefresh(token) {
		return application.ErrSessionInvalid
	}
	r := t.db.Exec(`INSERT INTO identity.session_refresh_token(id,session_id,generation,token_digest,issued_at) SELECT ?,id,?,?,? FROM identity.user_session WHERE id=? AND account_id=? AND protocol_version=2 AND refresh_generation=? AND revoked_at IS NULL`, token.ID, token.Generation, token.Digest, token.IssuedAt, token.SessionID, token.AccountID, token.Generation)
	if r.Error != nil {
		return fmt.Errorf("insert refresh digest: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return application.ErrSessionInvalid
	}
	return nil
}

// RotateRefresh consumes exactly once and saves the child and receipt atomically,
// even if the enclosing caller catches the error. Receipt encryption/AAD belongs
// to the protector; this boundary validates its session/generation/operation binding.
func (t *authTx) RotateRefresh(old, next domain.RefreshToken, operation uuid.UUID, replayUntil time.Time, receipt []byte) error {
	if !freshRefresh(next) || old.ID == uuid.Nil || old.ConsumedAt != nil || old.RevokedAt != nil || old.SessionID != next.SessionID || old.AccountID != next.AccountID || old.Generation < 0 || next.Generation <= 0 || next.Generation-1 != old.Generation || old.ID == next.ID || len(old.Digest) != 32 || bytes.Equal(old.Digest, next.Digest) || operation == uuid.Nil || len(receipt) == 0 || next.IssuedAt.Before(old.IssuedAt) || !replayUntil.After(next.IssuedAt) || replayUntil.After(next.IssuedAt.Add(10*time.Second)) {
		return application.ErrSessionInvalid
	}
	return t.db.Transaction(func(db *gorm.DB) error {
		// Session before token; the application has already locked identity authority.
		r := db.Exec(`UPDATE identity.user_session SET refresh_generation=? WHERE id=? AND account_id=? AND protocol_version=2 AND refresh_generation=? AND revoked_at IS NULL`, next.Generation, old.SessionID, old.AccountID, old.Generation)
		if r.Error != nil {
			return fmt.Errorf("advance refresh generation: %w", r.Error)
		}
		if r.RowsAffected != 1 {
			return application.ErrSessionInvalid
		}
		r = db.Exec(`UPDATE identity.session_refresh_token SET consumed_at=?,rotation_request_id=?,replay_until=?,receipt_ciphertext=? WHERE id=? AND session_id=? AND generation=? AND token_digest=? AND consumed_at IS NULL AND revoked_at IS NULL AND issued_at<=?`, next.IssuedAt, operation, replayUntil, receipt, old.ID, old.SessionID, old.Generation, old.Digest, next.IssuedAt)
		if r.Error != nil {
			return fmt.Errorf("consume refresh digest: %w", r.Error)
		}
		if r.RowsAffected != 1 {
			return application.ErrSessionInvalid
		}
		return (&authTx{db: db}).InsertRefresh(next)
	})
}

func (t *authTx) RevokeRefreshChain(sid uuid.UUID, now time.Time) error {
	return t.db.Exec(`UPDATE identity.session_refresh_token SET revoked_at=COALESCE(revoked_at,?),receipt_ciphertext=NULL,replay_until=NULL WHERE session_id=?`, now, sid).Error
}

func (t *authTx) AppendSessionSecurityEvent(actor, sid, operation uuid.UUID, action, reason string, now time.Time) error {
	if actor == uuid.Nil || sid == uuid.Nil || operation == uuid.Nil || action == "" {
		return application.ErrSessionInvalid
	}
	diff, err := json.Marshal(map[string]string{"session_id": sid.String()})
	if err != nil {
		return err
	}
	r := t.db.Exec(`INSERT INTO identity.security_event(id,actor_id,target_user_id,scope_kind,action,result,reason,request_id,safe_diff,created_at) SELECT ?,?,account_id,'self',?,'success',?,?,?::jsonb,? FROM identity.user_session WHERE id=?`, uuid.New(), actor, action, reason, operation.String(), string(diff), now, sid)
	if r.Error != nil {
		return fmt.Errorf("append session security event: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return application.ErrSessionInvalid
	}
	return nil
}

// CleanupExpiredRefresh bounds each pass; expired receipts are erased while
// digest history remains until the parent's immutable absolute deadline.
func (t *authTx) CleanupExpiredRefresh(now time.Time, limit int) (int, error) {
	if limit < 1 || limit > 1000 {
		return 0, application.ErrSessionInvalid
	}
	var count int
	r := t.db.Raw(`WITH candidates AS (
 SELECT r.id,s.absolute_expires_at<=? AS expired FROM identity.session_refresh_token r JOIN identity.user_session s ON s.id=r.session_id
 WHERE s.absolute_expires_at<=? OR (r.receipt_ciphertext IS NOT NULL AND r.replay_until<=?) ORDER BY r.id LIMIT ? FOR UPDATE OF r SKIP LOCKED
 ), removed AS (
 DELETE FROM identity.session_refresh_token r USING candidates c WHERE r.id=c.id AND c.expired RETURNING r.id
 ), erased AS (
 UPDATE identity.session_refresh_token r SET receipt_ciphertext=NULL,replay_until=NULL FROM candidates c WHERE r.id=c.id AND NOT c.expired RETURNING r.id
 ) SELECT (SELECT count(*) FROM removed)+(SELECT count(*) FROM erased) AS count`, now, now, now, limit).Scan(&count)
	if r.Error != nil {
		return 0, fmt.Errorf("cleanup refresh history: %w", r.Error)
	}
	return count, nil
}

// RefreshByGeneration reads one committed parent receipt after account/session locks.
func (t *authTx) RefreshByGeneration(sid uuid.UUID, generation int64, lock bool) (domain.RefreshToken, error) {
	query := `SELECT r.id,r.session_id,s.account_id,r.generation,r.token_digest AS digest,r.issued_at,r.consumed_at,r.revoked_at,r.rotation_request_id,r.replay_until,r.receipt_ciphertext FROM identity.session_refresh_token r JOIN identity.user_session s ON s.id=r.session_id WHERE r.session_id=? AND r.generation=?`
	if lock {
		query += ` FOR UPDATE OF r`
	}
	var token domain.RefreshToken
	r := t.db.Raw(query, sid, generation).Scan(&token)
	if r.Error != nil {
		return token, fmt.Errorf("read refresh generation: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return token, application.ErrSessionInvalid
	}
	return token, nil
}

// UpgradeLegacySession changes only protocol, retaining the original device's
// authority, deadlines, CSRF proof and legacy digest for bounded receipt lookup.
func (t *authTx) UpgradeLegacySession(sid, account uuid.UUID, digest []byte) error {
	if sid == uuid.Nil || account == uuid.Nil || len(digest) != 32 {
		return application.ErrSessionInvalid
	}
	r := t.db.Exec(`UPDATE identity.user_session SET protocol_version=2 WHERE id=? AND account_id=? AND token_hash=? AND protocol_version=1 AND refresh_generation=0 AND revoked_at IS NULL`, sid, account, digest)
	if r.Error != nil {
		return fmt.Errorf("upgrade legacy protocol: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return application.ErrSessionInvalid
	}
	return nil
}

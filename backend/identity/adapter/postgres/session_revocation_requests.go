package postgres

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

var _ application.SessionRevocationRequestStore = (*SessionRevocationStore)(nil)

type sessionRevocationRequestRow struct {
	ID               uuid.UUID
	IdemKey          uuid.UUID
	AccountID        uuid.UUID
	RequestSessionID uuid.UUID
	Action           application.SessionRevocationAction
	TargetSessionID  *uuid.UUID
	RequestedAt      time.Time
	CompletedAt      *time.Time
}

// PrepareSessionRevocationRequest freezes the accepted intent, proofs, targets and
// all original source decisions in one issuer-local root-pool transaction.
func (s *SessionRevocationStore) PrepareSessionRevocationRequest(ctx context.Context, actor application.Principal, input application.SessionRevocationInput, clock func() time.Time) (application.SessionRevocationRequest, error) {
	if err := input.Validate(); err != nil {
		return application.SessionRevocationRequest{}, err
	}
	if clock == nil || input.Primary == "" {
		return application.SessionRevocationRequest{}, application.ErrSessionInvalid
	}
	var result application.SessionRevocationRequest
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if _, err := lockSelfActor(tx, actor, clock); err != nil {
			return err
		}
		var previous struct{ ID uuid.UUID }
		read := tx.Raw(`SELECT id FROM identity.session_revocation_request WHERE account_id=? AND request_session_id=? AND idem_key=?`, actor.ID, actor.SessionID, input.Key).Scan(&previous)
		if read.Error != nil {
			return fmt.Errorf("find accepted session revocation request: %w", read.Error)
		}
		if read.RowsAffected == 1 {
			saved, err := readSessionRevocationRequest(tx, previous.ID, false)
			if err != nil {
				return err
			}
			if err := matchSessionRevocationIntent(saved, input); err != nil {
				return err
			}
			if _, err := authenticatedSessionRevocationProofs(tx, actor, input, clock); err != nil {
				return err
			}
			result = saved
			return nil
		}
		proofs, err := authenticatedSessionRevocationProofs(tx, actor, input, clock)
		if err != nil {
			return err
		}
		now := clock().UTC().Truncate(time.Microsecond)
		result = application.SessionRevocationRequest{ID: uuid.New(), Key: input.Key, AccountID: actor.ID, RequestSessionID: actor.SessionID, Action: input.Action, TargetID: input.TargetID, RequestedAt: now}
		ids := []uuid.UUID{input.TargetID}
		if input.Action == application.RevokeOtherSessionsRequest {
			// Include accepted pending responsibilities even if their credential rows
			// were already collected. Never re-run this query for an existing key.
			var targets []struct{ ID uuid.UUID }
			if err := tx.Raw(`SELECT id FROM identity.user_session WHERE account_id=? AND id<>?
				UNION SELECT session_id AS id FROM identity.session_revocation WHERE account_id=? AND session_id<>? AND completed_at IS NULL ORDER BY id`, actor.ID, actor.SessionID, actor.ID, actor.SessionID).Scan(&targets).Error; err != nil {
				return fmt.Errorf("freeze session revocation targets: %w", err)
			}
			ids = make([]uuid.UUID, 0, len(targets))
			for _, target := range targets {
				ids = append(ids, target.ID)
			}
		}
		for _, id := range ids {
			source, err := prepareSessionRevocationTx(ctx, tx, actor, id, func() time.Time { return now })
			if err != nil {
				return err
			}
			result.Revocations = append(result.Revocations, source)
		}
		if err := result.Validate(); err != nil {
			return err
		}
		var target *uuid.UUID
		if input.Action == application.RevokeSessionRequest || input.Action == application.RevokeRefreshReuseRequest {
			target = &input.TargetID
		}
		if err := tx.Exec(`INSERT INTO identity.session_revocation_request(id,idem_key,account_id,request_session_id,action,target_session_id,requested_at) VALUES (?,?,?,?,?,?,?)`, result.ID, input.Key, actor.ID, actor.SessionID, input.Action, target, now).Error; err != nil {
			return fmt.Errorf("persist original session revocation request: %w", err)
		}
		for _, source := range result.Revocations {
			if err := tx.Exec(`INSERT INTO identity.session_revocation_request_target(request_id,session_id) VALUES (?,?)`, result.ID, source.SessionID).Error; err != nil {
				return fmt.Errorf("persist fixed session revocation target: %w", err)
			}
		}
		for _, proof := range proofs {
			if err := tx.Exec(`INSERT INTO identity.session_revocation_request_proof(request_id,kind,token_digest,csrf_hash) VALUES (?,?,?,?)`, result.ID, proof.Kind, proof.TokenDigest, proof.CSRFHash).Error; err != nil {
				return fmt.Errorf("persist authenticated session revocation proof: %w", err)
			}
		}
		if err := taskruntime.Enqueue(ctx, tx, application.SessionRevocationRecoveryTask, result.ID.String(), now); err != nil {
			return fmt.Errorf("persist original session recovery task: %w", err)
		}
		return nil
	})
	return result, err
}

// authenticatedSessionRevocationProofs runs only after lockSelfActor. Access is
// trusted exclusively as the authenticator-selected primary; all opaque evidence
// is checked against this exact locked session, never accepted from a caller UUID.
func authenticatedSessionRevocationProofs(tx *gorm.DB, actor application.Principal, input application.SessionRevocationInput, clock func() time.Time) ([]application.SessionRevocationProof, error) {
	var session struct {
		TokenHash       []byte
		CSRFHash        []byte
		ProtocolVersion int
	}
	read := tx.Raw(`SELECT token_hash,csrf_hash,protocol_version FROM identity.user_session WHERE id=? AND account_id=?`, actor.SessionID, actor.ID).Scan(&session)
	if read.Error != nil {
		return nil, fmt.Errorf("read original session evidence: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return nil, application.ErrSessionInvalid
	}
	if input.Primary == application.SessionCredentialRefresh && ((input.Action != application.RevokeSessionRequest && input.Action != application.RevokeRefreshReuseRequest) || input.TargetID != actor.SessionID) {
		return nil, application.ErrForbidden
	}
	var accepted []application.SessionRevocationProof
	primaryAccepted := false
	for _, proof := range input.Proofs {
		primary := proof.Kind == input.Primary
		if subtle.ConstantTimeCompare(session.CSRFHash, proof.CSRFHash) != 1 {
			if primary {
				return nil, application.ErrSessionCSRFInvalid
			}
			continue
		}
		valid := false
		switch proof.Kind {
		case application.SessionCredentialLegacy:
			valid = primary && session.ProtocolVersion == 1 && subtle.ConstantTimeCompare(session.TokenHash, proof.TokenDigest) == 1
		case application.SessionCredentialAccess:
			valid = primary && session.ProtocolVersion == 2
		case application.SessionCredentialRefresh:
			if session.ProtocolVersion == 2 {
				if input.Action == application.RevokeRefreshReuseRequest {
					parent, err := (&authTx{db: tx}).FindRefresh(proof.TokenDigest, true)
					if err != nil {
						return nil, fmt.Errorf("lock original reused parent: %w", err)
					}
					now := clock().UTC()
					valid = parent.SessionID == actor.SessionID && parent.AccountID == actor.ID && parent.RevokedAt == nil && parent.ConsumedAt != nil && !now.Before(parent.IssuedAt) && (parent.ReplayUntil == nil || !now.Before(*parent.ReplayUntil))
				} else {
					var exists bool
					if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM identity.session_refresh_token WHERE session_id=? AND token_digest=? AND revoked_at IS NULL)`, actor.SessionID, proof.TokenDigest).Scan(&exists).Error; err != nil {
						return nil, fmt.Errorf("verify original refresh evidence: %w", err)
					}
					valid = exists
				}
			}
		}
		if primary && !valid {
			return nil, application.ErrSessionInvalid
		}
		if valid {
			accepted = append(accepted, proof)
			primaryAccepted = primaryAccepted || primary
		}
	}
	if !primaryAccepted {
		return nil, application.ErrSessionInvalid
	}
	return accepted, nil
}

// LoadSessionRevocationRequest recognizes only an already accepted credential,
// CSRF hash and original key. It cannot enroll evidence or select new targets.
func (s *SessionRevocationStore) LoadSessionRevocationRequest(ctx context.Context, input application.SessionRevocationInput) (application.SessionRevocationRequest, error) {
	if err := input.Validate(); err != nil {
		return application.SessionRevocationRequest{}, err
	}
	var result application.SessionRevocationRequest
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var err error
		result, err = findSessionRevocationProof(tx, input.Key, input.Proofs)
		if err != nil {
			return err
		}
		return matchSessionRevocationIntent(result, input)
	})
	return result, err
}

// CompleteSessionRevocationRequest acknowledges only the full immutable set of
// already-completed source journals. The batch audit and completion share a tx.
func (s *SessionRevocationStore) CompleteSessionRevocationRequest(ctx context.Context, request application.SessionRevocationRequest, now time.Time) error {
	if err := request.Validate(); err != nil {
		return err
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT id FROM identity."user" WHERE id=? FOR KEY SHARE`, request.AccountID).Error; err != nil {
			return fmt.Errorf("lock original request audit account: %w", err)
		}
		saved, err := readSessionRevocationRequest(tx, request.ID, true)
		if err != nil {
			return err
		}
		if !sameSessionRevocationRequest(saved, request) {
			return writefence.ErrConflict
		}
		for _, source := range saved.Revocations {
			if source.CompletedAt == nil {
				return application.ErrSessionRevocationUnavailable
			}
		}
		if saved.CompletedAt != nil {
			return nil
		}
		now = now.UTC().Truncate(time.Microsecond)
		if now.Before(saved.RequestedAt) {
			return application.ErrSessionRevocationUnavailable
		}
		// Each source already owns its session.revoked audit. Only an accepted
		// batch needs a further aggregate success event, never a duplicate source.
		if saved.Action == application.RevokeOtherSessionsRequest {
			diff, err := json.Marshal(map[string]any{"account_id": saved.AccountID.String(), "request_session_id": saved.RequestSessionID.String(), "target_count": len(saved.Revocations)})
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO identity.security_event(id,actor_id,target_user_id,scope_kind,action,result,reason,request_id,safe_diff,created_at)
				VALUES (?,(SELECT id FROM identity."user" WHERE id=?),(SELECT id FROM identity."user" WHERE id=?),'self','session.others_revoked','success','all original source revocations completed',?,?::jsonb,?)`, uuid.New(), saved.AccountID, saved.AccountID, saved.ID.String(), string(diff), now).Error; err != nil {
				return fmt.Errorf("audit completed session revocation request: %w", err)
			}
		}
		updated := tx.Exec(`UPDATE identity.session_revocation_request SET completed_at=? WHERE id=? AND completed_at IS NULL`, now, saved.ID)
		if updated.Error != nil {
			return fmt.Errorf("complete original session revocation request: %w", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return writefence.ErrConflict
		}
		return nil
	})
}

func matchSessionRevocationIntent(saved application.SessionRevocationRequest, input application.SessionRevocationInput) error {
	if saved.Key != input.Key || saved.Action != input.Action || saved.TargetID != input.TargetID {
		return application.ErrForbidden
	}
	return nil
}

func readSessionRevocationRequest(tx *gorm.DB, id uuid.UUID, lock bool) (application.SessionRevocationRequest, error) {
	query := `SELECT id,idem_key,account_id,request_session_id,action,target_session_id,requested_at,completed_at FROM identity.session_revocation_request WHERE id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	var row sessionRevocationRequestRow
	read := tx.Raw(query, id).Scan(&row)
	if read.Error != nil {
		return application.SessionRevocationRequest{}, fmt.Errorf("read original session revocation request: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return application.SessionRevocationRequest{}, writefence.ErrNotFound
	}
	result := application.SessionRevocationRequest{ID: row.ID, Key: row.IdemKey, AccountID: row.AccountID, RequestSessionID: row.RequestSessionID, Action: row.Action, RequestedAt: row.RequestedAt, CompletedAt: row.CompletedAt}
	if row.TargetSessionID != nil {
		result.TargetID = *row.TargetSessionID
	}
	var targets []struct{ SessionID uuid.UUID }
	if err := tx.Raw(`SELECT session_id FROM identity.session_revocation_request_target WHERE request_id=? ORDER BY session_id`, id).Scan(&targets).Error; err != nil {
		return application.SessionRevocationRequest{}, fmt.Errorf("read fixed session revocation targets: %w", err)
	}
	for _, target := range targets {
		source, err := readSessionRevocation(tx, target.SessionID, lock)
		if err != nil {
			if errors.Is(err, writefence.ErrNotFound) {
				return application.SessionRevocationRequest{}, application.ErrSessionRevocationUnavailable
			}
			return application.SessionRevocationRequest{}, err
		}
		result.Revocations = append(result.Revocations, source)
	}
	return result, result.Validate()
}

func sameSessionRevocationRequest(a, b application.SessionRevocationRequest) bool {
	if a.ID != b.ID || a.Key != b.Key || a.AccountID != b.AccountID || a.RequestSessionID != b.RequestSessionID || a.Action != b.Action || a.TargetID != b.TargetID || !a.RequestedAt.Equal(b.RequestedAt) || len(a.Revocations) != len(b.Revocations) {
		return false
	}
	sources := make(map[uuid.UUID]application.SessionRevocation, len(a.Revocations))
	for _, source := range a.Revocations {
		sources[source.SessionID] = source
	}
	for _, source := range b.Revocations {
		saved, ok := sources[source.SessionID]
		if !ok || saved.AccountID != source.AccountID || saved.RequestSessionID != source.RequestSessionID || saved.Control != source.Control || !saved.RequestedAt.Equal(source.RequestedAt) {
			return false
		}
	}
	return true
}

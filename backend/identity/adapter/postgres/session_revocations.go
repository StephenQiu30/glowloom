package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// SessionRevocationStore persists Identity's source decision, not remote effects.
// Its root connection must be an actual restricted Identity capability.
type SessionRevocationStore struct{ db *gorm.DB }

// NewSessionRevocationStore binds a root pool; an open caller transaction is rejected.
func NewSessionRevocationStore(db *gorm.DB) *SessionRevocationStore {
	return &SessionRevocationStore{db: db}
}

var _ application.SessionRevocationStore = (*SessionRevocationStore)(nil)

type sessionRevocationRow struct {
	SessionID        uuid.UUID
	AccountID        uuid.UUID
	RequestSessionID uuid.UUID
	CoordinationID   uuid.UUID
	ExpectedEpoch    int64
	RequestedAt      time.Time
	CompletedAt      *time.Time
}

func (r sessionRevocationRow) revocation() (application.SessionRevocation, error) {
	if r.ExpectedEpoch < 0 || r.ExpectedEpoch == math.MaxInt64 {
		return application.SessionRevocation{}, application.ErrSessionRevocationUnavailable
	}
	result := application.SessionRevocation{
		AccountID: r.AccountID, SessionID: r.SessionID, RequestSessionID: r.RequestSessionID,
		Control:     writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Session, ID: r.SessionID, Epoch: r.ExpectedEpoch + 1}, CoordinationID: r.CoordinationID, Blocked: true},
		RequestedAt: r.RequestedAt, CompletedAt: r.CompletedAt,
	}
	return result, result.Validate()
}

// PrepareSessionRevocation commits policy, parent/refresh revocation, original
// recovery identity and closed admission together. It never calls a remote writer.
func (s *SessionRevocationStore) PrepareSessionRevocation(ctx context.Context, actor application.Principal, id uuid.UUID, clock func() time.Time) (application.SessionRevocation, error) {
	if id == uuid.Nil {
		return application.SessionRevocation{}, application.ErrSessionInvalid
	}
	if clock == nil {
		return application.SessionRevocation{}, application.ErrSessionRevocationUnavailable
	}
	var result application.SessionRevocation
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if _, err := lockSelfActor(tx, actor, clock); err != nil {
			return err
		}
		var err error
		result, err = prepareSessionRevocationTx(ctx, tx, actor, id, clock)
		return err
	})
	return result, err
}

// LoadSessionRevocation recovers only the original source fact, even after session
// cleanup. It does not authenticate a user or admit a new business command.
func (s *SessionRevocationStore) LoadSessionRevocation(ctx context.Context, id uuid.UUID) (application.SessionRevocation, error) {
	if id == uuid.Nil {
		return application.SessionRevocation{}, application.ErrSessionInvalid
	}
	var result application.SessionRevocation
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var err error
		result, err = readSessionRevocation(tx, id, false)
		return err
	})
	return result, err
}

// CompleteSessionRevocation verifies all original writer evidence and atomically
// records source completion plus one audit event. No session is reopened or touched.
func (s *SessionRevocationStore) CompleteSessionRevocation(ctx context.Context, revocation application.SessionRevocation, now time.Time) error {
	if err := revocation.Validate(); err != nil {
		return err
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		// Audit user foreign keys take KEY SHARE too. Acquire it before the
		// journal, matching Prepare's account -> session -> journal order; else
		// a valid concurrent replay could deadlock at the audit INSERT. A missing
		// account is allowed: its immutable source decision must survive cleanup.
		if err := tx.Exec(`SELECT id FROM identity."user" WHERE id=? FOR KEY SHARE`, revocation.AccountID).Error; err != nil {
			return fmt.Errorf("lock original session audit account: %w", err)
		}
		saved, err := readSessionRevocation(tx, revocation.SessionID, true)
		if err != nil {
			return err
		}
		if saved.AccountID != revocation.AccountID || saved.RequestSessionID != revocation.RequestSessionID || saved.Control != revocation.Control || !saved.RequestedAt.Equal(revocation.RequestedAt) {
			return writefence.ErrConflict
		}
		if saved.CompletedAt != nil {
			return nil
		}
		if now.Before(saved.RequestedAt) {
			return application.ErrSessionRevocationUnavailable
		}
		var installed bool
		if err := tx.Raw(`SELECT blocked AND complete FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='identity' AND authority_kind='session' AND authority_id=? AND epoch=? AND coordination_id=? FOR UPDATE`, saved.SessionID, saved.Control.Epoch, saved.Control.CoordinationID).Scan(&installed).Error; err != nil {
			return fmt.Errorf("verify session source closure: %w", err)
		}
		if !installed {
			return writefence.ErrFenced
		}
		state, err := pgfence.NewAuthorityStore(tx).LoadCoordination(ctx, saved.Control.CoordinationID)
		if err != nil {
			return err
		}
		if !state.Complete || state.Control != saved.Control {
			return writefence.ErrConflict
		}
		diff, err := json.Marshal(map[string]string{"session_id": saved.SessionID.String(), "request_session_id": saved.RequestSessionID.String(), "account_id": saved.AccountID.String()})
		if err != nil {
			return err
		}
		// The immutable source IDs survive physical account/session cleanup. Nullable
		// audit references are resolved locally; safe_diff retains the original IDs.
		if err := tx.Exec(`INSERT INTO identity.security_event(id,actor_id,target_user_id,scope_kind,action,result,reason,request_id,safe_diff,created_at)
			VALUES (?,(SELECT id FROM identity."user" WHERE id=?),(SELECT id FROM identity."user" WHERE id=?),'self','session.revoked','success','durable writer acknowledgements',?,?::jsonb,?)`, uuid.New(), saved.AccountID, saved.AccountID, saved.Control.CoordinationID.String(), string(diff), now.UTC()).Error; err != nil {
			return fmt.Errorf("audit completed session revocation: %w", err)
		}
		updated := tx.Exec(`UPDATE identity.session_revocation SET completed_at=? WHERE session_id=? AND coordination_id=? AND completed_at IS NULL`, now.UTC(), saved.SessionID, saved.Control.CoordinationID)
		if updated.Error != nil {
			return fmt.Errorf("complete session revocation: %w", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return writefence.ErrConflict
		}
		return nil
	})
}

func (s *SessionRevocationStore) transaction(ctx context.Context, run func(*gorm.DB) error) error {
	if s == nil {
		return application.ErrSessionRevocationUnavailable
	}
	return identityWriterTransaction(ctx, s.db, run)
}

func identityWriterTransaction(ctx context.Context, db *gorm.DB, run func(*gorm.DB) error) error {
	if db == nil || db.Statement == nil {
		return writefence.ErrUnavailable
	}
	if _, open := db.Statement.ConnPool.(gorm.TxCommitter); open {
		return writefence.ErrTransaction
	}
	return taskruntime.Transaction(ctx, db, func(tx *gorm.DB) error {
		owner, err := pgfence.New(tx).Owner(ctx)
		if err != nil {
			return err
		}
		if owner != writefence.WriterIdentity {
			return writefence.ErrUnavailable
		}
		return run(tx)
	})
}

// prepareSessionRevocationTx shares the exact source closure path with atomic
// public batches. Its caller already holds the current account and actor locks.
func prepareSessionRevocationTx(ctx context.Context, tx *gorm.DB, actor application.Principal, id uuid.UUID, clock func() time.Time) (application.SessionRevocation, error) {
	var target struct {
		ID        uuid.UUID
		RevokedAt *time.Time
	}
	read := tx.Raw(`SELECT id,revoked_at FROM identity.user_session WHERE id=? AND account_id=? FOR UPDATE`, id, actor.ID).Scan(&target)
	if read.Error != nil {
		return application.SessionRevocation{}, fmt.Errorf("lock session revocation target: %w", read.Error)
	}
	if saved, err := readSessionRevocation(tx, id, true); err == nil {
		// A collected target is valid only when the immutable source has the
		// authenticated account. An extant target must already be locally closed.
		if saved.AccountID != actor.ID && read.RowsAffected != 1 {
			return application.SessionRevocation{}, application.ErrSessionInvalid
		}
		if saved.AccountID != actor.ID || (read.RowsAffected == 1 && target.RevokedAt == nil) {
			return application.SessionRevocation{}, application.ErrSessionRevocationUnavailable
		}
		return saved, nil
	} else if !errors.Is(err, writefence.ErrNotFound) {
		return application.SessionRevocation{}, err
	}
	if read.RowsAffected != 1 {
		return application.SessionRevocation{}, application.ErrSessionInvalid
	}
	// PostgreSQL persists microseconds; return exactly its recovery precision.
	now := clock().UTC().Truncate(time.Microsecond)
	if err := revokeSessions(tx, actor.ID, &id, nil, now); err != nil {
		return application.SessionRevocation{}, err
	}
	var head struct{ Epoch int64 }
	if err := tx.Raw(`SELECT epoch FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='identity' AND authority_kind='session' AND authority_id=? FOR UPDATE`, id).Scan(&head).Error; err != nil {
		return application.SessionRevocation{}, fmt.Errorf("lock original session authority: %w", err)
	}
	state, err := pgfence.NewAuthorityStore(tx).BeginTx(ctx, tx, writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Session, ID: id, Epoch: head.Epoch}, uuid.New(), true)
	if err != nil {
		return application.SessionRevocation{}, err
	}
	if err := tx.Exec(`INSERT INTO identity.session_revocation(session_id,account_id,request_session_id,coordination_id,expected_epoch,requested_at) VALUES (?,?,?,?,?,?)`, id, actor.ID, actor.SessionID, state.Control.CoordinationID, head.Epoch, now).Error; err != nil {
		return application.SessionRevocation{}, fmt.Errorf("persist original session revocation: %w", err)
	}
	result := application.SessionRevocation{AccountID: actor.ID, SessionID: id, RequestSessionID: actor.SessionID, Control: state.Control, RequestedAt: now}
	return result, result.Validate()
}

func readSessionRevocation(tx *gorm.DB, id uuid.UUID, lock bool) (application.SessionRevocation, error) {
	query := `SELECT session_id,account_id,request_session_id,coordination_id,expected_epoch,requested_at,completed_at FROM identity.session_revocation WHERE session_id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	var row sessionRevocationRow
	result := tx.Raw(query, id).Scan(&row)
	if result.Error != nil {
		return application.SessionRevocation{}, fmt.Errorf("read original session revocation: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return application.SessionRevocation{}, writefence.ErrNotFound
	}
	return row.revocation()
}

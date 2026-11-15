package postgres

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

type sessionActivationRow struct {
	RequestID, AccountID, SessionID, CoordinationID uuid.UUID
	InputDigest                                     []byte
	ResponseKeyID                                   string
	ResponseCiphertext                              []byte
	AcceptedAt                                      time.Time
	CompletedAt                                     *time.Time
}

func (r sessionActivationRow) activation() application.SessionActivation {
	return application.SessionActivation{RequestID: r.RequestID, AccountID: r.AccountID, SessionID: r.SessionID, InputSHA256: hex.EncodeToString(r.InputDigest), Control: writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Session, ID: r.SessionID, Epoch: 1}, CoordinationID: r.CoordinationID}, ResponseKeyID: r.ResponseKeyID, ResponseCiphertext: append([]byte(nil), r.ResponseCiphertext...), AcceptedAt: r.AcceptedAt, Completed: r.CompletedAt != nil}
}
func readSessionActivation(tx *gorm.DB, id uuid.UUID, lock bool) (sessionActivationRow, bool, error) {
	var row sessionActivationRow
	query := `SELECT request_id,account_id,session_id,coordination_id,input_digest,response_key_id,response_ciphertext,accepted_at,completed_at FROM identity.session_activation WHERE request_id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	result := tx.Raw(query, id).Scan(&row)
	if result.Error != nil {
		return row, false, fmt.Errorf("read original login activation: %w", result.Error)
	}
	return row, result.RowsAffected == 1, nil
}

// PrepareSessionActivation runs inside the original account/password/session
// transaction. It never publishes a credential or performs a network call.
func (t *registrationAuthTx) PrepareSessionActivation(p application.SessionActivation) error {
	proof, err := hex.DecodeString(p.InputSHA256)
	if err != nil || len(proof) != 32 || p.RequestID == uuid.Nil || p.AccountID == uuid.Nil || p.SessionID == uuid.Nil || p.Control.Validate() != nil || p.Control.ID != p.SessionID || p.Control.Issuer != writefence.Identity || p.Control.Kind != writefence.Session || p.Control.Epoch != 1 || p.Control.Blocked || p.AcceptedAt.IsZero() {
		return application.ErrSessionActivation
	}
	saved, found, err := readSessionActivation(t.db, p.RequestID, true)
	if err != nil {
		return err
	}
	if found {
		if !hmac.Equal(saved.InputDigest, proof) {
			return application.ErrProofInvalid
		}
		return application.ErrSessionActivationExists
	}
	if err := requireOpenAccountHead(t.db, p.AccountID); err != nil {
		return err
	}
	expected := p.Control.Authority
	expected.Epoch = 0
	state, err := pgfence.NewAuthorityStore(t.db).BeginTx(t.db.Statement.Context, t.db, expected, p.Control.CoordinationID, false)
	if err != nil {
		return err
	}
	if state.Control != p.Control {
		return application.ErrSessionActivation
	}
	if err := t.db.Exec(`INSERT INTO identity.session_activation(request_id,account_id,session_id,coordination_id,input_digest,response_key_id,response_ciphertext,accepted_at) VALUES(?,?,?,?,?,?,?,?)`, p.RequestID, p.AccountID, p.SessionID, p.Control.CoordinationID, proof, p.ResponseKeyID, p.ResponseCiphertext, p.AcceptedAt).Error; err != nil {
		return fmt.Errorf("persist original login activation: %w", err)
	}
	return taskruntime.Enqueue(t.db.Statement.Context, t.db, application.SessionActivationRecoveryTask, p.RequestID.String(), p.AcceptedAt)
}

func requireOpenAccountHead(tx *gorm.DB, account uuid.UUID) error {
	var pending bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM identity.account_qualification_change WHERE target_id=? AND outcome='pending')`, account).Scan(&pending).Error; err != nil {
		return err
	}
	if pending {
		return application.ErrSessionActivation
	}
	var head struct {
		Epoch             int64
		Blocked, Complete bool
		CoordinationID    uuid.UUID
	}
	result := tx.Raw(`SELECT epoch,blocked,complete,coordination_id FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='identity' AND authority_kind='account' AND authority_id=? FOR UPDATE`, account).Scan(&head)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 || head.Epoch < 1 || head.Blocked || !head.Complete {
		return application.ErrSessionActivation
	}
	state, err := pgfence.NewAuthorityStore(tx).LoadCoordination(tx.Statement.Context, head.CoordinationID)
	if err != nil {
		return err
	}
	if !state.Complete || state.Control.Kind != writefence.Account || state.Control.ID != account || state.Control.Epoch != head.Epoch || state.Control.Blocked || len(state.Acknowledged) != len(writefence.Writers()) {
		return application.ErrSessionActivation
	}
	return nil
}

// LoadSessionActivation never authorizes a new login using a previous key alone.
func (s *RegistrationStore) LoadSessionActivation(ctx context.Context, id uuid.UUID, proof []byte) (application.SessionActivation, bool, error) {
	var result application.SessionActivation
	found := false
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		row, exists, err := readSessionActivation(tx, id, false)
		if err != nil {
			return err
		}
		found = exists
		if !exists {
			return nil
		}
		if len(proof) != 32 || !hmac.Equal(row.InputDigest, proof) {
			return application.ErrProofInvalid
		}
		result = row.activation()
		return nil
	})
	return result, found, err
}

// LoadAcceptedSessionActivation is the owner worker's fixed original-ID reader.
func (s *RegistrationStore) LoadAcceptedSessionActivation(ctx context.Context, id uuid.UUID) (application.SessionActivation, error) {
	var result application.SessionActivation
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		row, exists, err := readSessionActivation(tx, id, false)
		if err != nil {
			return err
		}
		if !exists {
			return application.ErrSessionActivation
		}
		result = row.activation()
		return nil
	})
	return result, err
}

// CompleteSessionActivation rechecks live identity under account→session→request
// locks, then verifies the exact initial head and all durable writer ACKs. An
// original reply cannot revive a subsequently revoked or disabled session.
func (s *RegistrationStore) CompleteSessionActivation(ctx context.Context, p application.SessionActivation) error {
	return identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		t := &emailAuthTx{authTx: &authTx{db: tx}}
		user, err := t.AccountByID(p.AccountID)
		if err != nil {
			return application.ErrSessionInvalid
		}
		session, err := t.SessionByID(p.SessionID, true)
		if err != nil {
			return application.ErrSessionInvalid
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if user.Status != domain.StatusActive || user.EmailVerifiedAt.IsZero() || !session.Valid(user, now) {
			return application.ErrSessionInvalid
		}
		row, found, err := readSessionActivation(tx, p.RequestID, true)
		if err != nil {
			return err
		}
		if !found || row.AccountID != p.AccountID || row.SessionID != p.SessionID || row.CoordinationID != p.Control.CoordinationID || hex.EncodeToString(row.InputDigest) != p.InputSHA256 || row.ResponseKeyID != p.ResponseKeyID || !bytes.Equal(row.ResponseCiphertext, p.ResponseCiphertext) {
			return application.ErrSessionActivation
		}
		if err := requireOpenAccountHead(tx, p.AccountID); err != nil {
			return err
		}
		state, err := pgfence.NewAuthorityStore(tx).LoadCoordination(ctx, p.Control.CoordinationID)
		if err != nil {
			return err
		}
		if !state.Complete || state.Control != p.Control || len(state.Acknowledged) != len(writefence.Writers()) {
			return application.ErrSessionActivation
		}
		var head struct {
			Epoch             int64
			Blocked, Complete bool
			CoordinationID    uuid.UUID
		}
		result := tx.Raw(`SELECT epoch,blocked,complete,coordination_id FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='identity' AND authority_kind='session' AND authority_id=? FOR UPDATE`, p.SessionID).Scan(&head)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 || head.Epoch != 1 || head.Blocked || !head.Complete || head.CoordinationID != p.Control.CoordinationID {
			return application.ErrSessionActivation
		}
		if row.CompletedAt != nil {
			return nil
		}
		if now.Before(row.AcceptedAt) {
			now = row.AcceptedAt
		}
		if err := t.AppendAuthEvent(p.AccountID, "logged_in", now); err != nil {
			return err
		}
		return tx.Exec(`UPDATE identity.session_activation SET completed_at=? WHERE request_id=? AND completed_at IS NULL`, now, p.RequestID).Error
	})
}

var _ application.SessionActivationTx = (*registrationAuthTx)(nil)
var _ application.SessionActivationStore = (*RegistrationStore)(nil)

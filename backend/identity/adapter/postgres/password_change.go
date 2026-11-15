package postgres

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
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

// PasswordChangeStore owns only the explicit self-password source responsibility.
type PasswordChangeStore struct{ db *gorm.DB }

// NewPasswordChangeStore requires the restricted Identity root connection.
func NewPasswordChangeStore(db *gorm.DB) *PasswordChangeStore { return &PasswordChangeStore{db: db} }

type passwordCandidate struct {
	User                domain.User
	Persistent, IssueV2 bool
}
type passwordChangeRow struct {
	RequestID, AccountID, ActorSessionID, CloseCoordinationID             uuid.UUID
	InputDigest, ProofDigest                                              []byte
	Candidate                                                             string
	CloseExpectedEpoch                                                    int64
	ReopenCoordinationID, ReplacementSessionID, ReplacementCoordinationID *uuid.UUID
	ReopenExpectedEpoch                                                   *int64
	ResponseKeyID                                                         *string
	ResponseCiphertext                                                    []byte
	Action, Outcome                                                       string
	AcceptedAt                                                            time.Time
	CompletedAt                                                           *time.Time
}

func (r passwordChangeRow) record() (application.PasswordChange, error) {
	if r.Action != "password_change" {
		return application.PasswordChange{}, application.ErrPasswordChangeUnavailable
	}
	var candidate passwordCandidate
	decoder := json.NewDecoder(bytes.NewBufferString(r.Candidate))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&candidate) != nil {
		return application.PasswordChange{}, application.ErrPasswordChangeUnavailable
	}
	p := application.PasswordChange{RequestID: r.RequestID, AccountID: r.AccountID, ActorSessionID: r.ActorSessionID, InputSHA256: hex.EncodeToString(r.InputDigest), ProofSHA256: hex.EncodeToString(r.ProofDigest), Candidate: candidate.User, Persistent: candidate.Persistent, IssueV2: candidate.IssueV2, AcceptedAt: r.AcceptedAt, Completed: r.CompletedAt != nil, ResponseCiphertext: append([]byte(nil), r.ResponseCiphertext...), CloseControl: writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: r.AccountID, Epoch: r.CloseExpectedEpoch + 1}, CoordinationID: r.CloseCoordinationID, Blocked: true}}
	if r.ReopenCoordinationID != nil && r.ReopenExpectedEpoch != nil {
		p.ReopenControl = writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: r.AccountID, Epoch: *r.ReopenExpectedEpoch + 1}, CoordinationID: *r.ReopenCoordinationID}
	}
	if r.ReplacementSessionID != nil && r.ReplacementCoordinationID != nil {
		p.ReplacementSessionID = *r.ReplacementSessionID
		p.SessionControl = writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Session, ID: *r.ReplacementSessionID, Epoch: 1}, CoordinationID: *r.ReplacementCoordinationID}
	}
	if r.ResponseKeyID != nil {
		p.ResponseKeyID = *r.ResponseKeyID
	}
	return p, nil
}
func readPasswordChange(tx *gorm.DB, id uuid.UUID, lock bool) (passwordChangeRow, bool, error) {
	var row passwordChangeRow
	sql := `SELECT request_id,target_id AS account_id,actor_session_id,action,input_digest,proof_digest,candidate::text,close_coordination_id,close_expected_epoch,reopen_coordination_id,reopen_expected_epoch,replacement_session_id,replacement_coordination_id,response_key_id,response_ciphertext,outcome,accepted_at,completed_at FROM identity.account_qualification_change WHERE request_id=?`
	if lock {
		sql += ` FOR UPDATE`
	}
	result := tx.Raw(sql, id).Scan(&row)
	return row, result.RowsAffected == 1, result.Error
}

// PreparePasswordChange closes source admission and queues the original request
// inside its verified account/session transaction, before any password mutation.
func (t *registrationAuthTx) PreparePasswordChange(p application.PasswordChange) (application.PasswordChange, error) {
	digest, err := hex.DecodeString(p.InputSHA256)
	if err != nil || len(digest) != 32 {
		return application.PasswordChange{}, application.ErrProofInvalid
	}
	proof, err := hex.DecodeString(p.ProofSHA256)
	if err != nil || len(proof) != 32 || p.RequestID == uuid.Nil || p.AccountID != p.Candidate.ID || p.ActorSessionID == uuid.Nil {
		return application.PasswordChange{}, application.ErrProofInvalid
	}
	saved, found, err := readPasswordChange(t.db, p.RequestID, true)
	if err != nil {
		return application.PasswordChange{}, err
	}
	if found {
		if !hmac.Equal(saved.InputDigest, digest) || !hmac.Equal(saved.ProofDigest, proof) {
			return application.PasswordChange{}, application.ErrProofInvalid
		}
		return application.PasswordChange{}, application.ErrPasswordChangeExists
	}
	user, err := t.AccountByID(p.AccountID)
	if err != nil {
		return application.PasswordChange{}, err
	}
	session, err := t.SessionByID(p.ActorSessionID, true)
	if err != nil || !session.Valid(user, p.AcceptedAt) || user.Status != domain.StatusActive || user.EmailVerifiedAt.IsZero() || user.SessionEpoch != p.Candidate.SessionEpoch || user.Revision != p.Candidate.Revision || user.EmailVersion != p.Candidate.EmailVersion {
		return application.PasswordChange{}, application.ErrSessionInvalid
	}
	var pending bool
	if err := t.db.Raw(`SELECT EXISTS(SELECT 1 FROM identity.account_qualification_change WHERE target_id=? AND outcome='pending')`, p.AccountID).Scan(&pending).Error; err != nil {
		return application.PasswordChange{}, err
	}
	if pending {
		return application.PasswordChange{}, application.ErrPasswordChangeUnavailable
	}
	var head struct {
		Epoch             int64
		Blocked, Complete bool
	}
	query := t.db.Raw(`SELECT epoch,blocked,complete FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='identity' AND authority_kind='account' AND authority_id=? FOR UPDATE`, p.AccountID).Scan(&head)
	if query.Error != nil {
		return application.PasswordChange{}, query.Error
	}
	if query.RowsAffected != 1 || head.Epoch < 1 || head.Blocked || !head.Complete {
		return application.PasswordChange{}, application.ErrPasswordChangeUnavailable
	}
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte("identity.password.close/"+p.RequestID.String()+"/"+p.AccountID.String()))
	state, err := pgfence.NewAuthorityStore(t.db).BeginTx(t.db.Statement.Context, t.db, writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: p.AccountID, Epoch: head.Epoch}, id, true)
	if err != nil {
		return application.PasswordChange{}, err
	}
	p.CloseControl = state.Control
	candidate, err := json.Marshal(passwordCandidate{User: p.Candidate, Persistent: p.Persistent, IssueV2: p.IssueV2})
	if err != nil {
		return application.PasswordChange{}, err
	}
	defer clear(candidate)
	if err := t.db.Exec(`INSERT INTO identity.account_qualification_change(request_id,actor_id,actor_session_id,target_id,action,input_digest,proof_digest,candidate,close_coordination_id,close_expected_epoch,accepted_at) VALUES(?,?,?,?,'password_change',?,?,?::jsonb,?,?,?)`, p.RequestID, p.AccountID, p.ActorSessionID, p.AccountID, digest, proof, string(candidate), id, head.Epoch, p.AcceptedAt).Error; err != nil {
		return application.PasswordChange{}, fmt.Errorf("persist password source responsibility: %w", err)
	}
	if err := taskruntime.Enqueue(t.db.Statement.Context, t.db, application.PasswordChangeRecoveryTask, p.RequestID.String(), p.AcceptedAt); err != nil {
		return application.PasswordChange{}, err
	}
	return p, nil
}

// LoadPasswordChange accepts only both immutable original request proofs.
func (s *PasswordChangeStore) LoadPasswordChange(ctx context.Context, id uuid.UUID, input, proof []byte) (application.PasswordChange, bool, error) {
	var p application.PasswordChange
	found := false
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		row, exists, err := readPasswordChange(tx, id, false)
		found = exists
		if err != nil || !exists {
			return err
		}
		if len(input) != 32 || len(proof) != 32 || !hmac.Equal(input, row.InputDigest) || !hmac.Equal(proof, row.ProofDigest) {
			return application.ErrProofInvalid
		}
		p, err = row.record()
		return err
	})
	return p, found, err
}

// LoadAcceptedPasswordChange is the trusted worker's fixed original-ID reader.
func (s *PasswordChangeStore) LoadAcceptedPasswordChange(ctx context.Context, id uuid.UUID) (application.PasswordChange, error) {
	var p application.PasswordChange
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		row, found, err := readPasswordChange(tx, id, false)
		if err != nil {
			return err
		}
		if !found {
			return application.ErrPasswordChangeUnavailable
		}
		p, err = row.record()
		return err
	})
	return p, err
}
func requirePasswordControl(ctx context.Context, tx *gorm.DB, control writefence.Control, current bool) error {
	state, err := pgfence.NewAuthorityStore(tx).LoadCoordination(ctx, control.CoordinationID)
	if err != nil {
		return err
	}
	if !state.Complete || state.Control != control || len(state.Acknowledged) != len(writefence.Writers()) {
		return application.ErrPasswordChangeUnavailable
	}
	if !current {
		return nil
	}
	var head struct {
		Epoch             int64
		CoordinationID    uuid.UUID
		Blocked, Complete bool
	}
	query := tx.Raw(`SELECT epoch,coordination_id,blocked,complete FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer=? AND authority_kind=? AND authority_id=? FOR UPDATE`, control.Issuer, control.Kind, control.ID).Scan(&head)
	if query.Error != nil {
		return query.Error
	}
	if query.RowsAffected != 1 || head.Epoch != control.Epoch || head.CoordinationID != control.CoordinationID || head.Blocked != control.Blocked || !head.Complete {
		return application.ErrPasswordChangeUnavailable
	}
	return nil
}
func samePasswordBinding(a, b application.PasswordChange) bool {
	return a.RequestID == b.RequestID && a.AccountID == b.AccountID && a.ActorSessionID == b.ActorSessionID && a.InputSHA256 == b.InputSHA256 && a.ProofSHA256 == b.ProofSHA256 && a.CloseControl == b.CloseControl
}

// CommitPasswordChange performs the original mutation only after actual closure
// ACKs. New credentials and both opening decisions are persisted atomically.
func (s *PasswordChangeStore) CommitPasswordChange(ctx context.Context, p application.PasswordChange, mint func(application.AuthTx, domain.User, bool, time.Time) (application.PasswordChangeReply, error)) (application.PasswordChange, error) {
	var out application.PasswordChange
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		t := &emailAuthTx{authTx: &authTx{db: tx}}
		user, err := t.AccountByID(p.AccountID)
		if err != nil {
			return err
		}
		row, found, err := readPasswordChange(tx, p.RequestID, true)
		if err != nil {
			return err
		}
		if !found {
			return application.ErrPasswordChangeUnavailable
		}
		saved, err := row.record()
		if err != nil {
			return err
		}
		if !samePasswordBinding(p, saved) {
			return application.ErrProofInvalid
		}
		out = saved
		if saved.ReopenControl.CoordinationID != uuid.Nil {
			return nil
		}
		if err := requirePasswordControl(ctx, tx, saved.CloseControl, true); err != nil {
			return err
		}
		if user.Status != domain.StatusActive || user.EmailVerifiedAt.IsZero() || user.SessionEpoch != saved.Candidate.SessionEpoch || user.Revision != saved.Candidate.Revision || user.EmailVersion != saved.Candidate.EmailVersion {
			return domain.ErrRevisionConflict
		}
		reply, err := mint(t, user, saved.Persistent, saved.AcceptedAt)
		if err != nil {
			return err
		}
		if reply.SessionID == uuid.Nil || reply.KeyID == "" || len(reply.Ciphertext) == 0 || len(reply.Ciphertext) > 65536 {
			return application.ErrPasswordChangeUnavailable
		}
		reopenID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("identity.password.reopen/"+saved.RequestID.String()))
		reopen, err := pgfence.NewAuthorityStore(tx).BeginTx(ctx, tx, saved.CloseControl.Authority, reopenID, false)
		if err != nil {
			return err
		}
		sessionID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("identity.password.session/"+saved.RequestID.String()+"/"+reply.SessionID.String()))
		session, err := pgfence.NewAuthorityStore(tx).BeginTx(ctx, tx, writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Session, ID: reply.SessionID, Epoch: 0}, sessionID, false)
		if err != nil {
			return err
		}
		result := tx.Exec(`UPDATE identity.account_qualification_change SET reopen_coordination_id=?,reopen_expected_epoch=?,reopen_purpose='credential_replacement',replacement_session_id=?,replacement_coordination_id=?,response_key_id=?,response_ciphertext=? WHERE request_id=? AND reopen_coordination_id IS NULL AND outcome='pending'`, reopenID, saved.CloseControl.Epoch, reply.SessionID, sessionID, reply.KeyID, reply.Ciphertext, saved.RequestID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return application.ErrPasswordChangeUnavailable
		}
		out = saved
		out.ReopenControl = reopen.Control
		out.SessionControl = session.Control
		out.ReplacementSessionID = reply.SessionID
		out.ResponseKeyID = reply.KeyID
		out.ResponseCiphertext = append([]byte(nil), reply.Ciphertext...)
		return nil
	})
	return out, err
}

// CompletePasswordChange verifies persisted original ACKs and the current exact
// heads under the account/new-session locks before publishing success.
func (s *PasswordChangeStore) CompletePasswordChange(ctx context.Context, p application.PasswordChange) error {
	return identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		t := &authTx{db: tx}
		user, err := t.AccountByID(p.AccountID)
		if err != nil {
			return application.ErrSessionInvalid
		}
		session, err := t.SessionByID(p.ReplacementSessionID, true)
		if err != nil {
			return application.ErrSessionInvalid
		}
		if user.Status != domain.StatusActive || user.SessionEpoch != p.Candidate.SessionEpoch+1 || !session.Valid(user, time.Now().UTC()) {
			return application.ErrSessionInvalid
		}
		row, found, err := readPasswordChange(tx, p.RequestID, true)
		if err != nil {
			return err
		}
		if !found {
			return application.ErrPasswordChangeUnavailable
		}
		saved, err := row.record()
		if err != nil {
			return err
		}
		if !samePasswordBinding(saved, p) || saved.ReopenControl != p.ReopenControl || saved.SessionControl != p.SessionControl || saved.ReplacementSessionID != p.ReplacementSessionID || saved.ResponseKeyID != p.ResponseKeyID || !bytes.Equal(saved.ResponseCiphertext, p.ResponseCiphertext) {
			return application.ErrPasswordChangeUnavailable
		}
		if err := requirePasswordControl(ctx, tx, saved.CloseControl, false); err != nil {
			return err
		}
		if err := requirePasswordControl(ctx, tx, saved.ReopenControl, true); err != nil {
			return err
		}
		if err := requirePasswordControl(ctx, tx, saved.SessionControl, true); err != nil {
			return err
		}
		if saved.Completed {
			return nil
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if now.Before(saved.AcceptedAt) {
			now = saved.AcceptedAt
		}
		return tx.Exec(`UPDATE identity.account_qualification_change SET outcome='accepted',completed_at=? WHERE request_id=? AND outcome='pending'`, now, p.RequestID).Error
	})
}

var _ application.PasswordChangeTx = (*registrationAuthTx)(nil)
var _ application.PasswordChangeStore = (*PasswordChangeStore)(nil)

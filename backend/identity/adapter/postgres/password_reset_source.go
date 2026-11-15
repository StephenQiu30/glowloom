package postgres

import (
	"context"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// PasswordResetStore owns only mailbox-proven reset responsibilities.
type PasswordResetStore struct{ db *gorm.DB }

// NewPasswordResetStore injects the actual restricted Identity root pool.
func NewPasswordResetStore(db *gorm.DB) *PasswordResetStore { return &PasswordResetStore{db: db} }

type passwordResetRow struct {
	RequestID, AccountID, ProofOperationID, CloseCoordinationID uuid.UUID
	Action, Candidate                                           string
	InputDigest, ProofDigest                                    []byte
	CloseExpectedEpoch                                          int64
	ReopenCoordinationID                                        *uuid.UUID
	ReopenExpectedEpoch                                         *int64
	AcceptedAt                                                  time.Time
	MutationAppliedAt, CompletedAt                              *time.Time
}

func (r passwordResetRow) record() (application.PasswordResetResponsibility, error) {
	if r.Action != "password_reset" {
		return application.PasswordResetResponsibility{}, application.ErrProofInvalid
	}
	var candidate passwordCandidate
	if json.Unmarshal([]byte(r.Candidate), &candidate) != nil {
		return application.PasswordResetResponsibility{}, application.ErrPasswordChangeUnavailable
	}
	p := application.PasswordResetResponsibility{RequestID: r.RequestID, AccountID: r.AccountID, ProofOperationID: r.ProofOperationID, InputSHA256: hex.EncodeToString(r.InputDigest), ProofSHA256: hex.EncodeToString(r.ProofDigest), Candidate: candidate.User, AcceptedAt: r.AcceptedAt, Applied: r.MutationAppliedAt != nil, Completed: r.CompletedAt != nil, CloseControl: writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: r.AccountID, Epoch: r.CloseExpectedEpoch + 1}, CoordinationID: r.CloseCoordinationID, Blocked: true}}
	if r.ReopenCoordinationID != nil && r.ReopenExpectedEpoch != nil {
		p.ReopenControl = writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: r.AccountID, Epoch: *r.ReopenExpectedEpoch + 1}, CoordinationID: *r.ReopenCoordinationID}
	}
	return p, nil
}
func readPasswordReset(tx *gorm.DB, id uuid.UUID, lock bool) (passwordResetRow, bool, error) {
	var row passwordResetRow
	query := `SELECT request_id,target_id AS account_id,action,proof_operation_id,input_digest,proof_digest,candidate::text,close_coordination_id,close_expected_epoch,reopen_coordination_id,reopen_expected_epoch,accepted_at,mutation_applied_at,completed_at FROM identity.account_qualification_change WHERE request_id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	result := tx.Raw(query, id).Scan(&row)
	return row, result.RowsAffected == 1, result.Error
}

// PreparePasswordReset consumes no invented session and never enables an account.
func (t *registrationAuthTx) PreparePasswordReset(p application.PasswordResetResponsibility) (application.PasswordResetResponsibility, error) {
	digest, err := hex.DecodeString(p.InputSHA256)
	if err != nil || len(digest) != 32 {
		return application.PasswordResetResponsibility{}, application.ErrProofInvalid
	}
	proof, err := hex.DecodeString(p.ProofSHA256)
	if err != nil || len(proof) != 32 || p.RequestID == uuid.Nil || p.ProofOperationID == uuid.Nil || p.AccountID != p.Candidate.ID {
		return application.PasswordResetResponsibility{}, application.ErrProofInvalid
	}
	existing, found, err := readPasswordReset(t.db, p.RequestID, true)
	if err != nil {
		return application.PasswordResetResponsibility{}, err
	}
	if found {
		if !hmac.Equal(existing.InputDigest, digest) || !hmac.Equal(existing.ProofDigest, proof) {
			return application.PasswordResetResponsibility{}, application.ErrProofInvalid
		}
		return application.PasswordResetResponsibility{}, application.ErrPasswordChangeExists
	}
	user, err := t.AccountByID(p.AccountID)
	if err != nil {
		return application.PasswordResetResponsibility{}, err
	}
	if user.SessionEpoch != p.Candidate.SessionEpoch || user.Revision != p.Candidate.Revision || user.EmailVersion != p.Candidate.EmailVersion || user.Status != p.Candidate.Status || (user.Status != domain.StatusActive && user.Status != domain.StatusDisabled) {
		return application.PasswordResetResponsibility{}, application.ErrProofInvalid
	}
	var proved bool
	if err := t.db.Raw(`SELECT EXISTS(SELECT 1 FROM identity.auth_challenge WHERE operation_id=? AND purpose='password_reset' AND user_id=? AND email_key=? AND email_version=? AND credential_epoch=? AND consumed_at IS NOT NULL)`, p.ProofOperationID, p.AccountID, user.Email, user.EmailVersion, user.SessionEpoch).Scan(&proved).Error; err != nil {
		return application.PasswordResetResponsibility{}, err
	}
	if !proved {
		return application.PasswordResetResponsibility{}, application.ErrProofInvalid
	}
	var pending bool
	if err := t.db.Raw(`SELECT EXISTS(SELECT 1 FROM identity.account_qualification_change WHERE target_id=? AND outcome='pending')`, p.AccountID).Scan(&pending).Error; err != nil {
		return application.PasswordResetResponsibility{}, err
	}
	if pending {
		return application.PasswordResetResponsibility{}, application.ErrPasswordChangeUnavailable
	}
	var head struct {
		Epoch             int64
		Blocked, Complete bool
	}
	read := t.db.Raw(`SELECT epoch,blocked,complete FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='identity' AND authority_kind='account' AND authority_id=? FOR UPDATE`, p.AccountID).Scan(&head)
	if read.Error != nil {
		return application.PasswordResetResponsibility{}, read.Error
	}
	if read.RowsAffected != 1 || head.Epoch < 1 || !head.Complete || head.Blocked != (user.Status == domain.StatusDisabled) {
		return application.PasswordResetResponsibility{}, application.ErrPasswordChangeUnavailable
	}
	closeID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("identity.reset.close/"+p.RequestID.String()+"/"+p.AccountID.String()))
	state, err := pgfence.NewAuthorityStore(t.db).BeginTx(t.db.Statement.Context, t.db, writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: p.AccountID, Epoch: head.Epoch}, closeID, true)
	if err != nil {
		return application.PasswordResetResponsibility{}, err
	}
	p.CloseControl = state.Control
	candidate, err := json.Marshal(passwordCandidate{User: p.Candidate})
	if err != nil {
		return application.PasswordResetResponsibility{}, err
	}
	defer clear(candidate)
	if err := t.db.Exec(`INSERT INTO identity.account_qualification_change(request_id,actor_id,actor_session_id,target_id,action,proof_operation_id,input_digest,proof_digest,candidate,close_coordination_id,close_expected_epoch,accepted_at) VALUES(?,?,NULL,?,'password_reset',?,?,?,?::jsonb,?,?,?)`, p.RequestID, p.AccountID, p.AccountID, p.ProofOperationID, digest, proof, string(candidate), closeID, head.Epoch, p.AcceptedAt).Error; err != nil {
		return application.PasswordResetResponsibility{}, err
	}
	if err := taskruntime.Enqueue(t.db.Statement.Context, t.db, application.PasswordResetRecoveryTask, p.RequestID.String(), p.AcceptedAt); err != nil {
		return application.PasswordResetResponsibility{}, err
	}
	return p, nil
}

// LoadPasswordReset verifies both immutable original HMAC proofs.
func (s *PasswordResetStore) LoadPasswordReset(ctx context.Context, id uuid.UUID, input, proof []byte) (application.PasswordResetResponsibility, bool, error) {
	var p application.PasswordResetResponsibility
	found := false
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		row, exists, err := readPasswordReset(tx, id, false)
		found = exists
		if err != nil || !exists {
			return err
		}
		if len(input) != 32 || len(proof) != 32 || !hmac.Equal(row.InputDigest, input) || !hmac.Equal(row.ProofDigest, proof) {
			return application.ErrProofInvalid
		}
		p, err = row.record()
		return err
	})
	return p, found, err
}

// LoadAcceptedPasswordReset is an owner worker capability, never an anonymous route.
func (s *PasswordResetStore) LoadAcceptedPasswordReset(ctx context.Context, id uuid.UUID) (application.PasswordResetResponsibility, error) {
	var p application.PasswordResetResponsibility
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		row, found, err := readPasswordReset(tx, id, false)
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
func sameResetBinding(a, b application.PasswordResetResponsibility) bool {
	return a.RequestID == b.RequestID && a.AccountID == b.AccountID && a.ProofOperationID == b.ProofOperationID && a.InputSHA256 == b.InputSHA256 && a.ProofSHA256 == b.ProofSHA256 && a.CloseControl == b.CloseControl
}

// CommitPasswordReset applies the original reset atomically after four close ACKs.
// Disabled accounts have no reopening decision and receive no replacement session.
func (s *PasswordResetStore) CommitPasswordReset(ctx context.Context, p application.PasswordResetResponsibility, apply func(application.EmailTx, domain.User, time.Time) error) (application.PasswordResetResponsibility, error) {
	var result application.PasswordResetResponsibility
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		t := &emailAuthTx{authTx: &authTx{db: tx}}
		user, err := t.AccountByID(p.AccountID)
		if err != nil {
			return err
		}
		row, found, err := readPasswordReset(tx, p.RequestID, true)
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
		if !sameResetBinding(saved, p) {
			return application.ErrProofInvalid
		}
		result = saved
		if saved.Applied {
			return nil
		}
		if err := requirePasswordControl(ctx, tx, saved.CloseControl, true); err != nil {
			return err
		}
		if user.Status != saved.Candidate.Status || user.SessionEpoch != saved.Candidate.SessionEpoch || user.Revision != saved.Candidate.Revision || user.EmailVersion != saved.Candidate.EmailVersion {
			return domain.ErrRevisionConflict
		}
		if err := apply(t, user, saved.AcceptedAt); err != nil {
			return err
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if now.Before(saved.AcceptedAt) {
			now = saved.AcceptedAt
		}
		switch user.Status {
		case domain.StatusActive:
			reopenID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("identity.reset.reopen/"+saved.RequestID.String()))
			state, err := pgfence.NewAuthorityStore(tx).BeginTx(ctx, tx, saved.CloseControl.Authority, reopenID, false)
			if err != nil {
				return err
			}
			result.ReopenControl = state.Control
			if err := tx.Exec(`UPDATE identity.account_qualification_change SET mutation_applied_at=?,reopen_coordination_id=?,reopen_expected_epoch=?,reopen_purpose='credential_replacement' WHERE request_id=? AND mutation_applied_at IS NULL AND outcome='pending'`, now, reopenID, saved.CloseControl.Epoch, saved.RequestID).Error; err != nil {
				return err
			}
		case domain.StatusDisabled:
			if err := tx.Exec(`UPDATE identity.account_qualification_change SET mutation_applied_at=? WHERE request_id=? AND mutation_applied_at IS NULL AND outcome='pending'`, now, saved.RequestID).Error; err != nil {
				return err
			}
		default:
			return application.ErrPasswordChangeUnavailable
		}
		result.Applied = true
		return nil
	})
	return result, err
}

// CompletePasswordReset validates the final exact source decision before success.
func (s *PasswordResetStore) CompletePasswordReset(ctx context.Context, p application.PasswordResetResponsibility) error {
	return identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		user, err := (&authTx{db: tx}).AccountByID(p.AccountID)
		if err != nil {
			return err
		}
		row, found, err := readPasswordReset(tx, p.RequestID, true)
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
		if !sameResetBinding(saved, p) || !saved.Applied || saved.ReopenControl != p.ReopenControl || user.Status != saved.Candidate.Status || user.SessionEpoch != saved.Candidate.SessionEpoch+1 {
			return application.ErrPasswordChangeUnavailable
		}
		if err := requirePasswordControl(ctx, tx, saved.CloseControl, false); err != nil {
			return err
		}
		switch user.Status {
		case domain.StatusActive:
			if err := requirePasswordControl(ctx, tx, saved.ReopenControl, true); err != nil {
				return err
			}
		case domain.StatusDisabled:
			if saved.ReopenControl.CoordinationID != uuid.Nil {
				return application.ErrPasswordChangeUnavailable
			}
			if err := requirePasswordControl(ctx, tx, saved.CloseControl, true); err != nil {
				return err
			}
		default:
			return application.ErrPasswordChangeUnavailable
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

var _ application.PasswordResetSourceTx = (*registrationAuthTx)(nil)
var _ application.PasswordResetSourceStore = (*PasswordResetStore)(nil)

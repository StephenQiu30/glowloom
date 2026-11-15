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

type emailCandidate struct {
	User        domain.User
	OperationID uuid.UUID
	NewEmail    string
}
type emailChangeRow struct {
	RequestID, AccountID, ActorSessionID, CloseCoordinationID uuid.UUID
	InputDigest, ProofDigest                                  []byte
	Candidate, Action                                         string
	CloseExpectedEpoch                                        int64
	ReopenCoordinationID                                      *uuid.UUID
	ReopenExpectedEpoch                                       *int64
	AcceptedAt                                                time.Time
	MutationAppliedAt, CompletedAt                            *time.Time
}

func (r emailChangeRow) emailRecord() (application.EmailChangeResponsibility, error) {
	var candidate emailCandidate
	if r.Action != "email_change" || json.Unmarshal([]byte(r.Candidate), &candidate) != nil || candidate.OperationID == uuid.Nil || candidate.NewEmail == "" {
		return application.EmailChangeResponsibility{}, application.ErrProofInvalid
	}
	p := application.EmailChangeResponsibility{RequestID: r.RequestID, OperationID: candidate.OperationID, AccountID: r.AccountID, ActorSessionID: r.ActorSessionID, InputSHA256: hex.EncodeToString(r.InputDigest), ProofSHA256: hex.EncodeToString(r.ProofDigest), Candidate: candidate.User, NewEmail: candidate.NewEmail, AcceptedAt: r.AcceptedAt, Applied: r.MutationAppliedAt != nil, Completed: r.CompletedAt != nil, CloseControl: writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: r.AccountID, Epoch: r.CloseExpectedEpoch + 1}, CoordinationID: r.CloseCoordinationID, Blocked: true}}
	if r.ReopenCoordinationID != nil && r.ReopenExpectedEpoch != nil {
		p.ReopenControl = writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: r.AccountID, Epoch: *r.ReopenExpectedEpoch + 1}, CoordinationID: *r.ReopenCoordinationID}
	}
	return p, nil
}
func readEmailChange(tx *gorm.DB, id uuid.UUID, lock bool) (emailChangeRow, bool, error) {
	var row emailChangeRow
	query := `SELECT request_id,target_id AS account_id,actor_session_id,action,input_digest,proof_digest,candidate::text,close_coordination_id,close_expected_epoch,reopen_coordination_id,reopen_expected_epoch,accepted_at,mutation_applied_at,completed_at FROM identity.account_qualification_change WHERE request_id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	r := tx.Raw(query, id).Scan(&row)
	return row, r.RowsAffected == 1, r.Error
}

// PrepareEmailChange binds the original two verified proofs and reserves its
// destination under the same settings lock used by new registrations.
func (t *emailChangeTx) PrepareEmailChange(p application.EmailChangeResponsibility) error {
	digest, err := hex.DecodeString(p.InputSHA256)
	if err != nil || len(digest) != 32 {
		return application.ErrProofInvalid
	}
	proof, err := hex.DecodeString(p.ProofSHA256)
	if err != nil || len(proof) != 32 || p.RequestID == uuid.Nil || p.OperationID == uuid.Nil || p.AccountID != t.user.ID || p.ActorSessionID != t.session.ID || p.Candidate.ID != t.user.ID || p.Candidate.Revision != t.user.Revision || p.Candidate.SessionEpoch != t.user.SessionEpoch || p.Candidate.EmailVersion != t.user.EmailVersion || !t.session.RecentlyAuthenticated(t.user, p.AcceptedAt) {
		return application.ErrProofInvalid
	}
	row, found, err := readEmailChange(t.db, p.RequestID, true)
	if err != nil {
		return err
	}
	if found {
		if !hmac.Equal(row.InputDigest, digest) || !hmac.Equal(row.ProofDigest, proof) || row.Action != "email_change" {
			return application.ErrProofInvalid
		}
		return application.ErrPasswordChangeExists
	}
	if err := requireOpenAccountHead(t.db, p.AccountID); err != nil {
		return err
	}
	var epoch int64
	if err := t.db.Raw(`SELECT epoch FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='identity' AND authority_kind='account' AND authority_id=? FOR UPDATE`, p.AccountID).Scan(&epoch).Error; err != nil {
		return err
	}
	if epoch < 1 {
		return application.ErrPasswordChangeUnavailable
	}
	closeID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("identity.email.close/"+p.RequestID.String()+"/"+p.AccountID.String()))
	if _, err := pgfence.NewAuthorityStore(t.db).BeginTx(t.db.Statement.Context, t.db, writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: p.AccountID, Epoch: epoch}, closeID, true); err != nil {
		return err
	}
	candidate, err := json.Marshal(emailCandidate{User: p.Candidate, OperationID: p.OperationID, NewEmail: p.NewEmail})
	if err != nil {
		return err
	}
	defer clear(candidate)
	if err := t.db.Exec(`INSERT INTO identity.account_qualification_change(request_id,actor_id,actor_session_id,target_id,action,input_digest,proof_digest,candidate,close_coordination_id,close_expected_epoch,accepted_at) VALUES(?,?,?,?,'email_change',?,?,?::jsonb,?,?,?)`, p.RequestID, p.AccountID, p.ActorSessionID, p.AccountID, digest, proof, string(candidate), closeID, epoch, p.AcceptedAt).Error; err != nil {
		return err
	}
	return taskruntime.Enqueue(t.db.Statement.Context, t.db, application.EmailChangeRecoveryTask, p.RequestID.String(), p.AcceptedAt)
}

// LoadEmailChange requires the original immutable body and credential HMACs.
func (s *EmailChangeStore) LoadEmailChange(ctx context.Context, id uuid.UUID, digest, proof []byte) (application.EmailChangeResponsibility, bool, error) {
	var p application.EmailChangeResponsibility
	found := false
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		row, exists, err := readEmailChange(tx, id, false)
		found = exists
		if err != nil || !exists {
			return err
		}
		if len(digest) != 32 || len(proof) != 32 || !hmac.Equal(row.InputDigest, digest) || !hmac.Equal(row.ProofDigest, proof) {
			return application.ErrProofInvalid
		}
		p, err = row.emailRecord()
		return err
	})
	return p, found, err
}

// LoadAcceptedEmailChange is available only to an Identity recovery worker.
func (s *EmailChangeStore) LoadAcceptedEmailChange(ctx context.Context, id uuid.UUID) (application.EmailChangeResponsibility, error) {
	var p application.EmailChangeResponsibility
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		row, found, err := readEmailChange(tx, id, false)
		if err != nil {
			return err
		}
		if !found {
			return application.ErrPasswordChangeUnavailable
		}
		p, err = row.emailRecord()
		return err
	})
	return p, err
}
func sameEmailChange(a, b application.EmailChangeResponsibility) bool {
	return a.RequestID == b.RequestID && a.OperationID == b.OperationID && a.AccountID == b.AccountID && a.ActorSessionID == b.ActorSessionID && a.InputSHA256 == b.InputSHA256 && a.ProofSHA256 == b.ProofSHA256 && a.NewEmail == b.NewEmail && a.CloseControl == b.CloseControl
}

// CommitEmailChange retains the original atomic email, session, audit and two
// notification mutation after exact four-writer source closure.
func (s *EmailChangeStore) CommitEmailChange(ctx context.Context, p application.EmailChangeResponsibility, apply func(application.EmailChangeTx, time.Time) error) (application.EmailChangeResponsibility, error) {
	var out application.EmailChangeResponsibility
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		base := &emailAuthTx{authTx: &authTx{db: tx}}
		if _, err := base.RegistrationEnabled(); err != nil {
			return err
		}
		user, err := base.AccountByID(p.AccountID)
		if err != nil {
			return err
		}
		row, found, err := readEmailChange(tx, p.RequestID, true)
		if err != nil {
			return err
		}
		if !found {
			return application.ErrPasswordChangeUnavailable
		}
		saved, err := row.emailRecord()
		if err != nil {
			return err
		}
		if !sameEmailChange(saved, p) {
			return application.ErrProofInvalid
		}
		out = saved
		if saved.Applied {
			return nil
		}
		if err := requirePasswordControl(ctx, tx, saved.CloseControl, true); err != nil {
			return err
		}
		if !user.DeletedAt.IsZero() || user.Status != domain.StatusActive || user.Email != saved.Candidate.Email || user.Revision != saved.Candidate.Revision || user.SessionEpoch != saved.Candidate.SessionEpoch || user.EmailVersion != saved.Candidate.EmailVersion {
			return domain.ErrRevisionConflict
		}
		if err := apply(&emailChangeTx{emailAuthTx: base, user: user}, saved.AcceptedAt); err != nil {
			return err
		}
		reopenID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("identity.email.reopen/"+saved.RequestID.String()))
		state, err := pgfence.NewAuthorityStore(tx).BeginTx(ctx, tx, saved.CloseControl.Authority, reopenID, false)
		if err != nil {
			return err
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if now.Before(saved.AcceptedAt) {
			now = saved.AcceptedAt
		}
		if err := tx.Exec(`UPDATE identity.account_qualification_change SET mutation_applied_at=?,reopen_coordination_id=?,reopen_expected_epoch=?,reopen_purpose='credential_replacement' WHERE request_id=? AND outcome='pending' AND mutation_applied_at IS NULL`, now, reopenID, saved.CloseControl.Epoch, saved.RequestID).Error; err != nil {
			return err
		}
		out.Applied, out.ReopenControl = true, state.Control
		return nil
	})
	return out, err
}

// CompleteEmailChange rereads the actual current head and immutable close ACKs.
func (s *EmailChangeStore) CompleteEmailChange(ctx context.Context, p application.EmailChangeResponsibility) error {
	return identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		user, err := (&authTx{db: tx}).AccountByID(p.AccountID)
		if err != nil {
			return err
		}
		row, found, err := readEmailChange(tx, p.RequestID, true)
		if err != nil {
			return err
		}
		if !found {
			return application.ErrPasswordChangeUnavailable
		}
		saved, err := row.emailRecord()
		if err != nil {
			return err
		}
		if !sameEmailChange(saved, p) || !saved.Applied || saved.ReopenControl != p.ReopenControl || !user.DeletedAt.IsZero() || user.Status != domain.StatusActive || user.Email != saved.NewEmail || user.Revision != saved.Candidate.Revision+1 || user.SessionEpoch != saved.Candidate.SessionEpoch+1 || user.EmailVersion != saved.Candidate.EmailVersion+1 {
			return application.ErrPasswordChangeUnavailable
		}
		for _, check := range []struct {
			control writefence.Control
			current bool
		}{{saved.CloseControl, false}, {saved.ReopenControl, true}} {
			if err := requirePasswordControl(ctx, tx, check.control, check.current); err != nil {
				return err
			}
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

var _ application.EmailChangeSourceStore = (*EmailChangeStore)(nil)
var _ application.EmailChangeSourceTx = (*emailChangeTx)(nil)

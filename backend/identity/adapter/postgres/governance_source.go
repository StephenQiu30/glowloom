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

type governanceCandidate struct {
	Actor, User domain.User
	Reason      string
}
type governanceSourceRow struct {
	RequestID, ActorID, ActorSessionID, AccountID, CloseCoordinationID uuid.UUID
	Action, Candidate                                                  string
	ProofOperationID                                                   *uuid.UUID
	InputDigest, ProofDigest                                           []byte
	CloseExpectedEpoch                                                 int64
	ReopenCoordinationID                                               *uuid.UUID
	ReopenExpectedEpoch                                                *int64
	AcceptedAt                                                         time.Time
	MutationAppliedAt, CompletedAt                                     *time.Time
	CreationReceipt                                                    json.RawMessage
	Outcome                                                            string
}

func (r governanceSourceRow) record() (application.GovernanceResponsibility, error) {
	var candidate governanceCandidate
	if (r.Action != "revoke_sessions" && r.Action != "revoke_admin" && r.Action != "disable" && r.Action != "enable" && r.Action != "delete" && r.Action != "self_delete") || json.Unmarshal([]byte(r.Candidate), &candidate) != nil || candidate.Actor.ID != r.ActorID || candidate.User.ID != r.AccountID {
		return application.GovernanceResponsibility{}, application.ErrProofInvalid
	}
	p := application.GovernanceResponsibility{RequestID: r.RequestID, ActorSessionID: r.ActorSessionID, Actor: candidate.Actor, Candidate: candidate.User, Action: r.Action, Reason: candidate.Reason, InputSHA256: hex.EncodeToString(r.InputDigest), ProofSHA256: hex.EncodeToString(r.ProofDigest), AcceptedAt: r.AcceptedAt, Applied: r.MutationAppliedAt != nil, Completed: r.CompletedAt != nil, CloseControl: writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: r.AccountID, Epoch: r.CloseExpectedEpoch + 1}, CoordinationID: r.CloseCoordinationID, Blocked: true}}
	if r.Action == "self_delete" {
		if r.ProofOperationID == nil || *r.ProofOperationID == uuid.Nil || r.ActorID != r.AccountID || r.ActorSessionID == uuid.Nil {
			return p, application.ErrProofInvalid
		}
		p.ProofOperationID = *r.ProofOperationID
	}
	if r.ReopenCoordinationID != nil && r.ReopenExpectedEpoch != nil {
		p.ReopenControl = writefence.Control{Authority: writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: r.AccountID, Epoch: *r.ReopenExpectedEpoch + 1}, CoordinationID: *r.ReopenCoordinationID}
	}
	if len(r.CreationReceipt) > 0 {
		var receipt application.GovernanceTeamReceipt
		if json.Unmarshal(r.CreationReceipt, &receipt) != nil || receipt.RequestID != p.RequestID || receipt.AccountID != p.Candidate.ID || receipt.Action != governanceIntent(p).Action || receipt.InputSHA256 != p.InputSHA256 || receipt.IdentityClose != p.CloseControl {
			return p, application.ErrProofInvalid
		}
		p.CreationReceipt = &receipt
		p.Rejected = receipt.RejectedDuties
		if p.Rejected {
			p.ReopenControl.Blocked = p.Candidate.Status == domain.StatusDisabled
		}
	}
	if r.Outcome == "rejected_duties" && !p.Rejected {
		return p, application.ErrProofInvalid
	}
	return p, nil
}
func readGovernanceSource(tx *gorm.DB, id uuid.UUID, lock bool) (governanceSourceRow, bool, error) {
	var row governanceSourceRow
	query := `SELECT request_id,actor_id,actor_session_id,target_id AS account_id,action,proof_operation_id,input_digest,proof_digest,candidate::text,close_coordination_id,close_expected_epoch,reopen_coordination_id,reopen_expected_epoch,accepted_at,mutation_applied_at,completed_at,creation_receipt,outcome FROM identity.account_qualification_change WHERE request_id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	r := tx.Raw(query, id).Scan(&row)
	return row, r.RowsAffected == 1, r.Error
}

// PrepareGovernance closes target admission inside its original locked platform
// transaction. No user, role, session, notification or audit changes here.
func (t *governanceTx) PrepareGovernance(p application.GovernanceResponsibility) error {
	if !t.source || p.RequestID == uuid.Nil || p.Actor.ID != t.actor.ID || p.ActorSessionID != t.principal.SessionID || (p.Action != "revoke_admin" && p.Action != "revoke_sessions" && p.Action != "disable" && p.Action != "enable" && p.Action != "delete") {
		return application.ErrInvalidGovernance
	}
	if err := t.RequirePlatform(); err != nil {
		return err
	}
	return t.prepareQualification(p)
}

// prepareQualification is called only after the original platform or self-proof
// policy has succeeded under the shared settings/account/session locks.
func (t *governanceTx) prepareQualification(p application.GovernanceResponsibility) error {
	digest, err := hex.DecodeString(p.InputSHA256)
	if err != nil || len(digest) != 32 {
		return application.ErrProofInvalid
	}
	proof, err := hex.DecodeString(p.ProofSHA256)
	if err != nil || len(proof) != 32 {
		return application.ErrProofInvalid
	}
	row, found, err := readGovernanceSource(t.db, p.RequestID, true)
	if err != nil {
		return err
	}
	if found {
		if !hmac.Equal(row.InputDigest, digest) || !hmac.Equal(row.ProofDigest, proof) {
			return application.ErrProofInvalid
		}
		return application.ErrPasswordChangeExists
	}
	user, err := t.AccountByID(p.Candidate.ID)
	if err != nil {
		return err
	}
	if user.Revision != p.Candidate.Revision || user.SessionEpoch != p.Candidate.SessionEpoch || user.PlatformEpoch != p.Candidate.PlatformEpoch || user.Status != p.Candidate.Status {
		return domain.ErrRevisionConflict
	}
	var pending bool
	if err := t.db.Raw(`SELECT EXISTS(SELECT 1 FROM identity.account_qualification_change WHERE target_id=? AND outcome='pending')`, user.ID).Scan(&pending).Error; err != nil {
		return err
	}
	if pending {
		return application.ErrPasswordChangeUnavailable
	}
	if p.Action == "disable" || p.Action == "enable" || p.Action == "delete" || p.Action == "self_delete" {
		// Retain the original settings-serialized team qualification order. A
		// second target must not close first and then wait on the first candidate.
		if err := t.db.Raw(`SELECT EXISTS(SELECT 1 FROM identity.account_qualification_change WHERE action IN ('disable','enable','delete','self_delete') AND outcome='pending')`).Scan(&pending).Error; err != nil {
			return err
		}
		if pending {
			return application.ErrPasswordChangeUnavailable
		}
	}
	var head struct {
		Epoch             int64
		Blocked, Complete bool
	}
	r := t.db.Raw(`SELECT epoch,blocked,complete FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='identity' AND authority_kind='account' AND authority_id=? FOR UPDATE`, user.ID).Scan(&head)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 || !head.Complete || head.Epoch < 1 || head.Blocked != (user.Status == domain.StatusDisabled) {
		return application.ErrPasswordChangeUnavailable
	}
	closeID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("identity.platform.close/"+p.Action+"/"+p.RequestID.String()+"/"+user.ID.String()))
	if _, err := pgfence.NewAuthorityStore(t.db).BeginTx(t.db.Statement.Context, t.db, writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: user.ID, Epoch: head.Epoch}, closeID, true); err != nil {
		return err
	}
	candidate, err := json.Marshal(governanceCandidate{Actor: p.Actor, User: p.Candidate, Reason: p.Reason})
	if err != nil {
		return err
	}
	defer clear(candidate)
	var operation *uuid.UUID
	kind := application.GovernanceRecoveryTask
	if p.Action == "self_delete" {
		operation = &p.ProofOperationID
		kind = application.SelfDeletionRecoveryTask
	}
	if err := t.db.Exec(`INSERT INTO identity.account_qualification_change(request_id,actor_id,actor_session_id,proof_operation_id,target_id,action,input_digest,proof_digest,candidate,close_coordination_id,close_expected_epoch,accepted_at) VALUES(?,?,?,?,?,?,?,?,?::jsonb,?,?,?)`, p.RequestID, p.Actor.ID, p.ActorSessionID, operation, user.ID, p.Action, digest, proof, string(candidate), closeID, head.Epoch, p.AcceptedAt).Error; err != nil {
		return err
	}
	return taskruntime.Enqueue(t.db.Statement.Context, t.db, kind, p.RequestID.String(), p.AcceptedAt)
}

// LoadGovernance matches only immutable original body and credential proofs.
func (s *GovernanceStore) LoadGovernance(ctx context.Context, id uuid.UUID, input, proof []byte) (application.GovernanceResponsibility, bool, error) {
	var out application.GovernanceResponsibility
	found := false
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		row, exists, err := readGovernanceSource(tx, id, false)
		found = exists
		if err != nil || !exists {
			return err
		}
		if len(input) != 32 || len(proof) != 32 || !hmac.Equal(row.InputDigest, input) || !hmac.Equal(row.ProofDigest, proof) {
			return application.ErrProofInvalid
		}
		out, err = row.record()
		return err
	})
	return out, found, err
}

// LoadAcceptedGovernance is the Identity recovery worker's original-ID reader.
func (s *GovernanceStore) LoadAcceptedGovernance(ctx context.Context, id uuid.UUID) (application.GovernanceResponsibility, error) {
	var out application.GovernanceResponsibility
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		row, found, err := readGovernanceSource(tx, id, false)
		if err != nil {
			return err
		}
		if !found {
			return application.ErrPasswordChangeUnavailable
		}
		out, err = row.record()
		return err
	})
	return out, err
}
func sameGovernance(a, b application.GovernanceResponsibility) bool {
	return a.RequestID == b.RequestID && a.Actor.ID == b.Actor.ID && a.ActorSessionID == b.ActorSessionID && a.Candidate.ID == b.Candidate.ID && a.Action == b.Action && a.InputSHA256 == b.InputSHA256 && a.ProofSHA256 == b.ProofSHA256 && a.CloseControl == b.CloseControl
}

// CommitGovernance applies exactly the accepted local action after actual four
// close ACKs, preserving the original atomic user mutation, audit and encrypted mail.
func (s *GovernanceStore) CommitGovernance(ctx context.Context, p application.GovernanceResponsibility, apply func(application.GovernanceTx, domain.User, time.Time) error) (application.GovernanceResponsibility, error) {
	var out application.GovernanceResponsibility
	err := identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		auth := &authTx{db: tx}
		if _, err := auth.RegistrationEnabled(); err != nil {
			return err
		}
		user, err := auth.AccountByID(p.Candidate.ID)
		if err != nil {
			return err
		}
		row, found, err := readGovernanceSource(tx, p.RequestID, true)
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
		if !sameGovernance(saved, p) {
			return application.ErrProofInvalid
		}
		out = saved
		if saved.Applied {
			return nil
		}
		if saved.Action == "disable" || saved.Action == "enable" || saved.Action == "delete" || saved.Action == "self_delete" {
			if saved.CreationReceipt == nil || saved.Rejected || !saved.CreationReceipt.Matches(governanceIntent(saved)) {
				return application.ErrPasswordChangeUnavailable
			}
		}
		if err := requirePasswordControl(ctx, tx, saved.CloseControl, true); err != nil {
			return err
		}
		if user.Revision != saved.Candidate.Revision || user.SessionEpoch != saved.Candidate.SessionEpoch || user.PlatformEpoch != saved.Candidate.PlatformEpoch || user.Status != saved.Candidate.Status {
			return domain.ErrRevisionConflict
		}
		g := &governanceTx{authTx: auth, actor: saved.Actor, source: true}
		if (saved.Action == "revoke_admin" || saved.Action == "disable" || saved.Action == "delete" || saved.Action == "self_delete") && user.Status == domain.StatusActive && user.PlatformRole == "platform_admin" && !user.EmailVerifiedAt.IsZero() {
			n, err := g.AvailablePlatformAdmins()
			if err != nil {
				return err
			}
			if n < 1 {
				return application.ErrLastPlatformAdmin
			}
		}
		if err := apply(g, user, saved.AcceptedAt); err != nil {
			return err
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if now.Before(saved.AcceptedAt) {
			now = saved.AcceptedAt
		}
		switch {
		case postGovernanceActive(saved):
			reopenID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("identity.platform.reopen/"+saved.RequestID.String()))
			state, err := pgfence.NewAuthorityStore(tx).BeginTx(ctx, tx, saved.CloseControl.Authority, reopenID, false)
			if err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE identity.account_qualification_change SET mutation_applied_at=?,reopen_coordination_id=?,reopen_expected_epoch=?,reopen_purpose='qualification_restore' WHERE request_id=? AND outcome='pending' AND mutation_applied_at IS NULL`, now, reopenID, saved.CloseControl.Epoch, saved.RequestID).Error; err != nil {
				return err
			}
			out.ReopenControl = state.Control
		default:
			if err := tx.Exec(`UPDATE identity.account_qualification_change SET mutation_applied_at=? WHERE request_id=? AND outcome='pending' AND mutation_applied_at IS NULL`, now, saved.RequestID).Error; err != nil {
				return err
			}
		}
		out.Applied = true
		return nil
	})
	return out, err
}

// CompleteGovernance validates actual current decisions before saving the public
// exact command receipt. An older source operation cannot reopen a later closure.
func (s *GovernanceStore) CompleteGovernance(ctx context.Context, p application.GovernanceResponsibility, receipt application.UserCommandReceipt) error {
	return identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		user, err := readGovernanceResultUser(tx, p.Candidate.ID)
		if err != nil {
			return err
		}
		row, found, err := readGovernanceSource(tx, p.RequestID, true)
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
		expectedStatus := saved.Candidate.Status
		switch saved.Action {
		case "disable", "delete", "self_delete":
			expectedStatus = domain.StatusDisabled
		case "enable":
			expectedStatus = domain.StatusActive
		}
		if !sameGovernance(saved, p) || !saved.Applied || saved.Rejected || saved.ReopenControl != p.ReopenControl || user.Status != expectedStatus || user.Revision != saved.Candidate.Revision+1 || receipt.RequestID != saved.RequestID || receipt.TargetID != saved.Candidate.ID || receipt.Action != saved.Action {
			return application.ErrPasswordChangeUnavailable
		}
		switch saved.Action {
		case "revoke_sessions":
			if user.SessionEpoch != saved.Candidate.SessionEpoch+1 || user.PlatformEpoch != saved.Candidate.PlatformEpoch {
				return application.ErrPasswordChangeUnavailable
			}
		case "revoke_admin":
			if user.SessionEpoch != saved.Candidate.SessionEpoch || user.PlatformEpoch != saved.Candidate.PlatformEpoch+1 || user.PlatformRole != "creator" {
				return application.ErrPasswordChangeUnavailable
			}
		case "disable", "delete", "self_delete":
			if user.SessionEpoch != saved.Candidate.SessionEpoch+1 || user.PlatformEpoch != saved.Candidate.PlatformEpoch+1 || user.PlatformRole != saved.Candidate.PlatformRole {
				return application.ErrPasswordChangeUnavailable
			}
			if saved.Action == "delete" && user.DeletedAt.IsZero() {
				return application.ErrPasswordChangeUnavailable
			}
		case "enable":
			if user.SessionEpoch != saved.Candidate.SessionEpoch || user.PlatformEpoch != saved.Candidate.PlatformEpoch || user.PlatformRole != saved.Candidate.PlatformRole {
				return application.ErrPasswordChangeUnavailable
			}
		default:
			return application.ErrInvalidGovernance
		}
		if err := requirePasswordControl(ctx, tx, saved.CloseControl, false); err != nil {
			return err
		}
		current := saved.ReopenControl
		if !postGovernanceActive(saved) {
			current = saved.CloseControl
		}
		if err := requirePasswordControl(ctx, tx, current, true); err != nil {
			return err
		}
		if saved.Completed {
			return nil
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if now.Before(saved.AcceptedAt) {
			now = saved.AcceptedAt
		}
		g := &governanceTx{authTx: &authTx{db: tx}, actor: saved.Actor, source: true}
		if err := g.SaveUserCommand(receipt, now); err != nil {
			return err
		}
		return tx.Exec(`UPDATE identity.account_qualification_change SET outcome='accepted',completed_at=? WHERE request_id=? AND outcome='pending'`, now, saved.RequestID).Error
	})
}

var _ application.GovernanceSourceStore = (*GovernanceStore)(nil)
var _ application.GovernanceSourceTx = (*governanceTx)(nil)

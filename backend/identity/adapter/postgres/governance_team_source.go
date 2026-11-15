package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func governanceIntent(p application.GovernanceResponsibility) application.AccountQualificationIntent {
	return application.AccountQualificationIntent{RequestID: p.RequestID, AccountID: p.Candidate.ID, Action: p.Action, InputSHA256: p.InputSHA256, CloseControl: p.CloseControl, EligibleAfter: p.Action == "enable" && !p.Candidate.EmailVerifiedAt.IsZero() && p.Candidate.DeletedAt.IsZero()}
}
func readGovernanceResultUser(tx *gorm.DB, id uuid.UUID) (domain.User, error) {
	var user domain.User
	read := tx.Raw(`SELECT id,email_key AS email,email_verified_at,status,revision,session_epoch,platform_epoch,platform_role,deleted_at FROM identity."user" WHERE id=? FOR UPDATE`, id).Scan(&user)
	if read.Error != nil {
		return user, read.Error
	}
	if read.RowsAffected != 1 {
		return user, application.ErrPasswordChangeUnavailable
	}
	return user, nil
}

func postGovernanceActive(p application.GovernanceResponsibility) bool {
	if p.Rejected {
		return p.Candidate.Status == domain.StatusActive
	}
	switch p.Action {
	case "disable", "delete":
		return false
	case "enable":
		return true
	default:
		return p.Candidate.Status == domain.StatusActive
	}
}

// AdoptGovernanceTeamReceipt persists only the exact fixed Creation outcome.
// Rejection restores the original account policy at a strictly higher generation;
// it never changes shared scope policy or interprets an unknown peer as negative.
func (s *GovernanceStore) AdoptGovernanceTeamReceipt(ctx context.Context, p application.GovernanceResponsibility, receipt application.GovernanceTeamReceipt) (application.GovernanceResponsibility, error) {
	if !receipt.Matches(governanceIntent(p)) {
		return p, application.ErrProofInvalid
	}
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
		if saved.CreationReceipt != nil {
			old, _ := json.Marshal(saved.CreationReceipt)
			next, _ := json.Marshal(receipt)
			if string(old) != string(next) {
				return application.ErrProofInvalid
			}
			return nil
		}
		if saved.Applied || saved.Completed || user.Revision != saved.Candidate.Revision || user.SessionEpoch != saved.Candidate.SessionEpoch || user.PlatformEpoch != saved.Candidate.PlatformEpoch || user.Status != saved.Candidate.Status {
			return application.ErrGovernanceConflict
		}
		if err := requirePasswordControl(ctx, tx, saved.CloseControl, true); err != nil {
			return err
		}
		encoded, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		if receipt.RejectedDuties {
			coord := uuid.NewSHA1(saved.RequestID, []byte("identity.platform.restore_rejected_duties"))
			state, err := pgfence.NewAuthorityStore(tx).BeginTx(ctx, tx, saved.CloseControl.Authority, coord, saved.Candidate.Status == domain.StatusDisabled)
			if err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE identity.account_qualification_change SET creation_receipt=?::jsonb,reopen_coordination_id=?,reopen_expected_epoch=?,reopen_purpose='restore_rejection' WHERE request_id=? AND outcome='pending' AND creation_receipt IS NULL`, string(encoded), coord, saved.CloseControl.Epoch, saved.RequestID).Error; err != nil {
				return err
			}
			out.Rejected = true
			out.ReopenControl = state.Control
		} else if err := tx.Exec(`UPDATE identity.account_qualification_change SET creation_receipt=?::jsonb WHERE request_id=? AND outcome='pending' AND creation_receipt IS NULL`, string(encoded), saved.RequestID).Error; err != nil {
			return err
		}
		out.CreationReceipt = &receipt
		return nil
	})
	return out, err
}

// CompleteGovernanceRejection returns the original duties conflict only after
// actual current account restoration and four original writer receipts.
func (s *GovernanceStore) CompleteGovernanceRejection(ctx context.Context, p application.GovernanceResponsibility) error {
	return identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		user, err := (&authTx{db: tx}).AccountByID(p.Candidate.ID)
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
		if !sameGovernance(saved, p) || !saved.Rejected || saved.Applied || saved.CreationReceipt == nil || !saved.CreationReceipt.Matches(governanceIntent(saved)) || saved.ReopenControl != p.ReopenControl || saved.ReopenControl.Blocked != (saved.Candidate.Status == domain.StatusDisabled) {
			return application.ErrProofInvalid
		}
		if user.Revision != saved.Candidate.Revision || user.SessionEpoch != saved.Candidate.SessionEpoch || user.PlatformEpoch != saved.Candidate.PlatformEpoch || user.Status != saved.Candidate.Status || user.PlatformRole != saved.Candidate.PlatformRole {
			return application.ErrGovernanceConflict
		}
		if err := requirePasswordControl(ctx, tx, saved.CloseControl, false); err != nil {
			return err
		}
		if err := requirePasswordControl(ctx, tx, saved.ReopenControl, true); err != nil {
			return err
		}
		if saved.Completed {
			return nil
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if now.Before(saved.AcceptedAt) {
			now = saved.AcceptedAt
		}
		return tx.Exec(`UPDATE identity.account_qualification_change SET outcome='rejected_duties',completed_at=? WHERE request_id=? AND outcome='pending' AND mutation_applied_at IS NULL`, now, saved.RequestID).Error
	})
}

var _ application.GovernanceTeamSourceStore = (*GovernanceStore)(nil)

package postgres

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// AccountDeletionStore serializes identity revocation and retained-resource soft deletion.
type AccountDeletionStore struct{ db *gorm.DB }

// NewAccountDeletionStore constructs AccountDeletionStore with explicit caller-supplied dependencies.
func NewAccountDeletionStore(db *gorm.DB) *AccountDeletionStore { return &AccountDeletionStore{db: db} }

type accountDeletionTx struct {
	*emailAuthTx
	user    domain.User
	session domain.AuthSession
}

func (t *accountDeletionTx) Account() domain.User        { return t.user }
func (t *accountDeletionTx) Session() domain.AuthSession { return t.session }

// DeletionTransaction locks governance, identity and session state before mutation.
func (s *AccountDeletionStore) DeletionTransaction(ctx context.Context, actor application.Principal, now time.Time, run func(application.AccountDeletionTx) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		base := &emailAuthTx{authTx: &authTx{db: tx}}
		// Serialize governance before taking user/session/team locks. Every operation
		// affecting administrator eligibility must use the same settings lock first.
		if _, err := base.RegistrationEnabled(); err != nil {
			return err
		}
		u, err := lockSelfActor(tx, actor, now)
		if err != nil {
			return err
		}
		var session domain.AuthSession
		if err := tx.Raw(`SELECT * FROM identity.user_session WHERE id=?`, actor.SessionID).Scan(&session).Error; err != nil {
			return err
		}
		return run(&accountDeletionTx{emailAuthTx: base, user: u, session: session})
	})
}

func (t *accountDeletionTx) Impact() (application.DeletionImpact, error) {
	out := application.DeletionImpact{Revision: t.user.Revision, TeamIDs: []uuid.UUID{}, BlockingDuties: []string{}}
	if t.user.PlatformRole == "platform_admin" {
		var count int64
		if err := t.db.Raw(`SELECT count(*) FROM identity."user" WHERE platform_role='platform_admin' AND status='active' AND email_verified_at IS NOT NULL AND NOT is_delete`).Scan(&count).Error; err != nil {
			return out, err
		}
		if count <= 1 {
			out.BlockingDuties = append(out.BlockingDuties, "platform_admin")
		}
	}
	var teams []struct{ ID uuid.UUID }
	if err := t.db.Raw(`SELECT t.id FROM workspace.team t JOIN workspace.team_member m ON m.team_id=t.id WHERE m.user_id=? AND m.status<>'removed' ORDER BY t.id FOR UPDATE OF t,m`, t.user.ID).Scan(&teams).Error; err != nil {
		return out, err
	}
	for _, team := range teams {
		out.TeamIDs = append(out.TeamIDs, team.ID)
		var last bool
		if err := t.db.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.team_member m WHERE m.team_id=? AND m.user_id=? AND m.role='team_admin' AND m.status='active') AND NOT EXISTS(SELECT 1 FROM workspace.team_member m JOIN identity."user" u ON u.id=m.user_id WHERE m.team_id=? AND m.user_id<>? AND m.role='team_admin' AND m.status='active' AND u.status='active' AND NOT u.is_delete AND u.email_verified_at IS NOT NULL)`, team.ID, t.user.ID, team.ID, t.user.ID).Scan(&last).Error; err != nil {
			return out, err
		}
		if last {
			out.BlockingDuties = append(out.BlockingDuties, "team_admin:"+team.ID.String())
		}
	}
	return out, nil
}

func (t *accountDeletionTx) SoftDelete(now time.Time) error {
	if t.user.SessionEpoch >= math.MaxInt32 || t.user.Revision >= math.MaxInt32 {
		return domain.ErrInvalidAccountState
	}
	result := t.db.Exec(`UPDATE identity."user" SET is_delete=true,deleted_at=?,status='disabled',session_epoch=session_epoch+1,platform_epoch=platform_epoch+1,revision=revision+1,update_time=? WHERE id=? AND revision=? AND NOT is_delete`, now, now, t.user.ID, t.user.Revision)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrRevisionConflict
	}
	if err := t.RevokeAccountSessions(t.user.ID, now); err != nil {
		return err
	}
	if err := t.db.Exec(`UPDATE identity.auth_challenge SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL AND consumed_at IS NULL`, now, t.user.ID).Error; err != nil {
		return err
	}
	if err := t.db.Exec(`UPDATE workspace.team SET auth_epoch=auth_epoch+1,revision=revision+1,update_time=? WHERE id IN (SELECT team_id FROM workspace.team_member WHERE user_id=? AND status<>'removed')`, now, t.user.ID).Error; err != nil {
		return err
	}
	var governance struct{ Invitations, Proposals bool }
	if err := t.db.Raw(`SELECT to_regclass('identity.invitation') IS NOT NULL AS invitations,to_regclass('identity.authority_proposal') IS NOT NULL AS proposals`).Scan(&governance).Error; err != nil {
		return err
	}
	if governance.Invitations {
		if err := t.db.Exec(`UPDATE identity.invitation SET status='revoked',revision=revision+1,updated_at=? WHERE status='pending' AND (inviter_id=? OR email_key=?)`, now, t.user.ID, t.user.Email).Error; err != nil {
			return err
		}
	}
	if governance.Proposals {
		if err := t.db.Exec(`UPDATE identity.authority_proposal SET status='cancelled',revision=revision+1,updated_at=? WHERE status='pending' AND (source_user_id=? OR target_user_id=?)`, now, t.user.ID, t.user.ID).Error; err != nil {
			return err
		}
	}
	if err := t.db.Exec(`UPDATE workspace.team_member SET status='removed',auth_epoch=auth_epoch+1,revision=revision+1,update_time=? WHERE user_id=? AND status<>'removed'`, now, t.user.ID).Error; err != nil {
		return err
	}
	return appendSelfEvent(t.db, t.user.ID, "account.deleted", now)
}

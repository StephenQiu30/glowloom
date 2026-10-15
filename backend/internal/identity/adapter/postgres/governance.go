package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// TeamGovernanceFactory binds workspace mutations to the current identity transaction.
type TeamGovernanceFactory func(*gorm.DB) application.TeamGovernance

// GovernanceStore shares the platform singleton lock with team changes and deletion.
type GovernanceStore struct {
	db    *gorm.DB
	teams TeamGovernanceFactory
}

// NewGovernanceStore injects the existing database and transaction-bound workspace gateway.
func NewGovernanceStore(db *gorm.DB, teams TeamGovernanceFactory) *GovernanceStore {
	return &GovernanceStore{db: db, teams: teams}
}

type governanceTx struct {
	*authTx
	actor     domain.User
	principal application.Principal
	settings  application.PlatformSettings
	teams     application.TeamGovernance
}

// GovernanceTransaction holds the shared settings lock before checking current session and authority.
func (s *GovernanceStore) GovernanceTransaction(ctx context.Context, actor application.Principal, recent bool, now time.Time, run func(application.GovernanceTx) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var settings application.PlatformSettings
		r := tx.Raw(`SELECT registration_enabled,team_creation_enabled,revision FROM identity.platform_settings WHERE singleton FOR UPDATE`).Scan(&settings)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrForbidden
		}
		user, err := lockSelfActor(tx, actor, now)
		if err != nil {
			return err
		}
		if recent {
			if err := NewStore(tx).CheckRecentSession(ctx, actor, now); err != nil {
				return err
			}
		}
		g := &governanceTx{authTx: &authTx{db: tx}, actor: user, principal: actor, settings: settings}
		if s.teams != nil {
			g.teams = s.teams(tx)
		}
		return run(g)
	})
}

// Actor defines the explicit governance contract.
func (t *governanceTx) Actor() domain.User { return t.actor }

// RequirePlatform defines the explicit governance contract.
func (t *governanceTx) RequirePlatform() error {
	if t.actor.PlatformRole != "platform_admin" || t.actor.PlatformEpoch != t.principal.PlatformEpoch {
		return application.ErrForbidden
	}
	return nil
}

// User defines the explicit governance contract.
func (t *governanceTx) User(id uuid.UUID) (domain.User, error) { return t.AccountByID(id) }

// TeamGovernance defines the explicit governance contract.
func (t *governanceTx) TeamGovernance() application.TeamGovernance { return t.teams }

// AvailablePlatformAdmins defines the explicit governance contract.
func (t *governanceTx) AvailablePlatformAdmins() (int64, error) {
	var n int64
	err := t.db.Raw(`SELECT count(*) FROM identity."user" WHERE platform_role='platform_admin' AND status='active' AND email_verified_at IS NOT NULL AND NOT is_delete`).Scan(&n).Error
	return n, err
}

// ChangeUser defines the explicit governance contract.
func (t *governanceTx) ChangeUser(u domain.User, c application.UserGovernanceChange, now time.Time) error {
	values := map[string]any{"revision": gorm.Expr("revision+1"), "update_time": now}
	revoke := false
	switch c.Action {
	case "disable":
		values["status"] = "disabled"
		values["session_epoch"] = gorm.Expr("session_epoch+1")
		values["platform_epoch"] = gorm.Expr("platform_epoch+1")
		revoke = true
	case "enable":
		values["status"] = "active"
	case "unlock":
		values["failed_login_count"] = 0
		values["locked_until"] = nil
	case "revoke_sessions":
		values["session_epoch"] = gorm.Expr("session_epoch+1")
		revoke = true
	case "revoke_admin":
		values["platform_role"] = "creator"
		values["platform_epoch"] = gorm.Expr("platform_epoch+1")
	case "grant_admin":
		values["platform_role"] = "platform_admin"
		values["platform_epoch"] = gorm.Expr("platform_epoch+1")
	case "profile_reset":
		if c.ResetName {
			values["display_name"] = "创作者"
		}
		if c.ResetAvatar {
			values["avatar_asset_id"] = nil
		}
	default:
		return application.ErrInvalidGovernance
	}
	result := t.db.Table(`identity."user"`).Where("id=? AND revision=? AND NOT is_delete", u.ID, u.Revision).Updates(values)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrRevisionConflict
	}
	if c.Action == "unlock" {
		if err := t.db.Exec(`UPDATE identity.login_guard SET failure_times='{}',locked_until=NULL WHERE login_key=?`, c.LoginGuardKey).Error; err != nil {
			return err
		}
	}
	if revoke {
		if err := t.RevokeAccountSessions(u.ID, now); err != nil {
			return err
		}
		if err := t.db.Exec(`UPDATE identity.auth_challenge SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL AND consumed_at IS NULL`, now, u.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

// Settings reads current platform policy under current platform authorization.
func (t *governanceTx) Settings() application.PlatformSettings { return t.settings }

// SaveSettings defines the explicit governance contract.
func (t *governanceTx) SaveSettings(s application.PlatformSettings, actor uuid.UUID, now time.Time) error {
	r := t.db.Exec(`UPDATE identity.platform_settings SET registration_enabled=?,team_creation_enabled=?,revision=?,updated_by=?,update_time=? WHERE singleton AND revision=?`, s.RegistrationEnabled, s.TeamCreationEnabled, s.Revision, actor, now, s.Revision-1)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return domain.ErrRevisionConflict
	}
	return nil
}

// InsertProposal defines the explicit governance contract.
func (t *governanceTx) InsertProposal(p application.AuthorityProposal) error {
	if err := t.db.Exec(`UPDATE identity.authority_proposal SET status='expired',revision=revision+1,updated_at=? WHERE status='pending' AND expires_at<=?`, p.CreatedAt, p.CreatedAt).Error; err != nil {
		return err
	}
	var exists bool
	if err := t.db.Raw(`SELECT EXISTS(SELECT 1 FROM identity.authority_proposal WHERE status='pending' AND ((?='platform_grant' AND kind='platform_grant' AND target_user_id=?) OR (?<>'platform_grant' AND team_id=?)))`, p.Kind, p.TargetUserID, p.Kind, p.TeamID).Scan(&exists).Error; err != nil {
		return err
	}
	if exists {
		return application.ErrGovernanceConflict
	}
	return t.db.Exec(`INSERT INTO identity.authority_proposal(id,kind,team_id,source_user_id,target_user_id,status,expires_at,reason,source_authority_revision,target_revision,team_revision,revision,created_at,updated_at,target_email_version,target_credential_epoch) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, p.ID, p.Kind, p.TeamID, p.SourceUserID, p.TargetUserID, p.Status, p.ExpiresAt, p.Reason, p.SourceAuthorityRevision, p.TargetRevision, p.TeamRevision, p.Revision, p.CreatedAt, p.CreatedAt, p.TargetEmailVersion, p.TargetCredentialEpoch).Error
}

// Proposal defines the explicit governance contract.
func (t *governanceTx) Proposal(id uuid.UUID) (application.AuthorityProposal, error) {
	var p application.AuthorityProposal
	r := t.db.Raw(`SELECT * FROM identity.authority_proposal WHERE id=? FOR UPDATE`, id).Scan(&p)
	if r.Error != nil {
		return p, r.Error
	}
	if r.RowsAffected != 1 {
		return p, domain.ErrUserNotFound
	}
	return p, nil
}

// SaveProposal defines the explicit governance contract.
func (t *governanceTx) SaveProposal(p application.AuthorityProposal, now time.Time) error {
	r := t.db.Exec(`UPDATE identity.authority_proposal SET status=?,revision=?,updated_at=? WHERE id=? AND revision=? AND status='pending'`, p.Status, p.Revision, now, p.ID, p.Revision-1)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return domain.ErrRevisionConflict
	}
	return nil
}

// AppendGovernanceEvent defines the explicit governance contract.
func (t *governanceTx) AppendGovernanceEvent(target, team *uuid.UUID, action, reason string, now time.Time) error {
	scope := "platform"
	if team != nil {
		scope = "team"
	}
	return t.db.Exec(`INSERT INTO identity.security_event(id,actor_id,target_user_id,team_id,scope_kind,action,result,reason,created_at) VALUES(?,?,?,?,?,?,'success',?,?)`, uuid.New(), t.actor.ID, target, team, scope, action, reason, now).Error
}

func (t *governanceTx) ProposalReceipt(key uuid.UUID, hash string) (application.AuthorityProposal, bool, error) {
	var row struct {
		RequestHash  string
		ResponseBody []byte
	}
	r := t.db.Raw(`SELECT request_hash,response_body FROM infra.idempotency_record WHERE actor_id=? AND idem_key=? AND NOT is_delete`, t.actor.ID, "authority.propose:"+key.String()).Scan(&row)
	if r.Error != nil {
		return application.AuthorityProposal{}, false, r.Error
	}
	if r.RowsAffected == 0 {
		return application.AuthorityProposal{}, false, nil
	}
	if row.RequestHash != hash {
		return application.AuthorityProposal{}, false, application.ErrGovernanceConflict
	}
	var p application.AuthorityProposal
	if err := json.Unmarshal(row.ResponseBody, &p); err != nil {
		return p, false, err
	}
	return p, true, nil
}
func (t *governanceTx) SaveProposalReceipt(key uuid.UUID, hash string, p application.AuthorityProposal, now time.Time) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return t.db.Exec(`INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at,create_time,update_time) VALUES(?,?,?,?,201,?::jsonb,?,?,?)`, uuid.New(), t.actor.ID, "authority.propose:"+key.String(), hash, string(body), now.Add(24*time.Hour), now, now).Error
}

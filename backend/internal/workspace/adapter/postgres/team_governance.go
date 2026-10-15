package postgres

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// TeamGovernance is a transaction-bound gateway used by identity governance.
// The caller must already hold the shared platform_settings governance lock.
type TeamGovernance struct{ db *gorm.DB }

// NewTeamGovernance binds the workspace gateway to the caller existing transaction.
func NewTeamGovernance(tx *gorm.DB) *TeamGovernance { return &TeamGovernance{db: tx} }

// AccountAccessChanged invalidates team access versions and freezes teams lacking an available administrator.
func (g *TeamGovernance) AccountAccessChanged(user uuid.UUID, now time.Time) error {
	return g.db.Exec(`UPDATE workspace.team t SET governance_blocked=t.governance_blocked OR NOT EXISTS(SELECT 1 FROM workspace.team_member m JOIN identity."user" u ON u.id=m.user_id WHERE m.team_id=t.id AND m.role='team_admin' AND m.status='active' AND u.status='active' AND NOT u.is_delete AND u.email_verified_at IS NOT NULL),auth_epoch=t.auth_epoch+1,revision=t.revision+1,update_time=? WHERE EXISTS(SELECT 1 FROM workspace.team_member m WHERE m.team_id=t.id AND m.user_id=? AND m.status<>'removed')`, now, user).Error
}

// PrepareTransfer checks current team and membership authority under the shared governance lock.
func (g *TeamGovernance) PrepareTransfer(team, source, target uuid.UUID, takeover bool, revision int64) (int64, int64, error) {
	var row struct {
		Status            string
		GovernanceBlocked bool
		Revision          int64
	}
	r := g.db.Raw(`SELECT status,governance_blocked,revision FROM workspace.team WHERE id=? FOR UPDATE`, team).Scan(&row)
	if r.Error != nil {
		return 0, 0, r.Error
	}
	if r.RowsAffected != 1 {
		return 0, 0, application.ErrScopeNotFound
	}
	if row.Revision != revision {
		return 0, 0, domain.ErrRevisionConflict
	}
	if row.Status == "suspended" {
		return 0, 0, application.ErrTeamStateConflict
	}
	if takeover {
		if !row.GovernanceBlocked {
			return 0, 0, application.ErrTeamStateConflict
		}
		return 0, row.Revision, nil
	}
	var member struct {
		Role, Status string
		Revision     int64
	}
	r = g.db.Raw(`SELECT role,status,revision FROM workspace.team_member WHERE team_id=? AND user_id=? FOR UPDATE`, team, source).Scan(&member)
	if r.Error != nil {
		return 0, 0, r.Error
	}
	if member.Role != "team_admin" || member.Status != "active" {
		return 0, 0, identity.ErrForbidden
	}
	var eligible bool
	if err := g.db.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.team_member WHERE team_id=? AND user_id=? AND status='active')`, team, target).Scan(&eligible).Error; err != nil {
		return 0, 0, err
	}
	if !eligible {
		return 0, 0, application.ErrTeamStateConflict
	}
	return member.Revision, row.Revision, nil
}

// AcceptTransfer atomically promotes the target and completes the recorded team handover.
func (g *TeamGovernance) AcceptTransfer(p identity.AuthorityProposal, now time.Time) error {
	if p.TeamID == nil || p.TeamRevision == nil {
		return identity.ErrGovernanceConflict
	}
	takeover := p.Kind == "platform_takeover"
	src, _, err := g.PrepareTransfer(*p.TeamID, p.SourceUserID, p.TargetUserID, takeover, *p.TeamRevision)
	if err != nil {
		return err
	}
	if !takeover && src != p.SourceAuthorityRevision {
		return identity.ErrForbidden
	}
	if takeover {
		var current struct{ Status string }
		r := g.db.Raw(`SELECT status FROM workspace.team_member WHERE team_id=? AND user_id=? FOR UPDATE`, *p.TeamID, p.TargetUserID).Scan(&current)
		if r.Error != nil {
			return r.Error
		}
		if current.Status == "suspended" {
			return application.ErrTeamStateConflict
		}
		if current.Status != "active" {
			var joined, seats int64
			if err := g.db.Raw(`SELECT count(*) FROM workspace.team_member WHERE user_id=? AND status IN ('active','suspended')`, p.TargetUserID).Scan(&joined).Error; err != nil {
				return err
			}
			if err := g.db.Raw(`SELECT count(*) FROM workspace.team_member WHERE team_id=? AND status IN ('active','suspended')`, *p.TeamID).Scan(&seats).Error; err != nil {
				return err
			}
			if joined >= 20 || seats >= 100 {
				return application.ErrTeamCapacity
			}
		}
		if err := g.db.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,joined_by,source) VALUES(?,?,'team_admin','active',?,?,'transferred') ON CONFLICT(team_id,user_id) DO UPDATE SET role='team_admin',status='active',joined_at=EXCLUDED.joined_at,joined_by=EXCLUDED.joined_by,source='transferred',auth_epoch=workspace.team_member.auth_epoch+1,revision=workspace.team_member.revision+1,update_time=?`, *p.TeamID, p.TargetUserID, now, p.SourceUserID, now).Error; err != nil {
			return err
		}
	} else {
		if err := g.db.Exec(`UPDATE workspace.team_member SET role=CASE WHEN user_id=? THEN 'team_admin' ELSE 'member' END,auth_epoch=auth_epoch+1,revision=revision+1,update_time=? WHERE team_id=? AND user_id IN (?,?)`, p.TargetUserID, now, *p.TeamID, p.SourceUserID, p.TargetUserID).Error; err != nil {
			return err
		}
	}
	return g.db.Exec(`UPDATE workspace.team SET governance_blocked=false,revision=revision+1,auth_epoch=auth_epoch+1,update_time=? WHERE id=?`, now, *p.TeamID).Error
}

// CheckInvitation checks current invitation policy and returns any removed membership version.
func (g *TeamGovernance) CheckInvitation(team, actor uuid.UUID, email, role string) (*int64, error) {
	if role != "member" && role != "team_admin" {
		return nil, identity.ErrInvalidGovernance
	}
	var allowed bool
	if err := g.db.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.team t JOIN workspace.team_member m ON m.team_id=t.id WHERE t.id=? AND t.status='active' AND NOT t.governance_blocked AND t.invitation_enabled AND m.user_id=? AND m.status='active' AND m.role='team_admin')`, team, actor).Scan(&allowed).Error; err != nil {
		return nil, err
	}
	if !allowed {
		return nil, identity.ErrForbidden
	}
	return g.CheckPlatformInvitation(team, email, role)
}

// CheckPlatformInvitation checks team policy and target membership without inventing membership for the platform actor.
func (g *TeamGovernance) CheckPlatformInvitation(team uuid.UUID, email, role string) (*int64, error) {
	if role != "member" && role != "team_admin" {
		return nil, identity.ErrInvalidGovernance
	}
	var available bool
	if err := g.db.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.team WHERE id=? AND status='active' AND NOT governance_blocked AND invitation_enabled)`, team).Scan(&available).Error; err != nil {
		return nil, err
	}
	if !available {
		return nil, application.ErrTeamStateConflict
	}
	if email == "" {
		return nil, nil
	}
	var current struct {
		Status   string
		Revision int64
	}
	r := g.db.Raw(`SELECT m.status,m.revision FROM workspace.team_member m JOIN identity."user" u ON u.id=m.user_id WHERE m.team_id=? AND u.email_key=?`, team, email).Scan(&current)
	if r.Error != nil {
		return nil, r.Error
	}
	if r.RowsAffected == 0 {
		return nil, nil
	}
	if current.Status != "removed" {
		return nil, application.ErrTeamStateConflict
	}
	return &current.Revision, nil
}

// AcceptInvitation joins only the verified addressed account after rechecking team authority.
func (g *TeamGovernance) AcceptInvitation(i identity.Invitation, user uuid.UUID, now time.Time) error {
	if i.TeamID == nil || i.TeamRole == nil {
		return identity.ErrGovernanceConflict
	}
	var revision *int64
	var err error
	if i.PlatformIssued {
		revision, err = g.CheckPlatformInvitation(*i.TeamID, i.EmailKey, *i.TeamRole)
	} else {
		revision, err = g.CheckInvitation(*i.TeamID, i.InviterID, i.EmailKey, *i.TeamRole)
	}
	if err != nil {
		return err
	}
	if (revision == nil) != (i.MembershipRevision == nil) || (revision != nil && *revision != *i.MembershipRevision) {
		return application.ErrTeamRevisionConflict
	}
	var seats, joined int64
	if err := g.db.Raw(`SELECT count(*) FROM workspace.team_member WHERE team_id=? AND status IN ('active','suspended')`, *i.TeamID).Scan(&seats).Error; err != nil {
		return err
	}
	if err := g.db.Raw(`SELECT count(*) FROM workspace.team_member WHERE user_id=? AND status IN ('active','suspended')`, user).Scan(&joined).Error; err != nil {
		return err
	}
	if seats >= 100 || joined >= 20 {
		return application.ErrTeamCapacity
	}
	if err := g.db.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,joined_by,source) VALUES(?,?,?,'active',?,?,'invited') ON CONFLICT(team_id,user_id) DO UPDATE SET role=EXCLUDED.role,status='active',joined_at=EXCLUDED.joined_at,joined_by=EXCLUDED.joined_by,source='invited',auth_epoch=workspace.team_member.auth_epoch+1,revision=workspace.team_member.revision+1,update_time=?`, *i.TeamID, user, *i.TeamRole, now, i.InviterID, now).Error; err != nil {
		return err
	}
	return g.db.Exec(`UPDATE workspace.team SET revision=revision+1,auth_epoch=auth_epoch+1,update_time=? WHERE id=?`, now, *i.TeamID).Error
}

// AuthorizeInvitations checks management authority without requiring invitations to be enabled.
func (g *TeamGovernance) AuthorizeInvitations(team, actor uuid.UUID) error {
	var allowed bool
	if err := g.db.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.team t JOIN workspace.team_member m ON m.team_id=t.id WHERE t.id=? AND t.status IN ('active','archived') AND m.user_id=? AND m.status='active' AND m.role='team_admin')`, team, actor).Scan(&allowed).Error; err != nil {
		return err
	}
	if !allowed {
		return identity.ErrForbidden
	}
	return nil
}

// InvitationAuthority returns current team revision after membership authorization.
func (g *TeamGovernance) InvitationAuthority(team, actor uuid.UUID) (int64, error) {
	if err := g.AuthorizeInvitations(team, actor); err != nil {
		return 0, err
	}
	var revision int64
	err := g.db.Raw(`SELECT revision FROM workspace.team WHERE id=?`, team).Scan(&revision).Error
	return revision, err
}

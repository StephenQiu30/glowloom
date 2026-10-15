package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// TeamActorAccessFactory binds current identity checks to the team transaction.
type TeamActorAccessFactory func(*gorm.DB) application.TeamActorAccess

// TeamStore persists team governance using the existing database connection.
type TeamStore struct {
	db      *gorm.DB
	actors  TeamActorAccessFactory
	avatars TeamAvatarAccessFactory
}

// NewTeamStore injects identity authorization without opening another environment.
func NewTeamStore(db *gorm.DB, actors TeamActorAccessFactory) *TeamStore {
	return &TeamStore{db: db, actors: actors}
}

type teamTx struct {
	db              *gorm.DB
	actor           identity.Principal
	creationEnabled bool
	actors          application.TeamActorAccess
	clock           func() time.Time
	avatars         application.TeamAvatarAccess
}

// TeamTransaction holds the shared governance lock before identity and team locks.
func (s *TeamStore) TeamTransaction(ctx context.Context, actor identity.Principal, clock func() time.Time, run func(application.TeamTx) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var settings struct{ TeamCreationEnabled bool }
		r := tx.Raw(`SELECT team_creation_enabled FROM identity.platform_settings WHERE singleton FOR UPDATE`).Scan(&settings)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrTeamCreationClosed
		}
		if s.actors == nil {
			return application.ErrTeamForbidden
		}
		if err := s.actors(tx).CheckSession(ctx, actor, clock); err != nil {
			return err
		}
		var avatars application.TeamAvatarAccess
		if s.avatars != nil {
			avatars = s.avatars(tx)
		}
		return run(&teamTx{avatars: avatars, db: tx, actor: actor, creationEnabled: settings.TeamCreationEnabled, actors: s.actors(tx), clock: clock})
	})
}

func (t *teamTx) RequireRecentAuthentication() error {
	return t.actors.CheckRecentSession(t.db.Statement.Context, t.actor, t.clock)
}
func (t *teamTx) Spaces() ([]application.Space, error) {
	spaces := []application.Space{}
	err := t.db.Raw(`SELECT s.id,s.kind,NULL::uuid AS team_id,'个人空间' AS name,'owner' AS role,true AS writable FROM workspace.creation_scope s WHERE s.kind='personal' AND s.owner_user_id=?
UNION ALL
SELECT s.id,s.kind,t.id AS team_id,t.name,m.role,(t.status='active' AND NOT t.governance_blocked) AS writable FROM workspace.creation_scope s JOIN workspace.team t ON t.id=s.team_id JOIN workspace.team_member m ON m.team_id=t.id WHERE s.kind='team' AND m.user_id=? AND m.status='active' AND t.status IN ('active','archived') ORDER BY kind,id`, t.actor.ID, t.actor.ID).Scan(&spaces).Error
	return spaces, err
}
func (t *teamTx) CreationEnabled() bool { return t.creationEnabled }
func (t *teamTx) CreationCounts() (created, joined int64, err error) {
	err = t.db.Raw(`SELECT count(*) FROM workspace.team WHERE created_by=? AND (status='active' OR status='suspended' AND resume_status='active')`, t.actor.ID).Scan(&created).Error
	if err != nil {
		return
	}
	err = t.db.Raw(`SELECT count(*) FROM workspace.team_member WHERE user_id=? AND status IN ('active','suspended')`, t.actor.ID).Scan(&joined).Error
	return
}
func (t *teamTx) CreationReceipt(key uuid.UUID, hash string) (application.Team, bool, error) {
	var row struct {
		RequestHash  string
		ResponseBody []byte
	}
	r := t.db.Raw(`SELECT request_hash,response_body FROM infra.idempotency_record WHERE actor_id=? AND idem_key=? AND NOT is_delete FOR UPDATE`, t.actor.ID, "team.create:"+key.String()).Scan(&row)
	if r.Error != nil {
		return application.Team{}, false, r.Error
	}
	if r.RowsAffected == 0 {
		return application.Team{}, false, nil
	}
	if row.RequestHash != hash {
		return application.Team{}, false, application.ErrIdempotencyConflict
	}
	var team application.Team
	if err := json.Unmarshal(row.ResponseBody, &team); err != nil {
		return team, false, err
	}
	return team, true, nil
}
func (t *teamTx) SaveCreationReceipt(key uuid.UUID, hash string, team application.Team, now time.Time) error {
	body, err := json.Marshal(team)
	if err != nil {
		return err
	}
	return t.db.Exec(`INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at,create_time,update_time) VALUES(?,?,?,?,201,?::jsonb,?,?,?)`, uuid.New(), t.actor.ID, "team.create:"+key.String(), hash, string(body), now.Add(24*time.Hour), now, now).Error
}
func (t *teamTx) InsertTeam(name string, avatar *uuid.UUID, now time.Time) (application.Team, error) {
	team := application.Team{AvatarID: avatar, ID: uuid.New(), ScopeID: uuid.New(), PhysicalScopeID: uuid.New(), Name: name, Status: "active", ActorRole: "team_admin", Revision: 1, MemberRevision: 1, MemberCount: 1, InvitationEnabled: true, CreatedAt: now}
	if err := t.db.Exec(`INSERT INTO workspace.organization(id,name,create_time,update_time) VALUES(?,?,?,?)`, team.PhysicalScopeID, name, now, now).Error; err != nil {
		return team, err
	}
	if err := t.db.Exec(`INSERT INTO workspace.team(id,name,created_by,create_time,update_time,avatar_asset_id) VALUES(?,?,?,?,?,?)`, team.ID, name, t.actor.ID, now, now, avatar).Error; err != nil {
		return team, err
	}
	if err := t.db.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source,create_time,update_time) VALUES(?,?,'team_admin','active',?,'created',?,?)`, team.ID, t.actor.ID, now, now, now).Error; err != nil {
		return team, err
	}
	if err := t.db.Exec(`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, team.ScopeID, team.ID, team.PhysicalScopeID).Error; err != nil {
		return team, err
	}
	return team, nil
}
func (t *teamTx) LoadTeam(id uuid.UUID) (application.Team, error) {
	var team application.Team
	r := t.db.Raw(`SELECT t.id,t.avatar_asset_id AS avatar_id,t.name,t.status,t.revision,t.governance_blocked,t.invitation_enabled,t.create_time AS created_at,s.id AS scope_id,s.physical_scope_id,m.role AS actor_role,m.revision AS member_revision FROM workspace.team t JOIN workspace.creation_scope s ON s.team_id=t.id AND s.kind='team' JOIN workspace.team_member m ON m.team_id=t.id WHERE t.id=? AND m.user_id=? AND m.status='active' AND t.status IN ('active','archived') FOR UPDATE OF t,m`, id, t.actor.ID).Scan(&team)
	if r.Error != nil {
		return team, r.Error
	}
	if r.RowsAffected != 1 {
		return team, application.ErrScopeNotFound
	}
	err := t.db.Raw(`SELECT count(*) FROM workspace.team_member m JOIN identity."user" u ON u.id=m.user_id WHERE m.team_id=? AND m.status='active' AND u.status='active' AND NOT u.is_delete AND u.email_verified_at IS NOT NULL`, id).Scan(&team.MemberCount).Error
	return team, err
}
func (t *teamTx) SaveTeam(team application.Team, revision int64, now time.Time) error {
	r := t.db.Exec(`UPDATE workspace.team SET name=?,avatar_asset_id=?,status=?,invitation_enabled=?,revision=?,auth_epoch=auth_epoch+1,update_time=? WHERE id=? AND revision=?`, team.Name, team.AvatarID, team.Status, team.InvitationEnabled, team.Revision, now, team.ID, revision)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return application.ErrTeamRevisionConflict
	}
	return nil
}
func (t *teamTx) Members(id uuid.UUID, admin bool) ([]application.TeamMember, error) {
	members := []application.TeamMember{}
	projection := `m.user_id,u.display_name,u.avatar_asset_id AS avatar_id,m.role,m.joined_at`
	condition := ` AND m.status='active' AND u.status='active' AND NOT u.is_delete AND u.email_verified_at IS NOT NULL`
	if admin {
		projection += `,u.email_key AS email,m.status,m.source,m.revision`
		condition = ` AND m.status IN ('active','suspended')`
	}
	err := t.db.Raw(`SELECT `+projection+` FROM workspace.team_member m JOIN identity."user" u ON u.id=m.user_id WHERE m.team_id=?`+condition+` ORDER BY m.joined_at,m.user_id LIMIT 100`, id).Scan(&members).Error
	return members, err
}
func (t *teamTx) Member(team, user uuid.UUID) (application.TeamMemberState, error) {
	var member application.TeamMemberState
	r := t.db.Raw(`SELECT m.user_id,m.role,m.status,m.revision,(u.status='active' AND NOT u.is_delete AND u.email_verified_at IS NOT NULL) AS account_available FROM workspace.team_member m JOIN identity."user" u ON u.id=m.user_id WHERE m.team_id=? AND m.user_id=? FOR UPDATE OF m`, team, user).Scan(&member)
	if r.Error != nil {
		return member, r.Error
	}
	if r.RowsAffected != 1 {
		return member, application.ErrScopeNotFound
	}
	return member, nil
}
func (t *teamTx) AvailableAdmins(team uuid.UUID) (int64, error) {
	var count int64
	err := t.db.Raw(`SELECT count(*) FROM workspace.team_member m JOIN identity."user" u ON u.id=m.user_id WHERE m.team_id=? AND m.role='team_admin' AND m.status='active' AND u.status='active' AND NOT u.is_delete AND u.email_verified_at IS NOT NULL`, team).Scan(&count).Error
	return count, err
}
func (t *teamTx) SaveMember(team uuid.UUID, member application.TeamMemberState, revision int64, now time.Time) error {
	r := t.db.Exec(`UPDATE workspace.team_member SET status=?,role=?,revision=?,auth_epoch=auth_epoch+1,update_time=? WHERE team_id=? AND user_id=? AND revision=?`, member.Status, member.Role, member.Revision, now, team, member.UserID, revision)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return application.ErrTeamRevisionConflict
	}
	return t.db.Exec(`UPDATE workspace.team SET auth_epoch=auth_epoch+1,revision=revision+1,update_time=? WHERE id=?`, now, team).Error
}
func (t *teamTx) AppendTeamEvent(team uuid.UUID, target *uuid.UUID, action, reason string, now time.Time) error {
	if err := t.db.Exec(`INSERT INTO identity.security_event(id,actor_id,target_user_id,team_id,scope_kind,action,result,reason,created_at) VALUES(?,?,?,?,'team',?,'success',?,?)`, uuid.New(), t.actor.ID, target, team, action, reason, now).Error; err != nil {
		return err
	}
	var rows []struct{ UserID uuid.UUID }
	if err := t.db.Raw(`SELECT user_id FROM workspace.team_member WHERE team_id=? AND status IN ('active','suspended')`, team).Scan(&rows).Error; err != nil {
		return err
	}
	ids := []uuid.UUID{t.actor.ID}
	for _, row := range rows {
		ids = append(ids, row.UserID)
	}
	if target != nil {
		ids = append(ids, *target)
	}
	return t.actors.NotifyTeamAccounts(t.db.Statement.Context, ids, now)
}

func (t *teamTx) CanArchive(id uuid.UUID) error {
	var busy bool
	if err := t.db.Raw(`SELECT EXISTS(SELECT 1 FROM operation.operation o JOIN workspace.project p ON p.id=o.project_id JOIN workspace.creation_scope s ON s.physical_scope_id=p.org_id WHERE s.team_id=? AND NOT o.is_delete AND o.status IN ('confirmed','submitting','submitted','succeeded','ingesting','unknown','reconciling','manual','cancelling')) OR EXISTS(SELECT 1 FROM identity.authority_proposal WHERE team_id=? AND status='pending' AND expires_at>?)`, id, id, t.clock().UTC()).Scan(&busy).Error; err != nil {
		return err
	}
	if busy {
		return application.ErrTeamStateConflict
	}
	return nil
}

func (t *teamTx) CanRestore(id uuid.UUID) error {
	var n int64
	if err := t.db.Raw(`SELECT count(*) FROM workspace.team t WHERE t.created_by=(SELECT created_by FROM workspace.team WHERE id=?) AND (t.status='active' OR t.status='suspended' AND t.resume_status='active')`, id).Scan(&n).Error; err != nil {
		return err
	}
	if n >= 5 {
		return application.ErrTeamCapacity
	}
	return nil
}

func (t *teamTx) RevokePendingInvitations(team uuid.UUID, now time.Time) error {
	return t.actors.RevokePendingTeamInvitations(t.db.Statement.Context, team, now)
}

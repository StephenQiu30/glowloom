package postgres

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// TeamAvatarAccessFactory binds media validation to the current team transaction.
type TeamAvatarAccessFactory func(*gorm.DB) application.TeamAvatarAccess

// NewTeamProfileStore supplies normalized media validation in addition to current identity.
func NewTeamProfileStore(db *gorm.DB, actors TeamActorAccessFactory, avatars TeamAvatarAccessFactory) *TeamStore {
	return &TeamStore{db: db, actors: actors, avatars: avatars}
}
func (t *teamTx) RequirePlatform() error {
	return t.actors.CheckPlatformSession(t.db.Statement.Context, t.actor, t.now)
}

const platformTeamProjection = `t.id,t.name,t.avatar_asset_id AS avatar_id,t.created_by,t.status,t.resume_status,t.governance_blocked,t.invitation_enabled,t.revision,t.create_time AS created_at,(SELECT count(*) FROM workspace.team_member m WHERE m.team_id=t.id AND m.status IN ('active','suspended')) AS member_count,(SELECT count(*) FROM workspace.team_member m JOIN identity."user" u ON u.id=m.user_id WHERE m.team_id=t.id AND m.status='active' AND m.role='team_admin' AND u.status='active' AND u.email_verified_at IS NOT NULL AND NOT u.is_delete) AS administrator_count`

func (t *teamTx) LoadPlatformTeam(id uuid.UUID) (application.PlatformTeam, error) {
	var out application.PlatformTeam
	r := t.db.Table("workspace.team t").Select(platformTeamProjection).Where("t.id=?", id).Scan(&out)
	if r.Error != nil {
		return out, r.Error
	}
	if r.RowsAffected != 1 {
		return out, application.ErrScopeNotFound
	}
	return out, nil
}
func (t *teamTx) QueryPlatformTeams(f application.PlatformTeamFilter, p application.PlatformTeamPosition) ([]application.PlatformTeam, error) {
	out := []application.PlatformTeam{}
	q := t.db.Table("workspace.team t").Select(platformTeamProjection).Where("t.create_time<=?", p.AsOf)
	if f.ID != nil {
		q = q.Where("t.id=?", *f.ID)
	}
	if f.AdministratorID != nil {
		q = q.Where(`EXISTS (SELECT 1 FROM workspace.team_member m JOIN identity."user" u ON u.id=m.user_id WHERE m.team_id=t.id AND m.user_id=? AND m.status='active' AND m.role='team_admin' AND u.status='active' AND u.email_verified_at IS NOT NULL AND NOT u.is_delete)`, *f.AdministratorID)
	}
	if f.CreatedFrom != nil {
		q = q.Where("t.create_time>=?", *f.CreatedFrom)
	}
	if f.CreatedTo != nil {
		q = q.Where("t.create_time<?", *f.CreatedTo)
	}
	if f.Search != "" {
		v := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(f.Search)
		q = q.Where("t.name ILIKE ? ESCAPE '!'", "%"+v+"%")
	}
	if f.Status != "" {
		q = q.Where("t.status=?", f.Status)
	}
	if f.GovernanceBlocked != nil {
		q = q.Where("t.governance_blocked=?", *f.GovernanceBlocked)
	}
	if p.ID != uuid.Nil {
		q = q.Where("(t.create_time,t.id)<(?,?)", p.CreatedAt, p.ID)
	}
	err := q.Order("t.create_time DESC,t.id DESC").Limit(f.Limit + 1).Scan(&out).Error
	return out, err
}
func (t *teamTx) SavePlatformTeam(team application.PlatformTeam, v int64, now time.Time) error {
	r := t.db.Exec(`UPDATE workspace.team SET name=?,avatar_asset_id=?,status=?,resume_status=?,revision=?,auth_epoch=auth_epoch+1,update_time=? WHERE id=? AND revision=?`, team.Name, team.AvatarID, team.Status, team.ResumeStatus, team.Revision, now, team.ID, v)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return application.ErrTeamRevisionConflict
	}
	return nil
}
func (t *teamTx) ValidateTeamAvatar(id uuid.UUID) error {
	if t.avatars == nil {
		return identity.ErrAvatarUnavailable
	}
	return t.avatars.Validate(t.db.Statement.Context, t.actor.ID, id)
}

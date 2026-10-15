package postgres

import (
	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func (t *teamTx) BoundTeamAvatar(team uuid.UUID, member *uuid.UUID, platform bool) (application.TeamAvatarObject, error) {
	var id *uuid.UUID
	if platform {
		if member != nil {
			return application.TeamAvatarObject{}, application.ErrTeamForbidden
		}
		if err := t.RequirePlatform(); err != nil {
			return application.TeamAvatarObject{}, err
		}
		current, err := t.LoadPlatformTeam(team)
		if err != nil {
			return application.TeamAvatarObject{}, err
		}
		id = current.AvatarID
	} else {
		current, err := t.LoadTeam(team)
		if err != nil {
			return application.TeamAvatarObject{}, err
		}
		id = current.AvatarID
		if member != nil {
			rows, err := t.QueryMembers(team, current.ActorRole == "team_admin", application.MemberFilter{UserID: *member, Limit: 1}, application.MemberPosition{AsOf: t.now})
			if err != nil {
				return application.TeamAvatarObject{}, err
			}
			if len(rows) != 1 {
				return application.TeamAvatarObject{}, application.ErrScopeNotFound
			}
			id = rows[0].AvatarID
		}
	}
	if id == nil || t.avatars == nil {
		return application.TeamAvatarObject{}, identity.ErrAvatarUnavailable
	}
	return t.avatars.ReadBound(t.db.Statement.Context, *id)
}

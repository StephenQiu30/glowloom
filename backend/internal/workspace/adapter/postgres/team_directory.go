package postgres

import (
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func (t *teamTx) QueryMembers(id uuid.UUID, admin bool, f application.MemberFilter, position application.MemberPosition) ([]application.TeamMember, error) {
	members := []application.TeamMember{}
	projection := `m.user_id,u.display_name,u.avatar_asset_id AS avatar_id,m.role,m.joined_at`
	condition := ` AND m.status='active' AND u.status='active' AND NOT u.is_delete AND u.email_verified_at IS NOT NULL`
	if admin {
		projection += `,u.email_key AS email,m.status,m.source,m.revision,m.last_team_activity_at,(m.status='active' AND u.status='active' AND NOT u.is_delete AND u.email_verified_at IS NOT NULL) AS access_available`
		condition = ""
	}
	query := `SELECT ` + projection + ` FROM workspace.team_member m JOIN identity."user" u ON u.id=m.user_id WHERE m.team_id=? AND m.joined_at<=?` + condition
	args := []any{id, position.AsOf}
	if f.UserID != uuid.Nil {
		query += ` AND m.user_id=?`
		args = append(args, f.UserID)
	}
	if f.Role != "" {
		query += ` AND m.role=?`
		args = append(args, f.Role)
	}
	if f.Status != "" {
		query += ` AND m.status=?`
		args = append(args, f.Status)
	}
	if f.From != nil {
		query += ` AND m.joined_at>=?`
		args = append(args, *f.From)
	}
	if f.To != nil {
		query += ` AND m.joined_at<?`
		args = append(args, *f.To)
	}
	if f.Search != "" {
		if admin {
			query += ` AND (strpos(lower(u.display_name),lower(?))>0 OR strpos(lower(u.email_key),lower(?))>0)`
			args = append(args, f.Search, f.Search)
		} else {
			query += ` AND strpos(lower(u.display_name),lower(?))>0`
			args = append(args, f.Search)
		}
	}
	if position.UserID != uuid.Nil {
		query += ` AND (m.joined_at,m.user_id)>(?,?)`
		args = append(args, position.JoinedAt, position.UserID)
	}
	query += ` ORDER BY m.joined_at,m.user_id LIMIT ?`
	args = append(args, f.Limit+1)
	err := t.db.Raw(query, args...).Scan(&members).Error
	return members, err
}

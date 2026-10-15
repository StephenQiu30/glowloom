package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// PlatformDirectoryStore supplies platform-authorized account projections.
type PlatformDirectoryStore struct{ db *gorm.DB }

// NewPlatformDirectoryStore constructs PlatformDirectoryStore with explicit caller-supplied dependencies.
func NewPlatformDirectoryStore(db *gorm.DB) *PlatformDirectoryStore {
	return &PlatformDirectoryStore{db: db}
}

func lockPlatformReader(tx *gorm.DB, actor application.Principal, clock func() time.Time) error {
	u, err := lockSelfActor(tx, actor, clock)
	if err != nil {
		return err
	}
	if u.PlatformRole != "platform_admin" || u.PlatformEpoch != actor.PlatformEpoch {
		return application.ErrForbidden
	}
	return nil
}

// Team membership is correlated, so multiple teams cannot duplicate user rows.
const availableTeamsSQL = `(SELECT count(*) FROM workspace.team_member tm JOIN workspace.team team ON team.id=tm.team_id WHERE tm.user_id=u.id AND tm.status='active' AND team.status<>'suspended')`
const directoryProjection = `u.id,coalesce(u.email_key,'') AS email,u.display_name,CASE WHEN u.is_delete THEN 'deleted' ELSE u.status END AS status,u.platform_role,u.email_verified_at IS NOT NULL AS verified,u.email_verified_at,u.avatar_asset_id,u.revision,u.create_time AS created_at,u.last_login_at,` + availableTeamsSQL + ` AS active_team_count`

func directoryQuery(tx *gorm.DB) *gorm.DB {
	return tx.Table(`identity."user" AS u`).Select(directoryProjection)
}

// ReadDirectory queries bounded account summaries under current platform authority.
func (s *PlatformDirectoryStore) ReadDirectory(ctx context.Context, actor application.Principal, f application.DirectoryFilter, pos application.DirectoryPosition, clock func() time.Time) ([]application.UserSummary, error) {
	items := make([]application.UserSummary, 0)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformReader(tx, actor, clock); err != nil {
			return err
		}
		q := filteredDirectory(tx, f, pos.AsOf)
		order, comparison := "u.create_time DESC,u.id DESC", "<"
		if f.Order == "created_asc" {
			order, comparison = "u.create_time ASC,u.id ASC", ">"
		}
		if pos.ID != uuid.Nil {
			q = q.Where("(u.create_time,u.id) "+comparison+" (?,?)", pos.CreatedAt, pos.ID)
		}
		if err := q.Order(order).Limit(f.Limit + 1).Scan(&items).Error; err != nil {
			return fmt.Errorf("read platform directory: %w", err)
		}
		return nil
	})
	return items, err
}

// ReadUserDetail returns one authorized account and team summary.
func (s *PlatformDirectoryStore) ReadUserDetail(ctx context.Context, actor application.Principal, id uuid.UUID, clock func() time.Time) (application.UserDetail, error) {
	detail := application.UserDetail{Teams: []application.TeamMembershipSummary{}}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformReader(tx, actor, clock); err != nil {
			return err
		}
		r := directoryQuery(tx).Where("u.id=?", id).Scan(&detail.User)
		if r.Error != nil {
			return fmt.Errorf("read platform user: %w", r.Error)
		}
		if r.RowsAffected != 1 {
			return domain.ErrUserNotFound
		}
		// Actions advertise eligible workflows; submission still requires recent proof and CAS.
		u := detail.User
		detail.AllowedActions = []string{}
		if u.Status == "active" || u.Status == "disabled" {
			detail.AllowedActions = append(detail.AllowedActions, "profile_reset")
			if u.Verified {
				detail.AllowedActions = append(detail.AllowedActions, "password_reset")
			}
			var admins int64
			if err := tx.Raw(`SELECT count(*) FROM identity."user" WHERE platform_role='platform_admin' AND status='active' AND email_verified_at IS NOT NULL AND NOT is_delete`).Scan(&admins).Error; err != nil {
				return err
			}
			last := u.PlatformRole == "platform_admin" && u.Status == "active" && u.Verified && admins <= 1
			if u.Status == "disabled" {
				detail.AllowedActions = append(detail.AllowedActions, "enable")
			} else {
				detail.AllowedActions = append(detail.AllowedActions, "unlock", "revoke_sessions")
				if !last {
					detail.AllowedActions = append(detail.AllowedActions, "disable")
				}
				if u.PlatformRole == "creator" && u.Verified && u.ID != actor.ID {
					var pending bool
					if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM identity.authority_proposal WHERE kind='platform_grant' AND target_user_id=? AND status='pending' AND expires_at>?)`, u.ID, clock().UTC()).Scan(&pending).Error; err != nil {
						return err
					}
					if !pending {
						detail.AllowedActions = append(detail.AllowedActions, "propose_admin")
					}
				}
			}
			if u.PlatformRole == "platform_admin" && !last {
				detail.AllowedActions = append(detail.AllowedActions, "revoke_admin")
			}
		}
		if err := tx.Table("workspace.team_member").Select("team_id,role,status").Where("user_id=?", id).Order("team_id").Scan(&detail.Teams).Error; err != nil {
			return fmt.Errorf("read user team summary: %w", err)
		}
		return nil
	})
	return detail, err
}

func filteredDirectory(tx *gorm.DB, f application.DirectoryFilter, asOf time.Time) *gorm.DB {
	q := directoryQuery(tx).Where("u.create_time <= ?", asOf)
	if f.Search != "" {
		if email, err := domain.NormalizeEmail(f.Search); err == nil {
			q = q.Where("u.email_key=?", email)
		} else {
			value := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(f.Search)
			q = q.Where("u.display_name ILIKE ? ESCAPE '!'", "%"+value+"%")
		}
	}
	if f.Status != "" {
		q = q.Where("(CASE WHEN u.is_delete THEN 'deleted' ELSE u.status END)=?", f.Status)
	}
	if f.PlatformRole != "" {
		q = q.Where("u.platform_role=?", f.PlatformRole)
	}
	if f.Verified != nil {
		q = q.Where("(u.email_verified_at IS NOT NULL)=?", *f.Verified)
	}
	if f.InTeam != nil {
		q = q.Where("(NOT u.is_delete AND u.status='active' AND u.email_verified_at IS NOT NULL AND "+availableTeamsSQL+">0)=?", *f.InTeam)
	}
	if f.From != nil {
		q = q.Where("u.create_time>=?", *f.From)
	}
	if f.To != nil {
		q = q.Where("u.create_time<?", *f.To)
	}
	return q
}

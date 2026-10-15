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

type PlatformDirectoryStore struct{ db *gorm.DB }

func NewPlatformDirectoryStore(db *gorm.DB) *PlatformDirectoryStore {
	return &PlatformDirectoryStore{db: db}
}

func lockPlatformReader(tx *gorm.DB, actor application.Principal, now time.Time) error {
	u, err := lockSelfActor(tx, actor, now)
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
const directoryProjection = `u.id,coalesce(u.email_key,'') AS email,u.display_name,u.status,u.platform_role,u.email_verified_at IS NOT NULL AS verified,u.email_verified_at,u.avatar_asset_id,u.must_change_password,u.revision,u.create_time AS created_at,u.last_login_at,` + availableTeamsSQL + ` AS active_team_count`

func directoryQuery(tx *gorm.DB) *gorm.DB {
	return tx.Table(`identity."user" AS u`).Select(directoryProjection).Where("NOT u.is_delete")
}

func (s *PlatformDirectoryStore) ReadDirectory(ctx context.Context, actor application.Principal, f application.DirectoryFilter, pos application.DirectoryPosition, now time.Time) ([]application.UserSummary, error) {
	items := make([]application.UserSummary, 0)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformReader(tx, actor, now); err != nil {
			return err
		}
		q := directoryQuery(tx).Where("u.create_time <= ?", pos.AsOf)
		if f.Search != "" {
			if email, err := domain.NormalizeEmail(f.Search); err == nil {
				q = q.Where("u.email_key=?", email)
			} else {
				value := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(f.Search)
				q = q.Where("u.display_name ILIKE ? ESCAPE '!'", "%"+value+"%")
			}
		}
		if f.Status != "" {
			q = q.Where("u.status=?", f.Status)
		}
		if f.PlatformRole != "" {
			q = q.Where("u.platform_role=?", f.PlatformRole)
		}
		if f.Verified != nil {
			q = q.Where("(u.email_verified_at IS NOT NULL)=?", *f.Verified)
		}
		if f.InTeam != nil {
			q = q.Where("(u.status='active' AND u.email_verified_at IS NOT NULL AND "+availableTeamsSQL+">0)=?", *f.InTeam)
		}
		if f.From != nil {
			q = q.Where("u.create_time>=?", *f.From)
		}
		if f.To != nil {
			q = q.Where("u.create_time<?", *f.To)
		}
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

func (s *PlatformDirectoryStore) ReadUserDetail(ctx context.Context, actor application.Principal, id uuid.UUID, now time.Time) (application.UserDetail, error) {
	detail := application.UserDetail{Teams: []application.TeamMembershipSummary{}}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformReader(tx, actor, now); err != nil {
			return err
		}
		r := directoryQuery(tx).Where("u.id=?", id).Scan(&detail.User)
		if r.Error != nil {
			return fmt.Errorf("read platform user: %w", r.Error)
		}
		if r.RowsAffected != 1 {
			return domain.ErrUserNotFound
		}
		if err := tx.Table("workspace.team_member").Select("team_id,role,status").Where("user_id=?", id).Order("team_id").Scan(&detail.Teams).Error; err != nil {
			return fmt.Errorf("read user team summary: %w", err)
		}
		return nil
	})
	return detail, err
}

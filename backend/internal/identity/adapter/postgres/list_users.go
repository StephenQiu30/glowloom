package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// ListForAdmin returns a bounded organization-scoped keyset page after
// rechecking the administrator in the same transaction.
func (s *Store) ListForAdmin(ctx context.Context, actorID, orgID uuid.UUID, limit int, after *application.UserListCursor) (application.UserListPage, error) {
	if s == nil || s.db == nil || actorID == uuid.Nil || orgID == uuid.Nil || limit < 1 || limit > 200 ||
		(after != nil && (after.ID == uuid.Nil || after.CreateTime.IsZero())) {
		return application.UserListPage{}, application.ErrInvalidUserList
	}
	var page application.UserListPage
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentAdmin(tx, orgID, actorID); err != nil {
			return err
		}
		const baseQuery = `
			SELECT u.id, u.login_name, u.display_name, u.role, u.status, u.last_login_at, u.revision, u.create_time,
 COALESCE(GREATEST(u.locked_until,g.locked_until)>now(),false) AS login_locked
 FROM identity."user" u LEFT JOIN identity.login_guard g ON g.login_key=lower(u.login_name) WHERE u.org_id = ?::uuid AND NOT u.is_delete
		`
		query := baseQuery + ` ORDER BY create_time DESC, id DESC LIMIT ?`
		args := []any{orgID.String(), limit + 1}
		if after != nil {
			query = baseQuery + ` AND (create_time, id) < (?::timestamptz, ?::uuid)
				ORDER BY create_time DESC, id DESC LIMIT ?`
			args = []any{orgID.String(), after.CreateTime.UTC(), after.ID.String(), limit + 1}
		}
		var rows []struct {
			ID          uuid.UUID
			LoginName   string
			DisplayName string
			Role        string
			Status      string
			LoginLocked bool
			LastLoginAt *time.Time
			Revision    int64
			CreateTime  time.Time
		}
		if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
			return fmt.Errorf("read account directory: %w", err)
		}
		more := len(rows) > limit
		if more {
			rows = rows[:limit]
		}
		page.Users = make([]application.UserListItem, 0, len(rows))
		for _, row := range rows {
			page.Users = append(page.Users, application.UserListItem{
				ID: row.ID, LoginName: row.LoginName, DisplayName: row.DisplayName,
				Role: domain.Role(row.Role), Status: domain.Status(row.Status),
				LoginLocked: row.LoginLocked, LastLoginAt: row.LastLoginAt, Revision: row.Revision, CreateTime: row.CreateTime,
			})
		}
		if more {
			last := page.Users[len(page.Users)-1]
			page.Next = &application.UserListCursor{CreateTime: last.CreateTime, ID: last.ID}
		}
		return nil
	})
	if err != nil {
		return application.UserListPage{}, fmt.Errorf("list accounts transaction: %w", err)
	}
	return page, nil
}

var _ application.ListUsersStore = (*Store)(nil)

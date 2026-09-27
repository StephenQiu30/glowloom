package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// ErrInvalidUserList means the requested page size or cursor is invalid.
var ErrInvalidUserList = errors.New("invalid user list query")

// UserListCursor is an internal keyset cursor. Public encoding belongs to the API contract.
type UserListCursor struct {
	CreateTime time.Time
	ID         uuid.UUID
}

// UserListItem contains only safe account directory fields.
type UserListItem struct {
	ID          uuid.UUID
	LoginName   string
	DisplayName string
	Role        domain.Role
	Status      domain.Status
	LastLoginAt *time.Time
	Revision    int64
	CreateTime  time.Time
}

// UserListPage holds one page and the next keyset position when more rows exist.
type UserListPage struct {
	Users []UserListItem
	Next  *UserListCursor
}

// ListUsersStore rechecks administrator rights in the same read transaction.
type ListUsersStore interface {
	ListForAdmin(context.Context, uuid.UUID, uuid.UUID, int, *UserListCursor) (UserListPage, error)
}

// ListUsersInput requests a bounded internal page.
type ListUsersInput struct {
	Limit int
	After *UserListCursor
}

// ListUsersQuery returns organization-scoped account directory pages.
type ListUsersQuery struct{ store ListUsersStore }

// NewListUsersQuery injects the scoped account reader.
func NewListUsersQuery(store ListUsersStore) *ListUsersQuery { return &ListUsersQuery{store: store} }

// Execute checks the principal before the store rechecks its current permissions.
func (q *ListUsersQuery) Execute(ctx context.Context, actor Principal, input ListUsersInput) (UserListPage, error) {
	if q == nil || q.store == nil {
		return UserListPage{}, ErrInvalidUserList
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.Role != domain.RoleAdmin || actor.MustChangePassword {
		return UserListPage{}, ErrForbidden
	}
	if input.Limit < 0 || input.Limit > 200 ||
		(input.After != nil && (input.After.ID == uuid.Nil || input.After.CreateTime.IsZero())) {
		return UserListPage{}, ErrInvalidUserList
	}
	limit := input.Limit
	if limit == 0 {
		limit = 50
	}
	page, err := q.store.ListForAdmin(ctx, actor.ID, actor.OrgID, limit, input.After)
	if err != nil {
		return UserListPage{}, fmt.Errorf("list organization users: %w", err)
	}
	return page, nil
}

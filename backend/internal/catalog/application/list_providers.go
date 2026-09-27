package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// ErrInvalidProviderList means the page request or result is invalid.
var ErrInvalidProviderList = errors.New("invalid provider list")

// ProviderListCursor is an internal keyset position. Public encoding is an API concern.
type ProviderListCursor struct {
	CreateTime time.Time `json:"create_time"`
	ID         uuid.UUID `json:"id"`
}

// ProviderListItem exposes only display metadata and a real model count.
type ProviderListItem struct {
	Provider   CreatedProvider            `json:"provider"`
	Credential *ProviderCredentialSummary `json:"credential"`
	ModelCount int                        `json:"model_count"`
}

// ProviderListPage contains one bounded page and its next keyset position.
type ProviderListPage struct {
	Providers []ProviderListItem  `json:"providers"`
	Next      *ProviderListCursor `json:"next"`
}

// ListProvidersStore rechecks current administrator rights in its read transaction.
type ListProvidersStore interface {
	ListProvidersForAdmin(context.Context, uuid.UUID, uuid.UUID, int, *ProviderListCursor) (ProviderListPage, error)
}

// ListProvidersInput requests a bounded internal page.
type ListProvidersInput struct {
	Limit int
	After *ProviderListCursor
}

// ListProvidersQuery reads safe, paginated provider summaries.
type ListProvidersQuery struct{ store ListProvidersStore }

// NewListProvidersQuery injects the authorized list store.
func NewListProvidersQuery(store ListProvidersStore) *ListProvidersQuery {
	return &ListProvidersQuery{store: store}
}

// Execute validates the principal, page bounds, and returned summaries.
func (q *ListProvidersQuery) Execute(ctx context.Context, actor identityapp.Principal, input ListProvidersInput) (ProviderListPage, error) {
	if q == nil || q.store == nil {
		return ProviderListPage{}, ErrInvalidProviderList
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.Role != identitydomain.RoleAdmin || actor.MustChangePassword {
		return ProviderListPage{}, identityapp.ErrForbidden
	}
	if input.Limit < 0 || input.Limit > 200 ||
		(input.After != nil && (input.After.ID == uuid.Nil || input.After.CreateTime.IsZero())) {
		return ProviderListPage{}, ErrInvalidProviderList
	}
	limit := input.Limit
	if limit == 0 {
		limit = 50
	}
	page, err := q.store.ListProvidersForAdmin(ctx, actor.ID, actor.OrgID, limit, input.After)
	if err != nil {
		return ProviderListPage{}, fmt.Errorf("list providers: %w", err)
	}
	if len(page.Providers) > limit ||
		(page.Next != nil && (page.Next.ID == uuid.Nil || page.Next.CreateTime.IsZero() || len(page.Providers) == 0)) {
		return ProviderListPage{}, ErrInvalidProviderList
	}
	for _, item := range page.Providers {
		provider := domain.Provider{
			ID: item.Provider.ID, Key: item.Provider.Key, Name: item.Provider.Name,
			AdapterKey: item.Provider.AdapterKey, Region: item.Provider.Region,
			Status: item.Provider.Status, ConcurrencyLimit: item.Provider.ConcurrencyLimit,
			RateLimitPerMin: item.Provider.RateLimitPerMin, Revision: item.Provider.Revision,
		}
		if provider.Validate() != nil || item.Provider.CreateTime.IsZero() ||
			item.Provider.UpdateTime.IsZero() || item.ModelCount < 0 ||
			(item.Credential != nil && !validProviderCredentialSummary(*item.Credential, provider.ID)) {
			return ProviderListPage{}, ErrInvalidProviderList
		}
	}
	return page, nil
}

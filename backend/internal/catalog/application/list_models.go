package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// ErrInvalidModelCatalog means the internal request or stored page is invalid.
var ErrInvalidModelCatalog = errors.New("invalid model catalog")

// ModelCatalogCursor is an internal keyset position; API encoding is separate.
type ModelCatalogCursor struct {
	Key string
	ID  uuid.UUID
}

// ModelCatalogVersion is the current published configuration safe for display.
type ModelCatalogVersion struct {
	ID          uuid.UUID
	VersionNo   int
	Modes       []string
	Limits      json.RawMessage
	ParamSchema json.RawMessage
}

// ModelCatalogPrice is the latest currently effective price rule.
type ModelCatalogPrice struct {
	ID            uuid.UUID
	VersionNo     int
	Unit          string
	Rule          json.RawMessage
	Currency      string
	FXRateToCNY   *string
	EffectiveFrom time.Time
}

// ModelCatalogItem contains internal display data without credential secrets.
type ModelCatalogItem struct {
	ID                   uuid.UUID
	Key                  string
	DisplayName          string
	Capability           string
	Status               domain.ModelStatus
	ProviderID           uuid.UUID
	ProviderName         string
	ProviderRegion       domain.Region
	ProviderStatus       domain.ProviderStatus
	CredentialPresent    bool
	CredentialTestResult *domain.TestResult
	CurrentVersion       *ModelCatalogVersion
	CurrentPrice         *ModelCatalogPrice
}

// ModelCatalogPage contains one bounded internal keyset page.
type ModelCatalogPage struct {
	Models []ModelCatalogItem
	Next   *ModelCatalogCursor
}

// ListModelsInput scopes the catalog to a live project and optional filters.
type ListModelsInput struct {
	ProjectID  uuid.UUID
	Capability string
	Mode       string
	Limit      int
	After      *ModelCatalogCursor
}

// ListModelsStore rechecks the actor, organization, and project in one read transaction.
type ListModelsStore interface {
	ListModelsForProject(context.Context, identityapp.Principal, ListModelsInput) (ModelCatalogPage, error)
}

// ListModelsQuery validates the internal request and result bounds.
type ListModelsQuery struct{ store ListModelsStore }

// NewListModelsQuery injects the project-scoped catalog reader.
func NewListModelsQuery(store ListModelsStore) *ListModelsQuery {
	return &ListModelsQuery{store: store}
}

// Execute returns display data for an actor with current project access.
func (q *ListModelsQuery) Execute(ctx context.Context, actor identityapp.Principal, input ListModelsInput) (ModelCatalogPage, error) {
	if q == nil || q.store == nil {
		return ModelCatalogPage{}, ErrInvalidModelCatalog
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.MustChangePassword ||
		(actor.Role != identitydomain.RoleAdmin && actor.Role != identitydomain.RoleProducer) {
		return ModelCatalogPage{}, identityapp.ErrForbidden
	}
	if input.ProjectID == uuid.Nil || input.Limit < 0 || input.Limit > 200 ||
		(input.After != nil && (strings.TrimSpace(input.After.Key) == "" || input.After.ID == uuid.Nil)) {
		return ModelCatalogPage{}, ErrInvalidModelCatalog
	}
	if input.Limit == 0 {
		input.Limit = 50
	}
	page, err := q.store.ListModelsForProject(ctx, actor, input)
	if err != nil {
		return ModelCatalogPage{}, fmt.Errorf("list project models: %w", err)
	}
	if len(page.Models) > input.Limit ||
		(page.Next != nil && (len(page.Models) == 0 || strings.TrimSpace(page.Next.Key) == "" || page.Next.ID == uuid.Nil)) {
		return ModelCatalogPage{}, ErrInvalidModelCatalog
	}
	if page.Models == nil {
		page.Models = []ModelCatalogItem{}
	}
	return page, nil
}

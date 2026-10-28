package application

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// AssetCommandItem binds an object to the revisions displayed before confirmation.
type AssetCommandItem struct {
	ID                       uuid.UUID `json:"id"`
	ExpectedRevision         Revision  `json:"expected_revision" swaggertype:"string"`
	ExpectedFavoriteRevision Revision  `json:"expected_favorite_revision,omitempty" swaggertype:"string"`
	ExpectedProjectRevision  int64     `json:"expected_project_revision,omitempty"`
}

// AssetCommandInput is a closed object-level mutation; rename is always single-item.
type AssetCommandInput struct {
	Action   string             `json:"action" enums:"favorite,rename,delete"`
	Favorite *bool              `json:"favorite,omitempty"`
	Name     string             `json:"name,omitempty"`
	Items    []AssetCommandItem `json:"items"`
}

// Validate rejects mixed operations, duplicate objects, and missing revisions.
func (i AssetCommandInput) Validate() error {
	if len(i.Items) == 0 || len(i.Items) > 100 {
		return domain.ErrInvalidCommand
	}
	switch i.Action {
	case "favorite":
		if i.Favorite == nil || i.Name != "" {
			return domain.ErrInvalidCommand
		}
	case "rename":
		if len(i.Items) != 1 || i.Favorite != nil || strings.TrimSpace(i.Name) != i.Name || i.Name == "" || !utf8.ValidString(i.Name) || utf8.RuneCountInString(i.Name) > 50 {
			return domain.ErrInvalidCommand
		}
	case "delete":
		if i.Favorite != nil || i.Name != "" {
			return domain.ErrInvalidCommand
		}
	default:
		return domain.ErrInvalidCommand
	}
	seen := make(map[uuid.UUID]bool)
	for _, item := range i.Items {
		if item.ID == uuid.Nil || seen[item.ID] || item.ExpectedRevision < 0 || item.ExpectedFavoriteRevision < 0 || item.ExpectedProjectRevision < 0 || (i.Action != "favorite" && item.ExpectedRevision < 1) {
			return domain.ErrInvalidCommand
		}
		seen[item.ID] = true
	}
	return nil
}

// AssetCommandResult reports only safe individual outcomes and their current revisions.
type AssetCommandResult struct {
	ID               uuid.UUID `json:"id"`
	Status           string    `json:"status" enums:"succeeded,failed"`
	Code             string    `json:"code"`
	Revision         Revision  `json:"revision" swaggertype:"string"`
	ProjectRevision  int64     `json:"project_revision"`
	Favorite         bool      `json:"favorite"`
	FavoriteRevision Revision  `json:"favorite_revision" swaggertype:"string"`
}

// AssetCommandReceipt preserves partial outcomes for an exact idempotent replay.
type AssetCommandReceipt struct {
	Results   []AssetCommandResult `json:"results"`
	Succeeded int                  `json:"succeeded"`
	Failed    int                  `json:"failed"`
}

// AssetMutationFacts contains locked routing identity, not a client ownership claim.
type AssetMutationFacts struct {
	ID, ProjectID   uuid.UUID
	IsMain          bool
	Revision        int64
	ProjectRevision int64
	Collection      string
}

// AssetMutationAccess delegates existing lifecycle owners inside the same transaction.
type AssetMutationAccess interface {
	AuthorizeAssetCommand(context.Context, identity.Principal, bool) error
	AuthorizeAssetProject(context.Context, identity.Principal, uuid.UUID, bool) error
	RenameAsset(context.Context, identity.Principal, AssetMutationFacts, uuid.UUID, AssetCommandItem, string) (AssetCommandResult, error)
	DeleteAsset(context.Context, identity.Principal, AssetMutationFacts, uuid.UUID, AssetCommandItem) (AssetCommandResult, error)
}

// AssetActionError is a known per-item business refusal, safe to return to clients.
type AssetActionError struct{ Code string }

func (e *AssetActionError) Error() string { return e.Code }

// AssetCommandStore atomically persists outcomes with their object mutations.
type AssetCommandStore interface {
	ApplyAssetCommand(context.Context, identity.Principal, uuid.UUID, AssetCommandInput) (AssetCommandReceipt, error)
}

// AssetCommands validates a request before invoking the mutation owner.
type AssetCommands struct{ store AssetCommandStore }

// NewAssetCommands injects the authorized object mutation store.
func NewAssetCommands(store AssetCommandStore) *AssetCommands { return &AssetCommands{store: store} }

// Apply executes only an explicit user-confirmed action under a stable request key.
func (s *AssetCommands) Apply(ctx context.Context, actor identity.Principal, key uuid.UUID, input AssetCommandInput) (AssetCommandReceipt, error) {
	if key == uuid.Nil {
		return AssetCommandReceipt{}, domain.ErrInvalidCommand
	}
	if err := input.Validate(); err != nil {
		return AssetCommandReceipt{}, err
	}
	return s.store.ApplyAssetCommand(ctx, actor, key, input)
}

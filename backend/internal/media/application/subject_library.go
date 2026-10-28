package application

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// SubjectSourceActor first authorizes the target space, then resolves only the
// caller's personal source. It never accepts an owner supplied by the browser.
type SubjectSourceActor interface {
	PersonalSourceActor(context.Context, identity.Principal, bool) (identity.Principal, error)
}

// SubjectCatalogueQuery filters the full authorized mixed catalogue before paging.
type SubjectCatalogueQuery struct {
	Kind         string
	Search       string
	FavoriteOnly bool
	Quality      string
	Page         int
	PageSize     int
}

// Validate permits only source filters, never caller-selected ownership.
func (q SubjectCatalogueQuery) Validate() error {
	if !slices.Contains([]string{"image", "video", "audio"}, q.Kind) || q.Page < 1 || q.Page > 100000 || q.PageSize < 1 || q.PageSize > 120 || !slices.Contains([]string{"", "upscaled"}, q.Quality) || q.Quality == "upscaled" && q.Kind == "audio" || !utf8.ValidString(q.Search) || utf8.RuneCountInString(q.Search) > 512 || strings.ContainsRune(q.Search, 0) {
		return ErrInvalidQuery
	}
	return nil
}

// SubjectSourceCapabilities describes supported catalogue predicates.
type SubjectSourceCapabilities struct {
	UpscaledFilter bool   `json:"upscaled_filter"`
	UpscaledReason string `json:"upscaled_reason,omitempty"`
}

// SubjectSourceMedia retains the actual origin of an original, including system TTS.
type SubjectSourceMedia struct {
	LibraryAssetSummary
	Origin string `json:"origin"`
}

// SubjectSourceItem identifies its real owner without pretending to be a library.
type SubjectSourceItem struct {
	ID        uuid.UUID          `json:"id"`
	Source    string             `json:"source"`
	ProjectID *uuid.UUID         `json:"project_id,omitempty" extensions:"x-nullable"`
	Revision  int64              `json:"revision"`
	Favorite  bool               `json:"favorite"`
	Upscaled  bool               `json:"upscaled"`
	Title     string             `json:"title"`
	Kind      string             `json:"kind"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
	Media     SubjectSourceMedia `json:"media"`
}

// Validate rejects ambiguous source identity or incomplete ready asset facts.
func (i SubjectSourceItem) Validate() error {
	if i.ID == uuid.Nil || i.Media.ID == uuid.Nil || i.Revision < 0 || i.Media.Revision < 1 || i.Media.ByteSize < 1 || !slices.Contains([]string{"image", "video", "audio"}, i.Kind) || i.Media.Kind != i.Kind || strings.TrimSpace(i.Title) == "" || i.Media.MIMEType == "" || i.CreatedAt.IsZero() || i.UpdatedAt.IsZero() || !slices.Contains([]string{"upload", "generated", "system"}, i.Media.Origin) {
		return ErrUnavailable
	}
	if i.Source == "personal_asset" && i.ProjectID == nil && i.ID == i.Media.ID && !i.Upscaled {
		return nil
	}
	if i.Source == "generated_output" && i.ProjectID != nil && *i.ProjectID != uuid.Nil {
		return nil
	}
	return ErrUnavailable
}

// SubjectSourcePage is a mixed source catalogue bound to the target space.
type SubjectSourcePage struct {
	CurrentActorID uuid.UUID                 `json:"current_actor_id"`
	CurrentScopeID uuid.UUID                 `json:"current_scope_id"`
	Items          []SubjectSourceItem       `json:"items"`
	Page           int                       `json:"page"`
	PageSize       int                       `json:"page_size"`
	Total          int64                     `json:"total"`
	Capabilities   SubjectSourceCapabilities `json:"capabilities"`
}

// SubjectCatalogue owns source selection and proves initiator and current rights.
type SubjectCatalogue interface {
	List(context.Context, identity.Principal, SubjectCatalogueQuery) (SubjectSourcePage, error)
	Detail(context.Context, identity.Principal, uuid.UUID) (SubjectSourceItem, error)
}

// SubjectSourcePreview signs only a server-resolved ordinary media owner.
type SubjectSourcePreview interface {
	Preview(context.Context, identity.Principal, domain.LibraryScope, uuid.UUID) (LibraryMediaPreview, error)
}

// SubjectLibrary shares actual media queries without widening normal routes.
type SubjectLibrary struct {
	actor     SubjectSourceActor
	catalogue SubjectCatalogue
	files     SubjectSourcePreview
}

// NewSubjectLibrary injects source authority, existing metadata, and private I/O.
func NewSubjectLibrary(actor SubjectSourceActor, catalogue SubjectCatalogue, files SubjectSourcePreview) *SubjectLibrary {
	return &SubjectLibrary{actor: actor, catalogue: catalogue, files: files}
}

// SourceActor authorizes a target before upload admission reads any bytes.
func (s *SubjectLibrary) SourceActor(ctx context.Context, actor identity.Principal, write bool) (identity.Principal, error) {
	if s == nil || s.actor == nil {
		return identity.Principal{}, ErrUnavailable
	}
	resolved, err := s.actor.PersonalSourceActor(ctx, actor, write)
	if err != nil {
		return identity.Principal{}, err
	}
	if resolved.ID != actor.ID || resolved.SessionID != actor.SessionID || resolved.ID == uuid.Nil || resolved.ScopeID == uuid.Nil || resolved.OrgID == uuid.Nil {
		return identity.Principal{}, ErrNotFound
	}
	return resolved, nil
}

// List retains target authority before reading a complete mixed catalogue.
func (s *SubjectLibrary) List(ctx context.Context, actor identity.Principal, query SubjectCatalogueQuery) (SubjectSourcePage, error) {
	_, err := s.SourceActor(ctx, actor, false)
	if err != nil {
		return SubjectSourcePage{}, err
	}
	if s.catalogue == nil {
		return SubjectSourcePage{}, ErrUnavailable
	}
	if query.Validate() != nil {
		return SubjectSourcePage{}, ErrInvalidQuery
	}
	page, err := s.catalogue.List(ctx, actor, query)
	if err != nil {
		return SubjectSourcePage{}, err
	}
	if page.CurrentActorID != actor.ID || page.CurrentScopeID != actor.ScopeID || page.Page != query.Page || page.PageSize != query.PageSize || page.Total < 0 || len(page.Items) > page.PageSize {
		return SubjectSourcePage{}, ErrUnavailable
	}
	seen := make(map[uuid.UUID]bool, len(page.Items))
	for _, item := range page.Items {
		if item.Validate() != nil || seen[item.ID] {
			return SubjectSourcePage{}, ErrUnavailable
		}
		seen[item.ID] = true
	}
	return page, nil
}

// Detail checks a currently selectable source instead of trusting a stale card.
func (s *SubjectLibrary) Detail(ctx context.Context, actor identity.Principal, id uuid.UUID) (SubjectSourceItem, error) {
	_, err := s.SourceActor(ctx, actor, false)
	if err != nil {
		return SubjectSourceItem{}, err
	}
	if s.catalogue == nil || id == uuid.Nil {
		return SubjectSourceItem{}, ErrUnavailable
	}
	detail, err := s.catalogue.Detail(ctx, actor, id)
	if err != nil {
		return SubjectSourceItem{}, err
	}
	if detail.ID != id || detail.Validate() != nil {
		return SubjectSourceItem{}, ErrNotFound
	}
	return detail, nil
}

// Preview returns only the source owner's expiring private lease.
func (s *SubjectLibrary) Preview(ctx context.Context, actor identity.Principal, id uuid.UUID) (LibraryMediaPreview, error) {
	item, err := s.Detail(ctx, actor, id)
	if err != nil {
		return LibraryMediaPreview{}, err
	}
	if s.files == nil {
		return LibraryMediaPreview{}, ErrUnavailable
	}
	owner := actor
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: item.ProjectID}
	if item.Source == "personal_asset" {
		owner, err = s.SourceActor(ctx, actor, false)
		if err != nil {
			return LibraryMediaPreview{}, err
		}
		scope = domain.LibraryScope{Kind: domain.LibraryPersonal}
	}
	preview, err := s.files.Preview(ctx, owner, scope, item.Media.ID)
	if err != nil {
		return LibraryMediaPreview{}, err
	}
	current, err := s.Detail(ctx, actor, id)
	if err != nil {
		return LibraryMediaPreview{}, err
	}
	sameProject := current.ProjectID == nil && item.ProjectID == nil || current.ProjectID != nil && item.ProjectID != nil && *current.ProjectID == *item.ProjectID
	if current.Source != item.Source || !sameProject || current.Revision != item.Revision || current.Media.ID != item.Media.ID || current.Media.Revision != item.Media.Revision || preview.Asset.ID != item.Media.ID || preview.Asset.Revision != item.Media.Revision {
		return LibraryMediaPreview{}, ErrNotFound
	}
	return preview, nil
}

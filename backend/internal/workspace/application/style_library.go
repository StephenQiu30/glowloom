package application

import (
	"context"
	"errors"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

var (
	// ErrInvalidStyleLibrary rejects malformed library filters and commands.
	ErrInvalidStyleLibrary = errors.New("invalid style library request")
	// ErrStyleLibraryRevision reports an outdated observed style revision.
	ErrStyleLibraryRevision = errors.New("style library revision conflict")
	// ErrStyleLibraryReference reports a reference that cannot be adopted now.
	ErrStyleLibraryReference = errors.New("style reference unavailable")
	// ErrStyleLibraryInUse preserves existing project bindings during deletion.
	ErrStyleLibraryInUse = errors.New("style preset is bound to a project")
)

// StyleLibraryReference identifies the authorized personal catalog item and original.
type StyleLibraryReference struct {
	ID            uuid.UUID `json:"id"`
	Revision      int64     `json:"revision"`
	AssetID       uuid.UUID `json:"asset_id"`
	AssetRevision int64     `json:"asset_revision"`
	Title         string    `json:"title"`
}

// StyleLibraryEntry is the bounded card projection; it contains no prompt text.
type StyleLibraryEntry struct {
	ID                  uuid.UUID               `json:"id"`
	Name                string                  `json:"name"`
	Category            string                  `json:"category"`
	Revision            int64                   `json:"revision"`
	CoverItemID         *uuid.UUID              `json:"cover_item_id,omitempty"`
	ReferenceItems      []StyleLibraryReference `json:"reference_items"`
	ReferencesAvailable bool                    `json:"references_available"`
	CoverAssetID        *uuid.UUID              `json:"cover_asset_id" extensions:"x-nullable"`
	ReferenceAssetIDs   []uuid.UUID             `json:"reference_asset_ids"`
	Owned               bool                    `json:"owned"`
	CanEdit             bool                    `json:"can_edit"`
}

// StyleLibraryDetail is an authorized, revisioned snapshot for explicit adoption.
type StyleLibraryDetail struct {
	StyleLibraryEntry
	PromptFragment string `json:"prompt_fragment"`
	NegativePrompt string `json:"negative_prompt"`
}

// StyleLibraryFilter separates optional ownership filtering from categories.
type StyleLibraryFilter struct {
	Query, Category string
	Mine            *bool
	Limit           int
	After           *StylePresetCursor
}

// StyleLibraryPage preserves stable name/ID pagination.
type StyleLibraryPage struct {
	Items []StyleLibraryEntry `json:"items"`
	Next  *StylePresetCursor  `json:"-"`
}

// StyleLibraryMutation carries one immutable command and its observed revision.
type StyleLibraryMutation struct {
	Action                         string
	ID                             uuid.UUID
	IdempotencyKey                 uuid.UUID
	RequestID                      string
	ExpectedRevision               int64
	Name, PromptFragment, Category string
	ReferenceAssetIDs              []uuid.UUID
}

// StyleLibraryChangeResult is replayed unchanged for an identical command key.
type StyleLibraryChangeResult struct {
	RequestID uuid.UUID          `json:"request_id"`
	Action    string             `json:"action"`
	Preset    StyleLibraryDetail `json:"preset"`
	Deleted   bool               `json:"deleted"`
}

// StyleLibraryRepository owns transactional authorization, image proofs and receipts.
type StyleLibraryRepository interface {
	ListStyleLibrary(context.Context, identity.Principal, StyleLibraryFilter) (StyleLibraryPage, error)
	ReadStyleLibrary(context.Context, identity.Principal, uuid.UUID) (StyleLibraryDetail, error)
	ChangeStyleLibrary(context.Context, identity.Principal, StyleLibraryMutation) (StyleLibraryChangeResult, error)
}

// StyleLibrary validates the public style library contract independently of SQL.
type StyleLibrary struct{ store StyleLibraryRepository }

// NewStyleLibrary injects the transactional repository.
func NewStyleLibrary(store StyleLibraryRepository) *StyleLibrary { return &StyleLibrary{store: store} }
func styleLibraryActorValid(a identity.Principal) bool {
	return a.ID != uuid.Nil && a.OrgID != uuid.Nil && a.ScopeID != uuid.Nil && a.SessionID != uuid.Nil && a.CredentialEpoch > 0
}
func validLibraryCategory(v string) bool { return v == "realistic" || v == "2d" || v == "3d" }

// List returns a bounded authorized card page.
func (s *StyleLibrary) List(ctx context.Context, a identity.Principal, in StyleLibraryFilter) (StyleLibraryPage, error) {
	if !styleLibraryActorValid(a) {
		return StyleLibraryPage{}, identity.ErrForbidden
	}
	if s == nil || s.store == nil {
		return StyleLibraryPage{}, ErrProjectDependencyUnavailable
	}
	in.Query = strings.TrimSpace(in.Query)
	if strings.ContainsRune(in.Query, 0) || !utf8.ValidString(in.Query) || utf8.RuneCountInString(in.Query) > 100 || in.Limit < 0 || in.Limit > 100 || (in.Category != "" && !validLibraryCategory(in.Category)) || (in.After != nil && (in.After.ID == uuid.Nil || strings.ContainsRune(in.After.Name, 0) || !utf8.ValidString(in.After.Name) || len(in.After.Name) > 4096)) {
		return StyleLibraryPage{}, ErrInvalidStyleLibrary
	}
	if in.Limit == 0 {
		in.Limit = 24
	}
	page, err := s.store.ListStyleLibrary(ctx, a, in)
	if err != nil {
		return StyleLibraryPage{}, err
	}
	if page.Items == nil {
		page.Items = []StyleLibraryEntry{}
	}
	return page, nil
}

// Read returns the currently authorized editable style snapshot.
func (s *StyleLibrary) Read(ctx context.Context, a identity.Principal, id uuid.UUID) (StyleLibraryDetail, error) {
	if !styleLibraryActorValid(a) {
		return StyleLibraryDetail{}, identity.ErrForbidden
	}
	if s == nil || s.store == nil {
		return StyleLibraryDetail{}, ErrProjectDependencyUnavailable
	}
	if id == uuid.Nil {
		return StyleLibraryDetail{}, ErrInvalidStyleLibrary
	}
	return s.store.ReadStyleLibrary(ctx, a, id)
}

// Change validates one permanent-key mutation before persistence.
func (s *StyleLibrary) Change(ctx context.Context, a identity.Principal, in StyleLibraryMutation) (StyleLibraryChangeResult, error) {
	if !styleLibraryActorValid(a) {
		return StyleLibraryChangeResult{}, identity.ErrForbidden
	}
	if s == nil || s.store == nil {
		return StyleLibraryChangeResult{}, ErrProjectDependencyUnavailable
	}
	in.Name = strings.TrimSpace(in.Name)
	in.PromptFragment = strings.TrimSpace(in.PromptFragment)
	requestID, err := uuid.Parse(in.RequestID)
	if in.IdempotencyKey == uuid.Nil || err != nil || requestID == uuid.Nil {
		return StyleLibraryChangeResult{}, ErrInvalidStyleLibrary
	}
	if in.Action != "create" && in.Action != "patch" && in.Action != "delete" {
		return StyleLibraryChangeResult{}, ErrInvalidStyleLibrary
	}
	if in.Action == "create" {
		if in.ID != uuid.Nil || in.ExpectedRevision != 0 {
			return StyleLibraryChangeResult{}, ErrInvalidStyleLibrary
		}
	} else if in.ID == uuid.Nil || in.ExpectedRevision < 1 || in.ExpectedRevision >= math.MaxInt32 {
		return StyleLibraryChangeResult{}, ErrInvalidStyleLibrary
	}
	if in.Action != "delete" {
		if strings.ContainsRune(in.Name, 0) || strings.ContainsRune(in.PromptFragment, 0) || !utf8.ValidString(in.Name) || utf8.RuneCountInString(in.Name) < 1 || utf8.RuneCountInString(in.Name) > 20 || !utf8.ValidString(in.PromptFragment) || utf8.RuneCountInString(in.PromptFragment) < 1 || utf8.RuneCountInString(in.PromptFragment) > 2000 || !validLibraryCategory(in.Category) || len(in.ReferenceAssetIDs) > 4 {
			return StyleLibraryChangeResult{}, ErrInvalidStyleLibrary
		}
		seen := make(map[uuid.UUID]bool, len(in.ReferenceAssetIDs))
		for _, id := range in.ReferenceAssetIDs {
			if id == uuid.Nil || seen[id] {
				return StyleLibraryChangeResult{}, ErrInvalidStyleLibrary
			}
			seen[id] = true
		}
		if in.ReferenceAssetIDs == nil {
			in.ReferenceAssetIDs = []uuid.UUID{}
		}
	} else if in.Name != "" || in.PromptFragment != "" || in.Category != "" || len(in.ReferenceAssetIDs) > 0 {
		return StyleLibraryChangeResult{}, ErrInvalidStyleLibrary
	}
	return s.store.ChangeStyleLibrary(ctx, a, in)
}

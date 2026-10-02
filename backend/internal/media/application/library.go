package application

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

var (
	// ErrLibraryConflict reports a stale catalog or entity revision.
	ErrLibraryConflict = errors.New("media library revision conflict")
	// ErrLibraryKeyConflict rejects a different request under a permanent key.
	ErrLibraryKeyConflict = errors.New("media library idempotency conflict")
	// ErrLibraryFolderNotEmpty preserves all content in a nonempty project folder.
	ErrLibraryFolderNotEmpty = errors.New("media library folder is not empty")
)

// LibraryQuery describes full server-side filtering before a stable page is selected.
type LibraryQuery struct {
	Page, PageSize int
	Kind           string
	Category       string
	FolderID       *uuid.UUID
	RootOnly       bool
	FavoriteOnly   bool
	RecentOnly     bool
	State          string
	Search         string
	Order          string
}

// Validate permits only the mounted source's query semantics and bounded pages.
func (q LibraryQuery) Validate(scope domain.LibraryScope) error {
	if scope.Validate() != nil || q.Page < 1 || q.Page > 100000 || q.PageSize < 1 || q.PageSize > 120 ||
		!slices.Contains([]string{"", "text", "image", "video", "audio", "document", "model"}, q.Kind) ||
		!slices.Contains([]string{"", "character", "environment", "prop", "material", "other"}, q.Category) ||
		!slices.Contains([]string{"active", "trashed"}, q.State) ||
		!slices.Contains([]string{"updated_desc", "updated_asc", "name_asc"}, q.Order) ||
		q.FolderID != nil && (*q.FolderID == uuid.Nil || q.RootOnly) || q.FavoriteOnly && scope.Kind != domain.LibraryPersonal ||
		!utf8.ValidString(q.Search) || utf8.RuneCountInString(q.Search) > 512 || strings.ContainsRune(q.Search, 0) {
		return domain.ErrInvalidLibrary
	}
	return nil
}

// LibraryAssetSummary returns actual media facts without object keys or execution metadata.
type LibraryAssetSummary struct {
	ID         uuid.UUID `json:"id"`
	Kind       string    `json:"kind"`
	FileName   string    `json:"file_name"`
	MIMEType   string    `json:"mime_type"`
	ByteSize   int64     `json:"byte_size"`
	Width      *int32    `json:"width" extensions:"x-nullable"`
	Height     *int32    `json:"height" extensions:"x-nullable"`
	DurationMS *int32    `json:"duration_ms" extensions:"x-nullable"`
	Revision   int64     `json:"revision"`
}

// LibraryItemSummary excludes long plain text; Detail explicitly returns it.
type LibraryItemSummary struct {
	ID          uuid.UUID            `json:"id"`
	AssetID     *uuid.UUID           `json:"asset_id" extensions:"x-nullable"`
	FolderID    *uuid.UUID           `json:"folder_id" extensions:"x-nullable"`
	Kind        string               `json:"kind"`
	Title       string               `json:"title"`
	Category    string               `json:"category"`
	Tags        []string             `json:"tags"`
	SourceLabel string               `json:"source_label"`
	Note        string               `json:"note"`
	Favorite    bool                 `json:"favorite"`
	State       string               `json:"catalog_state"`
	TrashedAt   *time.Time           `json:"trashed_at" extensions:"x-nullable"`
	Position    int64                `json:"position"`
	Revision    int64                `json:"revision"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
	Media       *LibraryAssetSummary `json:"media,omitempty" extensions:"x-nullable"`
}

// LibraryItemDetail has no file URL; previews require a current owning authorization.
type LibraryItemDetail struct {
	LibraryItemSummary
	PlainText *string `json:"plain_text,omitempty" extensions:"x-nullable"`
}

// LibraryPage binds pages and facets to the current identity and complete scope.
type LibraryPage struct {
	CurrentActorID uuid.UUID              `json:"current_actor_id"`
	CurrentOrgID   uuid.UUID              `json:"current_org_id"`
	LibraryID      uuid.UUID              `json:"library_id"`
	Scope          domain.LibraryScope    `json:"scope"`
	Revision       int64                  `json:"revision"`
	Page           int                    `json:"page"`
	PageSize       int                    `json:"page_size"`
	Total          int64                  `json:"total"`
	Items          []LibraryItemSummary   `json:"items"`
	CategoryCounts map[string]int64       `json:"category_counts"`
	FolderCounts   map[string]int64       `json:"folder_counts"`
	Folders        []domain.LibraryFolder `json:"folders"`
}

// LibraryMetadata contains only editable text and classification.
type LibraryMetadata struct {
	PlainText   *string    `json:"plain_text,omitempty" extensions:"x-nullable"`
	FolderID    *uuid.UUID `json:"folder_id" extensions:"x-nullable"`
	Title       string     `json:"title"`
	Category    string     `json:"category"`
	Tags        []string   `json:"tags"`
	SourceLabel string     `json:"source_label"`
	Note        string     `json:"note"`
	Favorite    bool       `json:"favorite"`
}

// LibraryFolderInput is a complete editable folder draft, not an arbitrary tree node.
type LibraryFolderInput struct {
	ParentID *uuid.UUID `json:"parent_id" extensions:"x-nullable"`
	Name     string     `json:"name"`
	Style    string     `json:"style"`
	Theme    string     `json:"theme"`
}

// LibraryItemRevision binds every batch item to its displayed catalog revision.
// Revision zero denotes a real legacy asset with no metadata row.
type LibraryItemRevision struct {
	ID       uuid.UUID `json:"id"`
	Revision int64     `json:"revision"`
}

// LibraryCommand freezes one supported metadata action and its optimistic revisions.
type LibraryCommand struct {
	Scope                  domain.LibraryScope   `json:"scope"`
	Key                    uuid.UUID             `json:"-"`
	ExpectedRevision       int64                 `json:"expected_revision"`
	Action                 string                `json:"action"`
	FolderID               *uuid.UUID            `json:"folder_id,omitempty" extensions:"x-nullable"`
	ExpectedFolderRevision int64                 `json:"expected_folder_revision,omitempty"`
	Folder                 *LibraryFolderInput   `json:"folder,omitempty" extensions:"x-nullable"`
	ItemID                 *uuid.UUID            `json:"item_id,omitempty" extensions:"x-nullable"`
	ExpectedItemRevision   int64                 `json:"expected_item_revision,omitempty"`
	Metadata               *LibraryMetadata      `json:"metadata,omitempty" extensions:"x-nullable"`
	Items                  []LibraryItemRevision `json:"items,omitempty"`
	TargetFolderID         *uuid.UUID            `json:"target_folder_id,omitempty" extensions:"x-nullable"`
}

// Validate checks command shape before any admission lock or persistence.
func (c LibraryCommand) Validate() error {
	if c.Scope.Validate() != nil || c.Key == uuid.Nil || c.ExpectedRevision < 0 || c.ExpectedRevision > 2147483646 ||
		c.FolderID != nil && *c.FolderID == uuid.Nil || c.ItemID != nil && *c.ItemID == uuid.Nil ||
		c.TargetFolderID != nil && *c.TargetFolderID == uuid.Nil {
		return domain.ErrInvalidLibrary
	}
	noItem := c.ItemID == nil && c.Metadata == nil && c.ExpectedItemRevision == 0 && len(c.Items) == 0 && c.TargetFolderID == nil
	noFolder := c.FolderID == nil && c.Folder == nil && c.ExpectedFolderRevision == 0
	switch c.Action {
	case "create_folder":
		if !noItem || c.FolderID != nil || c.Folder == nil || c.ExpectedFolderRevision != 0 {
			return domain.ErrInvalidLibrary
		}
	case "update_folder":
		if !noItem || c.FolderID == nil || c.Folder == nil || c.ExpectedFolderRevision < 1 {
			return domain.ErrInvalidLibrary
		}
	case "delete_folder":
		if !noItem || c.FolderID == nil || c.Folder != nil || c.ExpectedFolderRevision < 1 {
			return domain.ErrInvalidLibrary
		}
	case "create_text":
		if !noFolder || c.ItemID != nil || c.ExpectedItemRevision != 0 || c.Metadata == nil || c.Metadata.PlainText == nil || len(c.Items) != 0 || c.TargetFolderID != nil {
			return domain.ErrInvalidLibrary
		}
	case "update_item":
		if !noFolder || c.ItemID == nil || c.ExpectedItemRevision < 0 || c.Metadata == nil || len(c.Items) != 0 || c.TargetFolderID != nil {
			return domain.ErrInvalidLibrary
		}
	case "move_items", "recycle_items", "restore_items", "remove_items":
		if !noFolder || c.ItemID != nil || c.Metadata != nil || c.ExpectedItemRevision != 0 || len(c.Items) < 1 || len(c.Items) > 200 || c.Action != "move_items" && c.TargetFolderID != nil || c.Action == "remove_items" && c.Scope.Kind != domain.LibraryProject {
			return domain.ErrInvalidLibrary
		}
		seen := make(map[uuid.UUID]bool, len(c.Items))
		for _, item := range c.Items {
			if item.ID == uuid.Nil || item.Revision < 0 || seen[item.ID] {
				return domain.ErrInvalidLibrary
			}
			seen[item.ID] = true
		}
	default:
		return domain.ErrInvalidLibrary
	}
	return nil
}

// LibraryItemChange is the bounded effect of a command. Long editable content
// is fetched through Detail, so 200 valid items never inflate a receipt.
type LibraryItemChange struct {
	ID        uuid.UUID  `json:"id"`
	AssetID   *uuid.UUID `json:"asset_id" extensions:"x-nullable"`
	FolderID  *uuid.UUID `json:"folder_id" extensions:"x-nullable"`
	Kind      string     `json:"kind"`
	State     string     `json:"catalog_state"`
	TrashedAt *time.Time `json:"trashed_at" extensions:"x-nullable"`
	Revision  int64      `json:"revision"`
}

// LibraryReceipt permanently binds the result to actor, scope and revisions.
type LibraryReceipt struct {
	CurrentActorID  uuid.UUID             `json:"current_actor_id"`
	CurrentOrgID    uuid.UUID             `json:"current_org_id"`
	LibraryID       uuid.UUID             `json:"library_id"`
	Scope           domain.LibraryScope   `json:"scope"`
	Revision        int64                 `json:"revision"`
	ProjectRevision *int64                `json:"project_revision,omitempty" extensions:"x-nullable"`
	Folder          *domain.LibraryFolder `json:"folder,omitempty" extensions:"x-nullable"`
	Items           []LibraryItemChange   `json:"items"`
}

// LibraryRepository owns complete scoped reads and one atomic permanent command.
type LibraryRepository interface {
	ListLibrary(context.Context, identityapp.Principal, domain.LibraryScope, LibraryQuery) (LibraryPage, error)
	LibraryDetail(context.Context, identityapp.Principal, domain.LibraryScope, uuid.UUID) (LibraryItemDetail, error)
	ApplyLibraryCommand(context.Context, identityapp.Principal, LibraryCommand) (LibraryReceipt, error)
}

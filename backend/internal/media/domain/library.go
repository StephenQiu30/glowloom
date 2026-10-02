package domain

import (
	"errors"
	"math"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ErrInvalidLibrary rejects ambiguous ownership, unsupported metadata and trees.
var ErrInvalidLibrary = errors.New("invalid media library")

// LibraryKind distinguishes project content from the current actor's private library.
type LibraryKind string

// Library kinds form the closed media ownership scopes.
const (
	LibraryProject  LibraryKind = "project"
	LibraryPersonal LibraryKind = "personal"
)

// PersonalOwnership has no project or execution identity.
type PersonalOwnership struct {
	OrgID   uuid.UUID `json:"org_id"`
	ActorID uuid.UUID `json:"actor_id"`
}

// LibraryScope never accepts an arbitrary personal actor from a public request.
// Current personal identity is supplied separately by the authenticated caller.
type LibraryScope struct {
	Kind      LibraryKind `json:"kind"`
	ProjectID *uuid.UUID  `json:"project_id,omitempty" extensions:"x-nullable"`
}

// Validate closes the two supported ownership choices.
func (s LibraryScope) Validate() error {
	if s.Kind == LibraryPersonal && s.ProjectID == nil || s.Kind == LibraryProject && s.ProjectID != nil && *s.ProjectID != uuid.Nil {
		return nil
	}
	return ErrInvalidLibrary
}

// Identity locates a library without creating a row during a read.
func (s LibraryScope) Identity(org, actor uuid.UUID) (uuid.UUID, error) {
	if s.Validate() != nil || org == uuid.Nil || actor == uuid.Nil {
		return uuid.Nil, ErrInvalidLibrary
	}
	if s.Kind == LibraryProject {
		return uuid.NewSHA1(uuid.Nil, []byte("media-library/project/"+org.String()+"/"+s.ProjectID.String())), nil
	}
	return uuid.NewSHA1(uuid.Nil, []byte("media-library/personal/"+org.String()+"/"+actor.String())), nil
}

// LibraryFolder retains the fixed source's position, style and theme choices.
type LibraryFolder struct {
	ID        uuid.UUID   `json:"id"`
	LibraryID uuid.UUID   `json:"library_id"`
	Kind      LibraryKind `json:"library_kind"`
	ParentID  *uuid.UUID  `json:"parent_id" extensions:"x-nullable"`
	Name      string      `json:"name"`
	Position  int64       `json:"position"`
	Style     string      `json:"style"`
	Theme     string      `json:"theme"`
	Revision  int64       `json:"revision"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// NameKey provides the same trimmed, case-insensitive sibling uniqueness as the source.
func (f LibraryFolder) NameKey() string { return strings.ToLower(strings.TrimSpace(f.Name)) }

// Validate checks one folder; placement additionally checks the complete scoped tree.
func (f LibraryFolder) Validate() error {
	limit := 60
	if f.Kind == LibraryPersonal {
		limit = 40
		if f.ParentID != nil || f.Style != "" || f.Theme != "" {
			return ErrInvalidLibrary
		}
	} else if f.Kind != LibraryProject || !slices.Contains([]string{"glass", "stacked", "midnight", "paper", "cinema", "compact"}, f.Style) || !slices.Contains([]string{"aurora", "obsidian", "ember", "pearl"}, f.Theme) {
		return ErrInvalidLibrary
	}
	if f.ID == uuid.Nil || f.LibraryID == uuid.Nil || f.ParentID != nil && (*f.ParentID == uuid.Nil || *f.ParentID == f.ID) ||
		f.Position < 0 || f.Position > math.MaxInt32 || f.Revision < 1 || f.Revision > math.MaxInt32 ||
		!libraryText(f.Name, limit, true) || strings.TrimSpace(f.Name) != f.Name {
		return ErrInvalidLibrary
	}
	return nil
}

// ValidateLibraryFolderPlacement checks sibling names, ancestry and every descendant
// after replacing a folder. Reparenting cannot hide an over-depth subtree or cycle.
func ValidateLibraryFolderPlacement(folder LibraryFolder, existing []LibraryFolder) error {
	if err := folder.Validate(); err != nil {
		return err
	}
	all := make(map[uuid.UUID]LibraryFolder, len(existing)+1)
	for _, current := range existing {
		if current.LibraryID != folder.LibraryID || current.Kind != folder.Kind || current.Validate() != nil {
			return ErrInvalidLibrary
		}
		if _, duplicate := all[current.ID]; duplicate {
			return ErrInvalidLibrary
		}
		if current.ID != folder.ID && sameLibraryParent(current.ParentID, folder.ParentID) && current.NameKey() == folder.NameKey() {
			return ErrInvalidLibrary
		}
		all[current.ID] = current
	}
	all[folder.ID] = folder
	siblings := make(map[string]bool, len(all))
	for _, current := range all {
		parent := "root"
		if current.ParentID != nil {
			parent = current.ParentID.String()
		}
		key := parent + "/" + current.NameKey()
		if siblings[key] {
			return ErrInvalidLibrary
		}
		siblings[key] = true
		seen := make(map[uuid.UUID]bool)
		for depth := 1; ; depth++ {
			if seen[current.ID] || depth > 8 {
				return ErrInvalidLibrary
			}
			seen[current.ID] = true
			if current.ParentID == nil {
				break
			}
			parent, found := all[*current.ParentID]
			if !found {
				return ErrInvalidLibrary
			}
			current = parent
		}
	}
	return nil
}

func sameLibraryParent(a, b *uuid.UUID) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// LibraryItem contains editable catalog data, never media execution or review facts.
type LibraryItem struct {
	ID          uuid.UUID  `json:"id"`
	LibraryID   uuid.UUID  `json:"library_id"`
	AssetID     *uuid.UUID `json:"asset_id" extensions:"x-nullable"`
	PlainText   *string    `json:"plain_text,omitempty" extensions:"x-nullable"`
	FolderID    *uuid.UUID `json:"folder_id" extensions:"x-nullable"`
	Title       string     `json:"title"`
	Category    string     `json:"category"`
	Tags        []string   `json:"tags"`
	SourceLabel string     `json:"source_label"`
	Note        string     `json:"note"`
	Favorite    bool       `json:"favorite"`
	State       string     `json:"catalog_state"`
	TrashedAt   *time.Time `json:"trashed_at" extensions:"x-nullable"`
	Position    int64      `json:"position"`
	Revision    int64      `json:"revision"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Validate rejects unsupported editorial fields and enforces media/text exclusivity.
func (i LibraryItem) Validate() error {
	if i.ID == uuid.Nil || i.LibraryID == uuid.Nil || (i.AssetID == nil) == (i.PlainText == nil) ||
		i.AssetID != nil && *i.AssetID == uuid.Nil || i.FolderID != nil && *i.FolderID == uuid.Nil ||
		!libraryText(i.Title, 240, true) || !slices.Contains([]string{"character", "environment", "prop", "material", "other"}, i.Category) ||
		!libraryText(i.SourceLabel, 240, false) || !libraryText(i.Note, 8000, false) || len(i.Tags) > 32 ||
		i.Position < 0 || i.Position > math.MaxInt32 || i.Revision < 1 || i.Revision > math.MaxInt32 ||
		!slices.Contains([]string{"active", "trashed", "removed"}, i.State) || (i.State == "trashed") != (i.TrashedAt != nil) {
		return ErrInvalidLibrary
	}
	if i.PlainText != nil && (len(*i.PlainText) > 64<<10 || !utf8.ValidString(*i.PlainText) || strings.ContainsRune(*i.PlainText, 0)) {
		return ErrInvalidLibrary
	}
	seen := make(map[string]bool, len(i.Tags))
	for _, tag := range i.Tags {
		if !libraryText(tag, 64, true) || strings.TrimSpace(tag) != tag || seen[tag] {
			return ErrInvalidLibrary
		}
		seen[tag] = true
	}
	return nil
}

func libraryText(value string, maxRunes int, required bool) bool {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxRunes || required && strings.TrimSpace(value) == "" {
		return false
	}
	for _, r := range value {
		if r == 0 || unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}

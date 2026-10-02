package application

import (
	"errors"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

var (
	// ErrInvalidPackage rejects malformed or ambiguous archive contents.
	ErrInvalidPackage = errors.New("invalid media library package")
	// ErrPackageIncomplete reports a declared file that is not actually present.
	ErrPackageIncomplete = errors.New("media library package missing declared resource")
	// ErrPackageUnsupportedKind reports a source entity with no media-owned importer.
	ErrPackageUnsupportedKind = errors.New("unsupported media library package item kind")
)

const (
	// MaxPackageBytes bounds the actual compressed input/output archive.
	MaxPackageBytes int64 = 500 << 20
	// MaxPackageExpandedBytes bounds every actual decompressed entry together.
	MaxPackageExpandedBytes int64 = 2 << 30
	// MaxPackageEntries includes the manifest and declared directory entries.
	MaxPackageEntries       = 2500
	maxPackageManifestBytes = 16 << 20
)

// LibraryPackageFolder contains exported placement, never target ownership.
type LibraryPackageFolder struct {
	ID       uuid.UUID  `json:"id"`
	ParentID *uuid.UUID `json:"parent_id"`
	Name     string     `json:"name"`
	Style    string     `json:"style"`
	Theme    string     `json:"theme"`
	Position int64      `json:"position"`
}

// LibraryPackageItem is a declarative source record. Its ID is remapped by the
// owning import admission; no source execution/review identity can be carried.
type LibraryPackageItem struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Metadata  LibraryMetadata `json:"metadata"`
	State     string          `json:"catalog_state"`
	TrashedAt *time.Time      `json:"trashed_at"`
	Position  int64           `json:"position"`
	FilePath  *string         `json:"file_path"`
}

// LibraryPackageFile closes the contained binary identity independently of any
// supplied URL, old browser storage key or alleged portrait certificate.
type LibraryPackageFile struct {
	Path     string `json:"path"`
	FileName string `json:"file_name"`
	MIMEType string `json:"mime_type"`
	ByteSize int64  `json:"byte_size"`
	SHA256   string `json:"sha256"`
}

// LibraryPackageManifest is the versioned complete library export contract.
// Scope identities and rights declarations are deliberately not exportable.
type LibraryPackageManifest struct {
	App         string                 `json:"app"`
	Version     int                    `json:"version"`
	ExportedAt  time.Time              `json:"exported_at"`
	LibraryKind domain.LibraryKind     `json:"library_kind"`
	Folders     []LibraryPackageFolder `json:"folders"`
	Items       []LibraryPackageItem   `json:"items"`
	Files       []LibraryPackageFile   `json:"files"`
}

// PackageWarning records an untrusted source annotation without retaining URLs.
type PackageWarning struct {
	ItemID string `json:"item_id"`
	Code   string `json:"code"`
}

func (m LibraryPackageManifest) validate() error {
	if m.App != "lanverse-media-library" || m.Version != 1 || m.ExportedAt.IsZero() ||
		!slices.Contains([]domain.LibraryKind{domain.LibraryPersonal, domain.LibraryProject}, m.LibraryKind) ||
		m.Folders == nil || m.Items == nil || m.Files == nil || len(m.Items) > MaxPackageEntries || len(m.Folders) > MaxPackageEntries || len(m.Files) >= MaxPackageEntries {
		return ErrInvalidPackage
	}
	library := uuid.NewSHA1(uuid.Nil, []byte("media-package-validation"))
	folders := make([]domain.LibraryFolder, 0, len(m.Folders))
	for _, f := range m.Folders {
		folders = append(folders, domain.LibraryFolder{ID: f.ID, LibraryID: library, Kind: m.LibraryKind, ParentID: f.ParentID, Name: f.Name, Style: f.Style, Theme: f.Theme, Position: f.Position, Revision: 1})
	}
	if len(folders) != 0 && domain.ValidateLibraryFolderPlacement(folders[0], folders) != nil {
		return ErrInvalidPackage
	}
	folderIDs := make(map[uuid.UUID]bool, len(folders))
	for _, f := range folders {
		folderIDs[f.ID] = true
	}
	files := make(map[string]LibraryPackageFile, len(m.Files))
	for _, f := range m.Files {
		name, err := SafeUploadFileName(f.FileName)
		if !packagePath(f.Path, false) || name != f.FileName || err != nil || !copySHA(&f.SHA256) || f.ByteSize < 1 || f.ByteSize > MaxPackageExpandedBytes || files[f.Path].Path != "" {
			return ErrInvalidPackage
		}
		files[f.Path] = f
	}
	ids, used := make(map[string]bool, len(m.Items)), make(map[string]bool, len(m.Files))
	for _, item := range m.Items {
		if !utf8.ValidString(item.ID) || len(item.ID) < 1 || len(item.ID) > 128 || strings.TrimSpace(item.ID) != item.ID || strings.ContainsAny(item.ID, "\x00\n\r") || ids[item.ID] || item.Position < 0 || item.Position > math.MaxInt32 || item.Metadata.FolderID != nil && !folderIDs[*item.Metadata.FolderID] {
			return ErrInvalidPackage
		}
		ids[item.ID] = true
		meta := item.Metadata
		dummy := domain.LibraryItem{ID: uuid.NewSHA1(library, []byte(item.ID)), LibraryID: library, FolderID: meta.FolderID, PlainText: meta.PlainText, Title: meta.Title, Category: meta.Category, Tags: meta.Tags, SourceLabel: meta.SourceLabel, Note: meta.Note, Favorite: meta.Favorite, State: item.State, TrashedAt: item.TrashedAt, Position: item.Position, Revision: 1}
		if item.Kind == "text" {
			if item.FilePath != nil || meta.PlainText == nil {
				return ErrInvalidPackage
			}
		} else {
			if !slices.Contains([]string{"image", "video", "audio", "model", "document"}, item.Kind) {
				return ErrPackageUnsupportedKind
			}
			if item.FilePath == nil || meta.PlainText != nil {
				return ErrInvalidPackage
			}
			f, found := files[*item.FilePath]
			if !found {
				return ErrPackageIncomplete
			}
			if !packageFileKind(item.Kind, f) {
				return ErrInvalidPackage
			}
			used[f.Path] = true
			dummy.AssetID = &dummy.ID
		}
		if dummy.Validate() != nil {
			return ErrInvalidPackage
		}
	}
	if len(used) != len(files) {
		return ErrInvalidPackage
	}
	return nil
}

func packageFileKind(kind string, f LibraryPackageFile) bool {
	var allowed []string
	var maxBytes int64
	switch kind {
	case "image":
		allowed, maxBytes = []string{"image/jpeg", "image/png", "image/webp", "image/gif"}, MaxUploadImageBytes
	case "video":
		allowed, maxBytes = []string{"video/mp4", "video/quicktime", "video/webm"}, MaxUploadVideoBytes
	case "audio":
		allowed, maxBytes = []string{"audio/mpeg", "audio/wave", "audio/mp4"}, MaxUploadAudioBytes
	case "model":
		allowed, maxBytes = []string{"model/gltf-binary", "model/gltf+json"}, MaxUploadModelBytes
	case "document":
		allowed, maxBytes = []string{domain.MIMEText, domain.MIMEDOCX}, MaxUploadDocumentBytes
	default:
		return false
	}
	return slices.Contains(allowed, f.MIMEType) && f.ByteSize <= maxBytes
}

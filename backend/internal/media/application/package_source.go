package application

import (
	"context"
	"encoding/json"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// These fields are the actual BeefTV 1ae25027 assets.json version-one contract
// from web/src/pages/assets/asset-transfer.ts and stores/use-asset-store.ts.
// URLs/certificates/source identities are accepted only as ignored annotations.
type sourceAssetPackage struct {
	App        string               `json:"app"`
	Version    int                  `json:"version"`
	ExportedAt time.Time            `json:"exportedAt"`
	Assets     []sourcePackageAsset `json:"assets"`
	Files      []sourcePackageFile  `json:"files"`
}

type sourcePackageAsset struct {
	ID                string          `json:"id"`
	Kind              string          `json:"kind"`
	Title             string          `json:"title"`
	CoverURL          string          `json:"coverUrl"`
	Tags              []string        `json:"tags"`
	FolderID          *string         `json:"folderId"`
	Category          *string         `json:"category"`
	Status            *string         `json:"status"`
	PrimaryVersionID  *string         `json:"primaryVersionId"`
	Source            *string         `json:"source"`
	Note              *string         `json:"note"`
	ArkAssetID        *string         `json:"arkAssetId"`
	PortraitCertified *bool           `json:"portraitCertified"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
	Metadata          json.RawMessage `json:"metadata"`
	Data              json.RawMessage `json:"data"`
}

type sourcePackageFile struct {
	StorageKey string `json:"storageKey"`
	Path       string `json:"path"`
	MIMEType   string `json:"mimeType"`
	ByteSize   int64  `json:"bytes"`
}

type sourcePackageAssetData struct {
	Content    *string  `json:"content"`
	StorageKey *string  `json:"storageKey"`
	DataURL    *string  `json:"dataUrl"`
	URL        *string  `json:"url"`
	Width      *float64 `json:"width"`
	Height     *float64 `json:"height"`
	ByteSize   *float64 `json:"bytes"`
	MIMEType   *string  `json:"mimeType"`
	DurationMS *float64 `json:"durationMs"`
	HasAudio   *bool    `json:"hasAudio"`
	FileName   *string  `json:"fileName"`
}

func (a *LibraryPackageArchive) readSource(ctx context.Context, data []byte) error {
	var source sourceAssetPackage
	if err := decodePackageJSON(data, &source); err != nil {
		return err
	}
	if source.App != "infinite-canvas" || source.Version != 1 || source.ExportedAt.IsZero() || source.Assets == nil || source.Files == nil || len(source.Assets) > MaxPackageEntries || len(source.Files) >= MaxPackageEntries {
		return ErrInvalidPackage
	}
	a.Manifest = LibraryPackageManifest{App: "lanverse-media-library", Version: 1, ExportedAt: source.ExportedAt, LibraryKind: domain.LibraryPersonal, Folders: []LibraryPackageFolder{}, Items: []LibraryPackageItem{}, Files: []LibraryPackageFile{}}
	byKey := make(map[string]sourcePackageFile, len(source.Files))
	for _, file := range source.Files {
		if strings.TrimSpace(file.StorageKey) != file.StorageKey || file.StorageKey == "" || len(file.StorageKey) > 512 || byKey[file.StorageKey].StorageKey != "" || !packagePath(file.Path, false) || file.ByteSize < 1 || file.ByteSize > MaxPackageExpandedBytes {
			return ErrInvalidPackage
		}
		entry, found := a.entries[file.Path]
		if !found {
			return ErrPackageIncomplete
		}
		sha, size, err := hashPackageEntry(ctx, entry, file.ByteSize)
		if err != nil || size != file.ByteSize {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return ErrInvalidPackage
		}
		byKey[file.StorageKey] = file
		a.Manifest.Files = append(a.Manifest.Files, LibraryPackageFile{Path: file.Path, FileName: path.Base(file.Path), MIMEType: file.MIMEType, ByteSize: size, SHA256: sha})
	}
	for index, asset := range source.Assets {
		if !slices.Contains([]string{"text", "image", "video", "audio", "model"}, asset.Kind) {
			return ErrPackageUnsupportedKind
		}
		if asset.CreatedAt.IsZero() || asset.UpdatedAt.Before(asset.CreatedAt) || asset.Tags == nil || asset.Status != nil && !slices.Contains([]string{"draft", "review", "confirmed", "archived"}, *asset.Status) {
			return ErrInvalidPackage
		}
		var data sourcePackageAssetData
		if err := decodePackageJSON(asset.Data, &data); err != nil {
			return err
		}
		meta := LibraryMetadata{Title: asset.Title, Category: "material", Tags: asset.Tags}
		if asset.Kind == "text" {
			meta.Category = "other"
		}
		if asset.Category != nil {
			meta.Category = strings.ToLower(strings.TrimSpace(*asset.Category))
			// Fixed source lib/asset-category.ts has these four legacy aliases.
			switch meta.Category {
			case "wardrobe", "weapon", "accessory":
				meta.Category = "prop"
			case "style":
				meta.Category = "material"
			}
		}
		if asset.Source != nil {
			meta.SourceLabel = *asset.Source
		}
		if asset.Note != nil {
			meta.Note = *asset.Note
		}
		item := LibraryPackageItem{ID: asset.ID, Kind: asset.Kind, Metadata: meta, State: "active", Position: int64(index)}
		if asset.Kind == "text" {
			if data.Content == nil || data.StorageKey != nil || data.URL != nil || data.DataURL != nil {
				return ErrInvalidPackage
			}
			item.Metadata.PlainText = data.Content
		} else {
			if data.StorageKey == nil || *data.StorageKey == "" || data.Content != nil {
				return ErrPackageIncomplete
			}
			file, found := byKey[*data.StorageKey]
			if !found {
				return ErrPackageIncomplete
			}
			item.FilePath = &file.Path
			if data.MIMEType == nil || *data.MIMEType != "application/octet-stream" && *data.MIMEType != file.MIMEType {
				return ErrInvalidPackage
			}
		}
		a.Manifest.Items = append(a.Manifest.Items, item)
		if asset.FolderID != nil {
			a.Warnings = append(a.Warnings, PackageWarning{ItemID: asset.ID, Code: "source_folder_not_exported"})
		}
		if asset.PrimaryVersionID != nil {
			a.Warnings = append(a.Warnings, PackageWarning{ItemID: asset.ID, Code: "source_version_not_exported"})
		}
		if asset.PortraitCertified != nil && *asset.PortraitCertified || asset.ArkAssetID != nil {
			a.Warnings = append(a.Warnings, PackageWarning{ItemID: asset.ID, Code: "source_certificate_is_not_consent"})
		}
		if len(asset.Metadata) > 0 {
			a.Warnings = append(a.Warnings, PackageWarning{ItemID: asset.ID, Code: "source_metadata_is_not_media_fact"})
		}
	}
	return nil
}

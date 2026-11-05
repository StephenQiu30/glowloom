package postgres

import (
	"bytes"
	"encoding/json"
	"sort"

	"gorm.io/gorm"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// packageExportEntries assembles the complete visible catalog from ORM-owned
// rows while retaining the exact source asset row for subsequent byte reads.
func packageExportEntries(tx *gorm.DB, actor identityapp.Principal, scope domain.LibraryScope, library uuid.UUID) ([]libraryEntryRow, error) {
	var items []libraryItemRow
	if err := tx.Where("library_id = ? AND purged_at IS NULL", library).Order("id").Limit(application.MaxPackageEntries + 1).Find(&items).Error; err != nil {
		return nil, err
	}
	if len(items) > application.MaxPackageEntries {
		return nil, application.ErrInvalidPackage
	}
	itemsByAsset := make(map[uuid.UUID]libraryItemRow, len(items))
	entries := make([]libraryEntryRow, 0, len(items))
	assetIDs := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		if item.AssetID != nil {
			itemsByAsset[*item.AssetID] = item
			assetIDs = append(assetIDs, *item.AssetID)
			continue
		}
		entries = append(entries, packageLibraryEntry(item, nil))
	}
	query := tx.Model(&assetRow{})
	if scope.Kind == domain.LibraryPersonal {
		query = query.Where("project_id IS NULL AND personal_org_id = ? AND personal_actor_id = ?", actor.OrgID, actor.ID)
	} else {
		query = query.Where("project_id = ? AND personal_org_id IS NULL AND personal_actor_id IS NULL", *scope.ProjectID)
	}
	query = query.Where("((is_delete = ? AND status = ? AND moderation_status = ?) OR id IN ?)", false, domain.StatusReady, domain.ModerationPassed, assetIDs)
	var assets []assetRow
	if err := query.Order("id").Limit(application.MaxPackageEntries + 1).Find(&assets).Error; err != nil {
		return nil, err
	}
	if len(assets) > application.MaxPackageEntries {
		return nil, application.ErrInvalidPackage
	}
	for i := range assets {
		asset := assets[i]
		if item, exists := itemsByAsset[asset.ID]; exists {
			entries = append(entries, packageLibraryEntry(item, &asset))
			continue
		}
		assetID := asset.ID
		title := asset.FileName
		if title == "" {
			title = asset.Kind
		}
		if runes := []rune(title); len(runes) > 240 {
			title = string(runes[:240])
		}
		entries = append(entries, libraryEntryRow{ID: asset.ID, AssetID: &assetID, Kind: asset.Kind, Title: title, Category: domain.LibraryItemCategoryMaterial, Tags: []byte("[]"), CatalogState: domain.LibraryItemStateActive, Revision: domain.LibraryItemInitialRevision, CreateTime: asset.CreateTime, UpdateTime: asset.UpdateTime, Asset: &asset})
	}
	sort.Slice(entries, func(i, j int) bool { return bytes.Compare(entries[i].ID[:], entries[j].ID[:]) < 0 })
	if len(entries) > application.MaxPackageEntries {
		return nil, application.ErrInvalidPackage
	}
	return entries, nil
}

func packageLibraryEntry(item libraryItemRow, asset *assetRow) libraryEntryRow {
	kind := "text"
	if asset != nil {
		kind = asset.Kind
	}
	return libraryEntryRow{ID: item.ID, AssetID: item.AssetID, FolderID: item.FolderID, PlainText: item.PlainText, Kind: kind, Title: item.Title, Category: item.Category, Tags: item.Tags, SourceLabel: item.SourceLabel, Note: item.Note, Favorite: item.Favorite, CatalogState: item.CatalogState, TrashedAt: item.TrashedAt, Position: item.Position, Revision: item.Revision, CreateTime: item.CreateTime, UpdateTime: item.UpdateTime, Asset: asset}
}

func decodePackageExportAsset(tx *gorm.DB, row *assetRow, state domain.LibraryItemState) (domain.MediaAsset, error) {
	if row == nil {
		return domain.MediaAsset{}, application.ErrUnavailable
	}
	loaded := *row
	if err := loaded.loadLocation(tx); err != nil {
		return domain.MediaAsset{}, err
	}
	a := loaded.domain()
	if a.Validate() != nil || a.Status != domain.StatusReady || a.ModerationStatus != domain.ModerationPassed || a.IsDelete && state != domain.LibraryItemStateTrashed {
		return domain.MediaAsset{}, application.ErrPackageIncomplete
	}
	return a, nil
}

func packageExportDetail(tx *gorm.DB, r libraryEntryRow, library uuid.UUID) (application.LibraryItemDetail, error) {
	if r.AssetID == nil {
		return r.detail(library)
	}
	a, err := decodePackageExportAsset(tx, r.Asset, r.CatalogState)
	if err != nil || a.ID != *r.AssetID || r.ID != a.ID || string(a.Kind) != r.Kind {
		return application.LibraryItemDetail{}, application.ErrPackageIncomplete
	}
	var tags []string
	if json.Unmarshal(r.Tags, &tags) != nil {
		return application.LibraryItemDetail{}, application.ErrUnavailable
	}
	item := domain.LibraryItem{ID: r.ID, LibraryID: library, AssetID: r.AssetID, FolderID: r.FolderID, Title: r.Title, Category: r.Category, Tags: tags, SourceLabel: r.SourceLabel, Note: r.Note, Favorite: r.Favorite, State: r.CatalogState, TrashedAt: r.TrashedAt, Position: r.Position, Revision: r.Revision, CreatedAt: r.CreateTime, UpdatedAt: r.UpdateTime}
	if item.Validate() != nil {
		return application.LibraryItemDetail{}, application.ErrPackageIncomplete
	}
	return application.LibraryItemDetail{LibraryItemSummary: application.LibraryItemSummary{ID: r.ID, AssetID: r.AssetID, FolderID: r.FolderID, Kind: r.Kind, Title: r.Title, Category: r.Category, Tags: tags, SourceLabel: r.SourceLabel, Note: r.Note, Favorite: r.Favorite, State: r.CatalogState, TrashedAt: r.TrashedAt, Position: r.Position, Revision: r.Revision, CreatedAt: r.CreateTime, UpdatedAt: r.UpdateTime}}, nil
}

func packageExportFileName(a domain.MediaAsset) (string, error) {
	if a.FileName != "" {
		name, err := application.SafeUploadFileName(a.FileName)
		if err != nil || name != a.FileName {
			return "", application.ErrPackageIncomplete
		}
		return name, nil
	}
	extensions := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif", "video/mp4": ".mp4", "video/quicktime": ".mov", "audio/mpeg": ".mp3", "audio/wave": ".wav", "audio/mp4": ".m4a"}
	ext, ok := extensions[a.MimeType]
	if !ok {
		return "", application.ErrPackageIncomplete
	}
	return string(a.Kind) + ext, nil
}

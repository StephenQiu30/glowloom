package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// The binary half retains existing asset identities without creating metadata
// on reads. The text half has no fake media asset, object key or project ID.
func libraryEntries(tx *gorm.DB, actor identityapp.Principal, scope domain.LibraryScope, id uuid.UUID) *gorm.DB {
	assetEntries := tx.Table("media.media_asset AS a").
		Select("COALESCE(i.id, a.id) AS id, a.id AS asset_id, i.folder_id AS folder_id, NULL::text AS plain_text, a.kind AS kind, LEFT(COALESCE(i.title, NULLIF(a.file_name, ''), a.kind), 240) AS title, COALESCE(i.category, ?) AS category, COALESCE(i.tags, '[]'::jsonb) AS tags, COALESCE(i.source_label, '') AS source_label, COALESCE(i.note, '') AS note, COALESCE(i.favorite, false) AS favorite, COALESCE(i.catalog_state, ?) AS catalog_state, i.trashed_at AS trashed_at, COALESCE(i.position, 0) AS position, COALESCE(i.revision, ?) AS revision, COALESCE(i.create_time, a.create_time) AS create_time, COALESCE(i.update_time, a.update_time) AS update_time, a.mime_type AS mime_type, to_jsonb(a) AS asset_facts", domain.LibraryItemCategoryMaterial, domain.LibraryItemStateActive, domain.LibraryItemInitialRevision).
		Joins("LEFT JOIN media.library_item AS i ON i.library_id = ? AND i.asset_id = a.id", id).
		Where("NOT a.is_delete AND a.status = ? AND a.moderation_status = ?", domain.StatusReady, domain.ModerationPassed).
		Where("i.purged_at IS NULL")
	if scope.Kind == domain.LibraryPersonal {
		assetEntries = assetEntries.Where("a.project_id IS NULL AND a.personal_org_id = ? AND a.personal_actor_id = ?", actor.OrgID, actor.ID)
	} else {
		assetEntries = assetEntries.Where("a.project_id = ? AND a.personal_org_id IS NULL AND a.personal_actor_id IS NULL", scope.ProjectID)
	}
	activePurgeStatuses := []domain.PurgeItemStatus{domain.PurgeItemStatusQueued, domain.PurgeItemStatusRunning, domain.PurgeItemStatusNeedsReconciliation, domain.PurgeItemStatusSucceeded}
	assetPurges := tx.Model(&purgeItemRow{}).Select("item_id").Where("item_id = a.id AND status IN ?", activePurgeStatuses)
	assetEntries = assetEntries.Where("NOT EXISTS (?)", assetPurges)

	textEntries := tx.Table("media.library_item AS i").
		Select("i.id AS id, NULL::uuid AS asset_id, i.folder_id AS folder_id, i.plain_text AS plain_text, ? AS kind, i.title AS title, i.category AS category, i.tags AS tags, i.source_label AS source_label, i.note AS note, i.favorite AS favorite, i.catalog_state AS catalog_state, i.trashed_at AS trashed_at, i.position AS position, i.revision AS revision, i.create_time AS create_time, i.update_time AS update_time, ''::text AS mime_type, NULL::jsonb AS asset_facts", domain.LibraryItemKindText).
		Where("i.library_id = ? AND i.asset_id IS NULL AND i.purged_at IS NULL", id)
	textPurges := tx.Model(&purgeItemRow{}).Select("item_id").Where("item_id = i.id AND status IN ?", activePurgeStatuses)
	textEntries = textEntries.Where("NOT EXISTS (?)", textPurges)
	return tx.Table("(?) AS entries", gorm.Expr("(?) UNION ALL (?)", assetEntries, textEntries))
}

type libraryEntryRow struct {
	ID           uuid.UUID
	AssetID      *uuid.UUID
	FolderID     *uuid.UUID
	PlainText    *string
	Kind         string
	Title        string
	Category     string
	Tags         []byte
	SourceLabel  string
	Note         string
	Favorite     bool
	CatalogState domain.LibraryItemState
	TrashedAt    *time.Time
	Position     int64
	Revision     int64
	CreateTime   time.Time
	UpdateTime   time.Time
	AssetFacts   []byte
	Asset        *assetRow `gorm:"-"`
}

func decodeLibraryAsset(raw []byte) (domain.MediaAsset, error) {
	var wire struct {
		assetRow
		ModerationDetail json.RawMessage `json:"moderation_detail"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&wire) != nil || decoder.Decode(new(any)) != io.EOF {
		return domain.MediaAsset{}, application.ErrUnavailable
	}
	wire.assetRow.ModerationDetail = wire.ModerationDetail
	a := wire.domain()
	// This decoder feeds only safe catalog metadata. Physical facts are loaded
	// separately under the owning transaction before any object access.
	metadata := a
	metadata.ObjectLocation = nil
	if a.ObjectLocation != nil && a.ObjectLocation.ID == uuid.Nil {
		return domain.MediaAsset{}, application.ErrUnavailable
	}
	if metadata.Validate() != nil || a.IsDelete || a.Status != domain.StatusReady || a.ModerationStatus != domain.ModerationPassed {
		return domain.MediaAsset{}, application.ErrUnavailable
	}
	return a, nil
}

func (r libraryEntryRow) detail(library uuid.UUID) (application.LibraryItemDetail, error) {
	var tags []string
	if err := json.Unmarshal(r.Tags, &tags); err != nil {
		return application.LibraryItemDetail{}, application.ErrUnavailable
	}
	item := domain.LibraryItem{ID: r.ID, LibraryID: library, AssetID: r.AssetID, PlainText: r.PlainText, FolderID: r.FolderID, Title: r.Title, Category: r.Category, Tags: tags, SourceLabel: r.SourceLabel, Note: r.Note, Favorite: r.Favorite, State: r.CatalogState, TrashedAt: r.TrashedAt, Position: r.Position, Revision: r.Revision, CreatedAt: r.CreateTime, UpdatedAt: r.UpdateTime}
	if item.Validate() != nil {
		return application.LibraryItemDetail{}, application.ErrUnavailable
	}
	result := application.LibraryItemDetail{LibraryItemSummary: application.LibraryItemSummary{ID: r.ID, AssetID: r.AssetID, FolderID: r.FolderID, Kind: r.Kind, Title: r.Title, Category: r.Category, Tags: tags, SourceLabel: r.SourceLabel, Note: r.Note, Favorite: r.Favorite, State: r.CatalogState, TrashedAt: r.TrashedAt, Position: r.Position, Revision: r.Revision, CreatedAt: r.CreateTime, UpdatedAt: r.UpdateTime}, PlainText: r.PlainText}
	if r.AssetID != nil {
		a, err := decodeLibraryAsset(r.AssetFacts)
		if err != nil || a.ID != *r.AssetID || r.ID != a.ID || string(a.Kind) != r.Kind {
			return application.LibraryItemDetail{}, application.ErrUnavailable
		}
		result.Media = &application.LibraryAssetSummary{ID: a.ID, Kind: string(a.Kind), FileName: a.FileName, MIMEType: a.MimeType, ByteSize: a.ByteSize, Width: a.Width, Height: a.Height, DurationMS: a.DurationMS, Revision: a.Revision}
	}
	return result, nil
}

func libraryFilters(q application.LibraryQuery, now time.Time) (string, []any) {
	conditions := []string{`catalog_state=?`}
	args := []any{q.State}
	if q.Kind != "" {
		conditions, args = append(conditions, `kind=?`), append(args, q.Kind)
	}
	if q.Category != "" {
		conditions, args = append(conditions, `category=?`), append(args, q.Category)
	}
	if q.FolderID != nil {
		conditions, args = append(conditions, `folder_id=?`), append(args, *q.FolderID)
	} else if q.RootOnly {
		conditions = append(conditions, `folder_id IS NULL`)
	}
	if q.FavoriteOnly {
		conditions = append(conditions, `favorite`)
	}
	if q.RecentOnly {
		conditions, args = append(conditions, `update_time>=?`), append(args, now.Add(-30*24*time.Hour))
	}
	if q.Search != "" {
		conditions = append(conditions, `(title ILIKE ? OR source_label ILIKE ? OR note ILIKE ? OR category ILIKE ? OR tags::text ILIKE ? OR plain_text ILIKE ? OR mime_type ILIKE ?)`)
		pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q.Search) + "%"
		for range 7 {
			args = append(args, pattern)
		}
	}
	pickerConditions, pickerArgs := libraryPickerFilters(q)
	conditions = append(conditions, pickerConditions...)
	args = append(args, pickerArgs...)
	return strings.Join(conditions, " AND "), args
}

// ListLibrary applies all filtering and the selected order before pagination.
// Facets describe the full authorized scope/state, independently of page filters.
func (s *LibraryStore) ListLibrary(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, q application.LibraryQuery) (application.LibraryPage, error) {
	if q.Validate(scope) != nil {
		return application.LibraryPage{}, domain.ErrInvalidLibrary
	}
	var result application.LibraryPage
	err := s.read(ctx, actor, scope, func(tx *gorm.DB, row libraryRow) error {
		result = application.LibraryPage{CurrentActorID: actor.ID, CurrentOrgID: actor.OrgID, Scope: scope, Page: q.Page, PageSize: q.PageSize, Items: []application.LibraryItemSummary{}, CategoryCounts: make(map[string]int64), FolderCounts: make(map[string]int64)}
		result.LibraryID, result.Revision = row.ID, row.Revision
		folders, err := libraryFolders(tx, row.ID, false)
		if err != nil {
			return err
		}
		slices.SortFunc(folders, func(a, b domain.LibraryFolder) int {
			if a.Position != b.Position {
				if a.Position < b.Position {
					return -1
				}
				return 1
			}
			return strings.Compare(a.ID.String(), b.ID.String())
		})
		result.Folders = folders
		if q.FolderID != nil && !slices.ContainsFunc(folders, func(f domain.LibraryFolder) bool { return f.ID == *q.FolderID }) {
			return application.ErrNotFound
		}
		filter, arguments := libraryFilters(q, s.clock())
		if err := libraryEntries(tx, actor, scope, row.ID).Where(filter, arguments...).Count(&result.Total).Error; err != nil {
			return fmt.Errorf("count media library page: %w", err)
		}
		order := "update_time DESC, id ASC"
		if q.Order == "updated_asc" {
			order = "update_time ASC, id ASC"
		}
		if q.Order == "name_asc" {
			order = `lower(title) COLLATE "C" ASC, id ASC`
		}
		var entries []libraryEntryRow
		if err := libraryEntries(tx, actor, scope, row.ID).Where(filter, arguments...).Order(order).Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&entries).Error; err != nil {
			return fmt.Errorf("read media library page: %w", err)
		}
		for _, entry := range entries {
			detail, err := entry.detail(row.ID)
			if err != nil {
				return err
			}
			result.Items = append(result.Items, detail.LibraryItemSummary)
		}
		var categories []struct {
			Category string
			Count    int64
		}
		if err := libraryEntries(tx, actor, scope, row.ID).Where("catalog_state = ?", q.State).Select("category, COUNT(*) AS count").Group("category").Scan(&categories).Error; err != nil {
			return err
		}
		for _, category := range categories {
			result.CategoryCounts[category.Category] = category.Count
		}
		var counts []struct {
			FolderID *uuid.UUID
			Count    int64
		}
		if err := libraryEntries(tx, actor, scope, row.ID).Where("catalog_state = ?", q.State).Select("folder_id, COUNT(*) AS count").Group("folder_id").Scan(&counts).Error; err != nil {
			return err
		}
		for _, folder := range counts {
			key := "root"
			if folder.FolderID != nil {
				key = folder.FolderID.String()
			}
			result.FolderCounts[key] = folder.Count
		}
		return nil
	})
	return result, err
}

// LibraryDetail returns one current scoped entry; file preview is separately authorized.
func (s *LibraryStore) LibraryDetail(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, id uuid.UUID) (application.LibraryItemDetail, error) {
	if id == uuid.Nil {
		return application.LibraryItemDetail{}, application.ErrNotFound
	}
	var result application.LibraryItemDetail
	err := s.read(ctx, actor, scope, func(tx *gorm.DB, row libraryRow) error {
		result = application.LibraryItemDetail{}
		var entry libraryEntryRow
		read := libraryEntries(tx, actor, scope, row.ID).Where("id = ? AND catalog_state IN ?", id, []domain.LibraryItemState{domain.LibraryItemStateActive, domain.LibraryItemStateTrashed}).Take(&entry)
		if read.Error != nil {
			if errors.Is(read.Error, gorm.ErrRecordNotFound) {
				return application.ErrNotFound
			}
			return fmt.Errorf("read media library detail: %w", read.Error)
		}
		var err error
		result, err = entry.detail(row.ID)
		return err
	})
	return result, err
}

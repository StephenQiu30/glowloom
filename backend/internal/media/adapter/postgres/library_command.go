package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func libraryCommandHash(c application.LibraryCommand) (string, error) {
	data, err := json.Marshal(c)
	if err != nil || len(data) > 512<<10 {
		return "", domain.ErrInvalidLibrary
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func libraryCommandLock(tx *gorm.DB, actor, key uuid.UUID) error {
	return lockStorageIdempotencyKey(tx, storageIdempotencyLibrary, actor, key)
}

func libraryReplay(tx *gorm.DB, actor identityapp.Principal, c application.LibraryCommand, hash string) (*application.LibraryReceipt, error) {
	var row libraryCommandRecord
	read := tx.Where(map[string]any{"actor_id": actor.ID, "idem_key": c.Key}).Take(&row)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if read.Error != nil {
		return nil, fmt.Errorf("read permanent media library command: %w", read.Error)
	}
	id, _ := c.Scope.Identity(actor.OrgID, actor.ID)
	if row.OrgID != actor.OrgID || row.LibraryID != id || row.Action != string(c.Action) || row.RequestSHA256 != hash {
		return nil, application.ErrLibraryKeyConflict
	}
	var receipt application.LibraryReceipt
	decoder := json.NewDecoder(bytes.NewReader(row.ResponseBody))
	decoder.DisallowUnknownFields()
	if len(row.ResponseBody) > 1<<20 || decoder.Decode(&receipt) != nil || decoder.Decode(new(any)) != io.EOF || receipt.LibraryID != id || receipt.CurrentActorID != actor.ID || receipt.CurrentOrgID != actor.OrgID || receipt.Revision < 0 || receipt.Scope.Kind != c.Scope.Kind || !sameScopeProject(receipt.Scope.ProjectID, c.Scope.ProjectID) {
		return nil, application.ErrUnavailable
	}
	return &receipt, nil
}

func sameScopeProject(a, b *uuid.UUID) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// ApplyLibraryCommand checks current authorization before permanent replay. New
// acceptance takes project then library locks; a read replay uses a separate
// transaction so two producers never upgrade competing project share locks.
func (s *LibraryStore) ApplyLibraryCommand(ctx context.Context, actor identityapp.Principal, c application.LibraryCommand) (application.LibraryReceipt, error) {
	if s == nil || s.db == nil {
		return application.LibraryReceipt{}, application.ErrUnavailable
	}
	if err := c.Validate(); err != nil {
		return application.LibraryReceipt{}, err
	}
	hash, err := libraryCommandHash(c)
	if err != nil {
		return application.LibraryReceipt{}, err
	}
	var cached *application.LibraryReceipt
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := libraryCommandLock(tx, actor.ID, c.Key); err != nil {
			return err
		}
		if _, err := s.authorize(ctx, tx, actor, c.Scope, false); err != nil {
			return err
		}
		var err error
		cached, err = libraryReplay(tx, actor, c, hash)
		return err
	})
	if err != nil {
		return application.LibraryReceipt{}, err
	}
	if cached != nil {
		return *cached, nil
	}
	var result application.LibraryReceipt
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := libraryCommandLock(tx, actor.ID, c.Key); err != nil {
			return err
		}
		facts, err := s.authorize(ctx, tx, actor, c.Scope, true)
		if err != nil {
			return err
		}
		replay, err := libraryReplay(tx, actor, c, hash)
		if err != nil {
			return err
		}
		if replay != nil {
			result = *replay
			return nil
		}
		id, _ := c.Scope.Identity(actor.OrgID, actor.ID)
		var personal *uuid.UUID
		if c.Scope.Kind == domain.LibraryPersonal {
			personal = &actor.ID
		}
		libraryIdentity := libraryIdentityRecord{ID: id, Kind: string(c.Scope.Kind), OrgID: actor.OrgID, ProjectID: c.Scope.ProjectID, PersonalActorID: personal}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&libraryIdentity).Error; err != nil {
			return fmt.Errorf("create media library: %w", err)
		}
		library, err := readLibrary(tx, actor, c.Scope, true)
		if err != nil {
			return err
		}
		if library.Revision != c.ExpectedRevision {
			return application.ErrLibraryConflict
		}
		folders, err := libraryFolders(tx, id, true)
		if err != nil {
			return err
		}
		result = application.LibraryReceipt{CurrentActorID: actor.ID, CurrentOrgID: actor.OrgID, LibraryID: id, Scope: c.Scope, Revision: library.Revision + 1, Items: []application.LibraryItemChange{}}
		now := s.clock().UTC().Truncate(time.Microsecond)
		if now.IsZero() {
			return application.ErrUnavailable
		}
		switch c.Action {
		case application.LibraryActionCreateFolder, application.LibraryActionUpdateFolder, application.LibraryActionDeleteFolder:
			if err := applyLibraryFolder(tx, actor, c, id, folders, now, &result); err != nil {
				return err
			}
		default:
			if err := applyLibraryItems(tx, actor, c, id, folders, now, &result); err != nil {
				return err
			}
		}
		if err := updateOne(tx, "media.library", map[string]any{"id": id, "revision": library.Revision}, map[string]any{"revision": gorm.Expr("revision + ?", 1), "update_time": now}, application.ErrLibraryConflict); err != nil {
			return err
		}
		if c.Scope.Kind == domain.LibraryProject {
			revision, err := s.project(tx).TouchContent(ctx, actor, facts.ProjectID, facts.Revision)
			if err != nil {
				return err
			}
			if revision != facts.Revision+1 {
				return application.ErrUnavailable
			}
			result.ProjectRevision = &revision
		}
		body, err := json.Marshal(result)
		if err != nil || len(body) > 1<<20 {
			return application.ErrUnavailable
		}
		commandID := uuid.NewSHA1(c.Key, []byte("media-library-command/"+actor.ID.String()))
		command := libraryCommandRecord{ID: commandID, OrgID: actor.OrgID, ActorID: actor.ID, LibraryID: id, IdemKey: c.Key, Action: string(c.Action), RequestSHA256: hash, ResponseBody: body}
		if err := requireOne(tx.Create(&command), application.ErrUnavailable); err != nil {
			return err
		}
		// Existing audit writer is INSERT-only. Content, notes and file names never
		// enter its security payload, and its failure rolls back the command.
		var project *uuid.UUID
		if c.Scope.Kind == domain.LibraryProject {
			project = c.Scope.ProjectID
		}
		safe, _ := json.Marshal(struct {
			LibraryID uuid.UUID `json:"library_id"`
			Revision  int64     `json:"revision"`
			Count     int       `json:"count"`
		}{id, result.Revision, len(result.Items)})
		audit := libraryAuditRecord{ID: commandID, OrgID: actor.OrgID, ProjectID: project, ActorID: actor.ID, ActorKind: "user", Action: "media.library." + string(c.Action), ObjectType: "media.library", ObjectID: id.String(), After: jsonDocument(safe), CreateTime: now, UpdateTime: now}
		return requireOne(tx.Create(&audit), application.ErrUnavailable)
	})
	return result, err
}

func libraryFolderExists(folders []domain.LibraryFolder, id *uuid.UUID) bool {
	return id == nil || slices.ContainsFunc(folders, func(f domain.LibraryFolder) bool { return f.ID == *id })
}

func applyLibraryFolder(tx *gorm.DB, actor identityapp.Principal, c application.LibraryCommand, library uuid.UUID, folders []domain.LibraryFolder, now time.Time, result *application.LibraryReceipt) error {
	var folder domain.LibraryFolder
	if c.Action == application.LibraryActionCreateFolder {
		if len(folders) >= 4096 {
			return domain.ErrInvalidLibrary
		}
		folder = domain.LibraryFolder{ID: uuid.NewSHA1(c.Key, []byte("media-library-folder/"+actor.ID.String()+"/"+library.String())), LibraryID: library, Kind: c.Scope.Kind, Revision: domain.LibraryFolderInitialRevision, CreatedAt: now, UpdatedAt: now}
		for _, existing := range folders {
			if existing.Position >= folder.Position {
				folder.Position = existing.Position + 1
			}
		}
	} else {
		found := false
		for _, existing := range folders {
			if existing.ID == *c.FolderID {
				folder, found = existing, true
				break
			}
		}
		if !found {
			return application.ErrNotFound
		}
		if folder.Revision != c.ExpectedFolderRevision || folder.Revision >= math.MaxInt32 {
			return application.ErrLibraryConflict
		}
	}
	if c.Action == application.LibraryActionDeleteFolder {
		if c.Scope.Kind == domain.LibraryProject {
			if slices.ContainsFunc(folders, func(f domain.LibraryFolder) bool { return f.ParentID != nil && *f.ParentID == folder.ID }) {
				return application.ErrLibraryFolderNotEmpty
			}
			var count int64
			if err := tx.Model(&libraryItemRow{}).Where(map[string]any{"library_id": library, "folder_id": folder.ID}).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return application.ErrLibraryFolderNotEmpty
			}
		} else {
			if err := tx.Model(&libraryItemRow{}).Where(map[string]any{"library_id": library, "folder_id": folder.ID}).Updates(map[string]any{"folder_id": nil, "revision": gorm.Expr("revision + ?", 1), "update_time": now}).Error; err != nil {
				return err
			}
		}
		return requireOne(tx.Where(map[string]any{"id": folder.ID, "library_id": library, "revision": folder.Revision}).Delete(&libraryFolderRow{}), application.ErrLibraryConflict)
	}
	folder.ParentID, folder.Name, folder.Style, folder.Theme = c.Folder.ParentID, c.Folder.Name, c.Folder.Style, c.Folder.Theme
	if err := domain.ValidateLibraryFolderPlacement(folder, folders); err != nil {
		return err
	}
	if c.Action == application.LibraryActionCreateFolder {
		row := libraryFolderRow{ID: folder.ID, LibraryID: library, LibraryKind: string(folder.Kind), ParentID: folder.ParentID, Name: folder.Name, NameKey: folder.NameKey(), Position: folder.Position, Style: folder.Style, Theme: folder.Theme, Revision: folder.Revision, CreateTime: now, UpdateTime: now}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	} else {
		if err := updateOne(tx, "media.library_folder", map[string]any{"id": folder.ID, "library_id": library, "revision": folder.Revision}, map[string]any{"parent_id": folder.ParentID, "name": folder.Name, "name_key": folder.NameKey(), "style": folder.Style, "theme": folder.Theme, "revision": gorm.Expr("revision + ?", 1), "update_time": now}, application.ErrLibraryConflict); err != nil {
			return err
		}
		folder.Revision++
		folder.UpdatedAt = now
	}
	result.Folder = &folder
	return nil
}

type libraryItemRow struct {
	ID, LibraryID          uuid.UUID
	AssetID, FolderID      *uuid.UUID
	PlainText              *string
	Title, Category        string
	Tags                   []byte
	SourceLabel, Note      string
	Favorite               bool
	CatalogState           domain.LibraryItemState
	TrashedAt              *time.Time
	PurgedAt               *time.Time
	Position, Revision     int64
	CreateTime, UpdateTime time.Time
}

func (libraryItemRow) TableName() string { return "media.library_item" }

func (r libraryItemRow) item() (domain.LibraryItem, error) {
	var tags []string
	if json.Unmarshal(r.Tags, &tags) != nil {
		return domain.LibraryItem{}, application.ErrUnavailable
	}
	i := domain.LibraryItem{ID: r.ID, LibraryID: r.LibraryID, AssetID: r.AssetID, PlainText: r.PlainText, FolderID: r.FolderID, Title: r.Title, Category: r.Category, Tags: tags, SourceLabel: r.SourceLabel, Note: r.Note, Favorite: r.Favorite, State: r.CatalogState, TrashedAt: r.TrashedAt, Position: r.Position, Revision: r.Revision, CreatedAt: r.CreateTime, UpdatedAt: r.UpdateTime}
	if i.Validate() != nil {
		return domain.LibraryItem{}, application.ErrUnavailable
	}
	return i, nil
}

func lockLibraryItems(tx *gorm.DB, actor identityapp.Principal, scope domain.LibraryScope, library uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]domain.LibraryItem, error) {
	if err := requireNoPurgeReservation(tx, ids); err != nil {
		return nil, err
	}
	var rows []libraryItemRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("library_id = ? AND id IN ? AND purged_at IS NULL", library, ids).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make(map[uuid.UUID]domain.LibraryItem, len(ids))
	assets := make([]uuid.UUID, 0, len(ids))
	for _, row := range rows {
		i, err := row.item()
		if err != nil {
			return nil, err
		}
		if i.State == domain.LibraryItemStateRemoved {
			return nil, application.ErrNotFound
		}
		items[i.ID] = i
		if i.AssetID != nil {
			assets = append(assets, *i.AssetID)
		}
	}
	for _, id := range ids {
		if _, exists := items[id]; !exists {
			assets = append(assets, id)
		}
	}
	if len(assets) != 0 {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", assets)
		if scope.Kind == domain.LibraryPersonal {
			query = query.Where(map[string]any{"project_id": nil, "personal_org_id": actor.OrgID, "personal_actor_id": actor.ID})
		} else {
			query = query.Where(map[string]any{"project_id": scope.ProjectID, "personal_org_id": nil, "personal_actor_id": nil})
		}
		var binary []assetRow
		if err := query.Order("id").Find(&binary).Error; err != nil {
			return nil, err
		}
		found := make(map[uuid.UUID]bool, len(binary))
		for _, row := range binary {
			if err := row.loadLocation(tx); err != nil {
				return nil, err
			}
			a := row.domain()
			if a.Validate() != nil {
				return nil, application.ErrUnavailable
			}
			if a.IsDelete || a.Status != domain.StatusReady || a.ModerationStatus != domain.ModerationPassed {
				return nil, application.ErrNotFound
			}
			found[a.ID] = true
			if _, existing := items[a.ID]; !existing {
				title := a.FileName
				if title == "" {
					title = string(a.Kind)
				}
				runes := []rune(title)
				if len(runes) > 240 {
					title = string(runes[:240])
				}
				id := a.ID
				item := domain.LibraryItem{ID: id, LibraryID: library, AssetID: &id, Title: title, Category: domain.LibraryItemCategoryMaterial, Tags: []string{}, State: domain.LibraryItemStateActive, Revision: domain.LibraryItemInitialRevision, CreatedAt: a.CreateTime, UpdatedAt: a.UpdateTime}
				if err := persistLibraryItem(tx, item, 0); err != nil {
					return nil, err
				}
				items[id] = item
			}
		}
		for _, id := range assets {
			if !found[id] {
				return nil, application.ErrNotFound
			}
		}
	}
	for _, id := range ids {
		if _, found := items[id]; !found {
			return nil, application.ErrNotFound
		}
	}
	return items, nil
}

func persistLibraryItem(tx *gorm.DB, item domain.LibraryItem, previous int64) error {
	if item.Tags == nil {
		item.Tags = []string{}
	}
	if item.Validate() != nil {
		return domain.ErrInvalidLibrary
	}
	tags, err := json.Marshal(item.Tags)
	if err != nil {
		return err
	}
	values := map[string]any{"id": item.ID, "library_id": item.LibraryID, "asset_id": item.AssetID, "plain_text": item.PlainText, "folder_id": item.FolderID, "title": item.Title, "category": item.Category, "tags": jsonDocument(tags), "source_label": item.SourceLabel, "note": item.Note, "favorite": item.Favorite, "catalog_state": item.State, "trashed_at": item.TrashedAt, "position": item.Position, "revision": item.Revision, "create_time": item.CreatedAt, "update_time": item.UpdatedAt}
	if previous == 0 {
		result := tx.Table("media.library_item").Create(values)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return application.ErrLibraryConflict
		}
		return nil
	}
	delete(values, "id")
	delete(values, "library_id")
	delete(values, "asset_id")
	delete(values, "create_time")
	return requireOne(tx.Model(&libraryItemRow{}).Where(map[string]any{"id": item.ID, "library_id": item.LibraryID, "revision": previous}).Updates(values), application.ErrLibraryConflict)
}

func applyLibraryItems(tx *gorm.DB, actor identityapp.Principal, c application.LibraryCommand, library uuid.UUID, folders []domain.LibraryFolder, now time.Time, result *application.LibraryReceipt) error {
	var targets []application.LibraryItemRevision
	switch {
	case c.Action == application.LibraryActionCreateText:
		targets = []application.LibraryItemRevision{{ID: uuid.NewSHA1(c.Key, []byte("media-library-text/"+actor.ID.String()+"/"+library.String()))}}
	case c.ItemID != nil:
		targets = []application.LibraryItemRevision{{ID: *c.ItemID, Revision: c.ExpectedItemRevision}}
	default:
		targets = slices.Clone(c.Items)
	}
	items := make(map[uuid.UUID]domain.LibraryItem, len(targets))
	if c.Action == application.LibraryActionCreateText {
		items[targets[0].ID] = domain.LibraryItem{ID: targets[0].ID, LibraryID: library, PlainText: c.Metadata.PlainText, State: domain.LibraryItemStateActive, CreatedAt: now, UpdatedAt: now}
	} else {
		ids := make([]uuid.UUID, 0, len(targets))
		for _, target := range targets {
			ids = append(ids, target.ID)
		}
		var err error
		items, err = lockLibraryItems(tx, actor, c.Scope, library, ids)
		if err != nil {
			return err
		}
	}
	if !libraryFolderExists(folders, c.TargetFolderID) {
		return application.ErrNotFound
	}
	// Validate the entire batch before the first item write.
	for _, target := range targets {
		item := items[target.ID]
		if item.Revision != target.Revision || item.Revision >= math.MaxInt32 {
			return application.ErrLibraryConflict
		}
		switch c.Action {
		case application.LibraryActionCreateText, application.LibraryActionUpdateItem:
			m := c.Metadata
			if (item.AssetID == nil) != (m.PlainText != nil) || c.Scope.Kind == domain.LibraryProject && m.Favorite {
				return domain.ErrInvalidLibrary
			}
			item.PlainText, item.FolderID, item.Title, item.Category, item.Tags, item.SourceLabel, item.Note, item.Favorite = m.PlainText, m.FolderID, m.Title, m.Category, m.Tags, m.SourceLabel, m.Note, m.Favorite
		case application.LibraryActionMoveItems:
			if item.State != domain.LibraryItemStateActive {
				return domain.ErrMediaStateConflict
			}
			item.FolderID = c.TargetFolderID
		case application.LibraryActionRecycleItems:
			if item.State != domain.LibraryItemStateActive {
				return domain.ErrMediaStateConflict
			}
			item.State, item.TrashedAt = domain.LibraryItemStateTrashed, &now
		case application.LibraryActionRestoreItems:
			if item.State != domain.LibraryItemStateTrashed {
				return domain.ErrMediaStateConflict
			}
			item.State, item.TrashedAt = domain.LibraryItemStateActive, nil
		case application.LibraryActionRemoveItems:
			if item.State != domain.LibraryItemStateActive {
				return domain.ErrMediaStateConflict
			}
			item.State, item.FolderID = domain.LibraryItemStateRemoved, nil
		default:
			return domain.ErrInvalidLibrary
		}
		if !libraryFolderExists(folders, item.FolderID) {
			return application.ErrNotFound
		}
		item.Revision++
		item.UpdatedAt = now
		if item.Validate() != nil {
			return domain.ErrInvalidLibrary
		}
		items[target.ID] = item
	}
	for _, target := range targets {
		item := items[target.ID]
		if err := persistLibraryItem(tx, item, target.Revision); err != nil {
			return err
		}
		kind := "text"
		if item.AssetID != nil {
			var asset assetRow
			if err := tx.Select("kind").Where("id = ?", *item.AssetID).Take(&asset).Error; err != nil {
				return err
			}
			kind = asset.Kind
		}
		result.Items = append(result.Items, application.LibraryItemChange{ID: item.ID, AssetID: item.AssetID, FolderID: item.FolderID, Kind: kind, State: item.State, TrashedAt: item.TrashedAt, Revision: item.Revision})
	}
	return nil
}

var _ application.LibraryRepository = (*LibraryStore)(nil)

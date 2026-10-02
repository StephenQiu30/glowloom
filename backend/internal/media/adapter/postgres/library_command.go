package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
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
	return tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "media-library-command/"+actor.String()+"/"+key.String()).Error
}

func libraryReplay(tx *gorm.DB, actor identityapp.Principal, c application.LibraryCommand, hash string) (*application.LibraryReceipt, error) {
	var row struct {
		OrgID, LibraryID      uuid.UUID
		Action, RequestSHA256 string
		ResponseBody          []byte
	}
	read := tx.Raw(`SELECT org_id,library_id,action,request_sha256,response_body FROM media.library_command WHERE actor_id=? AND idem_key=?`, actor.ID, c.Key).Scan(&row)
	if read.Error != nil {
		return nil, fmt.Errorf("read permanent media library command: %w", read.Error)
	}
	if read.RowsAffected == 0 {
		return nil, nil
	}
	id, _ := c.Scope.Identity(actor.OrgID, actor.ID)
	if row.OrgID != actor.OrgID || row.LibraryID != id || row.Action != c.Action || row.RequestSHA256 != hash {
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
		if err := requireCurrentActor(tx, actor); err != nil {
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
		if err := requireCurrentActor(tx, actor); err != nil {
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
		if err := tx.Exec(`INSERT INTO media.library(id,kind,org_id,project_id,personal_actor_id) VALUES(?,?,?,?,?) ON CONFLICT(id) DO NOTHING`, id, c.Scope.Kind, actor.OrgID, c.Scope.ProjectID, personal).Error; err != nil {
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
		if strings.HasSuffix(c.Action, "folder") {
			if err := applyLibraryFolder(tx, actor, c, id, folders, now, &result); err != nil {
				return err
			}
		} else if err := applyLibraryItems(tx, actor, c, id, folders, now, &result); err != nil {
			return err
		}
		if err := libraryChanged(tx, `UPDATE media.library SET revision=revision+1,update_time=? WHERE id=? AND revision=?`, now, id, library.Revision); err != nil {
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
		if err := libraryChanged(tx, `INSERT INTO media.library_command(id,org_id,actor_id,library_id,idem_key,action,request_sha256,response_body) VALUES(?,?,?,?,?,?,?,?)`, commandID, actor.OrgID, actor.ID, id, c.Key, c.Action, hash, body); err != nil {
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
		return libraryChanged(tx, `INSERT INTO audit.audit_log(id,org_id,project_id,actor_id,actor_kind,action,object_type,object_id,after) VALUES(?,?,?,?,'user',?,'media.library',?,?::jsonb)`, commandID, actor.OrgID, project, actor.ID, "media.library."+c.Action, id.String(), string(safe))
	})
	return result, err
}

func libraryFolderExists(folders []domain.LibraryFolder, id *uuid.UUID) bool {
	return id == nil || slices.ContainsFunc(folders, func(f domain.LibraryFolder) bool { return f.ID == *id })
}

func applyLibraryFolder(tx *gorm.DB, actor identityapp.Principal, c application.LibraryCommand, library uuid.UUID, folders []domain.LibraryFolder, now time.Time, result *application.LibraryReceipt) error {
	var folder domain.LibraryFolder
	if c.Action == "create_folder" {
		if len(folders) >= 4096 {
			return domain.ErrInvalidLibrary
		}
		folder = domain.LibraryFolder{ID: uuid.NewSHA1(c.Key, []byte("media-library-folder/"+actor.ID.String()+"/"+library.String())), LibraryID: library, Kind: c.Scope.Kind, Revision: 1, CreatedAt: now, UpdatedAt: now}
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
		if folder.Revision != c.ExpectedFolderRevision || folder.Revision >= 2147483647 {
			return application.ErrLibraryConflict
		}
	}
	if c.Action == "delete_folder" {
		if c.Scope.Kind == domain.LibraryProject {
			if slices.ContainsFunc(folders, func(f domain.LibraryFolder) bool { return f.ParentID != nil && *f.ParentID == folder.ID }) {
				return application.ErrLibraryFolderNotEmpty
			}
			var count int64
			if err := tx.Raw(`SELECT count(*) FROM media.library_item WHERE library_id=? AND folder_id=?`, library, folder.ID).Scan(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return application.ErrLibraryFolderNotEmpty
			}
		} else {
			if err := tx.Exec(`UPDATE media.library_item SET folder_id=NULL,revision=revision+1,update_time=? WHERE library_id=? AND folder_id=?`, now, library, folder.ID).Error; err != nil {
				return err
			}
		}
		return libraryChanged(tx, `DELETE FROM media.library_folder WHERE id=? AND library_id=? AND revision=?`, folder.ID, library, folder.Revision)
	}
	folder.ParentID, folder.Name, folder.Style, folder.Theme = c.Folder.ParentID, c.Folder.Name, c.Folder.Style, c.Folder.Theme
	if err := domain.ValidateLibraryFolderPlacement(folder, folders); err != nil {
		return err
	}
	if c.Action == "create_folder" {
		if err := libraryChanged(tx, `INSERT INTO media.library_folder(id,library_id,library_kind,parent_id,name,name_key,position,style,theme,revision,create_time,update_time) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, folder.ID, library, folder.Kind, folder.ParentID, folder.Name, folder.NameKey(), folder.Position, folder.Style, folder.Theme, folder.Revision, now, now); err != nil {
			return err
		}
	} else {
		if err := libraryChanged(tx, `UPDATE media.library_folder SET parent_id=?,name=?,name_key=?,style=?,theme=?,revision=revision+1,update_time=? WHERE id=? AND library_id=? AND revision=?`, folder.ParentID, folder.Name, folder.NameKey(), folder.Style, folder.Theme, now, folder.ID, library, folder.Revision); err != nil {
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
	CatalogState           string
	TrashedAt              *time.Time
	Position, Revision     int64
	CreateTime, UpdateTime time.Time
}

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
	if err := tx.Raw(`SELECT * FROM media.library_item WHERE library_id=? AND id IN ? AND purged_at IS NULL ORDER BY id FOR UPDATE`, library, ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make(map[uuid.UUID]domain.LibraryItem, len(ids))
	assets := make([]uuid.UUID, 0, len(ids))
	for _, row := range rows {
		i, err := row.item()
		if err != nil {
			return nil, err
		}
		if i.State == "removed" {
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
		query := `SELECT * FROM media.media_asset WHERE id IN ? AND project_id=? AND personal_actor_id IS NULL AND personal_org_id IS NULL ORDER BY id FOR UPDATE`
		args := []any{assets, scope.ProjectID}
		if scope.Kind == domain.LibraryPersonal {
			query = `SELECT * FROM media.media_asset WHERE id IN ? AND project_id IS NULL AND personal_org_id=? AND personal_actor_id=? ORDER BY id FOR UPDATE`
			args = []any{assets, actor.OrgID, actor.ID}
		}
		var binary []assetRow
		if err := tx.Raw(query, args...).Scan(&binary).Error; err != nil {
			return nil, err
		}
		found := make(map[uuid.UUID]bool, len(binary))
		for _, row := range binary {
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
				items[id] = domain.LibraryItem{ID: id, LibraryID: library, AssetID: &id, Title: title, Category: "material", Tags: []string{}, State: "active", CreatedAt: a.CreateTime, UpdatedAt: a.UpdateTime}
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
	if previous == 0 {
		return libraryChanged(tx, `INSERT INTO media.library_item(id,library_id,asset_id,plain_text,folder_id,title,category,tags,source_label,note,favorite,catalog_state,trashed_at,position,revision,create_time,update_time) VALUES(?,?,?,?,?,?,?,?::jsonb,?,?,?,?,?,?,?,?,?)`, item.ID, item.LibraryID, item.AssetID, item.PlainText, item.FolderID, item.Title, item.Category, string(tags), item.SourceLabel, item.Note, item.Favorite, item.State, item.TrashedAt, item.Position, item.Revision, item.CreatedAt, item.UpdatedAt)
	}
	return libraryChanged(tx, `UPDATE media.library_item SET plain_text=?,folder_id=?,title=?,category=?,tags=?::jsonb,source_label=?,note=?,favorite=?,catalog_state=?,trashed_at=?,position=?,revision=?,update_time=? WHERE id=? AND library_id=? AND revision=?`, item.PlainText, item.FolderID, item.Title, item.Category, string(tags), item.SourceLabel, item.Note, item.Favorite, item.State, item.TrashedAt, item.Position, item.Revision, item.UpdatedAt, item.ID, item.LibraryID, previous)
}

func applyLibraryItems(tx *gorm.DB, actor identityapp.Principal, c application.LibraryCommand, library uuid.UUID, folders []domain.LibraryFolder, now time.Time, result *application.LibraryReceipt) error {
	var targets []application.LibraryItemRevision
	switch {
	case c.Action == "create_text":
		targets = []application.LibraryItemRevision{{ID: uuid.NewSHA1(c.Key, []byte("media-library-text/"+actor.ID.String()+"/"+library.String()))}}
	case c.ItemID != nil:
		targets = []application.LibraryItemRevision{{ID: *c.ItemID, Revision: c.ExpectedItemRevision}}
	default:
		targets = slices.Clone(c.Items)
	}
	items := make(map[uuid.UUID]domain.LibraryItem, len(targets))
	if c.Action == "create_text" {
		items[targets[0].ID] = domain.LibraryItem{ID: targets[0].ID, LibraryID: library, PlainText: c.Metadata.PlainText, State: "active", CreatedAt: now, UpdatedAt: now}
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
		if item.Revision != target.Revision || item.Revision >= 2147483647 {
			return application.ErrLibraryConflict
		}
		switch c.Action {
		case "create_text", "update_item":
			m := c.Metadata
			if (item.AssetID == nil) != (m.PlainText != nil) || c.Scope.Kind == domain.LibraryProject && m.Favorite {
				return domain.ErrInvalidLibrary
			}
			item.PlainText, item.FolderID, item.Title, item.Category, item.Tags, item.SourceLabel, item.Note, item.Favorite = m.PlainText, m.FolderID, m.Title, m.Category, m.Tags, m.SourceLabel, m.Note, m.Favorite
		case "move_items":
			if item.State != "active" {
				return domain.ErrMediaStateConflict
			}
			item.FolderID = c.TargetFolderID
		case "recycle_items":
			if item.State != "active" {
				return domain.ErrMediaStateConflict
			}
			item.State, item.TrashedAt = "trashed", &now
		case "restore_items":
			if item.State != "trashed" {
				return domain.ErrMediaStateConflict
			}
			item.State, item.TrashedAt = "active", nil
		case "remove_items":
			if item.State != "active" {
				return domain.ErrMediaStateConflict
			}
			item.State, item.FolderID = "removed", nil
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
			var actual string
			if err := tx.Raw(`SELECT kind FROM media.media_asset WHERE id=?`, *item.AssetID).Scan(&actual).Error; err != nil {
				return err
			}
			kind = actual
		}
		result.Items = append(result.Items, application.LibraryItemChange{ID: item.ID, AssetID: item.AssetID, FolderID: item.FolderID, Kind: kind, State: item.State, TrashedAt: item.TrashedAt, Revision: item.Revision})
	}
	return nil
}

var _ application.LibraryRepository = (*LibraryStore)(nil)

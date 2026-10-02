package postgres

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

type copySourceLibrary struct {
	row     libraryRow
	folders []domain.LibraryFolder
	items   []domain.LibraryItem
}

// Freeze follows the coordinator's project locks with library then metadata
// locks before any original/rendition lock. Every catalog item is frozen; a
// missing binary mapping fails closed instead of silently losing its metadata.
func readCopySourceLibrary(tx *gorm.DB, actor identityapp.Principal, binding application.ProjectCopyBinding) (copySourceLibrary, error) {
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &binding.SourceProjectID}
	row, err := readLibrary(tx, actor, scope, false)
	if err != nil {
		return copySourceLibrary{}, err
	}
	folders, err := libraryFolders(tx, row.ID, false)
	if err != nil {
		return copySourceLibrary{}, err
	}
	var rows []libraryItemRow
	if err := tx.Raw(`SELECT * FROM media.library_item WHERE library_id=? AND purged_at IS NULL ORDER BY id LIMIT 8193 FOR SHARE`, row.ID).Scan(&rows).Error; err != nil {
		return copySourceLibrary{}, fmt.Errorf("freeze complete library items: %w", err)
	}
	if len(rows) > 8192 {
		return copySourceLibrary{}, application.ErrProjectCopyMediaUnavailable
	}
	items := make([]domain.LibraryItem, 0, len(rows))
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	if err := requireNoPurgeReservation(tx, ids); err != nil {
		return copySourceLibrary{}, err
	}
	for _, row := range rows {
		item, err := row.item()
		if err != nil {
			return copySourceLibrary{}, err
		}
		items = append(items, item)
	}
	return copySourceLibrary{row: row, folders: folders, items: items}, nil
}

func registerCopiedLibrary(tx *gorm.DB, actor identityapp.Principal, binding application.ProjectCopyBinding, catalog *application.ProjectLibraryCopy) error {
	if catalog == nil {
		return nil
	}
	if catalog.Validate(binding) != nil {
		return application.ErrProjectCopyMediaUnavailable
	}
	if err := tx.Exec(`INSERT INTO media.library(id,kind,org_id,project_id,revision) VALUES(?,'project',?,?,1) ON CONFLICT(id) DO NOTHING`, catalog.LibraryID, actor.OrgID, binding.TargetProjectID).Error; err != nil {
		return err
	}
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &binding.TargetProjectID}
	row, err := readLibrary(tx, actor, scope, true)
	if err != nil {
		return err
	}
	if row.Revision != 1 {
		return application.ErrObjectMismatch
	}
	// INSERT parent folders first while preserving each frozen position.
	remaining := append([]domain.LibraryFolder(nil), catalog.Folders...)
	inserted := make(map[uuid.UUID]bool, len(remaining))
	for len(remaining) > 0 {
		next := make([]domain.LibraryFolder, 0, len(remaining))
		advanced := false
		for _, folder := range remaining {
			if folder.ParentID != nil && !inserted[*folder.ParentID] {
				next = append(next, folder)
				continue
			}
			if err := libraryChanged(tx, `INSERT INTO media.library_folder(id,library_id,library_kind,parent_id,name,name_key,position,style,theme,revision,create_time,update_time) VALUES(?,?,'project',?,?,?,?,?,?,1,?,?)`, folder.ID, folder.LibraryID, folder.ParentID, folder.Name, folder.NameKey(), folder.Position, folder.Style, folder.Theme, folder.CreatedAt, folder.UpdatedAt); err != nil {
				return err
			}
			inserted[folder.ID] = true
			advanced = true
		}
		if !advanced {
			return application.ErrProjectCopyMediaUnavailable
		}
		remaining = next
	}
	for _, item := range catalog.Items {
		if err := persistLibraryItem(tx, item, 0); err != nil {
			return err
		}
	}
	return verifyCopiedLibrary(tx, actor, binding, catalog)
}

func copiedMediaContentBytes(content application.ProjectMediaCopy) ([]byte, error) {
	if content.Library == nil {
		return json.Marshal(content.Assets)
	}
	return json.Marshal(struct {
		Assets  []application.ProjectCopySourceAsset `json:"assets"`
		Library *application.ProjectLibraryCopy      `json:"library"`
	}{content.Assets, content.Library})
}

func verifyCopiedLibrary(tx *gorm.DB, actor identityapp.Principal, binding application.ProjectCopyBinding, catalog *application.ProjectLibraryCopy) error {
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &binding.TargetProjectID}
	row, err := readLibrary(tx, actor, scope, false)
	if err != nil {
		return err
	}
	folders, err := libraryFolders(tx, row.ID, false)
	if err != nil {
		return err
	}
	var rows []libraryItemRow
	if err := tx.Raw(`SELECT * FROM media.library_item WHERE library_id=? ORDER BY id LIMIT 8193 FOR SHARE`, row.ID).Scan(&rows).Error; err != nil {
		return err
	}
	if catalog == nil {
		if len(folders) > 0 || len(rows) > 0 {
			return application.ErrObjectMismatch
		}
		return nil
	}
	if catalog.Validate(binding) != nil || row.Revision != 1 || len(folders) != len(catalog.Folders) || len(rows) != len(catalog.Items) {
		return application.ErrObjectMismatch
	}
	expectedFolders := make(map[uuid.UUID]domain.LibraryFolder, len(folders))
	for _, folder := range catalog.Folders {
		expectedFolders[folder.ID] = folder
	}
	for _, folder := range folders {
		expected, ok := expectedFolders[folder.ID]
		if !ok || !equalCopyMedia(expected, folder) {
			return application.ErrObjectMismatch
		}
	}
	expectedItems := make(map[uuid.UUID]domain.LibraryItem, len(rows))
	for _, item := range catalog.Items {
		expectedItems[item.ID] = item
	}
	for _, row := range rows {
		item, err := row.item()
		if err != nil {
			return err
		}
		expected, ok := expectedItems[item.ID]
		if !ok || !equalCopyMedia(expected, item) {
			return application.ErrObjectMismatch
		}
	}
	return nil
}

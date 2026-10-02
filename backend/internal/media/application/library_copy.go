package application

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// ProjectLibraryCopy is private, immutable media-owner metadata in a Copy
// manifest. Nil keeps historical media content bytes exactly unchanged.
type ProjectLibraryCopy struct {
	Version         int                    `json:"version"`
	LibraryID       uuid.UUID              `json:"library_id"`
	SourceLibraryID uuid.UUID              `json:"source_library_id"`
	SourceRevision  int64                  `json:"source_revision"`
	Folders         []domain.LibraryFolder `json:"folders"`
	Items           []domain.LibraryItem   `json:"items"`
}

// PrepareProjectLibraryCopy maps the complete frozen project catalog to stable
// target identities; no binary identity or editable slice aliases its source.
func PrepareProjectLibraryCopy(binding ProjectCopyBinding, sourceLibrary uuid.UUID, sourceRevision int64, folders []domain.LibraryFolder, items []domain.LibraryItem, assets map[uuid.UUID]uuid.UUID, now time.Time) (*ProjectLibraryCopy, error) {
	if binding.Validate() != nil || now.IsZero() || sourceRevision < 0 || sourceRevision > 2147483647 || len(folders) > 4096 || len(items) > 8192 {
		return nil, ErrProjectCopyMediaUnavailable
	}
	sourceScope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &binding.SourceProjectID}
	sourceID, _ := sourceScope.Identity(binding.OrgID, binding.JobID)
	if sourceLibrary != sourceID {
		return nil, ErrProjectCopyMediaUnavailable
	}
	targetScope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &binding.TargetProjectID}
	targetID, _ := targetScope.Identity(binding.OrgID, binding.JobID)
	raw, err := json.Marshal(ProjectLibraryCopy{Version: 1, LibraryID: targetID, SourceLibraryID: sourceID, SourceRevision: sourceRevision, Folders: folders, Items: items})
	if err != nil || len(raw) > 32<<20 {
		return nil, ErrProjectCopyMediaUnavailable
	}
	var result ProjectLibraryCopy
	if json.Unmarshal(raw, &result) != nil {
		return nil, ErrProjectCopyMediaUnavailable
	}
	folderMap := make(map[uuid.UUID]uuid.UUID, len(folders))
	for _, folder := range folders {
		if folder.LibraryID != sourceID || folder.Kind != domain.LibraryProject || folder.Validate() != nil || folderMap[folder.ID] != uuid.Nil {
			return nil, ErrProjectCopyMediaUnavailable
		}
		folderMap[folder.ID] = uuid.NewSHA1(binding.JobID, []byte("library-folder/"+folder.ID.String()))
	}
	if len(folders) > 0 && domain.ValidateLibraryFolderPlacement(folders[0], folders) != nil {
		return nil, ErrProjectCopyMediaUnavailable
	}
	now = now.UTC().Truncate(time.Microsecond)
	for i := range result.Folders {
		folder := &result.Folders[i]
		folder.ID = folderMap[folder.ID]
		folder.LibraryID = targetID
		folder.Revision = 1
		folder.CreatedAt = now
		folder.UpdatedAt = now
		if folder.ParentID != nil {
			parent := folderMap[*folder.ParentID]
			folder.ParentID = &parent
		}
	}
	seen := make(map[uuid.UUID]bool, len(items))
	for i := range result.Items {
		item := &result.Items[i]
		if item.Validate() != nil || item.LibraryID != sourceID || item.Favorite || seen[item.ID] || item.AssetID != nil && item.ID != *item.AssetID {
			return nil, ErrProjectCopyMediaUnavailable
		}
		seen[item.ID] = true
		if item.AssetID == nil {
			item.ID = uuid.NewSHA1(binding.JobID, []byte("library-text/"+item.ID.String()))
		} else {
			target := assets[*item.AssetID]
			if target == uuid.Nil {
				return nil, ErrProjectCopyMediaUnavailable
			}
			item.ID = target
			item.AssetID = &target
		}
		item.LibraryID = targetID
		item.Revision = 1
		item.CreatedAt = now
		item.UpdatedAt = now
		if item.FolderID != nil {
			folder := folderMap[*item.FolderID]
			if folder == uuid.Nil {
				return nil, ErrProjectCopyMediaUnavailable
			}
			item.FolderID = &folder
		}
	}
	if len(result.Folders) == 0 && len(result.Items) == 0 {
		return nil, nil
	}
	if err := result.Validate(binding); err != nil {
		return nil, err
	}
	return &result, nil
}

// Validate closes stored catalog identities and all target references.
func (c ProjectLibraryCopy) Validate(binding ProjectCopyBinding) error {
	targetScope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &binding.TargetProjectID}
	targetID, _ := targetScope.Identity(binding.OrgID, binding.JobID)
	sourceScope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &binding.SourceProjectID}
	sourceID, _ := sourceScope.Identity(binding.OrgID, binding.JobID)
	if binding.Validate() != nil || c.Version != 1 || c.LibraryID != targetID || c.SourceLibraryID != sourceID || c.SourceRevision < 0 || c.SourceRevision > 2147483647 || len(c.Folders) > 4096 || len(c.Items) > 8192 {
		return ErrProjectCopyMediaUnavailable
	}
	folders := make(map[uuid.UUID]bool, len(c.Folders))
	for _, folder := range c.Folders {
		if folder.Validate() != nil || folder.LibraryID != targetID || folder.Kind != domain.LibraryProject || folder.Revision != 1 || folders[folder.ID] {
			return ErrProjectCopyMediaUnavailable
		}
		folders[folder.ID] = true
	}
	if len(c.Folders) > 0 && domain.ValidateLibraryFolderPlacement(c.Folders[0], c.Folders) != nil {
		return ErrProjectCopyMediaUnavailable
	}
	seen := make(map[uuid.UUID]bool, len(c.Items))
	for _, item := range c.Items {
		if item.Validate() != nil || item.LibraryID != targetID || item.Revision != 1 || item.Favorite || seen[item.ID] || item.AssetID != nil && *item.AssetID != item.ID || item.FolderID != nil && !folders[*item.FolderID] {
			return ErrProjectCopyMediaUnavailable
		}
		seen[item.ID] = true
	}
	return nil
}

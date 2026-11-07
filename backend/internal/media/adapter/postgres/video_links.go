package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// LookupPersonalVideoLink recovers only the current actor's exact share-page
// fingerprint and currently visible completed upload. No network retry is needed.
func (s *Store) LookupPersonalVideoLink(ctx context.Context, actor identity.Principal, key uuid.UUID, name string) (application.PersonalUploadResult, bool, error) {
	if s == nil || s.db == nil {
		return application.PersonalUploadResult{}, false, application.ErrUnavailable
	}
	if key == uuid.Nil || actor.ID == uuid.Nil || actor.OrgID == uuid.Nil {
		return application.PersonalUploadResult{}, false, application.ErrInvalidVideoLink
	}
	var result application.PersonalUploadResult
	found := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requirePersonal(tx, actor, true); err != nil {
			return err
		}
		owner := domain.PersonalOwnership{OrgID: actor.OrgID, ActorID: actor.ID}
		if err := personalUploadLock(tx, owner); err != nil {
			return err
		}
		var row uploadRequestRecord
		read := tx.Where(map[string]any{"project_id": nil, "personal_org_id": actor.OrgID, "principal_id": actor.ID, "request_key": key}).Take(&row)
		if errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return nil
		}
		if read.Error != nil {
			return read.Error
		}
		if row.FileName != name || row.SourceKind != "video_link" {
			return application.ErrUploadConflict
		}
		library, err := readLibrary(tx, actor, domain.LibraryScope{Kind: domain.LibraryPersonal}, false)
		if err != nil {
			return err
		}
		var visible int64
		// Ordinary uploads are canonical revision-1 entries until metadata is
		// materialized. Use the same effective projection as LibraryDetail so a
		// successful new upload replays without fabricating a library_item row.
		entries := libraryEntries(tx, actor, domain.LibraryScope{Kind: domain.LibraryPersonal}, library.ID)
		if err := entries.Where("asset_id=? AND catalog_state=?", row.AssetID, domain.LibraryItemStateActive).Count(&visible).Error; err != nil {
			return err
		}
		if visible != 1 {
			return application.ErrUploadConflict
		}
		result, found, err = replayPersonalUpload(tx, actor, application.UploadRequest{Key: key, Personal: &owner, FileName: name, SHA256: row.SHA256, ByteSize: row.ByteSize, VideoLink: true})
		return err
	})
	return result, found, err
}

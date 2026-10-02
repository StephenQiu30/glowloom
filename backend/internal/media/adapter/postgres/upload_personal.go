package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

var _ application.PersonalUploadRepository = (*Store)(nil)

func personalUploadRequest(actor identityapp.Principal, r application.UploadRequest) error {
	if application.ValidateUploadRequest(r) != nil || r.ProjectID != uuid.Nil || r.Personal == nil || r.Personal.OrgID != actor.OrgID || r.Personal.ActorID != actor.ID {
		return application.ErrInvalidUpload
	}
	return nil
}
func sameUploadOwnership(actor identityapp.Principal, r application.UploadRequest, a domain.MediaAsset) bool {
	if r.Personal == nil {
		return a.Personal == nil && r.ProjectID != uuid.Nil
	}
	return r.ProjectID == uuid.Nil && a.Personal != nil && *r.Personal == *a.Personal && a.Personal.OrgID == actor.OrgID && a.Personal.ActorID == actor.ID
}
func personalUploadLock(tx *gorm.DB, owner domain.PersonalOwnership) error {
	return tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "media-personal-upload/"+owner.OrgID.String()+"/"+owner.ActorID.String()).Error
}

// AuthorizePersonalUpload rechecks the current actor without inventing a project.
func (s *Store) AuthorizePersonalUpload(ctx context.Context, actor identityapp.Principal) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return requireCurrentActor(tx, actor) })
}

// FindPersonalUpload checks current authorization before reading a scoped receipt.
func (s *Store) FindPersonalUpload(ctx context.Context, actor identityapp.Principal, r application.UploadRequest) (application.PersonalUploadResult, bool, error) {
	if s == nil || s.db == nil {
		return application.PersonalUploadResult{}, false, application.ErrUnavailable
	}
	if err := personalUploadRequest(actor, r); err != nil {
		return application.PersonalUploadResult{}, false, err
	}
	var result application.PersonalUploadResult
	var found bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := personalUploadLock(tx, *r.Personal); err != nil {
			return err
		}
		if _, err := readLibrary(tx, actor, domain.LibraryScope{Kind: domain.LibraryPersonal}, false); err != nil {
			return err
		}
		var err error
		result, found, err = replayPersonalUpload(tx, actor, r)
		return err
	})
	return result, found, err
}
func replayPersonalUpload(tx *gorm.DB, actor identityapp.Principal, r application.UploadRequest) (application.PersonalUploadResult, bool, error) {
	var row struct {
		SHA256, FileName string
		ByteSize         int64
		AssetID          uuid.UUID
		Response         json.RawMessage
	}
	read := tx.Raw(`SELECT sha256,file_name,byte_size,asset_id,response FROM media.upload_request WHERE project_id IS NULL AND personal_org_id=? AND principal_id=? AND request_key=?`, actor.OrgID, actor.ID, r.Key).Scan(&row)
	if read.Error != nil {
		return application.PersonalUploadResult{}, false, fmt.Errorf("read personal upload receipt: %w", read.Error)
	}
	if read.RowsAffected == 0 {
		return application.PersonalUploadResult{}, false, nil
	}
	if row.SHA256 != r.SHA256 || row.FileName != r.FileName || row.ByteSize != r.ByteSize {
		return application.PersonalUploadResult{}, false, application.ErrUploadConflict
	}
	var result application.PersonalUploadResult
	decoder := json.NewDecoder(bytes.NewReader(row.Response))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || result.Asset.ID != row.AssetID || result.Asset.Revision < 1 || result.Asset.ByteSize < 1 {
		return application.PersonalUploadResult{}, false, application.ErrUnavailable
	}
	var available assetRow
	read = tx.Raw(`SELECT * FROM media.media_asset WHERE id=? AND project_id IS NULL AND personal_org_id=? AND personal_actor_id=? AND origin='upload' AND status='ready' AND moderation_status='passed' AND NOT is_delete AND NOT contains_real_person FOR SHARE`, row.AssetID, actor.OrgID, actor.ID).Scan(&available)
	if read.Error != nil {
		return application.PersonalUploadResult{}, false, read.Error
	}
	if read.RowsAffected != 1 {
		return application.PersonalUploadResult{}, false, application.ErrUploadConflict
	}
	if available.domain().Validate() != nil {
		return application.PersonalUploadResult{}, false, application.ErrUnavailable
	}
	return result, true, nil
}

// CommitPersonalUpload preserves all project, metadata and byte ownership.
// Actor -> scoped admission -> library -> original locks serialize deduplication.
func (s *Store) CommitPersonalUpload(ctx context.Context, actor identityapp.Principal, r application.UploadRequest, a domain.MediaAsset, renditions []domain.Rendition) (application.PersonalUploadResult, error) {
	if s == nil || s.db == nil {
		return application.PersonalUploadResult{}, application.ErrUnavailable
	}
	if err := personalUploadRequest(actor, r); err != nil {
		return application.PersonalUploadResult{}, err
	}
	if err := validateReviewedUpload(actor, r, a, renditions); err != nil {
		return application.PersonalUploadResult{}, err
	}
	var result application.PersonalUploadResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := personalUploadLock(tx, *r.Personal); err != nil {
			return err
		}
		scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
		id, _ := scope.Identity(actor.OrgID, actor.ID)
		if err := tx.Exec(`INSERT INTO media.library(id,kind,org_id,personal_actor_id) VALUES(?,'personal',?,?) ON CONFLICT(id) DO NOTHING`, id, actor.OrgID, actor.ID).Error; err != nil {
			return fmt.Errorf("ensure personal upload library: %w", err)
		}
		library, err := readLibrary(tx, actor, scope, true)
		if err != nil {
			return err
		}
		if prior, found, err := replayPersonalUpload(tx, actor, r); err != nil || found {
			result = prior
			return err
		}
		var existing assetRow
		read := tx.Raw(`SELECT * FROM media.media_asset WHERE project_id IS NULL AND personal_org_id=? AND personal_actor_id=? AND (sha256=? OR (? AND moderation_detail->'normalization'->'source'->>'sha256'=? AND moderation_detail->'normalization'->>'method'='webm_vp8_vp9_to_mp4_h264' AND moderation_detail->'normalization'->>'version'='1')) AND origin='upload' AND status='ready' AND moderation_status='passed' AND NOT is_delete AND NOT contains_real_person AND moderation_detail->>'method'='local_workspace_owner_review' ORDER BY create_time,id LIMIT 1 FOR SHARE`, actor.OrgID, actor.ID, *a.SHA256, *a.SHA256 != r.SHA256, r.SHA256).Scan(&existing)
		if read.Error != nil {
			return fmt.Errorf("find same personal original: %w", read.Error)
		}
		if read.RowsAffected == 1 {
			duplicate := existing.domain()
			if duplicate.Validate() != nil {
				return application.ErrUnavailable
			}
			duplicateID := duplicate.ID
			result = application.PersonalUploadResult{Asset: application.PersonalUploadSummary(duplicate), DuplicateOf: &duplicateID}
		} else {
			if err := libraryChanged(tx, `INSERT INTO media.media_asset(id,personal_org_id,personal_actor_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,width,height,duration_ms,fps,audio_channels,codec,moderation_status,moderation_detail,revision,create_time,update_time) VALUES(?,?,?,?,'upload','ready',?,?,?,?,?,?,?,?,?,?,?,'passed',?::jsonb,1,?,?)`, a.ID, actor.OrgID, actor.ID, string(a.Kind), a.ObjectKey, a.FileName, a.MimeType, a.ByteSize, a.SHA256, a.Width, a.Height, a.DurationMS, a.FPS, a.AudioChannels, a.Codec, string(a.ModerationDetail), a.CreateTime, a.UpdateTime); err != nil {
				return err
			}
			for _, rend := range renditions {
				if err := libraryChanged(tx, `INSERT INTO media.rendition(id,media_asset_id,kind,object_key,width,height,byte_size,create_time,update_time) VALUES(?,?,?,?,?,?,?,?,?)`, rend.ID, rend.MediaAssetID, string(rend.Kind), rend.ObjectKey, rend.Width, rend.Height, rend.ByteSize, rend.CreateTime, rend.UpdateTime); err != nil {
					return err
				}
			}
			if err := libraryChanged(tx, `UPDATE media.library SET revision=revision+1,update_time=? WHERE id=? AND revision=?`, a.UpdateTime, id, library.Revision); err != nil {
				return err
			}
			result = application.PersonalUploadResult{Asset: application.PersonalUploadSummary(a)}
		}
		body, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if err := libraryChanged(tx, `INSERT INTO media.upload_request(personal_org_id,principal_id,request_key,sha256,file_name,byte_size,asset_id,response) VALUES(?,?,?,?,?,?,?,?::jsonb)`, actor.OrgID, actor.ID, r.Key, r.SHA256, r.FileName, r.ByteSize, result.Asset.ID, string(body)); err != nil {
			return err
		}
		after, err := json.Marshal(map[string]any{"kind": result.Asset.Kind, "byte_size": result.Asset.ByteSize, "sha256": r.SHA256, "review_method": "local_workspace_owner_review", "reused": result.DuplicateOf != nil})
		if err != nil {
			return err
		}
		return libraryChanged(tx, `INSERT INTO audit.audit_log(id,org_id,actor_id,actor_kind,action,object_type,object_id,after,request_id,create_time,update_time) VALUES(?,?,?,'user','media.uploaded','media_asset',?,?::jsonb,?,?,?)`, uuid.New(), actor.OrgID, actor.ID, result.Asset.ID.String(), string(after), r.RequestID.String(), a.CreateTime, a.CreateTime)
	})
	return result, err
}

// PersonalUploadAssetExists waits for the same scope lock as Commit and includes
// deleted assets. An uncertain commit therefore never deletes a held object.
func (s *Store) PersonalUploadAssetExists(ctx context.Context, owner domain.PersonalOwnership, id uuid.UUID) (bool, error) {
	if s == nil || s.db == nil || owner.OrgID == uuid.Nil || owner.ActorID == uuid.Nil || id == uuid.Nil {
		return false, application.ErrUnavailable
	}
	var count int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := personalUploadLock(tx, owner); err != nil {
			return err
		}
		// No actor authorization is needed for this private cleanup decision. It
		// cannot return metadata and conservatively preserves any durable owner.
		return tx.Raw(`SELECT count(*) FROM media.media_asset WHERE id=? AND project_id IS NULL AND personal_org_id=? AND personal_actor_id=?`, id, owner.OrgID, owner.ActorID).Scan(&count).Error
	})
	return count != 0, err
}

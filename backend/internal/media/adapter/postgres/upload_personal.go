package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
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
	key := uuid.NewSHA1(owner.OrgID, []byte("media-personal-upload/"+owner.ActorID.String()))
	return lockStorageIdempotencyKey(tx, storageIdempotencyPersonalUpload, owner.ActorID, key)
}

// AuthorizePersonalUpload rechecks the current actor without inventing a project.
func (s *Store) AuthorizePersonalUpload(ctx context.Context, actor identityapp.Principal) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return s.requirePersonal(tx, actor, true) })
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
		if err := s.requirePersonal(tx, actor, true); err != nil {
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
	var row uploadRequestRecord
	read := tx.Where(map[string]any{"project_id": nil, "personal_org_id": actor.OrgID, "principal_id": actor.ID, "request_key": r.Key}).Take(&row)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return application.PersonalUploadResult{}, false, nil
	}
	if read.Error != nil {
		return application.PersonalUploadResult{}, false, fmt.Errorf("read personal upload receipt: %w", read.Error)
	}
	if row.SHA256 != r.SHA256 || row.FileName != r.FileName || row.ByteSize != r.ByteSize || row.SourceKind != r.SourceKind() {
		return application.PersonalUploadResult{}, false, application.ErrUploadConflict
	}
	var result application.PersonalUploadResult
	decoder := json.NewDecoder(bytes.NewReader(row.Response))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || result.Asset.ID != row.AssetID || result.Asset.Revision < 1 || result.Asset.ByteSize < 1 {
		return application.PersonalUploadResult{}, false, application.ErrUnavailable
	}
	var available assetRow
	read = tx.Clauses(clause.Locking{Strength: "SHARE"}).Where(map[string]any{"id": row.AssetID, "project_id": nil, "personal_org_id": actor.OrgID, "personal_actor_id": actor.ID, "origin": domain.OriginUpload, "status": domain.StatusReady, "moderation_status": domain.ModerationPassed, "is_delete": false, "contains_real_person": false}).Take(&available)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return application.PersonalUploadResult{}, false, application.ErrUploadConflict
	}
	if read.Error != nil {
		return application.PersonalUploadResult{}, false, read.Error
	}
	if err := available.loadLocation(tx); err != nil {
		return application.PersonalUploadResult{}, false, err
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
		if err := s.requirePersonal(tx, actor, true); err != nil {
			return err
		}
		if err := personalUploadLock(tx, *r.Personal); err != nil {
			return err
		}
		scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
		id, _ := scope.Identity(actor.OrgID, actor.ID)
		personalActorID := actor.ID
		libraryRecord := libraryRow{ID: id, Kind: string(domain.LibraryPersonal), OrgID: actor.OrgID, PersonalActorID: &personalActorID}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&libraryRecord).Error; err != nil {
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
		read := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("project_id IS NULL AND personal_org_id = ? AND personal_actor_id = ?", actor.OrgID, actor.ID).
			Where("(sha256 = ? OR (? AND moderation_detail->'normalization'->'source'->>'sha256' = ? AND moderation_detail->'normalization'->>'method' = ? AND moderation_detail->'normalization'->>'version' = ?))", *a.SHA256, *a.SHA256 != r.SHA256, r.SHA256, "webm_vp8_vp9_to_mp4_h264", "1").
			Where(map[string]any{"origin": domain.OriginUpload, "status": domain.StatusReady, "moderation_status": domain.ModerationPassed, "is_delete": false, "contains_real_person": false}).
			Where("moderation_detail->>'method' = ?", "local_workspace_owner_review").Order("create_time").Order("id").Limit(1).Take(&existing)
		if read.Error != nil {
			if errors.Is(read.Error, gorm.ErrRecordNotFound) {
				read.Error = nil
			} else {
				return fmt.Errorf("find same personal original: %w", read.Error)
			}
		}
		if read.Error == nil && existing.ID != uuid.Nil {
			if err := existing.loadLocation(tx); err != nil {
				return err
			}
			duplicate := existing.domain()
			if duplicate.Validate() != nil {
				return application.ErrUnavailable
			}
			duplicateID := duplicate.ID
			result = application.PersonalUploadResult{Asset: application.PersonalUploadSummary(duplicate), DuplicateOf: &duplicateID}
		} else {
			if err := insertReviewedUpload(tx, a, renditions); err != nil {
				return err
			}
			if err := updateOne(tx, "media.library", map[string]any{"id": id, "revision": library.Revision}, map[string]any{"revision": library.Revision + 1, "update_time": a.UpdateTime}, application.ErrLibraryConflict); err != nil {
				return err
			}
			result = application.PersonalUploadResult{Asset: application.PersonalUploadSummary(a)}
		}
		if err := acceptUploadLocations(tx, actor, r, a, renditions, result.Asset.ID); err != nil {
			return err
		}
		body, err := json.Marshal(result)
		if err != nil {
			return err
		}
		personalOrgID := actor.OrgID
		receipt := uploadRequestRecord{PersonalOrgID: &personalOrgID, PrincipalID: actor.ID, RequestKey: r.Key, SHA256: r.SHA256, FileName: r.FileName, SourceKind: r.SourceKind(), ByteSize: r.ByteSize, AssetID: result.Asset.ID, Response: jsonDocument(body)}
		if err := requireOne(tx.Create(&receipt), application.ErrUnavailable); err != nil {
			return err
		}
		after, err := json.Marshal(map[string]any{"kind": result.Asset.Kind, "byte_size": result.Asset.ByteSize, "sha256": r.SHA256, "review_method": "local_workspace_owner_review", "reused": result.DuplicateOf != nil})
		if err != nil {
			return err
		}
		audit := uploadAuditRecord{ID: uuid.New(), OrgID: actor.OrgID, ActorID: actor.ID, ActorKind: "user", Action: "media.uploaded", ObjectType: "media_asset", ObjectID: result.Asset.ID.String(), After: jsonDocument(after), RequestID: r.RequestID.String(), CreateTime: a.CreateTime, UpdateTime: a.CreateTime}
		return requireOne(tx.Create(&audit), application.ErrUnavailable)
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
		return tx.Model(&assetRow{}).Where(map[string]any{"id": id, "project_id": nil, "personal_org_id": owner.OrgID, "personal_actor_id": owner.ActorID}).Count(&count).Error
	})
	return count != 0, err
}

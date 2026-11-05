package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// UploadAdmission binds current authorization, idempotency and routing in one
// short transaction; no multipart read or provider call occurs under its locks.
type UploadAdmission struct {
	media *Store
}

// NewUploadAdmission loads limits and lifetime from the environment projection.
func NewUploadAdmission(media *Store) *UploadAdmission {
	return &UploadAdmission{media: media}
}

type uploadAdmissionRow struct {
	ID                                  uuid.UUID
	OrgID, ActorID, ScopeID, RequestKey uuid.UUID
	ScopeKind                           string
	ProfileID, Bucket, ConfigRevision   string
	Policy                              jsonDocument
	ExpiresAt                           time.Time
	AcceptedAssetID                     *uuid.UUID
}

func (uploadAdmissionRow) TableName() string { return "media.upload_admission" }

func (r uploadAdmissionRow) target() (application.UploadTarget, error) {
	var policy application.UploadPolicy
	decoder := json.NewDecoder(bytes.NewReader(r.Policy))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&policy) != nil || decoder.Decode(new(any)) != io.EOF || policy.Validate() != nil || r.ID == uuid.Nil || r.ExpiresAt.IsZero() || (domain.ObjectRef{ProfileID: r.ProfileID, Bucket: r.Bucket, Key: "validation"}).Validate() != nil {
		return application.UploadTarget{}, application.ErrStorageNotReady
	}
	return application.UploadTarget{SessionID: r.ID, ProfileID: r.ProfileID, Bucket: r.Bucket, ConfigRevision: r.ConfigRevision, ExpiresAt: r.ExpiresAt, Policy: policy}, nil
}

// PinUpload persists the first target and returns that same target on retries.
func (s *UploadAdmission) PinUpload(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, key uuid.UUID) (application.UploadTarget, error) {
	var target application.UploadTarget
	if s == nil || s.media == nil || s.media.db == nil {
		return target, application.ErrUnavailable
	}
	if scope.Validate() != nil || key == uuid.Nil {
		return target, application.ErrInvalidUpload
	}
	scopeID, err := scope.Identity(actor.OrgID, actor.ID)
	if err != nil {
		return target, err
	}
	err = s.media.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Keep authorization on this transaction while using the store's public
		// authority contract. A copy retains all explicitly injected dependencies.
		authority := *s.media
		authority.db = tx
		if scope.Kind == domain.LibraryProject {
			if _, err := authority.AuthorizeUpload(ctx, actor, *scope.ProjectID); err != nil {
				return err
			}
		} else {
			if err := authority.AuthorizePersonalUpload(ctx, actor); err != nil {
				return err
			}
		}
		read := func() (uploadAdmissionRow, bool, error) {
			var row uploadAdmissionRow
			err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("org_id = ? AND actor_id = ? AND scope_id = ? AND request_key = ?", actor.OrgID, actor.ID, scopeID, key).Take(&row).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return row, false, nil
			}
			return row, err == nil, err
		}
		row, found, err := read()
		if err != nil {
			return err
		}
		if !found {
			route, err := readStorageRoute(tx, true)
			if errors.Is(err, application.ErrStorageNotReady) {
				return application.ErrStorageNotReady
			}
			if err != nil {
				return err
			}
			var snapshot storageConfigRevisionRecord
			if err := tx.Where("revision = ? AND profile_id = ?", route.Revision, route.ProfileID).Take(&snapshot).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return application.ErrStorageNotReady
				}
				return err
			}
			var policy application.UploadPolicy
			if json.Unmarshal(snapshot.UploadPolicy, &policy) != nil || policy.Validate() != nil || snapshot.SessionTTLSeconds < 60 {
				return application.ErrStorageNotReady
			}
			candidate := uploadAdmissionRow{ID: uuid.New(), OrgID: actor.OrgID, ActorID: actor.ID, ScopeID: scopeID, ScopeKind: string(scope.Kind), RequestKey: key, ProfileID: route.ProfileID, Bucket: route.Bucket, ConfigRevision: route.Revision, Policy: snapshot.UploadPolicy, ExpiresAt: time.Now().UTC().Add(time.Duration(snapshot.SessionTTLSeconds) * time.Second)}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "org_id"}, {Name: "actor_id"}, {Name: "scope_id"}, {Name: "request_key"}}, DoNothing: true}).Create(&candidate).Error; err != nil {
				return fmt.Errorf("persist upload target: %w", err)
			}
			row, found, err = read()
			if err != nil {
				return err
			}
			if !found {
				return application.ErrUnavailable
			}
		}
		if !row.ExpiresAt.After(time.Now().UTC()) && row.AcceptedAssetID == nil {
			return application.ErrUploadConflict
		}
		target, err = row.target()
		return err
	})
	return target, err
}

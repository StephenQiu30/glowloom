package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// UploadAdmission binds current authorization, idempotency and routing in one
// short transaction; no multipart read or provider call occurs under its locks.
type UploadAdmission struct {
	activatedPolicy bool
	media           *Store
	policy          application.UploadPolicy
	ttl             time.Duration
}

// NewUploadAdmission injects live scope authority and the deployment upload policy.
func NewUploadAdmission(media *Store, policy application.UploadPolicy, ttl time.Duration) *UploadAdmission {
	return &UploadAdmission{media: media, policy: policy, ttl: ttl}
}

// NewActivatedUploadAdmission loads limits and lifetime from the accepted
// database revision. Preloading a future YAML never changes the active policy.
func NewActivatedUploadAdmission(media *Store) *UploadAdmission {
	return &UploadAdmission{media: media, activatedPolicy: true}
}

type uploadAdmissionRow struct {
	ID                                uuid.UUID
	ProfileID, Bucket, ConfigRevision string
	Policy                            json.RawMessage
	ExpiresAt                         time.Time
	AcceptedAssetID                   *uuid.UUID
}

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
	if scope.Validate() != nil || key == uuid.Nil || !s.activatedPolicy && (s.policy.Validate() != nil || s.ttl < time.Second) {
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
			r := tx.Raw(`SELECT * FROM media.upload_admission WHERE org_id=? AND actor_id=? AND scope_id=? AND request_key=? FOR SHARE`, actor.OrgID, actor.ID, scopeID, key).Scan(&row)
			return row, r.RowsAffected == 1, r.Error
		}
		row, found, err := read()
		if err != nil {
			return err
		}
		if !found {
			var route application.StorageRoute
			r := tx.Raw(`SELECT r.profile_id,p.bucket,r.revision FROM media.storage_route r JOIN media.storage_profile p ON p.id=r.profile_id WHERE r.singleton=1 FOR SHARE OF r`).Scan(&route)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return application.ErrStorageNotReady
			}
			selectedPolicy, ttl := s.policy, s.ttl
			if s.activatedPolicy {
				var snapshot struct {
					UploadPolicy      json.RawMessage
					SessionTTLSeconds int64
				}
				read := tx.Raw(`SELECT upload_policy,session_ttl_seconds FROM media.storage_config_revision WHERE revision=? AND profile_id=?`, route.Revision, route.ProfileID).Scan(&snapshot)
				if read.Error != nil {
					return read.Error
				}
				if read.RowsAffected != 1 || json.Unmarshal(snapshot.UploadPolicy, &selectedPolicy) != nil || selectedPolicy.Validate() != nil || snapshot.SessionTTLSeconds < 60 {
					return application.ErrStorageNotReady
				}
				ttl = time.Duration(snapshot.SessionTTLSeconds) * time.Second
			}
			policy, err := json.Marshal(selectedPolicy)
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO media.upload_admission(id,org_id,actor_id,scope_id,scope_kind,request_key,profile_id,bucket,config_revision,policy,expires_at) VALUES(?,?,?,?,?,?,?,?,?,?::jsonb,now()+(?*interval '1 second')) ON CONFLICT(org_id,actor_id,scope_id,request_key) DO NOTHING`, uuid.New(), actor.OrgID, actor.ID, scopeID, string(scope.Kind), key, route.ProfileID, route.Bucket, route.Revision, string(policy), int64(ttl/time.Second)).Error; err != nil {
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
		if !row.ExpiresAt.After(time.Now()) && row.AcceptedAssetID == nil {
			return application.ErrUploadConflict
		}
		target, err = row.target()
		return err
	})
	return target, err
}

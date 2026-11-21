package aigateway

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
)

func (r *Runtime) referenceFact(ctx context.Context, tx *gorm.DB, a identity.Principal, p uuid.UUID, ref domain.Reference) (domain.Reference, operation.ProjectMediaObject, error) {
	var fact operation.ProjectMediaObject
	bound := false
	if ref.BindingID != uuid.Nil {
		binding, err := subjectwire.GatewayBinding(ctx, tx, subjectwire.UseRegistry(r.Objects), a, p, ref.BindingID)
		if err != nil {
			return ref, fact, err
		}
		if binding.Media.AssetID != ref.ID || ref.Version != "" && ref.Version != binding.BindingSHA256 && ref.Version != strconv.FormatInt(binding.Media.Revision, 10) {
			return ref, fact, domain.ErrForbidden
		}
		ref.Version = binding.BindingSHA256
		bound = true
	}
	var row struct {
		ID                                                  uuid.UUID
		ProjectID                                           *uuid.UUID
		Revision                                            int64
		Kind, Key, SHA256, ProfileID, Bucket, LocationState string
		LocationID                                          *uuid.UUID
		ByteSize                                            int64
		LocationSHA                                         *string
		LocationSize                                        *int64
		LocationKey                                         string
	}
	q := tx.Raw(`SELECT a.id,a.project_id,a.revision,a.kind,a.object_key AS key,a.sha256,a.object_location_id AS location_id,a.byte_size,l.profile_id,l.bucket,l.state AS location_state,l.sha256 AS location_sha,l.byte_size AS location_size,l.object_key AS location_key
 FROM media.media_asset a JOIN media.object_location l ON l.id=a.object_location_id WHERE a.id=? AND NOT a.is_delete AND a.status='ready' AND a.moderation_status IN ('passed','skipped') AND NOT a.contains_real_person FOR SHARE OF a,l`, ref.ID).Scan(&row)
	if q.Error != nil {
		return ref, fact, q.Error
	}
	if q.RowsAffected != 1 || !bound && (row.ProjectID == nil || *row.ProjectID != p) || row.LocationID == nil || row.LocationState != "accepted" || row.LocationSHA == nil || row.LocationSize == nil || *row.LocationSHA != row.SHA256 || *row.LocationSize != row.ByteSize || row.LocationKey != row.Key {
		return ref, fact, domain.ErrForbidden
	}
	if !bound {
		if ref.Version != "" && ref.Version != row.SHA256 && ref.Version != strconv.FormatInt(row.Revision, 10) {
			return ref, fact, domain.ErrConflict
		}
		ref.Version = row.SHA256
	}
	fact = operation.ProjectMediaObject{AssetID: ref.ID, ProjectID: p, LocationID: row.LocationID, ProfileID: row.ProfileID, Bucket: row.Bucket, Key: row.Key, SHA256: row.SHA256, Kind: row.Kind, Role: ref.Role, ByteSize: row.ByteSize}
	return ref, fact, nil
}
func (r *Runtime) freezeReferences(ctx context.Context, tx *gorm.DB, a identity.Principal, p uuid.UUID, model domain.Model, refs []domain.Reference) ([]domain.Reference, error) {
	result := make([]domain.Reference, 0, len(refs))
	for _, ref := range refs {
		resolved, fact, err := r.referenceFact(ctx, tx, a, p, ref)
		if err != nil {
			return nil, err
		}
		if model.VideoMetering != nil && fact.Kind != "image" {
			return nil, domain.ErrInvalid
		}
		result = append(result, resolved)
	}
	return result, nil
}
func (r *Runtime) mediaReferences(ctx context.Context, d application.Dispatch, refs []domain.Reference) ([]operation.ProviderReference, error) {
	var a identity.Principal
	var row struct{ Actor []byte }
	if err := r.DB.WithContext(ctx).Raw("SELECT actor FROM ai.job WHERE request_id=?", d.Call.ID).Scan(&row).Error; err != nil {
		return nil, err
	}
	if err := json.Unmarshal(row.Actor, &a); err != nil || a.ID != d.Call.Actor {
		return nil, domain.ErrForbidden
	}
	facts := make([]operation.ProjectMediaObject, 0, len(refs))
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if a.SessionID != uuid.Nil {
			if err := Authority(ctx, tx, a, d.Call.Scope, d.Call.Project, false); err != nil {
				return err
			}
		} else {
			if _, err := scopewire.ProvideScopeAccess(tx).Resolve(ctx, a.ID, d.Call.Scope, true); err != nil {
				return err
			}
			if err := projectScope(tx, d.Call.Scope, d.Call.Project); err != nil {
				return err
			}
		}
		for _, ref := range refs {
			current, fact, err := r.referenceFact(ctx, tx, a, d.Call.Project, ref)
			if err != nil {
				return err
			}
			if current.Version != ref.Version {
				return domain.ErrConflict
			}
			facts = append(facts, fact)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := make([]operation.ProviderReference, 0, len(facts))
	signer := operationwire.NewProjectMediaSigner(r.Objects)
	for _, fact := range facts {
		signed, err := signer.SignProjectMedia(ctx, fact, 15*time.Minute)
		if err != nil {
			return nil, err
		}
		result = append(result, signed)
	}
	return result, nil
}

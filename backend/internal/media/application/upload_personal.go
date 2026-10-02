package application

import (
	"context"
	"path"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// PersonalUploadResult contains no synthetic project identity or private keys.
type PersonalUploadResult struct {
	Asset       LibraryAssetSummary `json:"asset"`
	DuplicateOf *uuid.UUID          `json:"duplicate_of" extensions:"x-nullable"`
}

// PersonalUploadRepository owns current identity, permanent receipts and cleanup
// serialization for the actor's real personal library.
type PersonalUploadRepository interface {
	AuthorizePersonalUpload(context.Context, identityapp.Principal) error
	FindPersonalUpload(context.Context, identityapp.Principal, UploadRequest) (PersonalUploadResult, bool, error)
	CommitPersonalUpload(context.Context, identityapp.Principal, UploadRequest, domain.MediaAsset, []domain.Rendition) (PersonalUploadResult, error)
	PersonalUploadAssetExists(context.Context, domain.PersonalOwnership, uuid.UUID) (bool, error)
}

// NewScopedUploadService extends the same pipeline through an explicit personal
// owner. The historical constructor continues to accept project uploads only.
func NewScopedUploadService(repo UploadRepository, personal PersonalUploadRepository, prober Prober, normalizer UploadNormalizer, renderer Renderer, objects UploadObjects, now func() time.Time) *UploadService {
	s := NewUploadService(repo, prober, normalizer, renderer, objects, now)
	s.personal = personal
	return s
}

// AuthorizePersonal rejects a stale identity before multipart body reading.
func (s *UploadService) AuthorizePersonal(ctx context.Context, actor identityapp.Principal) error {
	if s == nil || s.personal == nil {
		return ErrUnavailable
	}
	return s.personal.AuthorizePersonalUpload(ctx, actor)
}

// UploadPersonal constructs ownership exclusively from the authenticated actor.
// A caller cannot supply a project or select another user's personal scope.
func (s *UploadService) UploadPersonal(ctx context.Context, in UploadInput) (PersonalUploadResult, error) {
	if s == nil || s.personal == nil {
		return PersonalUploadResult{}, ErrUnavailable
	}
	if in.Request.ProjectID != uuid.Nil || in.Request.Personal != nil || in.Actor.ID == uuid.Nil || in.Actor.OrgID == uuid.Nil {
		return PersonalUploadResult{}, ErrInvalidUpload
	}
	in.Request.Personal = &domain.PersonalOwnership{OrgID: in.Actor.OrgID, ActorID: in.Actor.ID}
	destination := uploadDestination{
		authorize: func(ctx context.Context, _ UploadRequest) (string, error) {
			return "16:9", s.personal.AuthorizePersonalUpload(ctx, in.Actor)
		},
		replay: func(ctx context.Context, r UploadRequest) (uploadAccepted, bool, error) {
			result, found, err := s.personal.FindPersonalUpload(ctx, in.Actor, r)
			return uploadAccepted{ID: result.Asset.ID, Personal: result}, found, err
		},
		commit: func(ctx context.Context, r UploadRequest, a domain.MediaAsset, rends []domain.Rendition) (uploadAccepted, error) {
			result, err := s.personal.CommitPersonalUpload(ctx, in.Actor, r, a, rends)
			return uploadAccepted{ID: result.Asset.ID, Personal: result}, err
		},
		exists: func(ctx context.Context, r UploadRequest, id uuid.UUID) (bool, error) {
			return s.personal.PersonalUploadAssetExists(ctx, *r.Personal, id)
		},
	}
	result, err := s.upload(ctx, in, destination)
	return result.Personal, err
}

// PersonalUploadSummary projects actual facts without a project_id field.
func PersonalUploadSummary(a domain.MediaAsset) LibraryAssetSummary {
	return LibraryAssetSummary{ID: a.ID, Kind: string(a.Kind), FileName: a.FileName, MIMEType: a.MimeType, ByteSize: a.ByteSize, Width: a.Width, Height: a.Height, DurationMS: a.DurationMS, Revision: a.Revision}
}

type uploadAccepted struct {
	ID       uuid.UUID
	Project  UploadResult
	Personal PersonalUploadResult
}
type uploadDestination struct {
	authorize func(context.Context, UploadRequest) (string, error)
	replay    func(context.Context, UploadRequest) (uploadAccepted, bool, error)
	commit    func(context.Context, UploadRequest, domain.MediaAsset, []domain.Rendition) (uploadAccepted, error)
	exists    func(context.Context, UploadRequest, uuid.UUID) (bool, error)
}

func validUploadOwnership(r UploadRequest) bool {
	if r.Personal == nil {
		return r.ProjectID != uuid.Nil
	}
	return r.ProjectID == uuid.Nil && r.Personal.OrgID != uuid.Nil && r.Personal.ActorID != uuid.Nil
}
func uploadObjectKey(r UploadRequest, p ProbeResult, now time.Time, id uuid.UUID) string {
	if r.Personal == nil {
		return path.Join("projects", r.ProjectID.String(), string(p.Kind), now.Format("2006"), now.Format("01"), id.String()+"."+p.Extension)
	}
	return path.Join("personal", r.Personal.OrgID.String(), r.Personal.ActorID.String(), string(p.Kind), now.Format("2006"), now.Format("01"), id.String()+"."+p.Extension)
}

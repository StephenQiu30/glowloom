package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// DerivedRepository owns authorized metadata and review for local processing.
// The composition root binds it to the export command's transaction.
type DerivedRepository interface {
	AuthorizeDerivedProject(context.Context, identityapp.Principal, uuid.UUID, bool) error
	FindAsset(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.MediaAsset, error)
	FindRenditions(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) ([]domain.Rendition, error)
	StoreDerived(context.Context, identityapp.Principal, domain.MediaAsset, []domain.Rendition) error
	ReviewDerived(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, string, time.Time) error
	RejectDerived(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, string, time.Time) error
}

// DerivedService preserves the media ownership boundary for local tool jobs.
type DerivedService struct{ repo DerivedRepository }

// NewDerivedService injects a transaction-bound media repository.
func NewDerivedService(repo DerivedRepository) *DerivedService { return &DerivedService{repo: repo} }

// AuthorizeProject checks live membership and optional active-project write access.
func (s *DerivedService) AuthorizeProject(ctx context.Context, actor identityapp.Principal, project uuid.UUID, write bool) error {
	if s == nil || s.repo == nil {
		return ErrUnavailable
	}
	return s.repo.AuthorizeDerivedProject(ctx, actor, project, write)
}

// Sources returns ready, currently reviewed originals and no arbitrary object key.
func (s *DerivedService) Sources(ctx context.Context, actor identityapp.Principal, project uuid.UUID, ids []uuid.UUID) ([]domain.MediaAsset, error) {
	if s == nil || s.repo == nil || len(ids) > 1000 {
		return nil, ErrInvalidQuery
	}
	assets := make([]domain.MediaAsset, 0, len(ids))
	for _, id := range ids {
		asset, err := s.repo.FindAsset(ctx, actor, project, id)
		if err != nil {
			return nil, err
		}
		if !asset.CanReference() || asset.ContainsRealPerson || asset.ConsentRecordID != nil || asset.Kind != domain.KindImage && asset.Kind != domain.KindAudio && asset.Kind != domain.KindVideo {
			return nil, ErrNotFound
		}
		assets = append(assets, asset)
	}
	return assets, nil
}

// Store persists a checked output as processing/pending, without fake moderation.
func (s *DerivedService) Store(ctx context.Context, actor identityapp.Principal, asset domain.MediaAsset, renditions []domain.Rendition) error {
	if s == nil || s.repo == nil {
		return ErrUnavailable
	}
	if asset.Validate() != nil || asset.Origin != domain.OriginSystem || asset.Status != domain.StatusProcessing || asset.ModerationStatus != domain.ModerationPending || asset.ContainsRealPerson || (asset.Kind != domain.KindVideo && asset.Kind != domain.KindAudio) || asset.SHA256 == nil || len(*asset.SHA256) != 64 {
		return domain.ErrInvalidMediaAsset
	}
	required := []domain.RenditionKind{domain.RenditionPoster, domain.RenditionProxy720p}
	if asset.Kind == domain.KindAudio {
		if asset.Width != nil || asset.Height != nil || asset.FPS != nil || asset.DurationMS == nil || asset.AudioChannels == nil {
			return domain.ErrInvalidMediaAsset
		}
		required = []domain.RenditionKind{domain.RenditionWaveform}
	}
	if len(renditions) != len(required) {
		return domain.ErrInvalidRendition
	}
	for i, kind := range required {
		if renditions[i].Validate() != nil || renditions[i].Kind != kind || renditions[i].MediaAssetID != asset.ID {
			return domain.ErrInvalidRendition
		}
	}
	return s.repo.StoreDerived(ctx, actor, asset, renditions)
}

// Review records an explicit post-render declaration for the exact output SHA.
func (s *DerivedService) Review(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID, sha string, now time.Time) error {
	if s == nil || s.repo == nil {
		return ErrUnavailable
	}
	if id == uuid.Nil || project == uuid.Nil || len(sha) != 64 || now.IsZero() {
		return ErrInvalidQuery
	}
	return s.repo.ReviewDerived(ctx, actor, project, id, sha, now)
}

// Reject keeps an unreviewed cancelled result private and nonreferenceable.
func (s *DerivedService) Reject(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID, reason string, now time.Time) error {
	if s == nil || s.repo == nil {
		return ErrUnavailable
	}
	if id == uuid.Nil || project == uuid.Nil || reason != "export_cancelled" || now.IsZero() {
		return ErrInvalidQuery
	}
	return s.repo.RejectDerived(ctx, actor, project, id, reason, now)
}

// Asset returns scoped metadata for an output awaiting explicit owner inspection.
func (s *DerivedService) Asset(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (domain.MediaAsset, error) {
	if s == nil || s.repo == nil {
		return domain.MediaAsset{}, ErrUnavailable
	}
	return s.repo.FindAsset(ctx, actor, project, id)
}

// Renditions reads scoped private previews for their owning tool's review contract.
func (s *DerivedService) Renditions(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) ([]domain.Rendition, error) {
	if s == nil || s.repo == nil {
		return nil, ErrUnavailable
	}
	return s.repo.FindRenditions(ctx, actor, project, id)
}

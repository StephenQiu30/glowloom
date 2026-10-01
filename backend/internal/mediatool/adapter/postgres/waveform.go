package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

// PreviewWaveform rechecks the exact review revision and hash before exposing
// one private audio rendition through its owning media application port.
func (s *Store) PreviewWaveform(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID, revision int64, sha string) (mediadomain.Rendition, error) {
	var result mediadomain.Rendition
	if s == nil || s.db == nil {
		return result, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, project, false); err != nil {
			return err
		}
		row, err := readJob(tx, project, id, true)
		if err != nil {
			return err
		}
		if row.OutputKind != "audio" || row.Revision != revision || row.SHA256 == nil || *row.SHA256 != sha || row.AssetID == nil || (row.Status != "review_required" && row.Status != "succeeded") {
			return application.ErrConflict
		}
		media := s.media(tx)
		asset, err := media.Asset(ctx, actor, project, *row.AssetID)
		if err != nil {
			return normalize(err)
		}
		if asset.IsDelete || asset.Kind != mediadomain.KindAudio || asset.SHA256 == nil || *asset.SHA256 != sha || (row.Status == "succeeded") != asset.CanReference() {
			return application.ErrConflict
		}
		renditions, err := media.Renditions(ctx, actor, project, asset.ID)
		if err != nil {
			return normalize(err)
		}
		for _, rendition := range renditions {
			if rendition.Kind != mediadomain.RenditionWaveform {
				continue
			}
			if result.ID != uuid.Nil || rendition.Validate() != nil || rendition.IsDelete || rendition.MediaAssetID != asset.ID || rendition.Width == nil || rendition.Height == nil || rendition.ByteSize == nil || *rendition.ByteSize < 1 {
				return application.ErrConflict
			}
			result = rendition
		}
		if result.ID == uuid.Nil {
			return application.ErrConflict
		}
		return nil
	})
	return result, err
}

package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

// FreezeTimeline resolves only same-document nodes and authorized project media.
// When the store is constructed with an outer transaction, GORM uses a savepoint;
// the document and media locks remain held until the caller commits the export job.
func (s *Store) FreezeTimeline(ctx context.Context, actor identityapp.Principal, project, canvas, node uuid.UUID, revision int64) (domain.TimelineConfig, error) {
	var result domain.TimelineConfig
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := authorize(tx, actor, project, true); err != nil {
			return err
		}
		doc, err := load(tx, canvas, false)
		if err != nil {
			return err
		}
		if doc.ProjectID != project {
			return application.ErrNotFound
		}
		if doc.Revision != revision {
			return &application.RevisionConflict{CurrentRevision: doc.Revision}
		}
		byID := make(map[uuid.UUID]domain.Node, len(doc.Nodes))
		for _, current := range doc.Nodes {
			byID[current.ID] = current
		}
		target, found := byID[node]
		if !found {
			return application.ErrNotFound
		}
		if target.NodeType != "timeline" || target.NodeAction != "tool" || target.Config.Timeline == nil {
			return domain.ErrInvalidCommand
		}
		if err := target.Config.Timeline.Validate(); err != nil {
			return err
		}
		raw, err := json.Marshal(target.Config.Timeline)
		if err != nil {
			return fmt.Errorf("copy timeline input: %w", err)
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			return fmt.Errorf("decode timeline input: %w", err)
		}
		checked := make(map[uuid.UUID]mediaapp.AssetSummary)
		for i := range result.Clips {
			clip := &result.Clips[i]
			if clip.Kind == "text" || clip.Kind == "subtitle" {
				continue
			}
			if clip.NodeID != nil {
				reference, ok := byID[*clip.NodeID]
				if !ok || reference.NodeType != clip.Kind || reference.RefType != "media_asset" || reference.RefID == nil || clip.AssetID != nil && *clip.AssetID != *reference.RefID {
					return domain.ErrInvalidCommand
				}
				assetID := *reference.RefID
				clip.AssetID = &assetID
			}
			if clip.AssetID == nil || s.media == nil {
				return domain.ErrInvalidCommand
			}
			asset, already := checked[*clip.AssetID]
			if !already {
				var err error
				asset, err = s.media(tx).Reference(ctx, actor, project, *clip.AssetID)
				if err != nil {
					return err
				}
				checked[*clip.AssetID] = asset
			}
			if asset.Kind != clip.Kind {
				return domain.ErrInvalidCommand
			}
			if clip.Crop != nil && (asset.Width == nil || asset.Height == nil || clip.Crop.X+clip.Crop.Width > *asset.Width || clip.Crop.Y+clip.Crop.Height > *asset.Height) {
				return domain.ErrInvalidCommand
			}
		}
		return result.Validate()
	})
	return result, err
}

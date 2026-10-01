package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// FreezeTranscriptionSource retains project, document, and media read locks until
// the outer transcription transaction commits its immutable source facts.
func (s *Store) FreezeTranscriptionSource(ctx context.Context, actor identityapp.Principal, project, canvas, node uuid.UUID, revision int64) (uuid.UUID, error) {
	if s == nil || s.db == nil || s.media == nil {
		return uuid.Nil, domain.ErrInvalidCommand
	}
	var assetID uuid.UUID
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
		for _, current := range doc.Nodes {
			if current.ID != node {
				continue
			}
			if (current.NodeType != "audio" && current.NodeType != "video") || current.RefType != "media_asset" || current.RefID == nil || *current.RefID == uuid.Nil {
				return domain.ErrInvalidCommand
			}
			reader := s.media(tx)
			if reader == nil {
				return domain.ErrInvalidCommand
			}
			asset, err := reader.Reference(ctx, actor, project, *current.RefID)
			if err != nil {
				return err
			}
			if asset.ID != *current.RefID || asset.Kind != current.NodeType {
				return domain.ErrInvalidCommand
			}
			assetID = asset.ID
			return nil
		}
		return application.ErrNotFound
	})
	if err != nil {
		return uuid.Nil, err
	}
	return assetID, nil
}

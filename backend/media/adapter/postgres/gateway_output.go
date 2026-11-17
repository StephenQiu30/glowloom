package postgres

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// AcceptGatewayOutput accepts only real, verified locations in the caller's result
// transaction. The source request/project FK is independent of old operations.
func (s *Store) AcceptGatewayOutput(ctx context.Context, actor application.Actor, a domain.MediaAsset, rends []domain.Rendition) error {
	if s == nil || s.db == nil || s.db.Statement == nil {
		return application.ErrUnavailable
	}
	if _, ok := s.db.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return application.ErrUnavailable
	}
	if a.Validate() != nil || a.SourceAIRequestID == nil || a.SourceOperationID != nil || a.Origin != domain.OriginGenerated || a.SHA256 == nil || a.ObjectLocation == nil || a.Revision != 1 || a.IsDelete || a.Personal != nil || !application.CompleteOutputRenditions(a.Kind, rends) {
		return domain.ErrInvalidMediaAsset
	}
	for _, r := range rends {
		prefix := strings.TrimSuffix(a.ObjectKey, path.Ext(a.ObjectKey)) + "/"
		if r.Validate() != nil || r.MediaAssetID != a.ID || r.IsDelete || !strings.HasPrefix(r.ObjectKey, prefix) || strings.Contains(strings.TrimPrefix(r.ObjectKey, prefix), "/") {
			return domain.ErrInvalidRendition
		}
	}
	tx := s.db.WithContext(ctx)
	if err := s.requireCurrentActor(tx, actor); err != nil {
		return err
	}
	if err := s.requireProject(tx, actor, a.ProjectID, true); err != nil {
		return err
	}
	var prior assetRow
	e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", a.ID).Take(&prior).Error
	if e == nil {
		if prior.SourceAIRequestID == nil || *prior.SourceAIRequestID != *a.SourceAIRequestID || prior.ProjectID != a.ProjectID || prior.SHA256 == nil || *prior.SHA256 != *a.SHA256 || prior.ObjectKey != a.ObjectKey || prior.Status != string(a.Status) || prior.ByteSize != a.ByteSize || prior.MimeType != a.MimeType {
			return application.ErrObjectMismatch
		}
		return nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return e
	}
	if e := requireNoNewMediaPurge(tx, []uuid.UUID{a.ID}); e != nil {
		return e
	}
	if err := acceptIngestLocations(tx, a, rends); err != nil {
		return err
	}
	record := mediaAssetRecordFromDomain(a)
	if err := tx.Create(&record).Error; err != nil {
		return fmt.Errorf("insert AI media output: %w", err)
	}
	ids := []uuid.UUID{a.ObjectLocation.ID}
	for _, r := range rends {
		row := mediaRenditionRecordFromDomain(r)
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		ids = append(ids, r.ObjectLocation.ID)
	}
	intent, err := NewStorageRoutes(tx).ReadStorageWork(ctx, application.StorageWorkOwner{Kind: application.StorageWorkKindOperation, OwnerID: a.ProjectID, WorkID: *a.SourceAIRequestID})
	if err != nil {
		return err
	}
	return NewStorageRoutes(tx).AcceptStorageWork(ctx, intent.ID, ids)
}

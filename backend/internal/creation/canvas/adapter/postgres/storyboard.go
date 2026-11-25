package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func readStoryboardReceipt(tx *gorm.DB, actor, doc, key uuid.UUID, hash string) (*application.StoryboardView, error) {
	var row struct {
		DocumentID   uuid.UUID
		RequestHash  string
		ResponseBody []byte
	}
	q := tx.Raw(`SELECT document_id,request_hash,response_body FROM canvas.save_receipt WHERE actor_id=? AND idem_key=?`, actor, key).Scan(&row)
	if q.Error != nil || q.RowsAffected == 0 {
		return nil, q.Error
	}
	if row.DocumentID != doc || row.RequestHash != hash {
		return nil, domain.ErrStoryboardConflict
	}
	var view application.StoryboardView
	if err := json.Unmarshal(row.ResponseBody, &view); err != nil {
		return nil, err
	}
	if view.CanvasID != doc || view.Board == nil {
		return nil, application.ErrNotFound
	}
	return &view, nil
}

// ReadStoryboardReceipt rechecks current write authority before replaying.
func (s *Store) ReadStoryboardReceipt(ctx context.Context, actor identity.Principal, doc, key uuid.UUID, hash string) (*application.StoryboardView, error) {
	var result *application.StoryboardView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		project, err := documentProject(tx, doc)
		if err != nil {
			return err
		}
		if err := s.authorize(tx, actor, project, true); err != nil {
			return err
		}
		result, err = readStoryboardReceipt(tx, actor.ID, doc, key, hash)
		return err
	})
	return result, err
}

// CommitStoryboard accepts the binary canvas update and its command result in
// the same transaction, so reopening always returns the original stable IDs.
func (s *Store) CommitStoryboard(ctx context.Context, actor identity.Principal, prepared application.PreparedStoryboard) (application.StoryboardView, error) {
	var result application.StoryboardView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		id := prepared.View.CanvasID
		project, err := documentProject(tx, id)
		if err != nil {
			return err
		}
		if project != prepared.View.ProjectID {
			return application.ErrNotFound
		}
		if err := s.authorize(tx, actor, project, true); err != nil {
			return err
		}
		if _, err := load(tx, id, true); err != nil {
			return err
		}
		old, err := readStoryboardReceipt(tx, actor.ID, id, prepared.Key, prepared.Hash)
		if err != nil {
			return err
		}
		if old != nil {
			result = *old
			return nil
		}
		if take := prepared.Take; take != nil {
			var revision int64
			check := tx.Raw(`SELECT revision FROM media.media_asset WHERE id=? AND project_id=? AND source_ai_request_id=?
				AND kind='video' AND status='ready' AND moderation_status='passed' AND NOT is_delete FOR SHARE`,
				take.AssetID, project, take.OperationID).Scan(&revision)
			if check.Error != nil {
				return check.Error
			}
			if revision != take.AssetVersion {
				return domain.ErrStoryboardConflict
			}
		}
		bound := *s
		bound.db = tx
		receipt, err := bound.CommitYjsCommands(ctx, actor, *prepared.Command)
		if err != nil {
			return err
		}
		if receipt.Status != application.YjsReceiptStatusAccepted {
			return domain.ErrStoryboardConflict
		}
		result = prepared.View
		result.Revision = int64(receipt.ContentRevision)
		result.CommitSeq = receipt.CommitSeq
		body, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO canvas.save_receipt(actor_id,idem_key,document_id,request_hash,result_revision,response_body)
			VALUES(?,?,?,?,?,?::jsonb)`, actor.ID, prepared.Key, id, prepared.Hash, result.Revision, string(body)).Error
	})
	return result, err
}

package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// PublishHistory atomically retains the explicit public selection and batch receipt.
func (s *Store) PublishHistory(ctx context.Context, actor identity.Principal, key uuid.UUID, in application.HistoryPublicationInput) (application.HistoryPublicationReceipt, error) {
	receipt := application.HistoryPublicationReceipt{Items: []application.HistoryPublicationResult{}}
	if s == nil || s.db == nil || s.requestAccess == nil {
		return receipt, ErrUnavailable
	}
	body, err := json.Marshal(in)
	if err != nil {
		return receipt, err
	}
	digest := sha256.Sum256(body)
	hash := hex.EncodeToString(digest[:])
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The identity-bound lock serializes same-key requests before inspecting their receipt.
		lock := actor.ID.String() + ":" + actor.OrgID.String() + ":" + actor.ScopeID.String() + ":" + key.String()
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, lock).Error; err != nil {
			return err
		}
		// Recheck current authority even on idempotent replay. Stale membership never replays writes.
		projects := make(map[uuid.UUID]bool)
		for _, target := range in.Targets {
			var projectRow struct{ ProjectID uuid.UUID }
			row := tx.Raw(`SELECT o.project_id FROM operation.operation o JOIN workspace.project p ON p.id=o.project_id WHERE o.id=? AND p.org_id=? AND p.owner_scope_id=? AND NOT p.is_delete AND NOT o.is_delete`, target.OperationID, actor.OrgID, actor.ScopeID).Scan(&projectRow)
			if row.Error != nil {
				return row.Error
			}
			if row.RowsAffected == 0 {
				continue
			}
			project := projectRow.ProjectID
			if !projects[project] {
				if err := s.requirePublicProject(tx, actor, project, true); err != nil {
					return err
				}
				projects[project] = true
			}
		}
		var saved struct {
			RequestHash string
			Response    json.RawMessage
		}
		read := tx.Raw(`SELECT request_hash,response FROM operation.history_publication_receipt WHERE actor_id=? AND org_id=? AND scope_id=? AND request_key=?`, actor.ID, actor.OrgID, actor.ScopeID, key).Scan(&saved)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected > 0 {
			if saved.RequestHash != hash {
				return application.ErrPublicationConflict
			}
			return json.Unmarshal(saved.Response, &receipt)
		}
		for _, target := range in.Targets {
			result, err := s.publishHistoryTarget(tx, actor, target, in)
			if err != nil {
				return err
			}
			receipt.Items = append(receipt.Items, result)
		}
		resultJSON, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO operation.history_publication_receipt(actor_id,org_id,scope_id,request_key,request_hash,response) VALUES(?,?,?,?,?,?::jsonb)`, actor.ID, actor.OrgID, actor.ScopeID, key, hash, string(resultJSON)).Error
	})
	return receipt, err
}

func (s *Store) publishHistoryTarget(tx *gorm.DB, actor identity.Principal, target application.HistoryPublicationTarget, in application.HistoryPublicationInput) (application.HistoryPublicationResult, error) {
	result := application.HistoryPublicationResult{OperationID: target.OperationID, OutputID: target.OutputID, Status: "failed", ErrorCode: "not_publishable"}
	var candidate struct {
		ProjectID, AssetID uuid.UUID
		Kind, Prompt       string
	}
	query := personalOperationIDs + `SELECT op.project_id,asset.id AS asset_id,asset.kind,
 COALESCE((SELECT i.text_value FROM operation.operation_input i WHERE i.operation_id=op.id AND i.role='prompt' AND NOT i.is_delete ORDER BY i.seq_no LIMIT 1),'') AS prompt
 FROM operation.operation op JOIN personal_operation_ids initiated ON initiated.operation_id=op.id::text
 JOIN workspace.project p ON p.id=op.project_id AND p.org_id=? AND p.owner_scope_id=? AND p.status='active' AND NOT p.is_delete
 JOIN operation.operation_output output ON output.operation_id=op.id AND output.project_id=op.project_id
 JOIN media.media_asset asset ON asset.id=output.media_asset_id AND asset.project_id=op.project_id
 WHERE op.id=? AND output.id=? AND NOT op.is_delete AND NOT output.is_delete AND NOT asset.is_delete
 AND op.status='completed' AND output.moderation_status='passed' AND asset.moderation_status='passed' AND asset.status='ready'
 AND asset.kind IN ('image','video') AND op.capability<>'video.dubbing' AND op.mode NOT IN ('video_dubbing','dubbing')

 FOR SHARE OF op,output,asset,p`
	args := []any{actor.ID, actor.OrgID, actor.ID, actor.OrgID, actor.ID, actor.OrgID, actor.OrgID, actor.ScopeID, target.OperationID, target.OutputID}
	read := tx.Raw(query, args...).Scan(&candidate)
	if read.Error != nil {
		return result, read.Error
	}
	if read.RowsAffected == 0 {
		return result, nil
	}
	if err := s.requirePublicProject(tx, actor, candidate.ProjectID, true); err != nil {
		return result, err
	}
	var prompt *string
	if in.PublishPrompt {
		prompt = &candidate.Prompt
	}
	id := uuid.New()
	// A second request for the same candidate returns its existing public work.
	if err := tx.Exec(`INSERT INTO operation.published_work(id,actor_id,org_id,scope_id,project_id,operation_id,output_id,media_asset_id,title,description,prompt) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(actor_id,output_id) DO NOTHING`, id, actor.ID, actor.OrgID, actor.ScopeID, candidate.ProjectID, target.OperationID, target.OutputID, candidate.AssetID, in.Title, in.Description, prompt).Error; err != nil {
		return result, err
	}
	var existing struct{ ID uuid.UUID }
	read = tx.Raw(`SELECT id FROM operation.published_work WHERE actor_id=? AND output_id=? AND withdrawn_at IS NULL`, actor.ID, target.OutputID).Scan(&existing)
	if read.Error != nil {
		return result, read.Error
	}
	if read.RowsAffected == 0 {
		return result, nil
	}
	result.Status = "published"
	result.WorkID = &existing.ID
	result.URL = "/works/" + existing.ID.String()
	result.ErrorCode = ""
	return result, nil
}

// FindPublishedWork projects only a live public work with currently eligible bytes.
func (s *Store) FindPublishedWork(ctx context.Context, id uuid.UUID) (application.PublishedWorkFile, error) {
	var file application.PublishedWorkFile
	if s == nil || s.db == nil {
		return file, ErrUnavailable
	}
	var row struct {
		application.PublishedWork    `gorm:"embedded"`
		AssetID, LocationID          uuid.UUID
		ProfileID, Bucket, ObjectKey string
		Revision                     int64
	}
	read := s.db.WithContext(ctx).Raw(`SELECT w.id,w.title,w.description,w.prompt,w.created_at,a.kind,a.id AS asset_id,a.revision,a.object_key,
 COALESCE(location.id,'00000000-0000-0000-0000-000000000000'::uuid) AS location_id,COALESCE(location.profile_id,'') AS profile_id,COALESCE(location.bucket,'') AS bucket
 FROM operation.published_work w JOIN workspace.project p ON p.id=w.project_id AND NOT p.is_delete
 JOIN operation.operation op ON op.id=w.operation_id AND op.project_id=w.project_id AND NOT op.is_delete
 JOIN operation.operation_output output ON output.id=w.output_id AND output.operation_id=op.id AND output.project_id=p.id AND NOT output.is_delete AND output.moderation_status='passed'
 JOIN media.media_asset a ON a.id=w.media_asset_id AND a.project_id=p.id AND output.media_asset_id=a.id AND NOT a.is_delete AND a.status='ready' AND a.moderation_status='passed'
 LEFT JOIN media.object_location location ON location.id=a.object_location_id
 WHERE w.id=? AND w.withdrawn_at IS NULL AND a.kind IN ('image','video')
 AND (a.object_location_id IS NULL OR (location.object_key=a.object_key AND location.state IN ('accepted','present')))
 `, id).Scan(&row)
	if read.Error != nil {
		return file, fmt.Errorf("read public work: %w", read.Error)
	}
	if read.RowsAffected != 1 || strings.TrimSpace(row.Title) == "" {
		return file, application.ErrPublicNotFound
	}
	file = application.PublishedWorkFile{Work: row.PublishedWork, AssetID: row.AssetID, LocationID: row.LocationID, ProfileID: row.ProfileID, Bucket: row.Bucket, ObjectKey: row.ObjectKey, Revision: row.Revision}
	return file, nil
}

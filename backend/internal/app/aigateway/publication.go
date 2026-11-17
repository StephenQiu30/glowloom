package aigateway

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	op "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

// PublishHistory freezes only the selected, currently owned formal image/video result.
func (h History) PublishHistory(ctx context.Context, a identity.Principal, key uuid.UUID, in op.HistoryPublicationInput) (op.HistoryPublicationReceipt, error) {
	out := op.HistoryPublicationReceipt{Items: []op.HistoryPublicationResult{}}
	if key == uuid.Nil {
		return out, op.ErrInvalidPublicQuery
	}
	command := uuid.NewSHA1(key, []byte("ai/publication"))
	hash := domain.Digest(canonical(struct {
		Scope uuid.UUID
		Input op.HistoryPublicationInput
	}{a.ScopeID, in}))
	err := h.Creation.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := h.Creation.auth()(ctx, tx, a, a.ScopeID, uuid.Nil, false); err != nil {
			return err
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", a.ID.String()+command.String()).Error; err != nil {
			return err
		}
		var prior struct {
			InputHash string
			Response  []byte
		}
		q := tx.Raw("SELECT input_hash,response FROM ai.project_command WHERE actor_id=? AND command_key=?", a.ID, command).Scan(&prior)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 1 {
			if prior.InputHash != hash {
				return op.ErrPublicationConflict
			}
			return json.Unmarshal(prior.Response, &out)
		}
		for _, target := range in.Targets {
			result := op.HistoryPublicationResult{OperationID: target.OperationID, OutputID: target.OutputID, Status: "failed", ErrorCode: "not_publishable"}
			task, err := h.task(ctx, tx, a, target.OperationID)
			if errors.Is(err, op.ErrPublicNotFound) {
				out.Items = append(out.Items, result)
				continue
			}
			if err != nil {
				return err
			}
			if err := h.Creation.auth()(ctx, tx, a, a.ScopeID, task.ProjectID, false); err != nil {
				return err
			}
			var asset uuid.UUID
			for _, output := range task.Outputs {
				if output.ID == target.OutputID && task.Status == "completed" && output.ModerationStatus == "passed" && (output.Kind == "image" || output.Kind == "video") && output.MediaAssetID != nil {
					asset = *output.MediaAssetID
				}
			}
			if asset == uuid.Nil {
				out.Items = append(out.Items, result)
				continue
			}
			var present int
			if err := tx.Raw("SELECT 1 FROM media.media_asset WHERE id=? AND project_id=? AND source_ai_request_id=? AND NOT is_delete AND status='ready' AND moderation_status='passed' FOR SHARE", asset, task.ProjectID, task.ID).Scan(&present).Error; err != nil {
				return err
			}
			if present != 1 {
				out.Items = append(out.Items, result)
				continue
			}
			var prompt *string
			if in.PublishPrompt {
				for _, input := range task.Inputs {
					if input.Role == "prompt" {
						prompt = input.Text
					}
				}
			}
			id := uuid.New()
			if err := tx.Exec("INSERT INTO ai.work(id,actor_id,scope_id,project_id,request_id,asset_id,title,description,prompt) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(actor_id,request_id) DO NOTHING", id, a.ID, a.ScopeID, task.ProjectID, task.ID, asset, in.Title, in.Description, prompt).Error; err != nil {
				return err
			}
			if err := tx.Raw("SELECT id FROM ai.work WHERE actor_id=? AND request_id=?", a.ID, task.ID).Row().Scan(&id); err != nil {
				return err
			}
			result.Status = "published"
			result.WorkID = &id
			result.URL = "/works/" + id.String()
			result.ErrorCode = ""
			out.Items = append(out.Items, result)
		}
		return tx.Exec("INSERT INTO ai.project_command(actor_id,command_key,input_hash,response) VALUES(?,?,?,?::jsonb)", a.ID, command, hash, string(canonical(out))).Error
	})
	return out, err
}

// FindPublishedWork rechecks current project, physical bytes and moderation before signing.
func (h History) FindPublishedWork(ctx context.Context, id uuid.UUID) (op.PublishedWorkFile, error) {
	var row struct {
		op.PublishedWork             `gorm:"embedded"`
		AssetID, LocationID          uuid.UUID
		ProfileID, Bucket, ObjectKey string
		Revision                     int64
	}
	q := h.Creation.Runtime.DB.WithContext(ctx).Raw(`SELECT w.id,w.title,w.description,w.prompt,w.created_at,a.kind,a.id AS asset_id,a.revision,a.object_key,
 l.id AS location_id,l.profile_id,l.bucket FROM ai.work w JOIN workspace.project p ON p.id=w.project_id AND NOT p.is_delete
 JOIN ai.request r ON r.id=w.request_id AND r.project_id=p.id AND r.state='settled'
 JOIN ai.output o ON o.request_id=r.id AND o.asset_id=w.asset_id AND o.state='ready'
 JOIN media.media_asset a ON a.id=w.asset_id AND a.project_id=p.id AND a.source_ai_request_id=r.id AND NOT a.is_delete AND a.status='ready' AND a.moderation_status='passed' AND a.kind IN ('image','video')
 JOIN media.object_location l ON l.id=a.object_location_id AND l.object_key=a.object_key AND l.state IN ('accepted','present')
 WHERE w.id=?`, id).Scan(&row)
	if q.Error != nil {
		return op.PublishedWorkFile{}, q.Error
	}
	if q.RowsAffected != 1 {
		return op.PublishedWorkFile{}, op.ErrPublicNotFound
	}
	return op.PublishedWorkFile{Work: row.PublishedWork, AssetID: row.AssetID, LocationID: row.LocationID, ProfileID: row.ProfileID, Bucket: row.Bucket, ObjectKey: row.ObjectKey, Revision: row.Revision}, nil
}

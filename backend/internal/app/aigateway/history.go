package aigateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	op "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// History projects caller-owned new requests and independent personal metadata.
type History struct{ Creation Creation }

func (h History) readAuthority(ctx context.Context, tx *gorm.DB, a identity.Principal, sid, pid uuid.UUID, admin bool) error {
	if err := h.Creation.auth()(ctx, tx, a, sid, uuid.Nil, admin); err != nil {
		return err
	}
	if pid == uuid.Nil {
		return nil
	}
	var n int
	q := tx.Raw("SELECT 1 FROM workspace.project WHERE id=? AND owner_scope_id=? AND org_id=? AND NOT is_delete AND status IN ('active','archived') FOR SHARE", pid, sid, a.OrgID).Scan(&n)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return domain.ErrForbidden
	}
	return nil
}
func (h History) task(ctx context.Context, tx *gorm.DB, a identity.Principal, id uuid.UUID) (op.TaskDetail, error) {
	var row struct{ ProjectID uuid.UUID }
	q := tx.Raw("SELECT project_id FROM ai.creation WHERE id=? AND scope_id=? AND actor_id=?", id, a.ScopeID, a.ID).Scan(&row)
	if q.Error != nil {
		return op.TaskDetail{}, q.Error
	}
	if q.RowsAffected != 1 {
		return op.TaskDetail{}, op.ErrPublicNotFound
	}
	runtime := *h.Creation.Runtime
	runtime.DB = tx
	c := h.Creation
	c.Runtime = &runtime
	c.Authorize = h.readAuthority
	return c.Task(ctx, a, row.ProjectID, id)
}

// ListPersonalHistory retains stable keyset browsing over the rebuilt calls only.
func (h History) ListPersonalHistory(ctx context.Context, a identity.Principal, in op.PersonalHistoryInput) (op.PersonalHistoryPage, error) {
	out := op.PersonalHistoryPage{Items: []op.PersonalHistoryItem{}}
	if err := in.Normalize(); err != nil {
		return out, err
	}
	err := h.Creation.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := h.readAuthority(ctx, tx, a, a.ScopeID, uuid.Nil, false); err != nil {
			return err
		}
		query := `SELECT c.id FROM ai.creation c JOIN ai.request r ON r.id=c.id JOIN workspace.project p ON p.id=c.project_id
 LEFT JOIN ai.history m ON m.request_id=c.id AND m.actor_id=c.actor_id
 WHERE c.actor_id=? AND c.scope_id=? AND p.org_id=? AND p.owner_scope_id=? AND NOT p.is_delete AND p.status IN ('active','archived') AND m.deleted_at IS NULL`
		args := []any{a.ID, a.ScopeID, a.OrgID, a.ScopeID}
		capability := `COALESCE(r.contract->>'capability','text.generate')`
		if in.Capability != "" {
			query += " AND " + capability + "=?"
			args = append(args, in.Capability)
		}
		if in.Kind != "" {
			prefix := in.Kind
			if prefix == "document" {
				prefix = "text"
			}
			query += " AND " + capability + " LIKE ?"
			args = append(args, prefix+".%")
		}
		if in.Search != "" {
			query += " AND strpos(lower(concat_ws(' ',r.contract->>'name',c.presentation->>'final_prompt')),lower(?))>0"
			args = append(args, in.Search)
		}
		if in.FavoriteOnly {
			query += " AND COALESCE(m.favorite,false)"
		}
		if in.CreatedFrom != nil {
			query += " AND c.created_at>=?"
			args = append(args, *in.CreatedFrom)
		}
		if in.CreatedBefore != nil {
			query += " AND c.created_at<?"
			args = append(args, *in.CreatedBefore)
		}
		for _, f := range []struct {
			values []string
			field  string
		}{{in.Resolutions, "resolution"}, {in.Ratios, "ratio"}, {in.GenerationModes, "mode"}} {
			if len(f.values) > 0 {
				if f.field == "mode" {
					query += " AND COALESCE(q.contract->'Raw'->>'mode','text_to_text') IN ?"
				} else {
					query += " AND COALESCE(q.contract->'Raw'->'params'->>?,q.contract->'Raw'->'params'->>'aspect_ratio','') IN ?"
					args = append(args, f.field)
				}
				args = append(args, f.values)
			}
		}
		query = strings.Replace(query, "JOIN ai.request r ON r.id=c.id", "JOIN ai.request r ON r.id=c.id JOIN ai.quote q ON q.id=c.id", 1)
		if len(in.ProcessTypes) > 0 {
			query += " AND false"
		} // Processing is not a generation mode in this MVP.
		direction, comparison := "DESC", "<"
		if in.Order == "asc" {
			direction, comparison = "ASC", ">"
		}
		if in.After != nil {
			query += " AND (c.created_at,c.id)" + comparison + "(?,?::uuid)"
			args = append(args, in.After.CreateTime, in.After.ID)
		}
		query += " ORDER BY c.created_at " + direction + ",c.id " + direction + " LIMIT ?"
		args = append(args, in.Limit+1)
		var ids []uuid.UUID
		if err := tx.Raw(query, args...).Scan(&ids).Error; err != nil {
			return err
		}
		more := len(ids) > in.Limit
		if more {
			ids = ids[:in.Limit]
		}
		for _, id := range ids {
			task, err := h.task(ctx, tx, a, id)
			if err != nil {
				return err
			}
			var meta struct {
				Revision int64
				Favorite bool
			}
			if err := tx.Raw("SELECT revision,favorite FROM ai.history WHERE request_id=? AND actor_id=?", id, a.ID).Scan(&meta).Error; err != nil {
				return err
			}
			item := op.PersonalHistoryItem{TaskSummary: task.TaskSummary, HistoryRevision: meta.Revision, Outputs: []op.PersonalHistoryOutput{}}
			for _, input := range task.Inputs {
				if input.Role == "prompt" && input.Text != nil {
					r := []rune(*input.Text)
					item.PromptPreview = string(r[:min(240, len(r))])
				}
			}
			for _, output := range task.Outputs {
				ready := task.Status == "completed" && (output.ModerationStatus == "passed" || (output.Kind == "json" && output.ModerationStatus == "skipped" && output.OutputPolicy == "unreviewed_text_v1" && output.Text != nil))
				item.Outputs = append(item.Outputs, op.PersonalHistoryOutput{TaskOutput: output, Favorite: meta.Favorite, MetadataRevision: meta.Revision, ProcessTypes: []string{}, CanDownload: ready, CanPublish: ready && (output.Kind == "image" || output.Kind == "video")})
			}
			out.Items = append(out.Items, item)
		}
		if more && len(out.Items) > 0 {
			last := out.Items[len(out.Items)-1]
			out.Next = &op.TaskCursor{ID: last.ID, CreateTime: last.CreateTime}
		}
		return nil
	})
	return out, err
}

// ApplyPersonalHistoryCommand records a permanent partial-result receipt for one-output MVP metadata.
func (h History) ApplyPersonalHistoryCommand(ctx context.Context, a identity.Principal, key uuid.UUID, in op.PersonalHistoryCommand) (op.PersonalHistoryCommandReceipt, error) {
	out := op.PersonalHistoryCommandReceipt{RequestID: key, Action: in.Action, Results: []op.PersonalHistoryCommandResult{}}
	if err := in.Validate(key); err != nil {
		return out, err
	}
	hash := domain.Digest(canonical(struct {
		Scope uuid.UUID
		Input op.PersonalHistoryCommand
	}{a.ScopeID, in}))
	command := uuid.NewSHA1(key, []byte("ai/history"))
	err := h.Creation.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := h.readAuthority(ctx, tx, a, a.ScopeID, uuid.Nil, false); err != nil {
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
				return op.ErrHistoryCommandConflict
			}
			return json.Unmarshal(prior.Response, &out)
		}
		for _, target := range in.Targets {
			result := op.PersonalHistoryCommandResult{OperationID: target.OperationID, OutputID: target.OutputID, Code: "not_found", AffectedOutputIDs: []uuid.UUID{}}
			task, err := h.task(ctx, tx, a, target.OperationID)
			if errors.Is(err, op.ErrPublicNotFound) {
				out.Results = append(out.Results, result)
				continue
			}
			if err != nil {
				return err
			}
			oid := uuid.NewSHA1(task.ID, []byte("output/0"))
			if target.OutputID != nil && (*target.OutputID != oid || len(task.Outputs) == 0) {
				out.Results = append(out.Results, result)
				continue
			}
			if err := tx.Exec("INSERT INTO ai.history(request_id,actor_id) VALUES(?,?) ON CONFLICT DO NOTHING", task.ID, a.ID).Error; err != nil {
				return err
			}
			var meta struct {
				Revision  int64
				DeletedAt *time.Time
			}
			if err := tx.Raw("SELECT revision,deleted_at FROM ai.history WHERE request_id=? AND actor_id=? FOR UPDATE", task.ID, a.ID).Scan(&meta).Error; err != nil {
				return err
			}
			result.Revision = meta.Revision
			if meta.Revision != target.ExpectedRevision || meta.DeletedAt != nil {
				result.Code = "revision_conflict"
				out.Results = append(out.Results, result)
				continue
			}
			query := "UPDATE ai.history SET revision=revision+1,favorite=? WHERE request_id=? AND actor_id=?"
			value := any(in.Favorited)
			if in.Action == "delete" {
				query = "UPDATE ai.history SET revision=revision+1,deleted_at=? WHERE request_id=? AND actor_id=?"
				value = time.Now()
			}
			if err := tx.Exec(query, value, task.ID, a.ID).Error; err != nil {
				return err
			}
			result.OK = true
			result.Code = "ok"
			result.Revision++
			if len(task.Outputs) > 0 {
				result.AffectedOutputIDs = []uuid.UUID{oid}
			}
			out.Results = append(out.Results, result)
		}
		return tx.Exec("INSERT INTO ai.project_command(actor_id,command_key,input_hash,response) VALUES(?,?,?,?::jsonb)", a.ID, command, hash, string(canonical(out))).Error
	})
	return out, err
}

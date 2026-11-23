package aigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/http"
	operation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

func (c Creation) batchIDs(ctx context.Context, tx *gorm.DB, a identity.Principal, p, id uuid.UUID) ([]uuid.UUID, error) {
	if err := c.auth()(ctx, tx, a, a.ScopeID, p, false); err != nil {
		return nil, err
	}
	var row struct{ Quotes []byte }
	q := tx.Raw("SELECT quotes FROM ai.batch WHERE id=? AND scope_id=? AND project_id=? AND actor_id=?", id, a.ScopeID, p, a.ID).Scan(&row)
	if q.Error != nil {
		return nil, q.Error
	}
	if q.RowsAffected != 1 {
		return nil, domain.ErrForbidden
	}
	var ids []uuid.UUID
	if json.Unmarshal(row.Quotes, &ids) != nil {
		return nil, domain.ErrInvalid
	}
	return ids, nil
}

// BatchQuote creates bounded, ordered, independent requests in one source transaction.
func (c Creation) BatchQuote(ctx context.Context, a identity.Principal, p uuid.UUID, key string, items []operationhttp.QuoteItemRequest) (operation.CreateBatchFreeQuoteResult, error) {
	var out operation.CreateBatchFreeQuoteResult
	if len(items) < 1 || len(items) > 20 || key == "" || len(key) > 80 {
		return out, domain.ErrInvalid
	}
	hash := domain.Digest(canonical(items))
	err := c.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := c.auth()(ctx, tx, a, a.ScopeID, p, false); err != nil {
			return err
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", a.ScopeID.String()+"/batch/"+key).Error; err != nil {
			return err
		}
		var prior struct {
			ActorID, ProjectID uuid.UUID
			InputHash          string
			Response           []byte
		}
		q := tx.Raw("SELECT actor_id,project_id,input_hash,response FROM ai.batch WHERE scope_id=? AND command_key=?", a.ScopeID, key).Scan(&prior)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 1 {
			if prior.InputHash != hash || prior.ActorID != a.ID || prior.ProjectID != p {
				return domain.ErrConflict
			}
			return json.Unmarshal(prior.Response, &out)
		}
		runtime := *c.Runtime
		runtime.DB = tx
		inner := c
		inner.Runtime = &runtime
		id := uuid.New()
		out.BatchID = &id
		out.Confirmable = true
		ids := []uuid.UUID{}
		for i, item := range items {
			if item.InputSource != nil || (item.Source != nil && item.Source.ShotID != nil) {
				return domain.ErrInvalid
			}
			quote, err := inner.Quote(ctx, a, p, fmt.Sprintf("%s/%d", key, i), item)
			if err != nil {
				return err
			}
			ids = append(ids, quote.OperationID)
			amount := quote.QuoteMicros
			oid := quote.OperationID
			out.Items = append(out.Items, operation.BatchFreeQuoteItemResult{OperationID: &oid, ModelKey: item.ModelKey, Mode: item.Mode, QuoteMicros: &amount, QuoteDetail: quote.QuoteDetail, FinalPrompt: quote.FinalPrompt})
			out.TotalMicros += amount
			out.AvailableMicros = quote.AvailableMicros
			if out.ExpiresAt.IsZero() || quote.ExpiresAt.Before(out.ExpiresAt) {
				out.ExpiresAt = quote.ExpiresAt
			}
		}
		out.Confirmable = out.TotalMicros <= out.AvailableMicros
		return tx.Exec("INSERT INTO ai.batch(id,scope_id,actor_id,project_id,command_key,input_hash,quotes,response) VALUES(?,?,?,?,?,?,?::jsonb,?::jsonb)", id, a.ScopeID, a.ID, p, key, hash, string(canonical(ids)), string(canonical(out))).Error
	})
	return out, err
}
func (c Creation) registerBatches(g *gin.RouterGroup) {
	g.POST("/projects/:pid/quotes", func(ctx *gin.Context) {
		p, ok := httpapi.PathUUID(ctx, "pid")
		if !ok {
			return
		}
		var in struct {
			Items []operationhttp.QuoteItemRequest `json:"items"`
		}
		if !httpapi.Decode(ctx, &in) {
			return
		}
		out, err := c.BatchQuote(ctx.Request.Context(), identityhttp.Principal(ctx), p, ctx.GetHeader("Idempotency-Key"), in.Items)
		if err != nil {
			creationProblem(ctx, err)
			return
		}
		ctx.JSON(201, out)
	})
	g.GET("/batches/:id", func(ctx *gin.Context) {
		id, ok := httpapi.PathUUID(ctx, "id")
		if !ok {
			return
		}
		p, err := uuid.Parse(ctx.Query("project_id"))
		if err != nil {
			creationProblem(ctx, domain.ErrInvalid)
			return
		}
		a := identityhttp.Principal(ctx)
		ids, err := c.batchIDs(ctx.Request.Context(), c.Runtime.DB, a, p, id)
		if err != nil {
			creationProblem(ctx, err)
			return
		}
		items := []operation.TaskSummary{}
		total := int64(0)
		completed, unknown, failed := 0, 0, 0
		for _, id := range ids {
			task, err := c.Task(ctx.Request.Context(), a, p, id)
			if err != nil {
				creationProblem(ctx, err)
				return
			}
			items = append(items, task.TaskSummary)
			if task.QuoteMicros != nil {
				total += *task.QuoteMicros
			}
			switch task.Status {
			case "completed":
				completed++
			case "unknown":
				unknown++
			case "failed", "cancelled":
				failed++
			}
		}
		state := "running"
		if completed+failed == len(ids) {
			state = "completed"
		}
		if unknown > 0 {
			state = "unknown"
		}
		ctx.JSON(200, gin.H{"id": id, "project_id": p, "kind": "creation", "status": state, "total_count": len(ids), "succeeded_count": completed, "failed_count": failed, "unknown_count": unknown, "quote_total_micros": total, "items": items})
	})
	g.POST("/batches/:id/confirm", func(ctx *gin.Context) {
		id, ok := httpapi.PathUUID(ctx, "id")
		if !ok {
			return
		}
		var in struct {
			ProjectID uuid.UUID   `json:"project_id"`
			Exclude   []uuid.UUID `json:"exclude_operation_ids"`
		}
		if !httpapi.Decode(ctx, &in) {
			return
		}
		response, err := c.ConfirmBatch(ctx.Request.Context(), identityhttp.Principal(ctx), in.ProjectID, id, ctx.GetHeader("Idempotency-Key"), in.Exclude)
		if err != nil {
			creationProblem(ctx, err)
			return
		}
		ctx.Data(200, "application/json", response)
	})
	for _, action := range []string{"cancel", "resume"} {
		g.POST("/batches/:id/"+action, func(ctx *gin.Context) {
			id, ok := httpapi.PathUUID(ctx, "id")
			if !ok {
				return
			}
			var in struct {
				ProjectID uuid.UUID `json:"project_id"`
			}
			if !httpapi.Decode(ctx, &in) {
				return
			}
			out, err := c.Control(ctx.Request.Context(), identityhttp.Principal(ctx), in.ProjectID, id, "batch", action, ctx.GetHeader("Idempotency-Key"))
			if err != nil {
				creationProblem(ctx, err)
				return
			}
			ctx.JSON(202, out)
		})
	}
}

// ConfirmBatch freezes one immutable selection and one atomic confirmation result.
func (c Creation) ConfirmBatch(ctx context.Context, a identity.Principal, p, id uuid.UUID, key string, exclude []uuid.UUID) (json.RawMessage, error) {
	if key == "" || len(key) > 120 {
		return nil, domain.ErrInvalid
	}
	var out json.RawMessage
	err := c.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ids, err := c.batchIDs(ctx, tx, a, p, id)
		if err != nil {
			return err
		}
		if err = tx.Exec("SELECT id FROM ai.batch WHERE id=? FOR UPDATE", id).Error; err != nil {
			return err
		}
		known := map[uuid.UUID]bool{}
		for _, oid := range ids {
			known[oid] = true
		}
		excluded := map[uuid.UUID]bool{}
		for _, oid := range exclude {
			if !known[oid] || excluded[oid] {
				return domain.ErrInvalid
			}
			excluded[oid] = true
		}
		ordered := append([]uuid.UUID{}, exclude...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].String() < ordered[j].String() })
		hash := domain.Digest(canonical(ordered))
		var prior struct {
			ActorID               uuid.UUID
			CommandKey, InputHash string
			Response              []byte
		}
		q := tx.Raw("SELECT actor_id,command_key,input_hash,response FROM ai.batch_decision WHERE batch_id=?", id).Scan(&prior)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 1 {
			if prior.ActorID != a.ID || prior.CommandKey != key || prior.InputHash != hash {
				return domain.ErrConflict
			}
			out = prior.Response
			return nil
		}
		runtime := *c.Runtime
		runtime.DB = tx
		inner := c
		inner.Runtime = &runtime
		confirmed, amount := 0, int64(0)
		items := []gin.H{}
		for _, oid := range ids {
			if excluded[oid] {
				continue
			}
			record, err := inner.Confirm(ctx, a, p, oid, key)
			if err != nil {
				return err
			}
			confirmed++
			amount += record.Held * 100000
			items = append(items, gin.H{"operation_id": oid, "status": "accepted", "reasons": []string{}})
		}
		wallet, err := c.store(tx, a).Wallet(ctx, a.ScopeID)
		if err != nil {
			return err
		}
		available, _ := wallet["available"].(int64)
		out = canonical(gin.H{"items": items, "confirmed_count": confirmed, "quote_total_micros": amount, "available_micros": available * 100000, "budget_revision": wallet["version"]})
		return tx.Exec("INSERT INTO ai.batch_decision(batch_id,actor_id,command_key,input_hash,response) VALUES(?,?,?,?,?::jsonb)", id, a.ID, key, hash, string(out)).Error
	})
	return out, err
}

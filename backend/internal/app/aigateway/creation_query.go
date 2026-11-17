package aigateway

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	promptdomain "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/execution/operation/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
)

func submission(row creationRow) (operation.Submission, error) {
	out := operation.Submission{ID: row.ID, OperationID: row.ID, Epoch: row.Epoch, ConfirmKey: row.ConfirmKey, State: row.State}
	var quote operation.CreateFreeQuoteResult
	if json.Unmarshal(row.Source, &out.InputSource) != nil || json.Unmarshal(row.Draft, &out.Draft) != nil || json.Unmarshal(row.Presentation, &quote) != nil {
		return out, domain.ErrInvalid
	}
	out.Quote = &operation.EditorQuote{QuoteDetail: quote.QuoteDetail, CompilerVersion: quote.CompilerVersion, ReferenceMapping: quote.ReferenceMapping, QuoteMicros: quote.QuoteMicros, AvailableMicros: quote.AvailableMicros, FinalPrompt: quote.FinalPrompt, ExpiresAt: quote.ExpiresAt, Confirmable: quote.Confirmable}
	return out, nil
}

// CurrentSubmission recovers editor intent from the rebuilt source slot.
func (c Creation) CurrentSubmission(ctx context.Context, a identity.Principal, p uuid.UUID, source promptdomain.InputSource) (operation.SubmissionCurrent, error) {
	var out operation.SubmissionCurrent
	err := c.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := c.auth()(ctx, tx, a, a.ScopeID, p, false); err != nil {
			return err
		}
		if err := operationwire.ProvidePromptEditor(tx).AuthorizeOwner(ctx, a, p, source); err != nil {
			return err
		}
		var slot struct {
			Epoch      int64
			CreationID *uuid.UUID
		}
		if err := tx.Raw("SELECT epoch,creation_id FROM ai.creation_slot WHERE actor_id=? AND project_id=? AND owner_key=?", a.ID, p, source.OwnerKey()).Scan(&slot).Error; err != nil {
			return err
		}
		out.Epoch = slot.Epoch
		if slot.CreationID == nil {
			return nil
		}
		row, err := c.row(tx, a, p, *slot.CreationID)
		if err != nil {
			return err
		}
		s, err := submission(row)
		out.Submission = &s
		return err
	})
	return out, err
}

// ResolveSubmission closes a quoted or acknowledged intent; accepted unknown costs stay protected.
func (c Creation) ResolveSubmission(ctx context.Context, a identity.Principal, in operation.SubmissionResolution) (operation.SubmissionCurrent, error) {
	var out operation.SubmissionCurrent
	if in.Key == uuid.Nil || in.Action != "discard_quote" && in.Action != "acknowledge_result" {
		return out, domain.ErrInvalid
	}
	err := c.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := c.auth()(ctx, tx, a, a.ScopeID, in.ProjectID, false); err != nil {
			return err
		}
		if err := tx.Exec("SELECT id FROM ai.creation WHERE id=? FOR UPDATE", in.ID).Error; err != nil {
			return err
		}
		row, err := c.row(tx, a, in.ProjectID, in.ID)
		if err != nil {
			return err
		}
		if row.ActorID != a.ID || row.OwnerKey == nil {
			return domain.ErrForbidden
		}
		var slot struct {
			Epoch      int64
			CreationID *uuid.UUID
		}
		if err := tx.Raw("SELECT epoch,creation_id FROM ai.creation_slot WHERE actor_id=? AND project_id=? AND owner_key=? FOR UPDATE", a.ID, in.ProjectID, *row.OwnerKey).Scan(&slot).Error; err != nil {
			return err
		}
		if row.State == "closed" {
			out.Epoch = slot.Epoch
			return nil
		}
		if slot.Epoch != in.ExpectedEpoch || slot.CreationID == nil || *slot.CreationID != row.ID {
			return operation.ErrSubmissionEpoch
		}
		if row.State == "accepted" && in.Action == "acknowledge_result" {
			record, err := c.store(tx, a).Read(ctx, a.ScopeID, row.ID)
			if err != nil {
				return err
			}
			if record.State != "settled" && record.State != "released" {
				return domain.ErrUnknown
			}
		}
		if row.State == "accepted" && in.Action != "acknowledge_result" {
			return operation.ErrSubmissionAccepted
		}
		if row.State == "quoted" && in.Action != "discard_quote" {
			return domain.ErrConflict
		}
		if err := tx.Exec("UPDATE ai.creation SET state='closed' WHERE id=?", row.ID).Error; err != nil {
			return err
		}
		out.Epoch = slot.Epoch + 1
		return tx.Exec("UPDATE ai.creation_slot SET epoch=?,creation_id=NULL WHERE actor_id=? AND project_id=? AND owner_key=?", out.Epoch, a.ID, in.ProjectID, *row.OwnerKey).Error
	})
	return out, err
}

// Task projects outputs and billing exclusively from the new request/hold/receipt.
func (c Creation) Task(ctx context.Context, a identity.Principal, p, id uuid.UUID) (operation.TaskDetail, error) {
	out := operation.TaskDetail{Inputs: []operation.TaskInput{}, Outputs: []operation.TaskOutput{}, Events: []operation.TaskEvent{}, Params: json.RawMessage(`{}`), OutputCount: 1}
	err := c.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := c.auth()(ctx, tx, a, a.ScopeID, p, false); err != nil {
			return err
		}
		row, err := c.row(tx, a, p, id)
		if err != nil {
			return err
		}
		call, err := c.store(tx, a).LoadQuote(ctx, a.ScopeID, id)
		if err != nil {
			// An expired unconfirmed quote remains readable but cannot buy an execution.
			var q struct{ Contract []byte }
			if e := tx.Raw("SELECT contract FROM ai.quote WHERE id=?", id).Scan(&q).Error; e != nil {
				return e
			}
			if json.Unmarshal(q.Contract, &call) != nil {
				return err
			}
		}
		var quote operation.CreateFreeQuoteResult
		if json.Unmarshal(row.Presentation, &quote) != nil {
			return domain.ErrInvalid
		}
		out.TaskSummary = operation.TaskSummary{ID: id, ProjectID: p, Capability: "text.generate", Mode: "text_to_text", ModelKey: call.Model.Name, ModelName: call.Model.Name, TargetType: "free", Origin: call.Origin, Status: "quoted", QuoteMicros: &quote.QuoteMicros}
		if call.Model.Protocol == "media" {
			var mediaIntent domain.MediaInput
			if json.Unmarshal(call.Raw, &mediaIntent) != nil {
				return domain.ErrInvalid
			}
			out.Capability = mediaIntent.Capability
			out.Mode = mediaIntent.Mode
			out.Params = canonical(mediaIntent.Params)
			for i, ref := range mediaIntent.References {
				id := ref.ID
				out.Inputs = append(out.Inputs, operation.TaskInput{Sequence: int32(i + 1), Role: ref.Role, MediaAssetID: &id})
			}
		}
		out.Params = generationParams(call)
		out.QuoteDetail = quote.QuoteDetail
		out.QuoteExpiresAt = &quote.ExpiresAt
		out.EditorDraft = quote.EditorDraft
		out.EditorCompilerVersion = quote.CompilerVersion
		out.EditorReferenceMapping = quote.ReferenceMapping
		text := quote.FinalPrompt
		out.Inputs = append(out.Inputs, operation.TaskInput{Sequence: 0, Role: "prompt", Text: &text})
		var source promptdomain.InputSource
		if json.Unmarshal(row.Source, &source) == nil && source.Kind == "canvas_input" {
			out.Source = &operationdomain.CanvasSource{CanvasID: source.CanvasID, NodeID: source.NodeID, Revision: source.CanvasRevision}
		} else {
			var legacy operationdomain.CanvasSource
			if json.Unmarshal(row.Source, &legacy) == nil && legacy.Valid() {
				out.Source = &legacy
			}
		}
		var created time.Time
		if e := tx.Raw("SELECT created_at FROM ai.creation WHERE id=?", id).Row().Scan(&created); e != nil {
			return e
		}
		out.CreateTime = created
		out.UpdateTime = created
		var n int64
		if err := tx.Raw("SELECT count(*) FROM ai.request WHERE id=?", id).Scan(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		record, err := c.store(tx, a).Read(ctx, a.ScopeID, id)
		if err != nil {
			return err
		}
		reserved, charged, released := record.Held*100000, record.Charged*100000, record.Released*100000
		out.BillingOutcome = operation.BillingOutcome{ReservedMicros: &reserved, ChargedMicros: &charged, ReleasedMicros: &released, ChargeState: "pending", ProviderSettlementState: "unknown", DeliveryState: "pending", Regenerate: "blocked", Reason: "charge_pending", ReconciliationRequired: true}
		out.Status = "running"
		switch record.State {
		case "unknown":
			out.Status = "unknown"
		case "received":
			out.Status = "settling"
		case "pending":
			out.Status = "running"
		case "released":
			out.Status = "cancelled"
			out.BillingOutcome.ChargeState = "released"
			out.BillingOutcome.ProviderSettlementState = "released"
			out.BillingOutcome.ReconciliationRequired = false
			out.BillingOutcome.Regenerate = "confirm"
		case "settled":
			out.Status = "completed"
			out.SettledMicros = &charged
			out.BillingOutcome.ChargeState = "settled"
			out.BillingOutcome.ProviderSettlementState = "settled"
			out.BillingOutcome.DeliveryState = "ready"
			out.BillingOutcome.ReconciliationRequired = false
			out.BillingOutcome.Regenerate = "confirm"
			out.BillingOutcome.EvidencePresent = true
			var flags struct {
				ReconciliationOnly bool `json:"reconciliation_only"`
			}
			_ = json.Unmarshal(record.Receipt.Body, &flags)
			if flags.ReconciliationOnly {
				out.Status = "failed"
				out.BillingOutcome.DeliveryState = "unrecoverable"
				return nil
			}
			if call.Model.Protocol == "media" {
				out.Status = "ingesting"
				out.BillingOutcome.DeliveryState = "pending"
				var artifact struct {
					State   string
					Summary []byte
				}
				if err := tx.Raw("SELECT state,summary FROM ai.output WHERE request_id=?", id).Scan(&artifact).Error; err != nil {
					return err
				}
				if len(artifact.Summary) > 0 {
					var summary OutputSummary
					if json.Unmarshal(artifact.Summary, &summary) != nil {
						return domain.ErrInvalid
					}
					status := "pending"
					if artifact.State == "ready" {
						out.Status = "completed"
						out.BillingOutcome.DeliveryState = "ready"
						status = "passed"
					}
					aid := summary.AssetID
					var present int
					if err := tx.Raw("SELECT 1 FROM media.media_asset WHERE id=? AND project_id=? AND source_ai_request_id=? AND sha256=? AND NOT is_delete AND status='ready' AND moderation_status='passed'", aid, p, id, summary.SHA256).Scan(&present).Error; err != nil {
						return err
					}
					if artifact.State == "ready" && present != 1 {
						out.BillingOutcome.DeliveryState = "unrecoverable"
						out.Outputs = append(out.Outputs, operation.TaskOutput{ID: uuid.NewSHA1(id, []byte("output/0")), Sequence: 0, Kind: summary.Kind, ModerationStatus: "unavailable", CreateTime: record.Created})
						return nil
					}
					out.Outputs = append(out.Outputs, operation.TaskOutput{ID: uuid.NewSHA1(id, []byte("output/0")), Sequence: 0, Kind: summary.Kind, MediaAssetID: &aid, ModerationStatus: status, CreateTime: record.Created})
					out.CanvasResults = []operation.CanvasHistoryResult{{AssetID: aid, Kind: summary.Kind}}
				}
				return nil
			}
			result, e := agentResult(record, call.Model.Name, call.Model.Protocol)
			if e != nil {
				return e
			}
			var resultBody struct {
				Output string `json:"output"`
			}
			if json.Unmarshal(result.Result, &resultBody) != nil {
				return domain.ErrInvalid
			}
			out.Outputs = append(out.Outputs, operation.TaskOutput{ID: uuid.NewSHA1(id, []byte("output/0")), Sequence: 0, Kind: "text", Text: &resultBody.Output, ModerationStatus: "skipped", CreateTime: record.Created})
		}
		return nil
	})
	return out, err
}

// List returns the same new task projection used by direct task queries.
func (c Creation) List(ctx context.Context, a identity.Principal, p uuid.UUID) ([]operation.TaskSummary, error) {
	result := []operation.TaskSummary{}
	var ids []uuid.UUID
	err := c.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := c.auth()(ctx, tx, a, a.ScopeID, p, false); err != nil {
			return err
		}
		return tx.Raw("SELECT id FROM ai.creation WHERE project_id=? AND scope_id=? ORDER BY created_at DESC,id DESC LIMIT 100", p, a.ScopeID).Scan(&ids).Error
	})
	if err != nil {
		return result, err
	}
	for _, id := range ids {
		task, err := c.Task(ctx, a, p, id)
		if err != nil {
			return result, err
		}
		result = append(result, task.TaskSummary)
	}
	return result, nil
}

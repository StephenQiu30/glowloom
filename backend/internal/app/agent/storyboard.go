package agent

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/domain"
)

const storyboardReadTool = "canvas_storyboard_read"
const storyboardWriteTool = "canvas_storyboard_write"

type storyboardToolInput struct {
	CanvasID uuid.UUID `json:"canvas_id"`
	canvasdomain.StoryboardCommand
}

func storyboardTools() []toolBinding {
	const read = `{"type":"object","properties":{"canvas_id":{"type":"string","format":"uuid"}},"required":["canvas_id"],"additionalProperties":false}`
	const write = `{"type":"object","properties":{
		"canvas_id":{"type":"string","format":"uuid"},
		"expected_revision":{"type":"integer","minimum":1},
		"board_id":{"type":"string","format":"uuid"},
		"shots":{"type":"array","minItems":1,"maxItems":64,"items":{"type":"object","properties":{"title":{"type":"string"},"action":{"type":"string"},"duration_ms":{"type":"integer","minimum":1},"dialogue":{"type":"string"},"camera":{"type":"string"},"sound":{"type":"string"}},"required":["title","action","duration_ms"],"additionalProperties":false}},
		"edit":{"type":"object","properties":{"shot_id":{"type":"string","format":"uuid"},"expected_revision":{"type":"integer","minimum":1},"field":{"enum":["title","action","duration_ms","dialogue","camera","sound"]},"value":{"anyOf":[{"type":"string"},{"type":"integer"}]}},"required":["shot_id","expected_revision","field","value"],"additionalProperties":false},
		"move":{"type":"object","properties":{"shot_id":{"type":"string","format":"uuid"},"before_id":{"type":"string","format":"uuid"}},"required":["shot_id"],"additionalProperties":false}
	},"required":["canvas_id","expected_revision"],"additionalProperties":false}`
	return []toolBinding{
		tool(storyboardReadTool, "读取当前普通画布的正式故事板及镜头稳定 ID、内容修订。", "read", read,
			func(ctx context.Context, tx *gorm.DB, s domain.Scope, _ uuid.UUID, raw json.RawMessage) (any, error) {
				var request struct {
					CanvasID uuid.UUID `json:"canvas_id"`
				}
				if err := decode(raw, &request); err != nil {
					return nil, err
				}
				if s.CanvasID == nil || request.CanvasID != *s.CanvasID {
					return nil, domain.ErrNotFound
				}
				view, err := canvas.NewYjsService(canvasStore(tx), nil).ReadStoryboard(ctx, app.Principal(s), request.CanvasID)
				if err != nil {
					return nil, businessError(err)
				}
				if view.ProjectID != s.ProjectID {
					return nil, domain.ErrNotFound
				}
				return view, nil
			}),
		tool(storyboardWriteTool, "按授权与画布 revision 正式创建故事板或改写一镜指定字段，原镜身份和未修改字段保留。", "write", write, nil),
	}
}

func (b *Boundary) executeStoryboardWrite(ctx context.Context, in domain.CallInput) (domain.CallReceipt, error) {
	if b.canvasRuntime == nil {
		return domain.CallReceipt{}, domain.ErrUnavailable
	}
	if in.MaxMicros < 0 || in.Call.ContractVersion != "v1" || in.Call.InputHash != domain.Hash(in.Input) {
		return domain.CallReceipt{}, domain.ErrInvalid
	}
	old, err := b.Store.ReadCall(ctx, in.Grant.RootRunID, in.Grant.SubmissionID, in.Call)
	if err == nil {
		if old.Kind != "tool" || old.Name != storyboardWriteTool || old.Call.InputHash != in.Call.InputHash || old.MaxMicros != in.MaxMicros {
			return domain.CallReceipt{}, domain.ErrConflict
		}
		return old, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.CallReceipt{}, err
	}
	grant, err := b.Store.ValidateGrant(ctx, in.Grant)
	if err != nil {
		return domain.CallReceipt{}, err
	}
	var request storyboardToolInput
	if err := decode(in.Input, &request); err != nil {
		return domain.CallReceipt{}, err
	}
	if grant.Scope.CanvasID == nil || request.CanvasID != *grant.Scope.CanvasID {
		return domain.CallReceipt{}, domain.ErrNotFound
	}
	key := uuid.NewSHA1(grant.RootRunID, []byte("storyboard:"+grant.SubmissionID.String()+":"+domain.ContentHash(in.Call)))
	prepared, err := canvas.NewYjsService(canvasStore(b.db), b.canvasRuntime).PrepareStoryboard(ctx, app.Principal(grant.Scope), request.CanvasID, key, request.StoryboardCommand)
	if err != nil {
		return domain.CallReceipt{}, storyboardBusinessError(err)
	}
	if prepared.View.ProjectID != grant.Scope.ProjectID {
		return domain.CallReceipt{}, domain.ErrNotFound
	}
	receipt, err := b.Store.ExecuteAtomicTool(ctx, storyboardWriteTool, in, func(tx *gorm.DB, s domain.Scope, _ uuid.UUID) (domain.CallResult, error) {
		view, err := canvas.NewYjsService(canvasStore(tx), b.canvasRuntime).CommitStoryboard(ctx, app.Principal(s), prepared)
		if err != nil {
			return domain.CallResult{}, storyboardBusinessError(err)
		}
		data, err := json.Marshal(view)
		return domain.CallResult{Status: domain.CallStatusCompleted, Result: data}, err
	})
	if err == nil && receipt.Status == domain.CallStatusCompleted && prepared.Command != nil && b.canvasNotify != nil {
		b.canvasNotify(request.CanvasID)
	}
	return receipt, err
}

func storyboardBusinessError(err error) error {
	if errors.Is(err, canvasdomain.ErrStoryboardConflict) || errors.Is(err, canvas.ErrYjsContention) {
		return domain.ErrConflict
	}
	return businessError(err)
}

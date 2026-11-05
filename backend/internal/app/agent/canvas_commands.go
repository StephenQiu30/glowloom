package agent

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

// CanvasCommandsInput uses the existing closed Go canvas operation vocabulary.
// No caller supplies a binary update, actor, grant scope or command identity.
type CanvasCommandsInput struct {
	CanvasID          uuid.UUID                `json:"canvas_id"`
	ExpectedRevision  int64                    `json:"expected_revision"`
	ExpectedCommitSeq *canvas.Revision         `json:"expected_commit_seq,omitempty" swaggertype:"string"`
	Operations        []canvasdomain.Operation `json:"operations"`
}

// WithCanvasRuntime advertises structured editing only when the actual shared
// Yjs computation adapter was installed by the process composition.
func (b *Boundary) WithCanvasRuntime(runtime canvas.YjsCommandRuntime) *Boundary {
	b.canvasRuntime = runtime
	binding := tool("canvas_commands", "按当前revision原子写入结构化分镜、导演台镜头、时间线与真实媒体绑定。复用canvas operations闭集；覆盖需当前版本，媒体只接受项目正式asset_id。成功回执给出真实revision和commit_seq。", "write", commandSchema(CanvasCommandsInput{}), nil)
	b.bindings = append(b.bindings, binding)
	b.Capabilities.Tools = append(b.Capabilities.Tools, app.Tool{Descriptor: binding.descriptor})
	snapshot := tool("canvas_snapshot", "读取画布文档与同一事务的Yjs协作revision，作为下一写入的并发基线。", "read", `{"type":"object","properties":{"canvas_id":{"type":"string","format":"uuid"}},"required":["canvas_id"],"additionalProperties":false}`, func(ctx context.Context, tx *gorm.DB, s domain.Scope, _ uuid.UUID, raw json.RawMessage) (any, error) {
		var input struct {
			CanvasID uuid.UUID `json:"canvas_id"`
		}
		if err := decode(raw, &input); err != nil {
			return nil, err
		}
		state, err := canvasStore(tx).LoadYjs(ctx, s.Principal(), input.CanvasID)
		if err != nil {
			return nil, err
		}
		if state.Document.ProjectID != s.ProjectID || s.CanvasID != nil && *s.CanvasID != input.CanvasID {
			return nil, domain.ErrNotFound
		}
		return map[string]any{"document": state.Document, "epoch": state.Epoch, "commit_seq": state.CommitSeq, "writer_term": state.WriterTerm}, nil
	})
	b.bindings = append(b.bindings, snapshot)
	b.Capabilities.Tools = append(b.Capabilities.Tools, app.Tool{Descriptor: snapshot.descriptor})
	return b
}
func (b *Boundary) executeCanvasCommands(ctx context.Context, in domain.CallInput) (domain.CallReceipt, error) {
	if b.canvasRuntime == nil {
		return domain.CallReceipt{}, domain.ErrUnavailable
	}
	if in.MaxMicros < 0 || in.Call.ContractVersion != "v1" || in.Call.InputHash != domain.Hash(in.Input) {
		return domain.CallReceipt{}, domain.ErrInvalid
	}
	// A committed command is returned before preparing against a later prefix.
	old, err := b.Store.ReadCall(ctx, in.Grant.RootRunID, in.Grant.SubmissionID, in.Call)
	if err == nil {
		if old.Kind != "tool" || old.Name != "canvas_commands" || old.Call.InputHash != in.Call.InputHash || old.MaxMicros != in.MaxMicros {
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
	var requested CanvasCommandsInput
	if err = decode(in.Input, &requested); err != nil {
		return domain.CallReceipt{}, err
	}
	if requested.CanvasID == uuid.Nil || grant.Scope.CanvasID != nil && *grant.Scope.CanvasID != requested.CanvasID {
		return domain.CallReceipt{}, domain.ErrNotFound
	}
	state, err := canvasStore(b.db).LoadYjs(ctx, grant.Scope.Principal(), requested.CanvasID)
	if err != nil {
		return domain.CallReceipt{}, err
	}
	if state.Document.ProjectID != grant.Scope.ProjectID {
		return domain.CallReceipt{}, domain.ErrNotFound
	}
	commandID := uuid.NewSHA1(grant.RootRunID, []byte("canvas-command:"+grant.SubmissionID.String()+":"+domain.ContentHash(in.Call)))
	prepared, err := canvas.NewYjsService(canvasStore(b.db), b.canvasRuntime).PrepareCommands(ctx, grant.Scope.Principal(), requested.CanvasID, canvas.YjsCommandInput{CommandID: commandID, SessionID: grant.SubmissionID, ExpectedRevision: requested.ExpectedRevision, ExpectedCommitSeq: requested.ExpectedCommitSeq, Operations: requested.Operations})
	if err != nil {
		return domain.CallReceipt{}, businessError(err)
	}
	// All expensive Node computation is complete. This lock encloses the current
	// grant check, formal Yjs acceptance and original business call receipt.
	return b.Store.ExecuteAtomicTool(ctx, "canvas_commands", in, func(tx *gorm.DB, s domain.Scope, _ uuid.UUID) (domain.CallResult, error) {
		receipt, err := canvas.NewYjsService(canvasStore(tx), b.canvasRuntime).CommitCommands(ctx, s.Principal(), prepared)
		if err != nil {
			return domain.CallResult{}, businessError(err)
		}
		result, err := json.Marshal(map[string]any{"status": receipt.Status, "code": receipt.Code, "canvas_id": requested.CanvasID, "revision": receipt.ContentRevision, "commit_seq": receipt.CommitSeq, "epoch": receipt.Epoch, "graph_hash": receipt.GraphHash, "update_id": receipt.UpdateID})
		return domain.CallResult{Status: "completed", Result: result}, err
	})
}

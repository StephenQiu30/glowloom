package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	agentpostgres "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	scriptpostgres "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/postgres"
	script "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	scriptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

// ScriptSourceDocument accepts canonical authored content, without accepting
// caller-supplied external provenance or an unrelated HTML representation.
type ScriptSourceDocument struct {
	Kind     string                    `json:"source_kind"`
	Title    string                    `json:"title"`
	Status   string                    `json:"status"`
	Document scriptdomain.RichDocument `json:"document"`
}

// ScriptSourcePublishInput contains the confirmed script source and publication context.
type ScriptSourcePublishInput struct {
	Action           string                 `json:"action"`
	ExpectedRevision int64                  `json:"expected_revision"`
	BaseVersionID    *uuid.UUID             `json:"base_version_id,omitempty"`
	LineageID        *uuid.UUID             `json:"source_lineage_id,omitempty"`
	RightsConfirmed  bool                   `json:"rights_confirmed"`
	Sources          []ScriptSourceDocument `json:"sources,omitempty"`
	Order            []uuid.UUID            `json:"source_lineage_ids,omitempty"`
}

func (in ScriptSourcePublishInput) command(s domain.Scope, key uuid.UUID) script.SourceCommand {
	sources := make([]script.SourceInput, len(in.Sources))
	for i, v := range in.Sources {
		sources[i] = script.SourceInput{Kind: scriptdomain.SourceKind(v.Kind), Title: v.Title, Status: scriptdomain.SourceStatus(v.Status), Document: v.Document}
	}
	return script.SourceCommand{ProjectID: s.ProjectID, Key: key, RequestID: key, Action: scriptdomain.SourceAction(in.Action), ExpectedRevision: in.ExpectedRevision, BaseVersionID: in.BaseVersionID, LineageID: in.LineageID, RightsConfirmed: in.RightsConfirmed, Sources: sources, Order: in.Order}
}
func sourceStore(db *gorm.DB) *scriptpostgres.SourceStore {
	return scriptpostgres.NewSourceStore(db, func(tx *gorm.DB) script.ProjectAccess { return scopewire.ProvideWorkerProjectContentAccess(tx) }).WithObjectLocations(scriptwire.ProvideScriptObjectLocations).WithPublicationAuthorization(scriptwire.ProvideSourcePublicationAuthorization)
}

// WithSourceObjects installs the actual synchronous located byte owner. The
// shared I/O registry records real local call exits; absence never proves exit.
func (b *Boundary) WithSourceObjects(objects script.PrivateObjects) *Boundary {
	if objects == nil {
		return b
	}
	b.sourceObjects = objects
	b.sourceOwner = script.NewSourceService(sourceStore(b.db), objects, time.Now)
	binding := tool("script_source_publish", "发布正式原创剧本素材的完整富文本快照，支持create/update/import/delete/reorder。必须读取script_workspace取得当前revision及原lineage；原调用未知时只查询原回执，禁止换call重发。真实私有对象写入与正式版本发布，成功回执给出version_id及source映射。", "write", commandSchema(ScriptSourcePublishInput{}), nil)
	binding.descriptor.Roles = []string{"producer", "script"}
	binding.descriptor.Replay = "unsafe"
	b.bindings = append(b.bindings, binding)
	b.Capabilities.Tools = append(b.Capabilities.Tools, app.Tool{Descriptor: binding.descriptor})
	b.Capabilities.ToolRecover = b.recoverSourceCall
	return b
}
func (b *Boundary) executeSourcePublication(ctx context.Context, in domain.CallInput) (domain.CallReceipt, error) {
	if b.sourceObjects == nil {
		return domain.CallReceipt{}, domain.ErrUnavailable
	}
	var requested ScriptSourcePublishInput
	if err := decode(in.Input, &requested); err != nil {
		return domain.CallReceipt{}, err
	}
	// Validation is pure. The real command key comes exclusively from the durable
	// original call receipt and is assigned in its admission transaction below.
	provisional := requested.command(domain.Scope{ProjectID: uuid.New()}, uuid.New())
	if err := script.ValidateSourceCommand(provisional); err != nil {
		return domain.CallReceipt{}, domain.ErrInvalid
	}
	result, err := b.Store.ExecuteToolIntent(ctx, "script_source_publish", in, func(tx *gorm.DB, s domain.Scope, key uuid.UUID) error {
		hash, err := script.SourceCommandHash(requested.command(s, key))
		if err != nil {
			return err
		}
		return agentpostgres.BindSourceAuthorization(tx, key, in.Grant, hash)
	}, func(ctx context.Context, s domain.Scope, key uuid.UUID) (domain.CallResult, error) {
		store := sourceStore(b.db)
		command := requested.command(s, key)
		owner := script.NewSourceServiceSharingIO(store, b.sourceObjects, time.Now, b.sourceOwner)
		receipt, err := owner.Write(ctx, s.Principal(), command)
		if err != nil {
			hash, hashErr := script.SourceCommandHash(command)
			if hashErr == nil {
				prior, readErr := store.FindCommand(ctx, s.Principal(), s.ProjectID, key, hash)
				if readErr == nil && prior == nil {
					return domain.CallResult{Status: "failed", Result: json.RawMessage(`{"code":"script_source_not_published"}`)}, nil
				}
			}
			return domain.CallResult{}, err
		}
		raw, err := json.Marshal(receipt)
		return domain.CallResult{Status: "completed", Result: raw}, err
	})
	if err == nil && (result.Status == "in_flight" || result.Status == "unknown") {
		if _, recoveryErr := b.recoverSourceCall(ctx, result.ReceiptID); recoveryErr == nil {
			return b.Store.ReadCall(ctx, in.Grant.RootRunID, in.Grant.SubmissionID, in.Call)
		} else if !errors.Is(recoveryErr, domain.ErrUnavailable) && !errors.Is(recoveryErr, domain.ErrNotFound) {
			return result, recoveryErr
		}
	}
	return result, err
}
func (b *Boundary) recoverSourceCall(ctx context.Context, id uuid.UUID) (domain.CallResult, error) {
	receipt, err := b.Store.RecoverSourceCall(ctx, id, func(tx *gorm.DB, s domain.Scope, key uuid.UUID, hash string) (domain.CallResult, error) {
		prior, err := sourceStore(tx).FindCommand(ctx, s.Principal(), s.ProjectID, key, hash)
		if err != nil {
			return domain.CallResult{}, businessError(err)
		}
		if prior == nil {
			return domain.CallResult{}, domain.ErrUnavailable
		}
		if prior.Receipt == nil {
			cancelled, err := sourceStore(tx).CancelledPublication(ctx, s.Principal(), s.ProjectID, key, hash)
			if err != nil {
				return domain.CallResult{}, businessError(err)
			}
			if cancelled {
				return domain.CallResult{Status: "failed", Result: json.RawMessage(`{"code":"source_cancelled"}`)}, nil
			}
			return domain.CallResult{}, domain.ErrUnavailable
		}
		raw, err := json.Marshal(prior.Receipt)
		return domain.CallResult{Status: "completed", Result: raw}, err
	})
	return receipt.CallResult, err
}

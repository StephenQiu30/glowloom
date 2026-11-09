package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	pgcatalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	pgscript "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/postgres"
	script "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	pgsubject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/adapter/postgres"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
)

type toolBinding struct {
	descriptor app.ToolDescriptor
	run        func(context.Context, *gorm.DB, domain.Scope, uuid.UUID, json.RawMessage) (any, error)
}

func tool(name, description, effect, schema string, run func(context.Context, *gorm.DB, domain.Scope, uuid.UUID, json.RawMessage) (any, error)) toolBinding {
	roles := []string{"producer"}
	switch {
	case effect == "read":
		roles = append(roles, "script", "art", "storyboard", "shot", "voice", "edit", "qa")
	case strings.HasPrefix(name, "script_"):
		roles = append(roles, "script", "storyboard")
	case strings.HasPrefix(name, "bible_"):
		roles = append(roles, "script", "art", "storyboard")
	case strings.HasPrefix(name, "operation_"):
		roles = append(roles, "art", "shot", "voice")
	case strings.HasPrefix(name, "canvas_"):
		roles = append(roles, "art", "storyboard", "shot", "edit")
	}
	return toolBinding{descriptor: app.ToolDescriptor{Name: name, Roles: roles, Description: description, Parameters: json.RawMessage(schema), Effect: effect, ContractVersion: "v1", Replay: "safe"}, run: run}
}
func decode(raw json.RawMessage, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return domain.ErrInvalid
	}
	if decoder.Decode(new(any)) != io.EOF {
		return domain.ErrInvalid
	}
	return nil
}
func canvasStore(tx *gorm.DB) *pgcanvas.Store {
	return pgcanvas.NewScopedStore(tx, func(inner *gorm.DB) canvas.MediaReader {
		return media.NewAssetQuery(pgmedia.NewScopedStore(inner, mediawire.NewMediaWorkerScopeAccess))
	}, newCanvasAccess)
}
func canvasService(tx *gorm.DB) *canvas.Service { return canvas.NewService(canvasStore(tx)) }
func operationStore(tx *gorm.DB) *pgoperation.Store {
	return pgoperation.NewScopedStore(tx, operationwire.ProvideOperationWorkerAccess, operationwire.ProvideOperationWorkerAccess, operationwire.ProvideOperationPlatformAccess).WithExecutionAvailability(func(inner *gorm.DB) operation.ExecutionAvailability { return pgcatalog.NewStore(inner) })
}

func operationStoreWithCreditSaga(tx *gorm.DB, enabled bool) *pgoperation.Store {
	return operationStore(tx).WithCreditOperationSaga(enabled)
}
func scriptService(tx *gorm.DB) *script.SourceService {
	return script.NewSourceService(pgscript.NewSourceStore(tx, func(inner *gorm.DB) script.ProjectAccess { return scopewire.ProvideWorkerProjectContentAccess(inner) }), nil, time.Now)
}

type subjectScope struct{ tx *gorm.DB }

func (s subjectScope) Resolve(ctx context.Context, a identity.Principal, write bool) (subject.ScopeFacts, error) {
	r, err := scopewire.ProvideScopeAccess(s.tx).Resolve(ctx, a.ID, a.ScopeID, write)
	if err != nil {
		return subject.ScopeFacts{}, err
	}
	return subject.ScopeFacts{ScopeID: r.ID, PhysicalScopeID: r.PhysicalScopeID}, nil
}
func subjectStore(tx *gorm.DB) *pgsubject.Store {
	return pgsubject.NewStore(tx, pgsubject.Factories{Scope: func(inner *gorm.DB) subject.ScopeAccess { return subjectScope{inner} }})
}

type catalogAccess struct{ tx *gorm.DB }

func (c catalogAccess) AuthorizeCatalog(ctx context.Context, a identity.Principal, pid uuid.UUID) error {
	_, err := scopewire.ProvideWorkerProjectContentAccess(c.tx).Authorize(ctx, a, pid, false)
	return err
}
func catalogStore(tx *gorm.DB) *pgcatalog.Store {
	return pgcatalog.NewScopedStore(tx, func(inner *gorm.DB) catalog.ActorAccess { return pgidentity.NewStore(inner) }, func(inner *gorm.DB) catalog.ProjectAccess { return catalogAccess{inner} })
}
func businessTools() []toolBinding {
	return []toolBinding{
		tool("canvas_list", "列出当前项目画布与真实版本", "read", `{"type":"object","properties":{},"additionalProperties":false}`, func(ctx context.Context, tx *gorm.DB, s domain.Scope, _ uuid.UUID, raw json.RawMessage) (any, error) {
			var in struct{}
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			return canvasService(tx).List(ctx, s.Principal(), s.ProjectID)
		}),
		tool("canvas_get", "读取当前范围内画布现场", "read", `{"type":"object","properties":{"canvas_id":{"type":"string","format":"uuid"}},"required":["canvas_id"],"additionalProperties":false}`, func(ctx context.Context, tx *gorm.DB, s domain.Scope, _ uuid.UUID, raw json.RawMessage) (any, error) {
			var in struct {
				CanvasID uuid.UUID `json:"canvas_id"`
			}
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			doc, err := canvasService(tx).Get(ctx, s.Principal(), in.CanvasID)
			if err == nil && (doc.ProjectID != s.ProjectID || s.CanvasID != nil && doc.ID != *s.CanvasID) {
				return nil, domain.ErrNotFound
			}
			return doc, err
		}),
		tool("canvas_create", "建立正式项目画布", "write", `{"type":"object","properties":{"name":{"type":"string","minLength":1,"maxLength":128},"kind":{"type":"string","enum":["canvas","short_drama"]}},"required":["name"],"additionalProperties":false}`, func(ctx context.Context, tx *gorm.DB, s domain.Scope, key uuid.UUID, raw json.RawMessage) (any, error) {
			var in struct {
				Name string `json:"name"`
				Kind string `json:"kind,omitempty"`
			}
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			return canvasService(tx).Create(ctx, s.Principal(), s.ProjectID, key.String(), canvas.CreateInput{Name: in.Name, Kind: in.Kind, Scope: json.RawMessage(`{}`)})
		}),
		tool("canvas_rename", "按成功回执真实版本重命名画布", "write", `{"type":"object","properties":{"canvas_id":{"type":"string","format":"uuid"},"expected_revision":{"type":"integer","minimum":1},"name":{"type":"string","minLength":1}},"required":["canvas_id","expected_revision","name"],"additionalProperties":false}`, func(ctx context.Context, tx *gorm.DB, s domain.Scope, key uuid.UUID, raw json.RawMessage) (any, error) {
			var in struct {
				CanvasID         uuid.UUID `json:"canvas_id"`
				ExpectedRevision int64     `json:"expected_revision"`
				Name             string    `json:"name"`
			}
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			doc, err := canvasService(tx).Get(ctx, s.Principal(), in.CanvasID)
			if err != nil {
				return nil, err
			}
			if doc.ProjectID != s.ProjectID || s.CanvasID != nil && doc.ID != *s.CanvasID {
				return nil, domain.ErrNotFound
			}
			return canvasService(tx).Rename(ctx, s.Principal(), in.CanvasID, key.String(), canvas.RenameInput{ExpectedRevision: in.ExpectedRevision, Name: in.Name})
		}),
		tool("script_workspace", "读取正式剧本工作区版本", "read", `{"type":"object","properties":{},"additionalProperties":false}`, func(ctx context.Context, tx *gorm.DB, s domain.Scope, _ uuid.UUID, raw json.RawMessage) (any, error) {
			var in struct{}
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			return scriptService(tx).Workspace(ctx, s.Principal(), s.ProjectID)
		}),
		tool("script_sources", "分页读取冻结的剧本素材元数据", "read", `{"type":"object","properties":{"version_id":{"type":"string","format":"uuid"},"after":{"type":"integer","minimum":0},"limit":{"type":"integer","minimum":1,"maximum":100}},"additionalProperties":false}`, func(ctx context.Context, tx *gorm.DB, s domain.Scope, _ uuid.UUID, raw json.RawMessage) (any, error) {
			var in struct {
				VersionID *uuid.UUID `json:"version_id,omitempty"`
				After     int        `json:"after"`
				Limit     int        `json:"limit"`
			}
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			return scriptService(tx).Sources(ctx, s.Principal(), s.ProjectID, in.VersionID, in.After, in.Limit)
		}),
		tool("media_list", "分页读取项目正式媒体素材", "read", `{"type":"object","properties":{"kind":{"type":"string"},"cursor":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":100}},"additionalProperties":false}`, func(ctx context.Context, tx *gorm.DB, s domain.Scope, _ uuid.UUID, raw json.RawMessage) (any, error) {
			var in struct {
				Kind   string `json:"kind"`
				Cursor string `json:"cursor"`
				Limit  int    `json:"limit"`
			}
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			return media.NewAssetQuery(pgmedia.NewScopedStore(tx, mediawire.NewMediaWorkerScopeAccess)).List(ctx, s.Principal(), s.ProjectID, in.Kind, in.Cursor, in.Limit)
		}),
		tool("subject_list", "查询当前创作空间的正式主体", "read", `{"type":"object","properties":{"query":{"type":"string"},"page":{"type":"integer","minimum":1},"page_size":{"type":"integer","minimum":1,"maximum":100}},"additionalProperties":false}`, func(ctx context.Context, tx *gorm.DB, s domain.Scope, _ uuid.UUID, raw json.RawMessage) (any, error) {
			var in struct {
				Query    string `json:"query"`
				Page     int    `json:"page"`
				PageSize int    `json:"page_size"`
			}
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			return subjectStore(tx).List(ctx, s.Principal(), subject.ListInput{Query: in.Query, Page: in.Page, PageSize: in.PageSize, TimeField: "created_at", Order: "desc"})
		}),
		tool("subject_get", "读取当前空间主体的冻结资源绑定", "read", `{"type":"object","properties":{"subject_id":{"type":"string","format":"uuid"}},"required":["subject_id"],"additionalProperties":false}`, func(ctx context.Context, tx *gorm.DB, s domain.Scope, _ uuid.UUID, raw json.RawMessage) (any, error) {
			var in struct {
				SubjectID uuid.UUID `json:"subject_id"`
			}
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			return subjectStore(tx).Get(ctx, s.Principal(), in.SubjectID)
		}),
		tool("catalog_models", "读取当前项目可用模型发布与价格", "read", `{"type":"object","properties":{"model_key":{"type":"string"},"capability":{"type":"string"},"mode":{"type":"string"}},"additionalProperties":false}`, func(ctx context.Context, tx *gorm.DB, s domain.Scope, _ uuid.UUID, raw json.RawMessage) (any, error) {
			var in struct {
				ModelKey   string `json:"model_key"`
				Capability string `json:"capability"`
				Mode       string `json:"mode"`
			}
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			return catalog.NewListModelsQuery(catalogStore(tx)).Execute(ctx, s.Principal(), catalog.ListModelsInput{ProjectID: s.ProjectID, ModelKey: in.ModelKey, Capability: in.Capability, Mode: in.Mode, Limit: 50})
		}),
		tool("operation_task", "查询原正式生成任务与计费回执，不创建新请求", "read", `{"type":"object","properties":{"operation_id":{"type":"string","format":"uuid"}},"required":["operation_id"],"additionalProperties":false}`, func(ctx context.Context, tx *gorm.DB, s domain.Scope, _ uuid.UUID, raw json.RawMessage) (any, error) {
			var in struct {
				OperationID uuid.UUID `json:"operation_id"`
			}
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			return operation.NewPublicQuery(operationStore(tx)).Task(ctx, s.Principal(), s.ProjectID, in.OperationID)
		}),
	}
}

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/evidence"
	evidencepg "github.com/StephenQiu30/glowloom-server/backend/creation/evidence/adapter/postgres"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
)

const evidenceKinds = `["brief","decision","canvas_node","script_source","scene","script_line","character","location","prop","media","transcript"]`

func evidenceTools() []toolBinding {
	return []toolBinding{
		tool(evidence.SearchTool, "在当前项目的可读资料中按稳定 ID、名称、别名或原词查找证据。带回来源 ID、版本及段落或时间位置。无匹配时说明缺少依据。剧本正文仍在索引时说明材料尚未就绪。遇到歧义让用户选择。", "read",
			`{"type":"object","properties":{"query":{"type":"string","maxLength":160},"kinds":{"type":"array","items":{"type":"string","enum":`+evidenceKinds+`}},"limit":{"type":"integer","minimum":1,"maximum":12}},"required":["query"],"additionalProperties":false}`,
			func(ctx context.Context, tx *gorm.DB, s domain.Scope, _ uuid.UUID, raw json.RawMessage) (any, error) {
				var q evidence.Query
				if err := decode(raw, &q); err != nil {
					return nil, err
				}
				if err := authorizeEvidence(ctx, tx, s); err != nil {
					return nil, err
				}
				result, err := evidencepg.Search(tx, s.ProjectID, q)
				if errors.Is(err, evidence.ErrInvalid) {
					return nil, domain.ErrInvalid
				}
				if err != nil {
					return nil, err
				}
				version, err := evidencepg.PendingScriptIndex(tx, s.ProjectID)
				if err != nil {
					return nil, err
				}
				if version != uuid.Nil {
					result.ScriptIndexPending = true
					if err := evidencepg.ScheduleScriptIndex(ctx, tx, evidencepg.ScriptIndexRequest{ActorID: s.ActorID, OrgID: s.OrgID, ProjectID: s.ProjectID, VersionID: version}, time.Now()); err != nil {
						return nil, err
					}
				}
				return result, nil
			}),
		tool(evidence.ReadTool, "按 kind、id 和 part 打开项目证据。引用之前搜索结果时提供其 version。只会返回当前可读的同版内容。", "read",
			`{"type":"object","properties":{"kind":{"type":"string","enum":`+evidenceKinds+`},"id":{"type":"string","format":"uuid"},"part":{"type":"integer","minimum":1,"maximum":10000},"version":{"type":"string","pattern":"^[a-f0-9]{64}$"}},"required":["kind","id"],"additionalProperties":false}`,
			func(ctx context.Context, tx *gorm.DB, s domain.Scope, _ uuid.UUID, raw json.RawMessage) (any, error) {
				var ref evidence.Reference
				if err := decode(raw, &ref); err != nil {
					return nil, err
				}
				if err := authorizeEvidence(ctx, tx, s); err != nil {
					return nil, err
				}
				result, err := evidencepg.Read(tx, s.ProjectID, ref)
				if errors.Is(err, evidence.ErrInvalid) {
					return nil, domain.ErrInvalid
				}
				return result, err
			}),
	}
}

func authorizeEvidence(ctx context.Context, tx *gorm.DB, scope domain.Scope) error {
	project, err := scopewire.ProvideWorkerProjectContentAccess(tx).Authorize(ctx, app.Principal(scope), scope.ProjectID, false)
	if err != nil {
		return err
	}
	if project.OwnerScopeID != scope.ScopeID {
		return domain.ErrNotFound
	}
	return nil
}

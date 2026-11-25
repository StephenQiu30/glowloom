package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/evidence"
	evidencepg "github.com/StephenQiu30/glowloom-server/backend/internal/creation/evidence/adapter/postgres"
)

const searchableEvidenceKinds = `["brief","decision","canvas_node","storyboard_shot","script_source","scene","script_line","character","location","prop","media","transcript"]`

const readableEvidenceKinds = `["brief","decision","canvas_node","storyboard_shot","shot_decision","script_source","scene","script_line","character","location","prop","media","transcript"]`

func evidenceTools() []toolBinding {
	return []toolBinding{
		tool(evidence.SearchTool, "在当前项目的可读资料中按稳定 ID、名称、别名或原词查找证据。项目启用语义检索后也可用自然语言描述查找。用已核实的 shot_id 单独读取该镜已确认的局部约定。带回来源 ID、版本及定位。遇到歧义让用户选择。", "read",
			`{"type":"object","properties":{"query":{"type":"string","maxLength":160},"shot_id":{"type":"string","format":"uuid"},"kinds":{"type":"array","items":{"type":"string","enum":`+searchableEvidenceKinds+`}},"limit":{"type":"integer","minimum":1,"maximum":12}},"oneOf":[{"required":["query"]},{"required":["shot_id"]}],"additionalProperties":false}`,
			func(ctx context.Context, tx *gorm.DB, s domain.Scope, _ uuid.UUID, raw json.RawMessage) (any, error) {
				var q evidence.Query
				if err := decode(raw, &q); err != nil {
					return nil, err
				}
				if err := authorizeEvidence(ctx, tx, s); err != nil {
					return nil, err
				}
				return lexicalEvidence(ctx, tx, s, q)
			}),
		tool(evidence.ReadTool, "按 kind、id 和 part 打开项目证据。引用之前搜索结果时提供其 version。只会返回当前可读的同版内容。", "read",
			`{"type":"object","properties":{"kind":{"type":"string","enum":`+readableEvidenceKinds+`},"id":{"type":"string","format":"uuid"},"part":{"type":"integer","minimum":1,"maximum":10000},"version":{"type":"string","pattern":"^[a-f0-9]{64}$"}},"required":["kind","id"],"additionalProperties":false}`,
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

func lexicalEvidence(ctx context.Context, tx *gorm.DB, s domain.Scope, q evidence.Query) (evidence.Result, error) {
	result, err := evidencepg.Search(tx, s.ProjectID, q)
	if errors.Is(err, evidence.ErrInvalid) {
		return evidence.Result{}, domain.ErrInvalid
	}
	if err != nil || q.ShotID != nil {
		return result, err
	}
	version, err := evidencepg.PendingScriptIndex(tx, s.ProjectID)
	if err != nil {
		return evidence.Result{}, err
	}
	if version != uuid.Nil {
		result.ScriptIndexPending = true
		if err := evidencepg.ScheduleScriptIndex(ctx, tx, evidencepg.ScriptIndexRequest{ActorID: s.ActorID, OrgID: s.OrgID, ProjectID: s.ProjectID, VersionID: version}, time.Now()); err != nil {
			return evidence.Result{}, err
		}
	}
	return result, nil
}

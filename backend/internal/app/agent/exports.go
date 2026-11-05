package agent

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	pgtool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	mediatool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
)

func exportStore(tx *gorm.DB) *pgtool.Store {
	return pgtool.NewStore(tx, func(inner *gorm.DB) mediatool.TimelineReader { return canvas.NewTimelineReader(canvasStore(inner)) }, func(inner *gorm.DB) mediatool.DerivedMedia {
		return media.NewDerivedService(pgmedia.NewScopedStore(inner, mediawire.NewMediaWorkerScopeAccess))
	}).WithStorageAdmission(func(inner *gorm.DB) mediatool.ToolStorageLedger { return pgmedia.NewStorageRoutes(inner) })
}
func exportTools() []toolBinding {
	create := tool("media_export_create", "冻结当前已保存时间线并受理正式视频/音频或FCPXML/XMEML/剪映工程导出任务。返回真实任务，不把受理当成成片完成。", "write", commandSchema(mediatool.CreateInput{}), func(ctx context.Context, tx *gorm.DB, s domain.Scope, key uuid.UUID, raw json.RawMessage) (any, error) {
		var in mediatool.CreateInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		if s.CanvasID != nil && in.CanvasID != *s.CanvasID {
			return nil, domain.ErrNotFound
		}
		return exportStore(tx).Create(ctx, s.Principal(), s.ProjectID, key, in)
	})
	create.descriptor.Roles = []string{"producer", "edit"}
	query := tool("media_export_get", "查询原正式导出任务的真实状态/产物ID，不重复创建任务。", "read", `{"type":"object","properties":{"export_id":{"type":"string","format":"uuid"}},"required":["export_id"],"additionalProperties":false}`, func(ctx context.Context, tx *gorm.DB, s domain.Scope, _ uuid.UUID, raw json.RawMessage) (any, error) {
		var in struct {
			ExportID uuid.UUID `json:"export_id"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return exportStore(tx).Get(ctx, s.Principal(), s.ProjectID, in.ExportID)
	})
	return []toolBinding{create, query}
}

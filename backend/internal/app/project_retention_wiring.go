package app

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	pgprompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/postgres"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

type retentionContent struct {
	canvas, prompt pgworkspace.ProjectRetentionContent
}

func (o retentionContent) ClearProjectContent(ctx context.Context, project uuid.UUID) error {
	if err := o.canvas.ClearProjectContent(ctx, project); err != nil {
		return err
	}
	return o.prompt.ClearProjectContent(ctx, project)
}

// NewProjectRetention composes content owners under workspace's exact deletion lock.
// Media assets, physical objects and financial/audit history keep their own lifecycle.
func NewProjectRetention(db *gorm.DB) *workspaceapp.ProjectRetention {
	return workspaceapp.NewProjectRetention(pgworkspace.NewProjectRetentionStore(db, func(tx *gorm.DB) pgworkspace.ProjectRetentionContent {
		return retentionContent{canvas: pgcanvas.NewProjectRetentionStore(tx), prompt: pgprompt.NewProjectRetentionStore(tx)}
	}))
}

package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/internal/modules/scripts/domain"
)

type ScriptAnalysisPort interface {
	QueueAnalysis(context.Context, uuid.UUID) (domain.Operation, error)
	GetOperation(context.Context, uuid.UUID) (domain.Operation, error)
	GetAnalysisDraft(context.Context, uuid.UUID) (domain.Analysis, error)
	ApproveAnalysis(context.Context, uuid.UUID) (domain.Analysis, error)
	GetProjectAnalysis(context.Context, uuid.UUID) (domain.Analysis, error)
}

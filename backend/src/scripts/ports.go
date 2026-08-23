package scripts

import (
	"context"

	"github.com/google/uuid"
)

type ScriptAnalysisPort interface {
	QueueAnalysis(context.Context, uuid.UUID) (Operation, error)
	GetOperation(context.Context, uuid.UUID) (Operation, error)
	GetAnalysisDraft(context.Context, uuid.UUID) (Analysis, error)
	ReviseAnalysisDraft(context.Context, uuid.UUID, string, []EpisodeBreakdownOperation) (Analysis, error)
	ApproveAnalysis(context.Context, uuid.UUID) (Analysis, error)
	GetProjectAnalysis(context.Context, uuid.UUID) (Analysis, error)
	CreateShots(context.Context, uuid.UUID, uuid.UUID, int) ([]Shot, error)
	ListShots(context.Context, uuid.UUID, uuid.UUID) ([]Shot, error)
	CreateFixtureCandidate(context.Context, uuid.UUID, string) (Candidate, error)
	SelectCandidate(context.Context, uuid.UUID, string) (Selection, error)
}

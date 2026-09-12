package application

import (
	"context"
	"errors"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

// The reader authorizes and reads the complete frozen facts in one SQL snapshot.
type ReferenceExecutionProgressReader interface {
	ReadReferenceExecutionProgress(context.Context, Actor, string, string) (domain.ReferenceJobProgress, error)
}

type ReferenceExecutionQuery struct {
	reader ReferenceExecutionProgressReader
}

func NewReferenceExecutionQuery(reader ReferenceExecutionProgressReader) *ReferenceExecutionQuery {
	return &ReferenceExecutionQuery{reader: reader}
}

func (query *ReferenceExecutionQuery) Get(ctx context.Context, actor Actor, projectID, executionID string) (domain.ReferenceJobProgress, error) {
	if !validUUID(actor.UserID) || actor.TokenVersion < 1 || !validUUID(projectID) || !validUUID(executionID) {
		return domain.ReferenceJobProgress{}, invalid("Invalid Reference execution query")
	}
	if query == nil || query.reader == nil {
		return domain.ReferenceJobProgress{}, errors.New("Reference execution reader is unavailable")
	}
	value, err := query.reader.ReadReferenceExecutionProgress(ctx, actor, projectID, executionID)
	if err != nil {
		return domain.ReferenceJobProgress{}, err
	}
	if value.ExecutionRef.ID != executionID {
		return domain.ReferenceJobProgress{}, conflict("Reference execution query identity has drifted")
	}
	return value, nil
}

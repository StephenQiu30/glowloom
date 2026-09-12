package application

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

// ReferenceGenerationProgress exposes published identities, not execution rights.
// A nil Execution means preparation has not published an execution for this Target.
type ReferenceGenerationProgress struct {
	WorkspaceID         string                       `json:"workspace_id"`
	ProjectID           string                       `json:"project_id"`
	GenerationTargetRef domain.GenerationRevisionRef `json:"generation_target_ref"`
	PlanRef             domain.GenerationRevisionRef `json:"plan_ref"`
	ReferenceTargetRef  domain.GenerationRevisionRef `json:"reference_target_ref"`
	GenerationRound     int64                        `json:"generation_round"`
	Execution           *domain.ReferenceJobProgress `json:"execution"`
	ContentHash         string                       `json:"content_hash"`
}

// The reader authorizes and reads Target, Heads and complete execution facts in
// one consistent SQL snapshot. It must not create missing facts or dispatch work.
type ReferenceGenerationProgressReader interface {
	ReadReferenceGenerationProgress(context.Context, Actor, string, string) (ReferenceGenerationProgress, error)
}

type ReferenceGenerationQuery struct {
	reader ReferenceGenerationProgressReader
}

func NewReferenceGenerationQuery(reader ReferenceGenerationProgressReader) *ReferenceGenerationQuery {
	return &ReferenceGenerationQuery{reader: reader}
}

func (query *ReferenceGenerationQuery) Get(ctx context.Context, actor Actor, projectID, targetID string) (ReferenceGenerationProgress, error) {
	if !validUUID(actor.UserID) || actor.TokenVersion < 1 || !validUUID(projectID) || !validUUID(targetID) {
		return ReferenceGenerationProgress{}, invalid("Invalid Reference generation query")
	}
	if query == nil || query.reader == nil {
		return ReferenceGenerationProgress{}, errors.New("Reference generation reader is unavailable")
	}
	value, err := query.reader.ReadReferenceGenerationProgress(ctx, actor, projectID, targetID)
	if err != nil {
		return ReferenceGenerationProgress{}, err
	}
	if value.ProjectID != projectID || value.GenerationTargetRef.ID != targetID || !validUUID(value.WorkspaceID) || value.GenerationRound < 1 {
		return ReferenceGenerationProgress{}, conflict("Reference generation query identity has drifted")
	}
	for _, ref := range []domain.GenerationRevisionRef{value.GenerationTargetRef, value.PlanRef, value.ReferenceTargetRef} {
		if !validUUID(ref.ID) || ref.Revision < 1 || !intentHashPattern.MatchString(ref.ContentHash) {
			return ReferenceGenerationProgress{}, conflict("Reference generation query reference has drifted")
		}
	}
	value.ContentHash = ""
	raw, err := json.Marshal(value)
	if err != nil {
		return ReferenceGenerationProgress{}, err
	}
	value.ContentHash, err = canonical.Hash(raw)
	if err != nil {
		return ReferenceGenerationProgress{}, err
	}
	return value, nil
}

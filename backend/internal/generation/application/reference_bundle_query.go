package application

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

type ReferenceBundleInputReader interface {
	ReadReferenceBundleInputs(context.Context, Actor, string, string) (domain.ReferenceBundleInputCollection, error)
}

type ReferenceBundleQuery struct{ reader ReferenceBundleInputReader }

func NewReferenceBundleQuery(reader ReferenceBundleInputReader) *ReferenceBundleQuery {
	return &ReferenceBundleQuery{reader: reader}
}

func (query *ReferenceBundleQuery) Get(ctx context.Context, actor Actor, projectID, executionID string) (domain.ReferenceBundleInputCollection, error) {
	if !validUUID(actor.UserID) || actor.TokenVersion < 1 || !validUUID(projectID) || !validUUID(executionID) {
		return domain.ReferenceBundleInputCollection{}, invalid("Invalid Reference Bundle query")
	}
	if query == nil || query.reader == nil {
		return domain.ReferenceBundleInputCollection{}, errors.New("Reference Bundle reader is unavailable")
	}
	value, err := query.reader.ReadReferenceBundleInputs(ctx, actor, projectID, executionID)
	if err != nil {
		return domain.ReferenceBundleInputCollection{}, err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return domain.ReferenceBundleInputCollection{}, err
	}
	checked, err := domain.DecodeReferenceBundleInputs(raw)
	if err != nil || checked.ExecutionRef.ID != executionID {
		return domain.ReferenceBundleInputCollection{}, conflict("Reference Bundle query identity has drifted")
	}
	return checked, nil
}

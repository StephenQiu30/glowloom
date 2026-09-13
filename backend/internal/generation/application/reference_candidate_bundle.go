package application

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

type ReferenceCandidateBundleCommand struct {
	WorkspaceID     string
	ProjectID       string
	ExecutionRef    domain.GenerationRevisionRef
	VisionReviewRef domain.GenerationRevisionRef
}

type ReferenceCandidateBundlePersistence interface {
	MaterializeReferenceCandidateBundle(context.Context, Actor, ReferenceCandidateBundleCommand) (domain.ReferenceCandidateBundle, error)
	ReadReferenceCandidateBundle(context.Context, Actor, string, string) (domain.ReferenceCandidateBundle, error)
}

type ReferenceCandidateBundleService struct {
	persistence ReferenceCandidateBundlePersistence
}

func NewReferenceCandidateBundleService(persistence ReferenceCandidateBundlePersistence) *ReferenceCandidateBundleService {
	return &ReferenceCandidateBundleService{persistence: persistence}
}

func (service *ReferenceCandidateBundleService) Materialize(ctx context.Context, actor Actor, command ReferenceCandidateBundleCommand) (domain.ReferenceCandidateBundle, error) {
	if !validUUID(actor.UserID) || actor.TokenVersion < 1 || !validUUID(command.WorkspaceID) || !validUUID(command.ProjectID) || !command.ExecutionRef.Valid() || command.ExecutionRef.Revision != 1 || !command.VisionReviewRef.Valid() || command.VisionReviewRef.Revision != 1 {
		return domain.ReferenceCandidateBundle{}, invalid("Invalid candidate Bundle command")
	}
	if service == nil || service.persistence == nil {
		return domain.ReferenceCandidateBundle{}, errors.New("candidate Bundle Owner unavailable")
	}
	value, err := service.persistence.MaterializeReferenceCandidateBundle(ctx, actor, command)
	if err != nil {
		return domain.ReferenceCandidateBundle{}, err
	}
	value, err = checkedReferenceCandidateBundle(value)
	if err != nil || value.WorkspaceID != command.WorkspaceID || value.ProjectID != command.ProjectID || value.ExecutionRef != command.ExecutionRef || value.VisionReviewRef != command.VisionReviewRef {
		return domain.ReferenceCandidateBundle{}, conflict("Candidate Bundle identity changed")
	}
	return value, nil
}

func (service *ReferenceCandidateBundleService) Get(ctx context.Context, actor Actor, projectID, bundleID string) (domain.ReferenceCandidateBundle, error) {
	if !validUUID(actor.UserID) || actor.TokenVersion < 1 || !validUUID(projectID) || !validUUID(bundleID) {
		return domain.ReferenceCandidateBundle{}, invalid("Invalid candidate Bundle query")
	}
	if service == nil || service.persistence == nil {
		return domain.ReferenceCandidateBundle{}, errors.New("candidate Bundle Owner unavailable")
	}
	value, err := service.persistence.ReadReferenceCandidateBundle(ctx, actor, projectID, bundleID)
	if err != nil {
		return domain.ReferenceCandidateBundle{}, err
	}
	value, err = checkedReferenceCandidateBundle(value)
	if err != nil || value.ProjectID != projectID || value.ID != bundleID {
		return domain.ReferenceCandidateBundle{}, conflict("Candidate Bundle query identity changed")
	}
	return value, nil
}

func checkedReferenceCandidateBundle(value domain.ReferenceCandidateBundle) (domain.ReferenceCandidateBundle, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return domain.ReferenceCandidateBundle{}, err
	}
	return domain.DecodeReferenceCandidateBundle(raw)
}

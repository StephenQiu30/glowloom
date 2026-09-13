package application

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

type ReferenceCandidateSetCommand struct {
	ProjectID            string
	ExecutionRef         domain.GenerationRevisionRef
	ExpectedProgressHash string
	BundleRefs           []domain.GenerationRevisionRef
}

type ReferenceCandidateSetPersistence interface {
	MaterializeReferenceCandidateSet(context.Context, Actor, ReferenceCandidateSetCommand) (domain.ReferenceCandidateSet, error)
	ReadReferenceCandidateSet(context.Context, Actor, string, string) (domain.ReferenceCandidateSet, error)
}
type ReferenceCandidateSetService struct {
	persistence ReferenceCandidateSetPersistence
}

func NewReferenceCandidateSetService(persistence ReferenceCandidateSetPersistence) *ReferenceCandidateSetService {
	return &ReferenceCandidateSetService{persistence: persistence}
}

func (service *ReferenceCandidateSetService) Materialize(ctx context.Context, actor Actor, command ReferenceCandidateSetCommand) (domain.ReferenceCandidateSet, error) {
	if !validUUID(actor.UserID) || actor.TokenVersion < 1 || !validUUID(command.ProjectID) || !command.ExecutionRef.Valid() || command.ExecutionRef.Revision != 1 || !intentHashPattern.MatchString(command.ExpectedProgressHash) || command.BundleRefs == nil || len(command.BundleRefs) > 4 {
		return domain.ReferenceCandidateSet{}, invalid("Invalid candidate Set command")
	}
	seen := map[string]bool{}
	for _, ref := range command.BundleRefs {
		if !ref.Valid() || ref.Revision != 1 || seen[ref.ID] {
			return domain.ReferenceCandidateSet{}, invalid("Invalid candidate Set member")
		}
		seen[ref.ID] = true
	}
	if service == nil || service.persistence == nil {
		return domain.ReferenceCandidateSet{}, errors.New("candidate Set Owner unavailable")
	}
	value, err := service.persistence.MaterializeReferenceCandidateSet(ctx, actor, command)
	if err != nil {
		return domain.ReferenceCandidateSet{}, err
	}
	value, err = checkedReferenceCandidateSet(value)
	if err != nil || value.ProjectID != command.ProjectID || value.ExecutionRef != command.ExecutionRef || value.ExecutionProgressHash != command.ExpectedProgressHash || len(value.BundleRefs) != len(command.BundleRefs) {
		return domain.ReferenceCandidateSet{}, conflict("Candidate Set identity changed")
	}
	for _, member := range value.BundleRefs {
		if !slices.Contains(command.BundleRefs, member.BundleRef) {
			return domain.ReferenceCandidateSet{}, conflict("Candidate Set membership changed")
		}
	}
	return value, nil
}

func (service *ReferenceCandidateSetService) Get(ctx context.Context, actor Actor, projectID, setID string) (domain.ReferenceCandidateSet, error) {
	if !validUUID(actor.UserID) || actor.TokenVersion < 1 || !validUUID(projectID) || !validUUID(setID) {
		return domain.ReferenceCandidateSet{}, invalid("Invalid candidate Set query")
	}
	if service == nil || service.persistence == nil {
		return domain.ReferenceCandidateSet{}, errors.New("candidate Set Owner unavailable")
	}
	value, err := service.persistence.ReadReferenceCandidateSet(ctx, actor, projectID, setID)
	if err != nil {
		return domain.ReferenceCandidateSet{}, err
	}
	value, err = checkedReferenceCandidateSet(value)
	if err != nil || value.ProjectID != projectID || value.ID != setID {
		return domain.ReferenceCandidateSet{}, conflict("Candidate Set query identity changed")
	}
	return value, nil
}
func checkedReferenceCandidateSet(value domain.ReferenceCandidateSet) (domain.ReferenceCandidateSet, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return domain.ReferenceCandidateSet{}, err
	}
	return domain.DecodeReferenceCandidateSet(raw)
}

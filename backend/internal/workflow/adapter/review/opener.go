package review

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"

	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type Opener struct {
	service       *reviewapp.Service
	candidateSets CandidateSetSource
}

type CandidateSetSource interface {
	RequireCandidateSet(context.Context, generationapp.Actor, string) (generationdomain.CandidateSet, error)
}

func New(service *reviewapp.Service) *Opener {
	return &Opener{service: service}
}

func NewWithGeneration(service *reviewapp.Service, candidateSets CandidateSetSource) *Opener {
	return &Opener{service: service, candidateSets: candidateSets}
}

func (opener *Opener) OpenHumanTask(ctx context.Context, binding domain.HumanGateBinding) error {
	if opener == nil || opener.service == nil {
		return reviewapp.ErrNotFound
	}
	command := reviewapp.OpenCommand{
		WorkspaceID: binding.WorkspaceID, ProjectID: binding.ProjectID,
		WorkflowRunID: binding.WorkflowRunID, NodeRunID: binding.NodeRunID,
		SubjectType: binding.SubjectType, SubjectID: binding.SubjectID, SubjectRevision: binding.SubjectRevision,
		SubjectHash: binding.SubjectHash, CandidateIDs: binding.CandidateIDs, RubricVersion: binding.RubricVersion,
		AllowedDecisions: binding.AllowedDecisions,
	}
	if binding.Executor == "gate.generation_image_review" {
		if opener.candidateSets == nil || binding.InitiatorTokenVersion < 1 || !validUUID(binding.InitiatorUserID) ||
			binding.CandidateSet.Port != "candidates" || binding.CandidateSet.ValueType != "generation_candidate_set" ||
			binding.CandidateSet.SourceKind != domain.NodeInputSourceNodeOutput ||
			binding.CandidateSet.ReferenceVersion != "1" || !validUUID(binding.CandidateSet.ReferenceID) ||
			len(binding.CandidateSet.ContentHash) != 64 {
			return errors.New("invalid Generation CandidateSet Human Gate binding")
		}
		set, err := opener.candidateSets.RequireCandidateSet(ctx, generationapp.Actor{
			UserID: binding.InitiatorUserID, TokenVersion: binding.InitiatorTokenVersion,
		}, binding.CandidateSet.ReferenceID)
		if err != nil {
			return err
		}
		if set.ID != binding.CandidateSet.ReferenceID || set.WorkspaceID != binding.WorkspaceID ||
			set.ProjectID != binding.ProjectID || set.Revision != 1 || set.ContentHash != binding.CandidateSet.ContentHash ||
			len(set.Candidates) == 0 || len(set.Candidates) > 100 {
			return errors.New("Generation CandidateSet has drifted before HumanTask open")
		}
		command.SubjectType = "generation_candidate_selection"
		command.CandidateIDs = make([]string, len(set.Candidates))
		for index, candidate := range set.Candidates {
			if !validUUID(candidate.ID) {
				return errors.New("Generation CandidateSet has an invalid candidate")
			}
			command.CandidateIDs[index] = candidate.ID
		}
		slices.Sort(command.CandidateIDs)
		if len(slices.Compact(append([]string(nil), command.CandidateIDs...))) != len(command.CandidateIDs) {
			return errors.New("Generation CandidateSet has duplicate candidates")
		}
	}
	_, err := opener.service.Open(ctx, command)
	return err
}

func validUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}

var _ workflowapp.HumanTaskOpener = (*Opener)(nil)

package production

import (
	"context"
	"errors"
	"strconv"
	"strings"

	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const productionBibleConfirmOperation = "production_bible.confirm"

type BibleOwner interface {
	Confirm(context.Context, bibleapp.Actor, bibleapp.ConfirmCommand) (bibleapp.ConfirmResult, error)
}

type Applier struct{ bibles BibleOwner }

func New(bibles BibleOwner) *Applier { return &Applier{bibles: bibles} }

func (applier *Applier) ApplyHumanGateDecision(
	ctx context.Context,
	actor workflowapp.Actor,
	application domain.HumanGateOwnerApplication,
) (domain.HumanGateOwnerResult, error) {
	if applier == nil || applier.bibles == nil || strings.TrimSpace(actor.UserID) == "" ||
		application.Executor != "gate.production_bible_review" ||
		(application.Decision != "approved" && application.Decision != "selected") ||
		application.Candidate.ValueType != "production_bible_candidate" ||
		application.OutputPort != "bible" || application.OutputValueType != "production_bible" {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported workflow human gate owner application")
	}
	expectedRevision, err := strconv.Atoi(application.Candidate.ReferenceVersion)
	if err != nil || expectedRevision < 1 {
		return domain.HumanGateOwnerResult{}, errors.New("invalid production bible candidate revision")
	}
	result, err := applier.bibles.Confirm(ctx, bibleapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, bibleapp.ConfirmCommand{
		BibleID: application.Candidate.ReferenceID, ExpectedResultHash: application.Candidate.ContentHash,
		ExpectedRevision: expectedRevision, IdempotencyKey: "workflow-review:" + application.ReviewDecisionID,
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	if err = validateConfirmedBible(application, actor, result.Bible, result.Receipt.Operation, result.Receipt.ResourceID,
		result.Receipt.WorkspaceID, result.Receipt.CreatedBy); err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	output, _, outputHash, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: application.OutputPort, ValueType: application.OutputValueType,
			ReferenceID: result.Bible.ID, ReferenceVersion: strconv.Itoa(result.Bible.Revision),
			ContentHash: *result.Bible.ResultHash,
		}},
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	return domain.HumanGateOwnerResult{
		ReceiptID: result.Receipt.ID, Operation: result.Receipt.Operation, Output: output, OutputHash: outputHash,
	}, nil
}

func validateConfirmedBible(
	application domain.HumanGateOwnerApplication,
	actor workflowapp.Actor,
	bible bibledomain.Bible,
	operation string,
	resourceID string,
	workspaceID string,
	createdBy string,
) error {
	if bible.Status != "confirmed" || bible.ID != application.Candidate.ReferenceID ||
		bible.WorkspaceID != application.WorkspaceID || bible.ProjectID != application.ProjectID ||
		bible.ResultHash == nil || *bible.ResultHash != application.Candidate.ContentHash ||
		operation != productionBibleConfirmOperation || resourceID != bible.ID ||
		workspaceID != application.WorkspaceID || createdBy != actor.UserID {
		return errors.New("production bible owner result does not match workflow gate")
	}
	return nil
}

var _ workflowapp.HumanGateOwnerApplier = (*Applier)(nil)

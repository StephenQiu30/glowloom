package application

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/google/uuid"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

// ReferenceGenerationTargetReadRepository must use the caller's transaction.
// It cannot publish facts or create a missing authorization/receipt.
type ReferenceGenerationTargetReadRepository interface {
	AuthorizeReferenceGenerationProject(context.Context, Actor, string, string) error
	ReadReferenceGenerationBrief(context.Context, string, string, string, string) (agentapp.AcceptedReferenceBrief, error)
	ReadReferenceGenerationSource(context.Context, agentapp.AcceptedReferenceBrief) (ReferenceGenerationSourceCompilation, error)
	FindReferenceAuthorization(context.Context, string) (platformcommand.Receipt, error)
	ValidateReferenceGenerationCapabilities(context.Context, contract.ReferenceBriefInput) error
	FindReferenceGenerationTarget(context.Context, string, string, string) (ReferenceGenerationTarget, error)
	FindReferenceGenerationTargetReceipt(context.Context, string, string) (platformcommand.Receipt, error)
	ValidateReferenceGenerationTargetHead(context.Context, ReferenceGenerationTarget) error
}

type ReadReferenceGenerationTargetQuery struct {
	WorkspaceID, ProjectID string
	TargetRef              domain.GenerationRevisionRef
}

func (service *ReferenceGenerationTargetService) ReadCurrent(ctx context.Context, actor Actor, query ReadReferenceGenerationTargetQuery) (ReferenceGenerationTarget, error) {
	var result ReferenceGenerationTarget
	err := service.transactions.WithinReferenceGenerationTarget(ctx, func(repo ReferenceGenerationTargetRepository) error {
		var err error
		result, err = ReadCurrentReferenceGenerationTarget(ctx, repo, actor, query)
		return err
	})
	if err != nil {
		return ReferenceGenerationTarget{}, err
	}
	return result, nil
}

// ReadCurrentReferenceGenerationTarget revalidates execution input inside the
// caller's transaction; the returned value is not a durable execution permit.
func ReadCurrentReferenceGenerationTarget(ctx context.Context, repo ReferenceGenerationTargetReadRepository, actor Actor, query ReadReferenceGenerationTargetQuery) (ReferenceGenerationTarget, error) {
	for _, id := range []string{actor.UserID, query.WorkspaceID, query.ProjectID, query.TargetRef.ID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil || parsed.String() != id {
			return ReferenceGenerationTarget{}, invalid("Invalid Reference generation Target read scope")
		}
	}
	if repo == nil || actor.TokenVersion < 1 || query.TargetRef.Revision != 1 || !intentHashPattern.MatchString(query.TargetRef.ContentHash) {
		return ReferenceGenerationTarget{}, invalid("Invalid Reference generation Target read")
	}
	if err := repo.AuthorizeReferenceGenerationProject(ctx, actor, query.WorkspaceID, query.ProjectID); err != nil {
		return ReferenceGenerationTarget{}, err
	}
	persisted, err := repo.FindReferenceGenerationTarget(ctx, query.WorkspaceID, query.ProjectID, query.TargetRef.ID)
	if err != nil {
		return ReferenceGenerationTarget{}, err
	}
	raw, err := json.Marshal(persisted)
	if err != nil {
		return ReferenceGenerationTarget{}, err
	}
	persisted, err = DecodeReferenceGenerationTarget(raw)
	if err != nil || referenceGenerationTargetRef(persisted) != query.TargetRef || persisted.WorkspaceID != query.WorkspaceID || persisted.ProjectID != query.ProjectID {
		return ReferenceGenerationTarget{}, conflict("Reference generation Target identity has drifted")
	}
	authorizationReceipt, err := repo.FindReferenceAuthorization(ctx, persisted.GenerationAuthorizationRef.ID)
	if err != nil {
		return ReferenceGenerationTarget{}, err
	}
	authorization, err := domain.DecodeReferenceGenerationAuthorization(authorizationReceipt.Result)
	if err != nil || authorization.HumanActionRef != persisted.GenerationAuthorizationRef.ID || authorization.ContentHash != persisted.GenerationAuthorizationRef.ContentHash || authorization.AuthorizedBy != persisted.CreatedBy {
		return ReferenceGenerationTarget{}, conflict("Reference generation Target authorization has drifted")
	}
	// Preserve the original human action; a different operator must not replace
	// its actor or token when rebuilding the original publication input hash.
	author := Actor{UserID: authorization.AuthorizedBy, TokenVersion: authorization.MembershipTokenVersion}
	if author != actor {
		if err = repo.AuthorizeReferenceGenerationProject(ctx, author, query.WorkspaceID, query.ProjectID); err != nil {
			return ReferenceGenerationTarget{}, err
		}
	}
	command := BuildReferenceGenerationTargetCommand{
		WorkspaceID: query.WorkspaceID, ProjectID: query.ProjectID,
		AuthorizationID: authorization.HumanActionRef, AuthorizationHash: authorization.ContentHash,
		BriefRevisionID: persisted.ReferenceBriefRevisionRef.ID, BriefRevisionHash: persisted.ReferenceBriefRevisionRef.RevisionHash,
	}
	for _, slot := range persisted.OutputContract.Slots {
		command.SlotPolicies = append(command.SlotPolicies, ReferenceOutputSlotPolicy{ViewRole: slot.ViewRole, AllowedMediaTypes: slot.AllowedMediaTypes, AspectRatio: slot.AspectRatio, MinWidth: slot.MinWidth, MinHeight: slot.MinHeight, MaxBytes: slot.MaxBytes})
	}
	facts, err := readReferenceTargetInputs(ctx, repo, author, command)
	if err != nil {
		return ReferenceGenerationTarget{}, err
	}
	receipt, err := repo.FindReferenceGenerationTargetReceipt(ctx, query.WorkspaceID, query.TargetRef.ID)
	if err != nil {
		return ReferenceGenerationTarget{}, err
	}
	result, err := revalidateReferenceTargetPublication(author, persisted, receipt, facts)
	if err != nil {
		return ReferenceGenerationTarget{}, err
	}
	if err = repo.ValidateReferenceGenerationTargetHead(ctx, result); err != nil {
		return ReferenceGenerationTarget{}, err
	}
	return result, nil
}

type referenceTargetInputs struct {
	brief         agentapp.AcceptedReferenceBrief
	source        ReferenceGenerationSourceCompilation
	authorization domain.ReferenceGenerationAuthorization
	output        domain.ReferenceOutputContract
	readSetRoot   string
	inputHash     string
}

func readReferenceTargetInputs(ctx context.Context, repo ReferenceGenerationTargetReadRepository, actor Actor, command BuildReferenceGenerationTargetCommand) (referenceTargetInputs, error) {
	var facts referenceTargetInputs
	brief, err := repo.ReadReferenceGenerationBrief(ctx, command.WorkspaceID, command.ProjectID, command.BriefRevisionID, command.BriefRevisionHash)
	if err != nil {
		return facts, err
	}
	if brief.RevisionID != command.BriefRevisionID || brief.RevisionHash != command.BriefRevisionHash || brief.Input.WorkspaceID != command.WorkspaceID || brief.Input.ProjectID != command.ProjectID || brief.Candidate.ValidateFor(brief.Input) != nil {
		return facts, conflict("Reference Target Brief scope has drifted")
	}
	source, err := repo.ReadReferenceGenerationSource(ctx, brief)
	if err != nil {
		return facts, err
	}
	sourceHash, err := canonical.Hash(source.Payload)
	if err != nil || sourceHash != source.ContentHash || !intentHashPattern.MatchString(source.ProductionWorldOwnerSetHash) {
		return facts, conflict("Reference generation source compilation is invalid")
	}
	authorization, err := readReferenceTargetAuthorization(ctx, repo, actor, command, brief, source)
	if err != nil {
		return facts, err
	}
	if err = repo.ValidateReferenceGenerationCapabilities(ctx, brief.Input); err != nil {
		return facts, err
	}
	output, err := CompileReferenceOutputContract(brief.Input, brief.Candidate, authorization.RequestedCandidateBundleCount, command.SlotPolicies)
	if err != nil {
		// The accepted Brief was validated above; a caller's slot policy must
		// be reported as invalid input, retaining the compiler error for diagnosis.
		return facts, errors.Join(invalid("Invalid Reference output slot policies"), err)
	}
	root, err := referenceTargetReadSetHash(brief, source, authorization, output)
	if err != nil {
		return facts, err
	}
	inputHash, err := platformcommand.InputHash(struct {
		Actor                               Actor
		WorkspaceID, ProjectID, ReadSetRoot string
		ExpectedHeadRevision                int64
	}{actor, command.WorkspaceID, command.ProjectID, root, command.ExpectedHeadRevision})
	if err != nil {
		return facts, err
	}
	return referenceTargetInputs{brief, source, authorization, output, root, inputHash}, nil
}

func revalidateReferenceTargetPublication(actor Actor, persisted ReferenceGenerationTarget, receipt platformcommand.Receipt, facts referenceTargetInputs) (ReferenceGenerationTarget, error) {
	if receipt.InputHash != facts.inputHash {
		return ReferenceGenerationTarget{}, platformcommand.ErrInputMismatch
	}
	result, err := compileInitialReferenceTarget(persisted.ID, actor.UserID, persisted.CreatedAt, facts.brief, facts.source, facts.authorization, facts.output, facts.readSetRoot)
	if err != nil {
		return ReferenceGenerationTarget{}, err
	}
	id, idErr := uuid.Parse(receipt.ID)
	if idErr != nil || id == uuid.Nil || id.String() != receipt.ID || receipt.WorkspaceID != result.WorkspaceID || receipt.Operation != BuildReferenceGenerationTargetOperation || receipt.ResourceID != result.ID ||
		receipt.IdempotencyKey == "" || len(receipt.IdempotencyKey) > 200 || strings.TrimSpace(receipt.IdempotencyKey) != receipt.IdempotencyKey ||
		!reflect.DeepEqual(result, persisted) || receipt.CreatedBy != actor.UserID || !receipt.CreatedAt.Equal(persisted.CreatedAt) {
		return ReferenceGenerationTarget{}, conflict("Reference generation Target receipt has drifted")
	}
	var ref domain.GenerationRevisionRef
	if canonical.Decode(receipt.Result, &ref) != nil || ref != referenceGenerationTargetRef(result) {
		return ReferenceGenerationTarget{}, conflict("Reference generation Target receipt identity has drifted")
	}
	return result, nil
}

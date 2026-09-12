package application

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	owner "github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
)

const AuthorizeInitialReferenceGenerationOperation = "generation.reference.authorize_initial"

type AuthorizeInitialReferenceGenerationCommand struct {
	WorkspaceID          string `json:"workspace_id"`
	ProjectID            string `json:"project_id"`
	PlanVersionID        string `json:"plan_version_id"`
	PlanContentHash      string `json:"plan_content_hash"`
	TargetVersionID      string `json:"target_version_id"`
	TargetContentHash    string `json:"target_content_hash"`
	BriefRevisionID      string `json:"brief_revision_id"`
	BriefRevisionHash    string `json:"brief_revision_hash"`
	CandidateBundleCount int    `json:"candidate_bundle_count"`
	IdempotencyKey       string `json:"idempotency_key"`
}

type ReferenceGenerationAuthorizationRepository interface {
	AuthorizeReferenceGenerationProject(context.Context, Actor, string, string) error
	ReadReferenceGenerationBrief(context.Context, string, string, string, string) (agentapp.AcceptedReferenceBrief, error)
	ReadReferenceGenerationSource(context.Context, agentapp.AcceptedReferenceBrief) (ReferenceGenerationSourceCompilation, error)
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	EnsureReceipt(context.Context, platformcommand.Receipt) (platformcommand.Receipt, error)
}

type ReferenceGenerationAuthorizationTransactions interface {
	WithinReferenceGenerationAuthorization(context.Context, func(ReferenceGenerationAuthorizationRepository) error) error
}

type ReferenceGenerationAuthorizationService struct {
	transactions ReferenceGenerationAuthorizationTransactions
	now          func() time.Time
	newID        func() string
}

func NewReferenceGenerationAuthorizationService(transactions ReferenceGenerationAuthorizationTransactions, now func() time.Time, newID func() string) (*ReferenceGenerationAuthorizationService, error) {
	if transactions == nil || now == nil || newID == nil {
		return nil, errors.New("Reference generation authorization dependencies are required")
	}
	return &ReferenceGenerationAuthorizationService{transactions: transactions, now: now, newID: newID}, nil
}

func (service *ReferenceGenerationAuthorizationService) AuthorizeInitial(ctx context.Context, actor Actor, command AuthorizeInitialReferenceGenerationCommand) (domain.ReferenceGenerationAuthorization, error) {
	for _, id := range []string{actor.UserID, command.WorkspaceID, command.ProjectID, command.PlanVersionID, command.TargetVersionID, command.BriefRevisionID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil || parsed.String() != id {
			return domain.ReferenceGenerationAuthorization{}, invalid("Invalid Reference generation authorization scope")
		}
	}
	if actor.TokenVersion < 1 || command.CandidateBundleCount < 1 || command.CandidateBundleCount > 4 || len(command.IdempotencyKey) > 200 || command.IdempotencyKey == "" || strings.TrimSpace(command.IdempotencyKey) != command.IdempotencyKey ||
		!intentHashPattern.MatchString(command.PlanContentHash) || !intentHashPattern.MatchString(command.TargetContentHash) || !intentHashPattern.MatchString(command.BriefRevisionHash) {
		return domain.ReferenceGenerationAuthorization{}, invalid("Invalid Reference generation authorization request")
	}
	var result domain.ReferenceGenerationAuthorization
	err := service.transactions.WithinReferenceGenerationAuthorization(ctx, func(repo ReferenceGenerationAuthorizationRepository) error {
		if err := repo.AuthorizeReferenceGenerationProject(ctx, actor, command.WorkspaceID, command.ProjectID); err != nil {
			return err
		}
		brief, err := repo.ReadReferenceGenerationBrief(ctx, command.WorkspaceID, command.ProjectID, command.BriefRevisionID, command.BriefRevisionHash)
		if err != nil {
			return err
		}
		input := brief.Input
		if brief.RevisionID != command.BriefRevisionID || brief.RevisionHash != command.BriefRevisionHash || input.WorkspaceID != command.WorkspaceID || input.ProjectID != command.ProjectID ||
			input.ApprovedReferencePlanVersionRef.OwnerVersionID != command.PlanVersionID || input.ApprovedReferencePlanVersionRef.OwnerContentHash != command.PlanContentHash ||
			input.ReferencePlanTargetRef.OwnerVersionID != command.TargetVersionID || input.ReferencePlanTargetRef.OwnerContentHash != command.TargetContentHash ||
			brief.Candidate.ValidateFor(input) != nil {
			return conflict("Reference generation authorization input has drifted")
		}
		source, err := repo.ReadReferenceGenerationSource(ctx, brief)
		if err != nil {
			return err
		}
		sourceHash, err := canonical.Hash(source.Payload)
		if err != nil || sourceHash != source.ContentHash || !intentHashPattern.MatchString(source.ProductionWorldOwnerSetHash) {
			return conflict("Reference generation source compilation is invalid")
		}
		inputHash, err := platformcommand.InputHash(struct {
			Actor                       Actor
			Command                     AuthorizeInitialReferenceGenerationCommand
			SourceContentHash           string
			ProductionWorldOwnerSetHash string
		}{actor, command, source.ContentHash, source.ProductionWorldOwnerSetHash})
		if err != nil {
			return err
		}
		expected := domain.InitialReferenceGenerationAuthorizationInput{ApprovedReferencePlanVersionRef: referenceAuthorizationOwnerRef(input.ApprovedReferencePlanVersionRef), ReferencePlanTargetRef: referenceAuthorizationOwnerRef(input.ReferencePlanTargetRef), RequestedCandidateBundleCount: command.CandidateBundleCount, MembershipTokenVersion: actor.TokenVersion, AuthorizedBy: actor.UserID}
		receipt, err := repo.FindReceipt(ctx, command.WorkspaceID, AuthorizeInitialReferenceGenerationOperation, command.IdempotencyKey)
		if err == nil {
			result, err = replayReferenceGenerationAuthorization(receipt, inputHash, command, expected)
			return err
		}
		if !errors.Is(err, platformcommand.ErrReceiptNotFound) {
			return err
		}
		expected.HumanActionRef = service.newID()
		expected.AuthorizedAt = service.now().UTC().Truncate(time.Microsecond)
		result, err = domain.BuildInitialReferenceGenerationAuthorization(expected)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(result)
		if err != nil {
			return err
		}
		receipt, err = repo.EnsureReceipt(ctx, platformcommand.Receipt{ID: result.HumanActionRef, WorkspaceID: command.WorkspaceID, Operation: AuthorizeInitialReferenceGenerationOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: command.TargetVersionID, Result: raw, CreatedBy: actor.UserID, CreatedAt: result.AuthorizedAt})
		if err != nil {
			return err
		}
		result, err = replayReferenceGenerationAuthorization(receipt, inputHash, command, expected)
		return err
	})
	if err != nil {
		return domain.ReferenceGenerationAuthorization{}, err
	}
	return result, nil
}

func replayReferenceGenerationAuthorization(receipt platformcommand.Receipt, inputHash string, command AuthorizeInitialReferenceGenerationCommand, expected domain.InitialReferenceGenerationAuthorizationInput) (domain.ReferenceGenerationAuthorization, error) {
	if receipt.InputHash != inputHash {
		return domain.ReferenceGenerationAuthorization{}, platformcommand.ErrInputMismatch
	}
	value, err := domain.DecodeReferenceGenerationAuthorization(receipt.Result)
	if err != nil {
		return domain.ReferenceGenerationAuthorization{}, err
	}
	expected.HumanActionRef, expected.AuthorizedAt = receipt.ID, receipt.CreatedAt
	if receipt.WorkspaceID != command.WorkspaceID || receipt.Operation != AuthorizeInitialReferenceGenerationOperation || receipt.IdempotencyKey != command.IdempotencyKey || receipt.ResourceID != command.TargetVersionID || receipt.CreatedBy != expected.AuthorizedBy || !reflect.DeepEqual(value.InitialReferenceGenerationAuthorizationInput, expected) {
		return domain.ReferenceGenerationAuthorization{}, conflict("Reference generation authorization receipt has drifted")
	}
	return value, nil
}

func referenceAuthorizationOwnerRef(ref agentcontract.ReferencePlanOwnerRef) owner.VersionRef {
	return owner.VersionRef{WorkspaceID: ref.WorkspaceID, ProjectID: ref.ProjectID, OwnerKind: ref.OwnerKind, VersionFamily: ref.VersionFamily, OwnerLogicalID: ref.OwnerLogicalID, OwnerVersionID: ref.OwnerVersionID, OwnerRevision: ref.OwnerRevision, OwnerContentHash: ref.OwnerContentHash}
}

package application

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	presetdomain "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
	referencedomain "github.com/StephenQiu30/lanverse/backend/internal/production/reference/domain"
	storygraphdomain "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

var ErrVisualFoundationConfirmationConflict = errors.New("Visual Foundation confirmation input has changed")

type ExpectedHead struct {
	OwnerKind   string
	LogicalID   string
	Revision    int64
	ContentHash string
}

type CandidateRevision struct {
	ID           string
	Revision     int64
	RevisionHash string
	ContentHash  string
	Candidate    json.RawMessage
}

type ConfirmVisualFoundationCommand struct {
	CommandID                   string
	WorkspaceID                 string
	ProjectID                   string
	ActorID                     string
	GateInputID                 string
	GateInputHash               string
	ReviewDecisionID            string
	IdempotencyKey              string
	Selection                   presetdomain.ProjectSelection
	Release                     presetdomain.Release
	VisualCandidate             CandidateRevision
	ReferenceCandidate          CandidateRevision
	ProductionWorldOwnerSetHash string
	ReferenceTargetSeedRoot     string
	ExpectedTargetSet           storygraphdomain.ExpectedReferenceTargetSet
	Targets                     []referencedomain.TargetDraft
	ExpectedPresetHead          ExpectedHead
	ExpectedReferenceHead       ExpectedHead
}

type ConfirmationPersistence struct {
	PresetSet        presetdomain.EffectiveVisualFoundationSet
	ReferenceSet     referencedomain.ApprovedReferencePlanSet
	PresetReceipt    referencedomain.VisualScopeCollectionReceipt
	ReferenceReceipt referencedomain.VisualScopeCollectionReceipt
	CommandReceipt   platformcommand.Receipt
	Outbox           ConfirmationOutbox
}

type ConfirmationOutbox struct {
	ID, WorkspaceID, ProjectID, AggregateID, SourceReceiptID string
	AggregateRevision                                        int64
	Payload                                                  json.RawMessage
	PayloadHash                                              string
	OccurredAt                                               time.Time
}

type ConfirmationTransaction interface {
	FindCommandReceipt(context.Context, string, string) (platformcommand.Receipt, error)
	ValidateReadSet(context.Context, ConfirmVisualFoundationCommand) error
	PersistConfirmation(context.Context, ConfirmationPersistence) error
}

type ConfirmationStore interface {
	WithinVisualFoundationConfirmation(context.Context, func(ConfirmationTransaction) error) error
}

type ConfirmationService struct {
	store ConfirmationStore
	now   func() time.Time
}

func NewConfirmationService(store ConfirmationStore, now func() time.Time) *ConfirmationService {
	return &ConfirmationService{store: store, now: now}
}

func (service *ConfirmationService) ConfirmVisualFoundation(
	ctx context.Context,
	command ConfirmVisualFoundationCommand,
) (referencedomain.ConfirmVisualFoundationResult, error) {
	if service == nil || service.store == nil || service.now == nil {
		return referencedomain.ConfirmVisualFoundationResult{}, errors.New("Visual Foundation confirmation service is unavailable")
	}
	inputHash, err := platformcommand.InputHash(command)
	if err != nil {
		return referencedomain.ConfirmVisualFoundationResult{}, err
	}
	var result referencedomain.ConfirmVisualFoundationResult
	err = service.store.WithinVisualFoundationConfirmation(ctx, func(transaction ConfirmationTransaction) error {
		receipt, findErr := transaction.FindCommandReceipt(
			ctx, command.WorkspaceID, command.IdempotencyKey,
		)
		if findErr == nil {
			replayed, replayErr := platformcommand.Replay[referencedomain.ConfirmVisualFoundationResult](receipt, inputHash)
			if replayErr != nil {
				return replayErr
			}
			result = replayed
			return nil
		}
		if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		if validateErr := validateConfirmationCommand(command); validateErr != nil {
			return validateErr
		}
		if validateErr := transaction.ValidateReadSet(ctx, command); validateErr != nil {
			return validateErr
		}
		now := service.now().UTC()
		revision := command.ExpectedPresetHead.Revision + 1
		visualSet, buildErr := presetdomain.BuildEffectiveVisualFoundationSet(
			presetdomain.EffectiveVisualFoundationSetDraft{
				BindingVersionID: stableConfirmationID("binding", command),
				StyleSnapshotID:  stableConfirmationID("style", command),
				PolicySnapshotID: stableConfirmationID("policy", command),
				Revision:         revision, Selection: command.Selection, Release: command.Release,
				CandidateRevisionID:         command.VisualCandidate.ID,
				CandidateRevision:           command.VisualCandidate.Revision,
				CandidateRevisionHash:       command.VisualCandidate.RevisionHash,
				CandidateContentHash:        command.VisualCandidate.ContentHash,
				Candidate:                   command.VisualCandidate.Candidate,
				ProductionWorldOwnerSetHash: command.ProductionWorldOwnerSetHash,
				ReviewDecisionID:            command.ReviewDecisionID, CreatedBy: command.ActorID, CreatedAt: now,
			},
		)
		if buildErr != nil {
			return buildErr
		}
		styleRef := visualSet.Collection.Members[0]
		policyRef := visualSet.Collection.Members[1]
		if styleRef.OwnerVersionID != visualSet.Style.ID {
			styleRef, policyRef = policyRef, styleRef
		}
		targets := append([]referencedomain.TargetDraft(nil), command.Targets...)
		for index := range targets {
			targets[index].VersionID = stableConfirmationID("reference-target:"+targets[index].TargetBusinessKey, command)
		}
		referenceSet, buildErr := referencedomain.BuildApprovedReferencePlan(
			referencedomain.ApprovedReferencePlanDraft{
				PlanLogicalID: command.ExpectedReferenceHead.LogicalID,
				PlanVersionID: stableConfirmationID("reference-plan", command),
				Revision:      command.ExpectedReferenceHead.Revision + 1,
				WorkspaceID:   command.WorkspaceID, ProjectID: command.ProjectID,
				CandidateRevisionID:                   command.ReferenceCandidate.ID,
				CandidateRevision:                     command.ReferenceCandidate.Revision,
				CandidateRevisionHash:                 command.ReferenceCandidate.RevisionHash,
				CandidateContentHash:                  command.ReferenceCandidate.ContentHash,
				VisualFoundationCandidateRevisionID:   command.VisualCandidate.ID,
				VisualFoundationCandidateRevisionHash: command.VisualCandidate.RevisionHash,
				PresetReleaseContentHash:              command.Release.ContentHash,
				ProductionWorldOwnerSetHash:           command.ProductionWorldOwnerSetHash,
				ReferenceTargetSeedRoot:               command.ReferenceTargetSeedRoot,
				ExpectedTargetSet:                     command.ExpectedTargetSet,
				EffectiveStyleSnapshot:                styleRef, EffectivePolicySnapshot: policyRef,
				Targets: targets, ReviewDecisionID: command.ReviewDecisionID,
				CreatedBy: command.ActorID, CreatedAt: now,
			},
		)
		if buildErr != nil {
			return buildErr
		}
		presetReceipt, buildErr := referencedomain.NewVisualScopeCollectionReceipt(
			stableConfirmationID("preset-receipt", command), command.CommandID,
			command.GateInputID, command.ReviewDecisionID, command.ActorID,
			visualSet.Collection, now,
		)
		if buildErr != nil {
			return buildErr
		}
		referenceReceipt, buildErr := referencedomain.NewVisualScopeCollectionReceipt(
			stableConfirmationID("reference-receipt", command), command.CommandID,
			command.GateInputID, command.ReviewDecisionID, command.ActorID,
			referenceSet.Collection, now,
		)
		if buildErr != nil {
			return buildErr
		}
		result, buildErr = referencedomain.CompleteConfirmVisualFoundationResult(
			referencedomain.ConfirmVisualFoundationResult{
				CommandID:                  command.CommandID,
				CommandReceiptID:           stableConfirmationID("command-receipt", command),
				PresetCollectionReceipt:    presetReceipt,
				ReferenceCollectionReceipt: referenceReceipt,
				PlanVersionID:              referenceSet.Version.ID, PlanRevision: referenceSet.Version.Revision,
				PlanContentHash: referenceSet.Version.ContentHash,
				CommittedBy:     command.ActorID, CommittedAt: now,
			},
		)
		if buildErr != nil {
			return buildErr
		}
		resultJSON, buildErr := platformcommand.Result(result)
		if buildErr != nil {
			return buildErr
		}
		payload, buildErr := json.Marshal(struct {
			PlanVersionID, PlanContentHash, PresetCollectionRoot, ReferenceCollectionRoot string
			PlanRevision                                                                  int64
		}{
			result.PlanVersionID, result.PlanContentHash,
			result.PresetCollectionReceipt.Collection.CollectionRootHash,
			result.ReferenceCollectionReceipt.Collection.CollectionRootHash,
			result.PlanRevision,
		})
		if buildErr != nil {
			return buildErr
		}
		payloadHash, buildErr := platformcanonical.Hash(payload)
		if buildErr != nil {
			return buildErr
		}
		return transaction.PersistConfirmation(ctx, ConfirmationPersistence{
			PresetSet: visualSet, ReferenceSet: referenceSet,
			PresetReceipt: presetReceipt, ReferenceReceipt: referenceReceipt,
			CommandReceipt: platformcommand.Receipt{
				ID: result.CommandReceiptID, WorkspaceID: command.WorkspaceID,
				Operation:      referencedomain.ConfirmVisualFoundationOperation,
				IdempotencyKey: command.IdempotencyKey, InputHash: inputHash,
				ResourceID: result.PlanVersionID, Result: resultJSON,
				CreatedBy: command.ActorID, CreatedAt: now,
			},
			Outbox: ConfirmationOutbox{
				ID:          stableConfirmationID("outbox", command),
				WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
				AggregateID: result.PlanVersionID, SourceReceiptID: result.CommandReceiptID,
				AggregateRevision: result.PlanRevision, Payload: payload,
				PayloadHash: payloadHash, OccurredAt: now,
			},
		})
	})
	return result, normalizeConfirmationError(err)
}

func validateConfirmationCommand(command ConfirmVisualFoundationCommand) error {
	for _, identifier := range []string{
		command.CommandID, command.WorkspaceID, command.ProjectID, command.ActorID,
		command.GateInputID, command.ReviewDecisionID,
	} {
		if parsed, err := uuid.Parse(identifier); err != nil || parsed == uuid.Nil {
			return errors.New("invalid Visual Foundation confirmation identity")
		}
	}
	if command.IdempotencyKey == "" || len(command.GateInputHash) != 64 ||
		command.ExpectedPresetHead.OwnerKind != "preset" ||
		command.ExpectedPresetHead.LogicalID != command.ProjectID ||
		command.ExpectedReferenceHead.OwnerKind != "production/reference" ||
		uuid.Validate(command.ExpectedReferenceHead.LogicalID) != nil ||
		command.ExpectedPresetHead.Revision != command.ExpectedReferenceHead.Revision ||
		command.ExpectedPresetHead.Revision < 0 ||
		(command.ExpectedPresetHead.Revision == 0 &&
			(command.ExpectedPresetHead.ContentHash != "" || command.ExpectedReferenceHead.ContentHash != "")) ||
		(command.ExpectedPresetHead.Revision > 0 &&
			(len(command.ExpectedPresetHead.ContentHash) != 64 || len(command.ExpectedReferenceHead.ContentHash) != 64)) {
		return errors.New("invalid Visual Foundation confirmation command")
	}
	return nil
}

func stableConfirmationID(kind string, command ConfirmVisualFoundationCommand) string {
	return uuid.NewSHA1(
		uuid.NameSpaceURL,
		[]byte("lanverse:"+kind+":"+command.WorkspaceID+":"+command.ReviewDecisionID),
	).String()
}

func normalizeConfirmationError(err error) error {
	if errors.Is(err, platformcommand.ErrInputMismatch) {
		return ErrVisualFoundationConfirmationConflict
	}
	return err
}

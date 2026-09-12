package application

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

const BuildReferenceGenerationTargetOperation = "generation.reference.build_target"

type ReferenceGenerationAuthorizationRef struct {
	ID          string `json:"id"`
	ContentHash string `json:"content_hash"`
}

type ReferenceBriefRevisionRef struct {
	ID           string `json:"id"`
	Revision     int64  `json:"revision"`
	RevisionHash string `json:"revision_hash"`
	ContentHash  string `json:"content_hash"`
}

// ReferenceGenerationTarget is the Provider-neutral, immutable publication.
// Integrity decoding does not replace current Owner and authorization checks.
type ReferenceGenerationTarget struct {
	ID                              string                              `json:"target_id"`
	ContractID                      string                              `json:"contract_id"`
	Revision                        int64                               `json:"revision"`
	ContentHash                     string                              `json:"content_hash"`
	WorkspaceID                     string                              `json:"workspace_id"`
	ProjectID                       string                              `json:"project_id"`
	ApprovedReferencePlanVersionRef contract.ReferencePlanOwnerRef      `json:"approved_reference_plan_version_ref"`
	ReferencePlanTargetRef          contract.ReferencePlanOwnerRef      `json:"reference_plan_target_ref"`
	TargetBusinessKey               string                              `json:"target_business_key"`
	TargetKind                      string                              `json:"target_kind"`
	Fulfillment                     string                              `json:"fulfillment"`
	GenerationRound                 int64                               `json:"generation_round"`
	GenerationAuthorizationRef      ReferenceGenerationAuthorizationRef `json:"generation_authorization_ref"`
	ReferenceBriefRevisionRef       ReferenceBriefRevisionRef           `json:"reference_brief_candidate_revision_ref"`
	EffectiveStyleSnapshotRef       contract.ReferencePlanOwnerRef      `json:"effective_style_snapshot_ref"`
	EffectivePolicySnapshotRef      contract.ReferencePlanOwnerRef      `json:"effective_policy_snapshot_ref"`
	SourcePayload                   json.RawMessage                     `json:"source_payload"`
	DependencyAssetVersionRefs      []contract.ReferencePlanOwnerRef    `json:"dependency_asset_version_refs"`
	DependencyRootHash              string                              `json:"dependency_root_hash"`
	OutputContract                  domain.ReferenceOutputContract      `json:"output_contract"`
	TargetReadSetRoot               string                              `json:"target_read_set_root"`
	CreatedBy                       string                              `json:"created_by"`
	CreatedAt                       time.Time                           `json:"created_at"`
}

type BuildReferenceGenerationTargetCommand struct {
	WorkspaceID, ProjectID             string
	AuthorizationID, AuthorizationHash string
	BriefRevisionID, BriefRevisionHash string
	ExpectedHeadRevision               int64
	SlotPolicies                       []ReferenceOutputSlotPolicy
	IdempotencyKey                     string
}

type ReferenceGenerationTargetRepository interface {
	ReferenceGenerationAuthorizationRepository
	FindReferenceAuthorization(context.Context, string) (platformcommand.Receipt, error)
	ValidateReferenceGenerationCapabilities(context.Context, contract.ReferenceBriefInput) error
	FindReferenceGenerationTarget(context.Context, string, string, string) (ReferenceGenerationTarget, error)
	PublishInitialReferenceGenerationTarget(context.Context, ReferenceGenerationTarget) error
	ValidateReferenceGenerationTargetHead(context.Context, ReferenceGenerationTarget) error
}

type ReferenceGenerationTargetTransactions interface {
	WithinReferenceGenerationTarget(context.Context, func(ReferenceGenerationTargetRepository) error) error
}

type ReferenceGenerationTargetService struct {
	transactions ReferenceGenerationTargetTransactions
	now          func() time.Time
	newID        func() string
}

func NewReferenceGenerationTargetService(transactions ReferenceGenerationTargetTransactions, now func() time.Time, newID func() string) (*ReferenceGenerationTargetService, error) {
	if transactions == nil || now == nil || newID == nil {
		return nil, errors.New("Reference generation Target dependencies are required")
	}
	return &ReferenceGenerationTargetService{transactions: transactions, now: now, newID: newID}, nil
}

func (service *ReferenceGenerationTargetService) BuildInitial(ctx context.Context, actor Actor, command BuildReferenceGenerationTargetCommand) (ReferenceGenerationTarget, error) {
	for _, id := range []string{command.WorkspaceID, command.ProjectID, command.AuthorizationID, command.BriefRevisionID, actor.UserID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil || parsed.String() != id {
			return ReferenceGenerationTarget{}, invalid("Invalid Reference generation Target scope")
		}
	}
	if actor.TokenVersion < 1 || command.ExpectedHeadRevision != 0 || !intentHashPattern.MatchString(command.AuthorizationHash) || !intentHashPattern.MatchString(command.BriefRevisionHash) || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 || strings.TrimSpace(command.IdempotencyKey) != command.IdempotencyKey {
		return ReferenceGenerationTarget{}, invalid("Invalid initial Reference generation Target command")
	}
	var result ReferenceGenerationTarget
	err := service.transactions.WithinReferenceGenerationTarget(ctx, func(repo ReferenceGenerationTargetRepository) error {
		if err := repo.AuthorizeReferenceGenerationProject(ctx, actor, command.WorkspaceID, command.ProjectID); err != nil {
			return err
		}
		brief, err := repo.ReadReferenceGenerationBrief(ctx, command.WorkspaceID, command.ProjectID, command.BriefRevisionID, command.BriefRevisionHash)
		if err != nil {
			return err
		}
		if brief.RevisionID != command.BriefRevisionID || brief.RevisionHash != command.BriefRevisionHash || brief.Input.WorkspaceID != command.WorkspaceID || brief.Input.ProjectID != command.ProjectID || brief.Candidate.ValidateFor(brief.Input) != nil {
			return conflict("Reference Target Brief scope has drifted")
		}
		source, err := repo.ReadReferenceGenerationSource(ctx, brief)
		if err != nil {
			return err
		}
		authorization, err := readReferenceTargetAuthorization(ctx, repo, actor, command, brief, source)
		if err != nil {
			return err
		}
		if err = repo.ValidateReferenceGenerationCapabilities(ctx, brief.Input); err != nil {
			return err
		}
		output, err := CompileReferenceOutputContract(brief.Input, brief.Candidate, authorization.RequestedCandidateBundleCount, command.SlotPolicies)
		if err != nil {
			return err
		}
		readSetRoot, err := referenceTargetReadSetHash(brief, source, authorization, output)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(struct {
			Actor                               Actor
			WorkspaceID, ProjectID, ReadSetRoot string
			ExpectedHeadRevision                int64
		}{actor, command.WorkspaceID, command.ProjectID, readSetRoot, command.ExpectedHeadRevision})
		if err != nil {
			return err
		}
		receipt, err := repo.FindReceipt(ctx, command.WorkspaceID, BuildReferenceGenerationTargetOperation, command.IdempotencyKey)
		if err == nil {
			if receipt.InputHash != inputHash {
				return platformcommand.ErrInputMismatch
			}
			persisted, readErr := repo.FindReferenceGenerationTarget(ctx, command.WorkspaceID, command.ProjectID, receipt.ResourceID)
			if readErr != nil {
				return readErr
			}
			result, err = compileInitialReferenceTarget(persisted.ID, actor.UserID, persisted.CreatedAt, brief, source, authorization, output, readSetRoot)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(result, persisted) || receipt.CreatedBy != actor.UserID || !receipt.CreatedAt.Equal(persisted.CreatedAt) {
				return conflict("Reference generation Target receipt has drifted")
			}
			var ref ReferenceGenerationTargetRef
			if canonical.Decode(receipt.Result, &ref) != nil || ref != referenceGenerationTargetRef(result) {
				return conflict("Reference generation Target receipt identity has drifted")
			}
			return repo.ValidateReferenceGenerationTargetHead(ctx, result)
		}
		if !errors.Is(err, platformcommand.ErrReceiptNotFound) {
			return err
		}
		result, err = compileInitialReferenceTarget(service.newID(), actor.UserID, service.now().UTC().Truncate(time.Microsecond), brief, source, authorization, output, readSetRoot)
		if err != nil {
			return err
		}
		if err = repo.PublishInitialReferenceGenerationTarget(ctx, result); err != nil {
			return err
		}
		raw, err := json.Marshal(referenceGenerationTargetRef(result))
		if err != nil {
			return err
		}
		_, err = repo.EnsureReceipt(ctx, platformcommand.Receipt{ID: service.newID(), WorkspaceID: command.WorkspaceID, Operation: BuildReferenceGenerationTargetOperation, IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: result.ID, Result: raw, CreatedBy: actor.UserID, CreatedAt: result.CreatedAt})
		return err
	})
	if err != nil {
		return ReferenceGenerationTarget{}, err
	}
	return result, nil
}

type ReferenceGenerationTargetRef struct {
	ID          string `json:"id"`
	Revision    int64  `json:"revision"`
	ContentHash string `json:"content_hash"`
}

func referenceGenerationTargetRef(value ReferenceGenerationTarget) ReferenceGenerationTargetRef {
	return ReferenceGenerationTargetRef{value.ID, value.Revision, value.ContentHash}
}

func readReferenceTargetAuthorization(ctx context.Context, repo ReferenceGenerationTargetRepository, actor Actor, command BuildReferenceGenerationTargetCommand, brief agentapp.AcceptedReferenceBrief, source ReferenceGenerationSourceCompilation) (domain.ReferenceGenerationAuthorization, error) {
	receipt, err := repo.FindReferenceAuthorization(ctx, command.AuthorizationID)
	if err != nil {
		return domain.ReferenceGenerationAuthorization{}, err
	}
	authorization, err := domain.DecodeReferenceGenerationAuthorization(receipt.Result)
	if err != nil || authorization.ContentHash != command.AuthorizationHash || authorization.HumanActionRef != command.AuthorizationID {
		return domain.ReferenceGenerationAuthorization{}, conflict("Reference generation authorization identity has drifted")
	}
	input := brief.Input
	original := AuthorizeInitialReferenceGenerationCommand{WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, PlanVersionID: input.ApprovedReferencePlanVersionRef.OwnerVersionID, PlanContentHash: input.ApprovedReferencePlanVersionRef.OwnerContentHash, TargetVersionID: input.ReferencePlanTargetRef.OwnerVersionID, TargetContentHash: input.ReferencePlanTargetRef.OwnerContentHash, BriefRevisionID: brief.RevisionID, BriefRevisionHash: brief.RevisionHash, CandidateBundleCount: authorization.RequestedCandidateBundleCount, IdempotencyKey: receipt.IdempotencyKey}
	hash, err := referenceAuthorizationInputHash(actor, original, source)
	if err != nil {
		return domain.ReferenceGenerationAuthorization{}, err
	}
	expected := domain.InitialReferenceGenerationAuthorizationInput{ApprovedReferencePlanVersionRef: referenceAuthorizationOwnerRef(input.ApprovedReferencePlanVersionRef), ReferencePlanTargetRef: referenceAuthorizationOwnerRef(input.ReferencePlanTargetRef), RequestedCandidateBundleCount: authorization.RequestedCandidateBundleCount, MembershipTokenVersion: actor.TokenVersion, AuthorizedBy: actor.UserID}
	return replayReferenceGenerationAuthorization(receipt, hash, original, expected)
}

func referenceTargetReadSetHash(brief agentapp.AcceptedReferenceBrief, source ReferenceGenerationSourceCompilation, authorization domain.ReferenceGenerationAuthorization, output domain.ReferenceOutputContract) (string, error) {
	raw, err := json.Marshal(struct {
		ContractID           string                    `json:"contract_id"`
		BriefInputRoot       string                    `json:"brief_input_root"`
		BriefRevision        ReferenceBriefRevisionRef `json:"brief_revision"`
		SourceHash           string                    `json:"source_hash"`
		WorldOwnerSetHash    string                    `json:"world_owner_set_hash"`
		AuthorizationHash    string                    `json:"authorization_hash"`
		OutputHash           string                    `json:"output_hash"`
		ExpectedHeadRevision int64                     `json:"expected_head_revision"`
	}{"reference-generation-target-read-set-production", brief.Input.TypedReadSetRoot, ReferenceBriefRevisionRef{brief.RevisionID, brief.Revision, brief.RevisionHash, brief.ContentHash}, source.ContentHash, source.ProductionWorldOwnerSetHash, authorization.ContentHash, output.ContentHash, 0})
	if err != nil {
		return "", err
	}
	return canonical.Hash(raw)
}

func compileInitialReferenceTarget(id, actorID string, now time.Time, brief agentapp.AcceptedReferenceBrief, source ReferenceGenerationSourceCompilation, authorization domain.ReferenceGenerationAuthorization, output domain.ReferenceOutputContract, readSetRoot string) (ReferenceGenerationTarget, error) {
	input := brief.Input
	dependencies := []contract.ReferencePlanOwnerRef{}
	if len(input.DependencySelections) != 0 {
		return ReferenceGenerationTarget{}, invalid("Initial base Target cannot omit dependency AssetVersions")
	}
	dependencyRoot, err := canonical.Hash([]byte("[]"))
	if err != nil {
		return ReferenceGenerationTarget{}, err
	}
	value := ReferenceGenerationTarget{ID: id, ContractID: "generation-target-production", Revision: 1, WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID, ApprovedReferencePlanVersionRef: input.ApprovedReferencePlanVersionRef, ReferencePlanTargetRef: input.ReferencePlanTargetRef, TargetBusinessKey: input.TargetBusinessKey, TargetKind: input.TargetKind, Fulfillment: input.TargetFulfillment, GenerationRound: 1, GenerationAuthorizationRef: ReferenceGenerationAuthorizationRef{authorization.HumanActionRef, authorization.ContentHash}, ReferenceBriefRevisionRef: ReferenceBriefRevisionRef{brief.RevisionID, brief.Revision, brief.RevisionHash, brief.ContentHash}, EffectiveStyleSnapshotRef: input.EffectiveStyleSnapshotRef, EffectivePolicySnapshotRef: input.EffectivePolicySnapshotRef, SourcePayload: source.Payload, DependencyAssetVersionRefs: dependencies, DependencyRootHash: dependencyRoot, OutputContract: output, TargetReadSetRoot: readSetRoot, CreatedBy: actorID, CreatedAt: now}
	value.ContentHash, err = referenceGenerationTargetHash(value)
	if err != nil {
		return ReferenceGenerationTarget{}, err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ReferenceGenerationTarget{}, err
	}
	return DecodeReferenceGenerationTarget(raw)
}

func referenceGenerationTargetHash(value ReferenceGenerationTarget) (string, error) {
	value.ContentHash, value.CreatedBy, value.CreatedAt = "", "", time.Time{}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return canonical.Hash(raw)
}

func DecodeReferenceGenerationTarget(raw json.RawMessage) (ReferenceGenerationTarget, error) {
	var value ReferenceGenerationTarget
	if err := canonical.Decode(raw, &value); err != nil {
		return ReferenceGenerationTarget{}, err
	}
	for _, id := range []string{value.ID, value.WorkspaceID, value.ProjectID, value.CreatedBy, value.GenerationAuthorizationRef.ID, value.ReferenceBriefRevisionRef.ID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil || parsed.String() != id {
			return ReferenceGenerationTarget{}, invalid("Invalid persisted Reference Target identity")
		}
	}
	hash, err := referenceGenerationTargetHash(value)
	emptyDependencyHash, dependencyErr := canonical.Hash([]byte("[]"))
	if err != nil || dependencyErr != nil || hash != value.ContentHash || value.ContractID != "generation-target-production" || value.Revision != 1 || value.GenerationRound != 1 || value.CreatedAt.IsZero() || value.DependencyAssetVersionRefs == nil || len(value.DependencyAssetVersionRefs) != 0 || value.DependencyRootHash != emptyDependencyHash || !intentHashPattern.MatchString(value.TargetReadSetRoot) || !slices.Contains([]string{"required", "optional"}, value.Fulfillment) || value.ReferenceBriefRevisionRef.Revision < 1 {
		return ReferenceGenerationTarget{}, conflict("Reference generation Target content has drifted")
	}
	for _, hash := range []string{value.GenerationAuthorizationRef.ContentHash, value.ReferenceBriefRevisionRef.RevisionHash, value.ReferenceBriefRevisionRef.ContentHash} {
		if !intentHashPattern.MatchString(hash) {
			return ReferenceGenerationTarget{}, conflict("Reference generation Target reference hash has drifted")
		}
	}
	for _, item := range []struct {
		ref          contract.ReferencePlanOwnerRef
		kind, family string
	}{
		{value.ApprovedReferencePlanVersionRef, "production/reference", "reference_plan_set"},
		{value.ReferencePlanTargetRef, "production/reference", "reference_plan_set"},
		{value.EffectiveStyleSnapshotRef, "preset", "preset_effective_set"},
		{value.EffectivePolicySnapshotRef, "preset", "preset_effective_set"},
	} {
		if !validReferenceTargetOwnerRef(item.ref, value, item.kind, item.family) || item.ref.FragmentKey != nil {
			return ReferenceGenerationTarget{}, conflict("Reference generation Target Owner scope has drifted")
		}
	}
	if value.TargetBusinessKey != value.ReferencePlanTargetRef.OwnerLogicalID || value.ApprovedReferencePlanVersionRef.OwnerRevision != value.ReferencePlanTargetRef.OwnerRevision {
		return ReferenceGenerationTarget{}, conflict("Reference generation Target Plan identity has drifted")
	}
	if err = validateReferenceTargetSource(value); err != nil {
		return ReferenceGenerationTarget{}, err
	}
	outputRaw, err := json.Marshal(value.OutputContract)
	if err != nil {
		return ReferenceGenerationTarget{}, err
	}
	if _, err = domain.DecodeReferenceOutputContract(outputRaw, value.TargetKind); err != nil {
		return ReferenceGenerationTarget{}, err
	}
	value.SourcePayload, err = canonical.JSON(value.SourcePayload)
	return value, err
}

func validReferenceTargetOwnerRef(ref contract.ReferencePlanOwnerRef, target ReferenceGenerationTarget, kind, family string) bool {
	parsed, err := uuid.Parse(ref.OwnerVersionID)
	if err != nil || parsed == uuid.Nil || parsed.String() != ref.OwnerVersionID || ref.WorkspaceID != target.WorkspaceID || ref.ProjectID != target.ProjectID || ref.OwnerKind != kind || ref.VersionFamily != family || strings.TrimSpace(ref.OwnerLogicalID) == "" || ref.OwnerRevision < 1 || !intentHashPattern.MatchString(ref.OwnerContentHash) {
		return false
	}
	if ref.FragmentKey == nil || ref.FragmentContentHash == nil {
		return ref.FragmentKey == nil && ref.FragmentContentHash == nil
	}
	return strings.TrimSpace(*ref.FragmentKey) != "" && intentHashPattern.MatchString(*ref.FragmentContentHash)
}

func validateReferenceTargetSource(target ReferenceGenerationTarget) error {
	var base ReferenceBaseSource
	var identity, specification, state contract.ReferencePlanOwnerRef
	var kind string
	switch target.TargetKind {
	case "character_identity_anchor":
		var value CharacterIdentityAnchorSource
		if err := canonical.Decode(target.SourcePayload, &value); err != nil {
			return err
		}
		base, identity, specification, state, kind = value.ReferenceBaseSource, value.IdentityRef, value.CharacterSpecificationRef, value.IdentityAnchorAssetStateRef, value.TargetKind
	case "location_board":
		var value LocationBoardSource
		if err := canonical.Decode(target.SourcePayload, &value); err != nil {
			return err
		}
		base, identity, specification, state, kind = value.ReferenceBaseSource, value.LocationIdentityRef, value.LocationSpecificationRef, value.LocationAssetStateRef, value.TargetKind
	case "prop_sheet":
		var value PropSheetSource
		if err := canonical.Decode(target.SourcePayload, &value); err != nil {
			return err
		}
		base, identity, specification, state, kind = value.ReferenceBaseSource, value.PropIdentityRef, value.PropSpecificationRef, value.PropAssetStateRef, value.TargetKind
	default:
		return invalid("Unsupported base Reference generation Target kind")
	}
	if kind != target.TargetKind || !slices.Equal(base.RequiredViewRoles, contract.ReferenceBriefRequiredViewRoles(kind)) || len(base.OccurrenceRefs) == 0 ||
		!validReferenceTargetOwnerRef(identity, target, "asset", "asset_identity_state_set") || !validReferenceTargetOwnerRef(state, target, "asset", "asset_identity_state_set") ||
		!validReferenceTargetOwnerRef(specification, target, "production/bible", "bible_production_world_set") || !validReferenceTargetOwnerRef(base.ProductionBindingRef, target, "production/bible", "bible_production_world_set") {
		return conflict("Reference generation Target source has drifted")
	}
	for _, ref := range base.OccurrenceRefs {
		if !validReferenceTargetOwnerRef(ref, target, "production/planning", "planning_scene_set") {
			return conflict("Reference generation Target occurrence scope has drifted")
		}
	}
	return nil
}

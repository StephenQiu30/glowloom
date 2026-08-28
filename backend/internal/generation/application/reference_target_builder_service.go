package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

const referenceTargetBuildOperation = "generation.reference_targets.build"

type ReferenceTargetRequirement struct {
	AssetID                 string                      `json:"asset_id"`
	AssetKind               string                      `json:"asset_kind"`
	SpecificationVersionRef domain.FrozenOwnerReference `json:"specification_version_ref"`
	SpecificationSnapshot   json.RawMessage             `json:"specification_snapshot"`
	AssetStateRef           domain.FrozenOwnerReference `json:"asset_state_ref"`
	AssetStateSnapshot      json.RawMessage             `json:"asset_state_snapshot"`
	RequiredViewRoles       []string                    `json:"required_view_roles"`
}

type ReferenceTargetSource struct {
	WorkspaceID               string                       `json:"workspace_id"`
	ProjectID                 string                       `json:"project_id"`
	ApprovedIntentSetRef      domain.FrozenOwnerReference  `json:"approved_intent_set_ref"`
	EffectiveStyleSnapshotRef domain.FrozenOwnerReference  `json:"effective_style_snapshot_ref"`
	VisualStyle               string                       `json:"visual_style"`
	AspectRatio               string                       `json:"aspect_ratio"`
	Requirements              []ReferenceTargetRequirement `json:"requirements"`
	CreatedBy                 string                       `json:"created_by"`
}

type ReferenceTargetBuilderRepository interface {
	LoadReferenceTargetSource(context.Context, Actor, string) (ReferenceTargetSource, error)
	FindReferenceTargetBuildReceipt(context.Context, string, string) (platformcommand.Receipt, error)
	EnsureReferenceTargetBuildReceipt(context.Context, platformcommand.Receipt) (platformcommand.Receipt, error)
	EnsureGenerationTarget(context.Context, domain.GenerationTarget) (domain.GenerationTarget, error)
	FindGenerationTarget(context.Context, string) (domain.GenerationTarget, error)
}

type ReferenceTargetBuilderTransactions interface {
	WithinReferenceTargetTransaction(context.Context, func(ReferenceTargetBuilderRepository) error) error
}

type ReferenceTargetBuilderConfig struct {
	Now   func() time.Time
	NewID func() string
}

type ReferenceTargetBuilderService struct {
	transactions ReferenceTargetBuilderTransactions
	config       ReferenceTargetBuilderConfig
}

type BuildReferenceTargetsCommand struct {
	ApprovedIntentSetID string `json:"approved_intent_set_id"`
	ExpectedContentHash string `json:"expected_content_hash"`
	IdempotencyKey      string `json:"idempotency_key"`
}

type BuildReferenceTargetsResult struct {
	Targets []domain.GenerationTarget
	Receipt platformcommand.Receipt
}

type referenceTargetReceiptItem struct {
	TargetID               string `json:"target_id"`
	TargetHash             string `json:"target_hash"`
	AssetID                string `json:"asset_id"`
	SpecificationVersionID string `json:"specification_version_id"`
	AssetStateID           string `json:"asset_state_id"`
}

type referenceTargetBuildReceipt struct {
	SchemaVersion         string                       `json:"schema_version"`
	ApprovedIntentSetID   string                       `json:"approved_intent_set_id"`
	ApprovedIntentSetHash string                       `json:"approved_intent_set_hash"`
	Targets               []referenceTargetReceiptItem `json:"targets"`
}

func NewReferenceTargetBuilderService(
	transactions ReferenceTargetBuilderTransactions,
	config ReferenceTargetBuilderConfig,
) *ReferenceTargetBuilderService {
	return &ReferenceTargetBuilderService{transactions: transactions, config: config}
}

func (service *ReferenceTargetBuilderService) BuildReferenceTargets(
	ctx context.Context,
	actor Actor,
	command BuildReferenceTargetsCommand,
) (BuildReferenceTargetsResult, error) {
	command.ApprovedIntentSetID = strings.TrimSpace(command.ApprovedIntentSetID)
	command.ExpectedContentHash = strings.TrimSpace(command.ExpectedContentHash)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.transactions == nil || service.config.Now == nil || service.config.NewID == nil ||
		actor.TokenVersion < 1 || !validReferenceTargetUUID(actor.UserID) ||
		!validReferenceTargetUUID(command.ApprovedIntentSetID) || !intentHashPattern.MatchString(command.ExpectedContentHash) ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return BuildReferenceTargetsResult{}, invalid("Invalid reference target build request")
	}

	var result BuildReferenceTargetsResult
	err := service.transactions.WithinReferenceTargetTransaction(ctx, func(repo ReferenceTargetBuilderRepository) error {
		source, err := repo.LoadReferenceTargetSource(ctx, actor, command.ApprovedIntentSetID)
		if err != nil {
			return err
		}
		requirements, err := normalizeReferenceTargetSource(&source, command)
		if err != nil {
			return err
		}
		inputHash, err := platformcommand.InputHash(struct {
			Command BuildReferenceTargetsCommand `json:"command"`
			Source  ReferenceTargetSource        `json:"source"`
		}{command, source})
		if err != nil {
			return err
		}
		if found, findErr := repo.FindReferenceTargetBuildReceipt(ctx, source.WorkspaceID, command.IdempotencyKey); findErr == nil {
			result, err = replayReferenceTargetBuild(ctx, repo, actor, source, requirements, inputHash, found)
			return err
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}

		now := service.config.Now().UTC()
		items := make([]referenceTargetReceiptItem, len(requirements))
		for index, requirement := range requirements {
			payload, buildErr := referenceAssetPayload(source, requirement)
			if buildErr != nil {
				return buildErr
			}
			target, buildErr := domain.NewGenerationTarget(domain.GenerationTargetInput{
				ID: service.config.NewID(), WorkspaceID: source.WorkspaceID, ProjectID: source.ProjectID,
				Kind: domain.GenerationTargetReferenceAsset, SourceOwnerRef: source.ApprovedIntentSetRef,
				PolicySnapshotRef: source.EffectiveStyleSnapshotRef, ReferenceAsset: &payload,
				Revision: 1, CreatedBy: actor.UserID, CreatedAt: now,
			})
			if buildErr != nil {
				return conflict("Approved reference target input is invalid")
			}
			target, buildErr = repo.EnsureGenerationTarget(ctx, target)
			if buildErr != nil {
				return buildErr
			}
			items[index] = referenceTargetReceiptItem{
				TargetID: target.ID, TargetHash: target.TargetHash, AssetID: requirement.AssetID,
				SpecificationVersionID: requirement.SpecificationVersionRef.ID, AssetStateID: requirement.AssetStateRef.ID,
			}
		}
		encoded, err := platformcommand.Result(referenceTargetBuildReceipt{
			SchemaVersion:         "generation-reference-target-build-v1",
			ApprovedIntentSetID:   source.ApprovedIntentSetRef.ID,
			ApprovedIntentSetHash: source.ApprovedIntentSetRef.ContentHash,
			Targets:               items,
		})
		if err != nil {
			return err
		}
		receipt, err := repo.EnsureReferenceTargetBuildReceipt(ctx, platformcommand.Receipt{
			ID: service.config.NewID(), WorkspaceID: source.WorkspaceID, Operation: referenceTargetBuildOperation,
			IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: source.ApprovedIntentSetRef.ID,
			Result: encoded, CreatedBy: actor.UserID, CreatedAt: now,
		})
		if err != nil {
			return err
		}
		result, err = replayReferenceTargetBuild(ctx, repo, actor, source, requirements, inputHash, receipt)
		return err
	})
	if err != nil {
		if errors.Is(err, platformcommand.ErrInputMismatch) || errors.Is(err, ErrGenerationTargetNotFound) {
			return BuildReferenceTargetsResult{}, conflict("Approved reference target facts have drifted")
		}
		return BuildReferenceTargetsResult{}, normalizeError(err)
	}
	return result, nil
}

func normalizeReferenceTargetSource(
	source *ReferenceTargetSource,
	command BuildReferenceTargetsCommand,
) ([]ReferenceTargetRequirement, error) {
	source.WorkspaceID, source.ProjectID = strings.TrimSpace(source.WorkspaceID), strings.TrimSpace(source.ProjectID)
	source.VisualStyle, source.AspectRatio, source.CreatedBy = strings.TrimSpace(source.VisualStyle), strings.TrimSpace(source.AspectRatio), strings.TrimSpace(source.CreatedBy)
	if !validReferenceTargetUUID(source.WorkspaceID) || !validReferenceTargetUUID(source.ProjectID) ||
		!validReferenceTargetUUID(source.CreatedBy) || source.ApprovedIntentSetRef.ID != command.ApprovedIntentSetID ||
		source.ApprovedIntentSetRef.ContentHash != command.ExpectedContentHash ||
		source.ApprovedIntentSetRef.Owner != "storyboard" || source.ApprovedIntentSetRef.Resource != "approved_storyboard_intents" ||
		source.ApprovedIntentSetRef.Revision != 1 || source.EffectiveStyleSnapshotRef.Owner != "preset" ||
		source.EffectiveStyleSnapshotRef.Resource != "effective_style_snapshot" || source.EffectiveStyleSnapshotRef.Revision < 1 ||
		strings.TrimSpace(source.VisualStyle) == "" || strings.TrimSpace(source.AspectRatio) == "" || len(source.Requirements) == 0 {
		return nil, conflict("Approved reference target source has drifted")
	}

	byIdentityState := make(map[string]ReferenceTargetRequirement, len(source.Requirements))
	for _, requirement := range source.Requirements {
		requirement.AssetID, requirement.AssetKind = strings.TrimSpace(requirement.AssetID), strings.TrimSpace(requirement.AssetKind)
		if !validReferenceTargetUUID(requirement.AssetID) {
			return nil, conflict("Approved reference target source has an invalid Asset identity")
		}
		if requirement.AssetKind != "character" {
			continue
		}
		if requirement.SpecificationVersionRef.Owner != "production" ||
			requirement.SpecificationVersionRef.Resource != "production_bible_specification_version" ||
			requirement.AssetStateRef.Owner != "asset" || requirement.AssetStateRef.Resource != "asset_state" ||
			!slices.Equal(requirement.RequiredViewRoles, []string{"front", "profile", "back"}) {
			return nil, conflict("Reference asset MVP only supports approved character reference sheets")
		}
		specification, err := canonicalReferenceTargetObject(requirement.SpecificationSnapshot)
		if err != nil {
			return nil, conflict("Approved character Specification snapshot has drifted")
		}
		state, err := canonicalReferenceTargetObject(requirement.AssetStateSnapshot)
		if err != nil {
			return nil, conflict("Approved character AssetState snapshot has drifted")
		}
		requirement.SpecificationSnapshot, requirement.AssetStateSnapshot = specification, state
		key := requirement.AssetID + "\x00" + requirement.AssetStateRef.ID
		if current, exists := byIdentityState[key]; exists {
			if current.SpecificationVersionRef != requirement.SpecificationVersionRef || current.AssetStateRef != requirement.AssetStateRef ||
				!bytes.Equal(current.SpecificationSnapshot, requirement.SpecificationSnapshot) ||
				!bytes.Equal(current.AssetStateSnapshot, requirement.AssetStateSnapshot) {
				return nil, conflict("Approved character reference requirements contain mixed immutable facts")
			}
			continue
		}
		byIdentityState[key] = requirement
	}
	if len(byIdentityState) == 0 {
		return nil, conflict("Reference asset MVP has no approved missing character reference sheet")
	}
	requirements := make([]ReferenceTargetRequirement, 0, len(byIdentityState))
	for _, requirement := range byIdentityState {
		requirements = append(requirements, requirement)
	}
	slices.SortFunc(requirements, func(left, right ReferenceTargetRequirement) int {
		return strings.Compare(
			left.AssetID+"\x00"+left.AssetStateRef.ID+"\x00"+left.SpecificationVersionRef.ID,
			right.AssetID+"\x00"+right.AssetStateRef.ID+"\x00"+right.SpecificationVersionRef.ID,
		)
	})
	source.Requirements = requirements
	return requirements, nil
}

func referenceAssetPayload(
	source ReferenceTargetSource,
	requirement ReferenceTargetRequirement,
) (domain.ReferenceAssetTarget, error) {
	positive := fmt.Sprintf(
		"Create one character reference sheet with exactly three full-body panels: left front view, center profile view, right back view. Keep one identical character identity and one identical appearance state across all panels. Character specification JSON: %s. Appearance state JSON: %s. Visual style: %s. Project aspect ratio context: %s.",
		requirement.SpecificationSnapshot, requirement.AssetStateSnapshot, source.VisualStyle, source.AspectRatio,
	)
	return domain.ReferenceAssetTarget{
		AssetID: requirement.AssetID, AssetKind: requirement.AssetKind,
		SpecificationVersionRef: requirement.SpecificationVersionRef, AssetStateRef: requirement.AssetStateRef,
		OutputKind: "reference_sheet", RequiredViewRoles: []string{"front", "profile", "back"},
		PromptVersion: "character-reference-sheet-v1", PositivePrompt: positive,
		NegativePrompt: "different identities, inconsistent face, inconsistent clothing, missing view, extra panel, cropped body, text, watermark, logo",
		Width:          1536, Height: 1024, NumberResults: 4, OutputFormat: "PNG",
	}, nil
}

func replayReferenceTargetBuild(
	ctx context.Context,
	repo ReferenceTargetBuilderRepository,
	actor Actor,
	source ReferenceTargetSource,
	requirements []ReferenceTargetRequirement,
	inputHash string,
	receipt platformcommand.Receipt,
) (BuildReferenceTargetsResult, error) {
	if receipt.InputHash != inputHash {
		return BuildReferenceTargetsResult{}, platformcommand.ErrInputMismatch
	}
	var recorded referenceTargetBuildReceipt
	if err := decodeReferenceTargetStrict(receipt.Result, &recorded); err != nil {
		return BuildReferenceTargetsResult{}, platformcommand.ErrInputMismatch
	}
	if !validReferenceTargetUUID(receipt.ID) || receipt.WorkspaceID != source.WorkspaceID ||
		receipt.Operation != referenceTargetBuildOperation || receipt.ResourceID != source.ApprovedIntentSetRef.ID ||
		receipt.CreatedBy != actor.UserID || recorded.SchemaVersion != "generation-reference-target-build-v1" ||
		recorded.ApprovedIntentSetID != source.ApprovedIntentSetRef.ID ||
		recorded.ApprovedIntentSetHash != source.ApprovedIntentSetRef.ContentHash || len(recorded.Targets) != len(requirements) {
		return BuildReferenceTargetsResult{}, platformcommand.ErrInputMismatch
	}
	targets := make([]domain.GenerationTarget, len(requirements))
	for index, requirement := range requirements {
		item := recorded.Targets[index]
		payload, err := referenceAssetPayload(source, requirement)
		if err != nil || item.AssetID != requirement.AssetID ||
			item.SpecificationVersionID != requirement.SpecificationVersionRef.ID || item.AssetStateID != requirement.AssetStateRef.ID ||
			!validReferenceTargetUUID(item.TargetID) || !intentHashPattern.MatchString(item.TargetHash) {
			return BuildReferenceTargetsResult{}, platformcommand.ErrInputMismatch
		}
		target, err := repo.FindGenerationTarget(ctx, item.TargetID)
		if err != nil || domain.ValidateGenerationTarget(target) != nil || target.TargetHash != item.TargetHash ||
			target.WorkspaceID != source.WorkspaceID || target.ProjectID != source.ProjectID ||
			target.SourceOwnerRef != source.ApprovedIntentSetRef || target.PolicySnapshotRef != source.EffectiveStyleSnapshotRef ||
			target.CreatedBy != actor.UserID || target.Kind != domain.GenerationTargetReferenceAsset ||
			target.ReferenceAsset == nil || !reflect.DeepEqual(*target.ReferenceAsset, payload) {
			return BuildReferenceTargetsResult{}, platformcommand.ErrInputMismatch
		}
		targets[index] = target
	}
	return BuildReferenceTargetsResult{Targets: targets, Receipt: receipt}, nil
}

func canonicalReferenceTargetObject(raw json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, errors.New("snapshot must be a JSON object")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("snapshot contains multiple JSON values")
	}
	return json.Marshal(value)
}

func decodeReferenceTargetStrict(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("JSON contains multiple values")
	}
	return nil
}

func validReferenceTargetUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}
